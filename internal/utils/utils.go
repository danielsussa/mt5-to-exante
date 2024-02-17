package utils

import (
	"fmt"
	"github.com/danielsussa/mt5-to-exante/internal/exante"
	"github.com/danielsussa/mt5-to-exante/internal/orderdb"
	"github.com/google/uuid"
	"strconv"
)

func Convert5Decimals(k float64) string {
	return fmt.Sprintf("%.5f", k)
}

func ConvertNDecimals(k float64) string {
	return strconv.FormatFloat(k, 'f', -1, 64)
}

func ConvertExOrderToDB(v3 exante.OrderV3) *orderdb.OrderDB {
	return &orderdb.OrderDB{
		ID:         v3.OrderID,
		StopPrice:  v3.OrderParameters.StopPrice,
		Price:      v3.OrderParameters.LimitPrice,
		Quantity:   v3.OrderParameters.Quantity,
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

func IsPositionClosed(orders []exante.OrderV3) bool {
	if len(orders) == 0 {
		return true
	}

	hasBuy := false
	hasSell := false

	for _, order := range orders {
		if order.OrderState.Status == exante.FilledStatus && order.OrderParameters.Side == "buy" {
			hasBuy = true
		}
		if order.OrderState.Status == exante.FilledStatus && order.OrderParameters.Side == "sell" {
			hasSell = true
		}
	}

	return hasSell && hasBuy
}

func GetOCOGroup(orders []exante.OrderV3) string {
	for _, order := range orders {
		if len(order.OrderParameters.OcoGroup) > 0 {
			return order.OrderParameters.OcoGroup
		}
	}

	return uuid.NewString()
}
func GetStopLossOrder(orders []exante.OrderV3) (*exante.OrderV3, bool) {
	for _, order := range orders {
		if len(order.OrderParameters.OcoGroup) > 0 && order.OrderParameters.OrderType != "limit" {
			return &order, true
		}
	}

	return nil, false
}

func ConvertNDecimalsOrNil(k float64) *string {
	if k > 0 {
		valS := ConvertNDecimals(k)
		return &valS
	}
	return nil
}

func Convert5DecimalsOrNil(k float64) *string {
	if k > 0 {
		valS := Convert5Decimals(k)
		return &valS
	}
	return nil
}

func Hash(v any) string {
	return fmt.Sprintf("%v", v)
}
