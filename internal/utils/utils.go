package utils

import (
	"fmt"
	"mt-to-exante/internal/exante"
	"mt-to-exante/internal/orderdb"
)

func Convert5Decimals(k float64) string {
	return fmt.Sprintf("%.5f", k)
}

func ConvertExOrderToDB(v3 exante.OrderV3) *orderdb.OrderDB {
	return &orderdb.OrderDB{
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

func GetReverseOrderSide(side string) string {
	if side == "buy" {
		return "sell"
	}
	return "buy"
}
func GetParentOrder(orders []exante.OrderV3) (*exante.OrderV3, bool) {
	for _, order := range orders {
		if len(order.OrderParameters.OcoGroup) == 0 {
			return &order, true
		}
	}

	return nil, false
}
func GetTakeProfitOrder(orders []exante.OrderV3) (*exante.OrderV3, bool) {
	for _, order := range orders {
		if len(order.OrderParameters.OcoGroup) > 0 && order.OrderParameters.OrderType == "limit" {
			return &order, true
		}
	}

	return nil, false
}

func GetStopLossOrder(orders []exante.OrderV3) (*exante.OrderV3, bool) {
	for _, order := range orders {
		if len(order.OrderParameters.OcoGroup) > 0 && order.OrderParameters.OrderType != "limit" {
			return &order, true
		}
	}

	return nil, false
}
