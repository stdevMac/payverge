package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stdevmac/payverge/backend/internal/assistantcontract"
	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/services"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func seedStorefrontConciergeVenue(t *testing.T, db *gorm.DB) *database.Business {
	t.Helper()
	business := createAIWaiterBusiness(t, "storefront-concierge", true)
	business.Name = "Payverge AI Pro Demo Lounge"
	business.IsActive = true
	business.Timezone = "UTC"
	require.NoError(t, db.Save(business).Error)

	require.NoError(t, db.AutoMigrate(
		&database.Menu{},
		&database.Bundle{},
		&database.BusinessOperatingHours{},
		&database.ReservationSettings{},
	))

	require.NoError(t, db.Create(&database.Menu{
		BusinessID: business.ID,
		Categories: `[{"id":"mains","name":"Mains","items":[{"id":"demo-steak","name":"Steak Plate","price":29,"is_available":true,"currency":"USD"},{"id":"harvest-bowl","name":"Harvest Bowl","price":14,"is_available":true,"currency":"USD"}]},{"id":"drinks","name":"Drinks","items":[{"id":"demo-spritz","name":"Demo Spritz","price":11,"is_available":true,"currency":"USD"}]},{"id":"desserts","name":"Desserts","items":[{"id":"chocolate-tart","name":"Chocolate Tart","price":9,"is_available":true,"currency":"USD"}]}]`,
		IsActive:   true,
		Version:    1,
	}).Error)

	dateNight, err := json.Marshal([]database.BundleItemRef{
		{MenuItemID: "demo-steak", Name: "Steak Plate", Quantity: 1},
		{MenuItemID: "demo-spritz", Name: "Demo Spritz", Quantity: 2},
	})
	require.NoError(t, err)
	require.NoError(t, db.Create(&database.Bundle{
		BusinessID: business.ID,
		Name:       "Date Night for Two",
		Price:      48,
		Currency:   "USD",
		Items:      string(dateNight),
		IsActive:   true,
	}).Error)

	for day := 0; day <= 6; day++ {
		require.NoError(t, db.Create(&database.BusinessOperatingHours{
			BusinessID: business.ID,
			DayOfWeek:  day,
			OpenTime:   "11:00",
			CloseTime:  "23:00",
			CreatedAt:  time.Now(),
			UpdatedAt:  time.Now(),
		}).Error)
	}
	require.NoError(t, db.Create(&database.ReservationSettings{
		BusinessID:   business.ID,
		Enabled:      true,
		MinPartySize: 2,
		MaxPartySize: 8,
	}).Error)
	return business
}

func postStorefrontConcierge(t *testing.T, router *gin.Engine, businessID uint, sessionID, language, message string) *httptestBody {
	t.Helper()
	w := performAIWaiterRequest(t, router, http.MethodPost, fmt.Sprintf("/ai-waiter/%d", businessID), map[string]any{
		"session_token": sessionID,
		"mode":          "concierge",
		"table_code":    "",
		"language":      language,
		"history":       []map[string]any{{"role": "user", "content": message}},
	})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var payload struct {
		ResponseV2 assistantcontract.Response `json:"response_v2"`
		Parts      []struct {
			Text string `json:"text"`
		} `json:"parts"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &payload))
	content := payload.ResponseV2.Answer.Content
	if content == "" && len(payload.Parts) > 0 {
		content = payload.Parts[0].Text
	}
	return &httptestBody{raw: w.Body.String(), content: content, status: payload.ResponseV2.Status}
}

type httptestBody struct {
	raw     string
	content string
	status  assistantcontract.Status
}

func TestHandleAIWaiter_StorefrontConciergeGroundsHoursDateNightReservations(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupAIWaiterTestDB(t)
	business := seedStorefrontConciergeVenue(t, db)

	previous := loadUnrecommendableMenuItemIDs
	loadUnrecommendableMenuItemIDs = func(uint) (map[string]bool, error) {
		return map[string]bool{"demo-steak": true}, nil
	}
	t.Cleanup(func() { loadUnrecommendableMenuItemIDs = previous })

	SetAIService(nil)
	t.Cleanup(func() { SetAIService(nil) })

	router := gin.New()
	router.POST("/ai-waiter/:businessId/session", CreateAIWaiterSession)
	router.POST("/ai-waiter/:businessId", HandleAIWaiter)

	session := performAIWaiterRequest(t, router, http.MethodPost,
		fmt.Sprintf("/ai-waiter/%d/session", business.ID),
		map[string]any{"mode": "concierge", "table_code": "", "language": "en"})
	require.Equal(t, http.StatusOK, session.Code, session.Body.String())
	var minted struct {
		SessionToken string `json:"session_token"`
		Greeting     string `json:"greeting"`
	}
	require.NoError(t, json.Unmarshal(session.Body.Bytes(), &minted))
	assert.NotContains(t, strings.ToLower(minted.Greeting), "help ordering")
	// No model is wired (SetAIService(nil)), so the scripted no-AI greeting applies.
	assert.Equal(t, services.WaiterScriptedGreeting("en", "Alfred", business.Name), minted.Greeting)

	conv, ok := database.FindAiWaiterConversation(minted.SessionToken, business.ID, "concierge", "")
	require.True(t, ok)
	require.NotNil(t, conv)

	t.Run("hours use today's close not the degraded stub", func(t *testing.T) {
		got := postStorefrontConcierge(t, router, business.ID, minted.SessionToken, "en", "When do you close?")
		assert.NotEqual(t, waiterFinalizerCopy("en").safeFallback, got.content)
		assert.Contains(t, got.content, "23:00")
		assert.Equal(t, assistantcontract.StatusComplete, got.status)
	})

	t.Run("date night is named without the full bundle title", func(t *testing.T) {
		got := postStorefrontConcierge(t, router, business.ID, minted.SessionToken, "en", "What's Date Night?")
		assert.NotEqual(t, services.ClarifyItemMessage("en"), got.content)
		assert.NotContains(t, strings.ToLower(got.content), "couldn't find")
		assert.Contains(t, strings.ToLower(got.content), "date night")
	})

	t.Run("reservations are a grounded visit answer", func(t *testing.T) {
		got := postStorefrontConcierge(t, router, business.ID, minted.SessionToken, "en", "Do you take reservations tonight for two at 8pm?")
		assert.NotEqual(t, waiterFinalizerCopy("en").safeFallback, got.content)
		assert.Contains(t, got.content, "2")
		assert.Contains(t, got.content, "8")
	})

	t.Run("desserts plus close names chocolate tart and hours", func(t *testing.T) {
		got := postStorefrontConcierge(t, router, business.ID, minted.SessionToken, "en", "What desserts do you have and when do you close?")
		assert.NotEqual(t, services.ClarifyItemMessage("en"), got.content)
		assert.Contains(t, strings.ToLower(got.content), "chocolate tart")
		assert.Contains(t, got.content, "23:00")
	})

	t.Run("parking and delivery are not missing-menu stubs", func(t *testing.T) {
		got := postStorefrontConcierge(t, router, business.ID, minted.SessionToken, "es", "Hay estacionamiento? Y el steak plate se puede pedir para delivery?")
		lower := strings.ToLower(got.content)
		assert.NotEqual(t, services.ClarifyItemMessage("es"), got.content)
		assert.NotContains(t, lower, "no encontré")
		assert.Contains(t, lower, "estacionamiento")
		assert.Contains(t, lower, "delivery")
		assert.Contains(t, lower, "steak plate")
	})
}
