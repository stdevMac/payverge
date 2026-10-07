package observability

import (
	"math"
	"os"
	"strconv"
	"strings"

	"github.com/sirupsen/logrus"
)

type Config struct {
	Enabled          bool
	DSN              string
	Environment      string
	Release          string
	Service          string
	EnableTracing    bool
	TracesSampleRate float64
	EnableLogs       bool
	LogLevels        []logrus.Level
}

func LoadConfigFromEnv(service string) Config {
	dsn := strings.TrimSpace(os.Getenv("SENTRY_DSN"))
	environment := normalizeEnvironmentName(firstNonEmpty(
		os.Getenv("SENTRY_ENVIRONMENT"),
		os.Getenv("APP_ENV"),
		os.Getenv("ENV"),
		os.Getenv("NODE_ENV"),
		"development",
	))

	normalizedEnvironment := strings.ToLower(environment)
	enabled := dsn != "" && (normalizedEnvironment == "staging" || normalizedEnvironment == "production")
	if explicitEnabled, ok := parseOptionalBool(os.Getenv("SENTRY_ENABLED")); ok {
		enabled = dsn != "" && explicitEnabled
	}

	return Config{
		Enabled:          enabled,
		DSN:              dsn,
		Environment:      environment,
		Release:          firstNonEmpty(os.Getenv("SENTRY_RELEASE"), os.Getenv("NEXT_PUBLIC_RELEASE_SHA"), "dev"),
		Service:          service,
		EnableTracing:    parseBoolDefault(os.Getenv("SENTRY_ENABLE_TRACING"), true),
		TracesSampleRate: parseSampleRate(os.Getenv("SENTRY_TRACES_SAMPLE_RATE"), 0.05),
		EnableLogs:       parseBoolDefault(os.Getenv("SENTRY_ENABLE_LOGS"), false),
		LogLevels:        parseLogLevels(os.Getenv("SENTRY_LOG_LEVELS")),
	}
}

// normalizeEnvironmentName maps the ENV=prod alias to "production" so the
// enablement gate and the reported Sentry environment stay consistent with the
// rest of the codebase, which treats prod as production (see CLAUDE.md).
func normalizeEnvironmentName(value string) string {
	if strings.EqualFold(value, "prod") {
		return "production"
	}

	return value
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed != "" {
			return trimmed
		}
	}

	return ""
}

func parseOptionalBool(value string) (bool, bool) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return false, false
	}

	parsed, err := strconv.ParseBool(trimmed)
	if err != nil {
		return false, false
	}

	return parsed, true
}

func parseBoolDefault(value string, fallback bool) bool {
	parsed, ok := parseOptionalBool(value)
	if !ok {
		return fallback
	}

	return parsed
}

func parseSampleRate(value string, fallback float64) float64 {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return fallback
	}

	parsed, err := strconv.ParseFloat(trimmed, 64)
	if err != nil {
		return fallback
	}
	if math.IsNaN(parsed) || math.IsInf(parsed, 0) {
		return fallback
	}
	if parsed < 0 {
		return 0
	}
	if parsed > 1 {
		return 1
	}

	return parsed
}

func parseLogLevels(value string) []logrus.Level {
	levels := make([]logrus.Level, 0)
	for _, entry := range strings.Split(value, ",") {
		trimmed := strings.TrimSpace(entry)
		if trimmed == "" {
			continue
		}

		level, err := logrus.ParseLevel(trimmed)
		if err != nil {
			continue
		}
		levels = append(levels, level)
	}
	if len(levels) == 0 {
		return []logrus.Level{logrus.ErrorLevel, logrus.FatalLevel, logrus.PanicLevel}
	}

	return levels
}
