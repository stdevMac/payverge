package main

import (
	"context"
	"time"

	"github.com/stdevmac/payverge/backend/internal/logger"
	"github.com/stdevmac/payverge/backend/internal/middleware"
	"github.com/stdevmac/payverge/backend/internal/s3"

	"github.com/gin-gonic/gin"
)

// initObjectStorage wires the public and protected object stores from
// STORAGE_DRIVER (local by default) and the already-resolved S3 settings.
//
//   - STORAGE_DRIVER=local needs nothing S3: objects live under STORAGE_DIR.
//     If that directory is unusable the server still starts (menus, orders,
//     bills, KDS and payments do not need it) and uploads fail loudly.
//   - STORAGE_DRIVER=s3 fails fast on bad config, and in production refuses to
//     start when the probe reads a protected object anonymously.
func initObjectStorage(productionMode bool, settings s3.S3Settings) {
	cfg, err := s3.ConfigFromEnv(productionMode, settings)
	if err != nil {
		logger.Logger.Fatalf("Storage configuration: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	report, err := s3.Init(ctx, cfg)
	for _, warning := range report.Warnings {
		logger.Logger.Warnf("Storage: %s", warning)
	}
	if err != nil {
		if cfg.Driver == s3.DriverS3 {
			logger.Logger.Fatalf("Failed to initialize S3 storage: %v", err)
		}
		logger.Logger.Errorf("Local storage unavailable (uploads will fail until %s=%q is writable): %v", s3.EnvStorageDir, cfg.Dir, err)
		return
	}
	mediaBase := report.PublicURL + s3.MediaPathPrefix
	if settings.PublicBaseURL != "" && report.Driver == s3.DriverS3 {
		mediaBase = settings.PublicBaseURL + "/"
	}
	logger.Logger.Infof("Storage: driver=%s dir=%s public objects at %s<key>", report.Driver, report.Dir, mediaBase)
	if mediaBase == s3.MediaPathPrefix {
		logger.Logger.Infof("Storage: media URLs are relative; browsers reach /media through the frontend proxy (BACKEND_INTERNAL_URL) or a reverse-proxy /media route to this backend")
	}
}

// Per-IP budget for /media. The route is exempt from the global API limiter,
// so it carries its own token bucket; 0 disables it (a CDN in front).
const (
	envMediaRateLimitRPM   = "MEDIA_RATE_LIMIT_REQUESTS_PER_MINUTE"
	envMediaRateLimitBurst = "MEDIA_RATE_LIMIT_BURST"
)

// registerMediaRoutes mounts GET/HEAD /media/*key (public objects only) behind
// the per-IP media limiter. GET and HEAD share one bucket.
func registerMediaRoutes(r gin.IRoutes) {
	limit := middleware.MediaRateLimit(
		intEnv(envMediaRateLimitRPM, middleware.MediaRateLimitRequestsPerMinute),
		intEnv(envMediaRateLimitBurst, middleware.MediaRateLimitBurst),
	)
	handler := s3.MediaHandler()
	r.GET("/media/*key", limit, handler)
	r.HEAD("/media/*key", limit, handler)
}
