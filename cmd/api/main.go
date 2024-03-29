package main

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/danielsussa/mt5-to-exante/internal/controller"
	"github.com/danielsussa/mt5-to-exante/internal/exante"
	"github.com/danielsussa/mt5-to-exante/internal/exchanges"
	"github.com/danielsussa/mt5-to-exante/internal/orderdb"
	"github.com/danielsussa/mt5-to-exante/internal/utils"
	"github.com/joho/godotenv"
	"github.com/labstack/echo/v4"
)

var Hash string

func main() {
	ex, _ := os.Executable()
	exPath := filepath.Dir(ex)

	fmt.Println(fmt.Sprintf("version: %s", Hash))

	err := godotenv.Load(fmt.Sprintf("%s/%s.env", exPath, os.Args[1]))
	if err != nil {
		panic("cannot locate environment file")
	}

	exchangeApi, err := exchanges.New(fmt.Sprintf("%s/%s", exPath, os.Getenv("EXCHANGE_PATH")))
	if err != nil {
		panic(err)
	}

	orderState, err := orderdb.New(exPath)
	if err != nil {
		panic("cannot create local DB")
	}

	exanteApi := exante.NewApi(
		os.Getenv("BASE_URL"),
		os.Getenv("APPLICATION_ID"),
		os.Getenv("CLIENT_ID"),
		os.Getenv("SHARED_KEY"),
	)

	c := controller.New(exanteApi, *exchangeApi)

	h := api{
		accountID:   os.Getenv("ACCOUNT_ID"),
		exApi:       exanteApi,
		orderState:  orderState,
		exchangeApi: exchangeApi,
		controller:  c,
	}

	e := echo.New()

	e.GET("/health", func(c echo.Context) error {
		fmt.Println("health")
		return c.String(http.StatusOK, "ok")
	})

	e.GET("/jwt", h.getJwt)
	e.GET("/accounts", h.getAccounts)
	e.GET("/orders", h.getOrders)
	e.POST("/sync", h.sync)
	e.POST("/journal", h.journal)
	e.Logger.Fatal(e.Start(":1323"))
}

type api struct {
	accountID   string
	exApi       *exante.Api
	orderState  *orderdb.OrderState
	exchangeApi *exchanges.Api
	controller  *controller.Api
}

func (a api) getJwt(c echo.Context) error {
	return c.JSON(http.StatusOK, a.exApi.Jwt())
}
func (a api) getAccounts(c echo.Context) error {
	accounts, err := a.exApi.GetUserAccounts()
	if err != nil {
		return c.JSON(http.StatusBadRequest, echo.Map{
			"error": err.Error(),
		})
	}

	return c.JSON(http.StatusOK, accounts)
}

type placeOrderRequest struct {
	SymbolID   string  `json:"symbolID"`
	Duration   string  `json:"duration"`
	OrderType  string  `json:"orderType"`
	Quantity   float64 `json:"quantity"`
	Side       string  `json:"side"`
	AccountId  string  `json:"accountId"`
	LimitPrice float64 `json:"limitPrice"`
	TakeProfit float64 `json:"takeProfit"`
	StopLoss   float64 `json:"stopLoss"`
}

func (a api) placeOrder(c echo.Context) error {
	req := new(placeOrderRequest)
	if err := c.Bind(req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}

	exchange, has := a.exchangeApi.GetByMTValue(req.SymbolID)
	if !has {
		return c.JSON(http.StatusBadRequest, echo.Map{
			"error": "symbol not found",
		})
	}
	orderID := c.Param("orderID")

	orderDB, hasOrder := a.orderState.Get(orderID)
	if hasOrder {
		return c.JSON(http.StatusBadRequest, echo.Map{
			"error": "order already exist",
		})
	}

	// no active order
	orders, err := a.exApi.PlaceOrderV3(&exante.OrderSentTypeV3{
		SymbolID:   exchange.Exante,
		Duration:   req.Duration,
		OrderType:  req.OrderType,
		Quantity:   utils.Convert5Decimals(req.Quantity * exchange.PriceStep),
		Side:       req.Side,
		LimitPrice: utils.Convert5Decimals(req.LimitPrice),
		Instrument: exchange.Exante,
		StopLoss:   utils.Convert5DecimalsOrNil(req.StopLoss),
		TakeProfit: utils.Convert5DecimalsOrNil(req.TakeProfit),

		AccountID: req.AccountId,
		ClientTag: c.Param("orderID"),
	})
	if err != nil {
		return c.JSON(http.StatusBadRequest, echo.Map{
			"error": err.Error(),
		})
	}

	if len(orders) == 0 {
		return c.JSON(http.StatusBadRequest, echo.Map{
			"error": "no order registered",
		})
	}

	orderDB = orderdb.NewOrderGroup()

	parentOrder, has := utils.GetParentOrder(orders)
	if has {
		orderDB.Order = *utils.ConvertExOrderToDB(*parentOrder)
	}

	slOrder, has := utils.GetStopLossOrder(orders)
	if has {
		orderDB.StopLoss = utils.ConvertExOrderToDB(*slOrder)
	}

	tpOrder, has := utils.GetTakeProfitOrder(orders)
	if has {
		orderDB.TakeProfit = utils.ConvertExOrderToDB(*tpOrder)
	}

	a.orderState.Upsert(orderID, orderDB)

	return c.JSON(http.StatusOK, "ok")

}

type modifyOrderRequest struct {
	LimitPrice float64 `json:"limitPrice"`
	TakeProfit float64 `json:"takeProfit"`
	StopLoss   float64 `json:"stopLoss"`
}

func (a api) modifyOrder(c echo.Context) error {
	req := new(modifyOrderRequest)
	if err := c.Bind(req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}

	orderID := c.Param("orderID")

	orderDB, has := a.orderState.Get(orderID)
	if !has {
		return c.JSON(http.StatusBadRequest, echo.Map{
			"error": "no parent order registered",
		})
	}

	_, _ = a.exApi.ReplaceOrder(orderDB.Order.ID, exante.ReplaceOrderPayload{
		Action: "replace",
		Parameters: exante.ReplaceOrderParameters{
			Quantity:   orderDB.Order.Quantity,
			LimitPrice: utils.Convert5Decimals(req.LimitPrice),
		},
	})

	// cancel stop loss order
	if req.StopLoss == 0 && orderDB.StopLoss != nil {
		err := a.exApi.CancelOrder(orderDB.StopLoss.ID)
		if err == nil {
			orderDB.StopLoss = nil
		}
	}

	// cancel take profit order
	if req.TakeProfit == 0 && orderDB.TakeProfit != nil {
		err := a.exApi.CancelOrder(orderDB.TakeProfit.ID)
		if err == nil {
			orderDB.TakeProfit = nil
		}
	}

	if req.StopLoss > 0 {
		if orderDB.StopLoss == nil {
			orders, err := a.exApi.PlaceOrderV3(&exante.OrderSentTypeV3{
				SymbolID:       orderDB.Order.Symbol,
				Duration:       orderDB.Order.Duration,
				OrderType:      "stop",
				Quantity:       orderDB.Order.Quantity,
				Side:           utils.GetReverseOrderSide(orderDB.Order.Side),
				StopPrice:      utils.Convert5DecimalsOrNil(req.StopLoss),
				IfDoneParentID: orderDB.Order.ID,
				Instrument:     orderDB.Order.Symbol,
				OcoGroup:       orderDB.OcoGroup,
				AccountID:      orderDB.Order.AccountId,
			})
			if err == nil {
				orderDB.StopLoss = utils.ConvertExOrderToDB(orders[0])
			}

		} else {
			_, err := a.exApi.ReplaceOrder(orderDB.StopLoss.ID, exante.ReplaceOrderPayload{
				Action: "replace",
				Parameters: exante.ReplaceOrderParameters{
					Quantity:  orderDB.Order.Quantity,
					StopPrice: utils.Convert5Decimals(req.StopLoss),
				},
			})
			if err != nil {
				fmt.Println(fmt.Sprintf("error to replace order: %s", err.Error()))
			}
		}
	}

	if req.TakeProfit > 0 {
		if orderDB.TakeProfit == nil {
			orders, err := a.exApi.PlaceOrderV3(&exante.OrderSentTypeV3{
				SymbolID:       orderDB.Order.Symbol,
				Duration:       orderDB.Order.Duration,
				OrderType:      "limit",
				Quantity:       orderDB.Order.Quantity,
				Side:           utils.GetReverseOrderSide(orderDB.Order.Side),
				LimitPrice:     utils.Convert5Decimals(req.TakeProfit),
				Instrument:     orderDB.Order.Symbol,
				OcoGroup:       orderDB.OcoGroup,
				IfDoneParentID: orderDB.Order.ID,
				AccountID:      orderDB.Order.AccountId,
			})
			if err == nil {
				orderDB.TakeProfit = utils.ConvertExOrderToDB(orders[0])
			}
		} else {
			_, _ = a.exApi.ReplaceOrder(orderDB.TakeProfit.ID, exante.ReplaceOrderPayload{
				Action: "replace",
				Parameters: exante.ReplaceOrderParameters{
					Quantity:   orderDB.Order.Quantity,
					LimitPrice: utils.Convert5Decimals(req.TakeProfit),
				},
			})
		}
	}

	a.orderState.Upsert(orderID, orderDB)

	return c.JSON(http.StatusOK, "ok")

}

type cancelOrderRequest struct {
}

func (a api) cancelOrder(c echo.Context) error {
	req := new(cancelOrderRequest)
	if err := c.Bind(req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}

	orderId := c.Param("orderID")

	orderDB, hasOrder := a.orderState.Get(orderId)
	if !hasOrder {
		return c.JSON(http.StatusBadRequest, echo.Map{
			"error": "no active order found",
		})
	}

	err := a.exApi.CancelOrder(orderDB.Order.ID)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, echo.Map{
			"error": err.Error(),
		})
	}

	a.orderState.Delete(orderId)

	return c.JSON(http.StatusOK, "ok")
}

type closePositionRequest struct {
}

func (a api) closePosition(c echo.Context) error {
	req := new(closePositionRequest)
	if err := c.Bind(req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}

	orderId := c.Param("orderID")

	orderDB, hasOrder := a.orderState.Get(orderId)
	if !hasOrder {
		return c.JSON(http.StatusBadRequest, echo.Map{
			"error": "no active order found",
		})
	}

	_, err := a.exApi.PlaceOrderV3(&exante.OrderSentTypeV3{
		AccountID:  orderDB.Order.AccountId,
		Instrument: orderDB.Order.Symbol,
		Side:       utils.GetReverseOrderSide(orderDB.Order.Side),
		Quantity:   orderDB.Order.Quantity,
		OcoGroup:   orderDB.OcoGroup,
		Duration:   "good_till_cancel",
		OrderType:  "market",
		SymbolID:   orderDB.Order.Symbol,
	})
	if err != nil {
		return c.JSON(http.StatusInternalServerError, echo.Map{
			"error": err.Error(),
		})
	}

	a.orderState.Delete(orderId)

	return c.JSON(http.StatusOK, "ok")
}

func (a api) sync(c echo.Context) error {
	fmt.Println(fmt.Sprintf("%s - sync", time.Now().Format(time.RFC822Z)))

	req := new(controller.SyncRequest)
	if err := c.Bind(req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}

	res, err := a.controller.Sync(a.accountID, *req)
	if err != nil {
		fmt.Println("error processing sync: ", err.Error())
	}

	return c.JSON(http.StatusOK, res)
}

func (a api) getOrder(c echo.Context) error {
	orderDB, hasOrder := a.orderState.Get(c.Param("orderID"))
	if !hasOrder {
		return c.JSON(http.StatusBadRequest, echo.Map{
			"error": "no active order found",
		})
	}

	order, err := a.exApi.GetOrder(orderDB.Order.ID)
	if err != nil {
		return c.JSON(http.StatusBadRequest, echo.Map{
			"error": err.Error(),
		})
	}

	return c.JSON(http.StatusOK, order)
}

func (a api) getOrders(c echo.Context) error {
	orders, err := a.exApi.GetOrdersByLimitV3(5, a.accountID)
	if err != nil {
		return c.JSON(http.StatusBadRequest, echo.Map{
			"error": err.Error(),
		})
	}

	return c.JSON(http.StatusOK, orders)
}

func (a api) journal(c echo.Context) error {

	return c.JSON(http.StatusOK, controller.SyncResponse{
		JournalF: "dadadda\nnnndad\ndada",
	})
}
