package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/stdevmac/payverge/backend/internal/database"
)

func stubHomeLookups(t *testing.T, published []database.PublishedVenue) {
	t.Helper()
	prev := homeLookups
	t.Cleanup(func() { homeLookups = prev })
	homeLookups = homeVenueLookups{
		find: func(ref string) (*database.PublishedVenue, error) {
			for i := range published {
				if published[i].CustomURL == ref {
					return &published[i], nil
				}
			}
			return nil, nil
		},
		list: func(limit int) ([]database.PublishedVenue, error) {
			if len(published) > limit {
				return published[:limit], nil
			}
			return published, nil
		},
	}
}

var (
	venueA = database.PublishedVenue{ID: 1, Name: "Alpha", CustomURL: "alpha"}
	venueB = database.PublishedVenue{ID: 2, Name: "Bravo", CustomURL: "bravo"}
)

func TestResolveHome(t *testing.T) {
	cases := []struct {
		name      string
		primary   string
		published []database.PublishedVenue
		mode      string
		primarySl string
		venues    int
	}{
		{"primary env wins over many", "bravo", []database.PublishedVenue{venueA, venueB}, HomeModeVenue, "bravo", 1},
		{"unmatched primary falls back to directory", "nope", []database.PublishedVenue{venueA, venueB}, HomeModeDirectory, "", 2},
		{"single venue", "", []database.PublishedVenue{venueA}, HomeModeVenue, "alpha", 1},
		{"many venues", "", []database.PublishedVenue{venueA, venueB}, HomeModeDirectory, "", 2},
		{"zero venues", "", nil, HomeModeEmpty, "", 0},
		{"unmatched primary with one venue serves it", "nope", []database.PublishedVenue{venueA}, HomeModeVenue, "alpha", 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			stubHomeLookups(t, tc.published)
			got, err := ResolveHome(tc.primary)
			if err != nil {
				t.Fatal(err)
			}
			if got.Mode != tc.mode {
				t.Fatalf("mode = %q, want %q", got.Mode, tc.mode)
			}
			slug := ""
			if got.Primary != nil {
				slug = got.Primary.CustomURL
			}
			if slug != tc.primarySl {
				t.Fatalf("primary = %q, want %q", slug, tc.primarySl)
			}
			if len(got.Venues) != tc.venues {
				t.Fatalf("venues = %d, want %d", len(got.Venues), tc.venues)
			}
		})
	}
}

func TestGetHomeReadsPrimaryVenueEnv(t *testing.T) {
	gin.SetMode(gin.TestMode)
	stubHomeLookups(t, []database.PublishedVenue{venueA, venueB})
	t.Setenv("PRIMARY_VENUE", "alpha")

	r := gin.New()
	r.GET("/api/v1/home", GetHome)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/home", nil))

	if w.Code != http.StatusOK {
		t.Fatalf("status %d", w.Code)
	}
	if cc := w.Header().Get("Cache-Control"); cc != homeCacheControl {
		t.Fatalf("cache-control %q", cc)
	}
	var body map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body["mode"] != HomeModeVenue {
		t.Fatalf("mode %v", body["mode"])
	}
	primary, _ := body["primary"].(map[string]any)
	if primary["custom_url"] != "alpha" {
		t.Fatalf("primary %v", body["primary"])
	}
	if _, ok := body["venues"].([]any); !ok {
		t.Fatalf("venues must be an array, got %T", body["venues"])
	}
}
