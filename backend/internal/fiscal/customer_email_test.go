package fiscal

import (
	"context"
	"testing"

	"github.com/stdevmac/payverge/backend/internal/database"
)

// The guest-entered fiscal email must be the FIRST link in the recipient
// chain — ahead of the CRM customer and the delivery-order fallback — because
// it is the address the guest explicitly asked the factura to be sent to.
func TestCustomerEmailPrefersBillFiscalCustomerEmail(t *testing.T) {
	db := newFiscalTestDBWithCustomer(t)
	s := &Service{db: db}

	// Seed CRM customer 1 with a different email so we prove fiscal wins.
	crmEmail := "crm@example.com"
	cust := database.Customer{Email: crmEmail}
	if err := db.Create(&cust).Error; err != nil {
		t.Fatal(err)
	}
	email := "guest-factura@example.com"
	bill := database.Bill{CRMCustomerID: &cust.ID, FiscalCustomerEmail: &email}

	if got := s.customerEmail(context.Background(), bill); got != email {
		t.Fatalf("customerEmail = %q, want the bill's fiscal_customer_email %q", got, email)
	}
}

// Blank/whitespace fiscal email falls through to the existing chain.
func TestCustomerEmailBlankFiscalEmailFallsThrough(t *testing.T) {
	db := newFiscalTestDBWithCustomer(t)
	s := &Service{db: db}

	blank := "   "
	bill := database.Bill{FiscalCustomerEmail: &blank}
	if got := s.customerEmail(context.Background(), bill); got != "" {
		// no CRM id, no delivery order → chain resolves empty
		t.Fatalf("customerEmail = %q, want \"\"", got)
	}
}
