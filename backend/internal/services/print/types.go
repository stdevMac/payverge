package print

import (
	"errors"

	"github.com/stdevmac/payverge/backend/internal/database"
)

// EnqueueParams is the input to print.Service.Enqueue.
type EnqueueParams struct {
	BusinessID uint
	LocationID *uint
	Kind       database.PrintJobKind
	SourceType string
	SourceID   uint
	Language   string
	CreatedBy  string
	// OrderID lets the IMP-04 self-healing sweep correlate kitchen tickets
	// back to their order. Set to nil for non-order print jobs.
	OrderID *uint
}

// Validate returns nil when the params describe a queuable job.
func (p EnqueueParams) Validate() error {
	if p.BusinessID == 0 {
		return errors.New("business_id is required")
	}
	if !isRenderableKind(p.Kind) {
		return errors.New("invalid or unrenderable kind")
	}
	if p.SourceType == "" {
		return errors.New("source_type is required")
	}
	if p.SourceID == 0 {
		return errors.New("source_id is required")
	}
	if !sourceTypeMatchesKind(p.Kind, p.SourceType) {
		return errors.New("source_type does not match print job kind")
	}
	return nil
}

func sourceTypeMatchesKind(kind database.PrintJobKind, sourceType string) bool {
	switch kind {
	case database.PrintJobKindBill, database.PrintJobKindReceipt:
		// "reprint" is internal provenance for a deliberate copy. SourceID still
		// references the bill used by the formatter.
		return sourceType == "bill" || sourceType == "reprint"
	case database.PrintJobKindKitchen, database.PrintJobKindBar:
		return sourceType == "order"
	default:
		return false
	}
}

// isRenderableKind is the allowlist of print kinds that renderHTML can actually
// produce. PrintJobKind.IsValid() still accepts void/modify for forward-compat,
// but a job whose HTML cannot be rendered must never be queued — otherwise it
// inserts, routes, fails render, and burns the full retry budget into a
// dead-letter. Add a kind here only when its formatter ships.
func isRenderableKind(k database.PrintJobKind) bool {
	switch k {
	case database.PrintJobKindBill, database.PrintJobKindReceipt,
		database.PrintJobKindKitchen, database.PrintJobKindBar:
		return true
	}
	return false
}
