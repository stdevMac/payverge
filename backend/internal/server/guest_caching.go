package server

// guest_caching.go — ETag and Cache-Control helpers for public guest endpoints.
//
// Design:
//   - guestMenuETag / applyGuestMenuCaching: conservative public caching for the
//     guest menu (menu-browse, table-code access). max-age=5 keeps CDN/Caddy polls
//     collapsed without staling menu changes. Language is folded in so en vs es
//     responses never share a 304.
//
// NOTE: A follow-up task (P6.7) will add buildTranslatedMenuResponse into this
// file. Keep helpers narrow and well-named so that function slots cleanly.

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/services"
)

// guestMenuETag derives a weak ETag for a guest menu response. It incorporates
// businessID, menu version, locale, business.UpdatedAt, a translation revision,
// and any extra content revisions (promotions, live orderability) so that menu
// edits, translation backfills, offer/bundle changes, and inventory 86s all
// produce a new ETag.
//
// Weak ("W/") because the JSON serialisation byte-order is not guaranteed stable
// across server restarts, but the semantic content is fully keyed by these inputs.
func guestMenuETag(businessID uint, menuVersion uint, lang string, updatedAt time.Time, translationRevision string, extraRevisions ...string) string {
	revision := translationRevision
	for _, extra := range extraRevisions {
		if extra == "" {
			continue
		}
		revision += "|" + extra
	}
	return fmt.Sprintf(`W/"m-%d-%d-%s-%d-%s"`, businessID, menuVersion, lang, updatedAt.UTC().UnixNano(), revision)
}

// orderabilityRevisionDigest hashes the serve-time 86 / orderability stamps
// after ProjectOrderability. Inventory quantity is not a menu-version input, so
// this digest is what stops a shared cache from 304'ing a pre-86 storefront
// body. Callers must pass the already-projected categories + map — do not load
// inventory again.
func orderabilityRevisionDigest(categories []database.MenuCategory, projection map[string]services.Orderability) string {
	type row struct {
		ID              string `json:"id"`
		IsAvailable     bool   `json:"is_available"`
		InventoryStatus string `json:"inventory_status"`
		Orderable       bool   `json:"orderable"`
		State           string `json:"state"`
	}
	rows := make([]row, 0)
	for _, category := range categories {
		for _, item := range category.Items {
			id := strings.TrimSpace(item.ID)
			if id == "" {
				continue
			}
			entry := row{
				ID:              id,
				IsAvailable:     item.IsAvailable,
				InventoryStatus: item.InventoryStatus,
			}
			if decision, ok := projection[id]; ok {
				entry.Orderable = decision.Orderable
				entry.State = string(decision.State)
			}
			rows = append(rows, entry)
		}
	}
	sort.SliceStable(rows, func(i, j int) bool {
		return rows[i].ID < rows[j].ID
	})
	payload, err := json.Marshal(rows)
	if err != nil {
		return "marshal-error"
	}
	digest := sha256.Sum256(payload)
	return hex.EncodeToString(digest[:])
}

func guestMenuPayloadProjection(menuPayload gin.H) map[string]services.Orderability {
	if menuPayload == nil {
		return nil
	}
	projection, _ := menuPayload["item_orderability"].(map[string]services.Orderability)
	return projection
}

func guestMenuCacheValidator(
	business *database.Business,
	menu *database.Menu,
	lang, translationRevision string,
	offers []database.Offer,
	bundles []database.Bundle,
	categories []database.MenuCategory,
	menuPayload gin.H,
) string {
	var businessID uint
	var updatedAt time.Time
	if business != nil {
		businessID = business.ID
		updatedAt = business.UpdatedAt
	}
	var menuVersion uint
	if menu != nil {
		menuVersion = menu.Version
	}
	return guestMenuETag(
		businessID,
		menuVersion,
		lang,
		updatedAt,
		translationRevision,
		promotionRevisionDigest(offers, bundles),
		orderabilityRevisionDigest(categories, guestMenuPayloadProjection(menuPayload)),
	)
}

// promotionRevisionDigest returns a deterministic content revision for the
// active offer/bundle snapshot represented by a guest menu response. Sorting
// by ID keeps the digest stable even if database reads return rows in a
// different order; the JSON snapshot includes every public promotion field so
// any visible source change invalidates the guest menu validator.
func promotionRevisionDigest(offers []database.Offer, bundles []database.Bundle) string {
	sortedOffers := append([]database.Offer(nil), offers...)
	sort.SliceStable(sortedOffers, func(i, j int) bool {
		return sortedOffers[i].ID < sortedOffers[j].ID
	})
	sortedBundles := append([]database.Bundle(nil), bundles...)
	sort.SliceStable(sortedBundles, func(i, j int) bool {
		return sortedBundles[i].ID < sortedBundles[j].ID
	})

	payload, err := json.Marshal(struct {
		Offers  []database.Offer  `json:"offers"`
		Bundles []database.Bundle `json:"bundles"`
	}{Offers: sortedOffers, Bundles: sortedBundles})
	if err != nil {
		// database.Offer and database.Bundle contain only JSON-marshalable
		// fields, so this is defensive. Returning a non-empty sentinel is safer
		// than silently dropping the promotion component from the ETag.
		return "marshal-error"
	}
	digest := sha256.Sum256(payload)
	return hex.EncodeToString(digest[:])
}

// applyGuestMenuCaching sets Cache-Control + ETag on the response and, when the
// incoming If-None-Match header matches, writes 304 Not Modified and returns
// true so the caller can return immediately without serialising the body.
//
// Cache-Control: public, max-age=5, must-revalidate
//   - public: safe for shared caches (Caddy, CDN) — guest menu has no personal data.
//   - max-age=5: a five-second window collapses repeat polls without risking stale
//     menus for more than one polling interval.
//   - must-revalidate: after TTL expires, caches must re-check before serving stale.
//
// When incomplete is true (missing translations that will be backfilled), use
// no-store so a partially-English response cannot stick via ETag 304 after the
// backfill completes with the same menu version.
func applyGuestMenuCaching(c *gin.Context, etag string, incomplete ...bool) bool {
	cc := guestMenuCacheControl
	if len(incomplete) > 0 && incomplete[0] {
		cc = "private, no-store"
		c.Header("Cache-Control", cc)
		c.Header("ETag", etag)
		// Never short-circuit 304 for incomplete translated menus — clients must
		// re-fetch until backfill fills the missing rows.
		return false
	}
	c.Header("Cache-Control", cc)
	c.Header("ETag", etag)
	if ifNoneMatchMatches(c.GetHeader("If-None-Match"), etag) {
		c.AbortWithStatus(http.StatusNotModified)
		return true
	}
	return false
}

// guestMenuCacheControl is the shared-cache policy for complete guest menus.
const guestMenuCacheControl = "public, max-age=5, must-revalidate"

// guestTableCacheControl lets a guest's own browser revalidate the QR table
// context with If-None-Match while keeping it out of shared caches: the body
// carries the table identity a scanned code resolved to.
const guestTableCacheControl = "private, no-cache"

// guestValidatorKey names one remembered guest response variant. Only the
// business's own language is memoized: a translated body's validator carries
// a content digest of the translation rows, and translation backfills rewrite
// those rows in place without any cache hook, so translated requests always
// recompute (an empty key disables the memo).
func guestValidatorKey(endpoint, lookup, lang, defaultLang string) string {
	if defaultLang == "" {
		defaultLang = "en"
	}
	if lang == "" {
		lang = defaultLang
	}
	if lang != defaultLang {
		return ""
	}
	return endpoint + "|" + lookup + "|" + lang
}

// answerGuestConditionalFromMemo serves 304 from the remembered validator
// before any menu, promotion, inventory or translation load. It only fires
// when the client sent If-None-Match, the memo entry is fresh (write hooks
// drop it) and the business row it was computed from is unchanged.
func answerGuestConditionalFromMemo(c *gin.Context, key string, business *database.Business, cacheControl string) bool {
	if business == nil {
		return false
	}
	// Every caller loads the menu snapshot right after this returns false.
	// The generation is read before that load and stamped on the miss path
	// only, so a write that lands during the load keeps the resulting
	// validator out of the memo without costing the 304 path an alloc.
	gen := services.PricingGeneration(business.ID)
	header := c.GetHeader("If-None-Match")
	if strings.TrimSpace(header) != "" {
		if etag, ok := services.GuestValidator(key, business.ID, business.UpdatedAt); ok && ifNoneMatchMatches(header, etag) {
			c.Header("Cache-Control", cacheControl)
			c.Header("ETag", etag)
			c.AbortWithStatus(http.StatusNotModified)
			return true
		}
	}
	c.Set(guestValidatorGenContextKey, gen)
	return false
}

const guestValidatorGenContextKey = "guest_validator_pricing_gen"

// rememberGuestValidator memoizes etag under the generation stamped by
// answerGuestConditionalFromMemo. Without a stamp nothing is remembered.
func rememberGuestValidator(c *gin.Context, key string, business *database.Business, etag string) {
	if business == nil {
		return
	}
	raw, ok := c.Get(guestValidatorGenContextKey)
	gen, isGen := raw.(uint64)
	if !ok || !isGen {
		return
	}
	services.RememberGuestValidator(key, business.ID, business.UpdatedAt, gen, etag)
}

// applyRememberedGuestMenuCaching remembers a complete menu validator for the
// early-304 path, then applies the normal menu caching headers.
func applyRememberedGuestMenuCaching(c *gin.Context, key string, business *database.Business, etag string, incomplete bool) bool {
	if !incomplete {
		rememberGuestValidator(c, key, business, etag)
	}
	return applyGuestMenuCaching(c, etag, incomplete)
}

// writeGuestTableResponse serializes the QR table payload once, derives a
// content ETag from the bytes and answers 304 when the client already has
// them. Incomplete (translation-backfill) bodies are no-store and never
// remembered, matching the menu endpoints.
func writeGuestTableResponse(c *gin.Context, key string, business *database.Business, payload gin.H, incomplete bool) {
	body, err := json.Marshal(payload)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to encode table response"})
		return
	}
	if incomplete {
		c.Header("Cache-Control", "private, no-store")
		c.Data(http.StatusOK, "application/json; charset=utf-8", body)
		return
	}
	digest := sha256.Sum256(body)
	etag := `W/"t-` + hex.EncodeToString(digest[:16]) + `"`
	rememberGuestValidator(c, key, business, etag)
	c.Header("Cache-Control", guestTableCacheControl)
	c.Header("ETag", etag)
	if ifNoneMatchMatches(c.GetHeader("If-None-Match"), etag) {
		c.AbortWithStatus(http.StatusNotModified)
		return
	}
	c.Data(http.StatusOK, "application/json; charset=utf-8", body)
}

// applyGuestBillNoStore marks guest-bill JSON (public_token, wallets, items)
// uncacheable. These responses are capability-URL secrets; they must never be
// stored by a shared cache even if a Cache-Everything rule is added later (#524).
func applyGuestBillNoStore(c *gin.Context) {
	c.Header("Cache-Control", "private, no-store")
}

// ifNoneMatchMatches applies the weak comparison required by If-None-Match:
// comma-separated entity tags, wildcard, and strong/weak forms all match the
// same opaque tag. Guest endpoints only use this helper for safe GET reads, so
// a match is represented by the caller as 304 Not Modified.
func ifNoneMatchMatches(header, currentETag string) bool {
	if strings.TrimSpace(header) == "" {
		return false
	}
	if strings.TrimSpace(header) == "*" {
		return true
	}
	_, currentOpaque, currentOK := parseEntityTag(currentETag)
	if !currentOK {
		return false
	}
	for _, candidate := range strings.Split(header, ",") {
		candidate = strings.TrimSpace(candidate)
		if candidate == "*" {
			return true
		}
		_, opaque, ok := parseEntityTag(candidate)
		if ok && opaque == currentOpaque {
			return true
		}
	}
	return false
}

func parseEntityTag(value string) (weak bool, opaque string, ok bool) {
	value = strings.TrimSpace(value)
	if strings.HasPrefix(value, "W/") {
		weak = true
		value = strings.TrimSpace(strings.TrimPrefix(value, "W/"))
	}
	if len(value) < 2 || value[0] != '"' || value[len(value)-1] != '"' {
		return false, "", false
	}
	return weak, value[1 : len(value)-1], true
}

// buildTranslatedMenuResponse returns the guest menu gin.H payload with the
// translated categories under the canonical "categories" key.
//
// The legacy "parsed_categories" duplicate is dropped so multilingual guests
// do not transfer the full menu twice. The frontend accesses translated
// categories via the pattern `menuData.parsed_categories || menuData.categories`,
// so the fallback arm fires and returns the same translated data — no FE change
// required, but the wire-shape change should be noted in the PR body for FE
// coordination.
func buildTranslatedMenuResponse(
	menu *database.Menu,
	translatedCategories []database.MenuCategory,
	offers []database.Offer,
	bundles []database.Bundle,
	lang string,
	businesses ...*database.Business,
) gin.H {
	return gin.H{
		"menu":       publicGuestMenuMeta(menu, businesses...),
		"categories": translatedCategories,
		"language":   lang,
		"offers":     publicGuestOffers(offers),
		"bundles":    bundles,
	}
}

// translateGuestMenuForLanguage applies one translation snapshot to every
// guest-menu representation. Keeping the lookup shared makes the body and its
// ETag observe the same rows, while avoiding three independent translation
// queries plus a separate revision query on the hot public/table paths.
func translateGuestMenuForLanguage(
	businessID uint,
	categories []database.MenuCategory,
	offers []database.Offer,
	bundles []database.Bundle,
	languageCode string,
) (translatedCategories []database.MenuCategory, translatedOffers []database.Offer, translatedBundles []database.Bundle, missingMenu bool, missingPromotions bool, revision string, err error) {
	lookup := newTranslationLookup(
		database.GetDBWrapper(),
		businessID,
		languageCode,
		guestMenuTranslationLookupScope(categories, offers, bundles),
	)
	if lookup.err != nil {
		return nil, nil, nil, false, false, "", lookup.err
	}
	translatedCategories, missingMenu = applyTranslationsToMenuWithLookup(lookup, categories)
	translatedOffers, missingOffers := applyTranslationsToOffersWithLookup(offers, languageCode, lookup)
	translatedBundles, missingBundles := applyTranslationsToBundlesWithLookup(bundles, languageCode, lookup)
	return translatedCategories, translatedOffers, translatedBundles, missingMenu, missingOffers || missingBundles, lookup.revision, nil
}
