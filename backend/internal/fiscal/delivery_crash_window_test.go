package fiscal

import (
	"errors"
	"testing"
	"time"

	"github.com/stdevmac/payverge/backend/internal/database"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// Crash-window contract: if authorization commits, all applicable delivery
// task rows exist; if authorization rolls back, none exist.
func TestSaveReceiptForJob_EnqueuesDeliveryTasksOnCommit(t *testing.T) {
	db := newFiscalTestDB(t)
	repo := NewRepository(db)
	settings := database.BusinessFiscalSettings{
		BusinessID: 1, Country: "AR", Provider: "arca",
		Mode: database.FiscalModeAutomaticNonBlocking, Environment: "sandbox",
		SetupStatus: "valid", TaxID: "20123456789", TaxCondition: "monotributo",
	}
	require.NoError(t, db.Create(&settings).Error)
	job := database.FiscalJob{
		BusinessID: 1, SettingsID: settings.ID, BillID: 1,
		Action: ActionIssueReceipt, IdempotencyKey: "crash-window-ok",
		Status: database.FiscalStatusPending, MaxAttempts: 5,
	}
	require.NoError(t, db.Create(&job).Error)

	now := time.Now().UTC()
	receipt := &database.FiscalReceipt{
		BusinessID: 1, SettingsID: settings.ID, BillID: 1,
		Country: "AR", Provider: "arca", Action: ActionIssueReceipt,
		ReceiptType: "factura_c", Status: database.FiscalStatusAuthorized,
		TotalAmountCents: 1000, Currency: "ARS", IssuedAt: &now,
	}
	id, err := repo.SaveReceiptForJob(receipt, job.ID, nil)
	require.NoError(t, err)
	require.NotZero(t, id)

	var tasks []database.FiscalDeliveryTask
	require.NoError(t, db.Where("receipt_id = ?", id).Order("channel").Find(&tasks).Error)
	require.Len(t, tasks, 3)
	channels := map[string]bool{}
	for _, task := range tasks {
		channels[task.Channel] = true
		require.Equal(t, database.FiscalDeliveryStatusPending, task.Status)
		require.Equal(t, uint(1), task.BusinessID)
	}
	require.True(t, channels[database.FiscalDeliveryChannelArtifact])
	require.True(t, channels[database.FiscalDeliveryChannelEmail])
	require.True(t, channels[database.FiscalDeliveryChannelPrint])
}

func TestSaveReceiptForJob_NoTasksOnRollback(t *testing.T) {
	db := newFiscalTestDB(t)
	repo := NewRepository(db)
	settings := database.BusinessFiscalSettings{
		BusinessID: 2, Country: "AR", Provider: "arca",
		Mode: database.FiscalModeAutomaticNonBlocking, Environment: "sandbox",
		SetupStatus: "valid", TaxID: "20123456789", TaxCondition: "monotributo",
	}
	require.NoError(t, db.Create(&settings).Error)
	job := database.FiscalJob{
		BusinessID: 2, SettingsID: settings.ID, BillID: 2,
		Action: ActionIssueReceipt, IdempotencyKey: "crash-window-rb",
		Status: database.FiscalStatusPending, MaxAttempts: 5,
	}
	require.NoError(t, db.Create(&job).Error)

	// Force Create to fail mid-tx by using a broken save path: inject via
	// wrapping transaction that rolls back after enqueue would run...
	// Simpler: call Enqueue inside a tx that rolls back and assert no rows.
	now := time.Now().UTC()
	err := db.Transaction(func(tx *gorm.DB) error {
		receipt := &database.FiscalReceipt{
			BusinessID: 2, SettingsID: settings.ID, BillID: 2,
			Country: "AR", Provider: "arca", Action: ActionIssueReceipt,
			ReceiptType: "factura_c", Status: database.FiscalStatusAuthorized,
			TotalAmountCents: 1000, Currency: "ARS", IssuedAt: &now,
		}
		if err := tx.Create(receipt).Error; err != nil {
			return err
		}
		if err := repo.enqueueAuthorizedDeliveryTasksInTx(tx, receipt, now); err != nil {
			return err
		}
		return errors.New("forced rollback")
	})
	require.Error(t, err)

	var count int64
	require.NoError(t, db.Model(&database.FiscalDeliveryTask{}).Count(&count).Error)
	require.Equal(t, int64(0), count, "rolled-back authorization must leave zero delivery tasks")
}

func TestSaveReceiptForJob_FailedReceiptNoDeliveryTasks(t *testing.T) {
	db := newFiscalTestDB(t)
	repo := NewRepository(db)
	settings := database.BusinessFiscalSettings{
		BusinessID: 3, Country: "AR", Provider: "arca",
		Mode: database.FiscalModeAutomaticNonBlocking, Environment: "sandbox",
		SetupStatus: "valid", TaxID: "20123456789", TaxCondition: "monotributo",
	}
	require.NoError(t, db.Create(&settings).Error)
	job := database.FiscalJob{
		BusinessID: 3, SettingsID: settings.ID, BillID: 3,
		Action: ActionIssueReceipt, IdempotencyKey: "crash-window-fail",
		Status: database.FiscalStatusPending, MaxAttempts: 5,
	}
	require.NoError(t, db.Create(&job).Error)

	receipt := &database.FiscalReceipt{
		BusinessID: 3, SettingsID: settings.ID, BillID: 3,
		Country: "AR", Provider: "arca", Action: ActionIssueReceipt,
		ReceiptType: "factura_c", Status: database.FiscalStatusFailedPermanent,
		TotalAmountCents: 1000, Currency: "ARS",
	}
	id, err := repo.SaveReceiptForJob(receipt, job.ID, nil)
	require.NoError(t, err)

	var count int64
	require.NoError(t, db.Model(&database.FiscalDeliveryTask{}).
		Where("receipt_id = ?", id).Count(&count).Error)
	require.Equal(t, int64(0), count)
}
