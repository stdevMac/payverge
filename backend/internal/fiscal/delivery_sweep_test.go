package fiscal

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stdevmac/payverge/backend/internal/database"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func seedSweepReceipt(t *testing.T, db *gorm.DB, businessID, billID, settingsID uint, status database.FiscalStatus, issuedAt, deliveredAt *time.Time) uint {
	t.Helper()
	num := "43"
	cae := "71000000000001"
	qr := "https://www.afip.gob.ar/fe/qr/?p=abc"
	r := &database.FiscalReceipt{
		BusinessID:       businessID,
		SettingsID:       settingsID,
		BillID:           billID,
		Country:          "AR",
		Provider:         "arca",
		Action:           ActionIssueReceipt,
		ReceiptType:      "factura_c",
		ReceiptNumber:    &num,
		AuthCode:         &cae,
		QRPayload:        &qr,
		TotalAmountCents: 12100,
		Currency:         "ARS",
		Status:           status,
		IssuedAt:         issuedAt,
		DeliveredAt:      deliveredAt,
	}
	require.NoError(t, db.Create(r).Error)
	return r.ID
}

func TestListUndeliveredAuthorizedReceipts_FiltersByStatusDeliveryAndAge(t *testing.T) {
	db := newFiscalTestDB(t)
	repo := NewRepository(db)
	old := time.Date(2026, 6, 20, 12, 0, 0, 0, time.UTC)
	cutoff := old.Add(10 * time.Minute) // olderThan grace boundary
	fresh := cutoff.Add(time.Minute)    // newer than the cutoff (inside grace)
	delivered := old

	wantID := seedSweepReceipt(t, db, 1, 1, 1, database.FiscalStatusAuthorized, &old, nil) // INCLUDED
	seedSweepReceipt(t, db, 1, 2, 1, database.FiscalStatusAuthorized, &fresh, nil)         // too fresh
	seedSweepReceipt(t, db, 1, 3, 1, database.FiscalStatusAuthorized, &old, &delivered)    // already delivered
	seedSweepReceipt(t, db, 1, 4, 1, database.FiscalStatusFailedRetryable, &old, nil)      // not authorized
	seedSweepReceipt(t, db, 1, 5, 1, database.FiscalStatusAuthorized, nil, nil)            // never issued (NULL)

	got, err := repo.ListUndeliveredAuthorizedReceipts(cutoff, 25)
	require.NoError(t, err)
	require.Len(t, got, 1)
	require.Equal(t, wantID, got[0].ID)
}

func TestListUndeliveredAuthorizedReceipts_BoundedByLimit(t *testing.T) {
	db := newFiscalTestDB(t)
	repo := NewRepository(db)
	base := time.Date(2026, 6, 20, 12, 0, 0, 0, time.UTC)
	for i := 0; i < 5; i++ {
		issued := base.Add(time.Duration(i) * time.Minute)
		seedSweepReceipt(t, db, 1, uint(i+1), 1, database.FiscalStatusAuthorized, &issued, nil)
	}
	got, err := repo.ListUndeliveredAuthorizedReceipts(base.Add(time.Hour), 2)
	require.NoError(t, err)
	require.Len(t, got, 2, "must be bounded by limit")
}

func TestSweepUndeliveredReceipts_RedeliversAndStamps(t *testing.T) {
	disp := &stubDispatcher{}
	db := newFiscalTestDBWithCustomer(t)
	svc := NewService(db, NewProviderRegistry()).WithDelivery(disp)

	seedBillWithCustomer(t, db, 1, 1, 100, "guest@example.com", 12100)
	settingsID := seedReadySettings(t, db, 1)
	old := time.Date(2026, 6, 20, 12, 0, 0, 0, time.UTC)
	rid := seedSweepReceipt(t, db, 1, 1, settingsID, database.FiscalStatusAuthorized, &old, nil)

	delivered, err := svc.SweepUndeliveredReceipts(context.Background(), old.Add(time.Hour), 25)
	require.NoError(t, err)
	require.Equal(t, 1, delivered)
	require.Equal(t, 1, disp.emailCalls, "the undelivered receipt must be re-emailed")

	var r database.FiscalReceipt
	require.NoError(t, db.First(&r, rid).Error)
	require.NotNil(t, r.DeliveredAt, "a successful sweep delivery must stamp delivered_at")
}

func TestSweepUndeliveredReceipts_EmailFailureLeavesUndelivered(t *testing.T) {
	disp := &stubDispatcher{emailErr: errors.New("smtp down")}
	db := newFiscalTestDBWithCustomer(t)
	svc := NewService(db, NewProviderRegistry()).WithDelivery(disp)

	seedBillWithCustomer(t, db, 1, 1, 100, "guest@example.com", 12100)
	settingsID := seedReadySettings(t, db, 1)
	old := time.Date(2026, 6, 20, 12, 0, 0, 0, time.UTC)
	rid := seedSweepReceipt(t, db, 1, 1, settingsID, database.FiscalStatusAuthorized, &old, nil)

	delivered, err := svc.SweepUndeliveredReceipts(context.Background(), old.Add(time.Hour), 25)
	require.NoError(t, err)
	require.Equal(t, 0, delivered)

	var r database.FiscalReceipt
	require.NoError(t, db.First(&r, rid).Error)
	require.Nil(t, r.DeliveredAt, "a failed delivery must leave the receipt undelivered for the next sweep")
}

func TestSweepUndeliveredReceipts_NilDispatcherNoOp(t *testing.T) {
	db := newFiscalTestDB(t)
	svc := NewService(db, NewProviderRegistry()) // delivery not wired
	n, err := svc.SweepUndeliveredReceipts(context.Background(), time.Date(2026, 6, 20, 12, 0, 0, 0, time.UTC), 25)
	require.NoError(t, err)
	require.Equal(t, 0, n)
}
