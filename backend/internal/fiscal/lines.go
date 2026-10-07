package fiscal

import "github.com/stdevmac/payverge/backend/internal/fiscal/display"

// DefaultArgentinaVATRate is the standard IVA rate used in Argentina (21%).
const DefaultArgentinaVATRate = display.DefaultArgentinaVATRate

// SplitInclusiveVAT delegates to the shared display package; see
// display.SplitInclusiveVAT for the reconciliation invariant.
func SplitInclusiveVAT(totalCents int64, ratePct float64) (netCents, vatCents int64) {
	return display.SplitInclusiveVAT(totalCents, ratePct)
}
