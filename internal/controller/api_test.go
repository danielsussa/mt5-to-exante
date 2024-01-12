package controller

import (
	"github.com/danielsussa/mt5-to-exante/internal/exante"
	"github.com/danielsussa/mt5-to-exante/internal/exchanges"
	"github.com/danielsussa/mt5-to-exante/internal/orderdb"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"testing"
	"time"
)

func convertTypeToStatus(ot string) exante.Status {
	if ot == "market" {
		return exante.FilledStatus
	}
	return exante.WorkingStatus
}

func TestApi(t *testing.T) {
	t.Run("simple complete flow", func(t *testing.T) {
		apiMock := exante.ApiMock{
			CancelOrderFunc: nil,
			GetOrderFunc:    nil,
			ReplaceOrderFunc: func(orderID string, req exante.ReplaceOrderPayload) (*exante.OrderV3, error) {
				return &exante.OrderV3{
					OrderParameters: exante.OrderParameters{
						LimitPrice: req.Parameters.LimitPrice,
						StopPrice:  req.Parameters.StopPrice,
					},
					OrderID: orderID,
				}, nil
			},
			PlaceOrderV3Func: func(req *exante.OrderSentTypeV3) ([]exante.OrderV3, error) {

				orders := make([]exante.OrderV3, 0)
				orders = append(orders, exante.OrderV3{
					OrderState: exante.OrderState{
						Status: convertTypeToStatus(req.OrderType),
					},
					OrderParameters: exante.OrderParameters{
						Quantity:       req.Quantity,
						Instrument:     req.Instrument,
						OrderType:      req.OrderType,
						IfDoneParentID: req.IfDoneParentID,
						LimitPrice:     req.LimitPrice,
					},
					OrderID: uuid.NewString(),
				})

				ocoGroup := uuid.NewString()

				if req.TakeProfit != nil {
					orders = append(orders, exante.OrderV3{
						OrderState: exante.OrderState{
							Status: convertTypeToStatus(req.OrderType),
						},
						OrderParameters: exante.OrderParameters{
							OcoGroup:   ocoGroup,
							LimitPrice: *req.TakeProfit,
							OrderType:  "limit",
						},
						OrderID: uuid.NewString(),
					})
				}
				if req.StopLoss != nil {
					orders = append(orders, exante.OrderV3{
						OrderState: exante.OrderState{
							Status: convertTypeToStatus(req.OrderType),
						},
						OrderParameters: exante.OrderParameters{
							OcoGroup:   ocoGroup,
							OrderType:  "!limit",
							LimitPrice: *req.StopLoss,
							StopPrice:  *req.StopLoss,
						},
						OrderID: uuid.NewString(),
					})
				}

				return orders, nil
			},
		}

		db := orderdb.NewNoDisk()

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

		c := New(&apiMock, db, exchange)

		tNow := time.Now()
		{
			err := c.Sync(tNow, "acc-1", SyncRequest{Orders: []Mt5Order{
				{
					Symbol:    "EURUSD",
					Ticket:    "4321",
					Volume:    100,
					Type:      OrderTypeBuy,
					StopLoss:  9,
					Price:     10,
					State:     OrderStatePlaced,
					UpdatedAt: tNow,
				},
			}})
			assert.NoError(t, err)
		}
		{
			// call second time shouldn't call exante, only filled order
			err := c.Sync(tNow, "acc-1", SyncRequest{Positions: []Mt5Order{
				{
					Symbol:    "EURUSD",
					Ticket:    "4321",
					Volume:    100,
					Type:      OrderTypeBuy,
					StopLoss:  9,
					Price:     10,
					State:     OrderStateFilled,
					UpdatedAt: tNow,
				},
			}})
			assert.NoError(t, err)
		}
		{
			// change stop loss, should replace request
			err := c.Sync(tNow, "acc-1", SyncRequest{Positions: []Mt5Order{
				{
					Symbol:    "EURUSD",
					Ticket:    "4321",
					Volume:    100,
					Type:      OrderTypeBuy,
					StopLoss:  8,
					Price:     10,
					State:     OrderStateFilled,
					UpdatedAt: tNow,
				},
			}})
			assert.NoError(t, err)
		}
		{
			order, _ := db.Get("4321")
			assert.Equal(t, "8.00000", order.StopLoss.StopPrice)
		}
		{
			// change take profit, should call exante
			err := c.Sync(tNow, "acc-1", SyncRequest{Positions: []Mt5Order{
				{
					Symbol:     "EURUSD",
					Ticket:     "4321",
					Volume:     100,
					Type:       OrderTypeBuy,
					TakeProfit: 12,
					StopLoss:   8,
					Price:      10,
					State:      OrderStateFilled,
					UpdatedAt:  tNow,
				},
			}})
			assert.NoError(t, err)
		}
		{
			order, _ := db.Get("4321")
			assert.Equal(t, "8.00000", order.StopLoss.StopPrice)
			assert.Equal(t, "12.00000", order.TakeProfit.Price)
		}

		assert.Equal(t, 3, apiMock.TotalCalls())
	})
}
