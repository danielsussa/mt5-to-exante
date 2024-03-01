package controller

import (
	"github.com/danielsussa/mt5-to-exante/internal/exante"
	"github.com/danielsussa/mt5-to-exante/internal/exchanges"
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
			assert.Len(t, activeOrder, 0)
			allOrders, _ := c.exanteApi.GetOrdersByLimitV3(100, "acc-1")
			assert.Len(t, allOrders, 0)
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
			assert.Len(t, activeOrder, 1) // order will be executed soon
			assert.Equal(t, 1, exanteMock.TotalPlaceOrderV3)
		}
	})

}
