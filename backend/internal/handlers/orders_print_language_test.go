package handlers

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/stdevmac/payverge/backend/internal/database"
	print "github.com/stdevmac/payverge/backend/internal/services/print"
)

func TestEnqueueKitchenTicket_UsesBusinessDefaultLanguage(t *testing.T) {
	db, business, bill := setupPluginHandlersPrintTestDB(t)
	require.NoError(t, db.AutoMigrate(&database.Order{}, &database.Menu{}))
	require.NoError(t, db.Model(&database.Business{}).Where("id = ?", business.ID).
		Update("default_language", "es").Error)

	printer := database.Printer{
		BusinessID:   business.ID,
		Name:         "pass",
		Role:         "kitchen",
		Transport:    "browser",
		PaperWidthMM: 80,
		Enabled:      true,
	}
	require.NoError(t, db.Create(&printer).Error)

	order := database.Order{
		BillID:      bill.ID,
		BusinessID:  business.ID,
		OrderNumber: "O-" + t.Name(),
		Status:      database.OrderStatusApproved,
		Items:       "[]",
	}
	require.NoError(t, db.Create(&order).Error)

	prev := sharedPrintService
	SetSharedPrintService(print.NewService(db))
	defer SetSharedPrintService(prev)

	enqueueKitchenTicketForApprovedOrder(context.Background(), &order)

	var job database.PrintJob
	require.NoError(t, db.Where("order_id = ? AND kind = ?", order.ID, database.PrintJobKindKitchen).
		First(&job).Error)
	require.Equal(t, "es", job.Language,
		"kitchen tickets must print in the business default language, not default to en")
}
