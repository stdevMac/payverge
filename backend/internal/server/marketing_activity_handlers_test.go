package server

import (
	"encoding/json"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/stdevmac/payverge/backend/internal/database"
)

func setupMarketingActivityTestDB(t *testing.T) *database.Business {
	t.Helper()
	gin.SetMode(gin.TestMode)
	prev := database.GetDB()
	t.Cleanup(func() { database.SetTestDB(prev) })
	gormDB, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := gormDB.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, gormDB.AutoMigrate(&database.Business{}, &database.MarketingActivity{}))
	database.SetTestDB(gormDB)
	biz := &database.Business{
		BusinessId:      "mkt-biz-1",
		Name:            "Marketing Biz",
		OwnerAddress:    "0xowner",
		DefaultCurrency: "USD",
		DefaultLanguage: "en",
		IsActive:        true,
		CreatedAt:       time.Now(),
		UpdatedAt:       time.Now(),
	}
	require.NoError(t, database.GetDB().Create(biz).Error)
	return biz
}

func marketingActivityCtx(w *httptest.ResponseRecorder, method, body, bizID string) *gin.Context {
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: bizID}}
	var reader *strings.Reader
	if body != "" {
		reader = strings.NewReader(body)
	} else {
		reader = strings.NewReader("")
	}
	c.Request = httptest.NewRequest(method, "/businesses/"+bizID+"/marketing/activity", reader)
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set("address", "0xowner")
	return c
}

func TestMarketingActivity_RecordPostDismissAndList(t *testing.T) {
	biz := setupMarketingActivityTestDB(t)

	// POST a "post" action.
	w := httptest.NewRecorder()
	c := marketingActivityCtx(w, http.MethodPost,
		`{"action":"post","suggestion":{"id":"1:featured_dish:m1","play":"featured_dish","title":"Feature","target_name":"Carbonara","image_url":"https://cdn/c.jpg","caption":"Fresh"},"creative_snapshot":{"caption":"Fresh","image_url":"https://cdn/c.jpg","image_source":"menu","template":"editorial","aspect":"4:5","slots":{"dishName":"Carbonara"},"crop":{"x":0.5,"y":0.5,"zoom":1}}}`,
		biz.BusinessId)
	RecordMarketingActivityHandler(c)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	// POST a "dismiss" action for a different suggestion.
	w = httptest.NewRecorder()
	c = marketingActivityCtx(w, http.MethodPost,
		`{"action":"dismiss","suggestion":{"id":"1:offer:o1","play":"offer","title":"Sale","target_name":"","image_url":"","caption":""}}`,
		biz.BusinessId)
	RecordMarketingActivityHandler(c)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	// GET list, newest-first, both rows.
	w = httptest.NewRecorder()
	c, _ = gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: biz.BusinessId}}
	c.Request = httptest.NewRequest(http.MethodGet, "/businesses/"+biz.BusinessId+"/marketing/activity?per_page=9999", nil)
	GetMarketingActivity(c)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var listResp struct {
		Activity []database.MarketingActivity `json:"activity"`
		Total    int64                        `json:"total"`
		Page     int                          `json:"page"`
		PerPage  int                          `json:"per_page"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &listResp))
	require.Equal(t, int64(2), listResp.Total)
	require.Len(t, listResp.Activity, 2)
	require.Equal(t, 50, listResp.PerPage, "per_page clamps to max 50")

	// GET list filtered by status=posted.
	w = httptest.NewRecorder()
	c, _ = gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: biz.BusinessId}}
	c.Request = httptest.NewRequest(http.MethodGet, "/businesses/"+biz.BusinessId+"/marketing/activity?status=posted", nil)
	GetMarketingActivity(c)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var postedResp struct {
		Activity []database.MarketingActivity `json:"activity"`
		Total    int64                        `json:"total"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &postedResp))
	require.Equal(t, int64(1), postedResp.Total)
	require.Len(t, postedResp.Activity, 1)
	require.Equal(t, "posted", postedResp.Activity[0].Status)
}

func TestMarketingActivity_HandledFromCountsLifecycleTransitions(t *testing.T) {
	biz := setupMarketingActivityTestDB(t)
	now := time.Date(2026, 7, 15, 12, 0, 0, 0, time.UTC)
	cutoff := now.Add(-30 * 24 * time.Hour)
	recent := now.Add(-time.Hour)
	old := now.Add(-31 * 24 * time.Hour)
	createdOld := now.Add(-90 * 24 * time.Hour)
	rows := []database.MarketingActivity{
		{BusinessID: biz.ID, SuggestionID: "recent-post", Status: "posted", CreatedAt: createdOld, UpdatedAt: recent, PostedAt: &recent},
		{BusinessID: biz.ID, SuggestionID: "recent-dismiss", Status: "dismissed", CreatedAt: createdOld, UpdatedAt: recent, DismissedAt: &recent},
		{BusinessID: biz.ID, SuggestionID: "effective-transition", Status: "posted", CreatedAt: createdOld, UpdatedAt: recent, PostedAt: &old},
		{BusinessID: biz.ID, SuggestionID: "old-post", Status: "posted", CreatedAt: now, UpdatedAt: old, PostedAt: &old},
		{BusinessID: biz.ID, SuggestionID: "pending", Status: "pending", CreatedAt: now, UpdatedAt: recent},
	}
	require.NoError(t, database.GetDB().Create(&rows).Error)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: biz.BusinessId}}
	c.Request = httptest.NewRequest(http.MethodGet,
		"/businesses/"+biz.BusinessId+"/marketing/activity?handled_from="+cutoff.Format(time.RFC3339)+"&per_page=1", nil)
	GetMarketingActivity(c)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var response struct {
		Activity []database.MarketingActivity `json:"activity"`
		Total    int64                        `json:"total"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
	require.Equal(t, int64(3), response.Total)
	require.Len(t, response.Activity, 1, "pagination must not undercount the total")
}

func TestMarketingActivity_RestoreDismissedButNotPosted(t *testing.T) {
	biz := setupMarketingActivityTestDB(t)

	// Seed a dismissed row.
	w := httptest.NewRecorder()
	c := marketingActivityCtx(w, http.MethodPost,
		`{"action":"dismiss","suggestion":{"id":"1:happy_hour:","play":"happy_hour","title":"HH","target_name":"","image_url":"","caption":""}}`,
		biz.BusinessId)
	RecordMarketingActivityHandler(c)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	// Restore it -> restored:true.
	w = httptest.NewRecorder()
	c = marketingActivityCtx(w, http.MethodPost, `{"action":"restore","suggestion_id":"1:happy_hour:"}`, biz.BusinessId)
	RecordMarketingActivityHandler(c)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	require.Contains(t, w.Body.String(), `"restored":true`)

	// A repeated restore after the never-posted dismissal was deleted is harmless.
	w = httptest.NewRecorder()
	c = marketingActivityCtx(w, http.MethodPost, `{"action":"restore","suggestion_id":"1:happy_hour:"}`, biz.BusinessId)
	RecordMarketingActivityHandler(c)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	require.Contains(t, w.Body.String(), `"restored":false`)

	// Seed a posted row, restore it -> restored:false (permanent).
	w = httptest.NewRecorder()
	c = marketingActivityCtx(w, http.MethodPost,
		`{"action":"post","suggestion":{"id":"1:offer:o5","play":"offer","title":"Sale","target_name":"","image_url":"https://cdn/o5.jpg","caption":"Sale"},"creative_snapshot":{"caption":"Sale","image_url":"https://cdn/o5.jpg","image_source":"offer","template":"bold","aspect":"1:1","slots":{"headline":"Sale"},"crop":{"x":0.5,"y":0.5,"zoom":1}}}`,
		biz.BusinessId)
	RecordMarketingActivityHandler(c)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	w = httptest.NewRecorder()
	c = marketingActivityCtx(w, http.MethodPost, `{"action":"restore","suggestion_id":"1:offer:o5"}`, biz.BusinessId)
	RecordMarketingActivityHandler(c)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	require.Contains(t, w.Body.String(), `"restored":false`)
}

func TestMarketingActivity_InvalidAction(t *testing.T) {
	biz := setupMarketingActivityTestDB(t)
	w := httptest.NewRecorder()
	c := marketingActivityCtx(w, http.MethodPost, `{"action":"nope"}`, biz.BusinessId)
	RecordMarketingActivityHandler(c)
	require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
}

func TestCreativeSnapshotValidation_ValidPostedSnapshotPersistsExactly(t *testing.T) {
	biz := setupMarketingActivityTestDB(t)

	body := `{
		"action":"post",
		"suggestion":{
			"id":"1:featured_dish:m42",
			"play":"featured_dish",
			"title":"Feature",
			"target_name":"Carbonara",
			"image_url":"https://cdn.example.com/legacy.jpg",
			"caption":"Legacy caption"
		},
		"creative_snapshot":{
			"caption":"Tonight's handmade pasta — reservá tu mesa.",
			"image_url":"https://cdn.example.com/marketing/pasta.jpg",
			"image_source":"generated",
			"template":"editorial",
			"aspect":"4:5",
			"font_family":" serif ",
			"slots":{"headline":"Handmade tonight","cta":"Reserve a table","price":"$24"},
			"crop":{"x":0.42,"y":0.31,"zoom":1.35}
		}
	}`
	w := httptest.NewRecorder()
	c := marketingActivityCtx(w, http.MethodPost, body, biz.BusinessId)
	RecordMarketingActivityHandler(c)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var response struct {
		Activity database.MarketingActivity `json:"activity"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
	want := &database.MarketingCreativeSnapshot{
		Caption:     "Tonight's handmade pasta — reservá tu mesa.",
		ImageURL:    "https://cdn.example.com/marketing/pasta.jpg",
		ImageSource: "generated",
		Template:    "editorial",
		Aspect:      "4:5",
		FontFamily:  "Serif",
		Slots: map[string]string{
			"headline": "Handmade tonight",
			"cta":      "Reserve a table",
			"price":    "$24",
		},
		Crop: database.MarketingCrop{X: 0.42, Y: 0.31, Zoom: 1.35},
	}
	require.Equal(t, want, response.Activity.CreativeSnapshot)

	var stored database.MarketingActivity
	require.NoError(t, database.GetDB().First(&stored, response.Activity.ID).Error)
	require.Equal(t, want, stored.CreativeSnapshot)
}

func TestCreativeSnapshotValidation_DefaultsCropKeepsDismissedAndRejectsMissingPostedSnapshot(t *testing.T) {
	biz := setupMarketingActivityTestDB(t)

	postedBody := `{
		"action":"post",
		"suggestion":{"id":"posted","play":"offer","title":"Offer"},
		"creative_snapshot":{
			"caption":"` + strings.Repeat("é", 280) + `",
			"image_url":"https://cdn.example.com/offer.jpg",
			"image_source":"offer",
			"template":"bold",
			"aspect":"9:16",
			"slots":{"headline":"` + strings.Repeat("ñ", 120) + `","cta":"` + strings.Repeat("ç", 80) + `"}
		}
	}`
	w := httptest.NewRecorder()
	c := marketingActivityCtx(w, http.MethodPost, postedBody, biz.BusinessId)
	RecordMarketingActivityHandler(c)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	dismissedBody := `{
		"action":"dismiss",
		"suggestion":{"id":"dismissed","play":"offer","title":"Offer"},
		"creative_snapshot":{
			"caption":"must not persist",
			"image_source":"upload",
			"template":"minimal",
			"aspect":"1:1",
			"slots":{},
			"crop":{"x":0.1,"y":0.2,"zoom":1.2}
		}
	}`
	w = httptest.NewRecorder()
	c = marketingActivityCtx(w, http.MethodPost, dismissedBody, biz.BusinessId)
	RecordMarketingActivityHandler(c)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	legacyBody := `{"action":"post","suggestion":{"id":"legacy","play":"offer","title":"Offer"}}`
	w = httptest.NewRecorder()
	c = marketingActivityCtx(w, http.MethodPost, legacyBody, biz.BusinessId)
	RecordMarketingActivityHandler(c)
	require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
	var apiErr struct {
		Code string `json:"code"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &apiErr))
	require.Equal(t, "invalid_creative_snapshot", apiErr.Code)

	// The one-time localStorage import action is retired: no snapshot-less
	// posted row can be recorded.
	retiredImportBody := `{"action":"import_legacy_post","suggestion":{"id":"legacy-import","play":"offer","title":"Imported offer"}}`
	w = httptest.NewRecorder()
	c = marketingActivityCtx(w, http.MethodPost, retiredImportBody, biz.BusinessId)
	RecordMarketingActivityHandler(c)
	require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())

	var rows []database.MarketingActivity
	require.NoError(t, database.GetDB().Order("suggestion_id ASC").Find(&rows).Error)
	require.Len(t, rows, 2)
	require.Nil(t, rows[0].CreativeSnapshot, "dismissed snapshots are never stored")
	require.NotNil(t, rows[1].CreativeSnapshot)
	require.Equal(t, database.MarketingCrop{X: 0.5, Y: 0.5, Zoom: 1}, rows[1].CreativeSnapshot.Crop)
}

func TestCreativeSnapshotValidation_InvalidSnapshotIsAtomic(t *testing.T) {
	biz := setupMarketingActivityTestDB(t)

	validSnapshot := func(overrides string) string {
		return `{
			"caption":"Fresh pasta",
			"image_url":"https://cdn.example.com/pasta.jpg",
			"image_source":"menu",
			"template":"minimal",
			"aspect":"1:1",
			"slots":{"headline":"Fresh pasta","cta":"Order now"},
			"crop":{"x":0.5,"y":0.5,"zoom":1}` + overrides + `
		}`
	}
	tests := map[string]string{
		"aspect":           validSnapshot(`,"aspect":"16:9"`),
		"template":         validSnapshot(`,"template":"loud"`),
		"source":           validSnapshot(`,"image_source":"filesystem"`),
		"crop x below":     validSnapshot(`,"crop":{"x":-0.01,"y":0.5,"zoom":1}`),
		"crop y above":     validSnapshot(`,"crop":{"x":0.5,"y":1.01,"zoom":1}`),
		"crop zoom below":  validSnapshot(`,"crop":{"x":0.5,"y":0.5,"zoom":0.99}`),
		"crop zoom above":  validSnapshot(`,"crop":{"x":0.5,"y":0.5,"zoom":3.01}`),
		"caption runes":    validSnapshot(`,"caption":"` + strings.Repeat("é", 281) + `"`),
		"headline runes":   validSnapshot(`,"slots":{"headline":"` + strings.Repeat("ñ", 121) + `","cta":"Order"}`),
		"cta runes":        validSnapshot(`,"slots":{"headline":"Fresh","cta":"` + strings.Repeat("ç", 81) + `"}`),
		"font family":      validSnapshot(`,"font_family":"Papyrus"`),
		"font family size": validSnapshot(`,"font_family":"` + strings.Repeat("S", 33) + `"`),
		"kit":              validSnapshot(`,"kit":"vaporwave"`),
		"composition":      validSnapshot(`,"composition":"poster stack"`),
	}

	for name, snapshot := range tests {
		t.Run(name, func(t *testing.T) {
			body := `{"action":"post","suggestion":{"id":"` + name + `","play":"offer","title":"Offer"},"creative_snapshot":` + snapshot + `}`
			w := httptest.NewRecorder()
			c := marketingActivityCtx(w, http.MethodPost, body, biz.BusinessId)
			RecordMarketingActivityHandler(c)
			require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())

			var apiErr struct {
				Code string `json:"code"`
			}
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &apiErr))
			require.Equal(t, "invalid_creative_snapshot", apiErr.Code)

			var count int64
			require.NoError(t, database.GetDB().Model(&database.MarketingActivity{}).Count(&count).Error)
			require.Zero(t, count, "validation must happen before the activity write")
		})
	}
}

func TestCreativeSnapshotValidation_RejectsNonFiniteCropValues(t *testing.T) {
	values := map[string]float64{
		"nan":          math.NaN(),
		"positive inf": math.Inf(1),
		"negative inf": math.Inf(-1),
	}
	for valueName, value := range values {
		for _, field := range []string{"x", "y", "zoom"} {
			t.Run(field+" "+valueName, func(t *testing.T) {
				x, y, zoom := 0.5, 0.5, 1.0
				switch field {
				case "x":
					x = value
				case "y":
					y = value
				case "zoom":
					zoom = value
				}
				got, err := normalizeAndValidateMarketingCreativeSnapshot(&marketingCreativeSnapshotRequest{
					Caption:     "Fresh pasta",
					ImageURL:    "https://cdn.example.com/pasta.jpg",
					ImageSource: "menu",
					Template:    "minimal",
					Aspect:      "1:1",
					Slots:       map[string]string{"headline": "Fresh pasta", "cta": "Order now"},
					Crop:        &marketingCropRequest{X: &x, Y: &y, Zoom: &zoom},
				})
				require.Error(t, err)
				require.Nil(t, got, "non-finite crop values must be rejected before persistence")
			})
		}
	}
}

func TestMarketingActivity_DismissThenRestorePreservesPostedHistory(t *testing.T) {
	biz := setupMarketingActivityTestDB(t)
	postBody := `{
		"action":"post",
		"suggestion":{"id":"preserve-creative","play":"offer","title":"Offer","image_url":"https://cdn.example.com/activity.jpg","caption":"Activity caption"},
		"creative_snapshot":{
			"caption":"Exact approved caption",
			"image_url":"https://cdn.example.com/approved.jpg",
			"image_source":"generated",
			"template":"editorial",
			"aspect":"4:5",
			"slots":{"headline":"Tonight only","cta":"Book now"},
			"crop":{"x":0.42,"y":0.31,"zoom":1.35}
		}
	}`
	w := httptest.NewRecorder()
	c := marketingActivityCtx(w, http.MethodPost, postBody, biz.BusinessId)
	RecordMarketingActivityHandler(c)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var posted struct {
		Activity database.MarketingActivity `json:"activity"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &posted))
	require.NotNil(t, posted.Activity.CreativeSnapshot)
	require.NotNil(t, posted.Activity.PostedAt)
	oldPostedAt := time.Now().UTC().Add(-31 * 24 * time.Hour).Truncate(time.Second)
	require.NoError(t, database.GetDB().Model(&database.MarketingActivity{}).
		Where("id = ?", posted.Activity.ID).Update("posted_at", oldPostedAt).Error)

	w = httptest.NewRecorder()
	c = marketingActivityCtx(w, http.MethodPost,
		`{"action":"dismiss","suggestion":{"id":"preserve-creative","play":"offer","title":"Offer","image_url":"https://cdn.example.com/activity.jpg","caption":"Activity caption"}}`,
		biz.BusinessId)
	RecordMarketingActivityHandler(c)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var dismissed struct {
		Activity database.MarketingActivity `json:"activity"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &dismissed))
	require.Equal(t, "dismissed", dismissed.Activity.Status)
	require.Equal(t, posted.Activity.CreativeSnapshot, dismissed.Activity.CreativeSnapshot)
	require.Equal(t, oldPostedAt, dismissed.Activity.PostedAt.UTC())
	require.NotNil(t, dismissed.Activity.DismissedAt)
	require.Equal(t, "https://cdn.example.com/activity.jpg", dismissed.Activity.ImageURL)
	require.Equal(t, "Activity caption", dismissed.Activity.Caption)

	w = httptest.NewRecorder()
	c = marketingActivityCtx(w, http.MethodPost,
		`{"action":"restore","suggestion_id":"preserve-creative"}`,
		biz.BusinessId)
	RecordMarketingActivityHandler(c)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	require.Contains(t, w.Body.String(), `"restored":true`)

	var stored database.MarketingActivity
	require.NoError(t, database.GetDB().First(&stored, dismissed.Activity.ID).Error)
	require.Equal(t, "posted", stored.Status)
	require.Nil(t, stored.DismissedAt)
	require.Equal(t, posted.Activity.CreativeSnapshot, stored.CreativeSnapshot)
	require.Equal(t, oldPostedAt, stored.PostedAt.UTC())
	require.Equal(t, "https://cdn.example.com/activity.jpg", stored.ImageURL)
	require.Equal(t, "Activity caption", stored.Caption)

	w = httptest.NewRecorder()
	c = marketingActivityCtx(w, http.MethodPost,
		`{"action":"restore","suggestion_id":"preserve-creative"}`,
		biz.BusinessId)
	RecordMarketingActivityHandler(c)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	require.Contains(t, w.Body.String(), `"restored":false`)
}

func settingsCtx(w *httptest.ResponseRecorder, method, body, bizID string) *gin.Context {
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: bizID}}
	c.Request = httptest.NewRequest(method, "/businesses/"+bizID+"/marketing/settings", strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set("address", "0xowner")
	return c
}

func TestMarketingSettings_GetPutValidation(t *testing.T) {
	biz := setupMarketingActivityTestDB(t)

	// GET defaults (enabled true, empty disabled_plays).
	w := httptest.NewRecorder()
	c := settingsCtx(w, http.MethodGet, "", biz.BusinessId)
	GetMarketingSettingsHandler(c)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var got struct {
		Enabled       bool     `json:"enabled"`
		DisabledPlays []string `json:"disabled_plays"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
	require.True(t, got.Enabled)

	// PUT unknown play -> 400.
	w = httptest.NewRecorder()
	c = settingsCtx(w, http.MethodPut, `{"enabled":true,"disabled_plays":["not_a_play"]}`, biz.BusinessId)
	PutMarketingSettingsHandler(c)
	require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())

	// PUT valid -> persists + echoes.
	w = httptest.NewRecorder()
	c = settingsCtx(w, http.MethodPut, `{"enabled":false,"disabled_plays":["offer","happy_hour"]}`, biz.BusinessId)
	PutMarketingSettingsHandler(c)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
	require.False(t, got.Enabled)
	require.ElementsMatch(t, []string{"offer", "happy_hour"}, got.DisabledPlays)

	// GET reflects the persisted values.
	w = httptest.NewRecorder()
	c = settingsCtx(w, http.MethodGet, "", biz.BusinessId)
	GetMarketingSettingsHandler(c)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
	require.False(t, got.Enabled)
	require.ElementsMatch(t, []string{"offer", "happy_hour"}, got.DisabledPlays)
}

type marketingSettingsCreativeResponse struct {
	Enabled         bool                              `json:"enabled"`
	DisabledPlays   []string                          `json:"disabled_plays"`
	CreativeProfile database.MarketingCreativeProfile `json:"creative_profile"`
}

func TestMarketingSettingsCreativeProfile_GetPutGetPreservesAndShapesProfile(t *testing.T) {
	biz := setupMarketingActivityTestDB(t)

	w := httptest.NewRecorder()
	c := settingsCtx(w, http.MethodGet, "", biz.BusinessId)
	GetMarketingSettingsHandler(c)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var got marketingSettingsCreativeResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
	require.Equal(t, database.MarketingCreativeProfile{AvoidPhrases: []string{}}, got.CreativeProfile)

	body := `{
		"enabled": false,
		"disabled_plays": [" offer ", "offer"],
		"creative_profile": {
			"audience": "  neighborhood regulars  ",
			"voice": "  warm and specific  ",
			"visual_mood": " editorial ",
			"cta_style": " direct ",
			"hashtag_behavior": " light ",
			"avoid_phrases": ["  Best in town  ", "best IN TOWN", " guaranteed "],
			"default_language": " es-AR ",
			"default_tone": " punchy "
		}
	}`
	w = httptest.NewRecorder()
	c = settingsCtx(w, http.MethodPut, body, biz.BusinessId)
	PutMarketingSettingsHandler(c)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
	require.False(t, got.Enabled)
	require.Equal(t, []string{"offer"}, got.DisabledPlays)
	require.Equal(t, database.MarketingCreativeProfile{
		Audience:        "neighborhood regulars",
		Voice:           "warm and specific",
		VisualMood:      "editorial",
		CTAStyle:        "direct",
		HashtagBehavior: "light",
		AvoidPhrases:    []string{"Best in town", "guaranteed"},
		DefaultLanguage: "es-AR",
		DefaultTone:     "punchy",
	}, got.CreativeProfile)

	w = httptest.NewRecorder()
	c = settingsCtx(w, http.MethodGet, "", biz.BusinessId)
	GetMarketingSettingsHandler(c)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var roundTrip marketingSettingsCreativeResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &roundTrip))
	require.Equal(t, got, roundTrip)
}

func TestMarketingSettingsCreativeProfile_LegacyAndNullPUTPreserveStoredProfile(t *testing.T) {
	biz := setupMarketingActivityTestDB(t)
	d := database.GetDBWrapper()
	wantProfile := database.MarketingCreativeProfile{
		Audience:        "neighborhood regulars",
		Voice:           "warm and specific",
		VisualMood:      "editorial",
		CTAStyle:        "direct",
		HashtagBehavior: "light",
		AvoidPhrases:    []string{"best in town"},
		DefaultLanguage: "es-AR",
		DefaultTone:     "punchy",
	}

	for name, body := range map[string]string{
		"omitted profile": `{"enabled":false,"disabled_plays":["offer"]}`,
		"null profile":    `{"enabled":false,"disabled_plays":["offer"],"creative_profile":null}`,
	} {
		t.Run(name, func(t *testing.T) {
			_, err := d.UpdateMarketingSettings(biz.ID, database.MarketingSettings{
				Enabled: true, CreativeProfile: wantProfile,
			})
			require.NoError(t, err)

			w := httptest.NewRecorder()
			c := settingsCtx(w, http.MethodPut, body, biz.BusinessId)
			PutMarketingSettingsHandler(c)
			require.Equal(t, http.StatusOK, w.Code, w.Body.String())

			var response marketingSettingsCreativeResponse
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
			require.False(t, response.Enabled)
			require.Equal(t, []string{"offer"}, response.DisabledPlays)
			require.Equal(t, wantProfile, response.CreativeProfile)

			stored, getErr := d.GetMarketingSettings(biz.ID)
			require.NoError(t, getErr)
			require.Equal(t, wantProfile, stored.CreativeProfile)
		})
	}
}

func TestMarketingSettingsCreativeProfile_ExplicitEmptyPUTClearsOverrides(t *testing.T) {
	biz := setupMarketingActivityTestDB(t)
	d := database.GetDBWrapper()
	_, err := d.UpdateMarketingSettings(biz.ID, database.MarketingSettings{
		Enabled: true,
		CreativeProfile: database.MarketingCreativeProfile{
			Audience: "regulars", Voice: "warm", AvoidPhrases: []string{"guaranteed"},
		},
	})
	require.NoError(t, err)

	w := httptest.NewRecorder()
	c := settingsCtx(w, http.MethodPut, `{"enabled":false,"disabled_plays":[],"creative_profile":{}}`, biz.BusinessId)
	PutMarketingSettingsHandler(c)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var response marketingSettingsCreativeResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
	wantEmpty := database.MarketingCreativeProfile{AvoidPhrases: []string{}}
	require.Equal(t, wantEmpty, response.CreativeProfile)

	stored, getErr := d.GetMarketingSettings(biz.ID)
	require.NoError(t, getErr)
	require.Equal(t, wantEmpty, stored.CreativeProfile)
}

func TestMarketingSettingsCreativeProfile_InvalidValuesAreAtomic(t *testing.T) {
	biz := setupMarketingActivityTestDB(t)
	d := database.GetDBWrapper()
	baseline := database.MarketingSettings{
		Enabled:       false,
		DisabledPlays: []string{"offer"},
		CreativeProfile: database.MarketingCreativeProfile{
			Audience: "regulars", VisualMood: "natural", DefaultLanguage: "es-AR", DefaultTone: "warm",
			AvoidPhrases: []string{},
		},
	}
	_, err := d.UpdateMarketingSettings(biz.ID, baseline)
	require.NoError(t, err)

	tests := map[string]string{
		"mood":                `{"visual_mood":"cinematic"}`,
		"cta":                 `{"cta_style":"pushy"}`,
		"hashtags":            `{"hashtag_behavior":"heavy"}`,
		"tone":                `{"default_tone":"formal"}`,
		"language":            `{"default_language":"xx-not-real"}`,
		"audience over limit": `{"audience":"` + strings.Repeat("é", 201) + `"}`,
		"voice over limit":    `{"voice":"` + strings.Repeat("é", 201) + `"}`,
	}
	for name, profile := range tests {
		t.Run(name, func(t *testing.T) {
			_, seedErr := d.UpdateMarketingSettings(biz.ID, baseline)
			require.NoError(t, seedErr)

			body := `{"enabled":true,"disabled_plays":[],"creative_profile":` + profile + `}`
			w := httptest.NewRecorder()
			c := settingsCtx(w, http.MethodPut, body, biz.BusinessId)
			PutMarketingSettingsHandler(c)
			require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
			var apiErr struct {
				Code string `json:"code"`
			}
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &apiErr))
			require.Equal(t, "invalid_marketing_settings", apiErr.Code)

			got, getErr := d.GetMarketingSettings(biz.ID)
			require.NoError(t, getErr)
			require.Equal(t, baseline, got, "invalid PUT must not partially update settings")
		})
	}
}

func TestNormalizeCreativeSnapshotKit(t *testing.T) {
	base := func() *marketingCreativeSnapshotRequest {
		return &marketingCreativeSnapshotRequest{
			Caption:     "hello",
			ImageURL:    "https://cdn.example.com/a.jpg",
			ImageSource: "menu",
			Template:    "editorial",
			Aspect:      "4:5",
			Slots:       map[string]string{"dishName": "Milanesa"},
		}
	}

	t.Run("accepts a known kit and composition", func(t *testing.T) {
		in := base()
		in.Kit = "chalkboard"
		in.Composition = "posterStack"
		in.Treatment = "grain"
		out, err := normalizeAndValidateMarketingCreativeSnapshot(in)
		if err != nil {
			t.Fatalf("expected success, got %v", err)
		}
		if out.Kit != "chalkboard" || out.Composition != "posterStack" || out.Treatment != "grain" {
			t.Fatalf("fields not carried through: %+v", out)
		}
	})

	t.Run("legacy snapshot without a kit still validates", func(t *testing.T) {
		out, err := normalizeAndValidateMarketingCreativeSnapshot(base())
		if err != nil {
			t.Fatalf("expected legacy success, got %v", err)
		}
		if out.Kit != "" {
			t.Fatalf("expected empty kit for legacy, got %q", out.Kit)
		}
	})

	t.Run("rejects an unknown kit", func(t *testing.T) {
		in := base()
		in.Kit = "vaporwave"
		if _, err := normalizeAndValidateMarketingCreativeSnapshot(in); err == nil {
			t.Fatal("expected rejection of unknown kit")
		}
	})

	t.Run("rejects an over-long composition id", func(t *testing.T) {
		in := base()
		in.Composition = strings.Repeat("x", 65)
		if _, err := normalizeAndValidateMarketingCreativeSnapshot(in); err == nil {
			t.Fatal("expected rejection of over-long composition")
		}
	})

	t.Run("rejects a composition with a disallowed character", func(t *testing.T) {
		hazards := map[string]string{
			"space":     "poster stack",
			"path":      "poster/stack",
			"non-ASCII": "pósterStack",
		}
		for name, value := range hazards {
			t.Run(name, func(t *testing.T) {
				in := base()
				in.Composition = value
				if _, err := normalizeAndValidateMarketingCreativeSnapshot(in); err == nil {
					t.Fatalf("expected rejection of composition %q", value)
				}
			})
		}
	})
}

func TestNormalizeCreativeSnapshotMotion(t *testing.T) {
	base := func() *marketingCreativeSnapshotRequest {
		return &marketingCreativeSnapshotRequest{
			Caption:     "hello",
			ImageURL:    "https://cdn.example.com/a.jpg",
			ImageSource: "menu",
			Template:    "editorial",
			Aspect:      "4:5",
			Slots:       map[string]string{"dishName": "Milanesa"},
		}
	}

	t.Run("accepts a video snapshot with a preset", func(t *testing.T) {
		in := base()
		in.MediaKind = "video"
		in.MotionPreset = "pushIn"
		out, err := normalizeAndValidateMarketingCreativeSnapshot(in)
		if err != nil {
			t.Fatalf("expected success, got %v", err)
		}
		if out.MediaKind != "video" || out.MotionPreset != "pushIn" {
			t.Fatalf("fields not carried through: %+v", out)
		}
	})

	t.Run("accepts an explicit image snapshot with no preset", func(t *testing.T) {
		in := base()
		in.MediaKind = "image"
		out, err := normalizeAndValidateMarketingCreativeSnapshot(in)
		if err != nil {
			t.Fatalf("expected success, got %v", err)
		}
		if out.MediaKind != "image" || out.MotionPreset != "" {
			t.Fatalf("fields not carried through: %+v", out)
		}
	})

	t.Run("legacy snapshot without media_kind still validates", func(t *testing.T) {
		out, err := normalizeAndValidateMarketingCreativeSnapshot(base())
		if err != nil {
			t.Fatalf("expected legacy success, got %v", err)
		}
		if out.MediaKind != "" {
			t.Fatalf("expected empty media_kind for legacy, got %q", out.MediaKind)
		}
	})

	t.Run("normalizes case and whitespace on media_kind", func(t *testing.T) {
		in := base()
		in.MediaKind = "  VIDEO "
		in.MotionPreset = "pushIn"
		out, err := normalizeAndValidateMarketingCreativeSnapshot(in)
		if err != nil {
			t.Fatalf("expected success, got %v", err)
		}
		if out.MediaKind != "video" {
			t.Fatalf("expected canonical lowercase, got %q", out.MediaKind)
		}
	})

	t.Run("rejects an unknown media_kind", func(t *testing.T) {
		in := base()
		in.MediaKind = "gif"
		if _, err := normalizeAndValidateMarketingCreativeSnapshot(in); err == nil {
			t.Fatal("expected rejection of unknown media_kind")
		}
	})

	t.Run("rejects a video snapshot with no motion preset", func(t *testing.T) {
		in := base()
		in.MediaKind = "video"
		if _, err := normalizeAndValidateMarketingCreativeSnapshot(in); err == nil {
			t.Fatal("expected rejection of a video with nothing to play")
		}
	})

	t.Run("rejects a motion preset with a disallowed character", func(t *testing.T) {
		hazards := map[string]string{
			"space":     "push in",
			"path":      "push/in",
			"non-ASCII": "púshIn",
		}
		for name, value := range hazards {
			t.Run(name, func(t *testing.T) {
				in := base()
				in.MediaKind = "video"
				in.MotionPreset = value
				if _, err := normalizeAndValidateMarketingCreativeSnapshot(in); err == nil {
					t.Fatalf("expected rejection of motion_preset %q", value)
				}
			})
		}
	})

	t.Run("rejects an over-long motion preset", func(t *testing.T) {
		in := base()
		in.MediaKind = "video"
		in.MotionPreset = strings.Repeat("x", 65)
		if _, err := normalizeAndValidateMarketingCreativeSnapshot(in); err == nil {
			t.Fatal("expected rejection of over-long motion_preset")
		}
	})
}

func TestNormalizeCreativeSnapshotAspect(t *testing.T) {
	base := func() *marketingCreativeSnapshotRequest {
		return &marketingCreativeSnapshotRequest{
			Caption:     "hello",
			ImageURL:    "https://cdn.example.com/a.jpg",
			ImageSource: "menu",
			Template:    "editorial",
			Aspect:      "4:5",
			Slots:       map[string]string{"dishName": "Milanesa"},
		}
	}

	// The six ids in frontend formats/formats.ts FORMAT_ORDER. The three legacy
	// strings keep their exact spelling so no stored row needs migrating.
	accepted := []string{"1:1", "4:5", "9:16", "wide", "strip", "5:7"}
	for _, aspect := range accepted {
		t.Run("accepts "+aspect, func(t *testing.T) {
			in := base()
			in.Aspect = aspect
			out, err := normalizeAndValidateMarketingCreativeSnapshot(in)
			if err != nil {
				t.Fatalf("expected %q to validate, got %v", aspect, err)
			}
			if out.Aspect != aspect {
				t.Fatalf("aspect not carried through: got %q want %q", out.Aspect, aspect)
			}
		})
	}

	rejected := []string{"", "16:9", "5:7 ", "A4", "1:1;drop", "TABLETENT"}
	for _, aspect := range rejected {
		t.Run("rejects "+aspect, func(t *testing.T) {
			in := base()
			in.Aspect = aspect
			if _, err := normalizeAndValidateMarketingCreativeSnapshot(in); err == nil {
				t.Fatalf("expected rejection of aspect %q", aspect)
			}
		})
	}
}

// The AI hero-image generation surface stays at three aspects on purpose: a
// hero photo is generated ONCE at the hero aspect and every other format
// re-crops it through fitCover plus the stored MarketingCrop. Widening this
// would multiply image-credit spend by six for no visual gain and would need
// six new paragraphs in buildMarketingHeroPrompt. This test exists so that
// decision fails loudly if someone "completes" the widening later.
func TestMarketingImageAspectStaysThreeValued(t *testing.T) {
	for _, aspect := range []string{"wide", "strip", "5:7"} {
		t.Run(aspect, func(t *testing.T) {
			// Name is required by the image request normalizer; set it so the
			// rejection under test is the aspect whitelist, not a missing name.
			_, err := normalizeMarketingImageRequest(marketingImageRequest{
				Name:        "Milanesa",
				Play:        "featured_dish",
				AspectRatio: aspect,
			})
			if err == nil {
				t.Fatalf("expected image generation to reject aspect %q", aspect)
			}
		})
	}
}

func TestNormalizeCreativeSnapshotKitFormats(t *testing.T) {
	base := func() *marketingCreativeSnapshotRequest {
		return &marketingCreativeSnapshotRequest{
			Caption:     "hello",
			ImageURL:    "https://cdn.example.com/a.jpg",
			ImageSource: "menu",
			Template:    "editorial",
			Aspect:      "4:5",
			Slots:       map[string]string{"dishName": "Milanesa"},
		}
	}

	t.Run("carries a full kit through", func(t *testing.T) {
		in := base()
		in.KitFormats = []string{"4:5", "9:16", "1:1", "5:7", "wide", "strip"}
		out, err := normalizeAndValidateMarketingCreativeSnapshot(in)
		if err != nil {
			t.Fatalf("expected success, got %v", err)
		}
		if len(out.KitFormats) != 6 {
			t.Fatalf("expected 6 formats, got %v", out.KitFormats)
		}
		if out.KitFormats[0] != "4:5" || out.KitFormats[5] != "strip" {
			t.Fatalf("order not preserved: %v", out.KitFormats)
		}
	})

	// Absence is the legacy signal and must stay distinguishable from an empty
	// list, so the frontend can tell "single-format post" from "campaign with no
	// formats" without consulting a second field.
	t.Run("legacy snapshot without kit_formats validates and stays nil", func(t *testing.T) {
		out, err := normalizeAndValidateMarketingCreativeSnapshot(base())
		if err != nil {
			t.Fatalf("expected legacy success, got %v", err)
		}
		if out.KitFormats != nil {
			t.Fatalf("expected nil kit_formats for a legacy row, got %v", out.KitFormats)
		}
	})

	t.Run("dedupes while preserving first-seen order", func(t *testing.T) {
		in := base()
		in.KitFormats = []string{"9:16", "4:5", "9:16", "4:5"}
		out, err := normalizeAndValidateMarketingCreativeSnapshot(in)
		if err != nil {
			t.Fatalf("expected success, got %v", err)
		}
		if len(out.KitFormats) != 2 || out.KitFormats[0] != "9:16" || out.KitFormats[1] != "4:5" {
			t.Fatalf("expected [9:16 4:5], got %v", out.KitFormats)
		}
	})

	t.Run("rejects an unknown format", func(t *testing.T) {
		in := base()
		in.KitFormats = []string{"4:5", "billboard"}
		if _, err := normalizeAndValidateMarketingCreativeSnapshot(in); err == nil {
			t.Fatal("expected rejection of an unknown kit format")
		}
	})

	// The cap is the registry size. A list longer than the closed set can only be
	// duplicates or garbage, and an unbounded list is an unbounded jsonb write.
	t.Run("rejects more entries than there are formats", func(t *testing.T) {
		in := base()
		in.KitFormats = []string{"1:1", "4:5", "9:16", "wide", "strip", "5:7", "1:1"}
		in.KitFormats = append(in.KitFormats, "4:5")
		if _, err := normalizeAndValidateMarketingCreativeSnapshot(in); err == nil {
			t.Fatal("expected rejection of an over-long kit_formats list")
		}
	})
}

// The concept is one row. Formats derive from it, so the snapshot stores the
// hero aspect plus the list — never one row per format.
func TestCreativeSnapshotRoundTripsKitFormatsThroughJSONB(t *testing.T) {
	snap := &database.MarketingCreativeSnapshot{
		Caption:     "hi",
		ImageURL:    "https://cdn.example.com/a.jpg",
		ImageSource: "menu",
		Template:    "editorial",
		Aspect:      "4:5",
		Slots:       map[string]string{"dishName": "Milanesa"},
		Crop:        database.MarketingCrop{X: 0.5, Y: 0.5, Zoom: 1},
		KitFormats:  []string{"4:5", "9:16", "5:7"},
	}
	encoded, err := snap.Value()
	if err != nil {
		t.Fatalf("Value: %v", err)
	}
	var decoded database.MarketingCreativeSnapshot
	if err := decoded.Scan(encoded); err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if len(decoded.KitFormats) != 3 || decoded.KitFormats[2] != "5:7" {
		t.Fatalf("kit_formats did not round trip: %v", decoded.KitFormats)
	}

	// A row written before Wave 4 has no kit_formats key at all.
	var legacy database.MarketingCreativeSnapshot
	if err := legacy.Scan([]byte(`{"caption":"hi","aspect":"4:5","template":"editorial","image_source":"menu","slots":{},"crop":{"x":0.5,"y":0.5,"zoom":1}}`)); err != nil {
		t.Fatalf("legacy Scan: %v", err)
	}
	if legacy.KitFormats != nil {
		t.Fatalf("expected nil kit_formats on a legacy row, got %v", legacy.KitFormats)
	}
}

func TestNormalizeCreativeSnapshotDestinationID(t *testing.T) {
	base := func() *marketingCreativeSnapshotRequest {
		return &marketingCreativeSnapshotRequest{
			Caption:     "hello",
			ImageURL:    "https://cdn.example.com/a.jpg",
			ImageSource: "menu",
			Template:    "editorial",
			Aspect:      "4:5",
			Slots:       map[string]string{"dishName": "Milanesa"},
		}
	}

	t.Run("carries a known destination id", func(t *testing.T) {
		in := base()
		in.DestinationID = "ig_feed"
		out, err := normalizeAndValidateMarketingCreativeSnapshot(in)
		if err != nil {
			t.Fatalf("expected success, got %v", err)
		}
		if out.DestinationID != "ig_feed" {
			t.Fatalf("destination_id = %q", out.DestinationID)
		}
	})

	t.Run("legacy snapshot without destination_id stays empty", func(t *testing.T) {
		out, err := normalizeAndValidateMarketingCreativeSnapshot(base())
		if err != nil {
			t.Fatalf("expected legacy success, got %v", err)
		}
		if out.DestinationID != "" {
			t.Fatalf("expected empty destination_id, got %q", out.DestinationID)
		}
	})

	t.Run("rejects illegal characters", func(t *testing.T) {
		in := base()
		in.DestinationID = "ig feed!"
		if _, err := normalizeAndValidateMarketingCreativeSnapshot(in); err == nil {
			t.Fatal("expected rejection of illegal destination_id")
		}
	})

	t.Run("accepts future destination ids by charset only", func(t *testing.T) {
		// Destinations are frontend-owned; backend must not invent a whitelist
		// that rejects a newer FE before deploy ordering catches up.
		in := base()
		in.DestinationID = "future_place_v2"
		out, err := normalizeAndValidateMarketingCreativeSnapshot(in)
		if err != nil {
			t.Fatalf("expected charset-only accept, got %v", err)
		}
		if out.DestinationID != "future_place_v2" {
			t.Fatalf("destination_id = %q", out.DestinationID)
		}
	})
}

// S3-Reach: promo_code + print_preset on creative_snapshot (additive jsonb).
func TestNormalizeCreativeSnapshotPromoAndPrintPreset(t *testing.T) {
	base := func() *marketingCreativeSnapshotRequest {
		return &marketingCreativeSnapshotRequest{
			Caption:     "hello",
			ImageURL:    "https://cdn.example.com/a.jpg",
			ImageSource: "menu",
			Template:    "editorial",
			Aspect:      "4:5",
			Slots:       map[string]string{"dishName": "Milanesa"},
		}
	}

	t.Run("carries promo_code and print_preset", func(t *testing.T) {
		in := base()
		in.PromoCode = "PV-HH-A3F2"
		in.PrintPreset = "table_tent_5x7"
		out, err := normalizeAndValidateMarketingCreativeSnapshot(in)
		if err != nil {
			t.Fatalf("expected success, got %v", err)
		}
		if out.PromoCode != "PV-HH-A3F2" {
			t.Fatalf("promo_code = %q", out.PromoCode)
		}
		if out.PrintPreset != "table_tent_5x7" {
			t.Fatalf("print_preset = %q", out.PrintPreset)
		}
	})

	t.Run("legacy snapshot omits both", func(t *testing.T) {
		out, err := normalizeAndValidateMarketingCreativeSnapshot(base())
		if err != nil {
			t.Fatalf("expected legacy success, got %v", err)
		}
		if out.PromoCode != "" || out.PrintPreset != "" {
			t.Fatalf("expected empty promo/print_preset, got %q / %q", out.PromoCode, out.PrintPreset)
		}
	})

	t.Run("rejects illegal promo_code characters", func(t *testing.T) {
		in := base()
		in.PromoCode = "PV HH!"
		if _, err := normalizeAndValidateMarketingCreativeSnapshot(in); err == nil {
			t.Fatal("expected rejection of illegal promo_code")
		}
	})

	t.Run("accepts future print_preset ids by charset only", func(t *testing.T) {
		in := base()
		in.PrintPreset = "future_shop_preset_v2"
		out, err := normalizeAndValidateMarketingCreativeSnapshot(in)
		if err != nil {
			t.Fatalf("expected charset-only accept, got %v", err)
		}
		if out.PrintPreset != "future_shop_preset_v2" {
			t.Fatalf("print_preset = %q", out.PrintPreset)
		}
	})
}

// S3-Loop: freeform posted_channel on creative_snapshot (additive jsonb).
func TestNormalizeCreativeSnapshotPostedChannel(t *testing.T) {
	base := func() *marketingCreativeSnapshotRequest {
		return &marketingCreativeSnapshotRequest{
			Caption:     "hello",
			ImageURL:    "https://cdn.example.com/a.jpg",
			ImageSource: "menu",
			Template:    "editorial",
			Aspect:      "4:5",
			Slots:       map[string]string{"dishName": "Milanesa"},
		}
	}

	t.Run("accepts freeform channel with spaces", func(t *testing.T) {
		in := base()
		in.PostedChannel = "  Instagram Stories  "
		out, err := normalizeAndValidateMarketingCreativeSnapshot(in)
		if err != nil {
			t.Fatalf("expected success, got %v", err)
		}
		if out.PostedChannel != "Instagram Stories" {
			t.Fatalf("posted_channel = %q", out.PostedChannel)
		}
	})

	t.Run("legacy snapshot omits posted_channel", func(t *testing.T) {
		out, err := normalizeAndValidateMarketingCreativeSnapshot(base())
		if err != nil {
			t.Fatalf("expected legacy success, got %v", err)
		}
		if out.PostedChannel != "" {
			t.Fatalf("expected empty posted_channel, got %q", out.PostedChannel)
		}
	})

	t.Run("rejects control characters", func(t *testing.T) {
		in := base()
		in.PostedChannel = "Instagram\nStories"
		if _, err := normalizeAndValidateMarketingCreativeSnapshot(in); err == nil {
			t.Fatal("expected rejection of control characters in posted_channel")
		}
	})

	t.Run("rejects overlong channel", func(t *testing.T) {
		in := base()
		in.PostedChannel = strings.Repeat("a", 41)
		if _, err := normalizeAndValidateMarketingCreativeSnapshot(in); err == nil {
			t.Fatal("expected rejection of overlong posted_channel")
		}
	})
}

func TestRecordMarketingActivity_RootPostedChannelMergesIntoSnapshot(t *testing.T) {
	biz := setupMarketingActivityTestDB(t)
	body := `{
		"action":"post",
		"posted_channel":"WhatsApp group",
		"suggestion":{"id":"s3-loop","play":"offer","title":"Offer"},
		"creative_snapshot":{
			"caption":"cap",
			"image_url":"https://cdn.example.com/o.jpg",
			"image_source":"offer",
			"template":"bold",
			"aspect":"1:1",
			"slots":{"headline":"H"},
			"destination_id":"ig_feed"
		}
	}`
	w := httptest.NewRecorder()
	c := marketingActivityCtx(w, http.MethodPost, body, biz.BusinessId)
	RecordMarketingActivityHandler(c)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var resp struct {
		Activity database.MarketingActivity `json:"activity"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Equal(t, "posted", resp.Activity.Status)
	require.NotNil(t, resp.Activity.CreativeSnapshot)
	require.Equal(t, "WhatsApp group", resp.Activity.CreativeSnapshot.PostedChannel)
	require.Equal(t, "ig_feed", resp.Activity.CreativeSnapshot.DestinationID)
}

func TestCreativeSnapshotRoundTripsDestinationIDThroughJSONB(t *testing.T) {
	snap := &database.MarketingCreativeSnapshot{
		Caption:       "hi",
		ImageURL:      "https://cdn.example.com/a.jpg",
		ImageSource:   "menu",
		Template:      "editorial",
		Aspect:        "4:5",
		Slots:         map[string]string{"dishName": "Milanesa"},
		Crop:          database.MarketingCrop{X: 0.5, Y: 0.5, Zoom: 1},
		DestinationID: "tiktok",
	}
	encoded, err := snap.Value()
	if err != nil {
		t.Fatalf("Value: %v", err)
	}
	var decoded database.MarketingCreativeSnapshot
	if err := decoded.Scan(encoded); err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if decoded.DestinationID != "tiktok" {
		t.Fatalf("destination_id did not round trip: %q", decoded.DestinationID)
	}

	var legacy database.MarketingCreativeSnapshot
	if err := legacy.Scan([]byte(`{"caption":"hi","aspect":"4:5","template":"editorial","image_source":"menu","slots":{},"crop":{"x":0.5,"y":0.5,"zoom":1}}`)); err != nil {
		t.Fatalf("legacy Scan: %v", err)
	}
	if legacy.DestinationID != "" {
		t.Fatalf("expected empty destination_id on a legacy row, got %q", legacy.DestinationID)
	}
}

// A stored snapshot written by the pre-Wave-4 backend must survive a round trip
// through the CURRENT struct unchanged — same bytes out as in, no new keys.
// This is the regression net for "no data migration, no Library row breaks".
func TestLegacySnapshotRoundTripAddsNoKeys(t *testing.T) {
	const legacy = `{"caption":"Milanesa night","image_url":"https://cdn.example.com/a.jpg","image_source":"menu","template":"editorial","aspect":"4:5","slots":{"dishName":"Milanesa"},"crop":{"x":0.5,"y":0.5,"zoom":1}}`

	var decoded database.MarketingCreativeSnapshot
	if err := decoded.Scan([]byte(legacy)); err != nil {
		t.Fatalf("Scan: %v", err)
	}
	encoded, err := decoded.Value()
	if err != nil {
		t.Fatalf("Value: %v", err)
	}
	var reparsed map[string]any
	if err := json.Unmarshal(encoded.([]byte), &reparsed); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	for _, added := range []string{"kit", "composition", "treatment", "font_family", "kit_formats"} {
		if _, present := reparsed[added]; present {
			t.Fatalf("re-encoding a legacy row introduced key %q; omitempty is not holding", added)
		}
	}
}

func validCreativeSnapshotJSON() string {
	return `{"caption":"Ready caption","image_url":"https://cdn.example.com/x.jpg","image_source":"menu","template":"editorial","aspect":"4:5","slots":{"dishName":"Pasta"},"crop":{"x":0.5,"y":0.5,"zoom":1}}`
}

func TestMarketingActivity_HandoffReadyApprovePost(t *testing.T) {
	biz := setupMarketingActivityTestDB(t)
	snap := validCreativeSnapshotJSON()
	sug := `{"id":"1:featured_dish:h1","play":"featured_dish","title":"Feature","target_name":"Pasta","image_url":"https://cdn.example.com/x.jpg","caption":"Ready caption"}`

	// Owner marks ready.
	w := httptest.NewRecorder()
	c := marketingActivityCtx(w, http.MethodPost,
		`{"action":"mark_ready","suggestion":`+sug+`,"creative_snapshot":`+snap+`}`,
		biz.BusinessId)
	RecordMarketingActivityHandler(c)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var readyResp struct {
		Activity database.MarketingActivity `json:"activity"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &readyResp))
	require.Equal(t, "ready", readyResp.Activity.Status)
	require.Nil(t, readyResp.Activity.PostedAt, "ready must not stamp posted_at")
	require.NotNil(t, readyResp.Activity.CreativeSnapshot)

	// Owner approves.
	w = httptest.NewRecorder()
	c = marketingActivityCtx(w, http.MethodPost,
		`{"action":"approve","suggestion":`+sug+`}`,
		biz.BusinessId)
	RecordMarketingActivityHandler(c)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var approvedResp struct {
		Activity database.MarketingActivity `json:"activity"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &approvedResp))
	require.Equal(t, "approved", approvedResp.Activity.Status)
	require.Nil(t, approvedResp.Activity.PostedAt)
	// COALESCE preserves snapshot from ready.
	require.NotNil(t, approvedResp.Activity.CreativeSnapshot)

	// Owner marks posted.
	w = httptest.NewRecorder()
	c = marketingActivityCtx(w, http.MethodPost,
		`{"action":"post","suggestion":`+sug+`,"creative_snapshot":`+snap+`}`,
		biz.BusinessId)
	RecordMarketingActivityHandler(c)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var postedResp struct {
		Activity database.MarketingActivity `json:"activity"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &postedResp))
	require.Equal(t, "posted", postedResp.Activity.Status)
	require.NotNil(t, postedResp.Activity.PostedAt)

	// Unique (business_id, suggestion_id) preserved across transitions.
	var count int64
	require.NoError(t, database.GetDB().Model(&database.MarketingActivity{}).
		Where("business_id = ? AND suggestion_id = ?", biz.ID, "1:featured_dish:h1").
		Count(&count).Error)
	require.Equal(t, int64(1), count)
}

func TestMarketingActivity_StaffCannotApprove(t *testing.T) {
	biz := setupMarketingActivityTestDB(t)
	snap := validCreativeSnapshotJSON()
	sug := `{"id":"1:offer:staff1","play":"offer","title":"Sale","target_name":"Brunch","image_url":"https://cdn.example.com/x.jpg","caption":"Ready caption"}`

	// Seed ready as owner.
	w := httptest.NewRecorder()
	c := marketingActivityCtx(w, http.MethodPost,
		`{"action":"mark_ready","suggestion":`+sug+`,"creative_snapshot":`+snap+`}`,
		biz.BusinessId)
	RecordMarketingActivityHandler(c)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	// Staff tries to approve → 403 owner_required.
	w = httptest.NewRecorder()
	c = marketingActivityCtx(w, http.MethodPost,
		`{"action":"approve","suggestion":`+sug+`}`,
		biz.BusinessId)
	c.Set("staff_id", uint(42))
	RecordMarketingActivityHandler(c)
	require.Equal(t, http.StatusForbidden, w.Code, w.Body.String())
	require.Contains(t, w.Body.String(), "owner_required")
}

func TestMarketingActivity_StaffPostRequiresApproval(t *testing.T) {
	biz := setupMarketingActivityTestDB(t)
	snap := validCreativeSnapshotJSON()
	sug := `{"id":"1:offer:staff2","play":"offer","title":"Sale","target_name":"Brunch","image_url":"https://cdn.example.com/x.jpg","caption":"Ready caption"}`

	// Staff marks ready (allowed).
	w := httptest.NewRecorder()
	c := marketingActivityCtx(w, http.MethodPost,
		`{"action":"mark_ready","suggestion":`+sug+`,"creative_snapshot":`+snap+`}`,
		biz.BusinessId)
	c.Set("staff_id", uint(7))
	RecordMarketingActivityHandler(c)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	// Staff tries to post from ready → 403 approval_required.
	w = httptest.NewRecorder()
	c = marketingActivityCtx(w, http.MethodPost,
		`{"action":"post","suggestion":`+sug+`,"creative_snapshot":`+snap+`}`,
		biz.BusinessId)
	c.Set("staff_id", uint(7))
	RecordMarketingActivityHandler(c)
	require.Equal(t, http.StatusForbidden, w.Code, w.Body.String())
	require.Contains(t, w.Body.String(), "approval_required")

	// Owner approves.
	w = httptest.NewRecorder()
	c = marketingActivityCtx(w, http.MethodPost,
		`{"action":"approve","suggestion":`+sug+`}`,
		biz.BusinessId)
	RecordMarketingActivityHandler(c)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	// Staff can now mark posted.
	w = httptest.NewRecorder()
	c = marketingActivityCtx(w, http.MethodPost,
		`{"action":"post","suggestion":`+sug+`,"creative_snapshot":`+snap+`}`,
		biz.BusinessId)
	c.Set("staff_id", uint(7))
	RecordMarketingActivityHandler(c)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	require.Contains(t, w.Body.String(), `"status":"posted"`)
}

func TestMarketingActivity_IllegalApproveFromDraft(t *testing.T) {
	biz := setupMarketingActivityTestDB(t)
	sug := `{"id":"1:offer:nodraft","play":"offer","title":"Sale"}`
	w := httptest.NewRecorder()
	c := marketingActivityCtx(w, http.MethodPost,
		`{"action":"approve","suggestion":`+sug+`}`,
		biz.BusinessId)
	RecordMarketingActivityHandler(c)
	require.Equal(t, http.StatusConflict, w.Code, w.Body.String())
	require.Contains(t, w.Body.String(), "invalid_handoff_transition")
}
