package exante

import (
	"fmt"
	"github.com/google/uuid"
	"slices"
)

func NewMock(ordersList []OrderV3) *ApiMock {
	return &ApiMock{
		CancelOrderFunc: func(orderID string) error {
			for idx, val := range ordersList {
				if val.OrderID == orderID || val.OrderParameters.IfDoneParentID == orderID {
					ordersList[idx].OrderState.Status = CancelledStatus
				}
			}

			return nil
		},
		GetOrderFunc: func(orderID string) (*OrderV3, error) {
			if idx := slices.IndexFunc(ordersList, func(v3 OrderV3) bool { return v3.OrderID == orderID }); idx > -1 {
				return &ordersList[idx], nil
			}
			return nil, nil
		},
		PlaceOrderV3Func: func(req *OrderSentTypeV3) ([]OrderV3, error) {
			orders := make([]OrderV3, 0)

			doneParentOrder := uuid.NewString()
			if len(req.IfDoneParentID) > 0 {
				doneParentOrder = req.IfDoneParentID
			}
			if len(req.LimitPrice) > 0 {
				orders = append(orders, OrderV3{
					OrderState: OrderState{
						Status: convertTypeToStatus(req.OrderType),
					},
					OrderParameters: OrderParameters{
						Quantity:   req.Quantity,
						Side:       req.Side,
						Instrument: req.Instrument,
						OrderType:  req.OrderType,
						LimitPrice: req.LimitPrice,
						OcoGroup:   req.OcoGroup,
					},
					OrderID:   doneParentOrder,
					ClientTag: req.ClientTag,
				})
			}

			ocoGroup := uuid.NewString()
			if len(req.OcoGroup) > 0 {
				ocoGroup = req.OcoGroup
			}

			if req.TakeProfit != nil {
				orders = append(orders, OrderV3{
					OrderState: OrderState{
						Status: PendingStatus,
					},
					OrderParameters: OrderParameters{
						OcoGroup:       ocoGroup,
						LimitPrice:     *req.TakeProfit,
						OrderType:      "limit",
						IfDoneParentID: doneParentOrder,
					},
					OrderID:   uuid.NewString(),
					ClientTag: req.ClientTag,
				})
			}
			if req.StopLoss != nil {
				orders = append(orders, OrderV3{
					OrderState: OrderState{
						Status: PendingStatus,
					},
					OrderParameters: OrderParameters{
						OcoGroup:       ocoGroup,
						OrderType:      "stop",
						LimitPrice:     *req.StopLoss,
						StopPrice:      *req.StopLoss,
						IfDoneParentID: doneParentOrder,
					},
					OrderID:   uuid.NewString(),
					ClientTag: req.ClientTag,
				})
			}

			ordersList = append(ordersList, orders...)
			return orders, nil
		},
		ReplaceOrderFunc: func(orderID string, req ReplaceOrderPayload) (*OrderV3, error) {
			if idx := slices.IndexFunc(ordersList, func(v3 OrderV3) bool { return v3.OrderID == orderID }); idx > -1 {
				if len(req.Parameters.StopPrice) > 0 {
					ordersList[idx].OrderParameters.StopPrice = req.Parameters.StopPrice
				}
				if len(req.Parameters.LimitPrice) > 0 {
					ordersList[idx].OrderParameters.LimitPrice = req.Parameters.LimitPrice
				}
				return &ordersList[idx], nil
			}
			return nil, fmt.Errorf("no order to replace")
		},
		GetOrdersByLimitV3Func: func(limit int) ([]OrderV3, error) {
			return ordersList, nil
		},
		GetActiveOrdersV3Func: func() (OrdersV3, error) {
			newList := make([]OrderV3, 0)
			for _, order := range ordersList {
				if order.OrderState.Status == PendingStatus || order.OrderState.Status == WorkingStatus {
					newList = append(newList, order)
				}
			}
			return newList, nil
		},
	}
}

func convertTypeToStatus(ot string) Status {
	if ot == "market" {
		return FilledStatus
	}
	return WorkingStatus
}
