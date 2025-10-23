package server

import "payverge/internal/services"

var translationService *services.TranslationService
var googlePlacesService *services.GooglePlacesService

// SetTranslationService sets the translation service for the server package
func SetTranslationService(service *services.TranslationService) {
	translationService = service
}

// GetTranslationService returns the translation service
func GetTranslationService() *services.TranslationService {
	return translationService
}

// SetGooglePlacesService sets the Google Places service for the server package
func SetGooglePlacesService(service *services.GooglePlacesService) {
	googlePlacesService = service
}

// GetGooglePlacesService returns the Google Places service
func GetGooglePlacesService() *services.GooglePlacesService {
	return googlePlacesService
}
