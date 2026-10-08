package emails

import "strings"

// reservedRecipientTLDs are special-use suffixes no public mail exchanger
// serves (RFC 6761/6762 and common private-network names). Demo showrooms and
// local fixtures use them (demo+...@payverge.local), so a send to one is
// never a real delivery: handing it to Resend/Postmark/SMTP only burns quota
// and bounce reputation. ".test" is deliberately absent: test fixtures send
// through stub providers with it and must keep observing those sends.
var reservedRecipientTLDs = map[string]struct{}{
	"local":       {},
	"localhost":   {},
	"invalid":     {},
	"internal":    {},
	"lan":         {},
	"localdomain": {},
}

// isReservedRecipient reports whether addr is on a special-use domain.
func isReservedRecipient(addr string) bool {
	addr = strings.TrimSpace(addr)
	if i := strings.LastIndex(addr, "<"); i >= 0 {
		addr = strings.TrimSuffix(addr[i+1:], ">")
	}
	at := strings.LastIndex(addr, "@")
	if at < 0 || at == len(addr)-1 {
		return false
	}
	domain := strings.TrimSuffix(strings.ToLower(addr[at+1:]), ".")
	tld := domain
	if i := strings.LastIndex(domain, "."); i >= 0 {
		tld = domain[i+1:]
	}
	_, reserved := reservedRecipientTLDs[tld]
	return reserved
}

// IsReservedRecipient reports whether addr is on a special-use domain
// (.local, .test, .example, .invalid, ...). dispatch drops such recipients
// without sending, so callers that count sends can skip them up front.
func IsReservedRecipient(addr string) bool { return isReservedRecipient(addr) }

// withoutReservedRecipients returns to minus the special-use addresses and
// how many it dropped.
func withoutReservedRecipients(to []string) ([]string, int) {
	kept := make([]string, 0, len(to))
	dropped := 0
	for _, addr := range to {
		if isReservedRecipient(addr) {
			dropped++
			continue
		}
		kept = append(kept, addr)
	}
	return kept, dropped
}
