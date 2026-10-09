package services

import (
	"encoding/json"
	"testing"

	"github.com/stdevmac/payverge/backend/internal/database"
)

func zone(id uint, name string, feeDollars float64, minDollars float64, eta int, boundaries string) DeliveryZoneDTO {
	return DeliveryZoneDTO{
		ID: id, Name: name, DeliveryFee: feeDollars, MinimumOrderAmount: minDollars,
		EstimatedTime: eta, IsActive: true, Boundaries: json.RawMessage(boundaries),
	}
}

func addr(city, postal string) database.DeliveryAddress {
	return database.DeliveryAddress{Street: "18 Demo Market St", City: city, PostalCode: postal, Country: "AR"}
}

func TestZoneCoverageReady(t *testing.T) {
	if zoneCoverageReady(zone(1, "GeoJSON", 2, 0, 20, `{"type":"Polygon","coordinates":[]}`)) {
		t.Fatal("GeoJSON must not count as matchable coverage")
	}
	if zoneCoverageReady(zone(1, "Empty", 2, 0, 20, `{}`)) {
		t.Fatal("empty object must not count as matchable coverage")
	}
	ready := zone(1, "NY", 2, 0, 20, `{"postal_codes":["100*"],"cities":["New York"]}`)
	if !zoneCoverageReady(ready) {
		t.Fatal("postal/city matcher JSON must be coverage-ready")
	}
	ready.IsActive = false
	if zoneCoverageReady(ready) {
		t.Fatal("inactive zones are not coverage-ready")
	}
}

func TestZoneMatch_EmptyBoundariesMatchNothing(t *testing.T) {
	zones := []DeliveryZoneDTO{zone(1, "Broken", 2, 0, 20, `{}`)}
	if got := resolveDeliveryZone(zones, addr("Buenos Aires", "1407")); got != nil {
		t.Fatalf("empty boundaries must match nothing, got zone %d", got.ID)
	}
}

func TestZoneMatch_PostalNormalizationAndWildcard(t *testing.T) {
	zones := []DeliveryZoneDTO{zone(1, "Near", 2, 0, 20, `{"postal_codes":["1407*"]}`)}
	for _, postal := range []string{"1407", " 1407 ", "C1407ABC", "1407-XYZ"} {
		if resolveDeliveryZone(zones, addr("x", postal)) == nil {
			t.Fatalf("postal %q should match wildcard 1407*", postal)
		}
	}
	if resolveDeliveryZone(zones, addr("x", "2407")) != nil {
		t.Fatal("2407 must not match 1407*")
	}
}

func TestZoneMatch_ExactPostalIsNotSubstring(t *testing.T) {
	// Old matcher used strings.Contains: "140" matched "1407". Exact codes
	// (no trailing *) must compare on the normalized whole code.
	zones := []DeliveryZoneDTO{zone(1, "Near", 2, 0, 20, `{"postal_codes":["140"]}`)}
	if resolveDeliveryZone(zones, addr("x", "1407")) != nil {
		t.Fatal("exact code 140 must not substring-match 1407")
	}
	if resolveDeliveryZone(zones, addr("x", "140")) == nil {
		t.Fatal("exact code 140 must match 140")
	}
}

func TestZoneMatch_CityDiacriticsInsensitive(t *testing.T) {
	zones := []DeliveryZoneDTO{zone(1, "Centro", 2, 0, 20, `{"cities":["San Martín"]}`)}
	if resolveDeliveryZone(zones, addr("san martin", "")) == nil {
		t.Fatal("city match must be diacritics/case-insensitive")
	}
}

func TestZoneMatch_OverlapLowestFeeHighestMinimum(t *testing.T) {
	zones := []DeliveryZoneDTO{
		zone(1, "Far", 5, 10, 40, `{"postal_codes":["1407*"]}`),
		zone(2, "Near", 2, 25, 20, `{"postal_codes":["1407*"]}`),
	}
	got := resolveDeliveryZone(zones, addr("x", "1407"))
	if got == nil {
		t.Fatal("expected a match")
	}
	if got.DeliveryFee != 2 {
		t.Fatalf("lowest fee wins: got %.2f", got.DeliveryFee)
	}
	if got.MinimumOrderAmount != 25 {
		t.Fatalf("highest minimum applies: got %.2f", got.MinimumOrderAmount)
	}
	if got.EstimatedTime != 20 {
		t.Fatalf("ETA from fee-winning zone: got %d", got.EstimatedTime)
	}
}

func TestZoneMatch_FeeTieBreaksToFasterETA(t *testing.T) {
	zones := []DeliveryZoneDTO{
		zone(1, "A", 3, 0, 45, `{"postal_codes":["1407*"]}`),
		zone(2, "B", 3, 0, 25, `{"postal_codes":["1407*"]}`),
	}
	got := resolveDeliveryZone(zones, addr("x", "1407"))
	if got == nil || got.EstimatedTime != 25 {
		t.Fatalf("fee tie must break to faster ETA, got %+v", got)
	}
}

func TestZoneMatch_InactiveSkipped(t *testing.T) {
	z := zone(1, "Off", 2, 0, 20, `{"postal_codes":["1407*"]}`)
	z.IsActive = false
	if resolveDeliveryZone([]DeliveryZoneDTO{z}, addr("x", "1407")) != nil {
		t.Fatal("inactive zones must not match")
	}
}

func TestZoneMatch_NullBoundariesMatchNothing(t *testing.T) {
	zones := []DeliveryZoneDTO{zone(1, "Null", 2, 0, 20, `null`)}
	if got := resolveDeliveryZone(zones, addr("Buenos Aires", "1407")); got != nil {
		t.Fatalf("null boundaries must match nothing, got zone %d", got.ID)
	}
}

func TestZoneMatch_MalformedJSONBoundariesMatchNothing(t *testing.T) {
	zones := []DeliveryZoneDTO{zone(1, "Broken", 2, 0, 20, `{not valid json`)}
	if got := resolveDeliveryZone(zones, addr("Buenos Aires", "1407")); got != nil {
		t.Fatalf("malformed JSON boundaries must match nothing, got zone %d", got.ID)
	}
}

func TestZoneMatch_LooksLikeStreet(t *testing.T) {
	cases := []struct {
		street string
		want   bool
	}{
		{"18 Demo Market St", true},
		{"1 QA Verification Way", true},
		{"999999 This Street Does Not Exist", true},
		{"12 Court Street, Apt 4B", true},
		{"", false},
		{"   ", false},
		{"Main Street", false},
		{"12345", false},
		{"QA Verification Way", false},
	}
	for _, tc := range cases {
		if got := looksLikeStreet(tc.street); got != tc.want {
			t.Fatalf("looksLikeStreet(%q)=%v, want %v", tc.street, got, tc.want)
		}
	}
}

func TestCityStateContradicts_NewYorkRequiresNY(t *testing.T) {
	if cityStateContradicts(database.DeliveryAddress{City: "New York", State: "NY", Country: "US"}) {
		t.Fatal("New York + NY must not contradict")
	}
	if cityStateContradicts(database.DeliveryAddress{City: "New York", State: "New York", Country: "US"}) {
		t.Fatal("New York + New York (state alias) must not contradict")
	}
	if !cityStateContradicts(database.DeliveryAddress{City: "New York", State: "CA", Country: "US"}) {
		t.Fatal("New York + CA must contradict")
	}
	if cityStateContradicts(database.DeliveryAddress{City: "New York", Country: "US"}) {
		t.Fatal("omitted state is not a contradiction")
	}
	if cityStateContradicts(database.DeliveryAddress{City: "Brooklyn", State: "CA", Country: "US"}) {
		t.Fatal("unknown cities are not gazetteered")
	}
}

func TestZoneMatch_ImpossibleAddressesAndNYControl(t *testing.T) {
	// Downtown mirrors the production demo shape (city New York + 100xx
	// postals). 10118 is a real NYC ZIP but is NOT in this zone so a
	// supplied unmatched postal cannot inherit the city token.
	downtown := []DeliveryZoneDTO{
		zone(1, "Downtown", 4.99, 15, 32, `{"postal_codes":["100*"],"cities":["New York","Brooklyn"]}`),
	}

	cases := []struct {
		name    string
		addr    database.DeliveryAddress
		wantHit bool
	}{
		{
			name: "fabricated NY city with CA state and unmatched postal",
			addr: database.DeliveryAddress{
				Street: "1 QA Verification Way", City: "New York", State: "CA",
				PostalCode: "99999", Country: "US",
			},
		},
		{
			name: "fabricated street with NY postal outside the zone",
			addr: database.DeliveryAddress{
				Street: "999999 This Street Does Not Exist", City: "New York",
				State: "NY", PostalCode: "10118", Country: "US",
			},
		},
		{
			name: "Washington DC control stays outside",
			addr: database.DeliveryAddress{
				Street: "1600 Pennsylvania Ave NW", City: "Washington",
				State: "DC", PostalCode: "20500", Country: "US",
			},
		},
		{
			name: "legitimate NY in-zone address",
			addr: database.DeliveryAddress{
				Street: "18 Demo Market St", City: "New York", State: "NY",
				PostalCode: "10013", Country: "US",
			},
			wantHit: true,
		},
		{
			name: "city-only match still works when postal is omitted",
			addr: database.DeliveryAddress{
				Street: "12 Court Street", City: "Brooklyn", Country: "US",
			},
			wantHit: true,
		},
		{
			name: "postal-only match still works with a street-shaped line",
			addr: database.DeliveryAddress{
				Street: "88 Hudson St", City: "Elsewhere", PostalCode: "10014", Country: "US",
			},
			wantHit: true,
		},
		{
			name: "city match is not enough when a non-matching postal is supplied",
			addr: database.DeliveryAddress{
				Street: "18 Demo Market St", City: "New York", State: "NY",
				PostalCode: "99999", Country: "US",
			},
		},
		{
			name: "missing street is rejected even with matching city",
			addr: database.DeliveryAddress{
				City: "New York", State: "NY", Country: "US",
			},
		},
		{
			name: "letter-only street is rejected",
			addr: database.DeliveryAddress{
				Street: "Main Street", City: "New York", Country: "US",
			},
		},
		{
			name: "apartment is extra; street still has to look like a street",
			addr: database.DeliveryAddress{
				Street: "18 Demo Market St", Apartment: "Apt 4B",
				City: "New York", State: "NY", PostalCode: "10013", Country: "US",
			},
			wantHit: true,
		},
		{
			name: "apartment alone is not a street",
			addr: database.DeliveryAddress{
				Apartment: "Apt 4B", City: "New York", State: "NY",
				PostalCode: "10013", Country: "US",
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := resolveDeliveryZone(downtown, tc.addr)
			if tc.wantHit && got == nil {
				t.Fatalf("expected Downtown match, got none")
			}
			if !tc.wantHit && got != nil {
				t.Fatalf("expected no match, got zone %q", got.Name)
			}
		})
	}
}

func TestZoneMatch_CABACityAliasesBuenosAires(t *testing.T) {
	// Live Bodegón zone lists city "Buenos Aires"; guests type "CABA".
	zones := []DeliveryZoneDTO{
		zone(1, "CABA", 29, 150, 35, `{"postal_codes":["C10*"],"cities":["Buenos Aires"]}`),
	}
	cabaCityOnly := database.DeliveryAddress{
		Street: "Defensa 1148", City: "CABA", Country: "AR",
	}
	if resolveDeliveryZone(zones, cabaCityOnly) == nil {
		t.Fatal("city CABA must match a Buenos Aires zone when postal is omitted")
	}

	capital := database.DeliveryAddress{
		Street: "Defensa 1148", City: "Capital Federal", Country: "AR",
	}
	if resolveDeliveryZone(zones, capital) == nil {
		t.Fatal("Capital Federal must alias to Buenos Aires")
	}
}

func TestZoneMatch_C10WildcardMatchesBodegonPostal(t *testing.T) {
	zones := []DeliveryZoneDTO{
		zone(1, "CABA", 29, 150, 35, `{"postal_codes":["C10*","C14*"],"cities":["Buenos Aires"]}`),
	}
	addr := database.DeliveryAddress{
		Street: "Defensa 1148", City: "CABA", PostalCode: "C1065", Country: "AR",
	}
	if resolveDeliveryZone(zones, addr) == nil {
		t.Fatal("C10* must match the Bodegón postal C1065")
	}
}

func TestZoneMatch_GeoJSONBoundariesNeverMatchAnyAddress(t *testing.T) {
	// Production demo rows shipped GeoJSON polygons the matcher cannot read.
	// Prove the code gap: even the venue's own CABA address is out of zone
	// until the demo seed heals boundaries to postal_codes/cities JSON.
	zones := []DeliveryZoneDTO{
		zone(1, "CABA", 29, 150, 35, `{"type":"Polygon","coordinates":[]}`),
	}
	own := database.DeliveryAddress{
		Street: "Defensa 1148", City: "Buenos Aires", State: "CABA",
		PostalCode: "C1065", Country: "AR",
	}
	if got := resolveDeliveryZone(zones, own); got != nil {
		t.Fatalf("GeoJSON-only zone must not match via postal/city, got zone %d", got.ID)
	}
}

func TestAddressMatchesVenue_BodegonOwnStreet(t *testing.T) {
	biz := &database.Business{
		Address: database.BusinessAddress{
			Street: "Defensa 1148, San Telmo", City: "Buenos Aires",
			State: "CABA", PostalCode: "C1065", Country: "AR",
		},
	}
	guest := database.DeliveryAddress{
		Street: "Defensa 1148", City: "CABA", PostalCode: "C1065", Country: "AR",
	}
	if !addressMatchesVenue(guest, biz) {
		t.Fatal("guest quoting the venue's own Defensa 1148 / C1065 must match the business address")
	}

	ushuaia := database.DeliveryAddress{
		Street: "San Martín 100", City: "Ushuaia", PostalCode: "V9410", Country: "AR",
	}
	if addressMatchesVenue(ushuaia, biz) {
		t.Fatal("Ushuaia must not match the Bodegón venue address")
	}
}

// bodegonVenue is the live #892 venue: Defensa 1148, San Telmo / C1065.
func bodegonVenue() *database.Business {
	return &database.Business{
		Address: database.BusinessAddress{
			Street: "Defensa 1148, San Telmo", City: "Buenos Aires",
			State: "CABA", PostalCode: "C1065", Country: "AR",
		},
	}
}

func bodegonOwnAddress() database.DeliveryAddress {
	return database.DeliveryAddress{
		Street: "Defensa 1148", City: "CABA", PostalCode: "C1065", Country: "AR",
	}
}

// #892 repair A: an operator who lists C14* and not C1065 has EXCLUDED the
// venue's own postal code. The own-address fallback must honor that, not grant
// itself the excluded zone's fee.
func TestVenueFallback_FailsClosedWhenPostalRulesExcludeTheVenue(t *testing.T) {
	zones := []DeliveryZoneDTO{
		zone(1, "Palermo", 99, 0, 40, `{"postal_codes":["C14*"]}`),
	}
	own := bodegonOwnAddress()
	if got := resolveDeliveryZone(zones, own); got != nil {
		t.Fatalf("precondition: C1065 must not match a C14* zone, got zone %d", got.ID)
	}
	if got := venueFallbackZone(zones, own); got != nil {
		t.Fatalf("excluded venue postal must fail closed, got zone %d at fee %.2f", got.ID, got.DeliveryFee)
	}
}

// #892 repair A: same shape on the city dimension.
func TestVenueFallback_FailsClosedWhenCityRulesNameAnotherCity(t *testing.T) {
	zones := []DeliveryZoneDTO{
		zone(1, "Rosario", 99, 0, 40, `{"cities":["Rosario"]}`),
	}
	own := bodegonOwnAddress()
	if got := venueFallbackZone(zones, own); got != nil {
		t.Fatalf("a zone naming another city must fail closed, got zone %d", got.ID)
	}
}

// #892 repair A: unreadable boundaries are not an operator decision. This is
// the actual QA case — GeoJSON the matcher cannot evaluate — and the fallback
// must still quote the venue's own address.
func TestVenueFallback_FiresWhenNoZoneRuleCanJudgeTheAddress(t *testing.T) {
	zones := []DeliveryZoneDTO{
		zone(1, "CABA", 29, 150, 35, `{"type":"Polygon","coordinates":[]}`),
	}
	own := bodegonOwnAddress()
	got := venueFallbackZone(zones, own)
	if got == nil {
		t.Fatal("unmatchable GeoJSON must not block the venue's own address")
	}
	if got.ID != 1 {
		t.Fatalf("expected the venue's only zone, got %d", got.ID)
	}
}

// #892 repair A: a cities-only zone that DOES cover the venue city is coverage
// data the matcher can read but cannot apply once a postal code is supplied
// (zoneMatchesAddress short-circuits on a supplied postal). That is a data
// gap, not an exclusion — the fallback still fires.
func TestVenueFallback_FiresWhenCityRuleCoversTheVenueButPostalWasSupplied(t *testing.T) {
	zones := []DeliveryZoneDTO{
		zone(1, "CABA", 29, 150, 35, `{"cities":["Buenos Aires"]}`),
	}
	own := bodegonOwnAddress()
	if got := resolveDeliveryZone(zones, own); got != nil {
		t.Fatalf("precondition: supplied postal short-circuits a cities-only zone, got zone %d", got.ID)
	}
	if got := venueFallbackZone(zones, own); got == nil {
		t.Fatal("a zone that names the venue's own city must not read as an exclusion")
	}
}

// #892 repair B: the fallback must use the published overlap comparator —
// lowest fee, ties to the faster ETA, minimum raised to the highest — not the
// first row the zone query returned.
func TestVenueFallback_AppliesOverlapRulesInsteadOfFirstRow(t *testing.T) {
	zones := []DeliveryZoneDTO{
		zone(1, "Expensive", 99, 150, 60, `{"type":"Polygon","coordinates":[]}`),
		zone(2, "Cheap", 29, 200, 45, `{"type":"Polygon","coordinates":[]}`),
		zone(3, "CheapSlow", 29, 100, 90, `{"type":"Polygon","coordinates":[]}`),
	}
	got := venueFallbackZone(zones, bodegonOwnAddress())
	if got == nil {
		t.Fatal("expected a fallback zone")
	}
	if got.ID != 2 {
		t.Fatalf("lowest fee with the faster ETA must win, got zone %d (fee %.2f)", got.ID, got.DeliveryFee)
	}
	if got.MinimumOrderAmount != 200 {
		t.Fatalf("minimum must be the highest among overlapping zones, got %.2f", got.MinimumOrderAmount)
	}
}

func TestVenueFallback_SkipsInactiveZones(t *testing.T) {
	cheapInactive := zone(1, "CheapInactive", 5, 0, 20, `{"type":"Polygon","coordinates":[]}`)
	cheapInactive.IsActive = false
	zones := []DeliveryZoneDTO{
		cheapInactive,
		zone(2, "Active", 29, 150, 35, `{"type":"Polygon","coordinates":[]}`),
	}
	got := venueFallbackZone(zones, bodegonOwnAddress())
	if got == nil || got.ID != 2 {
		t.Fatalf("inactive zones must not be borrowed, got %+v", got)
	}
}

// #892 repair C: house numbers are compared as whole tokens. "Defensa 114" is
// the neighbor's address, not the venue's "Defensa 1148".
func TestAddressMatchesVenue_HouseNumberIsTokenExact(t *testing.T) {
	biz := bodegonVenue()

	if !addressMatchesVenue(bodegonOwnAddress(), biz) {
		t.Fatal("the venue's own Defensa 1148 must still match")
	}

	cases := []struct {
		name   string
		street string
	}{
		{"numeric prefix", "Defensa 114"},
		{"numeric extension", "Defensa 11480"},
		{"different number", "Defensa 2148"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			guest := bodegonOwnAddress()
			guest.Street = tc.street
			if addressMatchesVenue(guest, biz) {
				t.Fatalf("%q must not match the venue's Defensa 1148", tc.street)
			}
		})
	}
}

// The barrio suffix on the printed venue street and a unit suffix on the guest
// line must both still match — token-exactness is about the number only.
func TestAddressMatchesVenue_ToleratesTrailingAddressWords(t *testing.T) {
	biz := bodegonVenue()
	guest := bodegonOwnAddress()
	guest.Street = "Defensa 1148 Piso 2"
	if !addressMatchesVenue(guest, biz) {
		t.Fatal("a unit suffix on the guest line must not break the venue match")
	}
}
