package logger

import (
	"os"
	"time"

	appconfig "github.com/stdevmac/payverge/backend/internal/config"

	"github.com/sirupsen/logrus"
)

// Logger is initialized at package load so it is never nil — callers that run
// before main() invokes InitLogger() (e.g. unit tests exercising helpers that
// log) get a usable default logger instead of a nil-pointer panic. InitLogger()
// reconfigures output/level/format for the running environment.
var Logger = logrus.New()

// InitLogger initializes the structured logger
func InitLogger() {
	Logger = logrus.New()

	// Set output to stdout
	Logger.SetOutput(os.Stdout)

	// Set log level based on environment
	if appconfig.IsProductionMode(false) {
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
