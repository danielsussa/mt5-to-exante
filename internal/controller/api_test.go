package controller

import (
	"github.com/danielsussa/mt5-to-exante/internal/exante"
	"github.com/danielsussa/mt5-to-exante/internal/exchanges"
	"github.com/danielsussa/mt5-to-exante/internal/utils"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestApi(t *testing.T) {
	exchange := exchanges.Api{
		Data: exchanges.Data{
			Description: "",
			Exchanges: []exchanges.DataExchanges{
				{
					Exante:     "EUR/USD",
					MetaTrader: "EURUSD",
					PriceStep:  1,
				},
			},
		},
	}

	t.Run("new position was created with TP/SL should have 2 active orders on EXANTE", func(t *testing.T) {

		exanteMock := exante.NewMock(make([]exante.OrderV3, 0))
		c := New(exanteMock, exchange)

		{ // the program started with a recent position, and a recent order is visible
			_, err := c.Sync("acc-1", SyncRequest{
				ActivePositions: []Mt5Position{
					{PositionTicket: "1234", Ticket: "1234", Symbol: "EURUSD", Volume: 1, TakeProfit: 2, StopLoss: 1, Price: 1.2},
				},
				ActiveOrders: nil,
				RecentInactiveOrders: []Mt5Order{
					{Symbol: "EURUSD", Ticket: "1234", Volume: 1, Type: OrderTypeBuy, TakeProfit: 2, StopLoss: 1, Price: 1.2, State: OrderStateFilled},
				},
				RecentInactivePositions: []Mt5PositionHistory{
					{PositionTicket: "1234", Ticket: "1234", Symbol: "EURUSD", Volume: 1, TakeProfit: 2, StopLoss: 1, Price: 1.2, Entry: DealEntryIn},
				},
			})
			assert.NoError(t, err)
			activeOrder, _ := c.exanteApi.GetActiveOrdersV3()
			assert.Len(t, activeOrder, 2)
			allOrders, _ := c.exanteApi.GetOrdersByLimitV3(100)
			assert.Len(t, allOrders, 3)
		}
		{ // lets repeat the same action to check if nothing is deduplicated
			_, err := c.Sync("acc-1", SyncRequest{
				ActivePositions: []Mt5Position{
					{PositionTicket: "1234", Ticket: "1234", Symbol: "EURUSD", Volume: 1, TakeProfit: 2, StopLoss: 1, Price: 1.2},
				},
				ActiveOrders: nil,
				RecentInactiveOrders: []Mt5Order{
					{Symbol: "EURUSD", Ticket: "1234", Volume: 1, Type: OrderTypeBuy, TakeProfit: 2, StopLoss: 1, Price: 1.2, State: OrderStateFilled},
				},
				RecentInactivePositions: []Mt5PositionHistory{
					{PositionTicket: "1234", Ticket: "1234", Symbol: "EURUSD", Volume: 1, TakeProfit: 2, StopLoss: 1, Price: 1.2, Entry: DealEntryIn},
				},
			})
			assert.NoError(t, err)
			activeOrder, _ := c.exanteApi.GetActiveOrdersV3()
			assert.Len(t, activeOrder, 2)
			allOrders, _ := c.exanteApi.GetOrdersByLimitV3(100)
			assert.Len(t, allOrders, 3)
			assert.Equal(t, 1, exanteMock.TotalPlaceOrderV3)
		}
		{ // the recent order will disappear
			_, err := c.Sync("acc-1", SyncRequest{
				ActivePositions: []Mt5Position{
					{PositionTicket: "1234", Ticket: "1234", Symbol: "EURUSD", Volume: 1, TakeProfit: 2, StopLoss: 1, Price: 1.2},
				},
				ActiveOrders:         nil,
				RecentInactiveOrders: []Mt5Order{},
			})
			assert.NoError(t, err)
			activeOrder, _ := c.exanteApi.GetActiveOrdersV3()
			assert.Len(t, activeOrder, 2)
			allOrders, _ := c.exanteApi.GetOrdersByLimitV3(100)
			assert.Len(t, allOrders, 3)
		}
		{ // lets change the stop loss value to 1.1 and 2.1
			_, err := c.Sync("acc-1", SyncRequest{
				ActivePositions: []Mt5Position{
					{PositionTicket: "1234", Ticket: "1234", Symbol: "EURUSD", Volume: 1, TakeProfit: 2.1, StopLoss: 1.1, Price: 1.2},
				},
				ActiveOrders:         nil,
				RecentInactiveOrders: []Mt5Order{},
			})
			assert.NoError(t, err)
			activeOrder, _ := c.exanteApi.GetActiveOrdersV3()
			assert.Len(t, activeOrder, 2)
			allOrders, _ := c.exanteApi.GetOrdersByLimitV3(100)
			assert.Len(t, allOrders, 3)
			slOrder, _ := utils.GetStopLossOrder(allOrders)
			assert.Equal(t, "1.1", slOrder.OrderParameters.StopPrice)
			tpOrder, _ := utils.GetTakeProfitOrder(allOrders)
			assert.Equal(t, "2.1", tpOrder.OrderParameters.LimitPrice)
		}
	})

	t.Run("new order, add stops and cancel order", func(t *testing.T) {

		exanteMock := exante.NewMock(make([]exante.OrderV3, 0))
		c := New(exanteMock, exchange)

		{ // the program started with a recent order
			_, err := c.Sync("acc-1", SyncRequest{
				ActivePositions: []Mt5Position{},
				ActiveOrders: []Mt5Order{
					{Symbol: "EURUSD", Ticket: "1234", Volume: 1, Type: OrderTypeBuyLimit, TakeProfit: 2, Price: 1.2, State: OrderStatePlaced},
				},
			})
			assert.NoError(t, err)
			activeOrder, _ := c.exanteApi.GetActiveOrdersV3()
			assert.Len(t, activeOrder, 2)
			allOrders, _ := c.exanteApi.GetOrdersByLimitV3(100)
			assert.Len(t, allOrders, 2)
		}
		{ // add stop loss
			_, err := c.Sync("acc-1", SyncRequest{
				ActivePositions: []Mt5Position{},
				ActiveOrders: []Mt5Order{
					{Symbol: "EURUSD", Ticket: "1234", Volume: 1, Type: OrderTypeBuyLimit, StopLoss: 1, TakeProfit: 2, Price: 1.2, State: OrderStatePlaced},
				},
			})
			assert.NoError(t, err)
			activeOrder, _ := c.exanteApi.GetActiveOrdersV3()
			assert.Len(t, activeOrder, 3)
			allOrders, _ := c.exanteApi.GetOrdersByLimitV3(100)
			assert.Len(t, allOrders, 3)
		}
		{ // remove take profit
			_, err := c.Sync("acc-1", SyncRequest{
				ActivePositions: []Mt5Position{},
				ActiveOrders: []Mt5Order{
					{Symbol: "EURUSD", Ticket: "1234", Volume: 1, Type: OrderTypeBuyLimit, StopLoss: 1, Price: 1.2, State: OrderStatePlaced},
				},
			})
			assert.NoError(t, err)
			activeOrder, _ := c.exanteApi.GetActiveOrdersV3()
			assert.Len(t, activeOrder, 2)
			allOrders, _ := c.exanteApi.GetOrdersByLimitV3(100)
			assert.Len(t, allOrders, 3)
		}
		{ // cancel order
			_, err := c.Sync("acc-1", SyncRequest{
				ActivePositions: []Mt5Position{},
				RecentInactiveOrders: []Mt5Order{
					{Symbol: "EURUSD", Ticket: "1234", Volume: 1, Type: OrderTypeBuyLimit, StopLoss: 1, Price: 1.2, State: OrderStateCancelled},
				},
				ActiveOrders: []Mt5Order{},
			})
			assert.NoError(t, err)
			activeOrder, _ := c.exanteApi.GetActiveOrdersV3()
			assert.Len(t, activeOrder, 0)
			allOrders, _ := c.exanteApi.GetOrdersByLimitV3(100)
			assert.Len(t, allOrders, 3)
		}
	})

	t.Run("new order, change order's price", func(t *testing.T) {

		exanteMock := exante.NewMock(make([]exante.OrderV3, 0))
		c := New(exanteMock, exchange)

		{ // the program started with a recent order
			_, err := c.Sync("acc-1", SyncRequest{
				ActivePositions: []Mt5Position{},
				ActiveOrders: []Mt5Order{
					{Symbol: "EURUSD", Ticket: "1234", Volume: 1, Type: OrderTypeBuyLimit, Price: 1.2, State: OrderStatePlaced},
				},
			})
			assert.NoError(t, err)
			activeOrder, _ := c.exanteApi.GetActiveOrdersV3()
			assert.Len(t, activeOrder, 1)
			allOrders, _ := c.exanteApi.GetOrdersByLimitV3(100)
			assert.Len(t, allOrders, 1)
			assert.Equal(t, "1.2", activeOrder[0].OrderParameters.LimitPrice)
		}
		{ // change order price
			_, err := c.Sync("acc-1", SyncRequest{
				ActivePositions: []Mt5Position{},
				ActiveOrders: []Mt5Order{
					{Symbol: "EURUSD", Ticket: "1234", Volume: 1, Type: OrderTypeBuyLimit, Price: 1.1, State: OrderStatePlaced},
				},
			})
			assert.NoError(t, err)
			activeOrder, _ := c.exanteApi.GetActiveOrdersV3()
			assert.Len(t, activeOrder, 1)
			allOrders, _ := c.exanteApi.GetOrdersByLimitV3(100)
			assert.Len(t, allOrders, 1)
			assert.Equal(t, "1.1", activeOrder[0].OrderParameters.LimitPrice)
		}
	})

	t.Run("new order and become a position", func(t *testing.T) {

		exanteMock := exante.NewMock(make([]exante.OrderV3, 0))
		c := New(exanteMock, exchange)

		{ // the program started with a recent order
			_, err := c.Sync("acc-1", SyncRequest{
				ActivePositions: []Mt5Position{},
				ActiveOrders: []Mt5Order{
					{Symbol: "EURUSD", Ticket: "1234", Volume: 1, Type: OrderTypeBuyLimit, TakeProfit: 2, Price: 1.2, State: OrderStatePlaced},
				},
			})
			assert.NoError(t, err)
			activeOrder, _ := c.exanteApi.GetActiveOrdersV3()
			assert.Len(t, activeOrder, 2)
			allOrders, _ := c.exanteApi.GetOrdersByLimitV3(100)
			assert.Len(t, allOrders, 2)
		}
		{ // order become a position
			_, err := c.Sync("acc-1", SyncRequest{
				ActivePositions: []Mt5Position{
					{Symbol: "EURUSD", Ticket: "1234", Volume: 1, TakeProfit: 2, Price: 1.2},
				},
				RecentInactiveOrders: []Mt5Order{
					{Symbol: "EURUSD", Ticket: "1234", Volume: 1, Type: OrderTypeBuyLimit, TakeProfit: 2, Price: 1.2, State: OrderStateFilled},
				},
			})
			assert.NoError(t, err)
			activeOrder, _ := c.exanteApi.GetActiveOrdersV3()
			assert.Len(t, activeOrder, 2) // order will be executed soon
			assert.Equal(t, 1, exanteMock.TotalPlaceOrderV3)
		}

	})

	t.Run("has already a recent position on MT5 and a position on EXANTE", func(t *testing.T) {
		exanteMock := exante.NewMock([]exante.OrderV3{
			{
				OrderState: exante.OrderState{
					Status: exante.FilledStatus,
				},
				OrderID:   uuid.NewString(),
				ClientTag: "1234",
			},
		})
		c := New(exanteMock, exchange)
		{ // should not add another order on exante
			_, err := c.Sync("acc-1", SyncRequest{
				ActivePositions: []Mt5Position{
					{Symbol: "EURUSD", Ticket: "1234", Volume: 1, StopLoss: 1, Price: 1.2},
				},
				RecentInactiveOrders: []Mt5Order{
					{Symbol: "EURUSD", Ticket: "1234", Volume: 1, Type: OrderTypeBuy, StopLoss: 1, Price: 1.2, State: OrderStateFilled},
				},
			})
			assert.NoError(t, err)
			activeOrder, _ := c.exanteApi.GetActiveOrdersV3()
			assert.Len(t, activeOrder, 0)
			allOrders, _ := c.exanteApi.GetOrdersByLimitV3(100)
			assert.Len(t, allOrders, 1)
		}
	})

	t.Run("closed position by stop", func(t *testing.T) {
		parentOrderId := uuid.NewString()
		exanteMock := exante.NewMock([]exante.OrderV3{
			{
				OrderState: exante.OrderState{
					Status: exante.FilledStatus,
				},
				OrderID:   parentOrderId,
				ClientTag: "1234",
			},
			{
				OrderState: exante.OrderState{
					Status: exante.WorkingStatus,
				},
				OrderParameters: exante.OrderParameters{
					OcoGroup:       "oco-1",
					OrderType:      "stop",
					IfDoneParentID: parentOrderId,
				},
				OrderID:   uuid.NewString(),
				ClientTag: "1234",
			},
		})
		c := New(exanteMock, exchange)
		{
			_, err := c.Sync("acc-1", SyncRequest{
				ActivePositions: []Mt5Position{
					{Symbol: "EURUSD", Ticket: "1234", PositionTicket: "1234", Volume: 1, StopLoss: 1, Price: 1.2},
				},
			})
			assert.NoError(t, err)
			activeOrder, _ := c.exanteApi.GetActiveOrdersV3()
			assert.Len(t, activeOrder, 1)
			allOrders, _ := c.exanteApi.GetOrdersByLimitV3(100)
			assert.Len(t, allOrders, 2)
			assert.Equal(t, 0, exanteMock.TotalPlaceOrderV3)
		}
		{ // closed order by stop
			_, err := c.Sync("acc-1", SyncRequest{
				ActivePositions: []Mt5Position{},
				RecentInactiveOrders: []Mt5Order{
					{Symbol: "EURUSD", Ticket: "1235", Volume: 1, Type: OrderTypeSell, Price: 1, State: OrderStateFilled},
				},
				RecentInactivePositions: []Mt5PositionHistory{
					{Entry: DealEntryOut, Reason: DealReasonSL, Symbol: "EURUSD", Ticket: "1235", PositionTicket: "1234", Volume: 1, StopLoss: 1, Price: 1.2},
				},
			})
			assert.NoError(t, err)
			activeOrder, _ := c.exanteApi.GetActiveOrdersV3()
			assert.Len(t, activeOrder, 1)
			allOrders, _ := c.exanteApi.GetOrdersByLimitV3(100)
			assert.Len(t, allOrders, 2)
			assert.Equal(t, 0, exanteMock.TotalPlaceOrderV3)
		}
	})

	t.Run("has a open position, exante fills SL order but MT5 don't", func(t *testing.T) {
		parentOrderId := uuid.NewString()
		exanteOrders := []exante.OrderV3{
			{
				OrderState: exante.OrderState{
					Status: exante.FilledStatus,
				},
				OrderID:   parentOrderId,
				ClientTag: "1234",
			},
			{
				OrderState: exante.OrderState{
					Status: exante.FilledStatus,
				},
				OrderParameters: exante.OrderParameters{
					OcoGroup:       "oco-1",
					OrderType:      "stop",
					IfDoneParentID: parentOrderId,
				},
				OrderID:   uuid.NewString(),
				ClientTag: "1234",
			},
		}
		exanteMock := exante.NewMock(exanteOrders)
		c := New(exanteMock, exchange)

		{ // the status is filled on EXANTE but remains the same in MT5, shouldnt do anything

			_, err := c.Sync("acc-1", SyncRequest{
				ActivePositions: []Mt5Position{
					{Symbol: "EURUSD", Ticket: "1234", PositionTicket: "1234", Volume: 1, StopLoss: 1, Price: 1.2},
				},
			})
			assert.NoError(t, err)
			activeOrder, _ := c.exanteApi.GetActiveOrdersV3()
			assert.Len(t, activeOrder, 0)
			allOrders, _ := c.exanteApi.GetOrdersByLimitV3(100)
			assert.Len(t, allOrders, 2)
			assert.Equal(t, 0, exanteMock.TotalPlaceOrderV3)
		}
	})

	t.Run("has already a recent position on MT5 and a position on EXANTE with open stop order", func(t *testing.T) {
		parentOrderId := uuid.NewString()
		exanteMock := exante.NewMock([]exante.OrderV3{
			{
				OrderState: exante.OrderState{
					Status: exante.FilledStatus,
				},
				OrderID:   parentOrderId,
				ClientTag: "1234",
			},
			{
				OrderState: exante.OrderState{
					Status: exante.WorkingStatus,
				},
				OrderParameters: exante.OrderParameters{
					IfDoneParentID: parentOrderId,
				},
				OrderID:   uuid.NewString(),
				ClientTag: "1234",
			},
		})
		c := New(exanteMock, exchange)
		{ // should not add another order on exante
			_, err := c.Sync("acc-1", SyncRequest{
				ActivePositions: []Mt5Position{
					{Symbol: "EURUSD", Ticket: "1234", Volume: 1, StopLoss: 1, Price: 1.2},
				},
				RecentInactiveOrders: []Mt5Order{
					{Symbol: "EURUSD", Ticket: "1234", Volume: 1, Type: OrderTypeBuy, StopLoss: 1, Price: 1.2, State: OrderStateFilled},
				},
			})
			assert.NoError(t, err)
			activeOrder, _ := c.exanteApi.GetActiveOrdersV3()
			assert.Len(t, activeOrder, 1)
			allOrders, _ := c.exanteApi.GetOrdersByLimitV3(100)
			assert.Len(t, allOrders, 2)
		}
	})

	t.Run("has order on Exante, and CLOSE the deal on MT5, should cancel order and close position", func(t *testing.T) {
		parentOrderId := uuid.NewString()
		exanteMock := exante.NewMock([]exante.OrderV3{
			{
				OrderState: exante.OrderState{
					Status: exante.FilledStatus,
				},
				OrderParameters: exante.OrderParameters{
					Side: "buy",
				},
				OrderID:   parentOrderId,
				ClientTag: "1234",
			},
			{
				OrderState: exante.OrderState{
					Status: exante.WorkingStatus,
				},
				OrderParameters: exante.OrderParameters{
					IfDoneParentID: parentOrderId,
					Side:           "sell",
					OrderType:      "stop",
					OcoGroup:       uuid.NewString(),
				},
				OrderID:   uuid.NewString(),
				ClientTag: "1234",
			},
		})
		c := New(exanteMock, exchange)
		{ // should not add another order on exante
			_, err := c.Sync("acc-1", SyncRequest{
				ActivePositions: []Mt5Position{},
				RecentInactiveOrders: []Mt5Order{
					{Symbol: "EURUSD", Ticket: "1235", Volume: 1, Type: OrderTypeSell, StopLoss: 1, Price: 1.2, State: OrderStateFilled},
				},
				RecentInactivePositions: []Mt5PositionHistory{
					{Symbol: "EURUSD", Ticket: "1235", PositionTicket: "1234", Volume: 1, StopLoss: 1, Price: 1.2, Entry: DealEntryOut},
				},
			})
			assert.NoError(t, err)
			activeOrder, _ := c.exanteApi.GetActiveOrdersV3()
			assert.Len(t, activeOrder, 0)
			allOrders, _ := c.exanteApi.GetOrdersByLimitV3(100)
			assert.Len(t, allOrders, 3)
			assert.True(t, utils.IsPositionClosed(allOrders))
		}
	})

	t.Run("has an open position without previews orders, close the position", func(t *testing.T) {
		parentOrderId := uuid.NewString()
		exanteMock := exante.NewMock([]exante.OrderV3{
			{
				OrderState: exante.OrderState{
					Status: exante.FilledStatus,
				},
				OrderParameters: exante.OrderParameters{
					Side: "buy",
				},
				OrderID:   parentOrderId,
				ClientTag: "",
			},
		})
		c := New(exanteMock, exchange)
		{ // should not add another order on exante
			_, err := c.Sync("acc-1", SyncRequest{
				ActivePositions: []Mt5Position{},
				RecentInactiveOrders: []Mt5Order{
					{Symbol: "EURUSD", Ticket: "1234", Volume: 1, Type: OrderTypeSell, StopLoss: 1, Price: 1.2, State: OrderStateFilled},
				},
			})
			assert.NoError(t, err)
			activeOrder, _ := c.exanteApi.GetActiveOrdersV3()
			assert.Len(t, activeOrder, 0)
			assert.Equal(t, exanteMock.TotalPlaceOrderV3, 0)
		}
	})

	t.Run("open a position and closes soon", func(t *testing.T) {
		exanteMock := exante.NewMock([]exante.OrderV3{})
		c := New(exanteMock, exchange)
		{ // should open a new position
			_, err := c.Sync("acc-1", SyncRequest{
				ActivePositions: []Mt5Position{
					{Symbol: "EURUSD", Ticket: "1234", PositionTicket: "1234", Volume: 1, StopLoss: 1, Price: 1.2},
				},
				RecentInactiveOrders: []Mt5Order{
					{Symbol: "EURUSD", Ticket: "1234", Volume: 1, Type: OrderTypeBuy, Price: 1.2, StopLoss: 1, State: OrderStateFilled},
				},
				RecentInactivePositions: []Mt5PositionHistory{
					{Symbol: "EURUSD", Ticket: "1234", PositionTicket: "1234", Volume: 1, StopLoss: 1, Price: 1.2, Entry: DealEntryIn},
				},
			})
			assert.NoError(t, err)
			activeOrder, _ := c.exanteApi.GetActiveOrdersV3()
			assert.Len(t, activeOrder, 1)
			assert.Equal(t, 1, exanteMock.TotalPlaceOrderV3)
		}
		{ // closing the position
			_, err := c.Sync("acc-1", SyncRequest{
				ActivePositions: []Mt5Position{},
				RecentInactiveOrders: []Mt5Order{
					{Symbol: "EURUSD", Ticket: "1234", Volume: 1, Type: OrderTypeBuy, Price: 1.2, State: OrderStateFilled},
					{Symbol: "EURUSD", Ticket: "1235", Volume: 1, Type: OrderTypeSell, Price: 1.2, State: OrderStateFilled},
				},
				RecentInactivePositions: []Mt5PositionHistory{
					{Symbol: "EURUSD", Ticket: "1234", PositionTicket: "1234", Volume: 1, StopLoss: 1, Price: 1.2, Entry: DealEntryIn},
					{Symbol: "EURUSD", Ticket: "1235", PositionTicket: "1234", Volume: 1, StopLoss: 1, Price: 1.2, Entry: DealEntryOut},
				},
			})
			assert.NoError(t, err)
			activeOrder, _ := c.exanteApi.GetActiveOrdersV3()
			assert.Len(t, activeOrder, 0)
			assert.Equal(t, 2, exanteMock.TotalPlaceOrderV3)
		}
		{ // should not change anything
			_, err := c.Sync("acc-1", SyncRequest{
				ActivePositions: []Mt5Position{},
				RecentInactiveOrders: []Mt5Order{
					{Symbol: "EURUSD", Ticket: "1234", Volume: 1, Type: OrderTypeBuy, Price: 1.2, State: OrderStateFilled},
					{Symbol: "EURUSD", Ticket: "1235", Volume: 1, Type: OrderTypeSell, Price: 1.2, State: OrderStateFilled},
				},
				RecentInactivePositions: []Mt5PositionHistory{
					{Symbol: "EURUSD", Ticket: "1234", PositionTicket: "1234", Volume: 1, StopLoss: 1, Price: 1.2, Entry: DealEntryIn},
					{Symbol: "EURUSD", Ticket: "1235", PositionTicket: "1234", Volume: 1, StopLoss: 1, Price: 1.2, Entry: DealEntryOut},
				},
			})
			assert.NoError(t, err)
			activeOrder, _ := c.exanteApi.GetActiveOrdersV3()
			assert.Len(t, activeOrder, 0)
			assert.Equal(t, 2, exanteMock.TotalPlaceOrderV3)
		}
	})

	t.Run("open a position with SL and add TP later", func(t *testing.T) {
		exanteMock := exante.NewMock([]exante.OrderV3{})
		c := New(exanteMock, exchange)
		{ // should open a new position
			_, err := c.Sync("acc-1", SyncRequest{
				ActivePositions: []Mt5Position{
					{Symbol: "EURUSD", Ticket: "1234", PositionTicket: "1234", Volume: 1, StopLoss: 1, Price: 1.2},
				},
				RecentInactiveOrders: []Mt5Order{
					{Symbol: "EURUSD", Ticket: "1234", Volume: 1, Type: OrderTypeBuy, Price: 1.2, StopLoss: 1, State: OrderStateFilled},
				},
				RecentInactivePositions: []Mt5PositionHistory{
					{Symbol: "EURUSD", Ticket: "1234", PositionTicket: "1234", Volume: 1, StopLoss: 1, Price: 1.2, Entry: DealEntryIn},
				},
			})
			assert.NoError(t, err)
			activeOrder, _ := c.exanteApi.GetActiveOrdersV3()
			assert.Len(t, activeOrder, 1)
			assert.Equal(t, 1, exanteMock.TotalPlaceOrderV3)
		}
		{ // add TP
			_, err := c.Sync("acc-1", SyncRequest{
				ActivePositions: []Mt5Position{
					{Symbol: "EURUSD", Ticket: "1234", PositionTicket: "1234", Volume: 1, StopLoss: 1, TakeProfit: 2, Price: 1.2},
				},
				RecentInactiveOrders: []Mt5Order{
					{Symbol: "EURUSD", Ticket: "1234", Volume: 1, Type: OrderTypeBuy, Price: 1.2, StopLoss: 1, State: OrderStateFilled},
				},
				RecentInactivePositions: []Mt5PositionHistory{
					{Symbol: "EURUSD", Ticket: "1234", PositionTicket: "1234", Volume: 1, StopLoss: 1, Price: 1.2, Entry: DealEntryIn},
				},
			})
			assert.NoError(t, err)
			activeOrder, _ := c.exanteApi.GetActiveOrdersV3()
			assert.Len(t, activeOrder, 2)
			assert.Equal(t, 2, exanteMock.TotalPlaceOrderV3)
		}
		{ // check if no changes applied
			_, err := c.Sync("acc-1", SyncRequest{
				ActivePositions: []Mt5Position{
					{Symbol: "EURUSD", Ticket: "1234", PositionTicket: "1234", Volume: 1, StopLoss: 1, TakeProfit: 2, Price: 1.2},
				},
				RecentInactiveOrders: []Mt5Order{
					{Symbol: "EURUSD", Ticket: "1234", Volume: 1, Type: OrderTypeBuy, Price: 1.2, StopLoss: 1, State: OrderStateFilled},
				},
				RecentInactivePositions: []Mt5PositionHistory{
					{Symbol: "EURUSD", Ticket: "1234", PositionTicket: "1234", Volume: 1, StopLoss: 1, Price: 1.2, Entry: DealEntryIn},
				},
			})
			assert.NoError(t, err)
			activeOrder, _ := c.exanteApi.GetActiveOrdersV3()
			assert.Len(t, activeOrder, 2)
			assert.Equal(t, 2, exanteMock.TotalPlaceOrderV3)
		}

	})

	t.Run("has open position without STOP, add TP, should not duplicate requests", func(t *testing.T) {
		exanteMock := exante.NewMock([]exante.OrderV3{
			{
				OrderState: exante.OrderState{
					Status: exante.FilledStatus,
				},
				OrderID:   uuid.NewString(),
				ClientTag: "1234",
			},
		})
		c := New(exanteMock, exchange)
		{ // should only change take profit
			_, err := c.Sync("acc-1", SyncRequest{
				ActivePositions: []Mt5Position{
					{Symbol: "EURUSD", Ticket: "1234", PositionTicket: "1234", Volume: 1, TakeProfit: 2},
				},
				RecentInactiveOrders: []Mt5Order{
					{Symbol: "EURUSD", Ticket: "1234", Volume: 1, Type: OrderTypeBuy, Price: 1.2, State: OrderStateFilled},
				},
			})
			assert.NoError(t, err)
			activeOrder, _ := c.exanteApi.GetActiveOrdersV3()
			assert.Len(t, activeOrder, 1)
			assert.Equal(t, 1, exanteMock.TotalPlaceOrderV3)
		}

	})
}
