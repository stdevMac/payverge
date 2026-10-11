package server

import (
	"errors"
	"net/http"
	"net/mail"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/fiscal"
	"github.com/stdevmac/payverge/backend/internal/guestsession"

	"github.com/gin-gonic/gin"
)

const (
	// maxFiscalCustomerNameRunes is enough for a razón social / legal name and
	// blocks the 10KB TEXT dump from #546.
	maxFiscalCustomerNameRunes = 200
	// maxFiscalCustomerDocNumber is 11 CUIT digits plus ordinary separators
	// (20-11111111-2). Digit-only validation is not a length cap.
	maxFiscalCustomerDocNumber = 20
	maxFiscalCustomerEmail     = 254
)

// errGuestFiscalIdentityConflict is returned when a guest token tries to wipe
// or replace a fiscal identity it may not change: one an operator entered, or
// one another guest session set while holding at least as much payment proof.
var errGuestFiscalIdentityConflict = errors.New("Fiscal identity is already set")

// FiscalCustomerRequest is the wire shape for setting a bill's fiscal customer
// identity at checkout. Every field is optional at the API layer — the
// consumidor-final CF threshold check in backend/internal/fiscal/service.go
// (validateSynchronousIssue) enforces the above-threshold document requirement
// at issuance time, not here. On the operator PUT, blank fields clear the
// column to NULL (consumidor final / unidentified). Guest POST is bound to the
// guest session (see persistGuestFiscalCustomer).
type FiscalCustomerRequest struct {
	DocType      *string `json:"fiscal_customer_doc_type"`
	DocNumber    *string `json:"fiscal_customer_doc_number"`
	TaxCondition *string `json:"fiscal_customer_tax_condition"`
	Name         *string `json:"fiscal_customer_name"`
	// Email is where the guest wants the factura sent (fiscal delivery email
	// channel). Optional; blank clears to NULL like the other fields.
	Email *string `json:"fiscal_customer_email"`
}

// fiscalCustomerUpdate is the normalized, validated set of nullable column
// values produced from a FiscalCustomerRequest. The *Present flags record which
// keys the request actually supplied: a present key is written (blank clears the
// column to NULL on the operator PUT), an ABSENT key is left unchanged. `{}`
// is therefore a no-op. Guest POST additionally refuses to wipe or replace an
// identity owned by an operator or by a guest session with equal or stronger
// payment proof.
type fiscalCustomerUpdate struct {
	docType      *string
	docNumber    *string
	taxCondition *string
	name         *string
	email        *string

	docTypePresent      bool
	docNumberPresent    bool
	taxConditionPresent bool
	namePresent         bool
	emailPresent        bool
}

// hasAny reports whether the request supplied at least one fiscal field.
func (u fiscalCustomerUpdate) hasAny() bool {
	return u.docTypePresent || u.docNumberPresent || u.taxConditionPresent || u.namePresent || u.emailPresent
}

// validTaxConditions enumerates the AR fiscal recipient tax conditions the
// policy understands. Anything else is rejected at the API boundary.
var validTaxConditions = map[string]struct{}{
	"consumidor_final":      {},
	"responsable_inscripto": {},
	"monotributo":           {},
	"exento":                {},
}

// trimToNil trims surrounding whitespace and returns nil for an empty result so
// blank inputs persist as NULL rather than empty strings.
func trimToNil(s *string) *string {
	if s == nil {
		return nil
	}
	t := strings.TrimSpace(*s)
	if t == "" {
		return nil
	}
	return &t
}

// countDigits returns the number of ASCII digits in s, ignoring separators such
// as hyphens or dots. This mirrors the worker's mapper (digitString), which
// strips non-digits before validating CUIT/DNI length.
func countDigits(s string) int {
	n := 0
	for _, r := range s {
		if r >= '0' && r <= '9' {
			n++
		}
	}
	return n
}

// validateFiscalCustomerRequest normalizes and validates the request, returning
// a fiscalCustomerUpdate ready to persist or a human-readable error message
// (caller maps to 400). Validation rules:
//   - NUL and other control characters, and bidi embedding/override/isolate
//     controls, in any string field are 400 (NUL alone would be a Postgres 500)
//   - name / doc number / email are length-capped
//   - doc type, if present, must be one of CUIT/CUIL/DNI (case-insensitive)
//   - CUIT/CUIL doc numbers must carry exactly 11 digits and pass mod-11
//   - DNI doc numbers must carry 7-8 digits
//   - a doc number requires a doc type (otherwise digit shape is ambiguous)
//   - tax condition, if present, must be a recognized AR condition
func validateFiscalCustomerRequest(req FiscalCustomerRequest) (fiscalCustomerUpdate, string) {
	if msg := rejectUnsafeFiscalFields(req); msg != "" {
		return fiscalCustomerUpdate{}, msg
	}

	out := fiscalCustomerUpdate{
		docNumber: trimToNil(req.DocNumber),
		name:      trimToNil(req.Name),

		docTypePresent:      req.DocType != nil,
		docNumberPresent:    req.DocNumber != nil,
		taxConditionPresent: req.TaxCondition != nil,
		namePresent:         req.Name != nil,
		emailPresent:        req.Email != nil,
	}

	if out.name != nil && utf8.RuneCountInString(*out.name) > maxFiscalCustomerNameRunes {
		return fiscalCustomerUpdate{}, "Fiscal customer name is too long"
	}
	if out.docNumber != nil && utf8.RuneCountInString(*out.docNumber) > maxFiscalCustomerDocNumber {
		return fiscalCustomerUpdate{}, "Fiscal customer document number is too long"
	}

	// Doc type: normalize known types to a canonical upper-case form; reject
	// anything unrecognized.
	if dt := trimToNil(req.DocType); dt != nil {
		canonical := strings.ToUpper(*dt)
		switch canonical {
		case "CUIT", "CUIL", "DNI":
			out.docType = &canonical
		default:
			return fiscalCustomerUpdate{}, "Invalid fiscal customer document type (expected CUIT, CUIL, or DNI)"
		}
	}

	// Doc number digit-shape validation, gated on doc type.
	if out.docNumber != nil {
		if out.docType == nil {
			return fiscalCustomerUpdate{}, "A fiscal customer document type is required when a document number is provided"
		}
		digits := countDigits(*out.docNumber)
		switch *out.docType {
		case "CUIT", "CUIL":
			if digits != 11 {
				return fiscalCustomerUpdate{}, "CUIT/CUIL must contain exactly 11 digits"
			}
			if !fiscal.ValidateCUITMod11(*out.docNumber) {
				return fiscalCustomerUpdate{}, "Invalid CUIT/CUIL checksum"
			}
		case "DNI":
			if digits < 7 || digits > 8 {
				return fiscalCustomerUpdate{}, "DNI must contain 7 or 8 digits"
			}
		}
	}

	// Tax condition: normalize to lower-case and validate against the policy's
	// recognized set.
	if tc := trimToNil(req.TaxCondition); tc != nil {
		canonical := strings.ToLower(*tc)
		if _, ok := validTaxConditions[canonical]; !ok {
			return fiscalCustomerUpdate{}, "Invalid fiscal customer tax condition"
		}
		out.taxCondition = &canonical
	}

	// Email: syntactic validation only (RFC 5322 via net/mail); ownership is
	// not verifiable at checkout. 254 is the SMTP path length ceiling.
	if em := trimToNil(req.Email); em != nil {
		if len(*em) > maxFiscalCustomerEmail {
			return fiscalCustomerUpdate{}, "Fiscal customer email is too long"
		}
		addr, err := mail.ParseAddress(*em)
		if err != nil || addr.Address != *em {
			return fiscalCustomerUpdate{}, "Invalid fiscal customer email"
		}
		out.email = em
	}

	return out, ""
}

// rejectUnsafeFiscalFields refuses control characters (NUL included: Postgres
// TEXT rejects it as a 500, #546) and the explicit bidi embedding, override and
// isolate controls in every string field. The name and document are rendered
// into the receipt PDF that the fiscal delivery worker mails to the
// guest-supplied address, so they get the same character rules as guest-typed
// reservation text (services/reservation_field_limits.go). Plain LRM/RLM marks
// stay allowed for right-to-left legal names.
func rejectUnsafeFiscalFields(req FiscalCustomerRequest) string {
	for _, field := range []*string{req.DocType, req.DocNumber, req.TaxCondition, req.Name, req.Email} {
		if hasUnsafeFiscalRune(field) {
			return "Invalid character in fiscal customer field"
		}
	}
	return ""
}

func hasUnsafeFiscalRune(s *string) bool {
	if s == nil {
		return false
	}
	if !utf8.ValidString(*s) {
		return true
	}
	for _, r := range *s {
		if unicode.IsControl(r) || isFiscalBidiControl(r) {
			return true
		}
	}
	return false
}

// isFiscalBidiControl reports U+202A-U+202E and U+2066-U+2069, which can
// visually reorder the surrounding text.
func isFiscalBidiControl(r rune) bool {
	return (r >= '\u202A' && r <= '\u202E') || (r >= '\u2066' && r <= '\u2069')
}

func fiscalUpdateColumns(upd fiscalCustomerUpdate) map[string]any {
	// Only write columns whose key was supplied. An absent key leaves the column
	// untouched, so a partial/empty POST can never blank fields it didn't include.
	cols := map[string]any{}
	if upd.docTypePresent {
		cols["fiscal_customer_doc_type"] = upd.docType
	}
	if upd.docNumberPresent {
		cols["fiscal_customer_doc_number"] = upd.docNumber
	}
	if upd.taxConditionPresent {
		cols["fiscal_customer_tax_condition"] = upd.taxCondition
	}
	if upd.namePresent {
		cols["fiscal_customer_name"] = upd.name
	}
	if upd.emailPresent {
		cols["fiscal_customer_email"] = upd.email
	}
	return cols
}

// fiscalReplaceColumns is the full identity a guest override writes: supplied
// keys take their value and every other identity column is cleared, so no
// field of the replaced identity (for example its factura email) survives
// under the new guest's name.
func fiscalReplaceColumns(upd fiscalCustomerUpdate) map[string]any {
	cols := map[string]any{
		"fiscal_customer_doc_type":      nil,
		"fiscal_customer_doc_number":    nil,
		"fiscal_customer_tax_condition": nil,
		"fiscal_customer_name":          nil,
		"fiscal_customer_email":         nil,
	}
	for k, v := range fiscalUpdateColumns(upd) {
		cols[k] = v
	}
	return cols
}

func fiscalIdentityCaptureAllowed(country string) bool {
	return strings.EqualFold(strings.TrimSpace(country), "AR")
}

func rejectUnsupportedFiscalIdentityVenue(c *gin.Context, business *database.Business, upd fiscalCustomerUpdate) bool {
	if !upd.hasAny() {
		return false
	}
	if fiscalIdentityCaptureAllowed(business.Address.Country) {
		return false
	}
	c.JSON(http.StatusConflict, gin.H{"error": "Fiscal identity is not available for this venue"})
	return true
}

// persistFiscalCustomer writes the supplied fiscal-customer columns for the
// given bill, scoped by business so a foreign business cannot mutate the row
// even if the caller reaches this far. The doc number is customer PII; it is
// never logged. Operator PUT uses this path and may still wipe or overwrite.
// An operator write takes ownership of the identity: the guest-session setter
// is cleared, so guests can no longer replace it and issuance keeps it.
func persistFiscalCustomer(billID, businessID uint, upd fiscalCustomerUpdate) error {
	cols := fiscalUpdateColumns(upd)
	if len(cols) == 0 {
		return nil
	}
	cols["fiscal_customer_guest_session"] = nil
	return database.GetDB().Model(&database.Bill{}).
		Where("id = ? AND business_id = ?", billID, businessID).
		Updates(cols).Error
}

func fiscalStringSet(s *string) bool {
	return s != nil && strings.TrimSpace(*s) != ""
}

func fiscalIdentityStored(bill *database.Bill) bool {
	return fiscalStringSet(bill.FiscalCustomerDocType) ||
		fiscalStringSet(bill.FiscalCustomerDocNumber) ||
		fiscalStringSet(bill.FiscalCustomerTaxCondition) ||
		fiscalStringSet(bill.FiscalCustomerName) ||
		fiscalStringSet(bill.FiscalCustomerEmail)
}

func storedFiscalValue(s *string) string {
	if s == nil {
		return ""
	}
	return strings.TrimSpace(*s)
}

func incomingFiscalValue(s *string) string {
	if s == nil {
		return ""
	}
	return strings.TrimSpace(*s)
}

func sameFiscalField(stored *string, incoming *string, normalize func(string) string) bool {
	a := storedFiscalValue(stored)
	b := incomingFiscalValue(incoming)
	if normalize != nil {
		a = normalize(a)
		b = normalize(b)
	}
	return a == b
}

func normalizeFiscalDocNumber(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	if b.Len() > 0 {
		return b.String()
	}
	return strings.TrimSpace(s)
}

func guestFiscalIdentityConflict(bill *database.Bill, upd fiscalCustomerUpdate) bool {
	if !upd.hasAny() || !fiscalIdentityStored(bill) {
		return false
	}
	if upd.docTypePresent && !sameFiscalField(bill.FiscalCustomerDocType, upd.docType, strings.ToUpper) {
		return true
	}
	if upd.docNumberPresent && !sameFiscalField(bill.FiscalCustomerDocNumber, upd.docNumber, normalizeFiscalDocNumber) {
		return true
	}
	if upd.taxConditionPresent && !sameFiscalField(bill.FiscalCustomerTaxCondition, upd.taxCondition, strings.ToLower) {
		return true
	}
	if upd.namePresent && !sameFiscalField(bill.FiscalCustomerName, upd.name, nil) {
		return true
	}
	if upd.emailPresent && !sameFiscalField(bill.FiscalCustomerEmail, upd.email, strings.ToLower) {
		return true
	}
	return false
}

func loadBillFiscalIdentity(billID, businessID uint) (*database.Bill, error) {
	var stored database.Bill
	err := database.GetDB().Model(&database.Bill{}).
		Select(
			"id",
			"business_id",
			"fiscal_customer_doc_type",
			"fiscal_customer_doc_number",
			"fiscal_customer_tax_condition",
			"fiscal_customer_name",
			"fiscal_customer_email",
			"fiscal_customer_guest_session",
		).
		Where("id = ? AND business_id = ?", billID, businessID).
		Take(&stored).Error
	if err != nil {
		return nil, err
	}
	return &stored, nil
}

// persistGuestFiscalCustomer applies a guest-token write bound to the caller's
// guest session (callerSession is guestsession.Fingerprint of the
// pv_guest_session cookie). M-545: the table QR / bill token alone must not let
// one person squat the factura identity of the guest who pays.
//
//   - No identity stored: the write is accepted and the caller's session is
//     recorded as the setter.
//   - Same values as stored: idempotent no-op.
//   - Identity set by an operator (or before the binding existed): locked.
//   - Identity set by the caller's own session: the caller may correct it.
//   - Identity set by another guest session: the caller replaces it only with
//     strictly stronger payment proof on this bill (a pending tender/checkout
//     beats none; a confirmed payment beats pending). The replacement is the
//     caller's full identity; nothing of the old one is kept.
//
// Every write is a compare-and-swap on the state it was decided against, so a
// concurrent writer turns into a 409 instead of a lost update. Issuance
// additionally drops a guest-set identity whose setter never paid
// (database.ClearUnpaidGuestFiscalIdentity).
func persistGuestFiscalCustomer(bill *database.Bill, upd fiscalCustomerUpdate, callerSession string, now time.Time) error {
	if !upd.hasAny() {
		return nil
	}
	db := database.GetDB()
	if !fiscalIdentityStored(bill) {
		cols := fiscalUpdateColumns(upd)
		if len(cols) == 0 {
			return nil
		}
		cols["fiscal_customer_guest_session"] = nullableFingerprint(callerSession)
		res := db.Model(&database.Bill{}).
			Where("id = ? AND business_id = ?", bill.ID, bill.BusinessID).
			Where("fiscal_customer_doc_type IS NULL").
			Where("fiscal_customer_doc_number IS NULL").
			Where("fiscal_customer_tax_condition IS NULL").
			Where("fiscal_customer_name IS NULL").
			Where("fiscal_customer_email IS NULL").
			Updates(cols)
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return errGuestFiscalIdentityConflict
		}
		return nil
	}
	if !guestFiscalIdentityConflict(bill, upd) {
		// Idempotent match — skip the write so a no-op Updates cannot look like
		// a conflict via RowsAffected == 0.
		return nil
	}

	setter := storedFiscalValue(bill.FiscalCustomerGuestSession)
	if setter == "" || callerSession == "" {
		// Operator-entered (or pre-binding) identity: guests cannot replace it.
		return errGuestFiscalIdentityConflict
	}

	var cols map[string]any
	if setter == callerSession {
		cols = fiscalUpdateColumns(upd)
	} else {
		callerProof, err := database.GuestPaymentProofForSession(db, bill.ID, callerSession, now)
		if err != nil {
			return err
		}
		if callerProof == database.GuestPaymentProofNone {
			return errGuestFiscalIdentityConflict
		}
		setterProof, err := database.GuestPaymentProofForSession(db, bill.ID, setter, now)
		if err != nil {
			return err
		}
		if callerProof <= setterProof {
			return errGuestFiscalIdentityConflict
		}
		cols = fiscalReplaceColumns(upd)
		cols["fiscal_customer_guest_session"] = callerSession
	}
	if len(cols) == 0 {
		return nil
	}
	res := db.Model(&database.Bill{}).
		Where("id = ? AND business_id = ?", bill.ID, bill.BusinessID).
		Where("fiscal_customer_guest_session = ?", setter).
		Updates(cols)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return errGuestFiscalIdentityConflict
	}
	return nil
}

func nullableFingerprint(fp string) any {
	if strings.TrimSpace(fp) == "" {
		return nil
	}
	return fp
}

// SetBillFiscalCustomer is the operator-side handler: PUT /bills/:bill_id/fiscal-customer.
// It mirrors the auth of the sibling bill mutation routes (RequireBillBusinessAccess
// + bills:write) and additionally re-verifies business access here, exactly as
// UpdateBill does, so the bill is scoped to the caller's business (no IDOR).
func SetBillFiscalCustomer(c *gin.Context) {
	billID, err := strconv.ParseUint(c.Param("bill_id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid bill ID"})
		return
	}

	var req FiscalCustomerRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		RespondBindError(c, err)
		return
	}

	upd, validationErr := validateFiscalCustomerRequest(req)
	if validationErr != "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": validationErr})
		return
	}

	bill, items, err := database.GetBillByID(uint(billID))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Bill not found"})
		return
	}

	business, err := database.GetBusinessByID(bill.BusinessID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Business not found"})
		return
	}

	if !CheckBusinessAccess(c, business) {
		c.JSON(http.StatusForbidden, gin.H{"error": "Not authorized to modify this bill"})
		return
	}

	if rejectUnsupportedFiscalIdentityVenue(c, business, upd) {
		return
	}

	if err := persistFiscalCustomer(bill.ID, bill.BusinessID, upd); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update fiscal customer"})
		return
	}

	// Reflect the persisted identity back on the returned bill so the FE can
	// render the saved values without a refetch. Only overwrite fields the request
	// actually supplied; absent fields keep the values already loaded on `bill`,
	// matching what persistFiscalCustomer wrote.
	if upd.docTypePresent {
		bill.FiscalCustomerDocType = upd.docType
	}
	if upd.docNumberPresent {
		bill.FiscalCustomerDocNumber = upd.docNumber
	}
	if upd.taxConditionPresent {
		bill.FiscalCustomerTaxCondition = upd.taxCondition
	}
	if upd.namePresent {
		bill.FiscalCustomerName = upd.name
	}
	if upd.emailPresent {
		bill.FiscalCustomerEmail = upd.email
	}

	c.JSON(http.StatusOK, gin.H{
		"bill":  bill,
		"items": items,
	})
}

// SetBillFiscalCustomerByNumber is the guest-side handler:
// POST /guest/bill/:bill_token/fiscal-customer. Guests set their own fiscal
// identity at checkout (before payment), keyed by public_token. Scoped to the
// resolved bill's business by construction; no business override is accepted.
// The write is bound to the caller's guest session cookie (issued here when
// missing), which guest payment initiation stamps on the payment rows.
func SetBillFiscalCustomerByNumber(c *gin.Context) {
	token := strings.TrimSpace(c.Param("bill_token"))
	if token == "" {
		respondPublicGuestBillLookupError(c, errPublicGuestBillNotFound)
		return
	}

	var req FiscalCustomerRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		RespondBindError(c, err)
		return
	}

	upd, validationErr := validateFiscalCustomerRequest(req)
	if validationErr != "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": validationErr})
		return
	}

	bill, _, err := loadPublicGuestBillByToken(token)
	if err != nil {
		respondPublicGuestBillLookupError(c, err)
		return
	}

	// Only allow setting fiscal identity before settlement. A paid bill may
	// already have an issuance job queued; allowing a bearer capability to edit
	// it in that window can make the official receipt disagree with checkout.
	if bill.Status != database.BillStatusOpen && bill.Status != database.BillStatusPartial {
		c.JSON(http.StatusConflict, gin.H{"error": "Cannot modify fiscal identity after settlement"})
		return
	}

	business, err := database.GetBusinessByID(bill.BusinessID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Business not found"})
		return
	}
	if rejectUnsupportedFiscalIdentityVenue(c, business, upd) {
		return
	}

	if !upd.hasAny() {
		c.JSON(http.StatusOK, gin.H{"success": true})
		return
	}

	// Public bill projection omits fiscal columns; reload them (and the setter
	// session) so ownership is evaluated against the stored identity, not a
	// zero-value bill.
	stored, err := loadBillFiscalIdentity(bill.ID, bill.BusinessID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update fiscal customer"})
		return
	}

	callerSession := guestsession.Fingerprint(guestsession.GetOrIssue(c))
	if err := persistGuestFiscalCustomer(stored, upd, callerSession, time.Now()); err != nil {
		if errors.Is(err, errGuestFiscalIdentityConflict) {
			c.JSON(http.StatusConflict, gin.H{"error": errGuestFiscalIdentityConflict.Error()})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update fiscal customer"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true})
}
