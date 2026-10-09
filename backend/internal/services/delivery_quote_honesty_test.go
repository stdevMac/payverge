package services

import (
	"testing"
	"time"

	"github.com/stdevmac/payverge/backend/internal/database"
)

// TestQuote_IneligibleCarriesNoFabricatedNumbers ensures that ineligible
// delivery quotes carry zero fee, zero ETA minutes, and nil delivery-time
// pointer — the live site showed "$0.00 · 0 min" cards because EstimatedPrepTime
// was pre-filled in the initial quote struct even on early ineligible returns
// (R14).
func TestQuote_IneligibleCarriesNoFabricatedNumbers(t *testing.T) {
	db := setupHospitalityServiceTestDB(t)
	svc := NewDeliveryService(db, nil)
	business := createTestHospitalityBusiness(t, db, "quote-honesty-disabled")

	// Delivery explicitly disabled → ineligible
	createDeliverySettings(t, db, business.ID, func(s *database.DeliverySettings) {
		s.DeliveryEnabled = false
		s.InHouseDeliveryEnabled = false
		s.EstimatedPrepTime = 25 // would be fabricated if leaked
		s.FlatDeliveryFee = 800  // $8.00 — would be fabricated if leaked
	})

	quote, err := svc.QuoteDelivery(business.ID, DeliveryQuoteRequest{
		OrderSubtotal:   20,
		DeliveryAddress: database.DeliveryAddress{City: "x", PostalCode: "1407"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if quote.Eligible {
		t.Fatal("expected ineligible")
	}
	if quote.ReasonCode != "delivery_disabled" {
		t.Fatalf("expected reason_code delivery_disabled, got %q", quote.ReasonCode)
	}
	if quote.DeliveryFee != 0 {
		t.Fatalf("ineligible quote has non-zero DeliveryFee: %v", quote.DeliveryFee)
	}
	if quote.EstimatedTotalMinutes != 0 {
		t.Fatalf("ineligible quote has non-zero EstimatedTotalMinutes: %v", quote.EstimatedTotalMinutes)
	}
	if quote.EstimatedPrepTime != 0 {
		t.Fatalf("ineligible quote has non-zero EstimatedPrepTime: %v", quote.EstimatedPrepTime)
	}
	if quote.EstimatedDeliveryTime != nil {
		t.Fatalf("ineligible quote has non-nil EstimatedDeliveryTime: %v", quote.EstimatedDeliveryTime)
	}
}

// TestQuote_CapacityReachedCarriesNoFabricatedNumbers exercises a second
// ineligible path: delivery_unavailable (capacity) must also carry zero fabricated numbers.
func TestQuote_CapacityReachedCarriesNoFabricatedNumbers(t *testing.T) {
	db := setupHospitalityServiceTestDB(t)
	svc := NewDeliveryService(db, nil)
	business := createTestHospitalityBusiness(t, db, "quote-honesty-capacity")

	createDeliverySettings(t, db, business.ID, func(s *database.DeliverySettings) {
		s.MaxConcurrentDeliveries = 1
		s.EstimatedPrepTime = 25
		s.FlatDeliveryFee = 800
	})
	// Create one active order to saturate capacity
	active := database.DeliveryOrder{
		BusinessID:     business.ID,
		BillID:         1,
		DeliveryNumber: "DEL-HON-CAP0000001",
		DeliveryType:   database.DeliveryTypeInHouse,
		Status:         database.DeliveryStatusPreparing,
		CustomerName:   "g",
		CustomerPhone:  "1",
	}
	if err := db.Create(&active).Error; err != nil {
		t.Fatal(err)
	}

	quote, err := svc.QuoteDelivery(business.ID, DeliveryQuoteRequest{
		OrderSubtotal:   30,
		DeliveryAddress: database.DeliveryAddress{City: "x", PostalCode: "1407"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if quote.Eligible {
		t.Fatal("expected ineligible: capacity full")
	}
	if quote.ReasonCode != "delivery_unavailable" {
		t.Fatalf("expected reason_code delivery_unavailable, got %q", quote.ReasonCode)
	}
	if quote.DeliveryFee != 0 {
		t.Fatalf("delivery_unavailable quote has non-zero DeliveryFee: %v", quote.DeliveryFee)
	}
	if quote.EstimatedTotalMinutes != 0 {
		t.Fatalf("delivery_unavailable quote has non-zero EstimatedTotalMinutes: %v", quote.EstimatedTotalMinutes)
	}
	if quote.EstimatedPrepTime != 0 {
		t.Fatalf("delivery_unavailable quote has non-zero EstimatedPrepTime: %v", quote.EstimatedPrepTime)
	}
	if quote.EstimatedDeliveryTime != nil {
		t.Fatalf("delivery_unavailable quote has non-nil EstimatedDeliveryTime: %v", quote.EstimatedDeliveryTime)
	}
}

// TestCapacity_StaleDeliveriesDoNotCount ensures that a non-terminal delivery
// older than the staleness cutoff is excluded from the active-capacity count.
// A permanently "pending" order bricked a production business's delivery
// because the capacity limit was always reached (R5).
func TestCapacity_StaleDeliveriesDoNotCount(t *testing.T) {
	db := setupHospitalityServiceTestDB(t)
	svc := NewDeliveryService(db, nil)
	business := createTestHospitalityBusiness(t, db, "capacity-stale")

	// No ETA set, so the NULL-branch (created_at cutoff) applies.
	old := time.Now().Add(-4 * time.Hour)
	stale := database.DeliveryOrder{
		BusinessID:     business.ID,
		BillID:         1,
		DeliveryNumber: "DEL-STALE000000001",
		DeliveryType:   database.DeliveryTypeInHouse,
		Status:         database.DeliveryStatusPending,
		CustomerName:   "g",
		CustomerPhone:  "1",
	}
	if err := db.Create(&stale).Error; err != nil {
		t.Fatal(err)
	}
	// Backdate created_at/updated_at past the staleness cutoff.
	if err := db.Model(&database.DeliveryOrder{}).Where("id = ?", stale.ID).
		UpdateColumns(map[string]interface{}{"created_at": old, "updated_at": old}).Error; err != nil {
		t.Fatal(err)
	}

	count, err := svc.activeDeliveryCount(db, business.ID)
	if err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("stale delivery still counted toward capacity: got %d, want 0", count)
	}
}

// TestCapacity_FreshDeliveriesStillCount ensures normal (non-stale) active
// deliveries are still counted — the cutoff must not accidentally exclude
// real in-flight orders.
func TestCapacity_FreshDeliveriesStillCount(t *testing.T) {
	db := setupHospitalityServiceTestDB(t)
	svc := NewDeliveryService(db, nil)
	business := createTestHospitalityBusiness(t, db, "capacity-fresh")

	fresh := database.DeliveryOrder{
		BusinessID:     business.ID,
		BillID:         2,
		DeliveryNumber: "DEL-FRESH00000001",
		DeliveryType:   database.DeliveryTypeInHouse,
		Status:         database.DeliveryStatusPreparing,
		CustomerName:   "g",
		CustomerPhone:  "1",
	}
	if err := db.Create(&fresh).Error; err != nil {
		t.Fatal(err)
	}

	count, err := svc.activeDeliveryCount(db, business.ID)
	if err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("expected 1 fresh active delivery, got %d", count)
	}
}

// TestCapacity_FreshDeliveryWithEtaStillCounts verifies that a delivery whose
// estimated_delivery_time is in the future still counts toward capacity
// (the staleness predicate must not prematurely exclude it).
func TestCapacity_FreshDeliveryWithEtaStillCounts(t *testing.T) {
	db := setupHospitalityServiceTestDB(t)
	svc := NewDeliveryService(db, nil)
	business := createTestHospitalityBusiness(t, db, "capacity-fresh-eta")

	future := time.Now().Add(30 * time.Minute)
	fresh := database.DeliveryOrder{
		BusinessID:            business.ID,
		BillID:                3,
		DeliveryNumber:        "DEL-FRESHETA000001",
		DeliveryType:          database.DeliveryTypeInHouse,
		Status:                database.DeliveryStatusInTransit,
		CustomerName:          "g",
		CustomerPhone:         "1",
		EstimatedDeliveryTime: &future,
	}
	if err := db.Create(&fresh).Error; err != nil {
		t.Fatal(err)
	}

	count, err := svc.activeDeliveryCount(db, business.ID)
	if err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("expected 1 fresh active delivery with ETA, got %d", count)
	}
}

// TestCapacity_StaleDeliveryWithExpiredEtaDoesNotCount pins the ETA-branch of
// the pending staleness predicate: a pending delivery whose
// estimated_delivery_time is more than 45 minutes in the past is excluded
// from the active-capacity count. Later lifecycle statuses always count.
func TestCapacity_StaleDeliveryWithExpiredEtaDoesNotCount(t *testing.T) {
	db := setupHospitalityServiceTestDB(t)
	svc := NewDeliveryService(db, nil)
	business := createTestHospitalityBusiness(t, db, "capacity-stale-eta")

	// ETA expired 90 minutes ago — well past the 45-minute grace window.
	expiredETA := time.Now().Add(-90 * time.Minute)
	stale := database.DeliveryOrder{
		BusinessID:            business.ID,
		BillID:                4,
		DeliveryNumber:        "DEL-STALEETA000001",
		DeliveryType:          database.DeliveryTypeInHouse,
		Status:                database.DeliveryStatusPending,
		CustomerName:          "g",
		CustomerPhone:         "1",
		EstimatedDeliveryTime: &expiredETA,
	}
	if err := db.Create(&stale).Error; err != nil {
		t.Fatal(err)
	}

	count, err := svc.activeDeliveryCount(db, business.ID)
	if err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("delivery with expired ETA still counted toward capacity: got %d, want 0", count)
	}
}

// TestQuote_BelowMinimumCarriesHonestFee encodes the intended carve-out for
// below_minimum quotes: unlike other ineligible paths, a below_minimum quote
// MUST populate DeliveryFee and EstimatedDeliveryTime so the guest UI can
// display an accurate "you need $X more to unlock $Y delivery" prompt.
// A future refactor must not accidentally zero these fields on this path.
func TestQuote_BelowMinimumCarriesHonestFee(t *testing.T) {
	db := setupHospitalityServiceTestDB(t)
	svc := NewDeliveryService(db, nil)
	business := createTestHospitalityBusiness(t, db, "quote-below-min")

	// Use the same defaults as the zone-and-minimum test: zone minimum=$20.00,
	// zone fee=$7.00, settings minimum=$10.00 (zone wins via maxFloat).
	createDeliverySettings(t, db, business.ID, nil)
	createDeliveryZone(t, db, business.ID, "Downtown", nil)

	// Subtotal $18 is below the $20.00 zone minimum.
	quote, err := svc.QuoteDelivery(business.ID, DeliveryQuoteRequest{
		OrderSubtotal: 18,
		DeliveryAddress: database.DeliveryAddress{
			Street:     "123 Main Street",
			City:       "Testville",
			State:      "CA",
			PostalCode: "12345",
			Country:    "US",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if quote.Eligible {
		t.Fatal("expected ineligible: below minimum")
	}
	if quote.ReasonCode != "below_minimum" {
		t.Fatalf("expected reason_code below_minimum, got %q", quote.ReasonCode)
	}
	// Core carve-out assertions: fee and ETA must be present on below_minimum.
	if quote.DeliveryFee <= 0 {
		t.Fatalf("below_minimum quote has zero/negative DeliveryFee: %v", quote.DeliveryFee)
	}
	if quote.EstimatedDeliveryTime == nil {
		t.Fatal("below_minimum quote has nil EstimatedDeliveryTime")
	}
}
