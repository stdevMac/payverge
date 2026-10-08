package database

import "testing"

// A venue created without a slug gets one from its name, so publishing it
// makes /b/<slug> and "/" work without a separate Custom URL step.
func TestCreateBusinessAssignsCustomURLFromName(t *testing.T) {
	setupTestDB(t)

	first := &Business{BusinessId: "slug-1", Name: "Café Aurora", BusinessPageEnabled: true, IsActive: true}
	if err := CreateBusiness(first); err != nil {
		t.Fatalf("CreateBusiness: %v", err)
	}
	if first.CustomURL != "cafe-aurora" {
		t.Fatalf("custom_url = %q, want cafe-aurora", first.CustomURL)
	}

	second := &Business{BusinessId: "slug-2", Name: "Cafe Aurora", BusinessPageEnabled: true, IsActive: true}
	if err := CreateBusiness(second); err != nil {
		t.Fatalf("CreateBusiness: %v", err)
	}
	if second.CustomURL != "cafe-aurora-2" {
		t.Fatalf("custom_url = %q, want cafe-aurora-2", second.CustomURL)
	}

	chosen := &Business{BusinessId: "slug-3", Name: "Cafe Aurora", CustomURL: "my-pick", IsActive: true}
	if err := CreateBusiness(chosen); err != nil {
		t.Fatalf("CreateBusiness: %v", err)
	}
	if chosen.CustomURL != "my-pick" {
		t.Fatalf("explicit custom_url overwritten: %q", chosen.CustomURL)
	}

	v, err := FindPublishedVenue("cafe-aurora")
	if err != nil || v == nil || v.ID != first.ID {
		t.Fatalf("new venue not routable: %+v %v", v, err)
	}
}

func TestCreateBusinessIdempotentlyAssignsCustomURL(t *testing.T) {
	setupTestDB(t)
	if err := db.AutoMigrate(&BusinessCreationRequest{}); err != nil {
		t.Fatalf("migrate ledger: %v", err)
	}
	b := &Business{BusinessId: "slug-idem", Name: "Admin Grill", IsActive: true}
	got, _, err := CreateBusinessIdempotently(b, "owner", "key", "payload")
	if err != nil {
		t.Fatalf("CreateBusinessIdempotently: %v", err)
	}
	// "admin" is a reserved first segment, so the slug is namespaced.
	if got.CustomURL != "venue-admin-grill" {
		t.Fatalf("custom_url = %q, want venue-admin-grill", got.CustomURL)
	}
}
