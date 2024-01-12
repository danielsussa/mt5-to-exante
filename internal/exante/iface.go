package exante

type Iface interface {
	CancelOrder(orderID string) error
	GetOrder(orderID string) (*OrderV3, error)
	PlaceOrderV3(req *OrderSentTypeV3) ([]OrderV3, error)
	ReplaceOrder(orderID string, req ReplaceOrderPayload) (*OrderV3, error)
}
