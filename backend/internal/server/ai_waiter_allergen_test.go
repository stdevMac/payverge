package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/llm"
	"github.com/stdevmac/payverge/backend/internal/services"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type countingProvider struct{ calls int }

func (c *countingProvider) Generate(_ context.Context, _ llm.GenerateRequest) (*llm.Response, error) {
	c.calls++
	return &llm.Response{Text: "This dish is totally nut free, go ahead!"}, nil
}

func TestHandleAIWaiter_AllergenIntentDeterministicNoLLM(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupAIWaiterTestDB(t)
	business := createAIWaiterBusiness(t, "allergen-det", true)
	table := createAIWaiterTable(t, business.ID, "ALG-1")
	require.NoError(t, db.AutoMigrate(&database.Menu{}))
	require.NoError(t, db.Create(&database.Menu{BusinessID: business.ID,
		Categories: `[{"name":"Mains","items":[{"name":"Peanut Burger","allergens":["peanuts","gluten"]}]}]`,
		IsActive:   true, Version: 1}).Error)

	conv := createAIWaiterConversation(t, business.ID, "s-alg")
	conv.TableCode = table.TableCode
	require.NoError(t, db.Save(conv).Error)

	cp := &countingProvider{}
	svc, err := services.NewAIService(cp, llm.ModelConfig{Chat: "m", Image: "i", Director: "d", Menu: "mn"})
	require.NoError(t, err)
	SetAIService(svc)
	t.Cleanup(func() { SetAIService(nil) })

	router := gin.New()
	router.POST("/ai-waiter/:businessId", HandleAIWaiter)
	w := performAIWaiterRequest(t, router, http.MethodPost, fmt.Sprintf("/ai-waiter/%d", business.ID), map[string]any{
		"session_token": "s-alg", "mode": "ordering", "table_code": table.TableCode, "language": "en",
		"history": []map[string]any{{"role": "user", "content": "Does the Peanut Burger contain peanuts?"}},
	})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Equal(t, 0, cp.calls, "allergen intent must be answered deterministically without an LLM call")

	var resp struct {
		Parts []struct {
			Text string `json:"text"`
		} `json:"parts"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.NotEmpty(t, resp.Parts)
	assert.Contains(t, resp.Parts[0].Text, "peanuts")
	assert.Contains(t, resp.Parts[0].Text, services.AllergenDisclaimer("en"))
}

func TestHandleAIWaiter_ModelOnlyAllergenClaimIsDiscarded(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupAIWaiterTestDB(t)
	business := createAIWaiterBusiness(t, "allergen-post", true)
	table := createAIWaiterTable(t, business.ID, "ALG-2")
	require.NoError(t, db.AutoMigrate(&database.Menu{}))
	require.NoError(t, db.Create(&database.Menu{BusinessID: business.ID, Categories: "[]", IsActive: true, Version: 1}).Error)

	conv := createAIWaiterConversation(t, business.ID, "s-post")
	conv.TableCode = table.TableCode
	require.NoError(t, db.Save(conv).Error)

	cp := &countingProvider{}
	svc, _ := services.NewAIService(cp, llm.ModelConfig{Chat: "m", Image: "i", Director: "d", Menu: "mn"})
	SetAIService(svc)
	t.Cleanup(func() { SetAIService(nil) })

	router := gin.New()
	router.POST("/ai-waiter/:businessId", HandleAIWaiter)
	w := performAIWaiterRequest(t, router, http.MethodPost, fmt.Sprintf("/ai-waiter/%d", business.ID), map[string]any{
		"session_token": "s-post", "mode": "ordering", "table_code": table.TableCode, "language": "en",
		"history": []map[string]any{{"role": "user", "content": "tell me about the burger"}},
	})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.NotContains(t, w.Body.String(), "totally nut free")
	assert.NotContains(t, w.Body.String(), services.AllergenDisclaimer("en"))
}

type staffConfirmProvider struct{}

func (c *staffConfirmProvider) Generate(_ context.Context, _ llm.GenerateRequest) (*llm.Response, error) {
	return &llm.Response{Text: "I can't be sure about the nuts — please confirm with our staff before ordering."}, nil
}

func TestHandleAIWaiter_AllergenPostCheckNoDoubleWhenModelConfirmsWithStaff(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupAIWaiterTestDB(t)
	business := createAIWaiterBusiness(t, "allergen-staff", true)
	table := createAIWaiterTable(t, business.ID, "ALG-3")
	require.NoError(t, db.AutoMigrate(&database.Menu{}))
	require.NoError(t, db.Create(&database.Menu{BusinessID: business.ID, Categories: "[]", IsActive: true, Version: 1}).Error)

	conv := createAIWaiterConversation(t, business.ID, "s-staff")
	conv.TableCode = table.TableCode
	require.NoError(t, db.Save(conv).Error)

	svc, _ := services.NewAIService(&staffConfirmProvider{}, llm.ModelConfig{Chat: "m", Image: "i", Director: "d", Menu: "mn"})
	SetAIService(svc)
	t.Cleanup(func() { SetAIService(nil) })

	router := gin.New()
	router.POST("/ai-waiter/:businessId", HandleAIWaiter)
	// User message must NOT trigger the deterministic allergen path (no allergen keywords),
	// so the LLM is called and its response triggers the post-check.
	w := performAIWaiterRequest(t, router, http.MethodPost, fmt.Sprintf("/ai-waiter/%d", business.ID), map[string]any{
		"session_token": "s-staff", "mode": "ordering", "table_code": table.TableCode, "language": "en",
		"history": []map[string]any{{"role": "user", "content": "is the salad ok for me?"}},
	})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	// Model-owned allergen prose is discarded rather than treated as a trusted
	// safety fact or confirmation channel.
	disclaimer := services.AllergenDisclaimer("en")
	assert.NotContains(t, w.Body.String(), disclaimer,
		"discarded model prose must not trigger a surviving allergen answer")
	assert.NotContains(t, w.Body.String(), "please confirm with our staff")
}
