package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/services"
	"github.com/stretchr/testify/assert"
)

// mustTime parses an RFC3339 time string and panics on error.
func mustTime(s string) time.Time {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		panic(err)
	}
	return t
}

// TestGuestMenuETagShortCircuits304 verifies that applyGuestMenuCaching returns
// true (304 short-circuit) when the request's If-None-Match matches the ETag.
func TestGuestMenuETagShortCircuits304(t *testing.T) {
	etag := guestMenuETag(7, 42, "en", mustTime("2026-06-20T10:00:00Z"), "")
	if etag == "" {
		t.Fatal("expected non-empty weak ETag")
	}

	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/x", nil)
	c.Request.Header.Set("If-None-Match", etag)

	if !applyGuestMenuCaching(c, etag) {
		t.Fatal("expected 304 short-circuit on matching If-None-Match")
	}
	if w.Code != http.StatusNotModified {
		t.Fatalf("expected 304, got %d", w.Code)
	}
	if got := w.Header().Get("Cache-Control"); got != "public, max-age=5, must-revalidate" {
		t.Fatalf("Cache-Control = %q", got)
	}
}

// TestGuestMenuETagMismatchServes200 verifies that a non-matching If-None-Match
// returns false (caller continues to write 200 body) and headers are set.
func TestGuestMenuETagMismatchServes200(t *testing.T) {
	etag := guestMenuETag(7, 42, "en", mustTime("2026-06-20T10:00:00Z"), "")

	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/x", nil)
	c.Request.Header.Set("If-None-Match", `W/"stale-value"`)

	if applyGuestMenuCaching(c, etag) {
		t.Fatal("expected false (no 304) on mismatched ETag")
	}
	if got := w.Header().Get("ETag"); got != etag {
		t.Fatalf("ETag header = %q, want %q", got, etag)
	}
	if got := w.Header().Get("Cache-Control"); got != "public, max-age=5, must-revalidate" {
		t.Fatalf("Cache-Control = %q", got)
	}
}

func TestGuestMenuCachingSupportsEntityTagListsWeakValidatorsAndWildcard(t *testing.T) {
	etag := guestMenuETag(7, 42, "es", mustTime("2026-06-20T10:00:00Z"), "")
	for _, ifNoneMatch := range []string{
		etag,
		strings.TrimPrefix(etag, "W/"),
		`W/"other", ` + etag,
		`*`,
	} {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest("GET", "/x", nil)
		c.Request.Header.Set("If-None-Match", ifNoneMatch)

		if !applyGuestMenuCaching(c, etag) {
			t.Fatalf("expected 304 for If-None-Match %q", ifNoneMatch)
		}
		if w.Code != http.StatusNotModified {
			t.Fatalf("If-None-Match %q: expected 304, got %d", ifNoneMatch, w.Code)
		}
	}

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/x", nil)
	c.Request.Header.Set("If-None-Match", `W/"other", "another"`)
	if applyGuestMenuCaching(c, etag) {
		t.Fatal("expected a non-matching entity-tag list to continue with 200")
	}
}

func TestGuestMenuCachingNever304sIncompleteWildcard(t *testing.T) {
	etag := guestMenuETag(7, 42, "es", mustTime("2026-06-20T10:00:00Z"), "")
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/x", nil)
	c.Request.Header.Set("If-None-Match", "*")

	if applyGuestMenuCaching(c, etag, true) {
		t.Fatal("incomplete translations must not short-circuit a wildcard validator")
	}
	if w.Code == http.StatusNotModified {
		t.Fatal("incomplete translations must not write 304")
	}
	if got := w.Header().Get("Cache-Control"); got != "private, no-store" {
		t.Fatalf("Cache-Control = %q, want private, no-store", got)
	}
}

// TestGuestMenuETagChangesWithVersion verifies that bumping the menu version
// produces a different ETag (so stale 304s are never issued after a menu edit).
func TestGuestMenuETagChangesWithVersion(t *testing.T) {
	base := mustTime("2026-06-20T10:00:00Z")
	etag1 := guestMenuETag(7, 42, "en", base, "")
	etag2 := guestMenuETag(7, 43, "en", base, "")
	if etag1 == etag2 {
		t.Fatalf("expected different ETags for different menu versions, got %q for both", etag1)
	}
}

// business.UpdatedAt still contributes to the validator, so two writes within
// one PostgreSQL-second must not collapse. Inventory 86s now also hash the
// post-projection orderability digest (see TestGuestMenuETagChangesWithOrderabilityRevision).
func TestGuestMenuETagChangesWithSubsecondBusinessRevision(t *testing.T) {
	base := mustTime("2026-06-20T10:00:00Z")
	etag1 := guestMenuETag(7, 42, "en", base, "")
	etag2 := guestMenuETag(7, 42, "en", base.Add(time.Microsecond), "")
	if etag1 == etag2 {
		t.Fatalf("expected different ETags for sub-second business revisions, got %q for both", etag1)
	}
}

// TestGuestMenuETagChangesWithLang verifies that the same menu version at
// different locales produces different ETags (no cross-locale 304 collisions).
func TestGuestMenuETagChangesWithLang(t *testing.T) {
	base := mustTime("2026-06-20T10:00:00Z")
	etagEN := guestMenuETag(7, 42, "en", base, "")
	etagES := guestMenuETag(7, 42, "es", base, "")
	if etagEN == etagES {
		t.Fatalf("expected different ETags for different languages, got %q for both", etagEN)
	}
}

// TestGuestMenuETagChangesWithTranslationRevision ensures a translation
// backfill (without menu version bump) still invalidates cached public menus.
func TestGuestMenuETagChangesWithTranslationRevision(t *testing.T) {
	base := mustTime("2026-06-20T10:00:00Z")
	etagBefore := guestMenuETag(7, 42, "es", base, "")
	etagAfter := guestMenuETag(7, 42, "es", base, "revision-after")
	if etagBefore == etagAfter {
		t.Fatalf("expected different ETags after translation revision, got %q for both", etagBefore)
	}
}

func TestGuestMenuETagChangesWithOrderabilityRevision(t *testing.T) {
	base := mustTime("2026-06-20T10:00:00Z")
	stocked := []database.MenuCategory{{
		Items: []database.MenuItem{{ID: "demo-bife", Name: "Bife de chorizo", IsAvailable: true}},
	}}
	depleted := []database.MenuCategory{{
		Items: []database.MenuItem{{
			ID: "demo-bife", Name: "Bife de chorizo", IsAvailable: false, InventoryStatus: "out_of_stock",
		}},
	}}
	stockedProj := map[string]services.Orderability{
		"demo-bife": {Orderable: true, State: services.OrderabilityAvailable},
	}
	depletedProj := map[string]services.Orderability{
		"demo-bife": {State: services.OrderabilityInventoryOut},
	}

	etagStocked := guestMenuETag(7, 42, "es", base, "", orderabilityRevisionDigest(stocked, stockedProj))
	etagDepleted := guestMenuETag(7, 42, "es", base, "", orderabilityRevisionDigest(depleted, depletedProj))
	etagStockedAgain := guestMenuETag(7, 42, "es", base, "", orderabilityRevisionDigest(stocked, stockedProj))

	assert.NotEqual(t, etagStocked, etagDepleted, "beef 12 → 0 must change the guest menu ETag")
	assert.Equal(t, etagStocked, etagStockedAgain, "unchanged stock must keep the same validator")
}

func TestGuestMenuETagChangesWithOfferAndBundleSourceRevision(t *testing.T) {
	base := mustTime("2026-06-20T10:00:00Z")
	offerBefore := []database.Offer{{BusinessID: 7, ID: 11, Name: "Old offer", UpdatedAt: base}}
	offerAfter := []database.Offer{{BusinessID: 7, ID: 11, Name: "New offer", UpdatedAt: base.Add(time.Microsecond)}}
	bundleBefore := []database.Bundle{{BusinessID: 7, ID: 22, Name: "Old bundle", UpdatedAt: base}}
	bundleAfter := []database.Bundle{{BusinessID: 7, ID: 22, Name: "New bundle", UpdatedAt: base.Add(time.Microsecond)}}

	offerETagBefore := guestMenuETag(7, 42, "es", base, "translation", promotionRevisionDigest(offerBefore, nil))
	offerETagAfter := guestMenuETag(7, 42, "es", base, "translation", promotionRevisionDigest(offerAfter, nil))
	bundleETagBefore := guestMenuETag(7, 42, "es", base, "translation", promotionRevisionDigest(nil, bundleBefore))
	bundleETagAfter := guestMenuETag(7, 42, "es", base, "translation", promotionRevisionDigest(nil, bundleAfter))

	assert.NotEqual(t, offerETagBefore, offerETagAfter, "offer source changes must invalidate the guest menu ETag")
	assert.NotEqual(t, bundleETagBefore, bundleETagAfter, "bundle source changes must invalidate the guest menu ETag")
}

// TestGuestMenuCachingSkips304WhenIncomplete ensures partially translated
// responses are never sticky via If-None-Match after a backfill is scheduled.
func TestGuestMenuCachingSkips304WhenIncomplete(t *testing.T) {
	etag := guestMenuETag(7, 42, "es", mustTime("2026-06-20T10:00:00Z"), "")

	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/x", nil)
	c.Request.Header.Set("If-None-Match", etag)

	if applyGuestMenuCaching(c, etag, true) {
		t.Fatal("expected no 304 short-circuit for incomplete translated menus")
	}
	if got := w.Header().Get("Cache-Control"); got != "private, no-store" {
		t.Fatalf("Cache-Control = %q, want private, no-store", got)
	}
}

// TestGuestMenuETagNoMatchHeader verifies that a request without If-None-Match
// also gets the correct headers and a 200 response (no short-circuit).
func TestGuestMenuETagNoMatchHeader(t *testing.T) {
	etag := guestMenuETag(7, 42, "en", mustTime("2026-06-20T10:00:00Z"), "")

	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/x", nil)
	// No If-None-Match header.

	if applyGuestMenuCaching(c, etag) {
		t.Fatal("expected false (no 304) when no If-None-Match header")
	}
	if got := w.Header().Get("ETag"); got != etag {
		t.Fatalf("ETag header = %q, want %q", got, etag)
	}
}
