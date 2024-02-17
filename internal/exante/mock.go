package exante

type ApiMock struct {
	CancelOrderFunc        func(orderID string) error
	GetOrderFunc           func(orderID string) (*OrderV3, error)
	PlaceOrderV3Func       func(req *OrderSentTypeV3) ([]OrderV3, error)
	ReplaceOrderFunc       func(orderID string, req ReplaceOrderPayload) (*OrderV3, error)
	GetOrdersByLimitV3Func func(limit int) ([]OrderV3, error)
	GetActiveOrdersV3Func  func() (OrdersV3, error)
	TotalCalls             int
	TotalPlaceOrderV3      int
	orders                 []OrderV3
}

func (a *ApiMock) GetActiveOrdersV3() (OrdersV3, error) {
	return a.GetActiveOrdersV3Func()
}

func (a *ApiMock) GetOrdersByLimitV3(limit int) (OrdersV3, error) {
	return a.GetOrdersByLimitV3Func(limit)
}

func (a *ApiMock) ReplaceOrder(orderID string, req ReplaceOrderPayload) (*OrderV3, error) {
	a.TotalCalls++
	return a.ReplaceOrderFunc(orderID, req)
}

func (a *ApiMock) CancelOrder(orderID string) error {
	a.TotalCalls++
	return a.CancelOrderFunc(orderID)
}

func (a *ApiMock) GetOrder(orderID string) (*OrderV3, error) {
	a.TotalCalls++
	return a.GetOrderFunc(orderID)
}

func (a *ApiMock) PlaceOrderV3(req *OrderSentTypeV3) ([]OrderV3, error) {
	a.TotalCalls++
	a.TotalPlaceOrderV3++
	return a.PlaceOrderV3Func(req)
}
