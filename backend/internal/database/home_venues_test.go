package database

import (
	"fmt"
	"testing"
)

func seedHomeVenues(t *testing.T) {
	t.Helper()
	setupTestDB(t)
	db.Create(&Business{BusinessId: "v-b", Name: "Bravo", CustomURL: "bravo", BusinessPageEnabled: true, IsActive: true, Logo: "https://cdn.example/b.png", Address: BusinessAddress{City: "Rosario"}})
	db.Create(&Business{BusinessId: "v-a", Name: "Alpha", CustomURL: "Alpha-Bistro", BusinessPageEnabled: true, IsActive: true})
	db.Create(&Business{BusinessId: "v-draft", Name: "Draft", CustomURL: "draft", BusinessPageEnabled: false, IsActive: true})
	db.Create(&Business{BusinessId: "v-noslug", Name: "NoSlug", CustomURL: "", BusinessPageEnabled: true, IsActive: true})
	db.Create(&Business{BusinessId: "v-test", Name: "Fixture", CustomURL: "fixture", BusinessPageEnabled: true, IsActive: true, Kind: BusinessKindTest})
	db.Create(&Business{BusinessId: "v-demo", Name: "Showroom", CustomURL: "showroom", BusinessPageEnabled: true, IsActive: true, Kind: BusinessKindDemo})
	inactive := &Business{BusinessId: "v-off", Name: "Closed", CustomURL: "closed", BusinessPageEnabled: true, IsActive: true}
	db.Create(inactive)
	db.Model(&Business{}).Where("id = ?", inactive.ID).Update("is_active", false)
}

func TestListPublishedVenuesAppliesPublishGate(t *testing.T) {
	seedHomeVenues(t)
	got, err := ListPublishedVenues(100)
	if err != nil {
		t.Fatalf("ListPublishedVenues: %v", err)
	}
	var names []string
	for _, v := range got {
		names = append(names, v.Name)
	}
	want := []string{"Alpha", "Bravo", "Showroom"}
	if len(names) != len(want) {
		t.Fatalf("want %v, got %v", want, names)
	}
	for i := range want {
		if names[i] != want[i] {
			t.Fatalf("want %v, got %v", want, names)
		}
	}
	if got[1].City != "Rosario" || got[1].Logo == "" || got[1].ID == 0 {
		t.Fatalf("projection missing fields: %+v", got[1])
	}
	if lim, _ := ListPublishedVenues(1); len(lim) != 1 {
		t.Fatalf("limit not applied: %d", len(lim))
	}
}

func TestFindPublishedVenueBySlugOrID(t *testing.T) {
	seedHomeVenues(t)
	v, err := FindPublishedVenue("alpha-bistro")
	if err != nil || v == nil || v.Name != "Alpha" {
		t.Fatalf("case-insensitive slug: %+v %v", v, err)
	}
	byID, err := FindPublishedVenue(" " + fmt.Sprint(v.ID) + " ")
	if err != nil || byID == nil || byID.CustomURL != "Alpha-Bistro" {
		t.Fatalf("numeric id: %+v %v", byID, err)
	}
	byBusinessID, err := FindPublishedVenue("v-a")
	if err != nil || byBusinessID == nil || byBusinessID.Name != "Alpha" {
		t.Fatalf("business_id slug: %+v %v", byBusinessID, err)
	}
	for _, ref := range []string{"", "draft", "fixture", "closed", "missing", "999999", "v-draft", "v-test", "v-off", "v-noslug"} {
		got, err := FindPublishedVenue(ref)
		if err != nil || got != nil {
			t.Fatalf("ref %q must not resolve, got %+v %v", ref, got, err)
		}
	}
}

// A storefront slug wins over another venue's business_id with the same text.
func TestFindPublishedVenuePrefersCustomURLOverBusinessID(t *testing.T) {
	seedHomeVenues(t)
	db.Create(&Business{BusinessId: "bravo", Name: "Shadow", CustomURL: "shadow", BusinessPageEnabled: true, IsActive: true})
	v, err := FindPublishedVenue("bravo")
	if err != nil || v == nil || v.Name != "Bravo" {
		t.Fatalf("custom_url must win: %+v %v", v, err)
	}
}
