package services

import (
	"context"
	"strings"
	"testing"

	"github.com/stdevmac/payverge/backend/internal/analytics"
	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/llm"
	"github.com/stdevmac/payverge/backend/internal/services/director_tools"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDirectorUserLanguageAnchor_NativeName(t *testing.T) {
	es := directorUserLanguageAnchor("es")
	assert.Contains(t, es, "Español")
	assert.NotContains(t, es, "respond in: es")

	esAR := directorUserLanguageAnchor("es-AR")
	assert.Contains(t, esAR, "Español (Argentina)")

	en := directorUserLanguageAnchor("en")
	assert.Contains(t, en, "English")

	for _, a := range []string{es, esAR, en} {
		assert.True(t, strings.Contains(strings.ToLower(a), "json"), "anchor must mention JSON string values: %q", a)
	}
}

func TestDirectorLanguageMismatch_TriggersOneRegeneration(t *testing.T) {
	db := newServiceTestDB(t)
	db.GetGorm().AutoMigrate(&database.Payment{}, &database.Bill{})
	business := createTestBusinessForService(t, db, "Lang Restaurant")
	require.NotZero(t, business.ID)

	englishFinal := `{"summary":"You can review the sales with your team today.","diagnosis":"All good","evidence":[],"actions":[],"expected_impact":"","follow_ups":[]}`
	spanishFinal := `{"summary":"Los ingresos crecen de forma constante esta semana.","diagnosis":"Todo bien","evidence":[],"actions":[],"expected_impact":"","follow_ups":[]}`

	cap := &capturingDirectorProvider{scripted: []*llm.Response{
		newTextResponse(englishFinal), // tool-allowed probe
		newTextResponse(englishFinal), // schema finalization (wrong language)
		newTextResponse(spanishFinal), // probe after regeneration
		newTextResponse(spanishFinal), // schema finalization (corrected)
	}}

	svc := newDirectorServiceWithProvider(t, db, cap)
	res, err := svc.Ask(context.Background(), DirectorAskRequest{
		BusinessID: business.ID,
		Message:    "¿Por qué los martes son lentos?",
		Locale:     "es",
	})
	require.NoError(t, err)
	assert.Contains(t, res.Response.Summary, "ingresos")
	assert.Equal(t, 4, cap.calls, "exactly one regeneration on language mismatch")
}

func TestDirectorLanguageMatch_NoRegeneration(t *testing.T) {
	db := newServiceTestDB(t)
	db.GetGorm().AutoMigrate(&database.Payment{}, &database.Bill{})
	business := createTestBusinessForService(t, db, "Lang OK Restaurant")
	spanishFinal := `{"summary":"Los ingresos crecen de forma constante.","diagnosis":"ok","evidence":[],"actions":[],"expected_impact":"","follow_ups":[]}`
	cap := &capturingDirectorProvider{scripted: []*llm.Response{
		newTextResponse(spanishFinal),
		newTextResponse(spanishFinal),
	}}
	svc := newDirectorServiceWithProvider(t, db, cap)
	_, err := svc.Ask(context.Background(), DirectorAskRequest{
		BusinessID: business.ID, Message: "¿Cómo van las ventas?", Locale: "es",
	})
	require.NoError(t, err)
	assert.Equal(t, 2, cap.calls, "matching language must not regenerate")
}

func newDirectorServiceWithProvider(t *testing.T, db *database.DB, prov llm.Provider) *DirectorConsoleService {
	t.Helper()
	ai, err := NewAIService(prov, llm.ModelConfig{Director: "test-model"})
	require.NoError(t, err)
	reg := director_tools.NewRegistry()
	reg.Register(&director_tools.BusinessProfileTool{})
	return NewDirectorConsoleService(db, analytics.NewAnalyticsService(db), ai, reg)
}
