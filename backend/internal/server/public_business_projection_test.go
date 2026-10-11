package server

import (
	"testing"
	"time"

	"github.com/stdevmac/payverge/backend/internal/database"
)

func newTestBusinessForProjection() *database.Business {
	return &database.Business{ID: 1, Name: "Cafe", CustomURL: "cafe", DefaultCurrency: "USD", IsActive: true}
}

func TestPublicBusinessProjectionOmitsSensitiveFields(t *testing.T) {
	proj := publicBusinessProjection(newTestBusinessForProjection())

	forbidden := []string{
		"owner_address", "settlement_address", "tipping_address",
		"email", "user_id", "is_active", "closed_at",
	}
	for _, k := range forbidden {
		if _, ok := proj[k]; ok {
			t.Errorf("projection must NOT contain sensitive key %q", k)
		}
	}

	required := []string{"id", "name", "logo", "custom_url", "default_currency", "design_settings"}
	for _, k := range required {
		if _, ok := proj[k]; !ok {
			t.Errorf("projection must contain public key %q", k)
		}
	}
}

func TestPublicBusinessProjectionAIAvailable(t *testing.T) {
	suspended := newTestBusinessForProjection()
	suspended.IsActive = false
	suspended.AiSettings.AiEnabled = true
	suspended.AiSettings.BusinessPageAiEnabled = true
	proj := publicBusinessProjection(suspended)
	got, ok := proj["ai_available"]
	if !ok {
		t.Fatal("storefront projection must emit ai_available so locked venues do not offer Mozo")
	}
	if got != false {
		t.Fatalf("suspended + AI toggles: ai_available=%v, want false", got)
	}

	active := newTestBusinessForProjection()
	active.AiSettings.AiEnabled = true
	active.AiSettings.BusinessPageAiEnabled = true
	if publicBusinessProjection(active)["ai_available"] != true {
		t.Fatalf("active + enabled: ai_available=%v, want true", publicBusinessProjection(active)["ai_available"])
	}
}

// #944 guard: the storefront projection is the /b page's gate, so it must also
// honour the operator's storefront-only opt-out (ai_settings.business_page_ai_enabled,
// gorm default:false). AI being enabled alone must not switch Mozo on for a
// venue whose owner never enabled it on the business page.
func TestPublicBusinessProjectionAIAvailableHonorsStorefrontToggle(t *testing.T) {
	cases := []struct {
		name          string
		suspended     bool
		aiEnabled     bool
		storefrontOn  bool
		wantAvailable bool
	}{
		{"storefront opt-out", false, true, false, false},
		{"storefront opt-in", false, true, true, true},
		{"ai disabled", false, false, true, false},
		{"suspended storefront opt-in", true, true, true, false},
		{"suspended storefront opt-out", true, true, false, false},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			biz := newTestBusinessForProjection()
			biz.IsActive = !tc.suspended
			biz.AiSettings.AiEnabled = tc.aiEnabled
			biz.AiSettings.BusinessPageAiEnabled = tc.storefrontOn
			got := publicBusinessProjection(biz)["ai_available"]
			if got != tc.wantAvailable {
				t.Fatalf("ai_available=%v, want %v", got, tc.wantAvailable)
			}
		})
	}
}

func TestPublicBusinessProjectionStorefrontKeys(t *testing.T) {
	proj := publicBusinessProjection(newTestBusinessForProjection())
	for _, k := range []string{
		"id", "name", "logo", "address", "custom_url", "phone", "website",
		"banner_images", "default_currency", "display_currency", "default_language",
		"design_settings", "ai_settings", "welcome_message", "show_gallery",
		"timezone", "ai_available",
	} {
		if _, ok := proj[k]; !ok {
			t.Errorf("storefront key %q missing from projection", k)
		}
	}
}

func TestPublicBusinessProjectionKeysAreBlessed(t *testing.T) {
	// Allow-list of every key publicBusinessProjection is permitted to emit to
	// non-owners. Adding a key to the projection without blessing it here fails
	// this test on purpose: it forces a reviewer to confirm the new field is
	// non-sensitive before it can reach a cross-tenant response.
	blessedPublicKeys := map[string]struct{}{
		"id": {}, "name": {}, "logo": {}, "address": {}, "description": {},
		"custom_url": {}, "phone": {}, "website": {}, "social_media": {},
		"banner_images": {}, "show_reviews": {}, "google_reviews_enabled": {},
		"google_place_id": {}, "google_business_name": {}, "google_review_link": {},
		"google_business_url": {}, "default_currency": {}, "display_currency": {},
		"default_language": {}, "design_settings": {}, "welcome_message": {},
		"about_story": {}, "show_welcome_message": {}, "show_about_story": {},
		"show_gallery": {}, "show_operating_hours": {}, "show_special_features": {},
		"ai_settings": {}, "created_at": {}, "updated_at": {},
		"tax_rate": {}, "service_fee_rate": {}, "timezone": {},
		// #549: storefront noindex needs the existing demo flags, not slug guesses.
		"is_demo": {}, "kind": {},
		// #944: diner AI affordance is computed, not a leftover toggle.
		"ai_available": {},
		// OSS A1.1: "llm" | "basic" — whether an LLM provider is configured;
		// instance-level, non-sensitive, no business data.
		"ai_waiter_mode": {},
	}

	proj := publicBusinessProjection(newTestBusinessForProjection())
	for k := range proj {
		if _, ok := blessedPublicKeys[k]; !ok {
			t.Errorf("projection emits non-blessed key %q — if intentional, add it to blessedPublicKeys after confirming it is non-sensitive", k)
		}
	}
}

// PARITY-2: the storefront/open-closed badge and reservation slots compute in
// the venue's IANA timezone; the public projection must emit it (it was only in
// staffBusinessProjection before). Cross-tenant-safe: timezone is non-sensitive.
func TestPublicBusinessProjectionEmitsTimezone(t *testing.T) {
	biz := &database.Business{ID: 1, Name: "Cafe", CustomURL: "cafe", DefaultCurrency: "USD", Timezone: "Asia/Dubai"}
	proj := publicBusinessProjection(biz)

	got, ok := proj["timezone"]
	if !ok {
		t.Fatalf("projection must emit timezone")
	}
	if got != "Asia/Dubai" {
		t.Errorf("timezone = %v, want Asia/Dubai", got)
	}
}

func TestPublicBusinessProjectionEmitsDemoFlags(t *testing.T) {
	biz := &database.Business{
		ID: 1, Name: "Demo Lounge", CustomURL: "payverge-ai-pro-demo-lounge",
		DefaultCurrency: "USD", IsDemo: true, Kind: database.BusinessKindDemo,
	}
	proj := publicBusinessProjection(biz)
	if got, ok := proj["is_demo"]; !ok || got != true {
		t.Errorf("is_demo = %v, want true", got)
	}
	if got, ok := proj["kind"]; !ok || got != database.BusinessKindDemo {
		t.Errorf("kind = %v, want %q", got, database.BusinessKindDemo)
	}

	real := publicBusinessProjection(newTestBusinessForProjection())
	if got, ok := real["is_demo"]; !ok || got != false {
		t.Errorf("real tenant is_demo = %v, want false", got)
	}
}

// The staff dashboard derives its read-only state from the admin lock alone
// (is_active + closed_at), so the staff projection must carry both halves.
func TestStaffBusinessProjectionEmitsAdminLockFields(t *testing.T) {
	closedAt := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	biz := &database.Business{ID: 1, Name: "Cafe", IsActive: true, ClosedAt: &closedAt}
	proj := staffBusinessProjection(biz)

	if got, ok := proj["is_active"]; !ok || got != true {
		t.Errorf("is_active = %v, want true", got)
	}
	got, ok := proj["closed_at"].(*time.Time)
	if !ok || got == nil || !got.Equal(closedAt) {
		t.Errorf("closed_at = %v, want %v", proj["closed_at"], closedAt)
	}
}
