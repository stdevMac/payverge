package logger

import (
	"context"
	"os"
	"time"

	"github.com/sirupsen/logrus"
)

var Logger *logrus.Logger

// InitLogger initializes the structured logger
func InitLogger() {
	Logger = logrus.New()
	
	// Set output to stdout
	Logger.SetOutput(os.Stdout)
	
	// Set log level based on environment
	env := os.Getenv("NODE_ENV")
	if env == "production" {
		Logger.SetLevel(logrus.InfoLevel)
		Logger.SetFormatter(&logrus.JSONFormatter{
			TimestampFormat: time.RFC3339,
		})
	} else {
		Logger.SetLevel(logrus.DebugLevel)
		Logger.SetFormatter(&logrus.TextFormatter{
			FullTimestamp:   true,
			TimestampFormat: time.RFC3339,
			ForceColors:     true,
		})
	}
}

// LoggerMiddleware adds request logging with structured fields
func LoggerMiddleware() func(c interface{}) {
	return func(c interface{}) {
		// This will be implemented based on your framework
		// For now, providing the structure
	}
}

// WithRequestID adds request ID to log context
func WithRequestID(ctx context.Context, requestID string) *logrus.Entry {
	return Logger.WithContext(ctx).WithField("request_id", requestID)
}

// WithUserAddress adds user address to log context
func WithUserAddress(ctx context.Context, address string) *logrus.Entry {
	return Logger.WithContext(ctx).WithField("user_address", address)
}

// WithError adds error details to log context
func WithError(ctx context.Context, err error) *logrus.Entry {
	return Logger.WithContext(ctx).WithError(err)
}

// LogRequest logs HTTP request details
func LogRequest(method, path, ip, userAgent string, statusCode int, duration time.Duration) {
	Logger.WithFields(logrus.Fields{
		"method":     method,
		"path":       path,
		"ip":         ip,
		"user_agent": userAgent,
		"status":     statusCode,
		"duration":   duration.Milliseconds(),
		"type":       "http_request",
	}).Info("HTTP request processed")
}

// LogAuth logs authentication events
func LogAuth(event, address, ip string, success bool) {
	Logger.WithFields(logrus.Fields{
		"event":   event,
		"address": address,
		"ip":      ip,
		"success": success,
		"type":    "auth",
	}).Info("Authentication event")
}

// LogSecurity logs security-related events
func LogSecurity(event, ip, details string, severity string) {
	entry := Logger.WithFields(logrus.Fields{
		"event":   event,
		"ip":      ip,
		"details": details,
		"type":    "security",
	})
	
	switch severity {
	case "critical":
		entry.Error("Security event")
	case "warning":
		entry.Warn("Security event")
	default:
		entry.Info("Security event")
	}
}

// LogDatabase logs database operations
func LogDatabase(operation, collection string, duration time.Duration, err error) {
	fields := logrus.Fields{
		"operation":  operation,
		"collection": collection,
		"duration":   duration.Milliseconds(),
		"type":       "database",
	}
	
	if err != nil {
		Logger.WithFields(fields).WithError(err).Error("Database operation failed")
	} else {
		Logger.WithFields(fields).Debug("Database operation completed")
	}
}

// LogWeb3 logs Web3/blockchain operations
func LogWeb3(operation, network, address string, gasUsed uint64, err error) {
	fields := logrus.Fields{
		"operation": operation,
		"network":   network,
		"address":   address,
		"gas_used":  gasUsed,
		"type":      "web3",
	}
	
	if err != nil {
		Logger.WithFields(fields).WithError(err).Error("Web3 operation failed")
	} else {
		Logger.WithFields(fields).Info("Web3 operation completed")
	}
}

// LogEmail logs email sending operations
func LogEmail(to, template string, success bool, err error) {
	fields := logrus.Fields{
		"to":       to,
		"template": template,
		"success":  success,
		"type":     "email",
	}
	
	if err != nil {
		Logger.WithFields(fields).WithError(err).Error("Email sending failed")
	} else {
		Logger.WithFields(fields).Info("Email sent successfully")
	}
}
