package services

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestGuestCheckoutLoadsBusinessOnce(t *testing.T) {
	service, table := setupGuestCheckoutTest(t)

	var mu sync.Mutex
	var sqls []string
	const cb = "guest_checkout_loads_business_once"
	require.NoError(t, service.db.Callback().Query().After("gorm:query").Register(cb, func(tx *gorm.DB) {
		mu.Lock()
		sqls = append(sqls, tx.Statement.SQL.String())
		mu.Unlock()
	}))
	t.Cleanup(func() { _ = service.db.Callback().Query().Remove(cb) })

	_, err := service.Checkout(context.Background(), checkoutInput(table.TableCode, "access-shape-1"))
	require.NoError(t, err)

	mu.Lock()
	captured := append([]string(nil), sqls...)
	mu.Unlock()

	businessQueries := 0
	var locked []string
	for _, raw := range captured {
		norm := strings.ToLower(strings.NewReplacer("`", "", `"`, "", "\n", " ").Replace(raw))
		if strings.Contains(norm, "from businesses") {
			businessQueries++
		}
		// The in-transaction lock re-reads the table by primary key.
		if strings.HasPrefix(norm, "select id,business_id,table_code,name,is_active from tables") {
			locked = append(locked, norm)
		}
	}
	require.Equal(t, 1, businessQueries, "Checkout must load businesses once, got %v", captured)
	require.Len(t, locked, 1, "expected one narrow tables lock query without a business preload, got %v", captured)
}

func BenchmarkGuestCheckout(b *testing.B) {
	service, table := setupGuestCheckoutTest(b)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		res, err := service.Checkout(context.Background(), checkoutInput(table.TableCode, fmt.Sprintf("bench-%d", i)))
		if err != nil {
			b.Fatal(err)
		}
		// Close the bill so every iteration opens a fresh one; otherwise the
		// growing bill makes per-op cost depend on b.N.
		b.StopTimer()
		if err := service.db.Model(&database.Bill{}).Where("id = ?", res.Bill.ID).
			Update("status", database.BillStatusClosed).Error; err != nil {
			b.Fatal(err)
		}
		b.StartTimer()
	}
}
