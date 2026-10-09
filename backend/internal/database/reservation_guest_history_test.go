package database

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type guestHistorySQLRecorder struct {
	mu   sync.Mutex
	sqls []string
}

func (l *guestHistorySQLRecorder) LogMode(logger.LogLevel) logger.Interface { return l }
func (l *guestHistorySQLRecorder) Info(context.Context, string, ...interface{}) {
}
func (l *guestHistorySQLRecorder) Warn(context.Context, string, ...interface{}) {
}
func (l *guestHistorySQLRecorder) Error(context.Context, string, ...interface{}) {
}
func (l *guestHistorySQLRecorder) Trace(_ context.Context, _ time.Time, fc func() (string, int64), _ error) {
	sql, _ := fc()
	l.mu.Lock()
	defer l.mu.Unlock()
	l.sqls = append(l.sqls, sql)
}
func (l *guestHistorySQLRecorder) Reset() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.sqls = nil
}
func (l *guestHistorySQLRecorder) SQLs() []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	out := make([]string, len(l.sqls))
	copy(out, l.sqls)
	return out
}

func countSelects(sqls []string) int {
	n := 0
	for _, q := range sqls {
		if strings.HasPrefix(strings.ToUpper(strings.TrimSpace(q)), "SELECT") {
			n++
		}
	}
	return n
}

func setupGuestHistoryTestDB(t testing.TB, gormLogger logger.Interface) {
	t.Helper()
	dsn := fmt.Sprintf("file:guest-hist-%s-%d?mode=memory&cache=shared",
		strings.NewReplacer("/", "_", " ", "_").Replace(t.Name()), time.Now().UnixNano())
	cfg := &gorm.Config{}
	if gormLogger != nil {
		cfg.Logger = gormLogger
	}
	gormDB, err := gorm.Open(sqlite.Open(dsn), cfg)
	require.NoError(t, err)
	sqlDB, err := gormDB.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = sqlDB.Close() })

	prev := db
	SetTestDB(gormDB)
	t.Cleanup(func() { SetTestDB(prev) })

	require.NoError(t, db.AutoMigrate(&Business{}, &TableReservation{}))
}

func TestGetReservationGuestHistory_SingleGroupedQuery(t *testing.T) {
	recorder := &guestHistorySQLRecorder{}
	setupGuestHistoryTestDB(t, recorder)

	bizA := &Business{
		BusinessId:     fmt.Sprintf("guest-hist-a-%d", time.Now().UnixNano()),
		Name:           "A",
		OwnerAddress:   "0xA",
		SettlementAddr: "0x1111111111111111111111111111111111111111",
		TippingAddr:    "0x2222222222222222222222222222222222222222",
	}
	bizB := &Business{
		BusinessId:     fmt.Sprintf("guest-hist-b-%d", time.Now().UnixNano()),
		Name:           "B",
		OwnerAddress:   "0xB",
		SettlementAddr: "0x1111111111111111111111111111111111111111",
		TippingAddr:    "0x2222222222222222222222222222222222222222",
	}
	require.NoError(t, db.Create(bizA).Error)
	require.NoError(t, db.Create(bizB).Error)

	now := time.Now().UTC()
	mk := func(businessID uint, email, status string) {
		require.NoError(t, db.Create(&TableReservation{
			BusinessID:      businessID,
			CustomerName:    "Guest",
			CustomerEmail:   email,
			PartySize:       2,
			ReservationTime: now.Add(-24 * time.Hour),
			Status:          status,
			Language:        "en",
		}).Error)
	}

	// business A: ana@x.io — 2 no_show, 3 completed, 1 seated, 1 pending
	mk(bizA.ID, "Ana@X.io", "no_show")
	mk(bizA.ID, "ana@x.io", "no_show")
	mk(bizA.ID, "ana@x.io", "completed")
	mk(bizA.ID, "ana@x.io", "completed")
	mk(bizA.ID, "ana@x.io", "completed")
	mk(bizA.ID, "ana@x.io", "seated")
	mk(bizA.ID, "ana@x.io", "pending")
	// same email under business B must not leak
	mk(bizB.ID, "ana@x.io", "no_show")
	// bob@x.io — one completed
	mk(bizA.ID, "bob@x.io", "completed")

	recorder.Reset()
	history, err := GetReservationGuestHistory(bizA.ID, []string{"Ana@X.io", "bob@x.io"})
	require.NoError(t, err)

	selects := countSelects(recorder.SQLs())
	require.Equal(t, 1, selects, "must be a single grouped query, got: %v", recorder.SQLs())

	ana := history["ana@x.io"]
	require.EqualValues(t, 2, ana.PriorNoShows)
	require.EqualValues(t, 4, ana.PriorVisits) // 3 completed + 1 seated; pending not counted
	bob := history["bob@x.io"]
	require.EqualValues(t, 0, bob.PriorNoShows)
	require.EqualValues(t, 1, bob.PriorVisits)
}

func BenchmarkGetReservationGuestHistory(b *testing.B) {
	setupGuestHistoryTestDB(b, nil)

	biz := &Business{
		BusinessId:     fmt.Sprintf("guest-hist-bench-%d", time.Now().UnixNano()),
		Name:           "Bench",
		OwnerAddress:   "0xBench",
		SettlementAddr: "0x1111111111111111111111111111111111111111",
		TippingAddr:    "0x2222222222222222222222222222222222222222",
	}
	require.NoError(b, db.Create(biz).Error)

	now := time.Now().UTC()
	rows := make([]TableReservation, 0, 5000)
	for i := 0; i < 5000; i++ {
		email := fmt.Sprintf("guest%d@bench.io", i%50)
		status := "completed"
		if i%7 == 0 {
			status = "no_show"
		} else if i%11 == 0 {
			status = "seated"
		}
		rows = append(rows, TableReservation{
			BusinessID:      biz.ID,
			CustomerName:    "Guest",
			CustomerEmail:   email,
			PartySize:       2,
			ReservationTime: now.Add(-time.Duration(i) * time.Hour),
			Status:          status,
			Language:        "en",
		})
	}
	require.NoError(b, db.CreateInBatches(rows, 200).Error)

	queryEmails := make([]string, 0, 20)
	for i := 0; i < 20; i++ {
		queryEmails = append(queryEmails, fmt.Sprintf("guest%d@bench.io", i))
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := GetReservationGuestHistory(biz.ID, queryEmails); err != nil {
			b.Fatal(err)
		}
	}
}

func TestGetReservationGuestHistory_EmptyEmails(t *testing.T) {
	setupGuestHistoryTestDB(t, nil)
	history, err := GetReservationGuestHistory(1, []string{"", "  "})
	require.NoError(t, err)
	assert.Empty(t, history)
}
