package services

import (
	"fmt"
	"strings"
	"time"

	"github.com/stdevmac/payverge/backend/internal/database"
)

// BroadcastRecipientSampleSize is how many deliverable addresses the admin
// compose/confirm UI shows as a preview sample before send.
const BroadcastRecipientSampleSize = 5

// nonDeliverableTLDs are reserved / special-use domains (RFC 2606 and friends)
// that hard-bounce on real MTAs. Mailing them damages Resend sender reputation.
// Match is on the full domain suffix after the final '@'.
var nonDeliverableTLDs = []string{
	".test",
	".local",
	".invalid",
	".example",
}

// IsDeliverableEmail reports whether an address is safe to include in a
// platform broadcast. Empty, malformed, and reserved-domain addresses are not.
// This is the domain-level guard every broadcast path must share.
func IsDeliverableEmail(email string) bool {
	email = strings.TrimSpace(strings.ToLower(email))
	if email == "" {
		return false
	}
	at := strings.LastIndex(email, "@")
	if at <= 0 || at == len(email)-1 {
		return false
	}
	domain := email[at+1:]
	if domain == "" || strings.Contains(domain, " ") {
		return false
	}
	// Bare reserved labels (e.g. "example") and any subdomain of them.
	for _, tld := range nonDeliverableTLDs {
		label := strings.TrimPrefix(tld, ".")
		if domain == label || strings.HasSuffix(domain, tld) {
			return false
		}
	}
	return true
}

// ResolveBroadcastRecipients resolves admin broadcast recipient scopes to a
// map of deliverable email → display name. Scopes:
//
//   - "all_businesses"        — kind=real businesses with deliverable email
//   - "all_active_businesses" — same, limited to businesses with a paid bill
//     in the last 30 days
//   - any other string        — treated as a literal address (still domain-guarded)
//
// kind != real and non-deliverable domains are always excluded so every future
// caller inherits the same reputation-safe filter (Task 13).
func ResolveBroadcastRecipients(recipients []string) (map[string]string, error) {
	emailMap := make(map[string]string)

	for _, recipient := range recipients {
		recipient = strings.TrimSpace(recipient)
		if recipient == "" {
			continue
		}
		switch recipient {
		case "all_businesses":
			rows, err := listRealBusinessesWithEmail()
			if err != nil {
				return nil, err
			}
			for _, b := range rows {
				if IsDeliverableEmail(b.Email) {
					emailMap[strings.TrimSpace(b.Email)] = displayName(b)
				}
			}
		case "all_active_businesses":
			rows, err := listRealBusinessesWithEmail()
			if err != nil {
				return nil, err
			}
			cutoff := time.Now().AddDate(0, 0, -30)
			db := database.GetDB()
			if db == nil {
				return nil, fmt.Errorf("database not available")
			}
			for _, b := range rows {
				if !IsDeliverableEmail(b.Email) {
					continue
				}
				var orderCount int64
				if err := db.Model(&database.Bill{}).
					Where("business_id = ? AND status = ? AND created_at > ?",
						b.ID, database.BillStatusPaid, cutoff).
					Count(&orderCount).Error; err != nil {
					return nil, fmt.Errorf("failed to check activity for business %d: %w", b.ID, err)
				}
				if orderCount > 0 {
					emailMap[strings.TrimSpace(b.Email)] = displayName(b)
				}
			}
		default:
			// Literal address — still domain-guarded so free-form sends cannot
			// reintroduce reserved TLDs.
			if IsDeliverableEmail(recipient) {
				emailMap[strings.TrimSpace(strings.ToLower(recipient))] = "User"
			}
		}
	}

	return emailMap, nil
}

// BroadcastableBusiness is a kind=real business with a deliverable email,
// suitable for the admin compose/confirm preview list.
type BroadcastableBusiness struct {
	ID        uint   `json:"id"`
	Name      string `json:"name"`
	OwnerName string `json:"owner_name"`
	Email     string `json:"email"`
	Kind      string `json:"kind"`
}

// ListBroadcastableBusinessEmails returns kind=real businesses that have a
// deliverable email address, the deliverable count, and a short sample for the
// admin compose/confirm UI. Demo/test fixtures and reserved domains never appear.
func ListBroadcastableBusinessEmails() (rows []BroadcastableBusiness, total int64, sample []string, err error) {
	businesses, err := listRealBusinessesWithEmail()
	if err != nil {
		return nil, 0, nil, err
	}

	rows = make([]BroadcastableBusiness, 0, len(businesses))
	sample = make([]string, 0, BroadcastRecipientSampleSize)
	for _, b := range businesses {
		if !IsDeliverableEmail(b.Email) {
			continue
		}
		email := strings.TrimSpace(b.Email)
		rows = append(rows, BroadcastableBusiness{
			ID:        b.ID,
			Name:      b.Name,
			OwnerName: b.OwnerName,
			Email:     email,
			Kind:      string(b.Kind),
		})
		if len(sample) < BroadcastRecipientSampleSize {
			sample = append(sample, email)
		}
	}
	return rows, int64(len(rows)), sample, nil
}

func listRealBusinessesWithEmail() ([]database.Business, error) {
	businesses, _, err := database.ListBusinessesForAdmin(database.AdminBusinessFilter{
		Kind:  string(database.BusinessKindReal),
		Page:  1,
		Limit: 10000,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to fetch businesses: %w", err)
	}
	// Drop blank emails early; domain filtering happens at the caller.
	out := make([]database.Business, 0, len(businesses))
	for _, b := range businesses {
		if strings.TrimSpace(b.Email) == "" {
			continue
		}
		out = append(out, b)
	}
	return out, nil
}

func displayName(b database.Business) string {
	if name := strings.TrimSpace(b.OwnerName); name != "" {
		return name
	}
	if name := strings.TrimSpace(b.Name); name != "" {
		return name
	}
	return "User"
}
