package server

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/metrics"
)

const (
	telegramConnectionTokenPrefix      = "pv_tg_"
	telegramConnectionTokenRandomBytes = 32
	defaultTelegramConnectionTokenTTL  = 15 * time.Minute
	minTelegramConnectionTokenTTL      = 5 * time.Minute
	maxTelegramConnectionTokenTTL      = 60 * time.Minute
	telegramConnectionTokenRateLimit   = 5
	telegramConnectionTokenRateWindow  = 15 * time.Minute
)

var ErrTelegramConnectionTokenRateLimited = errors.New("telegram connection token generation rate limited")

// ErrTelegramBotUsernameNotConfigured is returned when a connect link is
// requested but TELEGRAM_BOT_USERNAME is unset. There is deliberately no
// default bot: a fork must never hand its operators a t.me link to the
// upstream project's bot.
var ErrTelegramBotUsernameNotConfigured = errors.New("TELEGRAM_BOT_USERNAME is not configured")

type TelegramConnectionService struct {
	BotUsername string
	TokenTTL    time.Duration
	Now         func() time.Time
}

type TelegramConnectionTokenResult struct {
	Token     string    `json:"-"`
	URL       string    `json:"url"`
	ExpiresAt time.Time `json:"expires_at"`
}

func (s *TelegramConnectionService) GenerateConnectionToken(businessID uint, createdByUserID *uint, createdByStaffID *uint, createdIP string) (*TelegramConnectionTokenResult, error) {
	if businessID == 0 {
		return nil, errors.New("business ID is required")
	}
	// Refuse before touching the DB: a token minted without a bot to redeem
	// it would only revoke the operator's previous pending link.
	if normalizeTelegramBotUsername(s.BotUsername) == "" {
		metrics.TelegramConnectionFailures.WithLabelValues("bot_username_not_configured").Inc()
		return nil, ErrTelegramBotUsernameNotConfigured
	}

	now := s.now()
	if recent, err := database.CountRecentTelegramConnectionTokens(businessID, now.Add(-telegramConnectionTokenRateWindow)); err != nil {
		return nil, err
	} else if recent >= telegramConnectionTokenRateLimit {
		metrics.TelegramConnectionFailures.WithLabelValues("token_generation_rate_limited").Inc()
		return nil, ErrTelegramConnectionTokenRateLimited
	}

	token, err := generateOpaqueTelegramConnectionToken()
	if err != nil {
		return nil, err
	}
	expiresAt := now.Add(s.ttl())

	if err := database.RevokePendingTelegramConnectionTokens(businessID); err != nil {
		return nil, err
	}

	if err := database.CreateTelegramConnectionToken(&database.TelegramConnectionToken{
		BusinessID:       businessID,
		TokenHash:        HashTelegramConnectionToken(token),
		ExpiresAt:        expiresAt,
		CreatedByUserID:  createdByUserID,
		CreatedByStaffID: createdByStaffID,
		CreatedIP:        strings.TrimSpace(createdIP),
		CreatedAt:        now,
	}); err != nil {
		return nil, err
	}
	metrics.TelegramConnectionTokensIssued.Inc()

	return &TelegramConnectionTokenResult{
		Token:     token,
		URL:       s.BuildTelegramConnectionURL(token),
		ExpiresAt: expiresAt,
	}, nil
}

// BuildTelegramConnectionURL returns the t.me deep link for token, or "" when
// no bot username is configured.
func (s *TelegramConnectionService) BuildTelegramConnectionURL(token string) string {
	username := normalizeTelegramBotUsername(s.BotUsername)
	if username == "" {
		return ""
	}
	return fmt.Sprintf("https://t.me/%s?start=%s", username, token)
}

func (s *TelegramConnectionService) now() time.Time {
	if s.Now != nil {
		return s.Now().UTC()
	}
	return time.Now().UTC()
}

func (s *TelegramConnectionService) ttl() time.Duration {
	if s.TokenTTL > 0 {
		return clampTelegramConnectionTokenTTL(s.TokenTTL)
	}
	return telegramConnectionTokenTTLFromEnv()
}

func DefaultTelegramConnectionService() *TelegramConnectionService {
	return &TelegramConnectionService{
		BotUsername: getTelegramBotUsername(),
		TokenTTL:    telegramConnectionTokenTTLFromEnv(),
	}
}

func HashTelegramConnectionToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func generateOpaqueTelegramConnectionToken() (string, error) {
	randomBytes := make([]byte, telegramConnectionTokenRandomBytes)
	if _, err := rand.Read(randomBytes); err != nil {
		return "", fmt.Errorf("failed to generate telegram connection token: %w", err)
	}
	return telegramConnectionTokenPrefix + base64.RawURLEncoding.EncodeToString(randomBytes), nil
}

func telegramConnectionTokenTTLFromEnv() time.Duration {
	raw := strings.TrimSpace(os.Getenv("TELEGRAM_CONNECTION_TOKEN_TTL_MINUTES"))
	if raw == "" {
		return defaultTelegramConnectionTokenTTL
	}
	minutes, err := strconv.Atoi(raw)
	if err != nil {
		return defaultTelegramConnectionTokenTTL
	}
	return clampTelegramConnectionTokenTTL(time.Duration(minutes) * time.Minute)
}

func clampTelegramConnectionTokenTTL(ttl time.Duration) time.Duration {
	if ttl < minTelegramConnectionTokenTTL {
		return minTelegramConnectionTokenTTL
	}
	if ttl > maxTelegramConnectionTokenTTL {
		return maxTelegramConnectionTokenTTL
	}
	return ttl
}

// TelegramBotUsernameConfigured reports whether TELEGRAM_BOT_USERNAME is set,
// which the operator connect-link flow requires whenever Telegram is enabled.
func TelegramBotUsernameConfigured() bool {
	return getTelegramBotUsername() != ""
}

func getTelegramBotUsername() string {
	return normalizeTelegramBotUsername(os.Getenv("TELEGRAM_BOT_USERNAME"))
}

// normalizeTelegramBotUsername trims whitespace and a leading "@". Blank stays
// blank — never a hard-coded default bot.
func normalizeTelegramBotUsername(botUsername string) string {
	return strings.TrimPrefix(strings.TrimSpace(botUsername), "@")
}
