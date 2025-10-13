package server

import "payverge/internal/services"

var translationService *services.TranslationService

// SetTranslationService sets the translation service for the server package
func SetTranslationService(service *services.TranslationService) {
	translationService = service
}

// GetTranslationService returns the translation service
func GetTranslationService() *services.TranslationService {
	return translationService
}
