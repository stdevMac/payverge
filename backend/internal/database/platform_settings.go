package database

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/stdevmac/payverge/backend/internal/security"

	"gorm.io/gorm"
)

// ErrInvalidImageLimit rejects a fair-use configuration that would break the
// cap arithmetic rather than relax it.
var ErrInvalidImageLimit = errors.New("image limit settings must be positive")

// Platform Settings Keys
const (
	// AI Image Fair-Use Settings — single source of truth shared with server readers.
	SettingImageDailyLimit   = "ai_image_daily_limit"
	SettingImageMonthlyAlert = "ai_image_monthly_alert"

	// Categories
	CategoryGeneral = "general"
)

// Fair-use defaults applied when the operator has never tuned the settings.
const (
	DefaultImageDailyLimit   = 500
	DefaultImageMonthlyAlert = 2000
)

// ImageLimitSettings is the fair-use configuration the server operator tunes
// with `server settings image-limits` (docs/self-hosting/ai.md). Neither
// value is customer-visible; the daily limit only surfaces inside the 429 body
// when it trips, and the monthly alert is internal.
type ImageLimitSettings struct {
	DailyLimit   int `json:"daily_limit"`
	MonthlyAlert int `json:"monthly_alert"`
}

// GetImageLimitSettings reads both fair-use keys, falling back to the built-in
// defaults when a value is unset, malformed, or non-positive so a fresh
// platform is never uncapped and a tampered one is never hard-blocked. The
// non-positive clamp is load-bearing: the reserve chokepoint consumes the daily
// limit as `daily_used < limit`, so a stored "0" (or a negative) would make that
// predicate unsatisfiable and 429 every image request on the platform forever.
func (db *DB) GetImageLimitSettings() (ImageLimitSettings, error) {
	var s ImageLimitSettings
	readInt := func(key string, def int) (int, error) {
		v, err := db.GetPlatformSettingValue(key)
		if err != nil {
			return 0, err
		}
		if v == "" {
			return def, nil
		}
		n, convErr := strconv.Atoi(v)
		if convErr != nil || n <= 0 {
			return def, nil
		}
		return n, nil
	}
	var err error
	if s.DailyLimit, err = readInt(SettingImageDailyLimit, DefaultImageDailyLimit); err != nil {
		return s, fmt.Errorf("get image limit settings: %w", err)
	}
	if s.MonthlyAlert, err = readInt(SettingImageMonthlyAlert, DefaultImageMonthlyAlert); err != nil {
		return s, fmt.Errorf("get image limit settings: %w", err)
	}
	return s, nil
}

// SetImageLimitSettings persists both fair-use keys atomically. Non-positive
// values are refused before the transaction opens so an operator can never
// write a configuration that hard-blocks generation for every business; the
// reader clamps the same way for rows that predate this guard.
func (db *DB) SetImageLimitSettings(s ImageLimitSettings) error {
	if s.DailyLimit <= 0 || s.MonthlyAlert <= 0 {
		return fmt.Errorf("%w: daily_limit=%d monthly_alert=%d", ErrInvalidImageLimit, s.DailyLimit, s.MonthlyAlert)
	}
	return db.conn.Transaction(func(tx *gorm.DB) error {
		txDB := &DB{conn: tx}
		pairs := [][2]string{
			{SettingImageDailyLimit, strconv.Itoa(s.DailyLimit)},
			{SettingImageMonthlyAlert, strconv.Itoa(s.MonthlyAlert)},
		}
		for _, p := range pairs {
			if err := txDB.SetPlatformSetting(p[0], p[1], CategoryGeneral, false); err != nil {
				return err
			}
		}
		return nil
	})
}

// encodePlatformSettingValue encrypts secret values before they hit the row.
// Fail closed: if encryption is unavailable, refuse to store the plaintext.
func encodePlatformSettingValue(value string, isSecret bool) (string, error) {
	if !isSecret || value == "" || strings.HasPrefix(value, "v1:") {
		return value, nil
	}
	encrypted, err := security.EncryptSecret(value)
	if err != nil {
		return "", fmt.Errorf("refusing to store platform secret in plaintext: %w", err)
	}
	return encrypted, nil
}

// decodePlatformSettingValue decrypts in place. Legacy plaintext rows (no v1:
// prefix) pass through unchanged and get encrypted on their next save.
func decodePlatformSettingValue(setting *PlatformSettings) error {
	if setting == nil || !setting.IsSecret || !strings.HasPrefix(setting.Value, "v1:") {
		return nil
	}
	decrypted, err := security.DecryptSecret(setting.Value)
	if err != nil {
		return fmt.Errorf("failed to decrypt platform secret %s: %w", setting.Key, err)
	}
	setting.Value = decrypted
	return nil
}

// GetPlatformSetting retrieves a platform setting by key
func (db *DB) GetPlatformSetting(key string) (*PlatformSettings, error) {
	var setting PlatformSettings
	err := db.conn.Where("key = ?", key).First(&setting).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get platform setting: %w", err)
	}
	if err := decodePlatformSettingValue(&setting); err != nil {
		return nil, err
	}
	return &setting, nil
}

// GetPlatformSettingValue retrieves just the value of a platform setting
func (db *DB) GetPlatformSettingValue(key string) (string, error) {
	setting, err := db.GetPlatformSetting(key)
	if err != nil {
		return "", err
	}
	if setting == nil {
		return "", nil
	}
	return setting.Value, nil
}

// SetPlatformSetting creates or updates a platform setting
func (db *DB) SetPlatformSetting(key, value, category string, isSecret bool) error {
	encoded, err := encodePlatformSettingValue(value, isSecret)
	if err != nil {
		return err
	}

	var setting PlatformSettings
	err = db.conn.Where("key = ?", key).First(&setting).Error

	if errors.Is(err, gorm.ErrRecordNotFound) {
		// Create new setting
		setting = PlatformSettings{
			Key:      key,
			Value:    encoded,
			Category: category,
			IsSecret: isSecret,
		}
		return db.conn.Create(&setting).Error
	} else if err != nil {
		return fmt.Errorf("failed to check existing setting: %w", err)
	}

	// Update existing setting
	setting.Value = encoded
	setting.Category = category
	setting.IsSecret = isSecret
	return db.conn.Save(&setting).Error
}

// GetPlatformSettingsByCategory retrieves all settings in a category
func (db *DB) GetPlatformSettingsByCategory(category string) ([]PlatformSettings, error) {
	var settings []PlatformSettings
	err := db.conn.Where("category = ?", category).Find(&settings).Error
	if err != nil {
		return nil, fmt.Errorf("failed to get platform settings by category: %w", err)
	}
	for i := range settings {
		if err := decodePlatformSettingValue(&settings[i]); err != nil {
			return nil, err
		}
	}
	return settings, nil
}
