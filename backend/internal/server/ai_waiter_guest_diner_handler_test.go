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
)

func seedGuestDinerVenue(t *testing.T, suffix string) (*database.Business, *database.Table) {
	t.Helper()
	db := setupAIWaiterTestDB(t)
	business := createAIWaiterBusiness(t, suffix, true)
	business.DefaultLanguage = "en"
	business.SourceLanguage = "en"
	require.NoError(t, db.Save(business).Error)
	table := createAIWaiterTable(t, business.ID, "M03Y18GB3P")

	require.NoError(t, db.AutoMigrate(
		&database.Menu{},
		&database.Bundle{},
		&database.Translation{},
		&database.Bill{},
	))

	dateNight, err := json.Marshal([]database.BundleItemRef{
		{MenuItemID: "demo-steak", Name: "Steak Plate", Quantity: 1},
		{MenuItemID: "demo-spritz", Name: "Demo Spritz", Quantity: 2},
	})
	require.NoError(t, err)
	require.NoError(t, db.Create(&database.Menu{
		BusinessID: business.ID,
		Categories: `[{"id":"mains","name":"Mains","items":[{"id":"demo-steak","name":"Steak Plate","price":29,"is_available":true,"currency":"USD"},{"id":"harvest-bowl","name":"Harvest Bowl","price":14,"is_available":true,"currency":"USD"}]},{"id":"drinks","name":"Drinks","items":[{"id":"demo-spritz","name":"Demo Spritz","price":11,"is_available":true,"currency":"USD"},{"id":"iced-tea","name":"Iced Tea","price":4,"is_available":true,"currency":"USD"}]}]`,
		IsActive:   true,
		Version:    1,
	}).Error)
	bundle := database.Bundle{
		BusinessID:  business.ID,
		Name:        "Date Night for Two",
		Description: "Steak plate, two spritz cocktails, and a chocolate tart to share.",
		Price:       68,
		Currency:    "USD",
		Items:       string(dateNight),
		IsActive:    true,
	}
	require.NoError(t, db.Create(&bundle).Error)
	now := time.Now().UTC()
	require.NoError(t, db.Create(&database.Translation{
		BusinessID: business.ID, EntityType: bundleTranslationEntityType, EntityID: bundle.ID,
		FieldName: "name", LanguageCode: "es", OriginalText: bundle.Name,
		TranslatedText: "Noche romántica para dos", CreatedAt: now, UpdatedAt: now,
	}).Error)

	return business, table
}

func postGuestWaiterTurn(t *testing.T, business *database.Business, table *database.Table, sessionID, language, message string) (string, assistantcontract.Status) {
	t.Helper()
	conv := createAIWaiterConversation(t, business.ID, sessionID)
	conv.TableCode = table.TableCode
	conv.Language = language
	require.NoError(t, database.GetDB().Save(conv).Error)

	SetAIService(nil)
	t.Cleanup(func() { SetAIService(nil) })

	router := gin.New()
	router.POST("/ai-waiter/:businessId", HandleAIWaiter)
	w := performAIWaiterRequest(t, router, http.MethodPost, fmt.Sprintf("/ai-waiter/%d", business.ID), map[string]any{
		"session_token": conv.SessionID, "mode": "ordering", "table_code": table.TableCode, "language": language,
		"history": []map[string]any{{"role": "user", "content": message}},
	})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	waitForPromotionTranslationBackfills(2 * time.Second)
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
	return content, payload.ResponseV2.Status
}

func TestHandleAIWaiter_HayDateNightMatchesLiveBundle(t *testing.T) {
	gin.SetMode(gin.TestMode)
	business, table := seedGuestDinerVenue(t, "hay-date-night")

	content, status := postGuestWaiterTurn(t, business, table, "s-678-date-night", "es", "hay date night?")
	lower := strings.ToLower(content)
	assert.NotEqual(t, services.ClarifyItemMessage("es"), content)
	assert.NotEqual(t, waiterFinalizerCopy("es").safeFallback, content)
	assert.NotContains(t, lower, "no encontr")
	assert.True(
		t,
		strings.Contains(lower, "date night") || strings.Contains(lower, "noche rom"),
		"must name the live Date Night bundle, got %q",
		content,
	)
	assert.NotEqual(t, assistantcontract.StatusNeedsClarification, status)
}

func TestHandleAIWaiter_EnglishBillAskOnSpanishSessionIsNotCannedStub(t *testing.T) {
	gin.SetMode(gin.TestMode)
	business, table := seedGuestDinerVenue(t, "en-bill-es-session")

	content, status := postGuestWaiterTurn(t, business, table, "s-679-en-bill", "es", "can I get the bill please?")
	assert.NotEqual(t, waiterFinalizerCopy("es").safeFallback, content)
	assert.NotContains(t, content, "Puedo ayudarte con preguntas sobre el menú y tu visita.")
	assert.Equal(t, services.WaiterBillAnswer("es", services.WaiterVisitFacts{}), content)
	assert.Equal(t, assistantcontract.StatusComplete, status)
}
