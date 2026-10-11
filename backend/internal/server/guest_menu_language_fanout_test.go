package server

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/locales"
	"github.com/stdevmac/payverge/backend/internal/services"
)

// TestGuestMenuLanguageFanoutRejectsUnknownCodes proves an anonymous caller
// cannot mint a paid translation backfill by inventing ?language= values.
// Only shipped guest locales (a bounded registry set) may schedule one.
func TestGuestMenuLanguageFanoutRejectsUnknownCodes(t *testing.T) {
	gin.SetMode(gin.TestMode)
	business, table := seedPublicTableMenuParityBusiness(t)

	var menuBackfills atomic.Int32
	var promoBackfills atomic.Int32
	prevMenu := scheduleGuestMenuTranslationBackfill
	prevPromo := scheduleGuestPromotionTranslationBackfill
	scheduleGuestMenuTranslationBackfill = func(uint, string) {
		menuBackfills.Add(1)
	}
	scheduleGuestPromotionTranslationBackfill = func(uint, string, string, []database.Offer, []database.Bundle) {
		promoBackfills.Add(1)
	}
	t.Cleanup(func() {
		scheduleGuestMenuTranslationBackfill = prevMenu
		scheduleGuestPromotionTranslationBackfill = prevPromo
	})

	r := gin.New()
	r.GET("/business/:customUrl/menu", GetMenuByBusinessCustomUrl)
	r.GET("/guest/table/:code/menu", GetMenuByTableCode)

	values := []string{"../etc", "EN-xx"}
	for n := 1; n <= 5; n++ {
		values = append(values, fmt.Sprintf("zz-random-%d", n))
	}

	var unknownBody []byte
	for _, v := range values {
		q := url.Values{}
		q.Set("language", v)
		paths := []string{
			"/business/" + business.CustomURL + "/menu?" + q.Encode(),
			"/guest/table/" + table.TableCode + "/menu?" + q.Encode(),
		}
		for _, path := range paths {
			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, path, nil)
			r.ServeHTTP(w, req)
			require.Equalf(t, http.StatusOK, w.Code, "%s: %s", path, w.Body.String())
			if unknownBody == nil && v == "zz-random-1" && strings.HasPrefix(path, "/business/") {
				unknownBody = append([]byte(nil), w.Body.Bytes()...)
			}
		}
	}
	assert.Equal(t, int32(0), menuBackfills.Load(), "unknown languages must not schedule a menu backfill")
	assert.Equal(t, int32(0), promoBackfills.Load(), "unknown languages must not schedule a promotion backfill")

	defaultRec := httptest.NewRecorder()
	defaultReq := httptest.NewRequest(http.MethodGet, "/business/"+business.CustomURL+"/menu", nil)
	r.ServeHTTP(defaultRec, defaultReq)
	require.Equal(t, http.StatusOK, defaultRec.Code, defaultRec.Body.String())

	unknownCategories := decodeGuestMenuCategories(t, unknownBody)
	defaultCategories := decodeGuestMenuCategories(t, defaultRec.Body.Bytes())
	require.NotEmpty(t, unknownCategories)
	require.NotEmpty(t, unknownCategories[0].Items)
	assert.Equal(t, "Harvest Bowl", unknownCategories[0].Items[0].Name)
	assert.Equal(t, defaultCategories, unknownCategories)
}

func TestResolveGuestMenuLanguage(t *testing.T) {
	gin.SetMode(gin.TestMode)
	business, _ := seedPublicTableMenuParityBusiness(t)
	services.ResetPricingCache()

	code, defaultLanguage, allowBackfill := resolveGuestMenuLanguage(business, "zz-random-1")
	assert.Equal(t, "en", code)
	assert.Equal(t, "en", defaultLanguage)
	assert.False(t, allowBackfill)

	code, defaultLanguage, allowBackfill = resolveGuestMenuLanguage(business, "en")
	assert.Equal(t, "en", code)
	assert.Equal(t, "en", defaultLanguage)
	assert.False(t, allowBackfill)

	code, defaultLanguage, allowBackfill = resolveGuestMenuLanguage(business, "es")
	assert.Equal(t, "es", code)
	assert.Equal(t, "en", defaultLanguage)
	assert.True(t, allowBackfill)

	other := ""
	for _, locale := range locales.GuestLocales() {
		if locale.Canonical != "en" && locale.Canonical != "es" {
			other = locale.Canonical
			break
		}
	}
	require.NotEmpty(t, other, "registry must contain a guest locale other than en and es")

	// The guest picker offers every storefront locale, so a shipped locale the
	// venue has not configured must still be able to backfill its menu.
	code, defaultLanguage, allowBackfill = resolveGuestMenuLanguage(business, other)
	assert.Equal(t, other, code)
	assert.Equal(t, "en", defaultLanguage)
	assert.True(t, allowBackfill)
}

// TestGuestMenuLanguageFanoutSchedulesShippedLocaleBackfill proves a shipped
// guest locale outside the venue's configured languages still schedules the
// menu backfill, while an invented code alongside it does not.
func TestGuestMenuLanguageFanoutSchedulesShippedLocaleBackfill(t *testing.T) {
	gin.SetMode(gin.TestMode)
	business, _ := seedPublicTableMenuParityBusiness(t)
	services.ResetPricingCache()

	other := ""
	for _, locale := range locales.GuestLocales() {
		if locale.Canonical != "en" && locale.Canonical != "es" {
			other = locale.Canonical
			break
		}
	}
	require.NotEmpty(t, other)

	var scheduled []string
	prevMenu := scheduleGuestMenuTranslationBackfill
	prevPromo := scheduleGuestPromotionTranslationBackfill
	scheduleGuestMenuTranslationBackfill = func(_ uint, lang string) {
		scheduled = append(scheduled, lang)
	}
	scheduleGuestPromotionTranslationBackfill = func(uint, string, string, []database.Offer, []database.Bundle) {}
	t.Cleanup(func() {
		scheduleGuestMenuTranslationBackfill = prevMenu
		scheduleGuestPromotionTranslationBackfill = prevPromo
	})

	r := gin.New()
	r.GET("/business/:customUrl/menu", GetMenuByBusinessCustomUrl)
	for _, lang := range []string{other, "zz-random-9"} {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/business/"+business.CustomURL+"/menu?language="+url.QueryEscape(lang), nil))
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	}
	assert.Equal(t, []string{other}, scheduled, "only the shipped locale may schedule a backfill")
}
