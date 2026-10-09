package services

import (
	"errors"
	"net/mail"
	"strings"
)

// ErrGuestReservationEmail is returned when a public booking email is missing
// or cannot receive mail. net/mail.ParseAddress is RFC 5322 only — it accepts
// single-label domains (john@gmail) and IP-literal hosts that no MTA will
// deliver to.
var ErrGuestReservationEmail = errors.New("a valid email address is required to book online")

// ParseGuestReservationEmail returns the bare mailbox for a guest booking.
// Staff/authenticated creates stay email-optional and must not use this gate.
func ParseGuestReservationEmail(raw string) (string, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", ErrGuestReservationEmail
	}
	addr, err := mail.ParseAddress(trimmed)
	if err != nil || addr == nil || strings.TrimSpace(addr.Address) == "" {
		return "", ErrGuestReservationEmail
	}
	mailbox := strings.TrimSpace(addr.Address)
	if strings.ContainsAny(mailbox, "\r\n") {
		return "", ErrGuestReservationEmail
	}
	at := strings.LastIndex(mailbox, "@")
	if at <= 0 || at == len(mailbox)-1 {
		return "", ErrGuestReservationEmail
	}
	local, domain := mailbox[:at], mailbox[at+1:]
	if local == "" || !guestReservationEmailDomainOK(domain) {
		return "", ErrGuestReservationEmail
	}
	return mailbox, nil
}

func guestReservationEmailDomainOK(domain string) bool {
	if domain == "" || strings.ContainsAny(domain, " \t") {
		return false
	}
	// user@[127.0.0.1] parses but is not a deliverable confirmation mailbox.
	if strings.HasPrefix(domain, "[") {
		return false
	}
	if !strings.Contains(domain, ".") {
		return false
	}
	if strings.HasPrefix(domain, ".") || strings.HasSuffix(domain, ".") || strings.Contains(domain, "..") {
		return false
	}
	return true
}
