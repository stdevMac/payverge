package print

import (
	"testing"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stretchr/testify/assert"
)

func TestPrintWakeEventType(t *testing.T) {
	assert.Equal(t, "print.bill_available", printWakeEventType(database.PrintJobKindBill))
	assert.Equal(t, "print.receipt_available", printWakeEventType(database.PrintJobKindReceipt))
	for _, kind := range []database.PrintJobKind{
		database.PrintJobKindKitchen,
		database.PrintJobKindBar,
		database.PrintJobKindVoid,
		database.PrintJobKindModify,
	} {
		assert.Equal(t, "print.kitchen_available", printWakeEventType(kind))
	}
}

func TestPrintWakePayloadContainsIdentifiersOnly(t *testing.T) {
	printerID := uint(7)
	payload := printWakePayload(&database.PrintJob{
		ID: 9, BusinessID: 3, PrinterID: &printerID,
		Kind: database.PrintJobKindBill,
	})
	assert.Equal(t, map[string]interface{}{
		"job_id":     uint(9),
		"printer_id": uint(7),
		"kind":       database.PrintJobKindBill,
	}, payload)
	assert.NotContains(t, payload, "payload_html")
	assert.NotContains(t, payload, "source_id")
}
