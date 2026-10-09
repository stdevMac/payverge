package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/llm"
	"github.com/stdevmac/payverge/backend/internal/services"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestRespondImageDailyLimit_WritesSelfContained429(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	err := database.ImageDailyLimitError{
		Limit:    500,
		ResetsAt: time.Now().UTC().Add(6 * time.Hour),
	}

	assert.True(t, respondImageDailyLimit(c, err), "must claim the error")
	assert.Equal(t, http.StatusTooManyRequests, w.Code, "429, not 402 — there is nothing to buy")

	var body map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.Equal(t, "image_daily_limit_reached", body["code"])
	assert.Equal(t, float64(500), body["daily_limit"])
	assert.Greater(t, body["resets_in_seconds"], float64(0), "client must not need a second fetch")
}

func TestRespondImageDailyLimit_IgnoresOtherErrors(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	assert.False(t, respondImageDailyLimit(c, assert.AnError))
	assert.Equal(t, 200, w.Code, "must not write a body it does not own")
}

// sharedResponderGuard is the exact statement every generation handler must
// place ahead of its generic 500 branch. Matching the whole block rather than
// the call name is deliberate: a bare substring count is satisfied by a call
// that has been disabled in place (`if false && respondImageDailyLimit(...)`)
// or moved into a comment, and would still report a green suite while the site
// answered 500. Two of the five sites are covered by this check alone —
// EnhanceMenuItemImage and CleanUpMarketingImage fetch from the public asset
// bucket before reserving, so they cannot be booted here.
// TestImageGenerationHandlers_DailyLimitReturns429 covers the other three
// behaviorally.
const sharedResponderGuard = "\tif respondImageDailyLimit(c, genErr) {\n\t\treturn\n\t}\n"

func TestAllImageCallSitesUseTheSharedResponder(t *testing.T) {
	expected := map[string]int{
		"ai_menu_handlers.go":   2,
		"menu_enhancements.go":  1,
		"marketing_handlers.go": 2,
	}
	for file, want := range expected {
		src, err := os.ReadFile(file)
		require.NoError(t, err)
		got := strings.Count(string(src), sharedResponderGuard)
		assert.Equal(t, want, got, "%s must route every generation failure through the shared responder", file)
		assert.Equal(t, want, strings.Count(string(src), "respondImageDailyLimit(c,"),
			"%s must not mention the shared responder outside the guard block", file)
	}
}

// setupImageLimitsTestDB gives every test in this file its own empty database.
// The DSN is keyed on the test name deliberately: the anonymous
// `file::memory:?cache=shared` handle is one database process-wide — this file
// is not even its only user, setupPromoSQLiteDB opens the same one — so a
// platform setting or usage row written by one test survived into the next and
// made these assertions depend on execution order. A settings override written
// by TestImageLimitReaders_HonorOperatorOverride was reaching imageDailyLimit()
// inside TestRegenerateImage_DailyLimitReturns429 under a `-run` subset.
func setupImageLimitsTestDB(t *testing.T) {
	t.Helper()

	dsn := fmt.Sprintf("file:image_limits_%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	gormDB, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}

	sqlDB, err := gormDB.DB()
	if err != nil {
		t.Fatalf("get sql.DB: %v", err)
	}
	sqlDB.SetMaxOpenConns(1)

	database.SetTestDB(gormDB)
	if err := gormDB.AutoMigrate(&database.PlatformSettings{}, &database.Business{}, &database.AIImageUsage{}); err != nil {
		t.Fatalf("automigrate: %v", err)
	}
}

func writeImageLimitSetting(t *testing.T, key, value string) {
	t.Helper()
	require.NoError(t, database.GetDBWrapper().SetPlatformSetting(key, value, database.CategoryGeneral, false))
}

// TestImageLimitReaders_DefaultWhenUnset pins the fallback: a platform nobody has
// tuned reserves against the built-in fair-use numbers, not against zero.
func TestImageLimitReaders_DefaultWhenUnset(t *testing.T) {
	setupImageLimitsTestDB(t)

	assert.Equal(t, database.DefaultImageDailyLimit, imageDailyLimit())
	assert.Equal(t, database.DefaultImageMonthlyAlert, imageMonthlyAlert())
}

// TestImageLimitReaders_HonorOperatorOverride pins the branch the admin form
// depends on: a stored value must actually reach the reserve call. The two knobs
// carry different numbers so a reader wired to the wrong key fails here instead
// of coincidentally matching.
func TestImageLimitReaders_HonorOperatorOverride(t *testing.T) {
	setupImageLimitsTestDB(t)
	writeImageLimitSetting(t, database.SettingImageDailyLimit, "120")
	writeImageLimitSetting(t, database.SettingImageMonthlyAlert, "3400")

	assert.Equal(t, 120, imageDailyLimit(), "a configured daily limit must not be silently ignored")
	assert.Equal(t, 3400, imageMonthlyAlert(), "a configured monthly alert must not be silently ignored")
}

// TestImageLimitReaders_ClampUnusableValues covers the platform-wide outage
// vector settingInt's clamp exists for: "0" and negatives parse cleanly, so
// without the clamp the reserve predicate (daily_used < limit) could never match
// and every generation on the platform would 429 forever. Malformed text rides
// the same branch.
func TestImageLimitReaders_ClampUnusableValues(t *testing.T) {
	for _, stored := range []string{"0", "-5", "not-a-number"} {
		t.Run(stored, func(t *testing.T) {
			setupImageLimitsTestDB(t)
			writeImageLimitSetting(t, database.SettingImageDailyLimit, stored)
			writeImageLimitSetting(t, database.SettingImageMonthlyAlert, stored)

			assert.Equal(t, database.DefaultImageDailyLimit, imageDailyLimit(),
				"an unusable stored daily limit must never reach the reserve predicate")
			assert.Equal(t, database.DefaultImageMonthlyAlert, imageMonthlyAlert())
		})
	}
}

// seedImageLimitsBusiness creates a business with no usage row yet; the first
// reservation creates it.
func seedImageLimitsBusiness(t *testing.T, slug, owner string) *database.Business {
	t.Helper()
	biz := &database.Business{
		BusinessId:      slug,
		Name:            "Image Limits Test Biz",
		OwnerAddress:    owner,
		SettlementAddr:  "0x1111111111111111111111111111111111111111",
		TippingAddr:     "0x2222222222222222222222222222222222222222",
		DefaultCurrency: "USD",
		DefaultLanguage: "en",
		IsActive:        true,
		CreatedAt:       time.Now(),
		UpdatedAt:       time.Now(),
	}
	require.NoError(t, database.GetDB().Create(biz).Error)
	return biz
}

func readImageUsage(t *testing.T, businessID uint) database.AIImageUsage {
	t.Helper()
	var row database.AIImageUsage
	require.NoError(t, database.GetDB().Where("business_id = ?", businessID).First(&row).Error)
	return row
}

func TestReserveGenerateRefundImageDeliveryFailureRefundsExactBucket(t *testing.T) {
	setupImageLimitsTestDB(t)
	business := seedImageLimitsBusiness(t, "delivery-refund", "0xdeliveryrefund")
	originalValidator := validateGeneratedImageDelivery
	t.Cleanup(func() { validateGeneratedImageDelivery = originalValidator })
	validateGeneratedImageDelivery = func(context.Context, *services.GeneratedImage) error {
		return errors.New("cdn miss")
	}
	_, err := reserveGenerateRefundImage(
		context.Background(),
		business,
		func() (*services.GeneratedImage, error) {
			return &services.GeneratedImage{
				URL: "https://images.payverge.io/x.png", MIMEType: "image/png",
			}, nil
		},
	)
	require.ErrorContains(t, err, "cdn miss")
	after := readImageUsage(t, business.ID)
	require.Equal(t, 0, after.DailyUsed)
	require.Equal(t, 0, after.MonthlyUsed)
}

func TestReserveGenerateRefundImageDeliverySuccessSettlesOneGeneration(t *testing.T) {
	setupImageLimitsTestDB(t)
	business := seedImageLimitsBusiness(t, "delivery-success", "0xdeliverysuccess")
	originalValidator := validateGeneratedImageDelivery
	validateGeneratedImageDelivery = func(context.Context, *services.GeneratedImage) error { return nil }
	t.Cleanup(func() { validateGeneratedImageDelivery = originalValidator })
	_, err := reserveGenerateRefundImage(
		context.Background(),
		business,
		func() (*services.GeneratedImage, error) {
			return &services.GeneratedImage{
				URL: "https://images.payverge.io/x.png", MIMEType: "image/png",
			}, nil
		},
	)
	require.NoError(t, err)
	row := readImageUsage(t, business.ID)
	require.Equal(t, 1, row.DailyUsed)
	require.Equal(t, 1, row.MonthlyUsed)
}

func TestReserveGenerateRefundImageSurfacesRefundFailure(t *testing.T) {
	setupImageLimitsTestDB(t)
	business := seedImageLimitsBusiness(t, "delivery-refund-error", "0xdeliveryrefunderr")
	errDelivery := errors.New("cdn miss")
	errRefund := errors.New("database unavailable")
	originalValidator := validateGeneratedImageDelivery
	originalRefund := refundReservedImageGeneration
	t.Cleanup(func() {
		validateGeneratedImageDelivery = originalValidator
		refundReservedImageGeneration = originalRefund
	})
	validateGeneratedImageDelivery = func(context.Context, *services.GeneratedImage) error {
		return errDelivery
	}
	refundReservedImageGeneration = func(database.ImageUsageReservation) error {
		return errRefund
	}

	_, err := reserveGenerateRefundImage(
		context.Background(),
		business,
		func() (*services.GeneratedImage, error) {
			return &services.GeneratedImage{
				URL: "https://images.payverge.io/x.png", MIMEType: "image/png",
			}, nil
		},
	)
	// Both causes must survive the join: the caller-facing failure stays
	// matchable through the wrap and the refund failure is not swallowed.
	// Asserted on the error values, not on the joining prose.
	require.ErrorIs(t, err, errDelivery)
	require.ErrorContains(t, err, errRefund.Error())
}

// seedCappedImageUsageBusiness creates a business whose daily window is already
// fully consumed for the current UTC day, so the next reservation trips the
// backstop instead of reaching the provider. The monthly counter carries the
// same generations because nothing else was produced this cycle — it is the
// same number, not the same knob.
func seedCappedImageUsageBusiness(t *testing.T, slug, owner string) *database.Business {
	t.Helper()
	dailySpent := database.DefaultImageDailyLimit

	biz := seedImageLimitsBusiness(t, slug, owner)
	now := time.Now().UTC()
	row := &database.AIImageUsage{
		BusinessID:         biz.ID,
		DailyUsed:          dailySpent,
		DailyPeriodStart:   time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC),
		MonthlyUsed:        dailySpent,
		MonthlyAnchorDay:   1,
		MonthlyPeriodStart: time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC),
	}
	require.NoError(t, database.GetDB().Create(row).Error)
	return biz
}

// unreachableImageProvider fails the test if the model is ever asked to draw.
// Every case below is already over its daily allowance, so the reservation must
// fail before any inference is bought — a call here means the 429 is answered
// too late to be a backstop.
type unreachableImageProvider struct{ t *testing.T }

func (p *unreachableImageProvider) Generate(context.Context, llm.GenerateRequest) (*llm.Response, error) {
	p.t.Helper()
	p.t.Error("daily limit must be answered before the provider is called")
	return nil, errors.New("provider must not be reached")
}

// TestImageGenerationHandlers_DailyLimitReturns429 boots the real handlers, not
// their source text, for the three generation routes that reserve without first
// fetching an operator asset. Each must answer with the shared fair-use response
// — 429 with a self-contained body, not the old 402 and not a generic 500 — and
// must not reach the provider. TestAllImageCallSitesUseTheSharedResponder proves
// all five sites carry the guard; these cases prove the guard actually answers,
// which a source-text count cannot. The remaining two sites (EnhanceMenuItemImage,
// CleanUpMarketingImage) download from the public asset bucket before reserving
// and stay text-guarded only.
func TestImageGenerationHandlers_DailyLimitReturns429(t *testing.T) {
	cases := []struct {
		name    string
		slug    string
		owner   string
		body    string
		handler func(*gin.Context)
	}{
		{
			name:    "RegenerateMenuItemImage",
			slug:    "regen-capped",
			owner:   "0xcappedregen",
			body:    `{"item_name":"Burger","item_description":"A juicy burger"}`,
			handler: RegenerateMenuItemImage,
		},
		{
			name:    "GenerateMenuImage",
			slug:    "generate-capped",
			owner:   "0xcappedgenerate",
			body:    `{"name":"Burger","description":"A juicy burger"}`,
			handler: GenerateMenuImage,
		},
		{
			// visual_mood is supplied so the handler skips the marketing-settings
			// lookup; the branch under test is the reservation, not the default.
			name:    "GenerateMarketingImage",
			slug:    "marketing-capped",
			owner:   "0xcappedmarketing",
			body:    `{"name":"Burger","description":"A juicy burger","play":"featured_dish","visual_mood":"natural"}`,
			handler: GenerateMarketingImage,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			gin.SetMode(gin.TestMode)
			setupImageLimitsTestDB(t)

			// Non-nil services so the handlers pass their nil guards and reach the
			// reservation; the provider underneath refuses to generate.
			provider := &unreachableImageProvider{t: t}
			aiSvc, err := services.NewAIService(provider, llm.ModelConfig{
				Chat: "test-chat", Image: "test-image", Director: "test-director", Menu: "test-menu",
			})
			require.NoError(t, err)
			SetAIService(aiSvc)
			SetMenuAIService(services.NewMenuAIService(provider, llm.ModelConfig{Image: "test-image", Menu: "test-menu"}))
			t.Cleanup(func() {
				SetAIService(nil)
				SetMenuAIService(nil)
			})

			biz := seedCappedImageUsageBusiness(t, tc.slug, tc.owner)

			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Params = gin.Params{{Key: "id", Value: fmt.Sprintf("%d", biz.ID)}}
			c.Set("address", tc.owner)
			req := httptest.NewRequest(http.MethodPost, "/", io.NopCloser(strings.NewReader(tc.body)))
			req.Header.Set("Content-Type", "application/json")
			c.Request = req

			tc.handler(c)

			require.Equal(t, http.StatusTooManyRequests, w.Code, w.Body.String())
			var resp map[string]interface{}
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
			assert.Equal(t, "image_daily_limit_reached", resp["code"])
			assert.Equal(t, float64(database.DefaultImageDailyLimit), resp["daily_limit"])
			assert.Greater(t, resp["resets_in_seconds"], float64(0))
		})
	}
}

// reserveGenerateRefundImage refunds the reservation and then re-raises a
// provider panic, so gin's recovery middleware still records it while the
// operator's daily/monthly image quota is not burned.
func TestReserveGenerateRefundImageRefundsOnPanic(t *testing.T) {
	setupImageLimitsTestDB(t)
	business := seedImageLimitsBusiness(t, "panic-refund", "0xpanicrefund")

	var recovered any
	func() {
		defer func() { recovered = recover() }()
		_, _ = reserveGenerateRefundImage(
			context.Background(),
			business,
			func() (*services.GeneratedImage, error) {
				panic("provider blew up")
			},
		)
	}()
	require.Equal(t, "provider blew up", recovered, "panic must be re-raised after the refund")

	row := readImageUsage(t, business.ID)
	require.Equal(t, 0, row.DailyUsed, "reserved then refunded after the panic -> net zero")
	require.Equal(t, 0, row.MonthlyUsed)
}
