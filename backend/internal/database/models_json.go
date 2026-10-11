package database

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

// =============================================================================
// JSON marshaling helpers: convert int64 cents → float64 dollars for API
// responses so the frontend contract remains in dollars.
// =============================================================================

// centsToDollars converts an int64 cent amount to a float64 dollar amount.
func centsToDollars(cents int64) float64 {
	return float64(cents) / 100.0
}

// DefaultDisplayCurrency is the last-resort currency label used when neither
// the row nor its business carries one. It is a display fallback only — a live
// venue that lands here is a data bug, not a USD venue (#852/#856).
const DefaultDisplayCurrency = "USD"

// BusinessDisplayCurrencySelectSQL is the SQL twin of BusinessDisplayCurrency:
// the same display → default precedence, evaluated in the database so projected
// bill reads can carry a venue's currency without preloading the Business
// aggregate. It resolves to the empty string (not 'USD') when the business has
// neither, so Go keeps ownership of the final fallback. Requires `businesses`
// to be joined.
const BusinessDisplayCurrencySelectSQL = "COALESCE(NULLIF(businesses.display_currency, ''), NULLIF(businesses.default_currency, ''), '')"

// BusinessDisplayCurrency resolves the guest/operator-facing currency label for
// a business using the canonical precedence: display_currency, then
// default_currency, then the USD fallback. Callers that shape a payload from a
// business already in hand should use this rather than re-implementing the
// order. Takes a pointer: Business is a wide struct and this runs per row on
// list responses.
func BusinessDisplayCurrency(business *Business) string {
	if business == nil {
		return DefaultDisplayCurrency
	}
	if business.DisplayCurrency != "" {
		return business.DisplayCurrency
	}
	if business.DefaultCurrency != "" {
		return business.DefaultCurrency
	}
	return DefaultDisplayCurrency
}

func resolveBusinessCurrency(explicit string, business Business) string {
	if explicit != "" {
		return explicit
	}
	return BusinessDisplayCurrency(&business)
}

// ResolvedCurrency returns the display currency for this order. Currency is
// not an orders column (gorm:"-"); it is resolved from an explicit value or
// the preloaded Business display/default currency, falling back to USD.
// Guest views that bypass Order.MarshalJSON must call this instead of
// copying the empty gorm:"-" field (#560).
func (o Order) ResolvedCurrency() string {
	return resolveBusinessCurrency(o.Currency, o.Business)
}

// ResolvedCurrency returns the currency a venue's money is denominated in:
// display_currency, then default_currency, then USD. Exported so surfaces that
// build their own money payloads outside MarshalJSON — CSV exports especially —
// name the venue's currency instead of leaving operators to assume dollars
// (#926). A row projected with only the two currency columns is enough.
func (b Business) ResolvedCurrency() string {
	return resolveBusinessCurrency("", b)
}

func resolvePaymentCurrency(payment Payment) string {
	if payment.Currency != "" {
		return payment.Currency
	}
	if payment.Bill.Currency != "" {
		return payment.Bill.Currency
	}
	return resolveBusinessCurrency("", payment.Bill.Business)
}

// billPublicBusiness is the safe subset of Business embedded in a bill payload.
// The full Business row carries owner/settlement/tipping wallet addresses, contact
// info, and internal settings that must never reach a bill consumer
// (operator OR guest). Internal Go callers read bill.Business.* struct fields
// directly and are unaffected — only JSON serialization passes through here. The
// bill emits its own payable settlement_address/tipping_address separately. (SEC-2)
type billPublicBusiness struct {
	ID              uint   `json:"id"`
	BusinessID      string `json:"business_id"`
	Name            string `json:"name"`
	Logo            string `json:"logo"`
	Timezone        string `json:"timezone"`
	DefaultCurrency string `json:"default_currency"`
	DisplayCurrency string `json:"display_currency"`
}

// newBillPublicBusiness projects a full Business row onto the safe summary that
// is allowed to reach bill/table consumers.
func newBillPublicBusiness(b Business) billPublicBusiness {
	return billPublicBusiness{
		ID:              b.ID,
		BusinessID:      b.BusinessId,
		Name:            b.Name,
		Logo:            b.Logo,
		Timezone:        b.Timezone,
		DefaultCurrency: b.DefaultCurrency,
		DisplayCurrency: b.DisplayCurrency,
	}
}

// MarshalJSON serializes Table while reducing its embedded Business to the safe
// summary. A Table embeds the full Business row for internal Go use (callers read
// table.Business.* directly); JSON consumers — including bill.table.business —
// must never receive the owner/settlement/Stripe fields, and an unloaded business
// is omitted rather than emitted empty. (SEC-2)
//
// Several list endpoints preload a table summary only:
//   - order-list bill.table: Select("id","name") (preloadOrderListTableName)
//   - reservation list: Select("id","name","table_code","capacity")
//     (preloadReservationTableSummary)
//
// A full Table MarshalJSON on those partial rows used to invent zero-value lies
// (is_active:false, empty QR defaults, business_id:0, zero timestamps) that look
// like real settings. Partial projections emit only the columns that were
// actually loaded. (FIND-040, FIND-046)
func (t Table) MarshalJSON() ([]byte, error) {
	// Partial projection heuristic: real persisted tables always have
	// business_id + created_at. Order-list / reservation-list Select projections
	// leave those zero — do not invent false QR / is_active defaults.
	if t.ID != 0 && t.BusinessID == 0 && t.CreatedAt.IsZero() {
		return json.Marshal(struct {
			ID        uint   `json:"id"`
			Name      string `json:"name"`
			TableCode string `json:"table_code,omitempty"`
			Capacity  int    `json:"capacity,omitempty"`
		}{
			ID:        t.ID,
			Name:      t.Name,
			TableCode: t.TableCode,
			Capacity:  t.Capacity,
		})
	}

	type tableAlias Table
	out := struct {
		tableAlias
		Business *billPublicBusiness `json:"business,omitempty"`
	}{tableAlias: tableAlias(t)}
	if t.Business.ID != 0 {
		safe := newBillPublicBusiness(t.Business)
		out.Business = &safe
	}
	return json.Marshal(out)
}

// MarshalJSON emits a slim customer shape when the row was preloaded with only
// dispatch/display columns (id, name, phone, email). A full default marshal
// invents is_active:false, email_verified:false, empty wallet/profile fields,
// and zero timestamps that look authoritative on delivery detail. (FIND-047)
func (c Customer) MarshalJSON() ([]byte, error) {
	// Partial projection heuristic: real persisted customers always have
	// created_at. Delivery Select("id","name","phone","email") leaves it zero.
	if c.ID != 0 && c.CreatedAt.IsZero() {
		return json.Marshal(struct {
			ID    uint   `json:"id"`
			Name  string `json:"name"`
			Phone string `json:"phone"`
			Email string `json:"email"`
		}{
			ID:    c.ID,
			Name:  c.Name,
			Phone: c.Phone,
			Email: c.Email,
		})
	}
	type alias Customer
	return json.Marshal(alias(c))
}

// MarshalJSON emits a slim staff shape when the row was preloaded with only
// display columns (id, name, email) — RBAC audit and alert label projections.
// A full default marshal invents is_active:false, authz_version:0, empty role,
// business_id:0, and zero timestamps that look authoritative. (FIND-052)
func (s Staff) MarshalJSON() ([]byte, error) {
	// Partial projection heuristic: real persisted staff always have created_at.
	// preloadRBACAuditStaffSummary Select("id","name","email") leaves it zero.
	if s.ID != 0 && s.CreatedAt.IsZero() {
		return json.Marshal(struct {
			ID    uint   `json:"id"`
			Name  string `json:"name"`
			Email string `json:"email"`
		}{
			ID:    s.ID,
			Name:  s.Name,
			Email: s.Email,
		})
	}
	type alias Staff
	return json.Marshal(alias(s))
}

// MarshalJSON emits a slim user shape when the row was loaded with only
// display columns (id, name, email) — L6-15 ledger list/detail actor
// projections. A full default marshal invents auth_method:"", nested
// notification_preferences zeros, email_verified:false, and zero timestamps
// that look authoritative and bloat the entries list wire body (decision-14).
func (u User) MarshalJSON() ([]byte, error) {
	// Partial projection heuristic: real persisted users always have created_at.
	// ListEntries Select("id, name, email") leaves it zero.
	if u.ID != 0 && u.CreatedAt.IsZero() {
		return json.Marshal(struct {
			ID    uint   `json:"id"`
			Name  string `json:"name"`
			Email string `json:"email"`
		}{
			ID:    u.ID,
			Name:  u.Name,
			Email: u.Email,
		})
	}
	type alias User
	return json.Marshal(alias(u))
}

// MarshalJSON serializes Bill monetary fields as dollars while keeping
// internal storage in cents.
//
// public_token, items (legacy JSON snapshot), settlement_address, and
// tipping_address are omit-empty. Order-list (preloadOrderListBillSummary) and
// other partial projections intentionally leave those columns unloaded; emitting
// "" would invent "no guest pay link / no wallets / no items" when the row
// actually has values on a full load (FIND-039 SQL + FIND-048 JSON honesty).
// Loaded items snapshots emit as a JSON array, never a quoted string (#771).
//
// billProjectionIsOrderListSummary fingerprints that same unload set: when those
// payment/token columns are all empty, also skip fiscal_* nulls and a zero
// loyalty_discount so list nests do not invent "no fiscal customer" /
// "no loyalty" (FIND-050). Full bill loads always have public_token and keep explicit fiscal nulls + loyalty 0.
func (b Bill) MarshalJSON() ([]byte, error) {
	currency := resolveBusinessCurrency(b.Currency, b.Business)
	for i := range b.Payments {
		if b.Payments[i].Currency == "" {
			b.Payments[i].Currency = currency
		}
	}

	listSummary := billProjectionIsOrderListSummary(b)

	dst := make([]byte, 0, 512)
	dst = append(dst, '{')
	first := true

	dst = appendJSONUintField(dst, &first, "id", uint64(b.ID))
	dst = appendJSONUintField(dst, &first, "business_id", uint64(b.BusinessID))
	dst = appendJSONUintField(dst, &first, "table_id", uint64(b.TableID))
	dst = appendJSONUintPtrOrNullField(dst, &first, "counter_id", b.CounterID)
	dst = appendJSONStringField(dst, &first, "bill_number", b.BillNumber)
	dst = appendJSONStringFieldOmitEmpty(dst, &first, "public_token", b.PublicToken)
	dst = appendJSONStringField(dst, &first, "notes", b.Notes)
	dst = appendJSONStringField(dst, &first, "currency", currency)
	// #771: items is a line array or omitted — never a quoted JSON string.
	// Empty/"[]" stay omitted so order-list summaries do not invent a blank
	// cart (FIND-048/057).
	if raw, empty, ok := jsonArraySnapshot(b.Items); ok && !empty {
		dst = appendJSONFieldPrefix(dst, &first, "items")
		dst = append(dst, raw...)
	}
	dst = appendJSONFloatField(dst, &first, "subtotal", centsToDollars(b.Subtotal))
	dst = appendJSONFloatField(dst, &first, "tax_amount", centsToDollars(b.TaxAmount))
	dst = appendJSONFloatField(dst, &first, "service_fee_amount", centsToDollars(b.ServiceFeeAmount))
	dst = appendJSONFloatField(dst, &first, "total_amount", centsToDollars(b.TotalAmount))
	dst = appendJSONFloatField(dst, &first, "paid_amount", centsToDollars(b.PaidAmount))
	remainingCents := b.TotalAmount - b.PaidAmount
	if remainingCents < 0 {
		remainingCents = 0
	}
	dst = appendJSONFloatField(dst, &first, "remaining", centsToDollars(remainingCents))
	dst = appendJSONFloatField(dst, &first, "tip_amount", centsToDollars(b.TipAmount))
	if !listSummary || b.LoyaltyDiscountCents != 0 {
		dst = appendJSONFloatField(dst, &first, "loyalty_discount", centsToDollars(b.LoyaltyDiscountCents))
	}
	dst = appendJSONStringField(dst, &first, "status", string(b.Status))
	dst = appendJSONStringFieldOmitEmpty(dst, &first, "settlement_address", b.SettlementAddr)
	dst = appendJSONStringFieldOmitEmpty(dst, &first, "tipping_address", b.TippingAddr)
	dst = appendJSONUintPtrField(dst, &first, "created_by_staff_id", b.CreatedByStaffID)
	dst = appendJSONUintPtrField(dst, &first, "closed_by_staff_id", b.ClosedByStaffID)
	dst = appendJSONUintPtrField(dst, &first, "crm_customer_id", b.CRMCustomerID)
	dst = appendJSONTimeField(dst, &first, "created_at", b.CreatedAt)
	dst = appendJSONTimeField(dst, &first, "updated_at", b.UpdatedAt)
	dst = appendJSONTimePtrOrNullField(dst, &first, "closed_at", b.ClosedAt)
	dst = appendJSONTimePtrField(dst, &first, "abandoned_at", b.AbandonedAt)
	dst = appendJSONTimePtrField(dst, &first, "settled_at", b.SettledAt)
	dst = appendJSONTimePtrField(dst, &first, "feedback_email_sent_at", b.FeedbackEmailSentAt)
	if !listSummary {
		dst = appendJSONStringPtrOrNullField(dst, &first, "fiscal_customer_doc_type", b.FiscalCustomerDocType)
		dst = appendJSONStringPtrOrNullField(dst, &first, "fiscal_customer_doc_number", b.FiscalCustomerDocNumber)
		dst = appendJSONStringPtrOrNullField(dst, &first, "fiscal_customer_tax_condition", b.FiscalCustomerTaxCondition)
		dst = appendJSONStringPtrOrNullField(dst, &first, "fiscal_customer_name", b.FiscalCustomerName)
		// Guest-entered factura delivery address. Custom
		// MarshalJSON (not the model json tag) is the wire contract for Bill.
		dst = appendJSONStringPtrOrNullField(dst, &first, "fiscal_customer_email", b.FiscalCustomerEmail)
	}

	if b.Business.ID != 0 {
		safe := newBillPublicBusiness(b.Business)
		var err error
		dst, err = appendJSONMarshaledField(dst, &first, "business", safe)
		if err != nil {
			return nil, err
		}
	}

	if b.Table.ID != 0 {
		var err error
		dst, err = appendJSONMarshaledField(dst, &first, "table", b.Table)
		if err != nil {
			return nil, err
		}
	}
	if len(b.Payments) > 0 {
		var err error
		dst, err = appendJSONMarshaledField(dst, &first, "payments", b.Payments)
		if err != nil {
			return nil, err
		}
	}
	if len(b.AlternativePayments) > 0 {
		var err error
		dst, err = appendJSONMarshaledField(dst, &first, "alternative_payments", b.AlternativePayments)
		if err != nil {
			return nil, err
		}
	}
	if len(b.ItemsRelation) > 0 {
		var err error
		dst, err = appendJSONMarshaledField(dst, &first, "items_relation", b.ItemsRelation)
		if err != nil {
			return nil, err
		}
	}
	if b.CreatedByStaff != nil {
		var err error
		dst, err = appendJSONMarshaledField(dst, &first, "created_by_staff", b.CreatedByStaff)
		if err != nil {
			return nil, err
		}
	}
	if b.ClosedByStaff != nil {
		var err error
		dst, err = appendJSONMarshaledField(dst, &first, "closed_by_staff", b.ClosedByStaff)
		if err != nil {
			return nil, err
		}
	}
	if b.CRMCustomer != nil {
		var err error
		dst, err = appendJSONMarshaledField(dst, &first, "crm_customer", b.CRMCustomer)
		if err != nil {
			return nil, err
		}
	}

	dst = append(dst, '}')
	return dst, nil
}

// UnmarshalJSON accepts items as a JSON array (wire contract #771) or the
// legacy quoted snapshot string, and stores the snapshot text GORM persists.
func (b *Bill) UnmarshalJSON(data []byte) error {
	type Shadow Bill
	aux := &struct {
		*Shadow
		Items json.RawMessage `json:"items"`
	}{Shadow: (*Shadow)(b)}
	if err := json.Unmarshal(data, aux); err != nil {
		return err
	}
	b.Items = snapshotFromJSONItems(aux.Items)
	return nil
}

// MarshalJSON serializes Payment monetary fields as dollars.
func (p Payment) MarshalJSON() ([]byte, error) {
	type Shadow Payment
	shadow := (Shadow)(p)
	shadow.Currency = resolvePaymentCurrency(p)

	return json.Marshal(&struct {
		Shadow
		Amount    float64 `json:"amount"`
		TipAmount float64 `json:"tip_amount"`
	}{
		Shadow:    shadow,
		Amount:    centsToDollars(p.Amount),
		TipAmount: centsToDollars(p.TipAmount),
	})
}

// MarshalJSON serializes Order with the business currency used by its money
// item payload. The Business relation itself is NEVER emitted (X-1): it
// carries owner identity and settlement details, and Order JSON
// flows to unauthenticated guests (create response, public guest-orders poll,
// SSE order.created/order.updated). Currency resolution is the only thing any
// order consumer needs from the relation, and that happens here, before the
// field is masked. The nil `any` override shadows the embedded field at a
// shallower depth, and omitempty drops the key entirely.
//
// Unloaded Bill (ID==0) is also masked: guest create / list paths often set
// bill_id without Preload("Bill"). Default Bill.MarshalJSON then invents a
// zero-money open bill (id:0, total:0, empty status, fiscal nulls) that looks
// like a real nested resource beside the honest top-level bill (FIND-051).
func (o Order) MarshalJSON() ([]byte, error) {
	type Shadow Order
	shadow := (Shadow)(o)
	shadow.Currency = o.ResolvedCurrency()
	shadow.Items = ""
	items := json.RawMessage("[]")
	if raw, _, ok := jsonArraySnapshot(o.Items); ok {
		items = raw
	}
	out := struct {
		Shadow
		Items    json.RawMessage `json:"items"`
		Business any             `json:"business,omitempty"`
		Bill     any             `json:"bill,omitempty"`
	}{Shadow: shadow, Items: items}
	if o.Bill.ID != 0 {
		out.Bill = o.Bill
	}
	return json.Marshal(out)
}

// UnmarshalJSON accepts items as a JSON array (wire contract #771) or the
// legacy quoted snapshot string, and stores the snapshot text GORM persists.
func (o *Order) UnmarshalJSON(data []byte) error {
	type Shadow Order
	aux := &struct {
		*Shadow
		Items json.RawMessage `json:"items"`
	}{Shadow: (*Shadow)(o)}
	if err := json.Unmarshal(data, aux); err != nil {
		return err
	}
	o.Items = snapshotFromJSONItems(aux.Items)
	return nil
}

// MarshalJSON serializes AlternativePayment amount as dollars.
func (ap AlternativePayment) MarshalJSON() ([]byte, error) {
	type Shadow AlternativePayment
	return json.Marshal(&struct {
		Shadow
		Amount float64 `json:"amount"`
	}{
		Shadow: (Shadow)(ap),
		Amount: centsToDollars(ap.Amount),
	})
}

// MarshalJSON serializes CashRegisterSession monetary fields as dollars.
func (s CashRegisterSession) MarshalJSON() ([]byte, error) {
	out := struct {
		ID              uint                      `json:"id"`
		BusinessID      uint                      `json:"business_id"`
		Status          CashRegisterSessionStatus `json:"status"`
		OpeningFloat    float64                   `json:"opening_float"`
		OpeningNote     string                    `json:"opening_note"`
		OpenedByUserID  *uint                     `json:"opened_by_user_id"`
		OpenedByStaffID *uint                     `json:"opened_by_staff_id"`
		OpenedByLabel   string                    `json:"opened_by_label"`
		OpenedAt        time.Time                 `json:"opened_at"`
		CashSales       float64                   `json:"cash_sales"`
		CashRefunds     float64                   `json:"cash_refunds"`
		CashIn          float64                   `json:"cash_in"`
		CashOut         float64                   `json:"cash_out"`
		ExpectedCash    *float64                  `json:"expected_cash,omitempty"`
		CountedCash     *float64                  `json:"counted_cash,omitempty"`
		Variance        *float64                  `json:"variance,omitempty"`
		ClosingNote     string                    `json:"closing_note"`
		ClosedByUserID  *uint                     `json:"closed_by_user_id"`
		ClosedByStaffID *uint                     `json:"closed_by_staff_id"`
		ClosedByLabel   string                    `json:"closed_by_label"`
		ClosedAt        *time.Time                `json:"closed_at"`
		CreatedAt       time.Time                 `json:"created_at"`
		UpdatedAt       time.Time                 `json:"updated_at"`
	}{
		ID:              s.ID,
		BusinessID:      s.BusinessID,
		Status:          s.Status,
		OpeningFloat:    centsToDollars(s.OpeningFloatCents),
		OpeningNote:     s.OpeningNote,
		OpenedByUserID:  s.OpenedByUserID,
		OpenedByStaffID: s.OpenedByStaffID,
		OpenedByLabel:   s.OpenedByLabel,
		OpenedAt:        s.OpenedAt,
		CashSales:       centsToDollars(s.CashSalesCents),
		CashRefunds:     centsToDollars(s.CashRefundsCents),
		CashIn:          centsToDollars(s.CashInCents),
		CashOut:         centsToDollars(s.CashOutCents),
		ClosingNote:     s.ClosingNote,
		ClosedByUserID:  s.ClosedByUserID,
		ClosedByStaffID: s.ClosedByStaffID,
		ClosedByLabel:   s.ClosedByLabel,
		ClosedAt:        s.ClosedAt,
		CreatedAt:       s.CreatedAt,
		UpdatedAt:       s.UpdatedAt,
	}
	if s.Status == CashRegisterSessionStatusClosed {
		expectedCash := centsToDollars(s.ExpectedCashCents)
		countedCash := centsToDollars(s.CountedCashCents)
		variance := centsToDollars(s.VarianceCents)
		out.ExpectedCash = &expectedCash
		out.CountedCash = &countedCash
		out.Variance = &variance
	}
	return json.Marshal(out)
}

// MarshalJSON serializes CashRegisterMovement amount as dollars.
func (m CashRegisterMovement) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		ID                   uint                     `json:"id"`
		BusinessID           uint                     `json:"business_id"`
		SessionID            uint                     `json:"session_id"`
		MovementType         CashRegisterMovementType `json:"movement_type"`
		Amount               float64                  `json:"amount"`
		Reason               string                   `json:"reason"`
		Note                 string                   `json:"note"`
		AlternativePaymentID *uint                    `json:"alternative_payment_id"`
		BillID               *uint                    `json:"bill_id"`
		ActorUserID          *uint                    `json:"actor_user_id"`
		ActorStaffID         *uint                    `json:"actor_staff_id"`
		ActorLabel           string                   `json:"actor_label"`
		OccurredAt           time.Time                `json:"occurred_at"`
		CreatedAt            time.Time                `json:"created_at"`
	}{
		ID:                   m.ID,
		BusinessID:           m.BusinessID,
		SessionID:            m.SessionID,
		MovementType:         m.MovementType,
		Amount:               centsToDollars(m.AmountCents),
		Reason:               m.Reason,
		Note:                 m.Note,
		AlternativePaymentID: m.AlternativePaymentID,
		BillID:               m.BillID,
		ActorUserID:          m.ActorUserID,
		ActorStaffID:         m.ActorStaffID,
		ActorLabel:           m.ActorLabel,
		OccurredAt:           m.OccurredAt,
		CreatedAt:            m.CreatedAt,
	})
}

// MarshalJSON serializes WithdrawalHistory monetary fields as dollars.
func (wh WithdrawalHistory) MarshalJSON() ([]byte, error) {
	type Shadow WithdrawalHistory
	return json.Marshal(&struct {
		Shadow
		PaymentAmount float64 `json:"payment_amount"`
		TipAmount     float64 `json:"tip_amount"`
		TotalAmount   float64 `json:"total_amount"`
	}{
		Shadow:        (Shadow)(wh),
		PaymentAmount: centsToDollars(wh.PaymentAmount),
		TipAmount:     centsToDollars(wh.TipAmount),
		TotalAmount:   centsToDollars(wh.TotalAmount),
	})
}

// MarshalJSON serializes ManualLedgerEntry amount as dollars.
func (mle ManualLedgerEntry) MarshalJSON() ([]byte, error) {
	type Shadow ManualLedgerEntry
	return json.Marshal(&struct {
		Shadow
		Amount float64 `json:"amount"`
	}{
		Shadow: (Shadow)(mle),
		Amount: centsToDollars(mle.Amount),
	})
}

// MarshalJSON serializes PayrollRun monetary fields as dollars.
func (pr PayrollRun) MarshalJSON() ([]byte, error) {
	type Shadow PayrollRun
	return json.Marshal(&struct {
		Shadow
		GrossTotal     float64 `json:"gross_total"`
		BonusTotal     float64 `json:"bonus_total"`
		DeductionTotal float64 `json:"deduction_total"`
		NetTotal       float64 `json:"net_total"`
	}{
		Shadow:         (Shadow)(pr),
		GrossTotal:     centsToDollars(pr.GrossTotal),
		BonusTotal:     centsToDollars(pr.BonusTotal),
		DeductionTotal: centsToDollars(pr.DeductionTotal),
		NetTotal:       centsToDollars(pr.NetTotal),
	})
}

// MarshalJSON serializes PayrollLineItem monetary fields as dollars.
func (pli PayrollLineItem) MarshalJSON() ([]byte, error) {
	type Shadow PayrollLineItem
	return json.Marshal(&struct {
		Shadow
		GrossAmount     float64 `json:"gross_amount"`
		BonusAmount     float64 `json:"bonus_amount"`
		DeductionAmount float64 `json:"deduction_amount"`
		NetAmount       float64 `json:"net_amount"`
	}{
		Shadow:          (Shadow)(pli),
		GrossAmount:     centsToDollars(pli.GrossAmount),
		BonusAmount:     centsToDollars(pli.BonusAmount),
		DeductionAmount: centsToDollars(pli.DeductionAmount),
		NetAmount:       centsToDollars(pli.NetAmount),
	})
}

// MarshalJSON serializes PaymentBreakdown fields as dollars.
func (pb PaymentBreakdown) MarshalJSON() ([]byte, error) {
	type Shadow PaymentBreakdown
	return json.Marshal(&struct {
		Shadow
		TotalAmount     float64 `json:"total_amount"`
		CryptoPaid      float64 `json:"crypto_paid"`
		AlternativePaid float64 `json:"alternative_paid"`
		Remaining       float64 `json:"remaining"`
	}{
		Shadow:          (Shadow)(pb),
		TotalAmount:     centsToDollars(pb.TotalAmount),
		CryptoPaid:      centsToDollars(pb.CryptoPaid),
		AlternativePaid: centsToDollars(pb.AlternativePaid),
		Remaining:       centsToDollars(pb.Remaining),
	})
}

// MarshalJSON serializes DeliveryOrder monetary fields as dollars while keeping
// internal storage in cents. The embedded Business relation is scrubbed down to
// the safe billPublicBusiness summary so that SSE delivery.* events never leak
// Stripe IDs, owner PII, or wallet addresses even when an emit site preloads
// the Business relation. Mirrors Table/Bill. (SSE-LEAK-3)
//
// Bill is a non-pointer association: encoding/json never omits zero-value
// structs even with omitempty, so dispatch list/detail used to emit a full
// empty bill{} (id:0, zero wallets/timestamps) on every row when Bill was not
// preloaded. Shadow with nil unless Bill.ID is set. (FIND-044)
func (do DeliveryOrder) MarshalJSON() ([]byte, error) {
	type doAlias DeliveryOrder
	out := struct {
		doAlias
		Business    *billPublicBusiness `json:"business,omitempty"`
		Bill        any                 `json:"bill,omitempty"`
		DeliveryFee float64             `json:"delivery_fee"`
		DriverTip   float64             `json:"driver_tip"`
		PlatformFee float64             `json:"platform_fee"`
		Total       *float64            `json:"total"`
		Currency    string              `json:"currency,omitempty"`
	}{
		doAlias:     doAlias(do),
		DeliveryFee: centsToDollars(do.DeliveryFee),
		DriverTip:   centsToDollars(do.DriverTip),
		PlatformFee: centsToDollars(do.PlatformFee),
		Currency:    do.DispatchCurrency,
	}
	// One conversion site: the dispatch hydrator hands over cents, exactly like
	// the preloaded Bill does, and centsToDollars is the only thing that divides.
	if do.DispatchTotalCents != nil {
		total := centsToDollars(*do.DispatchTotalCents)
		out.Total = &total
	} else if do.Bill.ID != 0 {
		total := centsToDollars(do.Bill.TotalAmount)
		out.Total = &total
	}
	if out.Currency == "" && do.Business.ID != 0 {
		out.Currency = resolveBusinessCurrency("", do.Business)
	}
	if do.Business.ID != 0 {
		safe := newBillPublicBusiness(do.Business)
		out.Business = &safe
	}
	if do.Bill.ID != 0 {
		out.Bill = do.Bill
	}
	return json.Marshal(out)
}

// MarshalJSON serializes DeliveryZone monetary fields as dollars.
func (dz DeliveryZone) MarshalJSON() ([]byte, error) {
	type Shadow DeliveryZone
	return json.Marshal(&struct {
		Shadow
		DeliveryFee        float64 `json:"delivery_fee"`
		MinimumOrderAmount float64 `json:"minimum_order_amount"`
	}{
		Shadow:             (Shadow)(dz),
		DeliveryFee:        centsToDollars(dz.DeliveryFee),
		MinimumOrderAmount: centsToDollars(dz.MinimumOrderAmount),
	})
}

// MarshalJSON serializes DeliverySettings monetary fields as dollars.
func (ds DeliverySettings) MarshalJSON() ([]byte, error) {
	type Shadow DeliverySettings
	return json.Marshal(&struct {
		Shadow
		FlatDeliveryFee     float64 `json:"flat_delivery_fee"`
		FreeDeliveryMinimum float64 `json:"free_delivery_minimum"`
		MinimumOrderAmount  float64 `json:"minimum_order_amount"`
	}{
		Shadow:              (Shadow)(ds),
		FlatDeliveryFee:     centsToDollars(ds.FlatDeliveryFee),
		FreeDeliveryMinimum: centsToDollars(ds.FreeDeliveryMinimum),
		MinimumOrderAmount:  centsToDollars(ds.MinimumOrderAmount),
	})
}

// MarshalJSON emits exception_date as YYYY-MM-DD for the hours editor / public page.
func (e BusinessOperatingException) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		ID               uint      `json:"id"`
		BusinessID       uint      `json:"business_id"`
		ExceptionDate    string    `json:"exception_date"`
		OpenTime         *string   `json:"open_time,omitempty"`
		CloseTime        *string   `json:"close_time,omitempty"`
		KitchenCloseTime *string   `json:"kitchen_close_time,omitempty"`
		IsClosed         bool      `json:"is_closed"`
		Label            string    `json:"label"`
		CreatedAt        time.Time `json:"created_at"`
		UpdatedAt        time.Time `json:"updated_at"`
	}{
		ID:               e.ID,
		BusinessID:       e.BusinessID,
		ExceptionDate:    e.ExceptionDate.UTC().Format("2006-01-02"),
		OpenTime:         e.OpenTime,
		CloseTime:        e.CloseTime,
		KitchenCloseTime: e.KitchenCloseTime,
		IsClosed:         e.IsClosed,
		Label:            e.Label,
		CreatedAt:        e.CreatedAt,
		UpdatedAt:        e.UpdatedAt,
	})
}

// UnmarshalJSON accepts exception_date as YYYY-MM-DD or RFC3339.
func (e *BusinessOperatingException) UnmarshalJSON(data []byte) error {
	aux := struct {
		ID               uint      `json:"id"`
		BusinessID       uint      `json:"business_id"`
		ExceptionDate    string    `json:"exception_date"`
		OpenTime         *string   `json:"open_time"`
		CloseTime        *string   `json:"close_time"`
		KitchenCloseTime *string   `json:"kitchen_close_time"`
		IsClosed         bool      `json:"is_closed"`
		Label            string    `json:"label"`
		CreatedAt        time.Time `json:"created_at"`
		UpdatedAt        time.Time `json:"updated_at"`
	}{}
	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}
	raw := strings.TrimSpace(aux.ExceptionDate)
	var day time.Time
	if raw != "" {
		if t, err := time.ParseInLocation("2006-01-02", raw, time.UTC); err == nil {
			day = t
		} else if t, err := time.Parse(time.RFC3339, raw); err == nil {
			day = time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
		} else {
			return fmt.Errorf("exception_date must be YYYY-MM-DD: %w", err)
		}
	}
	e.ID = aux.ID
	e.BusinessID = aux.BusinessID
	e.ExceptionDate = day
	e.OpenTime = aux.OpenTime
	e.CloseTime = aux.CloseTime
	e.KitchenCloseTime = aux.KitchenCloseTime
	e.IsClosed = aux.IsClosed
	e.Label = aux.Label
	e.CreatedAt = aux.CreatedAt
	e.UpdatedAt = aux.UpdatedAt
	return nil
}

func appendJSONFieldPrefix(dst []byte, first *bool, name string) []byte {
	if *first {
		*first = false
	} else {
		dst = append(dst, ',')
	}
	dst = appendJSONString(dst, name)
	dst = append(dst, ':')
	return dst
}

func appendJSONStringField(dst []byte, first *bool, name string, value string) []byte {
	dst = appendJSONFieldPrefix(dst, first, name)
	return appendJSONString(dst, value)
}

// appendJSONStringFieldOmitEmpty skips the key when value is empty so partial
// preloads do not invent blank secrets / wallets / snapshots.
func appendJSONStringFieldOmitEmpty(dst []byte, first *bool, name string, value string) []byte {
	if value == "" {
		return dst
	}
	return appendJSONStringField(dst, first, name, value)
}

// jsonArraySnapshot returns a JSON array payload for a stored items snapshot.
// Empty, "null", non-array, and invalid JSON are not ok. A compact "[]" is
// ok + empty so bill summaries can omit a blank cart while orders still emit [].
func jsonArraySnapshot(value string) (raw json.RawMessage, empty bool, ok bool) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" || trimmed == "null" {
		return nil, true, false
	}
	b := []byte(trimmed)
	if b[0] != '[' || !json.Valid(b) {
		return nil, true, false
	}
	return json.RawMessage(b), trimmed == "[]", true
}

// JSONItemsArrayWire returns a non-empty items snapshot as a JSON array, or
// nil when the snapshot is absent/empty so omitempty drops the key (#771).
func JSONItemsArrayWire(value string) json.RawMessage {
	raw, empty, ok := jsonArraySnapshot(value)
	if !ok || empty {
		return nil
	}
	return raw
}

func snapshotFromJSONItems(raw json.RawMessage) string {
	if len(raw) == 0 || string(raw) == "null" {
		return ""
	}
	if raw[0] == '"' {
		var s string
		if err := json.Unmarshal(raw, &s); err != nil {
			return ""
		}
		return s
	}
	return string(raw)
}

// billProjectionIsOrderListSummary reports whether this Bill looks like the
// kitchen/order-list projection (preloadOrderListBillSummary): payment identity
// columns were not selected, so their Go zero values are unloaded — not real
// empty data. Full bills post-token backfill always carry a public_token.
func billProjectionIsOrderListSummary(b Bill) bool {
	return b.PublicToken == "" && b.Items == "" && b.SettlementAddr == "" && b.TippingAddr == ""
}

func appendJSONUintField(dst []byte, first *bool, name string, value uint64) []byte {
	dst = appendJSONFieldPrefix(dst, first, name)
	return strconv.AppendUint(dst, value, 10)
}

func appendJSONUintPtrField(dst []byte, first *bool, name string, value *uint) []byte {
	if value == nil {
		return dst
	}
	return appendJSONUintField(dst, first, name, uint64(*value))
}

func appendJSONUintPtrOrNullField(dst []byte, first *bool, name string, value *uint) []byte {
	dst = appendJSONFieldPrefix(dst, first, name)
	if value == nil {
		return append(dst, "null"...)
	}
	return strconv.AppendUint(dst, uint64(*value), 10)
}

func appendJSONFloatField(dst []byte, first *bool, name string, value float64) []byte {
	dst = appendJSONFieldPrefix(dst, first, name)
	return strconv.AppendFloat(dst, value, 'f', -1, 64)
}

func appendJSONTimeField(dst []byte, first *bool, name string, value time.Time) []byte {
	dst = appendJSONFieldPrefix(dst, first, name)
	dst = append(dst, '"')
	dst = value.AppendFormat(dst, time.RFC3339Nano)
	dst = append(dst, '"')
	return dst
}

func appendJSONTimePtrField(dst []byte, first *bool, name string, value *time.Time) []byte {
	if value == nil {
		return dst
	}
	return appendJSONTimeField(dst, first, name, *value)
}

func appendJSONTimePtrOrNullField(dst []byte, first *bool, name string, value *time.Time) []byte {
	if value == nil {
		dst = appendJSONFieldPrefix(dst, first, name)
		return append(dst, "null"...)
	}
	return appendJSONTimeField(dst, first, name, *value)
}

// appendJSONStringPtrOrNullField emits the JSON key always: "null" when ptr is
// nil, or a quoted string when set. Used for nullable fiscal customer fields that
// consumers must distinguish from "not present" vs "explicitly unset".
func appendJSONStringPtrOrNullField(dst []byte, first *bool, name string, value *string) []byte {
	dst = appendJSONFieldPrefix(dst, first, name)
	if value == nil {
		return append(dst, "null"...)
	}
	return appendJSONString(dst, *value)
}

func appendJSONMarshaledField(dst []byte, first *bool, name string, value any) ([]byte, error) {
	dst = appendJSONFieldPrefix(dst, first, name)
	nested, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	dst = append(dst, nested...)
	return dst, nil
}

func appendJSONString(dst []byte, value string) []byte {
	dst = append(dst, '"')
	for len(value) > 0 {
		r, size := utf8.DecodeRuneInString(value)
		if r == utf8.RuneError && size == 1 {
			dst = append(dst, '\\', 'u', 'f', 'f', 'f', 'd')
			value = value[size:]
			continue
		}
		switch r {
		case '"', '\\':
			dst = append(dst, '\\', byte(r))
		case '\b':
			dst = append(dst, '\\', 'b')
		case '\f':
			dst = append(dst, '\\', 'f')
		case '\n':
			dst = append(dst, '\\', 'n')
		case '\r':
			dst = append(dst, '\\', 'r')
		case '\t':
			dst = append(dst, '\\', 't')
		default:
			if r < 0x20 {
				dst = append(dst, '\\', 'u', '0', '0', hexDigit(byte(r>>4)), hexDigit(byte(r&0x0f)))
			} else {
				dst = append(dst, value[:size]...)
			}
		}
		value = value[size:]
	}
	dst = append(dst, '"')
	return dst
}

func hexDigit(n byte) byte {
	if n < 10 {
		return '0' + n
	}
	return 'a' + (n - 10)
}
