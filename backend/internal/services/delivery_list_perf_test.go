package services

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stdevmac/payverge/backend/internal/database"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm/logger"
)

func TestGetBusinessDeliveriesRejectsUnknownStatusFilter(t *testing.T) {
	db := setupDeliveryPerfTestDB(t, nil)
	businessID := seedDeliveryListData(t, db, 5)
	bogus := database.DeliveryStatus("shipped")

	_, err := NewDeliveryService(db, nil).GetBusinessDeliveries(businessID, DeliveryListParams{
		Limit:  20,
		Status: &bogus,
	})
	require.Error(t, err)
	require.True(t, errors.Is(err, ErrUnknownDeliveryStatusFilter),
		"unknown status filter must surface a typed error, not silently return zero rows")
}

func TestGetBusinessDeliveriesAcceptsValidStatusFilter(t *testing.T) {
	db := setupDeliveryPerfTestDB(t, nil)
	businessID := seedDeliveryListData(t, db, 5)
	ready := database.DeliveryStatusReady

	result, err := NewDeliveryService(db, nil).GetBusinessDeliveries(businessID, DeliveryListParams{
		Limit:  20,
		Status: &ready,
	})
	require.NoError(t, err)
	require.Len(t, result.Deliveries, 5) // seedDeliveryListData seeds all rows as DeliveryStatusReady
}

func TestGetBusinessDeliveriesFilteredKeepsNarrowShape(t *testing.T) {
	recorder := &deliverySQLRecorder{Interface: logger.Default.LogMode(logger.Silent)}
	db := setupDeliveryPerfTestDB(t, recorder)
	businessID := seedDeliveryListData(t, db, 25)
	ready := database.DeliveryStatusReady

	recorder.statements = nil
	result, err := NewDeliveryService(db, nil).GetBusinessDeliveries(businessID, DeliveryListParams{
		Limit:  20,
		Status: &ready,
	})
	require.NoError(t, err)
	require.Len(t, result.Deliveries, 20)

	// Filtered list must not regress into bill/order preloads (dispatch UI renders neither).
	require.Zero(t, recorder.selectCount("bills"), "filtered delivery list must not preload bills")
	require.Zero(t, recorder.selectCount("orders"), "filtered delivery list must not preload orders")

	// The filtered path must not fan out into a SELECT * on the bills/orders
	// join tables. The base delivery_orders row IS selected in full and returned
	// to the dispatch UI as-is (the handler serializes the whole DeliveryListResult),
	// so a SELECT * on delivery_orders itself is the intended shape — what must
	// never happen is the filter dragging in an extra bills/orders scan.
	for _, stmt := range recorder.statements {
		normalized := strings.ToLower(strings.TrimSpace(stmt))
		if !strings.HasPrefix(normalized, "select *") {
			continue
		}
		if strings.Contains(normalized, "from `bills`") || strings.Contains(normalized, "from bills") {
			t.Fatalf("filtered delivery list issued a SELECT * on bills: %s", stmt)
		}
		if strings.Contains(normalized, "from `orders`") || strings.Contains(normalized, "from orders") {
			t.Fatalf("filtered delivery list issued a SELECT * on orders: %s", stmt)
		}
	}
}

func BenchmarkGetBusinessDeliveriesWithStatusFilterSQLite(b *testing.B) {
	db := setupDeliveryPerfTestDB(b, logger.Default.LogMode(logger.Silent))
	businessID := seedDeliveryListData(b, db, 500)
	service := NewDeliveryService(db, nil)
	ready := database.DeliveryStatusReady
	params := DeliveryListParams{Limit: 100, Status: &ready}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		result, err := service.GetBusinessDeliveries(businessID, params)
		if err != nil {
			b.Fatal(err)
		}
		if len(result.Deliveries) != 100 {
			b.Fatalf("expected 100 deliveries, got %d", len(result.Deliveries))
		}
	}
}

// TestGetBusinessDeliveries_ClaimSweepIsNotNPlusOne guards the L4-8 claim
// path: SweepStaleDeliveryClaims must be ONE bulk UPDATE (or zero when
// nothing is stale), not a per-row reload of claim holders. The list itself
// must still project claim columns from the base delivery_orders SELECT —
// no JOIN to staff and no second SELECT per row.
func TestGetBusinessDeliveries_ClaimSweepIsNotNPlusOne(t *testing.T) {
	recorder := &deliverySQLRecorder{Interface: logger.Default.LogMode(logger.Silent)}
	db := setupDeliveryPerfTestDB(t, recorder)
	businessID := seedDeliveryListData(t, db, 25)
	svc := NewDeliveryService(db, nil)

	// Seed a few fresh claims + one stale claim so the sweep has work.
	now := time.Now()
	stale := now.Add(-deliveryClaimIdleTTL - time.Minute)
	var rows []database.DeliveryOrder
	require.NoError(t, db.Where("business_id = ?", businessID).Limit(5).Find(&rows).Error)
	for i, row := range rows {
		sid := uint(100 + i)
		at := now
		if i == 0 {
			at = stale
		}
		require.NoError(t, db.Model(&database.DeliveryOrder{}).Where("id = ?", row.ID).Updates(map[string]interface{}{
			"claimed_by_staff_id": sid,
			"claimed_by_name":     fmt.Sprintf("Staff%d", sid),
			"claimed_by_role":     "server",
			"claimed_at":          at,
		}).Error)
	}

	recorder.statements = nil
	require.NoError(t, svc.SweepStaleDeliveryClaims(businessID))
	result, err := svc.GetBusinessDeliveries(businessID, DeliveryListParams{Limit: 20})
	require.NoError(t, err)
	require.NotEmpty(t, result.Deliveries)

	// Sweep: at most one UPDATE against delivery_orders (bulk).
	updateCount := 0
	for _, stmt := range recorder.statements {
		n := strings.ToLower(strings.TrimSpace(stmt))
		if strings.HasPrefix(n, "update") && strings.Contains(n, "delivery_orders") {
			updateCount++
		}
	}
	require.LessOrEqual(t, updateCount, 1, "claim sweep must be a single bulk UPDATE, got %d: %v", updateCount, recorder.statements)

	// List must not fan out into staff table reloads for claim holders.
	require.Zero(t, recorder.selectCount("staff"), "claim columns must ride the delivery_orders row, not a staff join")

	// Fresh claims survive the sweep; only the stale one is cleared.
	var stillClaimed int64
	require.NoError(t, db.Model(&database.DeliveryOrder{}).
		Where("business_id = ? AND claimed_by_staff_id IS NOT NULL", businessID).
		Count(&stillClaimed).Error)
	require.EqualValues(t, 4, stillClaimed, "exactly the 4 fresh claims must survive the idle sweep")

	// List SELECT is on delivery_orders itself (claim columns ride that row).
	// Assert we issued at least one SELECT against delivery_orders and never
	// against staff for claim-holder resolution.
	require.Greater(t, recorder.selectCount("delivery_orders"), 0)
	_ = result // list succeeded; claim columns are plain columns on DeliveryOrder
}

func BenchmarkGetBusinessDeliveriesWithClaimSweepSQLite(b *testing.B) {
	db := setupDeliveryPerfTestDB(b, logger.Default.LogMode(logger.Silent))
	businessID := seedDeliveryListData(b, db, 500)
	service := NewDeliveryService(db, nil)
	ready := database.DeliveryStatusReady
	params := DeliveryListParams{Limit: 100, Status: &ready}

	// Seed a mix of claimed rows so the sweep is realistic.
	var ids []uint
	require.NoError(b, db.Model(&database.DeliveryOrder{}).Where("business_id = ?", businessID).
		Limit(50).Pluck("id", &ids).Error)
	now := time.Now()
	for i, id := range ids {
		sid := uint(200 + i)
		at := now
		if i%5 == 0 {
			at = now.Add(-deliveryClaimIdleTTL - time.Minute)
		}
		_ = db.Model(&database.DeliveryOrder{}).Where("id = ?", id).Updates(map[string]interface{}{
			"claimed_by_staff_id": sid,
			"claimed_by_name":     "Dispatcher",
			"claimed_by_role":     "server",
			"claimed_at":          at,
		}).Error
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = service.SweepStaleDeliveryClaims(businessID)
		result, err := service.GetBusinessDeliveries(businessID, params)
		if err != nil {
			b.Fatal(err)
		}
		if len(result.Deliveries) == 0 {
			b.Fatal("expected deliveries")
		}
	}
}

func TestGetBusinessDeliveriesMultiStatusUsesINAndRejectsUnknown(t *testing.T) {
	recorder := &deliverySQLRecorder{Interface: logger.Default.LogMode(logger.Silent)}
	db := setupDeliveryPerfTestDB(t, recorder)
	businessID := seedDeliveryListData(t, db, 10)

	// Unknown value in the multi-status window must not silently match zero rows.
	_, err := NewDeliveryService(db, nil).GetBusinessDeliveries(businessID, DeliveryListParams{
		Limit:    20,
		Statuses: []database.DeliveryStatus{database.DeliveryStatusReady, database.DeliveryStatus("shipped")},
	})
	require.Error(t, err)
	require.True(t, errors.Is(err, ErrUnknownDeliveryStatusFilter))

	recorder.statements = nil
	result, err := NewDeliveryService(db, nil).GetBusinessDeliveries(businessID, DeliveryListParams{
		Limit: 20,
		Statuses: []database.DeliveryStatus{
			database.DeliveryStatusReady,
			database.DeliveryStatusPreparing,
			database.DeliveryStatusInTransit,
		},
	})
	require.NoError(t, err)
	require.Len(t, result.Deliveries, 10) // seed rows are all ready

	// Multi-status path must keep the narrow dispatch shape (no bill/order fan-out)
	// and issue an IN filter rather than N single-status round-trips.
	require.Zero(t, recorder.selectCount("bills"), "multi-status list must not preload bills")
	require.Zero(t, recorder.selectCount("orders"), "multi-status list must not preload orders")
	var sawIN bool
	for _, stmt := range recorder.statements {
		normalized := strings.ToLower(strings.Join(strings.Fields(stmt), " "))
		if strings.Contains(normalized, "status in") || strings.Contains(normalized, "`status` in") {
			sawIN = true
			break
		}
	}
	require.True(t, sawIN, "multi-status filter must compile to WHERE status IN (...)")
}

func TestUpdateDeliveryOrderContact_RejectsTerminalAndPatchesActive(t *testing.T) {
	db := setupDeliveryPerfTestDB(t, nil)
	require.NoError(t, db.AutoMigrate(&database.DeliveryStatusHistory{}))
	businessID := seedDeliveryListData(t, db, 2)
	service := NewDeliveryService(db, nil)

	var rows []database.DeliveryOrder
	require.NoError(t, db.Where("business_id = ?", businessID).Order("id asc").Find(&rows).Error)
	require.GreaterOrEqual(t, len(rows), 2)

	// Mark first row terminal.
	require.NoError(t, db.Model(&rows[0]).Update("status", database.DeliveryStatusDelivered).Error)
	phone := "555-EDIT"
	_, err := service.UpdateDeliveryOrderContact(businessID, rows[0].ID, DeliveryOrderContactPatch{
		CustomerPhone: &phone,
		ChangedBy:     "test",
	})
	require.Error(t, err)
	require.True(t, errors.Is(err, ErrDeliveryOrderNotEditable))

	// Active row accepts the patch; access shape is a single Updates + audit insert.
	apt := "4C"
	updated, err := service.UpdateDeliveryOrderContact(businessID, rows[1].ID, DeliveryOrderContactPatch{
		CustomerPhone: &phone,
		Apartment:     &apt,
		ChangedBy:     "test-operator",
	})
	require.NoError(t, err)
	require.Equal(t, "555-EDIT", updated.CustomerPhone)
	require.Equal(t, "4C", updated.DeliveryAddress.Apartment)

	var histCount int64
	require.NoError(t, db.Model(&database.DeliveryStatusHistory{}).
		Where("delivery_order_id = ? AND changed_by = ?", rows[1].ID, "test-operator").
		Count(&histCount).Error)
	require.Equal(t, int64(1), histCount)
}

func BenchmarkUpdateDeliveryOrderContactSQLite(b *testing.B) {
	db := setupDeliveryPerfTestDB(b, logger.Default.LogMode(logger.Silent))
	require.NoError(b, db.AutoMigrate(&database.DeliveryStatusHistory{}))
	businessID := seedDeliveryListData(b, db, 50)
	service := NewDeliveryService(db, nil)
	var order database.DeliveryOrder
	require.NoError(b, db.Where("business_id = ?", businessID).First(&order).Error)
	phone := "5550001111"

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		p := fmt.Sprintf("555-%d", i)
		_, err := service.UpdateDeliveryOrderContact(businessID, order.ID, DeliveryOrderContactPatch{
			CustomerPhone: &p,
			ChangedBy:     "bench",
		})
		if err != nil {
			// phone string is fine; ignore
			_ = phone
			b.Fatal(err)
		}
	}
}

func BenchmarkGetBusinessDeliveriesWithMultiStatusFilterSQLite(b *testing.B) {
	db := setupDeliveryPerfTestDB(b, logger.Default.LogMode(logger.Silent))
	businessID := seedDeliveryListData(b, db, 500)
	service := NewDeliveryService(db, nil)
	params := DeliveryListParams{
		Limit: 100,
		Statuses: []database.DeliveryStatus{
			database.DeliveryStatusReady,
			database.DeliveryStatusPreparing,
			database.DeliveryStatusAssigned,
			database.DeliveryStatusPickedUp,
			database.DeliveryStatusInTransit,
			database.DeliveryStatusNearby,
		},
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		result, err := service.GetBusinessDeliveries(businessID, params)
		if err != nil {
			b.Fatal(err)
		}
		if len(result.Deliveries) != 100 {
			b.Fatalf("expected 100 deliveries, got %d", len(result.Deliveries))
		}
	}
}

// countStatements returns how many recorded statements contain every one of the
// supplied lowercase fragments. Used to prove the money/geo hydrate is a single
// JOIN and never a per-row reload.
func (r *deliverySQLRecorder) countStatements(fragments ...string) int {
	count := 0
	for _, statement := range r.statements {
		normalized := strings.ToLower(strings.Join(strings.Fields(statement), " "))
		matched := true
		for _, fragment := range fragments {
			if !strings.Contains(normalized, fragment) {
				matched = false
				break
			}
		}
		if matched {
			count++
		}
	}
	return count
}

// Backend Performance Gate (#896): the dispatch list gained a bill/business
// JOIN to project total + currency + venue coordinates. The access shape must
// be ONE query for the whole page regardless of how many rows come back — a
// per-row bill or business reload here would be an N+1 on the single most
// polled operator screen.
func TestGetBusinessDeliveries_MoneyGeoHydrateIsOneQueryForAnyN(t *testing.T) {
	recorder := &deliverySQLRecorder{Interface: logger.Default.LogMode(logger.Silent)}
	db := setupDeliveryPerfTestDB(t, recorder)
	businessID := seedDeliveryListData(t, db, 40)
	svc := NewDeliveryService(db, nil)

	measure := func(limit int) (selects int, joins int, rows int) {
		recorder.statements = nil
		result, err := svc.GetBusinessDeliveries(businessID, DeliveryListParams{Limit: limit})
		require.NoError(t, err)
		require.Len(t, result.Deliveries, limit)
		for _, statement := range recorder.statements {
			if strings.HasPrefix(strings.ToLower(strings.TrimSpace(statement)), "select") {
				selects++
			}
		}
		return selects, recorder.countStatements("left join bills"), len(result.Deliveries)
	}

	smallSelects, smallJoins, smallRows := measure(4)
	bigSelects, bigJoins, bigRows := measure(32)

	require.Equal(t, 4, smallRows)
	require.Equal(t, 32, bigRows)
	require.Equal(t, 1, smallJoins, "money/geo hydrate must be exactly one JOIN query")
	require.Equal(t, 1, bigJoins,
		"money/geo hydrate must stay one JOIN query for a full page, got %d: %v",
		bigJoins, recorder.statements)
	require.Equal(t, smallSelects, bigSelects,
		"delivery list query count must be constant in row count (N+1 regression): %d selects for 4 rows vs %d for 32: %v",
		smallSelects, bigSelects, recorder.statements)

	// The hydrate must be a JOIN, never a standalone reload of the joined rows.
	require.Zero(t, recorder.selectCount("bills"),
		"hydrate must join bills, not issue its own SELECT FROM bills")
	require.Zero(t, recorder.selectCount("businesses"),
		"hydrate must join businesses, not preload the full business row per delivery")
	require.Zero(t, recorder.selectCount("orders"), "dispatch list must not preload orders")
}

// The hydrator must hand MarshalJSON raw cents. A pre-divided float here is the
// second conversion site the money wire contract forbids (#896).
func TestGetBusinessDeliveries_HydratesCentsNotDollars(t *testing.T) {
	db := setupDeliveryPerfTestDB(t, nil)
	businessID := seedDeliveryListData(t, db, 3)

	result, err := NewDeliveryService(db, nil).GetBusinessDeliveries(businessID, DeliveryListParams{Limit: 10})
	require.NoError(t, err)
	require.Len(t, result.Deliveries, 3)

	for i := range result.Deliveries {
		row := result.Deliveries[i]
		var bill database.Bill
		require.NoError(t, db.First(&bill, row.BillID).Error)
		require.NotNil(t, row.DispatchTotalCents, "every list row must carry its bill total")
		require.Equal(t, bill.TotalAmount, *row.DispatchTotalCents,
			"projection must carry cents verbatim; MarshalJSON owns the /100")

		raw, err := json.Marshal(row)
		require.NoError(t, err)
		var decoded map[string]any
		require.NoError(t, json.Unmarshal(raw, &decoded))
		require.InDelta(t, float64(bill.TotalAmount)/100.0, decoded["total"], 0.0001,
			"wire total must be dollars")
	}
}

// 896-B: currency follows display_currency → default_currency → USD, the same
// precedence bills and orders use. businesses.default_currency has no DB
// default, so the USD floor is what keeps the chip from rendering blank.
func TestGetBusinessDeliveries_CurrencyPrefersDisplayThenDefaultThenUSD(t *testing.T) {
	db := setupDeliveryPerfTestDB(t, nil)
	businessID := seedDeliveryListData(t, db, 2)
	svc := NewDeliveryService(db, nil)

	currencies := func() []string {
		result, err := svc.GetBusinessDeliveries(businessID, DeliveryListParams{Limit: 10})
		require.NoError(t, err)
		require.NotEmpty(t, result.Deliveries)
		out := make([]string, 0, len(result.Deliveries))
		for i := range result.Deliveries {
			out = append(out, result.Deliveries[i].DispatchCurrency)
		}
		return out
	}

	// Neither column set — the seeded shape, and the state the original bug
	// report hit: currency must not come back empty.
	for _, currency := range currencies() {
		require.Equal(t, "USD", currency, "unset currency columns must floor to USD, not blank")
	}

	require.NoError(t, db.Model(&database.Business{}).Where("id = ?", businessID).
		Update("default_currency", "ARS").Error)
	for _, currency := range currencies() {
		require.Equal(t, "ARS", currency, "default_currency is the fallback")
	}

	require.NoError(t, db.Model(&database.Business{}).Where("id = ?", businessID).
		Update("display_currency", "BRL").Error)
	for _, currency := range currencies() {
		require.Equal(t, "BRL", currency,
			"display_currency must win, or dispatch disagrees with every other screen")
	}
}

// The detail read shares the hydrator, so it must agree with the list on both
// the cents contract and the currency precedence.
func TestGetDeliveryOrder_MoneyProjectionMatchesList(t *testing.T) {
	db := setupDeliveryPerfTestDB(t, nil)
	// The detail read preloads relations the list does not; the shared perf
	// fixture only migrates the list's tables.
	require.NoError(t, db.AutoMigrate(&database.DeliveryStatusHistory{}, &database.Customer{}))
	businessID := seedDeliveryListData(t, db, 2)
	require.NoError(t, db.Model(&database.Business{}).Where("id = ?", businessID).
		Updates(map[string]any{"display_currency": "BRL", "default_currency": "ARS"}).Error)
	svc := NewDeliveryService(db, nil)

	list, err := svc.GetBusinessDeliveries(businessID, DeliveryListParams{Limit: 10})
	require.NoError(t, err)
	require.NotEmpty(t, list.Deliveries)
	listed := list.Deliveries[0]

	detail, err := svc.GetDeliveryOrder(listed.ID)
	require.NoError(t, err)
	require.NotNil(t, detail.DispatchTotalCents)
	require.NotNil(t, listed.DispatchTotalCents)
	require.Equal(t, *listed.DispatchTotalCents, *detail.DispatchTotalCents)
	require.Equal(t, "BRL", detail.DispatchCurrency)
	require.Equal(t, listed.DispatchCurrency, detail.DispatchCurrency)
}

func BenchmarkGetBusinessDeliveriesMoneyGeoHydrateSQLite(b *testing.B) {
	db := setupDeliveryPerfTestDB(b, logger.Default.LogMode(logger.Silent))
	businessID := seedDeliveryListData(b, db, 500)
	service := NewDeliveryService(db, nil)
	params := DeliveryListParams{Limit: 100}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		result, err := service.GetBusinessDeliveries(businessID, params)
		if err != nil {
			b.Fatal(err)
		}
		if len(result.Deliveries) != 100 {
			b.Fatalf("expected 100 deliveries, got %d", len(result.Deliveries))
		}
		if result.Deliveries[0].DispatchTotalCents == nil {
			b.Fatal("money/geo hydrate did not run")
		}
	}
}
