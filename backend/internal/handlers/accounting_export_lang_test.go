package handlers

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/server"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// entriesExportLangRouter wires the entries CSV route with an owner context and
// returns a caller that can set an Accept-Language header.
func entriesExportLangRouter(t *testing.T, owner string) (*gin.Engine, *database.Business) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	setupAccountingExportDB(t)
	business := createAccountingHandlerBusiness(t, owner)

	require.NoError(t, database.GetDB().Create(&database.ManualLedgerEntry{
		BusinessID:  business.ID,
		EntryType:   database.AccountingEntryTypeExpense,
		Category:    "rent",
		Amount:      420050,
		Currency:    "USD",
		OccurredAt:  time.Date(2026, time.June, 25, 15, 30, 0, 0, time.UTC),
		Description: "June rent",
	}).Error)

	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("token_type", "web3")
		c.Set("address", owner)
		c.Set("business_owner_address", owner)
		c.Next()
	})
	handler := NewAccountingHandler(database.GetDBWrapper())
	router.GET("/inside/businesses/:id/accounting/entries/export.csv",
		server.RoleBasedAccessMiddleware("financial:read"),
		handler.ExportEntriesCSV,
	)
	return router, business
}

func entriesExportHeaderLine(t *testing.T, router *gin.Engine, path, acceptLanguage string) string {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	if acceptLanguage != "" {
		req.Header.Set("Accept-Language", acceptLanguage)
	}
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)
	lines := strings.Split(strings.TrimSpace(w.Body.String()), "\n")
	require.GreaterOrEqual(t, len(lines), 1)
	return strings.TrimSpace(lines[0])
}

// TestExportEntriesCSV_ExplicitLangBeatsAcceptLanguage is the L6-12 follow-up
// gate: the operator's chosen UI locale travels as ?lang= and MUST win. Before
// the fix, `?lang=en` normalized to "en" and was then silently overwritten by an
// `Accept-Language: es` browser header, so an English dashboard downloaded a
// Spanish-headed CSV.
func TestExportEntriesCSV_ExplicitLangBeatsAcceptLanguage(t *testing.T) {
	router, business := entriesExportLangRouter(t, "0xExportLangOwner")
	base := fmt.Sprintf(
		"/inside/businesses/%d/accounting/entries/export.csv?start=2026-06-21&end=2026-07-20",
		business.ID,
	)

	// Explicit lang=en with a Spanish browser → English headers.
	assert.Equal(t,
		"occurred_at,type,category,description,reference,status,amount,currency,notes",
		entriesExportHeaderLine(t, router, base+"&lang=en", "es-AR,es;q=0.9,en;q=0.8"),
		"explicit ?lang=en must not be overridden by Accept-Language",
	)

	// Explicit lang=es with an English browser → Spanish headers.
	assert.Equal(t,
		"fecha,tipo,categoría,descripción,referencia,estado,monto,moneda,notas",
		entriesExportHeaderLine(t, router, base+"&lang=es", "en-US,en;q=0.9"),
		"explicit ?lang=es must not be overridden by Accept-Language",
	)

	// es-AR resolves to the es operator tier (voseo has no CSV header deltas).
	assert.Equal(t,
		"fecha,tipo,categoría,descripción,referencia,estado,monto,moneda,notas",
		entriesExportHeaderLine(t, router, base+"&lang=es-AR", "en-US,en;q=0.9"),
	)
}

// TestExportEntriesCSV_AcceptLanguageIsFallbackOnly: with no ?lang= the browser
// header still decides, so direct/legacy links keep localizing.
func TestExportEntriesCSV_AcceptLanguageIsFallbackOnly(t *testing.T) {
	router, business := entriesExportLangRouter(t, "0xExportLangFallback")
	base := fmt.Sprintf(
		"/inside/businesses/%d/accounting/entries/export.csv?start=2026-06-21&end=2026-07-20",
		business.ID,
	)

	assert.Equal(t,
		"fecha,tipo,categoría,descripción,referencia,estado,monto,moneda,notas",
		entriesExportHeaderLine(t, router, base, "es-AR,es;q=0.9"),
		"Accept-Language still applies when no ?lang= is supplied",
	)
	assert.Equal(t,
		"occurred_at,type,category,description,reference,status,amount,currency,notes",
		entriesExportHeaderLine(t, router, base, ""),
		"no ?lang= and no Accept-Language falls back to en",
	)
	// An empty ?lang= is not an explicit choice — header still wins.
	assert.Equal(t,
		"fecha,tipo,categoría,descripción,referencia,estado,monto,moneda,notas",
		entriesExportHeaderLine(t, router, base+"&lang=", "es"),
	)
}
