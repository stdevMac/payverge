package events

import (
	"testing"
	"time"

	"github.com/stdevmac/payverge/backend/internal/database"

	"github.com/stretchr/testify/require"
)

func TestTableHubScopesSignalsToOneTable(t *testing.T) {
	hub := NewTableHub(2, 10, 10, 4)
	one, cancelOne, err := hub.Subscribe(1, 1, "")
	require.NoError(t, err)
	defer cancelOne()
	two, cancelTwo, err := hub.Subscribe(2, 2, "")
	require.NoError(t, err)
	defer cancelTwo()

	hub.Signal(1, TableBillChanged{HasActiveBill: false})
	require.Equal(t, TableBillChanged{HasActiveBill: false}, <-one)
	select {
	case <-two:
		t.Fatal("event leaked to another table")
	case <-time.After(20 * time.Millisecond):
	}
}

func TestTableHubEnforcesPerTableLimit(t *testing.T) {
	hub := NewTableHub(1, 10, 10, 2)
	_, cancel, err := hub.Subscribe(1, 1, "203.0.113.1")
	require.NoError(t, err)
	defer cancel()
	_, _, err = hub.Subscribe(1, 1, "203.0.113.2")
	require.ErrorIs(t, err, ErrTableConnectionLimit)
}

func TestTableHubEnforcesPerBusinessLimit(t *testing.T) {
	hub := NewTableHub(10, 1, 10, 100)
	_, cancel, err := hub.Subscribe(1, 7, "203.0.113.1")
	require.NoError(t, err)
	defer cancel()

	_, _, err = hub.Subscribe(2, 7, "203.0.113.2")
	require.ErrorIs(t, err, ErrTableConnectionLimit)

	_, cancelOther, err := hub.Subscribe(3, 8, "203.0.113.3")
	require.NoError(t, err)
	defer cancelOther()
}

func TestTableHubEnforcesPerIPLimit(t *testing.T) {
	hub := NewTableHub(10, 10, 1, 100)
	_, cancel, err := hub.Subscribe(1, 7, "203.0.113.9")
	require.NoError(t, err)
	defer cancel()

	_, _, err = hub.Subscribe(2, 8, "203.0.113.9")
	require.ErrorIs(t, err, ErrTableConnectionLimit)

	_, cancelOther, err := hub.Subscribe(3, 8, "203.0.113.10")
	require.NoError(t, err)
	defer cancelOther()
}

func TestTableHubEmptyClientIPSkipsPerIPCap(t *testing.T) {
	hub := NewTableHub(10, 10, 1, 100)
	_, cancelA, err := hub.Subscribe(1, 7, "")
	require.NoError(t, err)
	defer cancelA()
	_, cancelB, err := hub.Subscribe(2, 8, "")
	require.NoError(t, err)
	defer cancelB()
}

func TestTableHubCancelFreesBusinessAndIPSlots(t *testing.T) {
	hub := NewTableHub(10, 1, 1, 100)
	_, cancel, err := hub.Subscribe(1, 7, "203.0.113.9")
	require.NoError(t, err)

	_, _, err = hub.Subscribe(2, 7, "203.0.113.10")
	require.ErrorIs(t, err, ErrTableConnectionLimit)
	_, _, err = hub.Subscribe(3, 8, "203.0.113.9")
	require.ErrorIs(t, err, ErrTableConnectionLimit)

	cancel()

	_, cancelBusiness, err := hub.Subscribe(2, 7, "203.0.113.10")
	require.NoError(t, err)
	cancelBusiness()

	_, cancelIP, err := hub.Subscribe(3, 8, "203.0.113.9")
	require.NoError(t, err)
	defer cancelIP()
}

func TestNotifyTableBillChangedUsesLoadedBillState(t *testing.T) {
	previous := publicTableHub
	publicTableHub = NewTableHub(5, 5, 5, 5)
	t.Cleanup(func() { publicTableHub = previous })

	updates, cancel, err := publicTableHub.Subscribe(42, 1, "")
	require.NoError(t, err)
	defer cancel()

	NotifyTableBillChanged(&database.Bill{ID: 7, TableID: 42, Status: database.BillStatusPartial})
	require.Equal(t, TableBillChanged{HasActiveBill: true}, <-updates)

	NotifyTableBillChanged(&database.Bill{ID: 7, TableID: 42, Status: database.BillStatusPaid})
	require.Equal(t, TableBillChanged{HasActiveBill: false}, <-updates)
}

// TestPublicTableHubAdmitsADiningRoomBehindOneNAT pins that the production
// per-IP cap does not lock out guests who share a venue Wi-Fi address: 40
// phones across 20 tables on one IP must all get a live stream.
func TestPublicTableHubAdmitsADiningRoomBehindOneNAT(t *testing.T) {
	hub := NewTableHub(publicTableMaxPerTable, publicTableMaxPerBusiness, publicTableMaxPerIP, publicTableMaxTotal)
	for i := 0; i < 40; i++ {
		_, cancel, err := hub.Subscribe(uint(i%20)+1, 7, "198.51.100.20")
		require.NoError(t, err, "guest %d behind the venue NAT was refused", i)
		defer cancel()
	}
}
