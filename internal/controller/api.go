package controller

import (
	"fmt"
	"github.com/danielsussa/mt5-to-exante/internal/exante"
	"github.com/danielsussa/mt5-to-exante/internal/exchanges"
	"github.com/danielsussa/mt5-to-exante/internal/utils"
	"slices"
	"strings"
	"time"
)

type Api struct {
	exanteApi exante.Iface
	exchange  exchanges.Api

	history      map[string]string
	currentState State
}

func New(exanteApi exante.Iface, exchange exchanges.Api) *Api {
	return &Api{
		currentState: State{
			ActivePositions: make(map[string]*StatePosition),
			ActiveOrders:    make(map[string]*StateOrder),
		},
		exanteApi: exanteApi,
		exchange:  exchange,
		history:   make(map[string]string),
	}
}

type (
	State struct {
		ActivePositions map[string]*StatePosition
		ActiveOrders    map[string]*StateOrder
	}

	StateOrder struct {
		Mt5Order         Mt5Order
		ExanteOrderGroup ExanteOrderGroup
	}

	StatePosition struct {
		Mt5Order         Mt5Position
		ExanteOrderGroup ExanteOrderGroup
	}

	ExanteOrderGroup struct {
		IsManageable bool
		ParentOrder  string
		TPOrder      string
		SLOrder      string
	}
	Mt5Requests interface {
		WithTicket() string
	}

	SyncRequest struct {
		// for each position the program should:
		// 1. check if there is any empty state on State.ActivePositions
		ActivePositions         []Mt5Position
		RecentInactivePositions []Mt5PositionHistory

		ActiveOrders         []Mt5Order
		RecentInactiveOrders []Mt5Order
	}

	Mt5OrderHistory struct {
		Ticket    string
		State     OrderState
		UpdatedAt time.Time
	}

	Mt5PositionHistory struct {
		Ticket string
		Entry  DealEntry
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
	}

	Mt5Position struct {
		Symbol     string
		Ticket     string
		Volume     float64
		TakeProfit float64
		StopLoss   float64
		Price      float64
	}

	OrderState string
	OrderType  string
	DealEntry  string
)

func (m Mt5Position) WithTicket() string {
	return m.Ticket
}

func (m Mt5Order) WithTicket() string {
	return m.Ticket
}

func (m Mt5OrderHistory) WithTicket() string {
	return m.Ticket
}

func (m Mt5PositionHistory) WithTicket() string {
	return m.Ticket
}

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

	DealEntryIn  DealEntry = "DEAL_ENTRY_IN"
	DealEntryOut DealEntry = "DEAL_ENTRY_OUT"
)

func (a *Api) Sync(accountID string, req SyncRequest) error {
	// collect new positions for the state
	for _, currentMT5Position := range req.ActivePositions {
		if !a.isNewRequest(currentMT5Position) {
			continue
		}

		// check for new state
		if activePosition, has := a.currentState.ActivePositions[currentMT5Position.Ticket]; has {
			if activePosition.Mt5Order.StopLoss != currentMT5Position.StopLoss {
				err := a.replaceSLOrder(currentMT5Position.StopLoss, activePosition.ExanteOrderGroup)
				if err != nil {
					return err
				}
			}
			if activePosition.Mt5Order.TakeProfit != currentMT5Position.TakeProfit {
				err := a.replaceTPOrder(currentMT5Position.TakeProfit, activePosition.ExanteOrderGroup)
				if err != nil {
					return err
				}
			}
		} else {
			// there is no state
			exanteOrders, err := a.findActiveAndFilledOrdersByTicket(currentMT5Position.Ticket)
			if err != nil {
				return err
			}

			exanteOrderGroup := exanteOrdersToOrderGroup(exanteOrders)

			// check for recent filled orders
			if idx := slices.IndexFunc(req.RecentInactiveOrders, hasInactiveFilledOrder(currentMT5Position.Ticket)); idx > -1 {
				recentMt5Order := req.RecentInactiveOrders[idx]
				parentOrder, hasParentOrder := utils.GetParentOrder(exanteOrders)

				if hasParentOrder && parentOrder.OrderState.Status == exante.FilledStatus && recentMt5Order.State == OrderStateFilled {
					continue
				}
				if strings.Contains(string(recentMt5Order.Type), "LIMIT") {
					continue
				}

				newExanteOrders, err := a.placeNewOrder(accountID, recentMt5Order)
				if err != nil {
					return err
				}
				exanteOrderGroup = exanteOrdersToOrderGroup(append(exanteOrders, newExanteOrders...))
			}
			a.currentState.ActivePositions[currentMT5Position.Ticket] = &StatePosition{
				Mt5Order:         currentMT5Position,
				ExanteOrderGroup: exanteOrderGroup,
			}
		}

		a.appendRequest(currentMT5Position)
	}

	for _, currentMT5OldPosition := range req.RecentInactivePositions {
		if !a.isNewRequest(currentMT5OldPosition) {
			continue
		}

		if currentMT5OldPosition.Entry != DealEntryOut {
			a.appendRequest(currentMT5OldPosition)
			continue
		}

		exanteOrders, err := a.findActiveAndFilledOrdersByTicket(currentMT5OldPosition.Ticket)
		if err != nil {
			return err
		}

		tpOrder, hasTPOrder := utils.GetTakeProfitOrder(exanteOrders)
		if hasTPOrder {
			err := a.exanteApi.CancelOrder(tpOrder.OrderID)
			if err != nil {
				return err
			}
		}

		slOrder, hasSLOrder := utils.GetStopLossOrder(exanteOrders)
		if hasSLOrder {
			err := a.exanteApi.CancelOrder(slOrder.OrderID)
			if err != nil {
				return err
			}
		}

		if idx := slices.IndexFunc(req.RecentInactiveOrders, hasInactiveFilledOrder(currentMT5OldPosition.Ticket)); idx > -1 {
			recentMt5Order := req.RecentInactiveOrders[idx]

			_, err := a.closePosition(accountID, recentMt5Order)
			if err != nil {
				return err
			}
		}

		delete(a.currentState.ActivePositions, currentMT5OldPosition.Ticket)
		a.appendRequest(currentMT5OldPosition)
	}

	for _, currentMT5Order := range req.ActiveOrders {
		if !a.isNewRequest(currentMT5Order) {
			continue
		}

		exanteActiveOrders, err := a.findActiveOrdersByTicket(currentMT5Order.Ticket)
		if err != nil {
			return err
		}

		ocoGroup := utils.GetOCOGroup(exanteActiveOrders)
		exanteParentOrder, _ := utils.GetParentOrder(exanteActiveOrders)

		if stateActiveOrder, has := a.currentState.ActiveOrders[currentMT5Order.Ticket]; has {
			if stateActiveOrder.Mt5Order.Price != currentMT5Order.Price {
				err := a.replaceParentOrder(currentMT5Order.Price, stateActiveOrder.ExanteOrderGroup)
				if err != nil {
					return err
				}
				stateActiveOrder.Mt5Order.Price = currentMT5Order.Price
			}
			if stateActiveOrder.Mt5Order.StopLoss != currentMT5Order.StopLoss {
				if stateActiveOrder.Mt5Order.StopLoss == 0 {
					order, err := a.placeStopLoss(currentMT5Order.StopLoss, *exanteParentOrder, ocoGroup)
					if err != nil {
						return err
					}
					stateActiveOrder.ExanteOrderGroup.SLOrder = order.OrderID
				} else if currentMT5Order.StopLoss == 0 {
					err := a.exanteApi.CancelOrder(stateActiveOrder.ExanteOrderGroup.SLOrder)
					if err != nil {
						return err
					}
					stateActiveOrder.ExanteOrderGroup.SLOrder = ""
					stateActiveOrder.Mt5Order.StopLoss = 0
				} else {
					err := a.replaceSLOrder(currentMT5Order.StopLoss, stateActiveOrder.ExanteOrderGroup)
					if err != nil {
						return err
					}
					stateActiveOrder.Mt5Order.StopLoss = currentMT5Order.StopLoss
				}

			}
			if stateActiveOrder.Mt5Order.TakeProfit != currentMT5Order.TakeProfit {
				if stateActiveOrder.Mt5Order.TakeProfit == 0 {
					order, err := a.placeTakeProfit(currentMT5Order.TakeProfit, *exanteParentOrder, ocoGroup)
					if err != nil {
						return err
					}
					stateActiveOrder.ExanteOrderGroup.TPOrder = order.OrderID
				} else if currentMT5Order.TakeProfit == 0 {
					err := a.exanteApi.CancelOrder(stateActiveOrder.ExanteOrderGroup.TPOrder)
					if err != nil {
						return err
					}
					stateActiveOrder.ExanteOrderGroup.TPOrder = ""
					stateActiveOrder.Mt5Order.TakeProfit = 0
				} else {
					err := a.replaceTPOrder(currentMT5Order.TakeProfit, stateActiveOrder.ExanteOrderGroup)
					if err != nil {
						return err
					}
					stateActiveOrder.Mt5Order.TakeProfit = currentMT5Order.TakeProfit
				}
			}
			stateActiveOrder.Mt5Order = currentMT5Order
		} else {

			exanteOrderGroup := exanteOrdersToOrderGroup(exanteActiveOrders)

			if len(exanteActiveOrders) == 0 {
				exanteActiveOrders, err = a.placeNewOrder(accountID, currentMT5Order)
				if err != nil {
					return err
				}

				exanteOrderGroup = exanteOrdersToOrderGroup(exanteActiveOrders)
			}

			a.currentState.ActiveOrders[currentMT5Order.Ticket] = &StateOrder{
				Mt5Order:         currentMT5Order,
				ExanteOrderGroup: exanteOrderGroup,
			}
		}

		a.appendRequest(currentMT5Order)
	}

	for _, currentMT5InactiveOrder := range req.RecentInactiveOrders {
		if !a.isNewRequest(currentMT5InactiveOrder) {
			continue
		}

		if stateActiveOrder, has := a.currentState.ActiveOrders[currentMT5InactiveOrder.Ticket]; has {
			if currentMT5InactiveOrder.State == OrderStateCancelled {
				err := a.exanteApi.CancelOrder(stateActiveOrder.ExanteOrderGroup.ParentOrder)
				if err != nil {
					return err
				}
				delete(a.currentState.ActiveOrders, currentMT5InactiveOrder.Ticket)
			}
		}

		a.appendRequest(currentMT5InactiveOrder)
	}

	return nil
}

func hasInactiveFilledOrder(ticket string) func(order Mt5Order) bool {
	return func(order Mt5Order) bool {
		return order.Ticket == ticket && order.State == OrderStateFilled
	}
}

func (a *Api) replaceUpdatePositionHistory(history Mt5PositionHistory) error {
	orders, err := a.findActiveAndFilledOrdersByTicket(history.Ticket)
	if err != nil {
		return err
	}

	if utils.IsPositionClosed(orders) {
		return nil
	}

	parentOrder, _ := utils.GetParentOrder(orders)

	_, err = a.exanteApi.PlaceOrderV3(&exante.OrderSentTypeV3{
		AccountID:  parentOrder.AccountID,
		Instrument: parentOrder.OrderParameters.SymbolId,
		Side:       utils.GetReverseOrderSide(parentOrder.OrderParameters.Side),
		Quantity:   parentOrder.OrderParameters.Quantity,
		OcoGroup:   utils.GetOCOGroup(orders),
		Duration:   "good_till_cancel",
		OrderType:  "market",
		SymbolID:   parentOrder.OrderParameters.SymbolId,
		ClientTag:  parentOrder.ClientTag,
	})

	return err
}

func (a *Api) isNewRequest(mt5Res Mt5Requests) bool {
	if val, has := a.history[mt5Res.WithTicket()]; has {
		return val != utils.Hash(mt5Res)
	}

	return true
}

func (a *Api) appendRequest(mt5Res Mt5Requests) {
	a.history[mt5Res.WithTicket()] = utils.Hash(mt5Res)
}

func (a *Api) placeNewOrder(accountID string, order Mt5Order) ([]exante.OrderV3, error) {
	exchange, has := a.exchange.GetByMTValue(order.Symbol)
	if !has {
		return nil, nil
	}

	orders, err := a.exanteApi.PlaceOrderV3(&exante.OrderSentTypeV3{
		SymbolID:   exchange.Exante,
		Duration:   "good_till_cancel",
		OrderType:  convertOrderType(order.Type),
		Quantity:   utils.Convert5Decimals(order.Volume * exchange.PriceStep),
		Side:       convertOrderSide(order.Type),
		LimitPrice: utils.ConvertNDecimals(order.Price),
		Instrument: exchange.Exante,
		StopLoss:   utils.ConvertNDecimalsOrNil(order.StopLoss),
		TakeProfit: utils.ConvertNDecimalsOrNil(order.TakeProfit),
		ClientTag:  order.Ticket,
		AccountID:  accountID,
	})
	if err != nil {
		return nil, err
	}

	return orders, nil
}

func (a *Api) closePosition(accountID string, order Mt5Order) ([]exante.OrderV3, error) {
	exchange, has := a.exchange.GetByMTValue(order.Symbol)
	if !has {
		return nil, nil
	}

	orders, err := a.exanteApi.PlaceOrderV3(&exante.OrderSentTypeV3{
		SymbolID:   exchange.Exante,
		Duration:   "good_till_cancel",
		OrderType:  convertOrderType(order.Type),
		Quantity:   utils.Convert5Decimals(order.Volume * exchange.PriceStep),
		Side:       convertOrderSide(order.Type),
		LimitPrice: utils.ConvertNDecimals(order.Price),
		Instrument: exchange.Exante,
		ClientTag:  order.Ticket,
		AccountID:  accountID,
	})
	if err != nil {
		return nil, err
	}

	return orders, nil
}

func (a *Api) placeStopLoss(price float64, exanteOrder exante.OrderV3, ocoGroup string) (*exante.OrderV3, error) {

	orders, err := a.exanteApi.PlaceOrderV3(&exante.OrderSentTypeV3{
		SymbolID:       exanteOrder.OrderParameters.SymbolId,
		Duration:       "good_till_cancel",
		OrderType:      "stop",
		Quantity:       exanteOrder.OrderParameters.Quantity,
		Side:           utils.GetReverseOrderSide(exanteOrder.OrderParameters.Side),
		StopPrice:      utils.ConvertNDecimalsOrNil(price),
		StopLoss:       utils.ConvertNDecimalsOrNil(price),
		Instrument:     exanteOrder.OrderParameters.SymbolId,
		AccountID:      exanteOrder.AccountID,
		IfDoneParentID: exanteOrder.OrderID,
		OcoGroup:       ocoGroup,
		ClientTag:      exanteOrder.ClientTag,
	})
	if err != nil {
		return nil, err
	}

	order, has := utils.GetStopLossOrder(orders)
	if !has {
		return nil, fmt.Errorf("couldnt create take SL order")
	}

	return order, nil
}

func (a *Api) placeTakeProfit(price float64, exanteOrder exante.OrderV3, ocoGroup string) (*exante.OrderV3, error) {
	orders, err := a.exanteApi.PlaceOrderV3(&exante.OrderSentTypeV3{
		SymbolID:       exanteOrder.OrderParameters.SymbolId,
		Duration:       "good_till_cancel",
		OrderType:      "limit",
		Quantity:       exanteOrder.OrderParameters.Quantity,
		Side:           utils.GetReverseOrderSide(exanteOrder.OrderParameters.Side),
		LimitPrice:     utils.ConvertNDecimals(price),
		Instrument:     exanteOrder.OrderParameters.SymbolId,
		AccountID:      exanteOrder.AccountID,
		IfDoneParentID: exanteOrder.OrderID,
		OcoGroup:       ocoGroup,
		ClientTag:      exanteOrder.ClientTag,
	})
	if err != nil {
		return nil, err
	}

	order, has := utils.GetTakeProfitOrder(orders)
	if !has {
		return nil, fmt.Errorf("couldnt create take profit order")
	}

	return order, nil
}

func (a *Api) replaceTPOrder(price float64, state ExanteOrderGroup) error {
	tpOrder, err := a.exanteApi.GetOrder(state.TPOrder)
	if err != nil {
		return err
	}

	_, err = a.exanteApi.ReplaceOrder(tpOrder.OrderID, exante.ReplaceOrderPayload{
		Action: "replace",
		Parameters: exante.ReplaceOrderParameters{
			Quantity:   tpOrder.OrderParameters.Quantity,
			LimitPrice: utils.ConvertNDecimals(price),
		},
	})
	if err != nil {
		if strings.Contains(err.Error(), "Unable to modify order") {
			return nil
		}
		return err
	}

	return nil
}

func (a *Api) replaceParentOrder(price float64, state ExanteOrderGroup) error {
	tpOrder, err := a.exanteApi.GetOrder(state.ParentOrder)
	if err != nil {
		return err
	}

	_, err = a.exanteApi.ReplaceOrder(tpOrder.OrderID, exante.ReplaceOrderPayload{
		Action: "replace",
		Parameters: exante.ReplaceOrderParameters{
			Quantity:   tpOrder.OrderParameters.Quantity,
			LimitPrice: utils.ConvertNDecimals(price),
		},
	})
	if err != nil {
		if strings.Contains(err.Error(), "Unable to modify order") {
			return nil
		}
		return err
	}

	return nil
}

func (a *Api) replaceSLOrder(price float64, state ExanteOrderGroup) error {
	slOrder, err := a.exanteApi.GetOrder(state.SLOrder)
	if err != nil {
		return err
	}

	_, err = a.exanteApi.ReplaceOrder(slOrder.OrderID, exante.ReplaceOrderPayload{
		Action: "replace",
		Parameters: exante.ReplaceOrderParameters{
			Quantity:  slOrder.OrderParameters.Quantity,
			StopPrice: utils.ConvertNDecimals(price),
		},
	})
	if err != nil {
		if strings.Contains(err.Error(), "Unable to modify order") {
			return nil
		}
		return err
	}

	return nil
}

func (a *Api) replaceMainOrder(mt5Order Mt5Order, exanteOrder exante.OrderV3) error {
	_, err := a.exanteApi.ReplaceOrder(exanteOrder.OrderID, exante.ReplaceOrderPayload{
		Action: "replace",
		Parameters: exante.ReplaceOrderParameters{
			Quantity:   exanteOrder.OrderParameters.Quantity,
			LimitPrice: utils.Convert5Decimals(mt5Order.Price),
		},
	})
	if err != nil {
		if strings.Contains(err.Error(), "Unable to modify order") {
			return nil
		}
		return err
	}

	return nil
}

func (a *Api) findActiveOrdersByTicket(ticket string) ([]exante.OrderV3, error) {
	orders, err := a.exanteApi.GetOrdersByLimitV3(100)
	if err != nil {
		return nil, err
	}

	returnOrders := make([]exante.OrderV3, 0)
	for _, order := range orders {
		if order.ClientTag == ticket && (order.OrderState.Status == exante.WorkingStatus || order.OrderState.Status == exante.PendingStatus) {
			returnOrders = append(returnOrders, order)
		}
	}

	return returnOrders, nil
}

func (a *Api) findActiveAndFilledOrdersByTicket(ticket string) ([]exante.OrderV3, error) {
	orders, err := a.exanteApi.GetOrdersByLimitV3(100)
	if err != nil {
		return nil, err
	}

	returnOrders := make([]exante.OrderV3, 0)
	for _, order := range orders {
		if order.ClientTag == ticket && (order.OrderState.Status == exante.WorkingStatus || order.OrderState.Status == exante.PendingStatus || order.OrderState.Status == exante.FilledStatus) {
			returnOrders = append(returnOrders, order)
		}
	}

	return returnOrders, nil
}

func (a *Api) findFilledOrdersByTicket(ticket string) ([]exante.OrderV3, error) {
	orders, err := a.exanteApi.GetOrdersByLimitV3(100)
	if err != nil {
		return nil, err
	}

	returnOrders := make([]exante.OrderV3, 0)
	for _, order := range orders {
		if order.ClientTag == ticket && order.OrderState.Status == exante.FilledStatus {
			returnOrders = append(returnOrders, order)
		}
	}

	return returnOrders, nil
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

func exanteOrdersToOrderGroup(exOrders []exante.OrderV3) ExanteOrderGroup {
	if len(exOrders) == 0 {
		return ExanteOrderGroup{IsManageable: false}
	}
	og := ExanteOrderGroup{IsManageable: true}

	parentOrder, has := utils.GetParentOrder(exOrders)
	if has {
		og.ParentOrder = parentOrder.OrderID
	}

	slOrder, has := utils.GetStopLossOrder(exOrders)
	if has {
		og.SLOrder = slOrder.OrderID
	}

	tpOrder, has := utils.GetTakeProfitOrder(exOrders)
	if has {
		og.TPOrder = tpOrder.OrderID
	}

	return og
}
