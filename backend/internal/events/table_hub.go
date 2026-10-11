package events

import (
	"errors"
	"sync"

	"github.com/stdevmac/payverge/backend/internal/database"

	"gorm.io/gorm"
)

var ErrTableConnectionLimit = errors.New("table event connection limit reached")

// TableBillChanged is intentionally content-free: public table streams carry
// only lifecycle state, never bill, customer, item, or payment details.
type TableBillChanged struct {
	HasActiveBill bool `json:"has_active_bill"`
}

type TableHub struct {
	mu             sync.RWMutex
	subscribers    map[uint]map[chan TableBillChanged]struct{}
	businessConns  map[uint]int
	ipConns        map[string]int
	total          int
	maxPerTable    int
	maxPerBusiness int
	maxPerIP       int
	maxTotal       int
}

func NewTableHub(maxPerTable, maxPerBusiness, maxPerIP, maxTotal int) *TableHub {
	return &TableHub{
		subscribers:    make(map[uint]map[chan TableBillChanged]struct{}),
		businessConns:  make(map[uint]int),
		ipConns:        make(map[string]int),
		maxPerTable:    maxPerTable,
		maxPerBusiness: maxPerBusiness,
		maxPerIP:       maxPerIP,
		maxTotal:       maxTotal,
	}
}

func (h *TableHub) Subscribe(tableID, businessID uint, clientIP string) (<-chan TableBillChanged, func(), error) {
	if h == nil || tableID == 0 {
		return nil, nil, ErrTableConnectionLimit
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	group := h.subscribers[tableID]
	if h.maxPerTable > 0 && len(group) >= h.maxPerTable {
		return nil, nil, ErrTableConnectionLimit
	}
	if h.maxTotal > 0 && h.total >= h.maxTotal {
		return nil, nil, ErrTableConnectionLimit
	}
	if h.maxPerBusiness > 0 && h.businessConns[businessID] >= h.maxPerBusiness {
		return nil, nil, ErrTableConnectionLimit
	}
	if clientIP != "" && h.maxPerIP > 0 && h.ipConns[clientIP] >= h.maxPerIP {
		return nil, nil, ErrTableConnectionLimit
	}
	if group == nil {
		group = make(map[chan TableBillChanged]struct{})
		h.subscribers[tableID] = group
	}
	if h.businessConns == nil {
		h.businessConns = make(map[uint]int)
	}
	if h.ipConns == nil {
		h.ipConns = make(map[string]int)
	}
	ch := make(chan TableBillChanged, 4)
	group[ch] = struct{}{}
	h.total++
	h.businessConns[businessID]++
	if clientIP != "" {
		h.ipConns[clientIP]++
	}
	var once sync.Once
	cancel := func() {
		once.Do(func() {
			h.mu.Lock()
			defer h.mu.Unlock()
			if members := h.subscribers[tableID]; members != nil {
				if _, ok := members[ch]; ok {
					delete(members, ch)
					h.total--
					if h.businessConns[businessID] > 0 {
						h.businessConns[businessID]--
					}
					if h.businessConns[businessID] == 0 {
						delete(h.businessConns, businessID)
					}
					if clientIP != "" {
						if h.ipConns[clientIP] > 0 {
							h.ipConns[clientIP]--
						}
						if h.ipConns[clientIP] == 0 {
							delete(h.ipConns, clientIP)
						}
					}
					close(ch)
				}
				if len(members) == 0 {
					delete(h.subscribers, tableID)
				}
			}
		})
	}
	return ch, cancel, nil
}

func (h *TableHub) Signal(tableID uint, event TableBillChanged) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	for ch := range h.subscribers[tableID] {
		select {
		case ch <- event:
		default:
		}
	}
}

// Public table-stream caps. Every seated guest's phone holds one stream, and a
// venue's guest Wi-Fi puts a whole dining room behind one NAT address, so the
// per-IP cap must cover a full room at dinner rush; the per-business cap is
// what bounds a single abusive client.
const (
	publicTableMaxPerTable    = 50
	publicTableMaxPerBusiness = 200
	publicTableMaxPerIP       = 60
	publicTableMaxTotal       = 1000
)

var publicTableHub = NewTableHub(publicTableMaxPerTable, publicTableMaxPerBusiness, publicTableMaxPerIP, publicTableMaxTotal)

func GetTableHub() *TableHub { return publicTableHub }

// SetTableHubForTest installs h as the process-wide public table hub and
// returns a function that restores the previous hub.
func SetTableHubForTest(h *TableHub) func() {
	previous := publicTableHub
	publicTableHub = h
	return func() { publicTableHub = previous }
}

// NotifyTableBillChanged publishes lifecycle state from a bill that the caller
// has already loaded from the database. This avoids reloading the same bill on
// hot payment paths while still deriving public table scope from the trusted
// domain object rather than accepting a caller-supplied table ID.
func NotifyTableBillChanged(bill *database.Bill) {
	if bill == nil || bill.ID == 0 || bill.TableID == 0 {
		return
	}
	active := bill.Status == database.BillStatusOpen || bill.Status == database.BillStatusPartial
	GetTableHub().Signal(bill.TableID, TableBillChanged{HasActiveBill: active})
}

// NotifyTableBillChangedByBillID derives table scope from the persisted bill;
// callers cannot inject a table identity into the public stream.
func NotifyTableBillChangedByBillID(db *gorm.DB, billID uint) error {
	if db == nil || billID == 0 {
		return gorm.ErrInvalidDB
	}
	var bill database.Bill
	if err := db.Select("id", "table_id", "status").First(&bill, billID).Error; err != nil {
		return err
	}
	NotifyTableBillChanged(&bill)
	return nil
}
