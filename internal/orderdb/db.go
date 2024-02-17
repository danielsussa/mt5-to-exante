package orderdb

import (
	"encoding/json"
	"fmt"
	"github.com/google/uuid"
	"github.com/peterbourgon/diskv/v3"
)

type OrderState struct {
	d        *diskv.Diskv
	orderMap map[string]OrderGroup
}

func (os *OrderState) List() []OrderGroup {
	l := make([]OrderGroup, 0)
	for _, val := range os.orderMap {
		l = append(l, val)
	}

	return l
}

type OrderGroup struct {
	Ticket     string
	Order      OrderDB
	OcoGroup   string
	StopLoss   *OrderDB
	TakeProfit *OrderDB
}

type OrderDB struct {
	ID         string
	Price      string
	StopPrice  string
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

func NewOrderGroupWithTicket(ticket string) OrderGroup {
	return OrderGroup{
		Ticket:   ticket,
		OcoGroup: uuid.NewString(),
	}
}

func NewNoDisk() *OrderState {
	return &OrderState{orderMap: map[string]OrderGroup{}}
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

	has := d.Has("root")
	if has {
		rootB, err := d.Read("root")
		if err != nil {
			return nil, err
		}

		var orderMap map[string]OrderGroup
		err = json.Unmarshal(rootB, &orderMap)
		if err != nil {
			return nil, err
		}

		return &OrderState{d: d, orderMap: orderMap}, err
	}

	return &OrderState{d: d, orderMap: make(map[string]OrderGroup)}, nil
}

func (os *OrderState) Upsert(ticketID string, order OrderGroup) {
	os.orderMap[ticketID] = order
	//b, _ := json.Marshal(order)
	//_ = os.d.Write(ticketID, b)
}

func (os *OrderState) Delete(ticketID string) {
	delete(os.orderMap, ticketID)
	//_ = os.d.Erase(ticketID)
}

func (os *OrderState) Get(ticketID string) (OrderGroup, bool) {
	for ticket, val := range os.orderMap {
		if ticket == ticketID {
			return val, true
		}
	}

	return OrderGroup{}, false
}

func (os *OrderState) Flush() error {
	b, _ := json.Marshal(os.orderMap)
	return os.d.Write("root", b)
}
