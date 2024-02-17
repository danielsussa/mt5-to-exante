package orderdb

type Iface interface {
	Upsert(ticketID string, order OrderGroup)
	Delete(ticketID string)
	Get(ticketID string) (OrderGroup, bool)
	List() []OrderGroup
	Flush() error
}
