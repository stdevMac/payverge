package server

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/stdevmac/payverge/backend/internal/database"
)

func setupTelegramConnectionTestDB(t *testing.T) *database.Business {
	t.Helper()

	gormDB, err := gorm.Open(sqlite.Open("file::memory:"), &gorm.Config{})
	require.NoError(t, err)

	sqlDB, err := gormDB.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)

	require.NoError(t, gormDB.AutoMigrate(
		&database.Business{},
		&database.TelegramConnectionToken{},
	))
	database.SetTestDB(gormDB)

	business := &database.Business{
		BusinessId:     "telegram-connection-test",
		OwnerAddress:   "owner",
		Name:           "Telegram Connection Test",
		SettlementAddr: "settlement",
		TippingAddr:    "tipping",
	}
	require.NoError(t, gormDB.Create(business).Error)
	return business
}

func TestGenerateTelegramConnectionToken_PersistsHashOnly(t *testing.T) {
	business := setupTelegramConnectionTestDB(t)
	now := time.Date(2026, 5, 11, 12, 0, 0, 0, time.UTC)
	service := TelegramConnectionService{
		BotUsername: "CustomPayvergeBot",
		TokenTTL:    15 * time.Minute,
		Now:         func() time.Time { return now },
	}

	result, err := service.GenerateConnectionToken(business.ID, nil, nil, "203.0.113.10")

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.True(t, strings.HasPrefix(result.Token, "pv_tg_"))
	assert.Equal(t, now.Add(15*time.Minute), result.ExpiresAt)
	assert.Equal(t, "https://t.me/CustomPayvergeBot?start="+result.Token, result.URL)

	var stored database.TelegramConnectionToken
	require.NoError(t, database.GetDB().First(&stored).Error)
	assert.Equal(t, business.ID, stored.BusinessID)
	assert.Equal(t, HashTelegramConnectionToken(result.Token), stored.TokenHash)
	assert.NotEqual(t, result.Token, stored.TokenHash)
	assert.Len(t, stored.TokenHash, 64)
	assert.Equal(t, "203.0.113.10", stored.CreatedIP)
}

func TestGenerateTelegramConnectionToken_RevokesPreviousPendingToken(t *testing.T) {
	business := setupTelegramConnectionTestDB(t)
	now := time.Date(2026, 5, 11, 12, 0, 0, 0, time.UTC)
	service := TelegramConnectionService{
		BotUsername: "CustomPayvergeBot",
		TokenTTL:    15 * time.Minute,
		Now:         func() time.Time { return now },
	}

	first, err := service.GenerateConnectionToken(business.ID, nil, nil, "203.0.113.10")
	require.NoError(t, err)
	second, err := service.GenerateConnectionToken(business.ID, nil, nil, "203.0.113.10")
	require.NoError(t, err)
	assert.NotEqual(t, first.Token, second.Token)

	var firstStored database.TelegramConnectionToken
	require.NoError(t, database.GetDB().Where("token_hash = ?", HashTelegramConnectionToken(first.Token)).First(&firstStored).Error)
	require.NotNil(t, firstStored.RevokedAt)

	var secondStored database.TelegramConnectionToken
	require.NoError(t, database.GetDB().Where("token_hash = ?", HashTelegramConnectionToken(second.Token)).First(&secondStored).Error)
	assert.Nil(t, secondStored.RevokedAt)
	assert.Nil(t, secondStored.UsedAt)
}

func TestGenerateTelegramConnectionToken_RateLimitsRepeatedGeneration(t *testing.T) {
	business := setupTelegramConnectionTestDB(t)
	now := time.Date(2026, 5, 11, 12, 0, 0, 0, time.UTC)
	service := TelegramConnectionService{
		BotUsername: "CustomPayvergeBot",
		TokenTTL:    15 * time.Minute,
		Now:         func() time.Time { return now },
	}

	for i := 0; i < telegramConnectionTokenRateLimit; i++ {
		_, err := service.GenerateConnectionToken(business.ID, nil, nil, "203.0.113.10")
		require.NoError(t, err)
	}

	_, err := service.GenerateConnectionToken(business.ID, nil, nil, "203.0.113.10")
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrTelegramConnectionTokenRateLimited)
}

func TestHashTelegramConnectionToken_IsStableAndDoesNotExposeRawToken(t *testing.T) {
	token := "pv_tg_example"

	first := HashTelegramConnectionToken(token)
	second := HashTelegramConnectionToken(token)

	assert.Equal(t, first, second)
	assert.NotContains(t, first, token)
	assert.Len(t, first, 64)
}

func TestBuildTelegramConnectionURLUsesConfiguredBotUsername(t *testing.T) {
	t.Setenv("TELEGRAM_BOT_USERNAME", "@CustomPayvergeBot")

	url := DefaultTelegramConnectionService().BuildTelegramConnectionURL("abc123")

	assert.Equal(t, "https://t.me/CustomPayvergeBot?start=abc123", url)
}

// No TELEGRAM_BOT_USERNAME means no link at all — never a hard-coded default
// bot that would route a fork's operators to the upstream project's bot.
func TestBuildTelegramConnectionURLEmptyWhenUnset(t *testing.T) {
	t.Setenv("TELEGRAM_BOT_USERNAME", "")

	assert.Empty(t, DefaultTelegramConnectionService().BuildTelegramConnectionURL("tok"))
	assert.False(t, TelegramBotUsernameConfigured())

	t.Setenv("TELEGRAM_BOT_USERNAME", "  @  ")
	assert.Empty(t, DefaultTelegramConnectionService().BuildTelegramConnectionURL("tok"))
	assert.False(t, TelegramBotUsernameConfigured())

	t.Setenv("TELEGRAM_BOT_USERNAME", "@MyRestoBot")
	assert.True(t, TelegramBotUsernameConfigured())
}

func TestGenerateTelegramConnectionToken_RefusesWithoutBotUsername(t *testing.T) {
	business := setupTelegramConnectionTestDB(t)
	service := TelegramConnectionService{BotUsername: "", TokenTTL: 15 * time.Minute}

	result, err := service.GenerateConnectionToken(business.ID, nil, nil, "203.0.113.10")

	require.ErrorIs(t, err, ErrTelegramBotUsernameNotConfigured)
	assert.Nil(t, result)
	var count int64
	require.NoError(t, database.GetDB().Model(&database.TelegramConnectionToken{}).Count(&count).Error)
	assert.Zero(t, count, "no token may be minted without a bot to redeem it")
}

func TestTelegramConnectionServiceBuildURLUsesConfiguredBotUsername(t *testing.T) {
	service := TelegramConnectionService{BotUsername: "@ServiceBot"}

	url := service.BuildTelegramConnectionURL("abc123")

	assert.Equal(t, "https://t.me/ServiceBot?start=abc123", url)
}
