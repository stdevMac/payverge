package crm

import (
	"gorm.io/gorm"

	"github.com/stdevmac/payverge/backend/internal/database"
)

// preloadCustomerConsent is the shared preload for the consent flag. It selects
// only the columns needed to evaluate consent (plus the FK GORM needs to join),
// so list pages pay one extra batched query — never per-row lookups.
func preloadCustomerConsent(q *gorm.DB) *gorm.DB {
	return q.Select("id", "customer_id", "share_data_with_businesses")
}

// customerConsentsToShare reports whether the customer's global preferences
// allow sharing PII with businesses. Fail-closed: a missing customer or
// missing preferences row means no consent (opt-in required).
func customerConsentsToShare(c *database.Customer) bool {
	if c == nil || c.Preferences == nil {
		return false
	}
	return c.Preferences.ShareDataWithBusinesses
}

// sanitizeCustomerForOperator is the single consent chokepoint for every
// operator-facing CRM read (list, detail, add/connect, export). It:
//   - ALWAYS strips Customer.Preferences (guest-global settings are never an
//     operator concern), and
//   - when the guest has opted out of cross-business sharing, redacts the
//     globally-collected PII (phone, birthday, wallet, avatar, last login)
//     while keeping email + name (the linkage keys the operator already holds)
//     and every business-owned CustomerBusiness field (notes, tags, loyalty,
//     visit stats) untouched.
func sanitizeCustomerForOperator(cb *database.CustomerBusiness) {
	if cb == nil {
		return
	}
	consent := customerConsentsToShare(&cb.Customer)
	cb.Customer.Preferences = nil
	if consent {
		return
	}
	cb.Customer.Phone = ""
	cb.Customer.Birthday = nil
	cb.Customer.WalletAddress = ""
	cb.Customer.ProfileImageURL = ""
	cb.Customer.LastLoginAt = nil
}
