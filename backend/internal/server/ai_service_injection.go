package server

import (
	"github.com/stdevmac/payverge/backend/internal/config"
	"github.com/stdevmac/payverge/backend/internal/services"
)

var aiService *services.AIService
var deliveryService *services.DeliveryService

// SetAIService sets the AI service for the server package. It also records
// whether an LLM provider is configured (config.AIProviderConfigured), which
// drives the 503 ai_not_configured answer on LLM-only routes and the
// ai_configured / ai_waiter_mode fields exposed to clients.
func SetAIService(service *services.AIService) {
	aiService = service
	config.SetAIProviderConfigured(service != nil)
}

// GetAIService returns the AI service
func GetAIService() *services.AIService {
	return aiService
}

// SetDeliveryService sets the delivery service for the server package
func SetDeliveryService(service *services.DeliveryService) {
	deliveryService = service
}

// GetDeliveryService returns the delivery service
func GetDeliveryService() *services.DeliveryService {
	return deliveryService
}
