package server

// GET /api/v1/home — public, unauthenticated resolution of what the instance
// root ("/") serves. Payverge is self-hosted per venue (or venue group), so
// "/" is the venue's public business page rather than a product site:
//
//   - PRIMARY_VENUE (numeric id, storefront slug or business_id slug) names
//     the venue to serve,
//     when it matches a published storefront;
//   - otherwise, exactly one published storefront is served;
//   - several published storefronts render a directory;
//   - none sends visitors to /dashboard (operator sign-in).
//
// Only the storefront publish gate decides visibility (page enabled, active,
// routable slug, not a kind=test fixture), so the payload never exposes a
// venue that /b/<slug> would not already show.

import (
	"log"
	"net/http"
	"os"
	"strings"
	"sync"

	"github.com/gin-gonic/gin"

	"github.com/stdevmac/payverge/backend/internal/database"
)

// HomeModeVenue, HomeModeDirectory and HomeModeEmpty are the three shapes of
// the instance root.
const (
	HomeModeVenue     = "venue"
	HomeModeDirectory = "directory"
	HomeModeEmpty     = "empty"
)

// homeDirectoryLimit bounds the directory listing. A multi-venue install with
// more storefronts than this still links each one from the sitemap.
const homeDirectoryLimit = 200

// homeCacheControl mirrors /instance: a short shared cache, since publish
// changes should reach "/" within a minute.
const homeCacheControl = "public, max-age=60"

// HomeResponse is the GET /api/v1/home payload.
type HomeResponse struct {
	Mode    string                    `json:"mode"`
	Primary *database.PublishedVenue  `json:"primary"`
	Venues  []database.PublishedVenue `json:"venues"`
}

// homeVenueLookups is the data seam; tests swap it.
type homeVenueLookups struct {
	find func(ref string) (*database.PublishedVenue, error)
	list func(limit int) ([]database.PublishedVenue, error)
}

var homeLookups = homeVenueLookups{
	find: database.FindPublishedVenue,
	list: database.ListPublishedVenues,
}

var warnedPrimaryVenue sync.Map

// ResolveHome decides what "/" serves for the given PRIMARY_VENUE value.
func ResolveHome(primaryRef string) (HomeResponse, error) {
	primaryRef = strings.TrimSpace(primaryRef)
	if primaryRef != "" {
		v, err := homeLookups.find(primaryRef)
		if err != nil {
			return HomeResponse{}, err
		}
		if v != nil {
			return HomeResponse{Mode: HomeModeVenue, Primary: v, Venues: []database.PublishedVenue{*v}}, nil
		}
		if _, seen := warnedPrimaryVenue.LoadOrStore(primaryRef, true); !seen {
			log.Printf("PRIMARY_VENUE=%q does not match a published storefront; falling back to the venue list", primaryRef)
		}
	}
	venues, err := homeLookups.list(homeDirectoryLimit)
	if err != nil {
		return HomeResponse{}, err
	}
	switch len(venues) {
	case 0:
		return HomeResponse{Mode: HomeModeEmpty, Venues: []database.PublishedVenue{}}, nil
	case 1:
		return HomeResponse{Mode: HomeModeVenue, Primary: &venues[0], Venues: venues}, nil
	default:
		return HomeResponse{Mode: HomeModeDirectory, Venues: venues}, nil
	}
}

// GetHome serves GET /api/v1/home.
func GetHome(c *gin.Context) {
	resp, err := ResolveHome(os.Getenv("PRIMARY_VENUE"))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to resolve home"})
		return
	}
	c.Header("Cache-Control", homeCacheControl)
	c.JSON(http.StatusOK, resp)
}
