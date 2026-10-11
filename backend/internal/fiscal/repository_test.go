package fiscal

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/runtimecontrol"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func newFiscalTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	require.NoError(t, err)
	// Pin to a single connection. An anonymous ":memory:" DB is per-connection,
	// so a second pooled connection is a brand-new empty database — AutoMigrate
	// and any test-created index (e.g. idx_fiscal_jobs_bill_issue_unique in
	// outbox_test.go) would be invisible on it, surfacing as intermittent
	// "no such table" / "UNIQUE constraint failed" flakes. Matches the pinning
	// already applied in worker_test.go and the bench harnesses.
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, db.AutoMigrate(
		&database.Business{},
		&database.Table{},
		&database.Bill{},
		&database.Payment{},
		&database.AlternativePayment{},
		&database.BusinessFiscalSettings{},
		&database.FiscalReceipt{},
		&database.FiscalJob{},
		&database.FiscalAuditEvent{},
		&database.FiscalDeliveryTask{},
		&runtimecontrol.Control{},
	))
	require.NoError(t, db.Create(&runtimecontrol.Control{
		Key: runtimecontrol.ControlFiscal, Enabled: true, Owner: "test",
		Reason: "fiscal tests enabled", ExpiresAt: time.Now().Add(24 * time.Hour),
		UpdatedBy: "test", UpdatedAt: time.Now(),
	}).Error)
	return db
}

// TestRepositoryListIssuableBills returns recent paid bills for the picker.
// Bills without a blocking issue_receipt stay issuable (ExistingReceiptID nil);
// authorized/pending bills are still listed with ExistingReceiptID so dinner
// service is not stuck on an empty "waiting" state (#260).
func TestRepositoryListIssuableBills(t *testing.T) {
	db := newFiscalTestDB(t)
	repo := NewRepository(db)
	const businessID uint = 50

	require.NoError(t, db.Create(&database.Business{
		ID: businessID, BusinessId: "biz-issuable-50", Name: "Issuable Cafe",
		OwnerAddress: "0xissuer", SettlementAddr: "settle", TippingAddr: "tip",
		DefaultCurrency: "ARS",
	}).Error)
	require.NoError(t, db.Create(&database.Table{
		ID: 7, BusinessID: businessID, TableCode: "T7-ISSUABLE", Name: "Patio 7",
	}).Error)

	closedOld := time.Now().UTC().Add(-2 * time.Hour)
	closedMid := time.Now().UTC().Add(-45 * time.Minute)
	closedNew := time.Now().UTC().Add(-30 * time.Minute)
	closedNewest := time.Now().UTC().Add(-10 * time.Minute)
	guest := "Ada Guest"

	// Paid, no receipt → issuable (newest).
	require.NoError(t, db.Create(&database.Bill{
		ID: 101, BusinessID: businessID, TableID: 7,
		BillNumber: "B-ISSUABLE-NEW", Status: database.BillStatusPaid,
		Items: "[]", TotalAmount: 5500, PaidAmount: 5500, ClosedAt: &closedNewest,
		FiscalCustomerName: &guest,
	}).Error)
	// Paid, only failed_retryable receipt → still issuable (older).
	require.NoError(t, db.Create(&database.Bill{
		ID: 102, BusinessID: businessID, TableID: 0,
		BillNumber: "B-ISSUABLE-FAILED", Status: database.BillStatusPaid,
		Items: "[]", TotalAmount: 1200, PaidAmount: 1200, ClosedAt: &closedOld,
	}).Error)
	require.NoError(t, db.Create(&database.FiscalReceipt{
		ID: 1, BusinessID: businessID, SettingsID: 1, BillID: 102,
		Country: "AR", Provider: "arca", Action: ActionIssueReceipt,
		ReceiptType: "B", TotalAmountCents: 1200, Currency: "ARS",
		Status: database.FiscalStatusFailedRetryable,
	}).Error)
	// Paid with authorized issue receipt → listed with ExistingReceiptID.
	require.NoError(t, db.Create(&database.Bill{
		ID: 103, BusinessID: businessID, TableID: 0,
		BillNumber: "B-BLOCKED-AUTH", Status: database.BillStatusPaid,
		Items: "[]", TotalAmount: 3000, PaidAmount: 3000, ClosedAt: &closedNew,
	}).Error)
	require.NoError(t, db.Create(&database.FiscalReceipt{
		ID: 2, BusinessID: businessID, SettingsID: 1, BillID: 103,
		Country: "AR", Provider: "arca", Action: ActionIssueReceipt,
		ReceiptType: "B", TotalAmountCents: 3000, Currency: "ARS",
		Status: database.FiscalStatusAuthorized,
	}).Error)
	// Paid with pending issue receipt → listed with ExistingReceiptID.
	require.NoError(t, db.Create(&database.Bill{
		ID: 104, BusinessID: businessID, TableID: 0,
		BillNumber: "B-BLOCKED-PEND", Status: database.BillStatusPaid,
		Items: "[]", TotalAmount: 800, PaidAmount: 800, ClosedAt: &closedMid,
	}).Error)
	require.NoError(t, db.Create(&database.FiscalReceipt{
		ID: 3, BusinessID: businessID, SettingsID: 1, BillID: 104,
		Country: "AR", Provider: "arca", Action: ActionIssueReceipt,
		ReceiptType: "B", TotalAmountCents: 800, Currency: "ARS",
		Status: database.FiscalStatusPending,
	}).Error)
	// Unpaid open bill → excluded.
	require.NoError(t, db.Create(&database.Bill{
		ID: 105, BusinessID: businessID, TableID: 0,
		BillNumber: "B-OPEN", Status: database.BillStatusOpen,
		Items: "[]", TotalAmount: 900, PaidAmount: 0,
	}).Error)
	// Paid with only a credit_note receipt → still issuable (credit notes do not block).
	require.NoError(t, db.Create(&database.Bill{
		ID: 106, BusinessID: businessID, TableID: 0,
		BillNumber: "B-CREDIT-ONLY", Status: database.BillStatusPaid,
		Items: "[]", TotalAmount: 400, PaidAmount: 400, ClosedAt: &closedOld,
	}).Error)
	require.NoError(t, db.Create(&database.FiscalReceipt{
		ID: 4, BusinessID: businessID, SettingsID: 1, BillID: 106,
		Country: "AR", Provider: "arca", Action: ActionCreditNote,
		ReceiptType: "NC", TotalAmountCents: 400, Currency: "ARS",
		Status: database.FiscalStatusAuthorized,
	}).Error)
	// Open bill that already collected payment (status not flipped yet) → issuable.
	require.NoError(t, db.Create(&database.Bill{
		ID: 107, BusinessID: businessID, TableID: 0,
		BillNumber: "B-OPEN-PAID", Status: database.BillStatusOpen,
		Items: "[]", TotalAmount: 2100, PaidAmount: 2100, ClosedAt: &closedNewest,
	}).Error)
	// Closed bill that already has an authorized invoice → listed for retrieve.
	require.NoError(t, db.Create(&database.Bill{
		ID: 108, BusinessID: businessID, TableID: 0,
		BillNumber: "B-CLOSED-INVOICED", Status: database.BillStatusClosed,
		Items: "[]", TotalAmount: 1800, PaidAmount: 1800, ClosedAt: &closedMid,
	}).Error)
	require.NoError(t, db.Create(&database.FiscalReceipt{
		ID: 5, BusinessID: businessID, SettingsID: 1, BillID: 108,
		Country: "AR", Provider: "arca", Action: ActionIssueReceipt,
		ReceiptType: "B", TotalAmountCents: 1800, Currency: "ARS",
		Status: database.FiscalStatusAuthorized,
	}).Error)
	// Paid with paid_amount 0 (known ledger edge) → still issuable.
	require.NoError(t, db.Create(&database.Bill{
		ID: 109, BusinessID: businessID, TableID: 0,
		BillNumber: "B-PAID-ZERO", Status: database.BillStatusPaid,
		Items: "[]", TotalAmount: 900, PaidAmount: 0, ClosedAt: &closedOld,
	}).Error)
	// Open unpaid, no receipt → still excluded.
	require.NoError(t, db.Create(&database.Bill{
		ID: 110, BusinessID: businessID, TableID: 0,
		BillNumber: "B-OPEN-UNPAID", Status: database.BillStatusOpen,
		Items: "[]", TotalAmount: 700, PaidAmount: 0,
	}).Error)

	// Other business paid bill → excluded.
	require.NoError(t, db.Create(&database.Business{
		ID: 99, BusinessId: "biz-other-99", Name: "Other",
		OwnerAddress: "0xother", SettlementAddr: "settle", TippingAddr: "tip",
		DefaultCurrency: "USD",
	}).Error)
	require.NoError(t, db.Create(&database.Bill{
		ID: 199, BusinessID: 99, BillNumber: "B-OTHER", Status: database.BillStatusPaid,
		Items: "[]", TotalAmount: 1000, PaidAmount: 1000, ClosedAt: &closedNew,
	}).Error)

	got, err := repo.ListIssuableBills(businessID, "", 20)
	require.NoError(t, err)
	require.Len(t, got, 8)
	// Newest first by closed_at, then id.
	require.Equal(t, uint(107), got[0].BillID)
	require.Equal(t, "B-OPEN-PAID", got[0].BillNumber)
	require.Nil(t, got[0].ExistingReceiptID)
	require.Equal(t, uint(101), got[1].BillID)
	require.Equal(t, "B-ISSUABLE-NEW", got[1].BillNumber)
	require.Equal(t, "Patio 7", got[1].TableLabel)
	require.InDelta(t, 55.0, got[1].TotalAmount, 0.001)
	require.Equal(t, "ARS", got[1].Currency)
	require.NotNil(t, got[1].ClosedAt)
	require.Nil(t, got[1].ExistingReceiptID)

	byID := map[uint]IssuableBill{}
	for _, row := range got {
		byID[row.BillID] = row
	}
	require.Contains(t, byID, uint(101))
	require.Contains(t, byID, uint(102))
	require.Contains(t, byID, uint(106))
	require.Contains(t, byID, uint(103))
	require.Contains(t, byID, uint(104))
	require.Contains(t, byID, uint(107))
	require.Contains(t, byID, uint(108))
	require.Contains(t, byID, uint(109))
	require.NotContains(t, byID, uint(105))
	require.NotContains(t, byID, uint(110))
	require.NotContains(t, byID, uint(199))
	require.Nil(t, byID[102].ExistingReceiptID)
	require.Nil(t, byID[106].ExistingReceiptID)
	require.Nil(t, byID[107].ExistingReceiptID)
	require.Nil(t, byID[109].ExistingReceiptID)
	require.NotNil(t, byID[103].ExistingReceiptID)
	require.Equal(t, uint(2), *byID[103].ExistingReceiptID)
	require.NotNil(t, byID[104].ExistingReceiptID)
	require.Equal(t, uint(3), *byID[104].ExistingReceiptID)
	require.NotNil(t, byID[108].ExistingReceiptID)
	require.Equal(t, uint(5), *byID[108].ExistingReceiptID)

	// q filters bill_number.
	byNumber, err := repo.ListIssuableBills(businessID, "ISSUABLE-NEW", 20)
	require.NoError(t, err)
	require.Len(t, byNumber, 1)
	require.Equal(t, uint(101), byNumber[0].BillID)

	// q filters table label.
	byTable, err := repo.ListIssuableBills(businessID, "patio", 20)
	require.NoError(t, err)
	require.Len(t, byTable, 1)
	require.Equal(t, uint(101), byTable[0].BillID)

	// q filters guest name.
	byGuest, err := repo.ListIssuableBills(businessID, "ada", 20)
	require.NoError(t, err)
	require.Len(t, byGuest, 1)
	require.Equal(t, uint(101), byGuest[0].BillID)

	// Already-invoiced bills remain discoverable by search.
	byBlocked, err := repo.ListIssuableBills(businessID, "BLOCKED-AUTH", 20)
	require.NoError(t, err)
	require.Len(t, byBlocked, 1)
	require.Equal(t, uint(103), byBlocked[0].BillID)
	require.NotNil(t, byBlocked[0].ExistingReceiptID)

	// limit caps results.
	limited, err := repo.ListIssuableBills(businessID, "", 1)
	require.NoError(t, err)
	require.Len(t, limited, 1)
	require.Equal(t, uint(107), limited[0].BillID)
}

func TestRepositoryCreateJobIdempotent(t *testing.T) {
	db := newFiscalTestDB(t)
	repo := NewRepository(db)
	settings := database.BusinessFiscalSettings{
		BusinessID:   1,
		Country:      "AR",
		Provider:     "arca",
		Mode:         database.FiscalModeAutomaticNonBlocking,
		Environment:  "sandbox",
		SetupStatus:  "valid",
		TaxID:        "20123456789",
		TaxCondition: "monotributo",
	}
	require.NoError(t, db.Create(&settings).Error)

	first, err := repo.CreateJobIfNotExists(CreateJobInput{
		BusinessID:     1,
		SettingsID:     settings.ID,
		BillID:         9,
		Action:         ActionIssueReceipt,
		IdempotencyKey: "business:1:bill:9:payment:none:action:issue_receipt:split:none",
		CreatedBy:      "system",
	})
	require.NoError(t, err)
	second, err := repo.CreateJobIfNotExists(CreateJobInput{
		BusinessID:     1,
		SettingsID:     settings.ID,
		BillID:         9,
		Action:         ActionIssueReceipt,
		IdempotencyKey: "business:1:bill:9:payment:none:action:issue_receipt:split:none",
		CreatedBy:      "system",
	})
	require.NoError(t, err)
	require.Equal(t, first.ID, second.ID)
	require.Equal(t, database.FiscalStatusPending, second.Status)

	var count int64
	require.NoError(t, db.Model(&database.FiscalJob{}).
		Where("idempotency_key = ?", "business:1:bill:9:payment:none:action:issue_receipt:split:none").
		Count(&count).Error)
	require.Equal(t, int64(1), count)
}

func TestRepositoryUpsertSettingsUpdatesExistingRow(t *testing.T) {
	db := newFiscalTestDB(t)
	repo := NewRepository(db)

	require.NoError(t, repo.UpsertSettings(&database.BusinessFiscalSettings{
		BusinessID:   1,
		Country:      "AR",
		Provider:     "arca",
		Mode:         database.FiscalModeManual,
		Environment:  "sandbox",
		TaxID:        "20123456789",
		TaxCondition: "monotributo",
		SetupStatus:  "draft",
	}))
	require.NoError(t, repo.UpsertSettings(&database.BusinessFiscalSettings{
		BusinessID:   1,
		Country:      "AR",
		Provider:     "arca",
		Mode:         database.FiscalModeAutomaticNonBlocking,
		Environment:  "production",
		TaxID:        "20987654321",
		TaxCondition: "responsable_inscripto",
		SetupStatus:  "valid",
	}))

	var count int64
	require.NoError(t, db.Model(&database.BusinessFiscalSettings{}).
		Where("business_id = ? AND country = ? AND provider = ?", 1, "AR", "arca").
		Count(&count).Error)
	require.Equal(t, int64(1), count)

	var persisted database.BusinessFiscalSettings
	require.NoError(t, db.Where("business_id = ? AND country = ? AND provider = ?", 1, "AR", "arca").
		First(&persisted).Error)
	require.Equal(t, database.FiscalModeAutomaticNonBlocking, persisted.Mode)
	require.Equal(t, "production", persisted.Environment)
	require.Equal(t, "20987654321", persisted.TaxID)
	require.Equal(t, "responsable_inscripto", persisted.TaxCondition)
	require.Equal(t, "valid", persisted.SetupStatus)
}

func TestRepositoryUpsertSettingsPreservesCredentialsWhenOmitted(t *testing.T) {
	db := newFiscalTestDB(t)
	repo := NewRepository(db)
	expiresAt := time.Date(2026, 5, 21, 12, 0, 0, 0, time.UTC)

	require.NoError(t, repo.UpsertSettings(&database.BusinessFiscalSettings{
		BusinessID:             1,
		Country:                "AR",
		Provider:               "arca",
		Mode:                   database.FiscalModeManual,
		Environment:            "sandbox",
		CredentialsEncrypted:   []byte("secret"),
		CredentialsFingerprint: "fingerprint",
		CredentialsExpiresAt:   &expiresAt,
		SetupStatus:            "valid",
	}))
	require.NoError(t, repo.UpsertSettings(&database.BusinessFiscalSettings{
		BusinessID:  1,
		Country:     "AR",
		Provider:    "arca",
		Mode:        database.FiscalModeAutomaticNonBlocking,
		Environment: "production",
		SetupStatus: "draft",
	}))

	var persisted database.BusinessFiscalSettings
	require.NoError(t, db.Where("business_id = ? AND country = ? AND provider = ?", 1, "AR", "arca").
		First(&persisted).Error)
	require.Equal(t, []byte("secret"), persisted.CredentialsEncrypted)
	require.Equal(t, "fingerprint", persisted.CredentialsFingerprint)
	require.NotNil(t, persisted.CredentialsExpiresAt)
	require.True(t, persisted.CredentialsExpiresAt.Equal(expiresAt))
	require.Equal(t, database.FiscalModeAutomaticNonBlocking, persisted.Mode)
	require.Equal(t, "production", persisted.Environment)
}

func TestRepositoryUpsertSettingsClearsCredentialsOnProviderSwitch(t *testing.T) {
	db := newFiscalTestDB(t)
	repo := NewRepository(db)
	expiresAt := time.Date(2026, 5, 21, 12, 0, 0, 0, time.UTC)

	require.NoError(t, repo.UpsertSettings(&database.BusinessFiscalSettings{
		BusinessID:             1,
		Country:                "AR",
		Provider:               "arca",
		Mode:                   database.FiscalModeManual,
		Environment:            "sandbox",
		CredentialsEncrypted:   []byte("arca-secret"),
		CredentialsFingerprint: "arca-fingerprint",
		CredentialsExpiresAt:   &expiresAt,
		SetupStatus:            "valid",
	}))
	require.NoError(t, repo.UpsertSettings(&database.BusinessFiscalSettings{
		BusinessID:  1,
		Country:     "AE",
		Provider:    "edicom",
		Mode:        database.FiscalModeAutomaticNonBlocking,
		Environment: "production",
		SetupStatus: "draft",
	}))

	settings, err := repo.GetSettings(1)
	require.NoError(t, err)
	require.NotNil(t, settings)
	require.Equal(t, "AE", settings.Country)
	require.Equal(t, "edicom", settings.Provider)
	require.Empty(t, settings.CredentialsEncrypted)
	require.Empty(t, settings.CredentialsFingerprint)
	require.Nil(t, settings.CredentialsExpiresAt)
}

func TestRepositoryUpsertSettingsUsesCanonicalRowPerBusiness(t *testing.T) {
	db := newFiscalTestDB(t)
	repo := NewRepository(db)

	require.NoError(t, repo.UpsertSettings(&database.BusinessFiscalSettings{
		BusinessID:  1,
		Country:     "AR",
		Provider:    "arca",
		Mode:        database.FiscalModeManual,
		Environment: "sandbox",
		SetupStatus: "valid",
	}))
	require.NoError(t, repo.UpsertSettings(&database.BusinessFiscalSettings{
		BusinessID:  1,
		Country:     "AE",
		Provider:    "edicom",
		Mode:        database.FiscalModeAutomaticNonBlocking,
		Environment: "production",
		SetupStatus: "valid",
	}))

	settings, err := repo.GetSettings(1)
	require.NoError(t, err)
	require.NotNil(t, settings)
	require.Equal(t, "AE", settings.Country)
	require.Equal(t, "edicom", settings.Provider)
	require.Equal(t, database.FiscalModeAutomaticNonBlocking, settings.Mode)

	active, err := repo.GetActiveSettings(1)
	require.NoError(t, err)
	require.NotNil(t, active)
	require.Equal(t, "AE", active.Country)
	require.Equal(t, "edicom", active.Provider)
}

func TestRepositoryUpsertSettingsDoesNotDeleteReferencedDuplicateRows(t *testing.T) {
	db := newFiscalTestDB(t)
	repo := NewRepository(db)

	canonical := database.BusinessFiscalSettings{
		BusinessID:  1,
		Country:     "AR",
		Provider:    "arca",
		Mode:        database.FiscalModeManual,
		Environment: "sandbox",
		SetupStatus: "valid",
	}
	require.NoError(t, db.Create(&canonical).Error)
	stale := database.BusinessFiscalSettings{
		BusinessID:  1,
		Country:     "AE",
		Provider:    "edicom",
		Mode:        database.FiscalModeManual,
		Environment: "sandbox",
		SetupStatus: "valid",
	}
	require.NoError(t, db.Create(&stale).Error)
	require.NoError(t, db.Create(&database.FiscalReceipt{
		BusinessID:       1,
		SettingsID:       stale.ID,
		BillID:           77,
		Country:          "AE",
		Provider:         "edicom",
		Action:           ActionIssueReceipt,
		ReceiptType:      "B",
		TotalAmountCents: 1000,
		Currency:         "AED",
		Status:           database.FiscalStatusAuthorized,
	}).Error)

	require.NoError(t, repo.UpsertSettings(&database.BusinessFiscalSettings{
		BusinessID:  1,
		Country:     "AE",
		Provider:    "edicom",
		Mode:        database.FiscalModeAutomaticNonBlocking,
		Environment: "production",
		SetupStatus: "valid",
	}))

	var count int64
	require.NoError(t, db.Model(&database.BusinessFiscalSettings{}).
		Where("business_id = ?", 1).
		Count(&count).Error)
	require.Equal(t, int64(2), count)

	settings, err := repo.GetSettings(1)
	require.NoError(t, err)
	require.NotNil(t, settings)
	require.Equal(t, stale.ID, settings.ID)
	require.Equal(t, "AE", settings.Country)
	require.Equal(t, "edicom", settings.Provider)
	require.Equal(t, database.FiscalModeAutomaticNonBlocking, settings.Mode)

	active, err := repo.GetActiveSettings(1)
	require.NoError(t, err)
	require.NotNil(t, active)
	require.Equal(t, "AE", active.Country)
	require.Equal(t, "edicom", active.Provider)
}

func TestRepositoryGetActiveSettings(t *testing.T) {
	db := newFiscalTestDB(t)
	repo := NewRepository(db)

	missing, err := repo.GetActiveSettings(404)
	require.NoError(t, err)
	require.Nil(t, missing)

	require.NoError(t, db.Create(&database.BusinessFiscalSettings{
		BusinessID:   1,
		Country:      "AR",
		Provider:     "arca",
		Mode:         database.FiscalModeOff,
		Environment:  "sandbox",
		SetupStatus:  "draft",
		TaxID:        "20123456789",
		TaxCondition: "monotributo",
	}).Error)
	off, err := repo.GetActiveSettings(1)
	require.NoError(t, err)
	require.Nil(t, off)

	require.NoError(t, db.Create(&database.BusinessFiscalSettings{
		BusinessID:   2,
		Country:      "AR",
		Provider:     "arca",
		Mode:         database.FiscalModeAutomaticNonBlocking,
		Environment:  "sandbox",
		SetupStatus:  "valid",
		TaxID:        "20987654321",
		TaxCondition: "responsable_inscripto",
	}).Error)
	active, err := repo.GetActiveSettings(2)
	require.NoError(t, err)
	require.NotNil(t, active)
	require.Equal(t, uint(2), active.BusinessID)
	require.Equal(t, database.FiscalModeAutomaticNonBlocking, active.Mode)
}

func TestRepositoryListReceiptsFiltersOrdersAndClampsLimits(t *testing.T) {
	db := newFiscalTestDB(t)
	repo := NewRepository(db)
	baseTime := time.Date(2026, 5, 21, 12, 0, 0, 0, time.UTC)

	for i := 0; i < 60; i++ {
		require.NoError(t, db.Create(&database.FiscalReceipt{
			BusinessID:       1,
			SettingsID:       1,
			BillID:           uint(i + 1),
			Country:          "AR",
			Provider:         "arca",
			Action:           ActionIssueReceipt,
			ReceiptType:      "B",
			TotalAmountCents: int64(i),
			Currency:         "ARS",
			Status:           database.FiscalStatusAuthorized,
			CreatedAt:        baseTime.Add(time.Duration(i) * time.Minute),
		}).Error)
	}
	require.NoError(t, db.Create(&database.FiscalReceipt{
		BusinessID:       1,
		SettingsID:       1,
		BillID:           100,
		Country:          "AR",
		Provider:         "arca",
		Action:           ActionIssueReceipt,
		ReceiptType:      "B",
		TotalAmountCents: 999,
		Currency:         "ARS",
		Status:           database.FiscalStatusRejected,
		CreatedAt:        baseTime.Add(2 * time.Hour),
	}).Error)
	require.NoError(t, db.Create(&database.FiscalReceipt{
		BusinessID:       2,
		SettingsID:       1,
		BillID:           101,
		Country:          "AR",
		Provider:         "arca",
		Action:           ActionIssueReceipt,
		ReceiptType:      "B",
		TotalAmountCents: 1000,
		Currency:         "ARS",
		Status:           database.FiscalStatusAuthorized,
		CreatedAt:        baseTime.Add(3 * time.Hour),
	}).Error)

	receipts, err := repo.ListReceipts(1, string(database.FiscalStatusAuthorized), 3)
	require.NoError(t, err)
	require.Len(t, receipts, 3)
	require.Equal(t, int64(59), receipts[0].TotalAmountCents)
	require.Equal(t, int64(58), receipts[1].TotalAmountCents)
	require.Equal(t, int64(57), receipts[2].TotalAmountCents)
	for _, receipt := range receipts {
		require.Equal(t, uint(1), receipt.BusinessID)
		require.Equal(t, database.FiscalStatusAuthorized, receipt.Status)
	}

	invalidLimit, err := repo.ListReceipts(1, string(database.FiscalStatusAuthorized), 0)
	require.NoError(t, err)
	require.Len(t, invalidLimit, 50)
	require.Equal(t, int64(59), invalidLimit[0].TotalAmountCents)
	require.Equal(t, int64(10), invalidLimit[49].TotalAmountCents)

	tooLargeLimit, err := repo.ListReceipts(1, string(database.FiscalStatusAuthorized), 1000)
	require.NoError(t, err)
	require.Len(t, tooLargeLimit, 50)
	require.Equal(t, int64(59), tooLargeLimit[0].TotalAmountCents)
	require.Equal(t, int64(10), tooLargeLimit[49].TotalAmountCents)
}

// seedListReceiptsPageFixture creates ~30 receipts across 3 UTC days for biz 1:
// two with a dead delivery task, one with failed_permanent status, and delivery
// tasks on every receipt so the embed is non-empty.
func seedListReceiptsPageFixture(t *testing.T, db *gorm.DB) (d1, d4 time.Time) {
	t.Helper()
	d1 = time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
	d4 = d1.AddDate(0, 0, 3) // exclusive end covering day 1–3
	now := time.Date(2026, 5, 4, 12, 0, 0, 0, time.UTC)

	for i := 0; i < 30; i++ {
		dayOffset := i / 10 // 0,1,2 → three days of 10
		created := d1.AddDate(0, 0, dayOffset).Add(time.Duration(i%10) * time.Hour)
		status := database.FiscalStatusAuthorized
		// One receipt with failed permanent status (needs_attention via receipt status).
		if i == 5 {
			status = database.FiscalStatusFailedPermanent
		}
		receipt := database.FiscalReceipt{
			BusinessID:       1,
			SettingsID:       1,
			BillID:           uint(i + 1),
			Country:          "AR",
			Provider:         "arca",
			Action:           ActionIssueReceipt,
			ReceiptType:      "B",
			TotalAmountCents: int64(1000 + i),
			Currency:         "ARS",
			Status:           status,
			CreatedAt:        created,
			UpdatedAt:        created,
		}
		require.NoError(t, db.Create(&receipt).Error)

		// Every receipt gets a succeeded email task so Delivery is non-empty.
		emailStatus := database.FiscalDeliveryStatusSucceeded
		// Two receipts get a dead print task (needs_attention via delivery).
		printStatus := ""
		if i == 7 || i == 12 {
			printStatus = database.FiscalDeliveryStatusDead
		}
		require.NoError(t, db.Create(&database.FiscalDeliveryTask{
			BusinessID:     1,
			ReceiptID:      receipt.ID,
			Channel:        database.FiscalDeliveryChannelEmail,
			Status:         emailStatus,
			MaxAttempts:    8,
			IdempotencyKey: fmt.Sprintf("receipt:%d:email", receipt.ID),
			NextAttemptAt:  &now,
			CreatedAt:      created,
			UpdatedAt:      created,
		}).Error)
		if printStatus != "" {
			require.NoError(t, db.Create(&database.FiscalDeliveryTask{
				BusinessID:     1,
				ReceiptID:      receipt.ID,
				Channel:        database.FiscalDeliveryChannelPrint,
				Status:         printStatus,
				MaxAttempts:    8,
				IdempotencyKey: fmt.Sprintf("receipt:%d:print", receipt.ID),
				NextAttemptAt:  &now,
				CreatedAt:      created,
				UpdatedAt:      created,
			}).Error)
		}
	}

	// Other business — must never appear in biz-1 pages.
	require.NoError(t, db.Create(&database.FiscalReceipt{
		BusinessID: 2, SettingsID: 1, BillID: 999,
		Country: "AR", Provider: "arca", Action: ActionIssueReceipt,
		ReceiptType: "B", TotalAmountCents: 1, Currency: "ARS",
		Status: database.FiscalStatusAuthorized, CreatedAt: d1.Add(time.Hour),
	}).Error)
	return d1, d4
}

func TestListReceiptsPageFiltersAndEmbedsDelivery(t *testing.T) {
	db := newFiscalTestDB(t)
	repo := NewRepository(db)
	d1, d4 := seedListReceiptsPageFixture(t, db)

	page, err := repo.ListReceiptsPage(ListReceiptsParams{
		BusinessID: 1, Start: &d1, End: &d4, Page: 1, PageSize: 20,
	})
	require.NoError(t, err)
	require.Len(t, page.Receipts, 20)
	require.Equal(t, int64(30), page.Total)
	require.Equal(t, 1, page.Page)
	require.Equal(t, 20, page.PageSize)
	require.Equal(t, 2, page.TotalPages)
	require.NotEmpty(t, page.Receipts[0].Delivery, "page rows must embed delivery badges")
	// Newest first: last seeded day-3 receipt is highest created_at among the 30.
	require.True(t, !page.Receipts[0].CreatedAt.Before(page.Receipts[len(page.Receipts)-1].CreatedAt))

	// needs_attention = receipt failed_* OR any dead delivery task → 1 + 2 = 3
	flagged, err := repo.ListReceiptsPage(ListReceiptsParams{
		BusinessID: 1, NeedsAttention: true, Page: 1, PageSize: 20,
	})
	require.NoError(t, err)
	require.Len(t, flagged.Receipts, 3)
	require.Equal(t, int64(3), flagged.Total)
	for _, row := range flagged.Receipts {
		require.True(t, row.NeedsAttention, "filtered rows must set needs_attention")
	}

	// Status filter still works on the paged path.
	authOnly, err := repo.ListReceiptsPage(ListReceiptsParams{
		BusinessID: 1,
		Status:     string(database.FiscalStatusAuthorized),
		Start:      &d1,
		End:        &d4,
		Page:       1,
		PageSize:   50,
	})
	require.NoError(t, err)
	require.Equal(t, int64(29), authOnly.Total) // 30 - 1 failed_permanent
	for _, row := range authOnly.Receipts {
		require.Equal(t, database.FiscalStatusAuthorized, row.Status)
	}

	// Half-open date window: only day 1 (10 receipts).
	d2 := d1.AddDate(0, 0, 1)
	day1, err := repo.ListReceiptsPage(ListReceiptsParams{
		BusinessID: 1, Start: &d1, End: &d2, Page: 1, PageSize: 50,
	})
	require.NoError(t, err)
	require.Equal(t, int64(10), day1.Total)
}

// Dinner-service findability: q matches guest/table/bill and rows embed table_label.
func TestListReceiptsPageSearchAndTableLabel(t *testing.T) {
	db := newFiscalTestDB(t)
	repo := NewRepository(db)

	require.NoError(t, db.Create(&database.Business{
		ID: 1, BusinessId: "biz-1", Name: "Cafe",
		OwnerAddress: "0x1", SettlementAddr: "s", TippingAddr: "t",
		DefaultCurrency: "ARS",
	}).Error)
	require.NoError(t, db.Create(&database.Table{
		ID: 11, BusinessID: 1, TableCode: "T6", Name: "Table 6",
	}).Error)
	guest := "Maria Guest"
	require.NoError(t, db.Create(&database.Bill{
		ID: 501, BusinessID: 1, TableID: 11,
		BillNumber: "DEMO-501", Status: database.BillStatusPaid,
		Items: "[]", TotalAmount: 2000, PaidAmount: 2000,
		FiscalCustomerName: &guest,
	}).Error)
	require.NoError(t, db.Create(&database.Bill{
		ID: 502, BusinessID: 1, TableID: 0,
		BillNumber: "OTHER-502", Status: database.BillStatusPaid,
		Items: "[]", TotalAmount: 1000, PaidAmount: 1000,
	}).Error)
	name := "Maria Guest"
	require.NoError(t, db.Create(&database.FiscalReceipt{
		ID: 901, BusinessID: 1, SettingsID: 1, BillID: 501,
		Country: "AR", Provider: "arca", Action: ActionIssueReceipt,
		ReceiptType: "factura_b", ReceiptNumber: strPtr("0001-00000901"),
		CustomerName: &name, TotalAmountCents: 2000, Currency: "ARS",
		Status: database.FiscalStatusAuthorized,
	}).Error)
	require.NoError(t, db.Create(&database.FiscalReceipt{
		ID: 902, BusinessID: 1, SettingsID: 1, BillID: 502,
		Country: "AR", Provider: "arca", Action: ActionIssueReceipt,
		ReceiptType: "factura_c", ReceiptNumber: strPtr("0001-00000902"),
		TotalAmountCents: 1000, Currency: "ARS",
		Status: database.FiscalStatusAuthorized,
	}).Error)

	byTable, err := repo.ListReceiptsPage(ListReceiptsParams{
		BusinessID: 1, Q: "table 6", Page: 1, PageSize: 20,
	})
	require.NoError(t, err)
	require.Equal(t, int64(1), byTable.Total)
	require.Equal(t, uint(901), byTable.Receipts[0].ID)
	require.Equal(t, "Table 6", byTable.Receipts[0].TableLabel)

	byGuest, err := repo.ListReceiptsPage(ListReceiptsParams{
		BusinessID: 1, Q: "maria", Page: 1, PageSize: 20,
	})
	require.NoError(t, err)
	require.Equal(t, int64(1), byGuest.Total)
	require.Equal(t, uint(901), byGuest.Receipts[0].ID)

	all, err := repo.ListReceiptsPage(ListReceiptsParams{
		BusinessID: 1, Page: 1, PageSize: 20,
	})
	require.NoError(t, err)
	require.Equal(t, int64(2), all.Total)
	byID := map[uint]ReceiptRow{}
	for _, row := range all.Receipts {
		byID[row.ID] = row
	}
	require.Equal(t, "Table 6", byID[901].TableLabel)
	require.Equal(t, "", byID[902].TableLabel)
}

func TestListReceiptsDeliveryIsSingleQuery(t *testing.T) {
	db := newFiscalTestDB(t)
	repo := NewRepository(db)
	_, _ = seedListReceiptsPageFixture(t, db)

	var deliveryQueryCount int64
	const cbName = "payverge:test:fiscal_delivery_tasks_query_count"
	require.NoError(t, db.Callback().Query().After("gorm:query").Register(cbName, func(tx *gorm.DB) {
		if tx.Statement == nil {
			return
		}
		sql := strings.ToLower(tx.Statement.SQL.String())
		table := tx.Statement.Table
		if tx.Statement.Schema != nil && tx.Statement.Schema.Table != "" {
			table = tx.Statement.Schema.Table
		}
		if table == "fiscal_delivery_tasks" || strings.Contains(sql, "fiscal_delivery_tasks") {
			// Count only SELECT-shaped queries (not the EXISTS subquery if GORM
			// materializes it separately — EXISTS is on the receipts filter).
			if strings.HasPrefix(strings.TrimSpace(sql), "select") {
				deliveryQueryCount++
			}
		}
	}))
	t.Cleanup(func() { _ = db.Callback().Query().Remove(cbName) })

	page, err := repo.ListReceiptsPage(ListReceiptsParams{
		BusinessID: 1, Page: 1, PageSize: 20,
	})
	require.NoError(t, err)
	require.Len(t, page.Receipts, 20)

	// One batched WHERE receipt_id IN (?) load — not N per row.
	require.Equal(t, int64(1), deliveryQueryCount,
		"delivery embed must use ONE fiscal_delivery_tasks query (got %d)", deliveryQueryCount)
}

// Comma-separated status filters translate to a status IN (...) so callers
// (sidebar failed-invoice badge, accounting badge probes) can count several
// failure statuses in one paginated probe instead of fetching every receipt.
func TestListReceiptsPageCommaStatusFilter(t *testing.T) {
	db := newFiscalTestDB(t)
	repo := NewRepository(db)
	d1, d4 := seedListReceiptsPageFixture(t, db)

	page, err := repo.ListReceiptsPage(ListReceiptsParams{
		BusinessID: 1,
		Status:     "authorized,failed_permanent",
		Start:      &d1,
		End:        &d4,
		Page:       1,
		PageSize:   1,
	})
	require.NoError(t, err)
	require.Equal(t, int64(30), page.Total, "comma statuses must union, not equality-match the raw string")

	failedOnly, err := repo.ListReceiptsPage(ListReceiptsParams{
		BusinessID: 1,
		Status:     "failed_permanent, rejected",
		Page:       1,
		PageSize:   1,
	})
	require.NoError(t, err)
	require.Equal(t, int64(1), failedOnly.Total, "whitespace around comma entries must be tolerated")

	// Legacy unpaginated path accepts the same comma form.
	legacy, err := repo.ListReceipts(1, "authorized,failed_permanent", 100)
	require.NoError(t, err)
	require.Len(t, legacy, 30)
}

func TestListReceiptsPageRemapsUSFacturaTypes(t *testing.T) {
	db := newFiscalTestDB(t)
	repo := NewRepository(db)
	const businessID uint = 74
	now := time.Now().UTC()

	require.NoError(t, db.Create(&database.Business{
		ID: businessID, BusinessId: "demo-admin-8-ai-pro", Name: "Core Kitchen",
		OwnerAddress: "0xus", SettlementAddr: "settle", TippingAddr: "tip",
		DefaultCurrency: "USD",
	}).Error)
	require.NoError(t, db.Create(&database.BusinessFiscalSettings{
		BusinessID: businessID, Country: "US", Provider: "demo",
		Mode: database.FiscalModeManual, Environment: "sandbox",
	}).Error)

	for i, rt := range []string{"factura_a", "factura_b", "factura_c"} {
		require.NoError(t, db.Create(&database.FiscalReceipt{
			BusinessID: businessID, SettingsID: 1, BillID: uint(200 + i),
			Country: "US", Provider: "demo", Action: ActionIssueReceipt,
			ReceiptType: rt, TotalAmountCents: 1000, Currency: "USD",
			Status: database.FiscalStatusAuthorized, CreatedAt: now,
		}).Error)
	}
	require.NoError(t, db.Create(&database.FiscalReceipt{
		BusinessID: businessID, SettingsID: 1, BillID: 210,
		Country: "AR", Provider: "arca", Action: ActionIssueReceipt,
		ReceiptType: "factura_a", TotalAmountCents: 1000, Currency: "ARS",
		Status: database.FiscalStatusAuthorized, CreatedAt: now,
	}).Error)

	page, err := repo.ListReceiptsPage(ListReceiptsParams{
		BusinessID: businessID, Page: 1, PageSize: 20,
	})
	require.NoError(t, err)
	got := map[string]int{}
	for _, row := range page.Receipts {
		if row.Country == "US" {
			require.NotContains(t, row.ReceiptType, "factura",
				"US leftover Factura letters must not leak on the list API")
		}
		got[row.Country+"/"+row.ReceiptType]++
	}
	require.Equal(t, 1, got["US/invoice"])
	require.Equal(t, 2, got["US/receipt"])
	require.Equal(t, 1, got["AR/factura_a"])

	invoices, err := repo.ListReceiptsPage(ListReceiptsParams{
		BusinessID: businessID, ReceiptType: "invoice", Page: 1, PageSize: 20,
	})
	require.NoError(t, err)
	require.GreaterOrEqual(t, invoices.Total, int64(1))
	for _, row := range invoices.Receipts {
		if row.Country == "US" {
			require.Equal(t, "invoice", row.ReceiptType)
		}
	}

	legacy, err := repo.ListReceipts(businessID, "authorized", 20)
	require.NoError(t, err)
	for _, rec := range legacy {
		if rec.Country == "US" {
			require.NotContains(t, rec.ReceiptType, "factura")
		}
	}
}

func TestProviderReceiptClaimedByOtherJob(t *testing.T) {
	db := newFiscalTestDB(t)
	repo := NewRepository(db)
	providerReceiptID := "11-1-42"
	receipt := &database.FiscalReceipt{
		BusinessID: 1, SettingsID: 9, BillID: 1,
		Country: "AR", Provider: "arca", Action: ActionIssueReceipt,
		ReceiptType: "factura_b", ProviderReceiptID: &providerReceiptID,
		TotalAmountCents: 5000, Currency: "ARS",
		Status: database.FiscalStatusAuthorized,
	}
	require.NoError(t, db.Create(receipt).Error)

	job := database.FiscalJob{SettingsID: 9}
	claimed, err := repo.ProviderReceiptClaimedByOtherJob(job, providerReceiptID)
	require.NoError(t, err)
	require.True(t, claimed, "authorized receipt with the same settings and id is claimed")

	job.ProducedReceiptID = &receipt.ID
	claimed, err = repo.ProviderReceiptClaimedByOtherJob(job, providerReceiptID)
	require.NoError(t, err)
	require.False(t, claimed, "the job's own produced receipt is not another job's claim")

	job.ProducedReceiptID = nil
	job.SettingsID = 10
	claimed, err = repo.ProviderReceiptClaimedByOtherJob(job, providerReceiptID)
	require.NoError(t, err)
	require.False(t, claimed, "a different settings id is a different series")

	job.SettingsID = 9
	require.NoError(t, db.Model(receipt).Update("status", database.FiscalStatusPending).Error)
	claimed, err = repo.ProviderReceiptClaimedByOtherJob(job, providerReceiptID)
	require.NoError(t, err)
	require.False(t, claimed, "a non-authorized receipt does not hold the voucher")
}
