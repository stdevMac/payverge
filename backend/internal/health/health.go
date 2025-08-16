package health

import (
	"context"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/readpref"
)

// HealthCheck represents the health status of a service component
type HealthCheck struct {
	Service string `json:"service"`
	Status  string `json:"status"`
	Message string `json:"message,omitempty"`
	Latency string `json:"latency,omitempty"`
}

// HealthResponse represents the overall health response
type HealthResponse struct {
	Status    string        `json:"status"`
	Timestamp time.Time     `json:"timestamp"`
	Version   string        `json:"version"`
	Uptime    string        `json:"uptime"`
	Checks    []HealthCheck `json:"checks"`
}

var startTime = time.Now()

// Handler returns a health check handler
func Handler(mongoClient *mongo.Client, version string) gin.HandlerFunc {
	return func(c *gin.Context) {
		checks := []HealthCheck{}
		overallStatus := "healthy"

		// Check MongoDB connection
		mongoCheck := checkMongoDB(mongoClient)
		checks = append(checks, mongoCheck)
		if mongoCheck.Status != "healthy" {
			overallStatus = "unhealthy"
		}

		// Check system resources
		systemCheck := checkSystem()
		checks = append(checks, systemCheck)
		if systemCheck.Status != "healthy" {
			overallStatus = "degraded"
		}

		response := HealthResponse{
			Status:    overallStatus,
			Timestamp: time.Now(),
			Version:   version,
			Uptime:    time.Since(startTime).String(),
			Checks:    checks,
		}

		statusCode := http.StatusOK
		if overallStatus == "unhealthy" {
			statusCode = http.StatusServiceUnavailable
		} else if overallStatus == "degraded" {
			statusCode = http.StatusPartialContent
		}

		c.JSON(statusCode, response)
	}
}

// ReadinessHandler returns a readiness check handler
func ReadinessHandler(mongoClient *mongo.Client) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Check if all critical services are ready
		mongoCheck := checkMongoDB(mongoClient)
		
		if mongoCheck.Status != "healthy" {
			c.JSON(http.StatusServiceUnavailable, gin.H{
				"status": "not ready",
				"reason": "database not available",
			})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"status": "ready",
		})
	}
}

// LivenessHandler returns a liveness check handler
func LivenessHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"status": "alive",
			"timestamp": time.Now(),
		})
	}
}

// checkMongoDB checks MongoDB connection health
func checkMongoDB(client *mongo.Client) HealthCheck {
	if client == nil {
		return HealthCheck{
			Service: "mongodb",
			Status:  "unhealthy",
			Message: "client not initialized",
		}
	}

	start := time.Now()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := client.Ping(ctx, readpref.Primary())
	latency := time.Since(start)

	if err != nil {
		return HealthCheck{
			Service: "mongodb",
			Status:  "unhealthy",
			Message: err.Error(),
			Latency: latency.String(),
		}
	}

	status := "healthy"
	if latency > 1*time.Second {
		status = "degraded"
	}

	return HealthCheck{
		Service: "mongodb",
		Status:  status,
		Latency: latency.String(),
	}
}

// checkSystem checks basic system health
func checkSystem() HealthCheck {
	// Basic system check - can be expanded with memory, disk, etc.
	return HealthCheck{
		Service: "system",
		Status:  "healthy",
		Message: "system resources normal",
	}
}
