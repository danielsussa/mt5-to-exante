package exante

type ApiMock struct {
	CancelOrderFunc  func(orderID string) error
	GetOrderFunc     func(orderID string) (*OrderV3, error)
	PlaceOrderV3Func func(req *OrderSentTypeV3) ([]OrderV3, error)
	ReplaceOrderFunc func(orderID string, req ReplaceOrderPayload) (*OrderV3, error)
	totalCalls       int
}

func (a *ApiMock) ReplaceOrder(orderID string, req ReplaceOrderPayload) (*OrderV3, error) {
	a.totalCalls++
	return a.ReplaceOrderFunc(orderID, req)
}

func (a *ApiMock) CancelOrder(orderID string) error {
	a.totalCalls++
	return a.CancelOrderFunc(orderID)
}

func (a *ApiMock) GetOrder(orderID string) (*OrderV3, error) {
	a.totalCalls++
	return a.GetOrderFunc(orderID)
}

func (a *ApiMock) PlaceOrderV3(req *OrderSentTypeV3) ([]OrderV3, error) {
	a.totalCalls++
	return a.PlaceOrderV3Func(req)
}

func (a *ApiMock) TotalCalls() int {
	return a.totalCalls
}
