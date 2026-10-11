package server

import (
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/stdevmac/payverge/backend/internal/database"
)

// TestPublicReservationPayloadStripsOperatorData guards the public reservation
// mutation responses (create/cancel) against leaking operator-internal
// data to unauthenticated guests. The raw TableReservation embeds the full
// Business (operator Stripe IDs, wallet/settlement addresses, contact email,
// internal billing state) and carries staff-internal Notes; none of that may
// reach a guest.
func TestPublicReservationPayloadStripsOperatorData(t *testing.T) {
	res := &database.TableReservation{
		ID:               42,
		BusinessID:       7,
		CustomerName:     "Guest",
		PartySize:        2,
		Status:           "confirmed",
		ConfirmationCode: "ABC123",
		Notes:            "VIP - comp dessert, owner friend", // internal staff note
		Business: database.Business{
			ID:             7,
			Email:          "owner@example.com",
			OwnerAddress:   "0xOWNERWALLET",
			SettlementAddr: "0xSETTLEMENT",
			TippingAddr:    "0xTIPPING",
		},
		StatusHistory: []database.ReservationStatusHistory{{ID: 1}},
		Table: &database.Table{
			ID:        3,
			Name:      "Patio 12",
			TableCode: "QRACCESS531SECRET",
			QRCode:    "qr_payload_531_secret",
			Capacity:  4,
		},
	}

	// publicReservationPayload now returns a single-marshal shadow view (any),
	// so inspect the JSON the way c.JSON serializes it.
	payload, got := topLevelKeys(t, publicReservationPayload(res))

	for _, k := range []string{"business", "notes", "status_history", "table"} {
		if _, ok := payload[k]; ok {
			t.Fatalf("public reservation payload must not contain %q key", k)
		}
	}

	// Defense in depth: no sensitive value should survive anywhere in the JSON.
	for _, secret := range []string{
		"owner@example.com",
		"0xOWNERWALLET", "0xSETTLEMENT", "0xTIPPING",
		"cus_SECRET123", "sub_SECRET123", "pm_SECRET123",
		"comp dessert",
		"QRACCESS531SECRET", "qr_payload_531_secret", `"table_code"`, `"qr_code"`,
	} {
		if strings.Contains(got, secret) {
			t.Fatalf("public reservation payload leaked operator/internal value %q: %s", secret, got)
		}
	}

	// The guest-facing fields the FE reads must still be present.
	for _, k := range []string{"id", "customer_name", "party_size", "status", "confirmation_code", "reservation_time"} {
		if _, ok := payload[k]; !ok {
			t.Fatalf("public reservation payload missing required guest field %q", k)
		}
	}
}

// TestReservationEventPayloadStripsEmbeddedBusiness guards the SSE
// reservation.new / reservation.updated events. They go to the authenticated
// business dashboard channel, so staff-facing fields (notes, table, status)
// stay — but the embedded Business aggregate (Stripe IDs, settlement/tipping
// wallets, billing state) must never ride along to every dashboard subscriber.
func TestReservationEventPayloadStripsEmbeddedBusiness(t *testing.T) {
	res := &database.TableReservation{
		ID:               42,
		BusinessID:       7,
		CustomerName:     "Guest",
		PartySize:        2,
		Status:           "confirmed",
		ConfirmationCode: "ABC123DEF456",
		Notes:            "VIP - comp dessert",
		Business: database.Business{
			ID:             7,
			Email:          "owner@example.com",
			OwnerAddress:   "0xOWNERWALLET",
			SettlementAddr: "0xSETTLEMENT",
			TippingAddr:    "0xTIPPING",
		},
	}

	// reservationEventPayload now returns a single-marshal shadow view (any),
	// so inspect the JSON the way Hub.PublishJSON serializes it.
	payload, got := topLevelKeys(t, reservationEventPayload(res))

	if _, ok := payload["business"]; ok {
		t.Fatalf("reservation event payload must not contain the embedded business")
	}

	for _, secret := range []string{
		"owner@example.com",
		"0xOWNERWALLET", "0xSETTLEMENT", "0xTIPPING",
		"cus_SECRET123", "sub_SECRET123", "pm_SECRET123",
	} {
		if strings.Contains(got, secret) {
			t.Fatalf("reservation event payload leaked operator value %q: %s", secret, got)
		}
	}

	// Staff dashboard consumers still need these.
	for _, k := range []string{"id", "business_id", "customer_name", "party_size", "status", "notes", "reservation_time"} {
		if _, ok := payload[k]; !ok {
			t.Fatalf("reservation event payload missing staff field %q", k)
		}
	}
}

// TestReservationHandlersNeverPublishRawReservations is an access-shape guard:
// every SSE publish in reservation_handlers.go must go through
// reservationEventPayload so a new call site cannot reintroduce the embedded
// Business leak.
func TestReservationHandlersNeverPublishRawReservations(t *testing.T) {
	src, err := os.ReadFile("reservation_handlers.go")
	if err != nil {
		t.Fatalf("read handlers source: %v", err)
	}
	publishCalls := regexp.MustCompile(`PublishJSON\([^)]*\)`).FindAllString(string(src), -1)
	if len(publishCalls) == 0 {
		t.Fatalf("expected PublishJSON call sites in reservation_handlers.go")
	}
	for _, call := range publishCalls {
		if !strings.Contains(call, "reservationEventPayload(") {
			t.Fatalf("SSE publish must wrap the reservation in reservationEventPayload: %s", call)
		}
	}
}
