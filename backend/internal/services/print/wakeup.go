package print

import (
	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/events"
)

func printWakeEventType(kind database.PrintJobKind) string {
	switch kind {
	case database.PrintJobKindBill:
		return "print.bill_available"
	case database.PrintJobKindReceipt:
		return "print.receipt_available"
	default:
		return "print.kitchen_available"
	}
}

func printWakePayload(job *database.PrintJob) map[string]interface{} {
	return map[string]interface{}{
		"job_id":     job.ID,
		"printer_id": *job.PrinterID,
		"kind":       job.Kind,
	}
}

func publishPrintWake(job *database.PrintJob) {
	if job == nil || job.PrinterID == nil || job.Status != database.PrintJobStatusRouted {
		return
	}
	events.GetHub().PublishJSON(job.BusinessID, printWakeEventType(job.Kind), printWakePayload(job))
}
