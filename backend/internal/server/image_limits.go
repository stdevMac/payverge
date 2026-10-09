package server

import (
	"errors"
	"fmt"
	"log"
	"net/http"
	"strconv"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/metrics"

	sentry "github.com/getsentry/sentry-go"
	"github.com/gin-gonic/gin"
)

// settingInt reads a positive integer platform setting, falling back to def
// when it is unset, unreadable, malformed, or non-positive. The non-positive
// clamp mirrors database.GetImageLimitSettings on purpose: this is the reader
// on the reserve hot path, and a stored "0" would make the daily predicate
// `daily_used < limit` unsatisfiable and 429 every generation platform-wide.
func settingInt(key string, def int) int {
	v, err := database.GetDBWrapper().GetPlatformSettingValue(key)
	if err != nil || v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil || n <= 0 {
		return def
	}
	return n
}

func imageDailyLimit() int {
	return settingInt(database.SettingImageDailyLimit, database.DefaultImageDailyLimit)
}

func imageMonthlyAlert() int {
	return settingInt(database.SettingImageMonthlyAlert, database.DefaultImageMonthlyAlert)
}

// respondImageDailyLimit writes the fair-use rejection and reports whether it
// claimed the error. The body is self-contained so the client renders the whole
// message without a follow-up request — there is no usage endpoint to call.
func respondImageDailyLimit(c *gin.Context, err error) bool {
	var limitErr database.ImageDailyLimitError
	if !errors.As(err, &limitErr) {
		return false
	}
	c.JSON(http.StatusTooManyRequests, gin.H{
		"error":             "Daily AI image limit reached",
		"code":              "image_daily_limit_reached",
		"daily_limit":       limitErr.Limit,
		"resets_in_seconds": limitErr.ResetsInSeconds(),
	})
	return true
}

// notifyImageUsageAlert reports a business crossing the monthly fair-use alert
// threshold. It never blocks the generation — the daily cap is the only thing
// that refuses work.
func notifyImageUsageAlert(res database.ImageUsageReservation) {
	if !res.AlertTriggered {
		return
	}
	metrics.AIImageMonthlyAlerts.Inc()
	sentry.CaptureMessage(fmt.Sprintf(
		"business %d crossed the monthly AI image alert threshold (%d generations this period)",
		res.BusinessID, res.MonthlyUsed,
	))
	log.Printf("image usage alert: business %d at %d generations this period", res.BusinessID, res.MonthlyUsed)
}
