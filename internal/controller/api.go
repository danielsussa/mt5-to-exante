package controller

import (
	"fmt"
	"github.com/danielsussa/mt5-to-exante/internal/exante"
	"github.com/danielsussa/mt5-to-exante/internal/exchanges"
	"github.com/danielsussa/mt5-to-exante/internal/orderdb"
	"github.com/danielsussa/mt5-to-exante/internal/utils"
	"strings"
	"time"
)

type Api struct {
	exanteApi exante.Iface
	db        orderdb.Iface
	exchange  exchanges.Api
}

func New(exanteApi exante.Iface, db orderdb.Iface, exchange exchanges.Api) *Api {
	return &Api{
		exanteApi: exanteApi,
		db:        db,
		exchange:  exchange,
	}
}

type (
	SyncRequest struct {
		Orders    []Mt5Order
		History   []Mt5OrderHistory
		Positions []Mt5Order
	}

	Mt5OrderHistory struct {
		Ticket string
		State  OrderState
	}

	Mt5Order struct {
		Symbol     string
		Ticket     string
		Volume     float64
		Type       OrderType
		TakeProfit float64
		StopLoss   float64
		Price      float64
		State      OrderState
		UpdatedAt  time.Time
	}

	OrderState string
	OrderType  string
)

const (
	OrderStatePlaced    OrderState = "ORDER_STATE_PLACED"
	OrderStateFilled    OrderState = "ORDER_STATE_FILLED"
	OrderStateCancelled OrderState = "ORDER_STATE_CANCELED"
	OrderStateStarted   OrderState = "ORDER_STATE_STARTED"

	OrderTypeBuy       OrderType = "ORDER_TYPE_BUY"
	OrderTypeSell      OrderType = "ORDER_TYPE_SELL"
	OrderTypeBuyLimit  OrderType = "ORDER_TYPE_BUY_LIMIT"
	OrderTypeSellLimit OrderType = "ORDER_TYPE_SELL_LIMIT"
	OrderTypeBuyStop   OrderType = "ORDER_TYPE_BUY_STOP"
	OrderTypeSellStop  OrderType = "ORDER_TYPE_SELL_STOP"
)

func (a *Api) Sync(tNow time.Time, accountID string, req SyncRequest) error {
	for _, mt5OrderHistory := range req.History {
		if dbOrderGroup, hasDBOrder := a.db.Get(mt5OrderHistory.Ticket); hasDBOrder {
			if mt5OrderHistory.State == OrderStateCancelled {
				err := a.cancelOrder(dbOrderGroup)
				if err != nil {
					return err
				}
			}
		}
	}

	for _, mt5Order := range req.Orders {
		if mt5Order.State != OrderStatePlaced {
			continue
		}
		dbOrderGroup, hasDBOrder := a.db.Get(mt5Order.Ticket)

		// diff main order
		if hasDBOrder && hasChangedPrice(&dbOrderGroup.Order, mt5Order) {
			orderDB, err := a.replaceOrder(dbOrderGroup.Order, mt5Order.Price)
			if err != nil {
				return err
			}
			dbOrderGroup.Order = *orderDB
			a.db.Upsert(dbOrderGroup.Ticket, dbOrderGroup)
		}
		// diff stop loss order
		if hasDBOrder && hasChangedStopLoss(dbOrderGroup.StopLoss, mt5Order) {
			orderDB, err := a.replaceStopOrder(*dbOrderGroup.StopLoss, mt5Order.StopLoss)
			if err != nil {
				return err
			}
			dbOrderGroup.StopLoss = orderDB
			a.db.Upsert(dbOrderGroup.Ticket, dbOrderGroup)
		}
		// diff take price order
		if hasDBOrder && hasChangedTakeProfit(dbOrderGroup.TakeProfit, mt5Order) {
			orderDB, err := a.replaceOrder(*dbOrderGroup.TakeProfit, mt5Order.TakeProfit)
			if err != nil {
				return err
			}
			dbOrderGroup.TakeProfit = orderDB
			a.db.Upsert(dbOrderGroup.Ticket, dbOrderGroup)
		}

		if hasDBOrder && hasAddedTakeProfit(dbOrderGroup, mt5Order) {
			err := a.placeTakeProfit(accountID, mt5Order, dbOrderGroup)
			if err != nil {
				return err
			}
		}

		if hasDBOrder && hasAddedStopLoss(dbOrderGroup, mt5Order) {
			err := a.placeStopLoss(accountID, mt5Order, dbOrderGroup)
			if err != nil {
				return err
			}
		}

		if !hasDBOrder && recentOrder(tNow, mt5Order.UpdatedAt) {
			err := a.placeNewOrder(accountID, mt5Order)
			if err != nil {
				return err
			}
		}
	}

	for _, mt5Position := range req.Positions {
		dbOrderGroup, hasDBOrder := a.db.Get(mt5Position.Ticket)

		if hasDBOrder && hasAddedTakeProfit(dbOrderGroup, mt5Position) {
			err := a.placeTakeProfit(accountID, mt5Position, dbOrderGroup)
			if err != nil {
				return err
			}
		}

		if hasDBOrder && hasAddedStopLoss(dbOrderGroup, mt5Position) {
			err := a.placeStopLoss(accountID, mt5Position, dbOrderGroup)
			if err != nil {
				return err
			}
		}

		if hasDBOrder && hasChangedStopLoss(dbOrderGroup.StopLoss, mt5Position) {
			orderDB, err := a.replaceStopOrder(*dbOrderGroup.StopLoss, mt5Position.StopLoss)
			if err != nil {
				return err
			}
			dbOrderGroup.StopLoss = orderDB
			a.db.Upsert(dbOrderGroup.Ticket, dbOrderGroup)
		}
		if hasDBOrder && hasChangedTakeProfit(dbOrderGroup.TakeProfit, mt5Position) {
			orderDB, err := a.replaceOrder(*dbOrderGroup.TakeProfit, mt5Position.TakeProfit)
			if err != nil {
				return err
			}
			dbOrderGroup.TakeProfit = orderDB
			a.db.Upsert(dbOrderGroup.Ticket, dbOrderGroup)
		}
	}

	return nil
}

func hasChangedPrice(dbOrder *orderdb.OrderDB, mt5Order Mt5Order) bool {
	if dbOrder == nil {
		return false
	}
	return dbOrder.Price != utils.Convert5Decimals(mt5Order.Price)
}

func hasChangedTakeProfit(dbOrder *orderdb.OrderDB, mt5Order Mt5Order) bool {
	if dbOrder == nil {
		return false
	}
	return dbOrder.Price != utils.Convert5Decimals(mt5Order.TakeProfit)
}

func hasAddedTakeProfit(dbOrder orderdb.OrderGroup, mt5Order Mt5Order) bool {
	return dbOrder.TakeProfit == nil && mt5Order.TakeProfit > 0
}

func hasAddedStopLoss(dbOrder orderdb.OrderGroup, mt5Order Mt5Order) bool {
	return dbOrder.StopLoss == nil && mt5Order.StopLoss > 0
}

func hasChangedStopLoss(dbOrder *orderdb.OrderDB, mt5Order Mt5Order) bool {
	if dbOrder == nil {
		return false
	}

	return dbOrder.StopPrice != utils.Convert5Decimals(mt5Order.StopLoss)
}

func hasCancelledStopLoss(dbOrderGroup *orderdb.OrderGroup, mt5Order Mt5Order) bool {
	return dbOrderGroup.StopLoss != nil && mt5Order.StopLoss == 0
}

func hasCancelledTakeProfit(dbOrderGroup *orderdb.OrderGroup, mt5Order Mt5Order) bool {
	return dbOrderGroup.TakeProfit != nil && mt5Order.TakeProfit == 0
}

func (a *Api) placeNewOrder(accountID string, order Mt5Order) error {
	exchange, has := a.exchange.GetByMTValue(order.Symbol)
	if !has {
		return fmt.Errorf("no exchange found")
	}

	exanteOrders, err := a.exanteApi.PlaceOrderV3(&exante.OrderSentTypeV3{
		SymbolID:   exchange.Exante,
		Duration:   "good_till_cancel",
		OrderType:  convertOrderType(order.Type),
		Quantity:   utils.Convert5Decimals(order.Volume * exchange.PriceStep),
		Side:       convertOrderSide(order.Type),
		LimitPrice: utils.Convert5Decimals(order.Price),
		Instrument: exchange.Exante,
		StopLoss:   utils.Convert5DecimalsOrNil(order.StopLoss),
		TakeProfit: utils.Convert5DecimalsOrNil(order.TakeProfit),
		AccountID:  accountID,
	})
	if err != nil {
		return err
	}

	orderDB := orderdb.NewOrderGroupWithTicket(order.Ticket)

	parentOrder, has := utils.GetParentOrder(exanteOrders)
	if has {
		orderDB.Order = *utils.ConvertExOrderToDB(*parentOrder)
	}

	slOrder, has := utils.GetStopLossOrder(exanteOrders)
	if has {
		orderDB.StopLoss = utils.ConvertExOrderToDB(*slOrder)
	}

	tpOrder, has := utils.GetTakeProfitOrder(exanteOrders)
	if has {
		orderDB.TakeProfit = utils.ConvertExOrderToDB(*tpOrder)
	}

	a.db.Upsert(order.Ticket, orderDB)

	return nil
}

func (a *Api) placeStopLoss(accountID string, order Mt5Order, orderGroup orderdb.OrderGroup) error {
	exchange, has := a.exchange.GetByMTValue(order.Symbol)
	if !has {
		return fmt.Errorf("no exchange found")
	}

	exanteOrders, err := a.exanteApi.PlaceOrderV3(&exante.OrderSentTypeV3{
		SymbolID:       exchange.Exante,
		Duration:       "good_till_cancel",
		OrderType:      "stop",
		Quantity:       utils.Convert5Decimals(order.Volume * exchange.PriceStep),
		Side:           utils.GetReverseOrderSide(convertOrderSide(order.Type)),
		StopPrice:      utils.Convert5DecimalsOrNil(order.StopLoss),
		Instrument:     exchange.Exante,
		StopLoss:       utils.Convert5DecimalsOrNil(order.StopLoss),
		TakeProfit:     utils.Convert5DecimalsOrNil(order.TakeProfit),
		AccountID:      accountID,
		IfDoneParentID: orderGroup.Order.ID,
		OcoGroup:       orderGroup.OcoGroup,
	})
	if err != nil {
		return err
	}

	lossOrder, has := utils.GetStopLossOrder(exanteOrders)
	if !has {
		return fmt.Errorf("cannot find stop loss order")
	}

	orderGroup.StopLoss = utils.ConvertExOrderToDB(*lossOrder)
	a.db.Upsert(order.Ticket, orderGroup)

	return nil
}

func (a *Api) placeTakeProfit(accountID string, order Mt5Order, orderGroup orderdb.OrderGroup) error {
	exchange, has := a.exchange.GetByMTValue(order.Symbol)
	if !has {
		return fmt.Errorf("no exchange found")
	}

	exanteOrders, err := a.exanteApi.PlaceOrderV3(&exante.OrderSentTypeV3{
		SymbolID:       exchange.Exante,
		Duration:       "good_till_cancel",
		OrderType:      "limit",
		Quantity:       utils.Convert5Decimals(order.Volume * exchange.PriceStep),
		Side:           utils.GetReverseOrderSide(convertOrderSide(order.Type)),
		LimitPrice:     utils.Convert5Decimals(order.TakeProfit),
		Instrument:     exchange.Exante,
		StopLoss:       utils.Convert5DecimalsOrNil(order.StopLoss),
		TakeProfit:     utils.Convert5DecimalsOrNil(order.TakeProfit),
		AccountID:      accountID,
		IfDoneParentID: orderGroup.Order.ID,
		OcoGroup:       orderGroup.OcoGroup,
	})
	if err != nil {
		return err
	}

	orderGroup.TakeProfit = utils.ConvertExOrderToDB(exanteOrders[0])
	a.db.Upsert(order.Ticket, orderGroup)

	return nil
}

func (a *Api) replaceOrder(order orderdb.OrderDB, price float64) (*orderdb.OrderDB, error) {
	orderExante, err := a.exanteApi.ReplaceOrder(order.ID, exante.ReplaceOrderPayload{
		Action: "replace",
		Parameters: exante.ReplaceOrderParameters{
			Quantity:   order.Quantity,
			LimitPrice: utils.Convert5Decimals(price),
		},
	})
	if err != nil {
		return nil, err
	}

	return utils.ConvertExOrderToDB(*orderExante), nil

}

func (a *Api) replaceStopOrder(order orderdb.OrderDB, price float64) (*orderdb.OrderDB, error) {
	orderExante, err := a.exanteApi.ReplaceOrder(order.ID, exante.ReplaceOrderPayload{
		Action: "replace",
		Parameters: exante.ReplaceOrderParameters{
			Quantity:  order.Quantity,
			StopPrice: utils.Convert5Decimals(price),
		},
	})
	if err != nil {
		return nil, err
	}

	return utils.ConvertExOrderToDB(*orderExante), nil

}

func (a *Api) cancelOrder(orderGroupDB orderdb.OrderGroup) error {
	err := a.exanteApi.CancelOrder(orderGroupDB.Order.ID)
	if err != nil {
		if strings.Contains(err.Error(), "Unable to modify order") {
			a.db.Delete(orderGroupDB.Ticket)
			return nil
		}
		return err
	}

	a.db.Delete(orderGroupDB.Ticket)
	return nil
}

func recentOrder(tNow, updatedAt time.Time) bool {
	return tNow.Sub(updatedAt).Seconds() < 60*60
}

func convertOrderType(ot OrderType) string {
	switch ot {
	case OrderTypeBuy:
		return "market"
	case OrderTypeSell:
		return "market"

	}

	return "limit"
}

func convertOrderSide(ot OrderType) string {
	if strings.Contains(string(ot), "BUY") {
		return "buy"
	}
	return "sell"
}

func (a *Api) Flush() error {
	return a.db.Flush()
}
