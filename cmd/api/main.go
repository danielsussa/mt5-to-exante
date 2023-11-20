package main

import (
	"fmt"
	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
	"github.com/slack-go/slack"
	httplib "mt-to-exante/internal/exante"
	"net/http"
	"os"
)

// MQL link orders / buy and sell
// take profit not finding limitPrice

func main() {
	h := api{
		exApi: httplib.NewApi(
			os.Getenv("BASE_URL"),
			os.Getenv("APPLICATION_ID"),
			os.Getenv("CLIENT_ID"),
			os.Getenv("SHARED_KEY"),
		),
		slackApi: slack.New("xoxb-5937889843297-5927731285988-rUJWhdHM5a3kf1ythfjKCflb", slack.OptionDebug(true)),
	}

	e := echo.New()

	e.GET("/health", func(c echo.Context) error {
		fmt.Println("health")
		return c.String(http.StatusOK, "ok")
	})

	//e.GET("/exchanges", h.getExchanges)
	e.GET("/accounts", h.getAccounts)
	e.POST("/v3/orders", h.getOrders)
	e.POST("/v3/orders/:orderID/place", h.placeOrder)
	e.POST("/v3/orders/:orderID/takeProfit", h.takeProfit)
	e.POST("/v3/orders/:orderID/cancel", h.cancelOrder)
	e.POST("/v3/positions/:orderID/close", h.closePosition)
	e.POST("/v3/positions/:orderID/tpls", h.changeTPLS)
	//e.POST("/v3/orders/:orderID/replace", h.replaceOrder)
	e.Logger.Fatal(e.Start(":1323"))
}

type api struct {
	exApi    httplib.Api
	slackApi *slack.Client
}

//func (a api) getExchanges(c echo.Context) error {
//	accounts, err := a.exApi.GetSymbolV3("FOREX")
//	if err != nil {
//		return c.JSON(http.StatusBadRequest, echo.Map{
//			"error": err.Error(),
//		})
//	}
//
//	return c.JSON(http.StatusOK, accounts)
//}

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
	var err error
	defer func() {
		if err != nil {
			a.sendErrorToSlack("placeOrder", err)
		}
	}()

	req := new(placeOrderRequest)
	if err = c.Bind(req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}

	symbol, instrument, has := convertToSymbolInstrument(req.SymbolID)
	if !has {
		err = fmt.Errorf("symbol not found")
		return c.JSON(http.StatusBadRequest, echo.Map{
			"error": err.Error(),
		})
	}
	orderID := c.Param("orderID")

	order, hasOrder, err := a.exApi.GetActiveOrderByID(orderID)
	if err != nil {
		return c.JSON(http.StatusBadRequest, echo.Map{
			"error": err.Error(),
		})
	}

	if hasOrder {
		if order.OrderState.Status == "filled" {
			// has active order and is filled
			_, err = a.exApi.ReplaceOrder(orderID, httplib.ReplaceOrderPayload{
				Action: "replace",
				Parameters: httplib.ReplaceOrderParameters{
					StopPrice: fmt.Sprintf("%.2f", req.StopLoss),
				},
			})

			if err != nil {
				return c.JSON(http.StatusBadRequest, echo.Map{
					"step":  "replace order",
					"error": err.Error(),
				})
			}
			return c.JSON(http.StatusOK, echo.Map{})
		} else {
			// has active order and is opened
			err = a.exApi.CancelOrder(order.OrderID)
			if err != nil {
				return c.JSON(http.StatusBadRequest, echo.Map{
					"error": err.Error(),
				})
			}
		}
	}

	// no active order
	orders, err := a.exApi.PlaceOrderV3(&httplib.OrderSentTypeV3{
		SymbolID:   symbol,
		Duration:   req.Duration,
		OrderType:  req.OrderType,
		Quantity:   fmt.Sprintf("%.5f", req.Quantity),
		Side:       req.Side,
		LimitPrice: fmt.Sprintf("%.5f", req.LimitPrice),
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

	a.successFullPlaceOrder(c.Param("orderID"))

	return c.JSON(http.StatusOK, orders)

}

type changeOrderRequest struct {
	LimitPrice float64 `json:"limitPrice"`
	TakeProfit float64 `json:"takeProfit"`
	StopLoss   float64 `json:"stopLoss"`
}

func (a api) changeOrder(c echo.Context) error {
	var err error
	defer func() {
		if err != nil {
			a.sendErrorToSlack("changeOrder", err)
		}
	}()

	req := new(changeOrderRequest)
	if err = c.Bind(req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}

	orderID := c.Param("orderID")

	orders, err := a.exApi.GetActiveOrdersByID(orderID)
	if err != nil {
		return c.JSON(http.StatusBadRequest, echo.Map{
			"error": err.Error(),
		})
	}

	parentOrder, has := getParentOrder(orders)
	if !has {
		return c.JSON(http.StatusBadRequest, echo.Map{
			"error": "no parent order registered",
		})
	}
	fmt.Println(parentOrder)

	if req.StopLoss > 0 {
		stopLossOrder, has := getTakeProfitOrder(orders)
		if !has {

		}

		a.exApi.ReplaceOrder(orderID, httplib.ReplaceOrderPayload{
			Action: "replace",
			Parameters: httplib.ReplaceOrderParameters{
				Quantity:   stopLossOrder.OrderParameters.Quantity,
				LimitPrice: fmt.Sprintf("%.5f", req.StopLoss),
			},
		})
	}

	a.successFullPlaceOrder(c.Param("orderID"))

	return c.JSON(http.StatusOK, orders)

}

type changeTPLSRequest struct {
	TakeProfit float64 `json:"takeProfit"`
	StopLoss   float64 `json:"stopLoss"`
}

func (a api) changeTPLS(c echo.Context) error {
	var err error
	defer func() {
		if err != nil {
			a.sendErrorToSlack("changeTPLS", err)
		}
	}()

	req := new(changeTPLSRequest)
	if err = c.Bind(req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}

	orderID := c.Param("orderID")

	orders, err := a.exApi.GetOrdersByID(orderID)
	if err != nil {
		return c.JSON(http.StatusBadRequest, echo.Map{
			"error": err.Error(),
		})
	}

	parentOrder, hasParent := getParentOrder(orders)
	if !hasParent {
		return c.JSON(http.StatusBadRequest, echo.Map{
			"error": "no parent order registered",
		})
	}

	ocoGroup := ""

	stopLossOrder, hasSLOrder := getStopLossOrder(orders)
	if hasSLOrder {
		ocoGroup = stopLossOrder.OrderParameters.OcoGroup
	}

	takePriceOrder, hasTPOrder := getTakeProfitOrder(orders)
	if hasTPOrder {
		ocoGroup = takePriceOrder.OrderParameters.OcoGroup
	}

	if ocoGroup == "" {
		ocoGroup = uuid.New().String()
	}

	if req.StopLoss > 0 {
		if !hasSLOrder {
			_, err = a.exApi.PlaceOrderV3(&httplib.OrderSentTypeV3{
				AccountID:      parentOrder.AccountID,
				Instrument:     parentOrder.OrderParameters.SymbolId,
				LimitPrice:     covert5Decimals(req.StopLoss),
				Side:           getReverseOrderSide(*parentOrder),
				Quantity:       parentOrder.OrderParameters.Quantity,
				Duration:       parentOrder.OrderParameters.Duration,
				IfDoneParentID: parentOrder.OrderID,
				OcoGroup:       ocoGroup,
				ClientTag:      orderID,
				OrderType:      "limit",
				SymbolID:       parentOrder.OrderParameters.SymbolId,
			})
		} else {
			_, err = a.exApi.ReplaceOrder(orderID, httplib.ReplaceOrderPayload{
				Action: "replace",
				Parameters: httplib.ReplaceOrderParameters{
					Quantity:   stopLossOrder.OrderParameters.Quantity,
					LimitPrice: fmt.Sprintf("%.5f", req.StopLoss),
				},
			})
		}
		if err != nil {
			return c.JSON(http.StatusInternalServerError, echo.Map{
				"error": err.Error(),
			})
		}
	}

	if req.TakeProfit > 0 {
		if !hasTPOrder {
			_, err = a.exApi.PlaceOrderV3(&httplib.OrderSentTypeV3{
				AccountID:      parentOrder.AccountID,
				Instrument:     parentOrder.OrderParameters.SymbolId,
				LimitPrice:     covert5Decimals(req.TakeProfit),
				Side:           getReverseOrderSide(*parentOrder),
				Quantity:       parentOrder.OrderParameters.Quantity,
				Duration:       parentOrder.OrderParameters.Duration,
				IfDoneParentID: parentOrder.OrderID,
				OcoGroup:       ocoGroup,
				ClientTag:      orderID,
				OrderType:      "limit",
				SymbolID:       parentOrder.OrderParameters.SymbolId,
			})
		} else {
			_, err = a.exApi.ReplaceOrder(orderID, httplib.ReplaceOrderPayload{
				Action: "replace",
				Parameters: httplib.ReplaceOrderParameters{
					Quantity:   stopLossOrder.OrderParameters.Quantity,
					LimitPrice: fmt.Sprintf("%.5f", req.TakeProfit),
				},
			})
		}
		if err != nil {
			return c.JSON(http.StatusInternalServerError, echo.Map{
				"error": err.Error(),
			})
		}
	}

	a.successFullPlaceOrder(c.Param("orderID"))

	return c.JSON(http.StatusOK, orders)

}

func floatToString(k float64) *string {
	if k > 0 {
		valS := fmt.Sprintf("%.5f", k)
		return &valS
	}
	return nil
}

type cancelOrderRequest struct {
	httplib.Api
}

func (a api) cancelOrder(c echo.Context) error {
	var err error
	defer func() {
		if err != nil {
			a.sendErrorToSlack("cancelOrder", err)
		}
	}()
	req := new(cancelOrderRequest)
	if err = c.Bind(req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}

	order, hasOrder, err := a.exApi.GetActiveOrderByID(c.Param("orderID"))
	if err != nil {
		return c.JSON(http.StatusBadRequest, echo.Map{
			"error": err.Error(),
		})
	}

	if !hasOrder {
		err = fmt.Errorf("no active order found")
		return c.JSON(http.StatusBadRequest, echo.Map{
			"error": err.Error(),
		})
	}

	err = a.exApi.CancelOrder(order.OrderID)
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
	var err error
	defer func() {
		if err != nil {
			a.sendErrorToSlack("closePosition", err)
		}
	}()
	req := new(closePositionRequest)
	if err = c.Bind(req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}

	order, hasOrder, err := a.exApi.GetFilledOrderByID(c.Param("orderID"))
	if err != nil {
		return c.JSON(http.StatusBadRequest, echo.Map{
			"error": err.Error(),
		})
	}

	if !hasOrder {
		err = fmt.Errorf("no active order found")
		return c.JSON(http.StatusBadRequest, echo.Map{
			"error": err.Error(),
		})
	}

	_, err = a.exApi.PlaceOrderV3(&httplib.OrderSentTypeV3{
		AccountID:  order.AccountID,
		Instrument: order.OrderParameters.SymbolId,
		Side:       getReverseOrderSide(order),
		Quantity:   order.OrderParameters.Quantity,
		Duration:   "day",
		OrderType:  "market",
		SymbolID:   order.OrderParameters.SymbolId,
	})
	if err != nil {
		return c.JSON(http.StatusInternalServerError, echo.Map{
			"error": err.Error(),
		})
	}

	return c.JSON(http.StatusOK, echo.Map{})
}

type replaceOrderRequest struct {
	httplib.Api
	Quantity      string `json:"quantity"`
	LimitPrice    string `json:"limitPrice"`
	StopPrice     string `json:"stopPrice"`
	PriceDistance string `json:"priceDistance"`
}

//func (a api) replaceOrder(c echo.Context) error {
//	var err error
//	defer func() {
//		if err != nil {
//			a.sendErrorToSlack("replaceOrder", err)
//		}
//	}()
//
//	req := new(replaceOrderRequest)
//	if err = c.Bind(req); err != nil {
//		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
//	}
//
//	orders, err := a.exApi.GetActiveOrdersV3(req.Api)
//	if err != nil {
//		return c.JSON(http.StatusBadRequest, echo.Map{
//			"error": err.Error(),
//		})
//	}
//
//	mtOrderId := c.Param("orderID")
//	var currOrder *httplib.OrderV3
//	for _, order := range *orders {
//		if order.ClientTag == mtOrderId {
//			currOrder = &order
//			break
//		}
//	}
//
//	if currOrder == nil {
//		return c.JSON(http.StatusBadRequest, echo.Map{
//			"error": "no active order found",
//		})
//	}
//
//	_, err = a.exApi.ReplaceOrder(currOrder.OrderID, httplib.ReplaceOrderPayload{
//		Api:    req.Api,
//		Action: "replace",
//		Parameters: httplib.ReplaceOrderParameters{
//			Quantity:      req.Quantity,
//			LimitPrice:    req.LimitPrice,
//			StopPrice:     req.StopPrice,
//			PriceDistance: req.PriceDistance,
//		},
//	})
//
//	if err != nil {
//		return c.JSON(http.StatusBadRequest, echo.Map{
//			"error": err.Error(),
//		})
//	}
//	return c.JSON(http.StatusOK, echo.Map{})
//}

func (a api) getOrders(c echo.Context) error {
	var err error
	defer func() {
		if err != nil {
			a.sendErrorToSlack("getOrders", err)
		}
	}()

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

func (a api) takeProfit(c echo.Context) error {
	var err error
	defer func() {
		if err != nil {
			a.sendErrorToSlack("takeProfit", err)
		}
	}()

	req := new(takeProfitRequest)
	if err = c.Bind(req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}

	orderID := c.Param("orderID")

	orders, err := a.exApi.GetActiveOrdersByID(orderID)
	if err != nil {
		return c.JSON(http.StatusBadRequest, echo.Map{
			"error": err.Error(),
		})
	}

	takeProfitOrder, hasOrder := getTakeProfitOrder(orders)
	if !hasOrder {
		err = fmt.Errorf("no active order found")
		return c.JSON(http.StatusBadRequest, echo.Map{
			"error": err.Error(),
		})
	}

	_, err = a.exApi.ReplaceOrder(takeProfitOrder.OrderID, httplib.ReplaceOrderPayload{
		Action: "replace",
		Parameters: httplib.ReplaceOrderParameters{
			Quantity:   takeProfitOrder.OrderParameters.Quantity,
			LimitPrice: covert5Decimals(req.LimitPrice),
		},
	})
	if err != nil {
		return c.JSON(http.StatusInternalServerError, echo.Map{
			"error": err.Error(),
		})
	}

	return c.JSON(http.StatusOK, echo.Map{})
}

func getReverseOrderSide(order httplib.OrderV3) string {
	if order.OrderParameters.Side == "buy" {
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

func (a api) successFullPlaceOrder(orderId string) {
	msg := fmt.Sprintf("successfull placing order id=%s", orderId)

	_, _, _ = a.slackApi.PostMessage(
		"C05T772CDKM",
		slack.MsgOptionText(msg, false),
	)
}

func (a api) sendErrorToSlack(scope string, err error) {
	fmt.Println("error: ", err.Error())
	msg := fmt.Sprintf(":red_circle: <!channel> error alert on %s service: %s", scope, err.Error())

	_, _, _ = a.slackApi.PostMessage(
		"C05TKSB2DR7",
		slack.MsgOptionText(msg, false),
	)
}

func covert5Decimals(k float64) string {
	return fmt.Sprintf("%.5f", k)
}
