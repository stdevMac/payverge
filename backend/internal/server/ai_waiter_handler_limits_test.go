package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/stdevmac/payverge/backend/internal/database"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestHandleAIWaiter_PayloadLimits ensures that the public POST /ai-waiter/:businessId
// endpoint rejects oversize payloads with 413 and code=payload_too_large before
// any downstream LLM call is made. This prevents token-cost blowup against the
// business AI budget and constrains the prompt-injection surface.
func TestHandleAIWaiter_PayloadLimits(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupAIWaiterTestDB(t)

	business := createAIWaiterBusiness(t, "payload-limits", true)
	table := createAIWaiterTable(t, business.ID, "LIMIT-TBL-1")

	router := gin.New()
	router.POST("/ai-waiter/:businessId", HandleAIWaiter)

	expectPayloadTooLarge := func(t *testing.T, body map[string]any) {
		t.Helper()
		w := performAIWaiterRequest(t, router, http.MethodPost, fmt.Sprintf("/ai-waiter/%d", business.ID), body)
		assert.Equal(t, http.StatusRequestEntityTooLarge, w.Code, "expected 413, got %d body=%s", w.Code, w.Body.String())

		var resp map[string]any
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		assert.Equal(t, "payload_too_large", resp["code"])
	}

	validSingleMessage := []map[string]any{
		{"role": "user", "content": "hi"},
	}

	t.Run("history exceeding 40 messages returns 413", func(t *testing.T) {
		history := make([]map[string]any, 41)
		for i := range history {
			history[i] = map[string]any{"role": "user", "content": "hi"}
		}
		expectPayloadTooLarge(t, map[string]any{
			"session_token": "session-history-too-long",
			"mode":          "ordering",
			"table_code":    table.TableCode,
			"history":       history,
		})
	})

	t.Run("single message content over 4096 bytes returns 413", func(t *testing.T) {
		oversizeContent := strings.Repeat("a", 4097)
		expectPayloadTooLarge(t, map[string]any{
			"session_token": "session-content-too-long",
			"mode":          "ordering",
			"table_code":    table.TableCode,
			"history": []map[string]any{
				{"role": "user", "content": oversizeContent},
			},
		})
	})

	t.Run("bill_context over 2048 bytes returns 413", func(t *testing.T) {
		expectPayloadTooLarge(t, map[string]any{
			"session_token": "session-billctx-too-long",
			"mode":          "ordering",
			"table_code":    table.TableCode,
			"bill_context":  strings.Repeat("b", 2049),
			"history":       validSingleMessage,
		})
	})

	t.Run("mode over 32 bytes returns 413", func(t *testing.T) {
		// Note: mode limit check fires before mode-validity check (which would
		// return 400). A 33-byte mode string therefore hits the size limit.
		expectPayloadTooLarge(t, map[string]any{
			"session_token": "session-mode-too-long",
			"mode":          strings.Repeat("m", 33),
			"table_code":    table.TableCode,
			"history":       validSingleMessage,
		})
	})

	t.Run("boundary payload is not rejected by size limits", func(t *testing.T) {
		// 40 messages, content exactly 4096 bytes, bill_context exactly 2048
		// bytes, mode exactly 32 bytes. Note mode must remain a valid value
		// ("ordering" or "concierge"); "ordering" padded won't normalize, so
		// we use "concierge" padded with trailing spaces - normalizeAIWaiterMode
		// trims whitespace. The net payload at the byte boundary must NOT be
		// rejected with 413. The downstream Gemini call will fail (no AI
		// service configured in tests) - that's fine. We only assert !413.
		history := make([]map[string]any, 40)
		for i := range history {
			history[i] = map[string]any{"role": "user", "content": strings.Repeat("c", 4096)}
		}
		// mode padded to exactly 32 bytes with spaces. normalizeAIWaiterMode
		// trims and lowercases.
		padded := "concierge" + strings.Repeat(" ", 32-len("concierge"))
		require.Equal(t, 32, len(padded))

		w := performAIWaiterRequest(t, router, http.MethodPost, fmt.Sprintf("/ai-waiter/%d", business.ID), map[string]any{
			"session_token": "session-boundary",
			"mode":          padded,
			"table_code":    table.TableCode,
			"bill_context":  strings.Repeat("b", 2048),
			"history":       history,
		})
		assert.NotEqual(t, http.StatusRequestEntityTooLarge, w.Code, "boundary payload must not be rejected with 413, body=%s", w.Body.String())
	})

	// Prove business fixture wasn't a limit-masking red herring by retrieving
	// it via the DB (sanity check).
	var reloaded database.Business
	require.NoError(t, database.GetDB().First(&reloaded, business.ID).Error)
	assert.True(t, reloaded.AiSettings.AiEnabled)
}
