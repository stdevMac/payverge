package analytics

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/stdevmac/payverge/backend/internal/database"
)

func TestGetTipsByStaff_AttributionAndSingleQuery(t *testing.T) {
	recorder := &recordingAnalyticsLogger{}
	db := setupAnalyticsTestDBWithLogger(t, recorder)
	fixedNow := time.Date(2026, 6, 15, 12, 0, 0, 0, time.UTC)
	svc := NewAnalyticsService(db).WithClock(func() time.Time { return fixedNow })

	business := &database.Business{
		BusinessId:     "tips-staff-" + t.Name(),
		Name:           "Tips",
		OwnerAddress:   "0xTipsOwner",
		SettlementAddr: "0x1111111111111111111111111111111111111111",
		TippingAddr:    "0x2222222222222222222222222222222222222222",
	}
	require.NoError(t, db.GetGorm().Create(business).Error)

	ana := &database.Staff{BusinessID: business.ID, Email: "ana@x.io", Name: "Ana", Role: "server", InvitedBy: "0xTipsOwner"}
	ben := &database.Staff{BusinessID: business.ID, Email: "ben@x.io", Name: "Ben", Role: "server", InvitedBy: "0xTipsOwner"}
	kai := &database.Staff{BusinessID: business.ID, Email: "kai@x.io", Name: "Kai", Role: "kitchen", InvitedBy: "0xTipsOwner"}
	require.NoError(t, db.GetGorm().Create(ana).Error)
	require.NoError(t, db.GetGorm().Create(ben).Error)
	require.NoError(t, db.GetGorm().Create(kai).Error)

	now := fixedNow.Add(-time.Hour)
	mkBill := func(number string, tip int64, created, closed *uint, status database.BillStatus) {
		var settledAt *time.Time
		paidAmount := int64(0)
		if status == database.BillStatusPaid || status == database.BillStatusClosed {
			settledAt = &now
			paidAmount = 1000
		}
		require.NoError(t, db.GetGorm().Create(&database.Bill{
			BusinessID:       business.ID,
			BillNumber:       number,
			Items:            "[]",
			TipAmount:        tip,
			TotalAmount:      1000,
			PaidAmount:       paidAmount,
			Status:           status,
			SettlementAddr:   "0x1111111111111111111111111111111111111111",
			TippingAddr:      "0x2222222222222222222222222222222222222222",
			CreatedByStaffID: created,
			ClosedByStaffID:  closed,
			CreatedAt:        now,
			ClosedAt:         settledAt,
		}).Error)
	}
	mkBill("T-1", 500, &ana.ID, &ben.ID, database.BillStatusPaid) // opener Ana wins over closer Ben
	mkBill("T-2", 300, &ana.ID, nil, database.BillStatusPaid)     // creator Ana
	mkBill("T-3", 200, nil, nil, database.BillStatusPaid)         // unattributed
	mkBill("T-4", 900, &ana.ID, &ana.ID, database.BillStatusOpen) // open — excluded
	mkBill("T-5", 0, &ana.ID, &ana.ID, database.BillStatusPaid)   // zero tip — excluded
	mkBill("T-6", 400, nil, &kai.ID, database.BillStatusPaid)     // kitchen closer → unattributed
	mkBill("T-7", 150, &kai.ID, &ben.ID, database.BillStatusPaid) // kitchen opener skipped → closer Ben

	recorder.Reset()
	rows, err := svc.GetTipsByStaff(business.ID, "month", time.UTC)
	require.NoError(t, err)

	// Access shape: ONE aggregate SELECT, no per-staff reloads, no SELECT *.
	sqls := recorder.SQLs()
	selects := 0
	for _, q := range sqls {
		trimmed := strings.TrimSpace(q)
		upper := strings.ToUpper(trimmed)
		if strings.HasPrefix(upper, "SELECT") || strings.HasPrefix(upper, "WITH") {
			selects++
			assert.NotContains(t, upper, "SELECT *")
		}
	}
	assert.Equal(t, 1, selects, "tips-by-staff must be a single aggregate query, got: %v", sqls)

	require.Len(t, rows, 3)
	byName := map[string]StaffTipRow{}
	for _, r := range rows {
		byName[r.StaffName] = r
	}
	assert.InDelta(t, 8.0, byName["Ana"].TotalTips, 0.001) // 500c + 300c opener
	assert.InDelta(t, 1.5, byName["Ben"].TotalTips, 0.001) // kitchen opener skipped → closer
	assert.InDelta(t, 6.0, byName[""].TotalTips, 0.001)    // unattributed + kitchen-only closer
	assert.NotContains(t, byName, "Kai")
	assert.EqualValues(t, 2, byName["Ana"].BillCount)
}

func TestGetTipsByStaff_UsesRecognizedSettlementEventsNotBillOpenTime(t *testing.T) {
	db := setupAnalyticsTestDB(t)
	fixedNow := time.Date(2026, 6, 15, 12, 0, 0, 0, time.UTC)
	svc := NewAnalyticsService(db).WithClock(func() time.Time { return fixedNow })

	business := &database.Business{Name: "Settlement tips"}
	require.NoError(t, db.GetGorm().Create(business).Error)
	server := &database.Staff{BusinessID: business.ID, Email: "settlement@tips.test", Name: "Settlement Server", Role: "server"}
	require.NoError(t, db.GetGorm().Create(server).Error)

	openedAt := time.Date(2026, 5, 31, 23, 55, 0, 0, time.UTC)
	closedAt := time.Date(2026, 6, 1, 0, 20, 0, 0, time.UTC)
	bill := &database.Bill{
		BusinessID: business.ID, BillNumber: "SETTLEMENT-TIPS-1", Status: database.BillStatusPaid,
		TotalAmount: 5000, PaidAmount: 5000, TipAmount: 600, CreatedByStaffID: &server.ID,
		ClosedByStaffID: &server.ID, CreatedAt: openedAt, UpdatedAt: closedAt, ClosedAt: &closedAt,
	}
	require.NoError(t, db.GetGorm().Create(bill).Error)

	first := time.Date(2026, 6, 1, 0, 5, 0, 0, time.UTC)
	second := time.Date(2026, 6, 1, 0, 20, 0, 0, time.UTC)
	reversed := time.Date(2026, 6, 2, 10, 0, 0, 0, time.UTC)
	require.NoError(t, db.GetGorm().Create(&database.Payment{
		BillID: bill.ID, Amount: 1000, TipAmount: 100, PaymentMethod: "card", TxHash: "settlement-reversed",
		Status: database.PaymentStatusReversed, ConfirmedAt: &first, ReversedAt: &reversed,
	}).Error)
	require.NoError(t, db.GetGorm().Create(&database.Payment{
		BillID: bill.ID, Amount: 2000, TipAmount: 200, PaymentMethod: "plugin", TxHash: "settlement-provider",
		Status: database.PaymentStatusConfirmed, ConfirmedAt: &second,
	}).Error)
	require.NoError(t, db.GetGorm().Create(&database.AlternativePayment{
		BillID: bill.ID, Amount: 2000, TipAmountCents: 300, PaymentMethod: database.PaymentMethodCash,
		ParticipantAddr: "cashier", Status: database.AltPaymentStatusConfirmed, ConfirmedAt: &second,
	}).Error)

	rows, err := svc.GetTipsByStaff(business.ID, "month", time.UTC)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.Equal(t, "Settlement Server", rows[0].StaffName)
	require.InDelta(t, 5.0, rows[0].TotalTips, 0.001, "100c reversed + 200c provider + 300c cash")
	require.EqualValues(t, 1, rows[0].BillCount, "multiple partial tips on one bill count once")
}

func BenchmarkGetTipsByStaff(b *testing.B) {
	db := setupAnalyticsTestDBWithLogger(b, nil)
	fixedNow := time.Date(2026, 6, 15, 12, 0, 0, 0, time.UTC)
	svc := NewAnalyticsService(db).WithClock(func() time.Time { return fixedNow })

	business := &database.Business{
		BusinessId:     fmt.Sprintf("tips-bench-%d", time.Now().UnixNano()),
		Name:           "Tips Bench",
		OwnerAddress:   "0xTipsBench",
		SettlementAddr: "0x1111111111111111111111111111111111111111",
		TippingAddr:    "0x2222222222222222222222222222222222222222",
	}
	require.NoError(b, db.GetGorm().Create(business).Error)

	runID := time.Now().UnixNano()
	staffIDs := make([]uint, 0, 20)
	for i := 0; i < 20; i++ {
		s := &database.Staff{
			BusinessID: business.ID,
			Email:      fmt.Sprintf("staff%d-%d@bench.io", i, runID),
			Name:       fmt.Sprintf("Staff%d", i),
			Role:       "server",
			InvitedBy:  "0xTipsBench",
		}
		require.NoError(b, db.GetGorm().Create(s).Error)
		staffIDs = append(staffIDs, s.ID)
	}

	bills := make([]database.Bill, 0, 2000)
	now := fixedNow.Add(-time.Hour)
	for i := 0; i < 2000; i++ {
		sid := staffIDs[i%len(staffIDs)]
		sidCopy := sid
		bills = append(bills, database.Bill{
			BusinessID:       business.ID,
			BillNumber:       fmt.Sprintf("BENCH-TIP-%d-%d", runID, i),
			Items:            "[]",
			TipAmount:        int64(100 + (i % 500)),
			Status:           database.BillStatusPaid,
			SettlementAddr:   "0x1111111111111111111111111111111111111111",
			TippingAddr:      "0x2222222222222222222222222222222222222222",
			CreatedByStaffID: &sidCopy,
			ClosedByStaffID:  &sidCopy,
			CreatedAt:        now,
		})
	}
	require.NoError(b, db.GetGorm().CreateInBatches(bills, 200).Error)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := svc.GetTipsByStaff(business.ID, "month", time.UTC); err != nil {
			b.Fatal(err)
		}
	}
}
