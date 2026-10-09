package server

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/stdevmac/payverge/backend/internal/database"
)

// newReservationWithSecrets builds a fully populated reservation whose embedded
// Business carries sentinel secrets, plus internal staff notes and a status
// history entry, for the SSE/public payload projection guards.
func newReservationWithSecrets() database.TableReservation {
	tableID := uint(3)
	return database.TableReservation{
		ID:               42,
		BusinessID:       7,
		TableID:          &tableID,
		CustomerName:     "Guest",
		CustomerPhone:    "+15555550100",
		CustomerEmail:    "guest@example.com",
		PartySize:        4,
		ReservationTime:  time.Date(2026, 6, 17, 19, 0, 0, 0, time.UTC),
		Duration:         120,
		Status:           "confirmed",
		Source:           "customer",
		ConfirmationCode: "ABC123DEF456",
		SpecialRequests:  "window seat",
		Notes:            "internal",
		CreatedBy:        "customer",
		CreatedAt:        time.Date(2026, 6, 17, 12, 0, 0, 0, time.UTC),
		UpdatedAt:        time.Date(2026, 6, 17, 12, 0, 0, 0, time.UTC),
		Business: database.Business{
			ID:             7,
			Email:          "owner@example.com",
			OwnerAddress:   "0xOWNERWALLET",
			SettlementAddr: "0xSETTLEMENT",
			TippingAddr:    "0xTIPPING",
		},
		Table: &database.Table{
			ID:        3,
			Name:      "Table 12",
			TableCode: "QRACCESS531SECRET",
			QRCode:    "qr_payload_531_secret",
		},
		StatusHistory: []database.ReservationStatusHistory{{ID: 1, Status: "confirmed"}},
	}
}

// topLevelKeys marshals v the way the hub / c.JSON does and returns its decoded
// top-level object so tests can assert key presence without tripping over
// unrelated nested back-references (e.g. status_history[].reservation.business,
// a zero-value model quirk never hydrated with secrets in production).
func topLevelKeys(t *testing.T, v any) (map[string]json.RawMessage, string) {
	t.Helper()
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("unmarshal payload object: %v", err)
	}
	return m, string(raw)
}

// TestNaiveReservationMarshalDoesNotLeakBusiness documents that the Reservation
// model itself omits Business from JSON (json:"-"/omitempty after FIND-037).
// Event/public projections remain belt-and-suspenders (SSE-03).
func TestNaiveReservationMarshalDoesNotLeakBusiness(t *testing.T) {
	r := newReservationWithSecrets()
	m, got := topLevelKeys(t, &r)
	if _, ok := m["business"]; ok {
		t.Fatalf("raw reservation marshal must not expose top-level business: %s", got)
	}
	if strings.Contains(got, "cus_LEAKME") {
		t.Fatalf("raw reservation marshal leaked embedded Stripe customer id: %s", got)
	}
}

// TestReservationEventPayloadShape asserts the dashboard SSE payload, marshaled
// the way the hub does, omits the embedded Business (no key, no secrets) while
// retaining staff/guest fields. (SSE-03)
func TestReservationEventPayloadShape(t *testing.T) {
	r := newReservationWithSecrets()

	m, got := topLevelKeys(t, reservationEventPayload(&r))

	if _, ok := m["business"]; ok {
		t.Fatalf("event payload must not contain a top-level business key: %s", got)
	}
	if strings.Contains(got, "cus_LEAKME") {
		t.Fatalf("event payload leaked embedded Stripe customer id: %s", got)
	}

	for _, k := range []string{
		"id", "status", "customer_name", "notes",
		"status_history", "confirmation_code", "table",
	} {
		if _, ok := m[k]; !ok {
			t.Fatalf("event payload missing required field %q: %s", k, got)
		}
	}
	if !strings.Contains(got, "QRACCESS531SECRET") {
		t.Fatalf("staff event payload must still include the assigned table_code: %s", got)
	}
}

// TestReservationPublicPayloadShape asserts the guest-facing public payload
// additionally omits internal notes and status history. (SSE-03)
func TestReservationPublicPayloadShape(t *testing.T) {
	r := newReservationWithSecrets()

	m, got := topLevelKeys(t, publicReservationPayload(&r))

	for _, forbidden := range []string{"business", "notes", "status_history", "table"} {
		if _, ok := m[forbidden]; ok {
			t.Fatalf("public payload must not contain top-level %q key: %s", forbidden, got)
		}
	}
	if strings.Contains(got, "cus_LEAKME") {
		t.Fatalf("public payload leaked embedded Stripe customer id: %s", got)
	}
	assertPublicJSONOmitsGuestTableCredential(t, got)

	for _, k := range []string{
		"id", "status", "customer_name",
		"confirmation_code", "party_size",
	} {
		if _, ok := m[k]; !ok {
			t.Fatalf("public payload missing required field %q: %s", k, got)
		}
	}
}

// BenchmarkReservationEventPayload measures the full hub round-trip: build the
// SSE payload and marshal it the way Hub.PublishJSON does. (SSE-03)
func BenchmarkReservationEventPayload(b *testing.B) {
	r := newReservationWithSecrets()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		raw, err := json.Marshal(reservationEventPayload(&r))
		if err != nil {
			b.Fatalf("marshal event payload: %v", err)
		}
		_ = raw
	}
}

// BenchmarkReservationPublicPayload measures the guest public mutation response
// round-trip: build the payload and marshal it the way c.JSON does. (SSE-03)
func BenchmarkReservationPublicPayload(b *testing.B) {
	r := newReservationWithSecrets()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		raw, err := json.Marshal(publicReservationPayload(&r))
		if err != nil {
			b.Fatalf("marshal public payload: %v", err)
		}
		_ = raw
	}
}
