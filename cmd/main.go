package main

import (
	"fmt"
	"github.com/labstack/echo/v4"
	httplib "mt-to-exante/internal/exante"
	"net/http"
)

func main() {
	exanteApi := httplib.NewAPI("https://api-demo.exante.eu",
		"2.0",
		"218cd7b4-2da4-4f4c-9f3c-9f47b0cdfc41",
		"31c78477-c140-4014-be90-e6b24a52f199",
		"Xw4B87A8NF0F02H9LZhGtrl5zL0Q6g5W",
		120, "", "",
	)

	h := api{
		exApi: exanteApi,
	}

	e := echo.New()
	e.GET("/health", func(c echo.Context) error {
		fmt.Println("health")
		return c.String(http.StatusOK, "ok")
	})
	e.GET("/accounts", h.getAccounts)
	e.POST("/v3/orders/:orderID/place", h.placeOrder)
	e.POST("/v3/orders/:orderID/cancel", h.cancelOrder)
	e.POST("/v3/orders/:orderID/replace", h.replace)
	e.Logger.Fatal(e.Start(":1323"))
}

type api struct {
	exApi httplib.HTTPApi
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
	//
	SymbolID   string `json:"symbolID"`
	Duration   string `json:"duration"`
	OrderType  string `json:"orderType"`
	Quantity   string `json:"quantity"`
	Side       string `json:"side"`
	Instrument string `json:"instrument"`
	AccountId  string `json:"accountId"`
	LimitPrice string `json:"limitPrice"`
}

func (a api) placeOrder(c echo.Context) error {
	req := new(placeOrderRequest)
	if err := c.Bind(req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}

	orders, err := a.exApi.PlaceOrderV3(&httplib.OrderSentTypeV3{
		SymbolID:   req.SymbolID,
		Duration:   req.Duration,
		OrderType:  req.OrderType,
		Quantity:   req.Quantity,
		Side:       req.Side,
		LimitPrice: req.LimitPrice,
		Instrument: req.Instrument,

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

	return c.JSON(http.StatusOK, orders)
}

func (a api) cancelOrder(c echo.Context) error {
	orders, err := a.exApi.GetActiveOrdersV3()
	if err != nil {
		return c.JSON(http.StatusBadRequest, echo.Map{
			"error": err.Error(),
		})
	}

	mtOrderId := c.Param("orderID")
	var currOrder *httplib.OrderV3
	for _, order := range *orders {
		if order.ClientTag == mtOrderId {
			currOrder = &order
			break
		}
	}

	if currOrder == nil {
		return c.JSON(http.StatusBadRequest, echo.Map{
			"error": "no active order found",
		})
	}

	_, err = a.exApi.ReplaceOrder(currOrder.OrderID, httplib.ReplaceOrderPayload{
		Action: "cancel",
	})

	if err != nil {
		return c.JSON(http.StatusBadRequest, echo.Map{
			"error": err.Error(),
		})
	}
	return c.JSON(http.StatusOK, echo.Map{})
}

type replaceOrderRequest struct {
	Quantity      string `json:"quantity"`
	LimitPrice    string `json:"limitPrice"`
	StopPrice     string `json:"stopPrice"`
	PriceDistance string `json:"priceDistance"`
}

func (a api) replace(c echo.Context) error {
	req := new(replaceOrderRequest)
	if err := c.Bind(req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}

	orders, err := a.exApi.GetActiveOrdersV3()
	if err != nil {
		return c.JSON(http.StatusBadRequest, echo.Map{
			"error": err.Error(),
		})
	}

	mtOrderId := c.Param("orderID")
	var currOrder *httplib.OrderV3
	for _, order := range *orders {
		if order.ClientTag == mtOrderId {
			currOrder = &order
			break
		}
	}

	if currOrder == nil {
		return c.JSON(http.StatusBadRequest, echo.Map{
			"error": "no active order found",
		})
	}

	_, err = a.exApi.ReplaceOrder(currOrder.OrderID, httplib.ReplaceOrderPayload{
		Action: "replace",
		Parameters: httplib.ReplaceOrderParameters{
			Quantity:      req.Quantity,
			LimitPrice:    req.LimitPrice,
			StopPrice:     req.StopPrice,
			PriceDistance: req.PriceDistance,
		},
	})

	if err != nil {
		return c.JSON(http.StatusBadRequest, echo.Map{
			"error": err.Error(),
		})
	}
	return c.JSON(http.StatusOK, echo.Map{})
}
