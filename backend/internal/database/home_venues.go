package database

import (
	"strconv"
	"strings"
)

// PublishedVenue is the narrow public projection the instance home ("/")
// needs: enough to render a venue directory card or pick the venue to serve.
// Only storefront-public fields; never owner, billing or settlement data.
type PublishedVenue struct {
	ID        uint   `json:"id"`
	Name      string `json:"name"`
	Logo      string `json:"logo"`
	CustomURL string `json:"custom_url"`
	City      string `json:"city"`
}

// publishedVenueColumns is the explicit projection for PublishedVenue.
var publishedVenueColumns = []string{"id", "name", "logo", "custom_url", "city"}

// ListPublishedVenues returns published storefronts for the instance home
// directory, ordered by name, bounded by limit.
func ListPublishedVenues(limit int) ([]PublishedVenue, error) {
	var out []PublishedVenue
	err := db.Model(&Business{}).
		Select(publishedVenueColumns).
		Where("business_page_enabled = ? AND is_active = ? AND custom_url <> ''", true, true).
		Where("(kind IS NULL OR kind = '' OR kind <> ?)", BusinessKindTest).
		Order("name ASC").
		Order("id ASC").
		Limit(limit).
		Find(&out).Error
	return out, err
}

// FindPublishedVenue resolves a PRIMARY_VENUE reference to a published venue.
// The reference may be a numeric business id, a storefront slug (custom_url)
// or the business_id slug, tried in that order. The custom_url match ignores
// case (idx_businesses_custom_url_lower); business_id is canonical as stored
// and matches exactly so idx_businesses_business_id serves it. A numeric
// reference falls through to the slugs (they may be all digits). It returns
// (nil, nil) when nothing published matches.
func FindPublishedVenue(ref string) (*PublishedVenue, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return nil, nil
	}
	if id, err := strconv.ParseUint(ref, 10, 64); err == nil && id > 0 {
		v, err := findPublishedVenueWhere("id = ?", id)
		if err != nil || v != nil {
			return v, err
		}
	}
	v, err := findPublishedVenueWhere("LOWER(custom_url) = ?", strings.ToLower(ref))
	if err != nil || v != nil {
		return v, err
	}
	return findPublishedVenueWhere("business_id = ?", ref)
}

func findPublishedVenueWhere(cond string, arg interface{}) (*PublishedVenue, error) {
	var out []PublishedVenue
	err := db.Model(&Business{}).
		Select(publishedVenueColumns).
		Where("business_page_enabled = ? AND is_active = ? AND custom_url <> ''", true, true).
		Where("(kind IS NULL OR kind = '' OR kind <> ?)", BusinessKindTest).
		Where(cond, arg).
		Limit(1).
		Find(&out).Error
	if err != nil {
		return nil, err
	}
	if len(out) == 0 {
		return nil, nil
	}
	return &out[0], nil
}
