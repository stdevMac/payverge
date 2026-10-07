// Package paidreceipt owns the non-blocking receipt side effect for a bill's
// transition to paid. Payment handlers call this operation instead of talking
// to the print queue directly.
package paidreceipt

import (
	"context"
	"fmt"
	"log"
	"strings"

	"gorm.io/gorm"

	"github.com/stdevmac/payverge/backend/internal/database"
	operationalalerts "github.com/stdevmac/payverge/backend/internal/services/operational_alerts"
	printsvc "github.com/stdevmac/payverge/backend/internal/services/print"
)

type State string

const (
	StateNotApplicable       State = "not_applicable"
	StateQueued              State = "queued"
	StatePendingPrinter      State = "pending_printer"
	StateManualPrintRequired State = "manual_print_required"
)

type Result struct {
	State              State
	Job                *database.PrintJob
	ManualRecoveryPath string
}

type Service struct {
	db *gorm.DB
}

func NewService(db *gorm.DB) *Service {
	return &Service{db: db}
}

// HandleBillPaid creates (or returns) the single final receipt job for a paid
// bill. Printer and queue failures are deliberately returned after recording
// an operator alert; callers must log them but must not roll back settlement.
func (s *Service) HandleBillPaid(ctx context.Context, bill *database.Bill, source string) (Result, error) {
	result := Result{State: StateNotApplicable}
	if bill == nil || bill.Status != database.BillStatusPaid {
		return result, nil
	}
	result.ManualRecoveryPath = fmt.Sprintf(
		"/api/v1/inside/businesses/%d/print/jobs", bill.BusinessID,
	)
	if s == nil || s.db == nil {
		result.State = StateManualPrintRequired
		return result, fmt.Errorf("paid receipt queue unavailable")
	}

	job, err := printsvc.NewService(s.db).Enqueue(ctx, printsvc.EnqueueParams{
		BusinessID: bill.BusinessID,
		Kind:       database.PrintJobKindReceipt,
		SourceType: "bill",
		SourceID:   bill.ID,
		Language:   printsvc.BusinessPrintLanguage(s.db, bill.BusinessID),
		CreatedBy:  "system",
	})
	result.Job = job
	if err != nil {
		result.State = StateManualPrintRequired
		s.raiseManualRecoveryAlert(ctx, bill, source, err)
		return result, err
	}

	switch job.Status {
	case database.PrintJobStatusPending:
		result.State = StatePendingPrinter
	case database.PrintJobStatusFailedRetryable, database.PrintJobStatusFailedPermanent:
		result.State = StateManualPrintRequired
		printErr := fmt.Errorf("receipt print job entered %s", job.Status)
		s.raiseManualRecoveryAlert(ctx, bill, source, printErr)
		return result, printErr
	default:
		result.State = StateQueued
	}
	return result, nil
}

func (s *Service) raiseManualRecoveryAlert(ctx context.Context, bill *database.Bill, source string, enqueueErr error) {
	if s == nil || s.db == nil || bill == nil {
		return
	}
	source = strings.TrimSpace(source)
	if source == "" {
		source = "paid_transition"
	}
	_, err := operationalalerts.NewService(s.db).UpsertAlert(ctx, operationalalerts.UpsertAlertInput{
		BusinessID:   bill.BusinessID,
		AlertType:    database.OperationalAlertTypePrintQueueStale,
		ResourceType: database.OperationalAlertResourceTypeBill,
		ResourceID:   bill.ID,
		Priority:     database.OperationalAlertPriorityHigh,
		Title:        "Paid receipt needs manual printing",
		Body: fmt.Sprintf(
			"Bill %s is paid, but its final receipt was not queued. Print it manually from the bill.",
			bill.BillNumber,
		),
		Metadata: map[string]any{
			"settlement_source": source,
			"manual_recovery":   fmt.Sprintf("/api/v1/inside/businesses/%d/print/jobs", bill.BusinessID),
			"queue_error":       enqueueErr.Error(),
		},
	})
	if err != nil {
		log.Printf("raise paid-receipt manual recovery alert for bill %d failed: %v", bill.ID, err)
	}
}
