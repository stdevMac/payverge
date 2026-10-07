package demo

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/stdevmac/payverge/backend/internal/database"
)

// Demo fiscal receipts must match the venue country: the AR demo venues seed
// AFIP Factura A/B/C with variety, and any non-AR venue would seed
// invoice/receipt (regression for issue #255 / L6-21 follow-up).
func TestDemoFiscalReceiptsMatchVenueCountry(t *testing.T) {
	db := newDemoServiceTestDB(t)
	admin := seedAdmin(t, db, "fiscal-rt-admin@example.com")
	now := fixedNow()
	svc := NewService(db, Options{Now: func() time.Time { return now }, SeedVersion: "fiscal-rt-seed", BaselineDays: 3})
	instance, err := svc.EnsureForAdmin(context.Background(), admin.ID)
	require.NoError(t, err)

	businessIDs := compactBusinessIDs(instance.PrimaryBusinessID, instance.SecondaryBusinessID)
	var receipts []database.FiscalReceipt
	require.NoError(t, db.Where("business_id IN ?", businessIDs).Find(&receipts).Error)
	require.NotEmpty(t, receipts, "demo should seed fiscal receipts")

	usAllowed := map[string]bool{"invoice": true, "receipt": true}
	arAllowed := map[string]bool{"factura_a": true, "factura_b": true, "factura_c": true}
	seenAR := map[string]int{}
	for _, r := range receipts {
		country := strings.ToUpper(strings.TrimSpace(r.Country))
		switch country {
		case "AR":
			require.Truef(t, arAllowed[r.ReceiptType],
				"AR demo ReceiptType %q must be factura_a/b/c, bill=%d", r.ReceiptType, r.BillID)
			seenAR[r.ReceiptType]++
		default:
			require.Truef(t, usAllowed[r.ReceiptType],
				"non-AR demo ReceiptType %q must be invoice/receipt (not AFIP letters), country=%s bill=%d",
				r.ReceiptType, r.Country, r.BillID)
			require.False(t, strings.HasPrefix(strings.ToLower(r.ReceiptType), "factura"),
				"non-AR venue must not seed Factura letters")
		}
	}
	require.GreaterOrEqual(t, len(seenAR), 2, "AR seed should vary the AFIP factura letters, got %v", seenAR)
}

func TestDemoReceiptTypeHelper(t *testing.T) {
	require.Equal(t, "invoice", demoReceiptType("US", 100))
	require.Equal(t, "receipt", demoReceiptType("US", 101))
	require.Equal(t, "factura_a", demoReceiptType("AR", 99)) // 99%3==0 → RI+CUIT
	require.Equal(t, "standard_invoice", demoReceiptType("AE", 1))
}
