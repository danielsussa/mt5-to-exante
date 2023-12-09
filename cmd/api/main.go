package main

import (
	"encoding/json"
	"fmt"
	"github.com/joho/godotenv"
	"github.com/labstack/echo/v4"
	"github.com/peterbourgon/diskv/v3"
	httplib "mt-to-exante/internal/exante"
	"net/http"
	"os"
)

func main() {
	err := godotenv.Load(fmt.Sprintf("%s.env", os.Args[1]))
	if err != nil {
		panic(err)
	}

	h := api{
		exApi: httplib.NewApi(
			os.Getenv("BASE_URL"),
			os.Getenv("APPLICATION_ID"),
			os.Getenv("CLIENT_ID"),
			os.Getenv("SHARED_KEY"),
		),
		orderState: startOrderState(),
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
	exApi      *httplib.Api
	orderState orderState
}

type orderState struct {
	d *diskv.Diskv
}

func (os orderState) upsert(ticketID string, order orderGroup) {
	b, _ := json.Marshal(order)
	_ = os.d.Write(ticketID, b)
}

func (os orderState) get(ticketID string) (orderGroup, bool) {
	if !os.d.Has(ticketID) {
		return orderGroup{}, false
	}
	b, err := os.d.Read(ticketID)
	if err != nil {
		panic(err)
	}
	var order orderGroup
	_ = json.Unmarshal(b, &order)

	return order, true
}

func startOrderState() orderState {
	d := diskv.New(diskv.Options{
		BasePath:     "orders",
		Transform:    func(s string) []string { return []string{} },
		CacheSizeMax: 1024 * 1024,
	})

	return orderState{d: d}
}

type orderGroup struct {
	Order      orderDB
	StopLoss   *orderDB
	TakeProfit *orderDB
}

type orderDB struct {
	ID         string
	Quantity   string
	OcoGroup   string
	Side       string
	Duration   string
	AccountId  string
	Symbol     string
	Instrument string
}

func (a api) getJwt(c echo.Context) error {
	return c.JSON(http.StatusOK, a.exApi.GetJwt())
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

	orderDB, hasOrder := a.orderState.get(orderID)
	if hasOrder {
		return c.JSON(http.StatusBadRequest, echo.Map{
			"error": "order already exist",
		})
	}

	// no active order
	orders, err := a.exApi.PlaceOrderV3(&httplib.OrderSentTypeV3{
		SymbolID:   symbol,
		Duration:   req.Duration,
		OrderType:  req.OrderType,
		Quantity:   convert5Decimals(req.Quantity),
		Side:       req.Side,
		LimitPrice: convert5Decimals(req.LimitPrice),
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

	parentOrder, has := getParentOrder(orders)
	if has {
		orderDB.Order = *convertExOrderToDB(*parentOrder)
	}

	slOrder, has := getStopLossOrder(orders)
	if has {
		orderDB.StopLoss = convertExOrderToDB(*slOrder)
	}

	tpOrder, has := getTakeProfitOrder(orders)
	if has {
		orderDB.TakeProfit = convertExOrderToDB(*tpOrder)
	}

	a.orderState.upsert(orderID, orderDB)

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

	orderDB, has := a.orderState.get(orderID)
	if !has {
		return c.JSON(http.StatusBadRequest, echo.Map{
			"error": "no parent order registered",
		})
	}

	a.exApi.ReplaceOrder(orderID, httplib.ReplaceOrderPayload{
		Action: "replace",
		Parameters: httplib.ReplaceOrderParameters{
			Quantity:   orderDB.Order.Quantity,
			LimitPrice: convert5Decimals(req.LimitPrice),
		},
	})

	if req.StopLoss > 0 {
		if orderDB.StopLoss == nil {
			orders, err := a.exApi.PlaceOrderV3(&httplib.OrderSentTypeV3{
				SymbolID:   orderDB.Order.Symbol,
				Duration:   orderDB.Order.Duration,
				OrderType:  "stop",
				Quantity:   orderDB.Order.Quantity,
				Side:       getReverseOrderSide(orderDB.Order.Side),
				LimitPrice: convert5Decimals(req.StopLoss),
				Instrument: orderDB.Order.Instrument,
				OcoGroup:   orderDB.Order.OcoGroup,
				AccountID:  orderDB.Order.AccountId,
			})
			if err != nil {
				return c.JSON(http.StatusBadRequest, echo.Map{
					"error": err.Error(),
				})
			}
			orderDB.StopLoss = convertExOrderToDB(orders[0])
		} else {
			a.exApi.ReplaceOrder(orderID, httplib.ReplaceOrderPayload{
				Action: "replace",
				Parameters: httplib.ReplaceOrderParameters{
					Quantity:   orderDB.Order.Quantity,
					LimitPrice: convert5Decimals(req.StopLoss),
				},
			})
		}
	}

	if req.TakeProfit > 0 {
		if orderDB.TakeProfit == nil {
			orders, err := a.exApi.PlaceOrderV3(&httplib.OrderSentTypeV3{
				SymbolID:   orderDB.Order.Symbol,
				Duration:   orderDB.Order.Duration,
				OrderType:  "limit",
				Quantity:   orderDB.Order.Quantity,
				Side:       getReverseOrderSide(orderDB.Order.Side),
				LimitPrice: convert5Decimals(req.TakeProfit),
				Instrument: orderDB.Order.Instrument,
				OcoGroup:   orderDB.Order.OcoGroup,
				AccountID:  orderDB.Order.AccountId,
			})
			if err != nil {
				return c.JSON(http.StatusBadRequest, echo.Map{
					"error": err.Error(),
				})
			}
			orderDB.StopLoss = convertExOrderToDB(orders[0])
		} else {
			a.exApi.ReplaceOrder(orderID, httplib.ReplaceOrderPayload{
				Action: "replace",
				Parameters: httplib.ReplaceOrderParameters{
					Quantity:   orderDB.Order.Quantity,
					LimitPrice: convert5Decimals(req.TakeProfit),
				},
			})
		}
	}

	return c.JSON(http.StatusOK, echo.Map{})

}

func floatToString(k float64) *string {
	if k > 0 {
		valS := convert5Decimals(k)
		return &valS
	}
	return nil
}

type cancelOrderRequest struct {
	httplib.Api
}

func (a api) cancelOrder(c echo.Context) error {
	req := new(cancelOrderRequest)
	if err := c.Bind(req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}

	orderDB, hasOrder := a.orderState.get(c.Param("orderID"))
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

	return c.JSON(http.StatusOK, echo.Map{})
}

type closePositionRequest struct {
	httplib.Api
}

func (a api) closePosition(c echo.Context) error {
	req := new(closePositionRequest)
	if err := c.Bind(req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}

	orderDB, hasOrder := a.orderState.get(c.Param("orderID"))
	if !hasOrder {
		return c.JSON(http.StatusBadRequest, echo.Map{
			"error": fmt.Errorf("no active order found"),
		})
	}

	_, err := a.exApi.PlaceOrderV3(&httplib.OrderSentTypeV3{
		AccountID:  orderDB.Order.AccountId,
		Instrument: orderDB.Order.Symbol,
		Side:       getReverseOrderSide(orderDB.Order.Side),
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

func getReverseOrderSide(side string) string {
	if side == "buy" {
		return "sell"
	}
	return "buy"
}
func getParentOrder(orders []httplib.OrderV3) (*httplib.OrderV3, bool) {
	for _, order := range orders {
		if len(order.OrderParameters.OcoGroup) == 0 {
			return &order, true
		}
	}

	return nil, false
}
func getTakeProfitOrder(orders []httplib.OrderV3) (*httplib.OrderV3, bool) {
	for _, order := range orders {
		if len(order.OrderParameters.OcoGroup) > 0 && order.OrderParameters.OrderType == "limit" {
			return &order, true
		}
	}

	return nil, false
}

func getStopLossOrder(orders []httplib.OrderV3) (*httplib.OrderV3, bool) {
	for _, order := range orders {
		if len(order.OrderParameters.OcoGroup) > 0 && order.OrderParameters.OrderType != "limit" {
			return &order, true
		}
	}

	return nil, false
}

func convert5Decimals(k float64) string {
	return fmt.Sprintf("%.5f", k)
}

func convertExOrderToDB(v3 httplib.OrderV3) *orderDB {
	return &orderDB{
		ID:         v3.OrderID,
		Quantity:   v3.OrderParameters.Quantity,
		OcoGroup:   v3.OrderParameters.OcoGroup,
		Side:       v3.OrderParameters.Side,
		Duration:   v3.OrderParameters.Duration,
		AccountId:  v3.AccountID,
		Symbol:     v3.OrderParameters.SymbolId,
		Instrument: v3.OrderParameters.Instrument,
	}
}
