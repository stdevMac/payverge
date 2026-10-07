package marketing

import (
	"net/url"
	"strings"

	"github.com/stdevmac/payverge/backend/internal/config"
)

// GuestURLKind classifies the public storefront destination for a play.
// Only public /b/{slug} guest routes are ever emitted — never signed private
// asset URLs.
type GuestURLKind string

const (
	GuestURLKindNone         GuestURLKind = ""
	GuestURLKindBusiness     GuestURLKind = "business"
	GuestURLKindMenu         GuestURLKind = "menu"
	GuestURLKindReservations GuestURLKind = "reservations"
)

// DefaultGuestOrigin is the storefront origin used when the caller does not
// supply one: this instance's PUBLIC_URL (config.PublicURL).
func DefaultGuestOrigin() string {
	return config.PublicURL()
}

// GuestURLInput is everything needed to construct a public guest deep link.
// CustomURL is the storefront slug (businesses.custom_url). PageEnabled mirrors
// businesses.business_page_enabled — when false or slug empty, no URL is built.
type GuestURLInput struct {
	// Origin e.g. "https://pos.example.com". Empty → DefaultGuestOrigin().
	Origin string
	// CustomURL storefront slug. Must be a single path segment (no slashes).
	CustomURL string
	// PageEnabled is businesses.business_page_enabled.
	PageEnabled bool
	// Play selects the tab destination when the storefront is live.
	Play Play
}

// GuestURLResult is the optional public URL + kind. Empty URL means "omit".
type GuestURLResult struct {
	URL  string
	Kind GuestURLKind
}

// BuildGuestURL constructs a public guest deep link for a marketing play.
//
// Security contract:
//   - Only absolute {PUBLIC_URL}/b/{slug}… paths (or caller origin)
//   - Never private/signed asset URLs
//   - Never open redirects: slug is path-escaped as a single segment; origin is
//     host-only (no user-controlled path injection)
//   - Dish-level deep links are not a storefront feature yet; dish/combo/offer
//     plays deep-link to the menu tab so guests land on the public menu.
//
// Returns empty result when the page is off or the slug is unusable.
func BuildGuestURL(in GuestURLInput) GuestURLResult {
	slug := normalizeGuestSlug(in.CustomURL)
	if !in.PageEnabled || slug == "" {
		return GuestURLResult{}
	}
	origin := strings.TrimRight(strings.TrimSpace(in.Origin), "/")
	if origin == "" {
		origin = DefaultGuestOrigin()
	}
	// Reject non-http(s) origins so a misconfigured host cannot emit javascript: URLs.
	if !strings.HasPrefix(origin, "https://") && !strings.HasPrefix(origin, "http://") {
		return GuestURLResult{}
	}

	base := origin + "/b/" + url.PathEscape(slug)
	kind, tab := guestTabForPlay(in.Play)
	if tab == "" {
		return GuestURLResult{URL: base, Kind: kind}
	}
	return GuestURLResult{URL: base + "?tab=" + tab, Kind: kind}
}

// guestTabForPlay maps plays onto storefront hash-tabs (useHashTabs).
// Empty tab = business landing page only.
func guestTabForPlay(play Play) (GuestURLKind, string) {
	switch play {
	case PlayFeaturedDish, PlayMoveItem, PlayComboDeal, PlayOffer, PlayHappyHour:
		return GuestURLKindMenu, "menu"
	case PlayWinBack:
		// Win-back often pairs with a return visit — reservations tab when the
		// storefront exposes it; otherwise still a valid public page deep link.
		return GuestURLKindReservations, "reservations"
	default:
		return GuestURLKindBusiness, ""
	}
}

// normalizeGuestSlug accepts only a single path segment of safe URL characters.
// Rejects empty, absolute URLs, path traversal, and multi-segment values.
func normalizeGuestSlug(raw string) string {
	s := strings.TrimSpace(raw)
	if s == "" {
		return ""
	}
	// Reject anything that looks like an absolute URL or path.
	if strings.Contains(s, "://") || strings.ContainsAny(s, "/\\?#") {
		return ""
	}
	if strings.Contains(s, "..") {
		return ""
	}
	// Keep only a conservative slug charset (matches CustomURL validation).
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			continue
		}
		return ""
	}
	if len(s) < 2 || len(s) > 64 {
		return ""
	}
	return s
}

// AttachGuestURLs fills GuestURL / GuestURLKind on each suggestion in place.
// No-ops when the business has no public page.
func AttachGuestURLs(suggestions []CampaignSuggestion, customURL string, pageEnabled bool, origin string) {
	if !pageEnabled || normalizeGuestSlug(customURL) == "" {
		return
	}
	for i := range suggestions {
		res := BuildGuestURL(GuestURLInput{
			Origin:      origin,
			CustomURL:   customURL,
			PageEnabled: pageEnabled,
			Play:        suggestions[i].Play,
		})
		if res.URL == "" {
			continue
		}
		suggestions[i].GuestURL = res.URL
		suggestions[i].GuestURLKind = string(res.Kind)
	}
}
