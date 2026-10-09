//go:build integration_postgres

package fiscal

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/testperf"

	"github.com/stretchr/testify/require"
)

// Postgres SKIP LOCKED proof: two concurrent ClaimDueDeliveryTasks yield exactly
// one lease owner for a single due task.
func TestClaimDueDeliveryTasks_PostgresSkipLocked(t *testing.T) {
	ctx := context.Background()
	pg, err := testperf.StartPostgres(ctx)
	if err != nil {
		t.Skipf("postgres unavailable: %v", err)
	}
	t.Cleanup(func() { _ = pg.Terminate(ctx) })

	db := pg.DB
	require.NoError(t, db.AutoMigrate(
		&database.Business{},
		&database.Bill{},
		&database.BusinessFiscalSettings{},
		&database.FiscalReceipt{},
		&database.FiscalJob{},
		&database.FiscalAuditEvent{},
		&database.FiscalDeliveryTask{},
	))

	biz := database.Business{ID: 1, BusinessId: "biz-1", OwnerAddress: "o", Name: "N", SettlementAddr: "s", TippingAddr: "t"}
	require.NoError(t, db.Create(&biz).Error)
	settings := database.BusinessFiscalSettings{
		BusinessID: 1, Country: "AR", Provider: "arca",
		Mode: database.FiscalModeAutomaticNonBlocking, Environment: "sandbox",
		SetupStatus: "valid", TaxID: "20123456789", TaxCondition: "monotributo",
	}
	require.NoError(t, db.Create(&settings).Error)
	bill := database.Bill{BusinessID: 1, Status: "paid", BillNumber: "B-skip"}
	require.NoError(t, db.Create(&bill).Error)
	now := time.Now().UTC()
	receipt := database.FiscalReceipt{
		BusinessID: 1, SettingsID: settings.ID, BillID: bill.ID,
		Country: "AR", Provider: "arca", Action: ActionIssueReceipt,
		ReceiptType: "factura_c", Status: database.FiscalStatusAuthorized,
		TotalAmountCents: 100, Currency: "ARS", IssuedAt: &now,
	}
	require.NoError(t, db.Create(&receipt).Error)

	repo := NewRepository(db)
	require.NoError(t, repo.EnqueueDeliveryTasks(db, receipt.ID, 1, []DeliveryChannelSpec{
		{Channel: database.FiscalDeliveryChannelArtifact},
	}, "skip-locked", now))

	var wg sync.WaitGroup
	var mu sync.Mutex
	var winners []string
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			claimed, err := repo.ClaimDueDeliveryTasks("pg-worker-"+string(rune('a'+id)), now, 2*time.Minute, 1)
			if err != nil || len(claimed) != 1 {
				return
			}
			mu.Lock()
			winners = append(winners, claimed[0].LeaseOwner)
			mu.Unlock()
		}(i)
	}
	wg.Wait()
	require.Len(t, winners, 1, "SKIP LOCKED must hand the row to exactly one worker; got %v", winners)
}
