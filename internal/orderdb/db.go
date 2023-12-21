package orderdb

import (
	"encoding/json"
	"fmt"
	"github.com/google/uuid"
	"github.com/peterbourgon/diskv/v3"
)

type OrderState struct {
	d *diskv.Diskv
}

type OrderGroup struct {
	Order      OrderDB
	OcoGroup   string
	StopLoss   *OrderDB
	TakeProfit *OrderDB
}

type OrderDB struct {
	ID         string
	Quantity   string
	Side       string
	Duration   string
	AccountId  string
	Symbol     string
	Instrument string
}

func NewOrderGroup() OrderGroup {
	return OrderGroup{
		OcoGroup: uuid.NewString(),
	}
}

func New(path string) (*OrderState, error) {
	d := diskv.New(diskv.Options{
		BasePath:     fmt.Sprintf("%s/.db", path),
		Transform:    func(s string) []string { return []string{} },
		CacheSizeMax: 1024 * 1024,
	})

	err := d.Write("test", []byte{})
	if err != nil {
		return nil, err
	}

	has := d.Has("test")
	if !has {
		return nil, fmt.Errorf("error creatind .db")
	}

	return &OrderState{d: d}, nil
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
