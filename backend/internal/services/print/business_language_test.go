package print

import (
	"context"
	"testing"

	"github.com/stdevmac/payverge/backend/internal/database"
)

func TestBusinessPrintLanguage_ResolvesAndNormalizes(t *testing.T) {
	db := setupTestDB(t)
	business := mustCreateBusiness(t, db)

	// Empty default_language → en.
	if got := BusinessPrintLanguage(db, business.ID); got != "en" {
		t.Fatalf("empty default_language: want en, got %q", got)
	}

	// Business default es-AR → canonical es-AR.
	if err := db.Model(&database.Business{}).Where("id = ?", business.ID).
		Update("default_language", "es-AR").Error; err != nil {
		t.Fatal(err)
	}
	if got := BusinessPrintLanguage(db, business.ID); got != "es-AR" {
		t.Fatalf("es-AR default: want es-AR, got %q", got)
	}

	// A guest locale with no label bundle collapses to en.
	if err := db.Model(&database.Business{}).Where("id = ?", business.ID).
		Update("default_language", "pt").Error; err != nil {
		t.Fatal(err)
	}
	if got := BusinessPrintLanguage(db, business.ID); got != "en" {
		t.Fatalf("pt default: want en, got %q", got)
	}

	// Missing business / nil db fall back to en.
	if got := BusinessPrintLanguage(db, 999999); got != "en" {
		t.Fatalf("missing business: want en, got %q", got)
	}
	if got := BusinessPrintLanguage(nil, business.ID); got != "en" {
		t.Fatalf("nil db: want en, got %q", got)
	}
}

func TestEnqueue_NormalizesJobLanguage(t *testing.T) {
	db := setupTestDB(t)
	business := mustCreateBusiness(t, db)
	bill := mustCreateBill(t, db, business.ID)

	svc := NewService(db)
	// Operator tier spells Rioplatense "es-ar"; the stored job language must be
	// the canonical label-bundle tag "es-AR".
	job, err := svc.Enqueue(context.Background(), EnqueueParams{
		BusinessID: business.ID,
		Kind:       database.PrintJobKindBill,
		SourceType: "bill",
		SourceID:   bill.ID,
		Language:   "es-ar",
		CreatedBy:  "test",
	})
	if err != nil {
		t.Fatal(err)
	}
	if job.Language != "es-AR" {
		t.Fatalf("want canonical es-AR, got %q", job.Language)
	}
}
