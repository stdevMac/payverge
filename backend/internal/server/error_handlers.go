package server

import (
	"log"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stdevMac/shares/internal/database"
)

// ErrorLogPayload represents the structure of the error log received from the frontend
type ErrorLogPayload struct {
	Timestamp      string                 `json:"timestamp"`
	Error          string                 `json:"error"`
	Component      string                 `json:"component"`
	Function       string                 `json:"function"`
	AdditionalInfo map[string]interface{} `json:"additionalInfo,omitempty"`
}

// LogError handles error logging requests from the frontend
func LogError(c *gin.Context) {
	var payload ErrorLogPayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		c.JSON(400, gin.H{"error": "Invalid payload format"})
		return
	}

	// Parse the timestamp
	timestamp, err := time.Parse(time.RFC3339, payload.Timestamp)
	if err != nil {
		c.JSON(400, gin.H{"error": "Invalid timestamp format"})
		return
	}

	// Create error log entry
	errorLog := &database.ErrorLog{
		Timestamp:      timestamp,
		Error:          payload.Error,
		Component:      payload.Component,
		Function:       payload.Function,
		AdditionalInfo: payload.AdditionalInfo,
		CreatedAt:      time.Now(),
	}

	// Store in database
	if err := database.StoreErrorLog(c.Request.Context(), errorLog); err != nil {
		log.Printf("Failed to store error log: %v", err)
		c.JSON(500, gin.H{"error": "Failed to store error log"})
		return
	}

	// Also log to console for immediate visibility
	log.Printf("Frontend Error - Component: %s, Function: %s, Error: %s",
		payload.Component,
		payload.Function,
		payload.Error,
	)

	if payload.AdditionalInfo != nil {
		log.Printf("Additional Info: %+v", payload.AdditionalInfo)
	}

	c.JSON(200, gin.H{"status": "error logged"})
}
