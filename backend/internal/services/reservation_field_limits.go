package services

import (
	"errors"
	"fmt"
	"unicode"
	"unicode/utf8"
)

// Guest-typed reservation fields are bounded so a booking cannot be used to
// park a paragraph of attacker text on a venue's records (and, before the
// sender scrub, in emails to arbitrary addresses). Limits count runes, not
// bytes, so non-Latin names get the same room as Latin ones.
const (
	MaxReservationCustomerNameRunes    = 80
	MaxReservationCustomerPhoneRunes   = 32
	MaxReservationSpecialRequestsRunes = 300
)

// Reasons carried by ReservationFieldError.
const (
	ReservationFieldReasonRequired = "required"
	ReservationFieldReasonTooLong  = "too_long"
	ReservationFieldReasonInvalid  = "invalid_characters"
)

// ErrReservationFieldInvalid is the sentinel every ReservationFieldError
// unwraps to.
var ErrReservationFieldInvalid = errors.New("invalid reservation field")

// ReservationFieldError names the offending field (its JSON name) and why it
// was refused. Max is set for ReservationFieldReasonTooLong.
type ReservationFieldError struct {
	Field  string
	Reason string
	Max    int
}

func (e *ReservationFieldError) Error() string {
	switch e.Reason {
	case ReservationFieldReasonTooLong:
		return fmt.Sprintf("%s must be at most %d characters", e.Field, e.Max)
	case ReservationFieldReasonRequired:
		return fmt.Sprintf("%s is required", e.Field)
	default:
		return fmt.Sprintf("%s contains characters that are not allowed", e.Field)
	}
}

func (e *ReservationFieldError) Unwrap() error { return ErrReservationFieldInvalid }

// validateReservationCustomerName enforces the name rules on an already
// trimmed value: non-empty, at most 80 runes, single line, no control or
// bidi-override characters.
func validateReservationCustomerName(name string) error {
	if name == "" {
		return &ReservationFieldError{Field: "customer_name", Reason: ReservationFieldReasonRequired}
	}
	return validateReservationText("customer_name", name, MaxReservationCustomerNameRunes, false)
}

// validateReservationCustomerPhone enforces at most 32 runes, single line, no
// control characters. Empty is allowed (phone is optional).
func validateReservationCustomerPhone(phone string) error {
	return validateReservationText("customer_phone", phone, MaxReservationCustomerPhoneRunes, false)
}

// validateReservationSpecialRequests allows line breaks and tabs but no other
// control characters, at most 300 runes. Empty is allowed.
func validateReservationSpecialRequests(requests string) error {
	return validateReservationText("special_requests", requests, MaxReservationSpecialRequestsRunes, true)
}

func validateReservationText(field, value string, maxRunes int, multiline bool) error {
	if !utf8.ValidString(value) {
		return &ReservationFieldError{Field: field, Reason: ReservationFieldReasonInvalid}
	}
	for _, r := range value {
		if multiline && (r == '\n' || r == '\r' || r == '\t') {
			continue
		}
		if unicode.IsControl(r) || isBidiControl(r) {
			return &ReservationFieldError{Field: field, Reason: ReservationFieldReasonInvalid}
		}
	}
	if utf8.RuneCountInString(value) > maxRunes {
		return &ReservationFieldError{Field: field, Reason: ReservationFieldReasonTooLong, Max: maxRunes}
	}
	return nil
}

// isBidiControl reports the explicit embedding/override/isolate controls,
// which can visually reorder text (e.g. hide a link's real target). Plain
// LRM/RLM marks stay allowed: right-to-left names legitimately use them.
func isBidiControl(r rune) bool {
	return (r >= '‪' && r <= '‮') || (r >= '⁦' && r <= '⁩')
}

// validateCreateReservationFields checks the trimmed guest fields of a new
// booking (staff-created and online alike).
func validateCreateReservationFields(name, phone, requests string) error {
	if err := validateReservationCustomerName(name); err != nil {
		return err
	}
	if err := validateReservationCustomerPhone(phone); err != nil {
		return err
	}
	return validateReservationSpecialRequests(requests)
}
