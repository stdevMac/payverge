package fiscal

import (
	"context"
	"testing"
	"time"

	"github.com/stdevmac/payverge/backend/internal/database"

	"github.com/stretchr/testify/require"
)

// D-3: a status_check that resolves a failed receipt to authorized must backfill
// the FULL authorization tuple (CAE, receipt number, CAE expiry, QR) so the
// receipt can render a compliant PDF — not just flip the status.
func TestProcessStatusCheckJob_BackfillsAuthorizationDetail(t *testing.T) {
	db := newFiscalTestDB(t)
	svc := NewService(db, NewProviderRegistry())

	prid := "11-3-43"
	receipt := &database.FiscalReceipt{
		BusinessID: 1, SettingsID: 1, BillID: 1,
		Status:            database.FiscalStatusFailedRetryable,
		ProviderReceiptID: &prid,
		ReceiptType:       "factura_c",
	}
	require.NoError(t, db.Create(receipt).Error)

	exp := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	prov := &stubProvider{StatusFn: func(_ context.Context, id string) (*ReceiptStatus, error) {
		require.Equal(t, prid, id)
		return &ReceiptStatus{
			Status:        StatusAuthorized,
			AuthCode:      "70123456789012",
			ReceiptNumber: "43",
			AuthExpiresAt: &exp,
			QRPayload:     "https://afip.gob.ar/qr?p=xyz",
		}, nil
	}}

	rid := receipt.ID
	job := database.FiscalJob{BusinessID: 1, SettingsID: 1, BillID: 1, ReceiptID: &rid}
	job.ID = 5
	jobCtx := &JobContext{
		Bill:     database.Bill{TotalAmount: 5000},
		Settings: database.BusinessFiscalSettings{Country: "AR", Provider: "arca"},
	}

	outcome, err := svc.processStatusCheckJob(context.Background(), job, jobCtx, prov)
	require.NoError(t, err)
	require.Equal(t, database.FiscalStatusAuthorized, outcome.TerminalStatus)

	var got database.FiscalReceipt
	require.NoError(t, db.First(&got, receipt.ID).Error)
	require.Equal(t, database.FiscalStatusAuthorized, got.Status)
	require.NotNil(t, got.AuthCode)
	require.Equal(t, "70123456789012", *got.AuthCode)
	require.NotNil(t, got.ReceiptNumber)
	require.Equal(t, "43", *got.ReceiptNumber)
	require.NotNil(t, got.AuthExpiresAt)
	require.NotNil(t, got.QRPayload)
	require.Equal(t, "https://afip.gob.ar/qr?p=xyz", *got.QRPayload)
	require.NotNil(t, got.IssuedAt, "issued_at stamped on first authorization")
}

// A reconcile that confirms authorized but carries NO enrichment detail (empty
// CAE/number/QR — e.g. an un-rebuildable QR) still flips the status to authorized
// and must NOT clobber existing non-empty columns to empty.
func TestProcessStatusCheckJob_AuthorizedWithEmptyDetailDoesNotClobber(t *testing.T) {
	db := newFiscalTestDB(t)
	svc := NewService(db, NewProviderRegistry())

	existingCAE := "70999999999999"
	existingQR := "https://afip.gob.ar/qr?p=existing"
	existingNum := "43"
	prid := "11-3-43"
	receipt := &database.FiscalReceipt{
		BusinessID: 1, SettingsID: 1, BillID: 1,
		Status:            database.FiscalStatusFailedRetryable,
		ProviderReceiptID: &prid,
		AuthCode:          &existingCAE,
		ReceiptNumber:     &existingNum,
		QRPayload:         &existingQR,
		ReceiptType:       "factura_c",
	}
	require.NoError(t, db.Create(receipt).Error)

	prov := &stubProvider{StatusFn: func(_ context.Context, _ string) (*ReceiptStatus, error) {
		return &ReceiptStatus{Status: StatusAuthorized}, nil
	}}
	rid := receipt.ID
	job := database.FiscalJob{BusinessID: 1, SettingsID: 1, BillID: 1, ReceiptID: &rid}
	job.ID = 6

	outcome, err := svc.processStatusCheckJob(context.Background(), job, &JobContext{}, prov)
	require.NoError(t, err)
	require.Equal(t, database.FiscalStatusAuthorized, outcome.TerminalStatus)

	var got database.FiscalReceipt
	require.NoError(t, db.First(&got, receipt.ID).Error)
	require.Equal(t, database.FiscalStatusAuthorized, got.Status)
	require.NotNil(t, got.AuthCode)
	require.Equal(t, existingCAE, *got.AuthCode, "existing CAE must not be clobbered")
	require.NotNil(t, got.QRPayload)
	require.Equal(t, existingQR, *got.QRPayload)
	require.NotNil(t, got.ReceiptNumber)
	require.Equal(t, existingNum, *got.ReceiptNumber)
}

func TestBackfillAuthorizedReceipt_DoesNotMoveIssuedAtOrClobber(t *testing.T) {
	db := newFiscalTestDB(t)
	repo := NewRepository(db)

	priorIssued := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
	existingCAE := "70111111111111"
	r := &database.FiscalReceipt{
		BusinessID: 1, SettingsID: 1, BillID: 1,
		Status:      database.FiscalStatusFailedRetryable,
		IssuedAt:    &priorIssued,
		AuthCode:    &existingCAE,
		ReceiptType: "factura_c",
	}
	require.NoError(t, db.Create(r).Error)

	newIssued := time.Date(2026, 6, 20, 0, 0, 0, 0, time.UTC)
	err := repo.BackfillAuthorizedReceipt(r.ID, BackfillAuthorizedReceiptFields{
		ReceiptNumber: "50",
		QRPayload:     "https://q",
		IssuedAt:      &newIssued, // must NOT overwrite the existing issued_at
	}, time.Now())
	require.NoError(t, err)

	var got database.FiscalReceipt
	require.NoError(t, db.First(&got, r.ID).Error)
	require.Equal(t, database.FiscalStatusAuthorized, got.Status)
	require.NotNil(t, got.AuthCode)
	require.Equal(t, existingCAE, *got.AuthCode, "empty AuthCode must not clobber")
	require.NotNil(t, got.ReceiptNumber)
	require.Equal(t, "50", *got.ReceiptNumber)
	require.NotNil(t, got.IssuedAt)
	require.True(t, got.IssuedAt.Equal(priorIssued), "issued_at must not be moved by a re-confirmation")
}

func TestBackfillAuthorizedReceipt_StampsIssuedAtWhenNull(t *testing.T) {
	db := newFiscalTestDB(t)
	repo := NewRepository(db)

	r := &database.FiscalReceipt{
		BusinessID: 1, SettingsID: 1, BillID: 1,
		Status:      database.FiscalStatusFailedRetryable,
		ReceiptType: "factura_c",
	}
	require.NoError(t, db.Create(r).Error)

	newIssued := time.Date(2026, 6, 20, 0, 0, 0, 0, time.UTC)
	require.NoError(t, repo.BackfillAuthorizedReceipt(r.ID, BackfillAuthorizedReceiptFields{
		AuthCode: "70222222222222",
		IssuedAt: &newIssued,
	}, time.Now()))

	var got database.FiscalReceipt
	require.NoError(t, db.First(&got, r.ID).Error)
	require.NotNil(t, got.IssuedAt)
	require.True(t, got.IssuedAt.Equal(newIssued))
}
