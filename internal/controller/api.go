package controller

import (
	"fmt"
	"github.com/danielsussa/mt5-to-exante/internal/exante"
	"github.com/danielsussa/mt5-to-exante/internal/exchanges"
	"github.com/danielsussa/mt5-to-exante/internal/utils"
	"strings"
	"time"
)

type Api struct {
	exanteApi exante.Iface
	exchange  exchanges.Api

	history map[string]string
}

func New(exanteApi exante.Iface, exchange exchanges.Api) *Api {
	return &Api{
		exanteApi: exanteApi,
		exchange:  exchange,
		history:   make(map[string]string),
	}
}

type (
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

	SyncResponse struct {
		Journal  []string `json:"journal"`
		JournalF string   `json:"journalF"`
	}

	Mt5OrderHistory struct {
		Ticket    string
		State     OrderState
		UpdatedAt time.Time
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
		Symbol         string
		Ticket         string
		PositionTicket string
		Volume         float64
		TakeProfit     float64
		StopLoss       float64
		Price          float64
	}

	Mt5PositionHistory struct {
		Symbol         string
		Entry          DealEntry
		Reason         DealReason
		Ticket         string
		PositionTicket string
		Volume         float64
		TakeProfit     float64
		StopLoss       float64
		Price          float64
	}

	OrderState string
	OrderType  string
	DealEntry  string
	DealReason string
)

func (r DealReason) IsStop() bool {
	return r == DealReasonSL || r == DealReasonTP
}

func (m Mt5Position) WithTicket() string {
	return fmt.Sprintf("position-%s", m.Ticket)
}

func (m Mt5PositionHistory) WithTicket() string {
	return fmt.Sprintf("position-history-%s", m.Ticket)
}

func (sr *SyncResponse) AddJournal(txt string) {
	if len(sr.JournalF) == 0 {
		sr.JournalF += txt
		return
	}
	sr.JournalF += "\n" + txt
}

func (m Mt5PositionHistory) ToHistory() Mt5Position {
	return Mt5Position{
		Symbol:         m.Symbol,
		Ticket:         m.Ticket,
		PositionTicket: m.PositionTicket,
		Volume:         m.Volume,
		TakeProfit:     m.TakeProfit,
		StopLoss:       m.StopLoss,
		Price:          m.Price,
	}
}

func (m Mt5Order) WithTicket() string {
	return fmt.Sprintf("order-%s", m.Ticket)
}

func (m Mt5OrderHistory) WithTicket() string {
	return fmt.Sprintf("order-history-%s", m.Ticket)
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

	DealReasonSL     DealReason = "DEAL_REASON_SL"
	DealReasonTP     DealReason = "DEAL_REASON_TP"
	DealReasonClient DealReason = "DEAL_REASON_CLIENT"
)

func (t OrderType) IsLimit() bool {
	return strings.Contains(string(t), "LIMIT")
}

func (a *Api) Sync(accountID string, req SyncRequest) (SyncResponse, error) {
	res := SyncResponse{}

	// recent inactive orders are responsible for:
	// 1. Cancel an order
	// 1. Open market order
	for _, currentMT5InactiveOrder := range req.RecentInactiveOrders {
		if !a.isNewRequest(currentMT5InactiveOrder) {
			continue
		}

		// Open market order
		if currentMT5InactiveOrder.State == OrderStateFilled {
			_, err := a.placeNewOrder(accountID, currentMT5InactiveOrder)
			if err != nil {
				return res, err
			}
			res.AddJournal(fmt.Sprintf("[%s] ORDER(HIST) > MKT ORDER", currentMT5InactiveOrder.Ticket))
		}

		// Cancel an order
		if currentMT5InactiveOrder.State == OrderStateCancelled {
			exanteActiveOrders, err := a.findActiveOrdersByTicket(currentMT5InactiveOrder.Ticket, accountID)
			if err != nil {
				return res, err
			}
			exanteParentOrder, hasParentOrder := utils.GetParentOrder(exanteActiveOrders)
			if !hasParentOrder {
				continue
			}

			err = a.cancelOrder(exanteParentOrder.OrderID)
			if err != nil {
				return res, err
			}
			res.AddJournal(fmt.Sprintf("[%s] ORD(HIST) > CANCEL ORDER", currentMT5InactiveOrder.Ticket))
		}

		a.appendRequest(currentMT5InactiveOrder)
	}

	return res, nil
}

func (a *Api) cancelOrder(orderID string) error {
	err := a.exanteApi.CancelOrder(orderID)
	if err != nil {
		if strings.Contains(err.Error(), "Unable to modify") {
			return nil
		}
		return err
	}
	return nil
}

func hasInactiveFilledOrder(ticket string) func(order Mt5Order) bool {
	return func(order Mt5Order) bool {
		return order.Ticket == ticket && order.State == OrderStateFilled
	}
}

func (a *Api) replaceUpdatePositionHistory(history Mt5Position, accountID string) error {
	orders, err := a.findActiveAndFilledOrdersByTicket(history.Ticket, accountID)
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

func (a *Api) replaceTPOrder(price float64, orderID string) error {
	tpOrder, err := a.exanteApi.GetOrder(orderID)
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

func (a *Api) replaceSLOrder(price float64, orderID string) error {
	slOrder, err := a.exanteApi.GetOrder(orderID)
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

func (a *Api) findActiveOrdersByTicket(ticket string, accountID string) ([]exante.OrderV3, error) {
	orders, err := a.exanteApi.GetOrdersByLimitV3(1000, accountID)
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

func (a *Api) findActiveAndFilledOrdersByTicket(ticket string, accountID string) ([]exante.OrderV3, error) {
	orders, err := a.exanteApi.GetOrdersByLimitV3(100, accountID)
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

func (a *Api) findFilledOrdersByTicket(ticket string, accountID string) ([]exante.OrderV3, error) {
	orders, err := a.exanteApi.GetOrdersByLimitV3(100, accountID)
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
