package handlers

import (
	"github.com/gin-gonic/gin"

	"github.com/stdevmac/payverge/backend/internal/guestsession"
	"github.com/stdevmac/payverge/backend/internal/server"
)

// guestPayerSession returns the guest session fingerprint to stamp on a
// guest-initiated payment row (payer_guest_session), issuing
// the pv_guest_session cookie when the browser has none yet. It is the payment
// proof the guest fiscal identity binding (M-545) checks. Nil when a staff
// member drives the flow from a POS device: that browser is not a diner's.
func guestPayerSession(c *gin.Context) *string {
	if c == nil || server.ExtractStaffIDFromContext(c) != nil {
		return nil
	}
	return guestsession.PayerFingerprint(c)
}
