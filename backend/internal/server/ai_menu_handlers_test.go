package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stdevmac/payverge/backend/internal/config"
	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/services"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// setupAIMenuImportTestDB migrates the tables the import/wizard rails touch
// (Business + Menu + wizard sessions + image credits), used by the M0a/M0b/M0c
// and L1 import-handler tests.
func setupAIMenuImportTestDB(t *testing.T) {
	t.Helper()

	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name())
	gormDB, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)

	sqlDB, err := gormDB.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)

	database.SetTestDB(gormDB)
	require.NoError(t, gormDB.AutoMigrate(
		&database.Business{},
		&database.Menu{},
		&database.MenuWizardSession{},
		&database.AIImageUsage{},
	))
}

func setupAIMenuHandlerTestDB(t *testing.T) {
	t.Helper()
	// Extraction handlers answer 503 ai_not_configured without an LLM
	// provider; these tests exercise the provider-configured path.
	config.SetAIProviderConfiguredForTesting(t, true)

	gormDB, err := gorm.Open(sqlite.Open("file::memory:"), &gorm.Config{})
	require.NoError(t, err)

	sqlDB, err := gormDB.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)

	database.SetTestDB(gormDB)
	require.NoError(t, gormDB.AutoMigrate(
		&database.Business{},
		&database.MenuExtractionJob{},
		&database.MenuExtractionImage{},
	))
}

func createAIMenuBusiness(t *testing.T, slug string) *database.Business {
	t.Helper()
	business := &database.Business{
		BusinessId:      slug,
		Name:            "AI Menu Slug",
		OwnerAddress:    "0xowner",
		SettlementAddr:  "0x1111111111111111111111111111111111111111",
		TippingAddr:     "0x2222222222222222222222222222222222222222",
		DefaultCurrency: "USD",
		DefaultLanguage: "en",
		IsActive:        true,
		CreatedAt:       time.Now(),
		UpdatedAt:       time.Now(),
	}
	require.NoError(t, database.GetDB().Create(business).Error)
	return business
}

func createAIMenuExtractionJob(t *testing.T, businessID uint) *database.MenuExtractionJob {
	t.Helper()
	job := &database.MenuExtractionJob{
		BusinessID: businessID,
		Status:     database.ExtractionStatusPending,
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
	}
	require.NoError(t, database.GetDB().Create(job).Error)
	return job
}

func newAIMenuUploadContext(t *testing.T, business *database.Business, job *database.MenuExtractionJob, req *http.Request) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{
		{Key: "id", Value: business.BusinessId},
		{Key: "jobId", Value: fmt.Sprintf("%d", job.ID)},
	}
	c.Request = req
	return c, w
}

func TestStartMenuExtraction_AcceptsBusinessSlug(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupAIMenuHandlerTestDB(t)

	business := createAIMenuBusiness(t, "ai-menu-slug")

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: business.BusinessId}}
	c.Request = httptest.NewRequest(http.MethodPost, "/businesses/ai-menu-slug/ai/extract-menu/start", nil)
	c.Set("address", "0xowner")

	StartMenuExtraction(c)

	require.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), `"job_id":`)
}

func TestUploadMenuPage_AllowsJPEGAndGeneratesServerFilename(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupAIMenuHandlerTestDB(t)

	var uploaded struct {
		contentType string
		folder      string
		name        string
		data        []byte
	}
	prevUp := uploadExtractionObject
	prevDel := deleteExtractionObject
	uploadExtractionObject = func(data []byte, name, folderPath, contentType string) (string, error) {
		uploaded.contentType = contentType
		uploaded.folder = folderPath
		uploaded.name = name
		uploaded.data = append([]byte(nil), data...)
		return folderPath + "/" + name, nil
	}
	deleteExtractionObject = func(string) error { return nil }
	t.Cleanup(func() {
		uploadExtractionObject = prevUp
		deleteExtractionObject = prevDel
	})

	business := createAIMenuBusiness(t, "ai-menu-upload-ok")
	job := createAIMenuExtractionJob(t, business.ID)
	body := minimalJPEG(4096)
	req := newUploadContractRequest(t, map[string]string{"page_order": "2"}, "owner-menu-secret.jpg", "image/jpeg", body)
	c, w := newAIMenuUploadContext(t, business, job, req)

	UploadMenuPage(c)

	require.Equal(t, http.StatusOK, w.Code)

	var image database.MenuExtractionImage
	require.NoError(t, database.GetDB().Where("job_id = ?", job.ID).First(&image).Error)
	assert.Equal(t, 2, image.PageOrder)
	assert.Equal(t, "image/jpeg", image.MIMEType)
	assert.NotEmpty(t, image.StorageKey)
	assert.Contains(t, image.StorageKey, fmt.Sprintf("ai/menu-extraction/%d/%d/", business.ID, job.ID))
	assert.NotContains(t, image.StorageKey, "owner-menu-secret")
	assert.Equal(t, "image/jpeg", uploaded.contentType)
	assert.Equal(t, body, uploaded.data)
	assert.Equal(t, "", image.FilePath, "new rows must not depend on local container paths")
}

func TestUploadMenuPage_PreservesVerifiedMIMEForPNGJPEGWebP(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name        string
		filename    string
		contentType string
		body        []byte
	}{
		{"png", "page.png", "image/png", minimalPNG(4096)},
		{"jpeg", "page.jpg", "image/jpeg", minimalJPEG(4096)},
		{"webp", "page.webp", "image/webp", minimalWebP(4096)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			setupAIMenuHandlerTestDB(t)

			var gotMIME string
			var gotData []byte
			prevUp := uploadExtractionObject
			prevDel := deleteExtractionObject
			uploadExtractionObject = func(data []byte, name, folderPath, contentType string) (string, error) {
				gotMIME = contentType
				gotData = append([]byte(nil), data...)
				return folderPath + "/" + name, nil
			}
			deleteExtractionObject = func(string) error { return nil }
			t.Cleanup(func() {
				uploadExtractionObject = prevUp
				deleteExtractionObject = prevDel
			})

			business := createAIMenuBusiness(t, "ai-menu-mime-"+tt.name)
			job := createAIMenuExtractionJob(t, business.ID)
			req := newUploadContractRequest(t, map[string]string{"page_order": "1"}, tt.filename, tt.contentType, tt.body)
			c, w := newAIMenuUploadContext(t, business, job, req)

			UploadMenuPage(c)

			require.Equal(t, http.StatusOK, w.Code, "body=%s", w.Body.String())
			assert.Equal(t, tt.contentType, gotMIME)
			assert.Equal(t, tt.body, gotData)

			var image database.MenuExtractionImage
			require.NoError(t, database.GetDB().Where("job_id = ?", job.ID).First(&image).Error)
			assert.Equal(t, tt.contentType, image.MIMEType)
			assert.NotEmpty(t, image.StorageKey)
		})
	}
}

func TestProcessMenuExtraction_DoubleSubmitIdempotent(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupAIMenuHandlerTestDB(t)

	var enqueued []uint
	// Install a worker so Enqueue succeeds without running extraction.
	w := services.NewMenuExtractionWorker(nil, nil, nil)
	services.SetMenuExtractionWorker(w)
	t.Cleanup(func() { services.SetMenuExtractionWorker(nil) })
	// Intercept by using a real worker queue; we only assert handler response.

	business := createAIMenuBusiness(t, "ai-menu-double-submit")
	job := createAIMenuExtractionJob(t, business.ID)
	job.Status = database.ExtractionStatusProcessing
	require.NoError(t, database.GetDB().Save(job).Error)

	// First process while already processing.
	wRec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(wRec)
	c.Params = gin.Params{
		{Key: "id", Value: business.BusinessId},
		{Key: "jobId", Value: fmt.Sprintf("%d", job.ID)},
	}
	c.Request = httptest.NewRequest(http.MethodPost, "/", nil)
	c.Set("address", "0xowner")
	ProcessMenuExtraction(c)

	require.Equal(t, http.StatusOK, wRec.Code, wRec.Body.String())
	assert.Contains(t, wRec.Body.String(), `"already_running":true`)
	assert.Contains(t, wRec.Body.String(), `"status":"processing"`)
	_ = enqueued
}

func TestProcessMenuExtraction_CompletedReturnsWithoutEnqueue(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupAIMenuHandlerTestDB(t)
	// No worker installed — completed path must not require one.
	services.SetMenuExtractionWorker(nil)

	business := createAIMenuBusiness(t, "ai-menu-completed")
	job := createAIMenuExtractionJob(t, business.ID)
	job.Status = database.ExtractionStatusCompleted
	require.NoError(t, database.GetDB().Save(job).Error)

	wRec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(wRec)
	c.Params = gin.Params{
		{Key: "id", Value: business.BusinessId},
		{Key: "jobId", Value: fmt.Sprintf("%d", job.ID)},
	}
	c.Request = httptest.NewRequest(http.MethodPost, "/", nil)
	c.Set("address", "0xowner")
	ProcessMenuExtraction(c)

	require.Equal(t, http.StatusOK, wRec.Code, wRec.Body.String())
	assert.Contains(t, wRec.Body.String(), `"status":"completed"`)
}

func TestUploadMenuPage_RejectsUnsafeContent(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name        string
		filename    string
		contentType string
		body        []byte
	}{
		{
			name:        "html",
			filename:    "page.jpg",
			contentType: "text/html",
			body:        []byte("<!doctype html><html><script>alert(1)</script></html>"),
		},
		{
			name:        "svg",
			filename:    "page.svg",
			contentType: "image/svg+xml",
			body:        []byte(`<svg xmlns="http://www.w3.org/2000/svg"><script>alert(1)</script></svg>`),
		},
		{
			name:        "mismatched_extension",
			filename:    "page.html",
			contentType: "image/jpeg",
			body:        minimalJPEG(4096),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			setupAIMenuHandlerTestDB(t)
			business := createAIMenuBusiness(t, "ai-menu-upload-"+tt.name)
			job := createAIMenuExtractionJob(t, business.ID)
			req := newUploadContractRequest(t, map[string]string{"page_order": "1"}, tt.filename, tt.contentType, tt.body)
			c, w := newAIMenuUploadContext(t, business, job, req)

			UploadMenuPage(c)

			assert.Equal(t, http.StatusUnsupportedMediaType, w.Code)
		})
	}
}

func TestImportExtractedMenu_SanitizesAndIsAtomic(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupAIMenuImportTestDB(t)

	business := createBusinessHandlerTestBusiness(t, "0xOwnerImp", "import-sanitize")
	// seed an existing menu so we exercise the update (not create) branch
	require.NoError(t, database.CreateMenu(&database.Menu{BusinessID: business.ID, IsActive: true, Version: 1},
		[]database.MenuCategory{{Name: "Existing"}}))

	// Near-canonical IDs (peanuts/tree_nuts/sulfites) must rewrite, not vanish;
	// only truly unknown IDs drop; response must report drop counts.
	body := `{"categories":[{"name":"New","items":[
		{"name":"good","price":9.5,"allergens":["peanuts","unicorn_dust","gluten","tree_nuts","sulfites","peanut"]},
		{"name":"free","price":0}]}]}`
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Set("address", "0xOwnerImp")
	c.Params = gin.Params{{Key: "id", Value: fmt.Sprint(business.ID)}}
	c.Request = httptest.NewRequest(http.MethodPost, "/import", strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	ImportExtractedMenu(c)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, true, resp["requires_confirmation"])
	require.Contains(t, resp, "sanitization")
	require.Contains(t, resp, "sanitized_categories")

	_, cats, _ := database.GetMenuByBusinessID(business.ID)
	require.Len(t, cats, 1, "review response must not persist before confirmation")

	confirmedBody := strings.Replace(body, `{"categories":`, `{"confirm_sanitization":true,"categories":`, 1)
	w = httptest.NewRecorder()
	c, _ = gin.CreateTestContext(w)
	c.Set("address", "0xOwnerImp")
	c.Params = gin.Params{{Key: "id", Value: fmt.Sprint(business.ID)}}
	c.Request = httptest.NewRequest(http.MethodPost, "/import", strings.NewReader(confirmedBody))
	c.Request.Header.Set("Content-Type", "application/json")
	ImportExtractedMenu(c)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, false, resp["requires_confirmation"])
	report, ok := resp["sanitization"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, float64(1), report["dropped_allergens"], "only unicorn_dust drops")
	assert.Equal(t, float64(0), report["dropped_dietary_tags"])
	assert.Equal(t, float64(1), report["dropped_items"], "free item dropped")
	require.NotEmpty(t, report["retained"])
	require.NotEmpty(t, report["dropped"])
	require.Contains(t, resp, "version")

	_, cats, _ = database.GetMenuByBusinessID(business.ID)
	require.Len(t, cats, 2)          // Existing + New (merge, no 500)
	require.Len(t, cats[1].Items, 1) // "free" dropped by sanitizer
	assert.Equal(t, []string{"peanut", "gluten", "treenuts", "so2"}, cats[1].Items[0].Allergens)
}

func TestImportWizardMenu_ExistingMenu_NoDuplicate(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupAIMenuImportTestDB(t)

	business := createBusinessHandlerTestBusiness(t, "0xOwnerWiz", "wizard-existing")
	require.NoError(t, database.CreateMenu(&database.Menu{BusinessID: business.ID, IsActive: true, Version: 1},
		[]database.MenuCategory{{Name: "Existing"}}))

	// Seed a wizard session whose GeneratedMenu the handler reads. StartWizardSession
	// / GenerateMenuFromWizard drive the LLM, so we persist a deterministic generated
	// menu directly — the handler only reads session.GeneratedMenu off the row.
	generated := services.GeneratedMenu{
		Currency: "USD",
		Categories: []database.MenuCategory{
			{Name: "Generated", Items: []database.MenuItem{{Name: "Soup", Price: 7.5}}},
		},
	}
	generatedJSON, err := json.Marshal(generated)
	require.NoError(t, err)
	wizSession := &database.MenuWizardSession{
		BusinessID:    business.ID,
		Status:        database.WizardStatusCompleted,
		GeneratedMenu: string(generatedJSON),
	}
	require.NoError(t, database.CreateWizardSession(wizSession))

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Set("address", "0xOwnerWiz")
	c.Params = gin.Params{
		{Key: "id", Value: fmt.Sprint(business.ID)},
		{Key: "sessionId", Value: fmt.Sprint(wizSession.ID)},
	}
	c.Request = httptest.NewRequest(http.MethodPost, "/wizard/import", nil)
	ImportWizardMenu(c)

	assert.Equal(t, http.StatusOK, w.Code)

	// Exactly one active menu row for the business — the wizard import must MERGE
	// into the existing menu, not create a duplicate active row (M0a regression).
	var activeMenus int64
	require.NoError(t, database.GetDB().Model(&database.Menu{}).
		Where("business_id = ? AND is_active = ?", business.ID, true).
		Count(&activeMenus).Error)
	assert.Equal(t, int64(1), activeMenus)

	_, cats, _ := database.GetMenuByBusinessID(business.ID)
	assert.Len(t, cats, 2) // Existing + Generated
}

// TestGenerateMenuImage_RejectsSuspendedBusiness asserts that the
// generate-menu-image route lives behind RequireOperationalBusiness(), so a
// suspended business gets HTTP 403 business_suspended before the fair-use
// reservation or the handler run.
func TestGenerateMenuImage_RejectsSuspendedBusiness(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupAIMenuImportTestDB(t)

	biz := &database.Business{
		BusinessId:      "generate-image-suspended-biz",
		Name:            "Suspended Biz",
		OwnerAddress:    "0xsuspendedowner",
		SettlementAddr:  "0x1111111111111111111111111111111111111111",
		TippingAddr:     "0x2222222222222222222222222222222222222222",
		DefaultCurrency: "USD",
		DefaultLanguage: "en",
		IsActive:        true,
	}
	require.NoError(t, database.GetDB().Create(biz).Error)
	require.NoError(t, database.GetDB().Model(biz).UpdateColumn("is_active", false).Error)

	// Mirror the production route group: RequireOperationalBusiness at the
	// group level, then the handler, exactly as registered in main.go.
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("token_type", "web3")
		c.Set("address", "0xsuspendedowner")
		c.Next()
	})
	group := router.Group("/businesses/:id", RequireOperationalBusiness())
	group.POST("/generate-menu-image", RoleBasedAccessMiddleware("menu:write"), GenerateMenuImage)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost,
		fmt.Sprintf("/businesses/%d/generate-menu-image", biz.ID),
		strings.NewReader(`{"name":"Burger"}`))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusForbidden, w.Code)
	assert.Contains(t, w.Body.String(), `"code":"business_suspended"`)
}

// TestStartMenuExtraction_DelegatedOperatorNotBlockedByOwnership pins the
// delegated menu-operator policy: handlers no longer reject non-owner addresses
// at the ownership layer. Production RBAC (menu:write + operational business + business access)
// is enforced by middleware; a staff/manager with those grants may start jobs.
func TestStartMenuExtraction_DelegatedOperatorNotBlockedByOwnership(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupAIMenuHandlerTestDB(t)

	business := createAIMenuBusiness(t, "ai-menu-delegated")
	// Caller address is intentionally not the owner.
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Set("address", "0xmanager-not-owner")
	c.Params = gin.Params{{Key: "id", Value: business.BusinessId}}
	c.Request = httptest.NewRequest(http.MethodPost, "/extract-menu/start", nil)

	StartMenuExtraction(c)

	require.Equal(t, http.StatusOK, w.Code, "body=%s", w.Body.String())
	assert.Contains(t, w.Body.String(), `"job_id":`)
	assert.NotContains(t, w.Body.String(), "don't own")
}

func TestUploadMenuPage_RejectsMultipartBodyOverCapBeforeParsing(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupAIMenuHandlerTestDB(t)

	business := createAIMenuBusiness(t, "ai-menu-upload-too-large")
	job := createAIMenuExtractionJob(t, business.ID)
	req := newUploadRequestWithExtraFields(t, map[string]string{"page_order": "1"}, "page.jpg", "image/jpeg", minimalJPEG(4096), map[string][]byte{
		"padding": make([]byte, maxMenuExtractionUploadBytes+multipartOverheadAllowanceBytes+1),
	})
	c, w := newAIMenuUploadContext(t, business, job, req)

	UploadMenuPage(c)

	assert.Equal(t, http.StatusRequestEntityTooLarge, w.Code)
}
