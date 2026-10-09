package server

import "github.com/stdevmac/payverge/backend/internal/services"

var translationService *services.TranslationService

// googlePlacesServiceConcrete is the production-grade Google Places client used
// by the admin/business handlers that need the full feature surface (search,
// validate, link generation, etc.). It is retained alongside the interface
// variable below so we don't have to widen the interface for every call site.
var googlePlacesServiceConcrete *services.GooglePlacesService

// GooglePlacesService is the minimal surface required by public diner-page
// handlers. Defining it here lets tests stub upstream errors without standing
// up the real Google Places client.
type GooglePlacesService interface {
	GetPlaceReviews(placeID, language string) ([]any, error)
	GetPlaceDetails(placeID string) (any, error)
}

// googlePlacesService is the swappable interface-typed handle the public
// handlers consult. Tests overwrite this variable to inject failure modes.
var googlePlacesService GooglePlacesService

// SetTranslationService sets the translation service for the server package
func SetTranslationService(service *services.TranslationService) {
	translationService = service
}

// GetTranslationService returns the translation service
func GetTranslationService() *services.TranslationService {
	return translationService
}

// SetGooglePlacesService sets the Google Places service for the server package.
// It stores both the concrete client (search/validate/link helpers) and a
// cached interface adapter that every reviews/details read goes through, so
// billed Place Details calls are bounded per Place ID (see places_cache.go).
func SetGooglePlacesService(service *services.GooglePlacesService) {
	googlePlacesServiceConcrete = service
	if service == nil {
		googlePlacesService = nil
		return
	}
	googlePlacesService = newCachedPlacesService(&googlePlacesAdapter{client: service})
}

// GetGooglePlacesService returns the concrete Google Places service. Returns
// nil when the service has not been configured (e.g., missing API key at
// startup); callers must handle that case.
func GetGooglePlacesService() *services.GooglePlacesService {
	return googlePlacesServiceConcrete
}

// googlePlacesAdapter bridges the concrete *services.GooglePlacesService to
// the local GooglePlacesService interface (which uses any/[]any so tests can
// supply trivial stubs).
type googlePlacesAdapter struct {
	client *services.GooglePlacesService
}

func (a *googlePlacesAdapter) GetPlaceReviews(placeID, language string) ([]any, error) {
	reviews, err := a.client.GetPlaceReviews(placeID, language)
	if err != nil {
		return nil, err
	}
	out := make([]any, 0, len(reviews))
	for i := range reviews {
		out = append(out, reviews[i])
	}
	return out, nil
}

func (a *googlePlacesAdapter) GetPlaceDetails(placeID string) (any, error) {
	details, err := a.client.GetPlaceDetails(placeID)
	if err != nil {
		return nil, err
	}
	return details, nil
}
