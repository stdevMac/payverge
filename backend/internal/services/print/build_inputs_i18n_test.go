package print

import (
	"context"
	"testing"
	"time"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/services/print/formatters"
)

// The receipt input builder must hand the formatter LOCALIZED payment-method
// labels and locale-formatted dates — the formatter renders these fields
// verbatim.
func TestBuildReceiptInput_LocalizesMethodAndDate(t *testing.T) {
	db := setupTestDB(t)
	business := mustCreateBusiness(t, db)
	bill := mustCreateBill(t, db, business.ID)

	now := time.Now()
	payment := database.Payment{
		BillID:        bill.ID,
		PayerAddr:     "0xpayer",
		Amount:        1100,
		TxHash:        "0xhash-" + t.Name(),
		Status:        database.PaymentStatusConfirmed,
		PaymentMethod: "cash",
		ConfirmedAt:   &now,
	}
	if err := db.Create(&payment).Error; err != nil {
		t.Fatal(err)
	}

	in, err := buildReceiptInputFromBill(context.Background(), db, bill.ID, business.ID, "es")
	if err != nil {
		t.Fatal(err)
	}
	if in.PaymentMethod != "Efectivo" {
		t.Fatalf("PaymentMethod: want localized \"Efectivo\", got %q", in.PaymentMethod)
	}
	wantDate := formatters.FormatTicketTime("es", bill.CreatedAt)
	if in.CreatedAt != wantDate {
		t.Fatalf("CreatedAt: want %q, got %q", wantDate, in.CreatedAt)
	}
}

func TestBuildBillInput_EnglishDateLayoutUnchanged(t *testing.T) {
	db := setupTestDB(t)
	business := mustCreateBusiness(t, db)
	bill := mustCreateBill(t, db, business.ID)

	in, err := buildBillInputFromBill(context.Background(), db, bill.ID, business.ID, "en")
	if err != nil {
		t.Fatal(err)
	}
	if want := bill.CreatedAt.Format("2006-01-02 15:04"); in.CreatedAt != want {
		t.Fatalf("en CreatedAt layout regressed: want %q, got %q", want, in.CreatedAt)
	}
}
