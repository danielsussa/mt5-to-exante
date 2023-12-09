package orderdb

import (
	"encoding/json"
	"github.com/peterbourgon/diskv/v3"
)

type OrderState struct {
	d *diskv.Diskv
}

type OrderGroup struct {
	Order      OrderDB
	StopLoss   *OrderDB
	TakeProfit *OrderDB
}

type OrderDB struct {
	ID         string
	Quantity   string
	OcoGroup   string
	Side       string
	Duration   string
	AccountId  string
	Symbol     string
	Instrument string
}

func New() *OrderState {
	d := diskv.New(diskv.Options{
		BasePath:     ".db",
		Transform:    func(s string) []string { return []string{} },
		CacheSizeMax: 1024 * 1024,
	})

	return &OrderState{d: d}
}

func (os *OrderState) Upsert(ticketID string, order OrderGroup) {
	b, _ := json.Marshal(order)
	_ = os.d.Write(ticketID, b)
}

func (os *OrderState) Delete(ticketID string) {
	_ = os.d.Erase(ticketID)
}

func (os *OrderState) Get(ticketID string) (OrderGroup, bool) {
	if !os.d.Has(ticketID) {
		return OrderGroup{}, false
	}
	b, err := os.d.Read(ticketID)
	if err != nil {
		panic(err)
	}
	var order OrderGroup
	_ = json.Unmarshal(b, &order)

	return order, true
}