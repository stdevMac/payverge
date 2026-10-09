package print

import (
	"testing"

	"github.com/stdevmac/payverge/backend/internal/database"
)

func TestEnqueueParams_Validate_RejectsUnrenderableKinds(t *testing.T) {
	base := EnqueueParams{BusinessID: 1, SourceType: "bill", SourceID: 5}
	for _, k := range []database.PrintJobKind{database.PrintJobKindVoid, database.PrintJobKindModify} {
		p := base
		p.Kind = k
		if err := p.Validate(); err == nil {
			t.Fatalf("kind %q must be rejected at enqueue (no formatter exists)", k)
		}
	}
}

func TestEnqueueParams_Validate_AllowsRenderableKinds(t *testing.T) {
	for _, k := range []database.PrintJobKind{
		database.PrintJobKindBill, database.PrintJobKindReceipt,
		database.PrintJobKindKitchen, database.PrintJobKindBar,
	} {
		sourceType := "bill"
		if k == database.PrintJobKindKitchen || k == database.PrintJobKindBar {
			sourceType = "order"
		}
		p := EnqueueParams{BusinessID: 1, Kind: k, SourceType: sourceType, SourceID: 5}
		if err := p.Validate(); err != nil {
			t.Fatalf("kind %q must be queuable: %v", k, err)
		}
	}
}

func TestEnqueueParams_Validate_RejectsKindSourceMismatch(t *testing.T) {
	for _, p := range []EnqueueParams{
		{BusinessID: 1, Kind: database.PrintJobKindBill, SourceType: "order", SourceID: 5},
		{BusinessID: 1, Kind: database.PrintJobKindReceipt, SourceType: "order", SourceID: 5},
		{BusinessID: 1, Kind: database.PrintJobKindKitchen, SourceType: "bill", SourceID: 5},
		{BusinessID: 1, Kind: database.PrintJobKindBar, SourceType: "bill", SourceID: 5},
	} {
		if err := p.Validate(); err == nil {
			t.Fatalf("kind %q must reject source_type %q", p.Kind, p.SourceType)
		}
	}
}

func TestEnqueueParams_Validate(t *testing.T) {
	valid := EnqueueParams{
		BusinessID: 1,
		Kind:       database.PrintJobKindBill,
		SourceType: "bill",
		SourceID:   42,
		Language:   "en",
	}
	if err := valid.Validate(); err != nil {
		t.Fatalf("expected nil, got %v", err)
	}

	bad := valid
	bad.Kind = database.PrintJobKind("garbage")
	if err := bad.Validate(); err == nil {
		t.Fatal("expected error for invalid kind")
	}

	bad = valid
	bad.SourceType = ""
	if err := bad.Validate(); err == nil {
		t.Fatal("expected error for empty source_type")
	}

	bad = valid
	bad.SourceID = 0
	if err := bad.Validate(); err == nil {
		t.Fatal("expected error for zero source_id")
	}
}
