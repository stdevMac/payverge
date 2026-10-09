package print

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/stdevmac/payverge/backend/internal/database"
)

// The printed receipt for a bill with an AUTHORIZED fiscal receipt must carry
// the fiscal block (CAE, número with punto de venta, QR, RG 5614 amounts).
func TestBuildReceiptInputAttachesAuthorizedFiscalReceipt(t *testing.T) {
	db := setupTestDB(t)
	if err := db.AutoMigrate(&database.FiscalReceipt{}, &database.BusinessFiscalSettings{}); err != nil {
		t.Fatal(err)
	}
	business := mustCreateBusiness(t, db)
	bill := mustCreateBill(t, db, business.ID)

	pos := 3
	if err := db.Create(&database.BusinessFiscalSettings{
		BusinessID:   business.ID,
		Country:      "AR",
		Provider:     "arca",
		TaxID:        "30123456789",
		TaxCondition: "responsable_inscripto",
		PointOfSale:  &pos,
	}).Error; err != nil {
		t.Fatal(err)
	}

	num := "00000042"
	cae := "71234567890123"
	qr := "https://www.arca.gob.ar/fe/qr/?p=eyJ0ZXN0IjoxfQ=="
	exp := time.Date(2026, 8, 20, 0, 0, 0, 0, time.UTC)
	if err := db.Create(&database.FiscalReceipt{
		BusinessID:       business.ID,
		SettingsID:       1,
		BillID:           bill.ID,
		Country:          "AR",
		Provider:         "arca",
		Action:           "issue",
		ReceiptType:      "factura_b",
		ReceiptNumber:    &num,
		AuthCode:         &cae,
		AuthExpiresAt:    &exp,
		QRPayload:        &qr,
		TotalAmountCents: 121000,
		Currency:         "ARS",
		Status:           database.FiscalStatusAuthorized,
	}).Error; err != nil {
		t.Fatal(err)
	}

	in, err := buildReceiptInputFromBill(context.Background(), db, bill.ID, business.ID, "es")
	if err != nil {
		t.Fatal(err)
	}
	if in.Fiscal == nil {
		t.Fatal("expected fiscal block for a bill with an authorized fiscal receipt")
	}
	if in.Fiscal.ReceiptTitle != "FACTURA B" {
		t.Errorf("ReceiptTitle = %q", in.Fiscal.ReceiptTitle)
	}
	if in.Fiscal.ReceiptNumber != "0003-00000042" {
		t.Errorf("ReceiptNumber = %q (want punto de venta prefix)", in.Fiscal.ReceiptNumber)
	}
	if in.Fiscal.CAE != cae {
		t.Errorf("CAE = %q", in.Fiscal.CAE)
	}
	if in.Fiscal.CAEExpiry != "20/08/2026" {
		t.Errorf("CAEExpiry = %q", in.Fiscal.CAEExpiry)
	}
	if in.Fiscal.IVAContained != "ARS 210,00" {
		t.Errorf("IVAContained = %q (RG 5614: 21%% contained in 1.210,00)", in.Fiscal.IVAContained)
	}
	if !strings.HasPrefix(string(in.Fiscal.QRDataURI), "data:image/png;base64,") {
		t.Errorf("QRDataURI = %q", in.Fiscal.QRDataURI)
	}
}

// No authorized receipt (none, or only pending/rejected) → plain courtesy
// ticket, never an error.
func TestBuildReceiptInputWithoutAuthorizedReceiptIsCourtesy(t *testing.T) {
	db := setupTestDB(t)
	if err := db.AutoMigrate(&database.FiscalReceipt{}, &database.BusinessFiscalSettings{}); err != nil {
		t.Fatal(err)
	}
	business := mustCreateBusiness(t, db)
	bill := mustCreateBill(t, db, business.ID)

	if err := db.Create(&database.FiscalReceipt{
		BusinessID: business.ID, SettingsID: 1, BillID: bill.ID,
		Country: "AR", Provider: "arca", Action: "issue",
		ReceiptType: "factura_b", TotalAmountCents: 121000, Currency: "ARS",
		Status: database.FiscalStatusPending,
	}).Error; err != nil {
		t.Fatal(err)
	}

	in, err := buildReceiptInputFromBill(context.Background(), db, bill.ID, business.ID, "en")
	if err != nil {
		t.Fatal(err)
	}
	if in.Fiscal != nil {
		t.Fatal("pending receipt must not produce a fiscal block")
	}
}
