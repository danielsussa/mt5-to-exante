package main

import (
	"fmt"
	"github.com/joho/godotenv"
	"github.com/labstack/echo/v4"
	"mt-to-exante/internal/exante"
	"mt-to-exante/internal/orderdb"
	"mt-to-exante/internal/utils"
	"net/http"
	"os"
)

func main() {
	err := godotenv.Load(fmt.Sprintf("%s.env", os.Args[1]))
	if err != nil {
		panic(err)
	}

	h := api{
		exApi: exante.NewApi(
			os.Getenv("BASE_URL"),
			os.Getenv("APPLICATION_ID"),
			os.Getenv("CLIENT_ID"),
			os.Getenv("SHARED_KEY"),
		),
		orderState: orderdb.New(),
	}

	e := echo.New()

	e.GET("/health", func(c echo.Context) error {
		fmt.Println("health")
		return c.String(http.StatusOK, "ok")
	})

	e.GET("/jwt", h.getJwt)
	e.GET("/accounts", h.getAccounts)
	e.POST("/v3/orders", h.getOrders)
	e.POST("/v3/orders/:orderID/place", h.placeOrder)
	e.POST("/v3/orders/:orderID/modify", h.modifyOrder)
	e.POST("/v3/orders/:orderID/cancel", h.cancelOrder)
	e.POST("/v3/positions/:orderID/close", h.closePosition)
	//e.POST("/v3/orders/:orderID/replace", h.replaceOrder)
	e.Logger.Fatal(e.Start(":1323"))
}

type api struct {
	exApi      *exante.Api
	orderState *orderdb.OrderState
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

func convertToSymbolInstrument(s string) (string, string, bool) {
	if len(s) != 6 {
		return "", "", false
	}

	return fmt.Sprintf("%s/%s.E.FX", s[0:3], s[3:]), fmt.Sprintf("%s/%s", s[0:3], s[3:]), true
}

func (a api) placeOrder(c echo.Context) error {
	req := new(placeOrderRequest)
	if err := c.Bind(req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}

	symbol, instrument, has := convertToSymbolInstrument(req.SymbolID)
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
		SymbolID:   symbol,
		Duration:   req.Duration,
		OrderType:  req.OrderType,
		Quantity:   utils.Convert5Decimals(req.Quantity),
		Side:       req.Side,
		LimitPrice: utils.Convert5Decimals(req.LimitPrice),
		Instrument: instrument,
		StopLoss:   floatToString(req.StopLoss),
		TakeProfit: floatToString(req.TakeProfit),

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

	return c.JSON(http.StatusOK, orders)

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

	a.exApi.ReplaceOrder(orderID, exante.ReplaceOrderPayload{
		Action: "replace",
		Parameters: exante.ReplaceOrderParameters{
			Quantity:   orderDB.Order.Quantity,
			LimitPrice: utils.Convert5Decimals(req.LimitPrice),
		},
	})

	if req.StopLoss > 0 {
		if orderDB.StopLoss == nil {
			orders, err := a.exApi.PlaceOrderV3(&exante.OrderSentTypeV3{
				SymbolID:   orderDB.Order.Symbol,
				Duration:   orderDB.Order.Duration,
				OrderType:  "stop",
				Quantity:   orderDB.Order.Quantity,
				Side:       utils.GetReverseOrderSide(orderDB.Order.Side),
				LimitPrice: utils.Convert5Decimals(req.StopLoss),
				Instrument: orderDB.Order.Instrument,
				OcoGroup:   orderDB.Order.OcoGroup,
				AccountID:  orderDB.Order.AccountId,
			})
			if err != nil {
				return c.JSON(http.StatusBadRequest, echo.Map{
					"error": err.Error(),
				})
			}
			orderDB.StopLoss = utils.ConvertExOrderToDB(orders[0])
		} else {
			a.exApi.ReplaceOrder(orderID, exante.ReplaceOrderPayload{
				Action: "replace",
				Parameters: exante.ReplaceOrderParameters{
					Quantity:   orderDB.Order.Quantity,
					LimitPrice: utils.Convert5Decimals(req.StopLoss),
				},
			})
		}
	}

	if req.TakeProfit > 0 {
		if orderDB.TakeProfit == nil {
			orders, err := a.exApi.PlaceOrderV3(&exante.OrderSentTypeV3{
				SymbolID:   orderDB.Order.Symbol,
				Duration:   orderDB.Order.Duration,
				OrderType:  "limit",
				Quantity:   orderDB.Order.Quantity,
				Side:       utils.GetReverseOrderSide(orderDB.Order.Side),
				LimitPrice: utils.Convert5Decimals(req.TakeProfit),
				Instrument: orderDB.Order.Instrument,
				OcoGroup:   orderDB.Order.OcoGroup,
				AccountID:  orderDB.Order.AccountId,
			})
			if err != nil {
				return c.JSON(http.StatusBadRequest, echo.Map{
					"error": err.Error(),
				})
			}
			orderDB.StopLoss = utils.ConvertExOrderToDB(orders[0])
		} else {
			a.exApi.ReplaceOrder(orderID, exante.ReplaceOrderPayload{
				Action: "replace",
				Parameters: exante.ReplaceOrderParameters{
					Quantity:   orderDB.Order.Quantity,
					LimitPrice: utils.Convert5Decimals(req.TakeProfit),
				},
			})
		}
	}

	return c.JSON(http.StatusOK, echo.Map{})

}

func floatToString(k float64) *string {
	if k > 0 {
		valS := utils.Convert5Decimals(k)
		return &valS
	}
	return nil
}

type cancelOrderRequest struct {
	exante.Api
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
			"error": fmt.Errorf("no active order found"),
		})
	}

	err := a.exApi.CancelOrder(orderDB.Order.ID)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, echo.Map{
			"error": err.Error(),
		})
	}

	a.orderState.Delete(orderId)

	return c.JSON(http.StatusOK, echo.Map{})
}

type closePositionRequest struct {
	exante.Api
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
			"error": fmt.Errorf("no active order found"),
		})
	}

	_, err := a.exApi.PlaceOrderV3(&exante.OrderSentTypeV3{
		AccountID:  orderDB.Order.AccountId,
		Instrument: orderDB.Order.Symbol,
		Side:       utils.GetReverseOrderSide(orderDB.Order.Side),
		Quantity:   orderDB.Order.Quantity,
		Duration:   "day",
		OrderType:  "market",
		SymbolID:   orderDB.Order.Symbol,
	})
	if err != nil {
		return c.JSON(http.StatusInternalServerError, echo.Map{
			"error": err.Error(),
		})
	}

	a.orderState.Delete(orderId)

	return c.JSON(http.StatusOK, echo.Map{})
}

func (a api) getOrders(c echo.Context) error {
	orders, err := a.exApi.GetActiveOrdersV3()
	if err != nil {
		return c.JSON(http.StatusBadRequest, echo.Map{
			"error": err.Error(),
		})
	}

	return c.JSON(http.StatusOK, orders)
}

type takeProfitRequest struct {
	LimitPrice float64 `json:"limitPrice"`
}
