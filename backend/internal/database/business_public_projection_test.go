package database

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// publicBizSQLRecorder captures emitted SQL so the column projection of the
// public storefront / QR table loaders can be asserted at the query layer
// (OF-04). Mirrors reservationReadSQLRecorder's column-capture helpers.
type publicBizSQLRecorder struct {
	logger.Interface
	statements []string
}

func (r *publicBizSQLRecorder) Trace(ctx context.Context, begin time.Time, fc func() (string, int64), err error) {
	sql, _ := fc()
	r.statements = append(r.statements, sql)
}

// statementTouchesBusinesses reports whether a SELECT reads the businesses
// table either directly (FROM businesses, the custom-URL path) or via a JOIN
// (the QR/table path reads FROM tables LEFT JOIN businesses).
func (r *publicBizSQLRecorder) statementTouchesBusinesses(statement string) bool {
	normalized := strings.ToLower(strings.TrimSpace(statement))
	if !strings.HasPrefix(normalized, "select") {
		return false
	}
	return strings.Contains(normalized, "`businesses`") ||
		strings.Contains(normalized, `"businesses"`) ||
		strings.Contains(normalized, " businesses")
}

// businessSelectStarCount counts statements that do an unprojected SELECT *
// against businesses, whether the table is in FROM or a JOIN.
func (r *publicBizSQLRecorder) businessSelectStarCount() int {
	count := 0
	for _, statement := range r.statements {
		normalized := strings.ToLower(strings.TrimSpace(statement))
		if !strings.HasPrefix(normalized, "select *") {
			continue
		}
		if r.statementTouchesBusinesses(statement) {
			count++
		}
	}
	return count
}

// businessSelectMentions reports whether any captured statement that touches the
// businesses table names the column. Works for both the standalone
// custom-URL First() ("businesses"."<col>") and the joined QR table read
// ("Business"."<col>").
func (r *publicBizSQLRecorder) businessSelectMentions(column string) bool {
	col := strings.ToLower(column)
	needleBacktick := "`" + col + "`"
	needleDouble := `"` + col + `"`
	for _, statement := range r.statements {
		if !r.statementTouchesBusinesses(statement) {
			continue
		}
		normalized := strings.ToLower(statement)
		if strings.Contains(normalized, needleBacktick) ||
			strings.Contains(normalized, needleDouble) {
			return true
		}
	}
	return false
}

func (r *publicBizSQLRecorder) reset() {
	r.statements = nil
}

func setupPublicBizProjectionDB(t testing.TB, rec *publicBizSQLRecorder) *gorm.DB {
	t.Helper()

	dsnName := strings.NewReplacer("/", "_", " ", "_").Replace(t.Name())
	dsn := fmt.Sprintf("file:%s-%d?mode=memory&cache=shared", dsnName, time.Now().UnixNano())
	gormDB, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{Logger: rec})
	require.NoError(t, err, "open in-memory database")

	sqlDB, err := gormDB.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = sqlDB.Close() })

	db = gormDB
	require.NoError(t, db.AutoMigrate(
		&Business{},
		&Table{},
	))
	return gormDB
}

// seedPublicFatBusiness creates a Business whose private/sensitive columns
// (Stripe IDs, onboarding blob) are populated alongside the full set of PUBLIC
// columns the storefront + QR guest responses actually render.
func seedPublicFatBusiness(t testing.TB, customURL, tableCode string) (*Business, *Table) {
	t.Helper()

	business := &Business{
		BusinessId:           fmt.Sprintf("pub-proj-%d", time.Now().UnixNano()),
		Name:                 "Public Bistro",
		OwnerAddress:         fmt.Sprintf("0xpubproj%x", time.Now().UnixNano()),
		Logo:                 "https://cdn.example.com/logo.png",
		CustomURL:            customURL,
		Description:          "A lovely public-facing description",
		Phone:                "+1-555-0100",
		Website:              "https://bistro.example.com",
		SocialMedia:          `{"instagram":"@bistro"}`,
		BannerImages:         `["https://cdn.example.com/banner.png"]`,
		WelcomeMessage:       "Welcome to our bistro!",
		AboutStory:           "Founded in 2020...",
		ShowWelcomeMessage:   true,
		ShowAboutStory:       true,
		ShowGallery:          true,
		ShowOperatingHours:   true,
		ShowSpecialFeatures:  true,
		ShowReviews:          true,
		GoogleReviewsEnabled: true,
		GooglePlaceID:        "place-123",
		GoogleBusinessName:   "Public Bistro",
		GoogleReviewLink:     "https://g.page/review",
		GoogleBusinessURL:    "https://maps.google.com/bistro",
		DefaultCurrency:      "USD",
		DisplayCurrency:      "EUR",
		DefaultLanguage:      "en",
		SourceLanguage:       "en",
		TaxRate:              8.5,
		ServiceFeeRate:       10.0,
		TaxInclusive:         true,
		ServiceInclusive:     false,
		Timezone:             "America/New_York",
		KitchenEnabled:       true,
		OrdersEnabled:        true,
		CRMEnabled:           true,
		CounterEnabled:       true,
		IsActive:             true,
		BusinessPageEnabled:  true,
		Address: BusinessAddress{
			Street:     "1 Main St",
			City:       "Townsville",
			State:      "NY",
			PostalCode: "10001",
			Country:    "US",
		},
		DesignSettings: BusinessDesignSettings{
			PrimaryColor: "#1a6b6a",
			Theme:        "light",
			MenuLayout:   "grid",
		},
		AiSettings: BusinessAiSettings{
			AiEnabled:             true,
			AiName:                "Sage",
			AiPriority:            "upselling",
			BusinessPageAiEnabled: true,
		},
		// Private columns that must NOT be projected on a guest path.
		OnboardingState: JSONRawMessage(`{"secret":"do-not-leak"}`),
	}
	require.NoError(t, db.Create(business).Error)

	table := &Table{
		BusinessID: business.ID,
		TableCode:  tableCode,
		Name:       "Patio",
		Capacity:   4,
		QRCode:     strings.Repeat("qr-payload", 64),
		IsActive:   true,
	}
	require.NoError(t, db.Create(table).Error)

	return business, table
}

// publicConsumedColumns are the DB columns that the public storefront
// (publicBusinessProjection) and the QR guest response
// (buildPublicGuestBusinessResponse, IsAIWaiterAvailable, guestOrderingEnabled)
// read off the returned Business. The projected SELECT must include every one
// of these — under-projecting any blanks the live storefront.
var publicConsumedColumns = []string{
	// gating + identity
	"id", "business_id", "is_active", "business_page_enabled",
	"name", "logo", "custom_url",
	// embedded address
	"street", "city", "state", "postal_code", "country",
	// contact + social
	"description", "phone", "website", "social_media", "banner_images",
	// reviews / google
	"show_reviews", "google_reviews_enabled", "google_place_id",
	"google_business_name", "google_review_link", "google_business_url",
	// currency / language
	"default_currency", "display_currency", "default_language", "source_language",
	// money rates
	"tax_rate", "service_fee_rate", "tax_inclusive", "service_inclusive",
	// hospitality copy + toggles
	"welcome_message", "about_story", "show_welcome_message", "show_about_story",
	"show_gallery", "show_operating_hours", "show_special_features",
	// embedded design settings (design_ prefix)
	"design_primary_color", "design_theme", "design_menu_layout",
	// embedded AI settings — also fed to IsAIWaiterAvailable. Columns are
	// DOUBLE-prefixed (embeddedPrefix "ai_" + "Ai*" field names).
	"ai_ai_enabled", "ai_ai_name", "ai_ai_priority", "ai_business_page_ai_enabled",
	// QR-response operational toggles
	"timezone", "kitchen_enabled", "orders_enabled", "crm_enabled", "counter_enabled",
	// is_demo — demo showrooms never lock.
	"is_demo",
	// closed_at — with is_active, the administrator lifecycle lock that
	// IsBusinessOperational / IsAIWaiterAvailable read.
	"closed_at",
	// kind — public storefront noindex for demo/test venues (#549).
	"kind",
	// timestamps surfaced by publicBusinessProjection
	"created_at", "updated_at",
}

// privateDroppedColumns must NOT appear in either guest-path projection.
var privateDroppedColumns = []string{
	"owner_address", "onboarding_state",
}

func assertPublicBusinessProjectionShape(t *testing.T, rec *publicBizSQLRecorder, biz *Business, loaded *Business) {
	t.Helper()

	// No SELECT * on businesses — the row must be projected.
	assert.Zero(t, rec.businessSelectStarCount(),
		"public guest read should project Business columns instead of SELECT *")

	// Every consumed column must be present in the projected SELECT.
	for _, col := range publicConsumedColumns {
		assert.True(t, rec.businessSelectMentions(col),
			"projected Business SELECT must include %q (a public consumer reads it)", col)
	}

	// Private columns must NOT be selected.
	for _, col := range privateDroppedColumns {
		assert.False(t, rec.businessSelectMentions(col),
			"projected Business SELECT must NOT include %q (leak / over-fetch)", col)
	}

	// The hydrated struct must carry the representative public fields...
	assert.Equal(t, biz.Name, loaded.Name)
	assert.Equal(t, biz.CustomURL, loaded.CustomURL)
	assert.Equal(t, biz.WelcomeMessage, loaded.WelcomeMessage)
	assert.Equal(t, biz.AboutStory, loaded.AboutStory)
	assert.Equal(t, biz.DefaultCurrency, loaded.DefaultCurrency)
	assert.Equal(t, biz.DisplayCurrency, loaded.DisplayCurrency)
	assert.Equal(t, biz.Address.City, loaded.Address.City)
	assert.Equal(t, biz.DesignSettings.PrimaryColor, loaded.DesignSettings.PrimaryColor)
	assert.Equal(t, biz.AiSettings.AiName, loaded.AiSettings.AiName)
	assert.True(t, loaded.KitchenEnabled)
	assert.True(t, loaded.OrdersEnabled)

	// ...and leave the un-projected private fields zeroed.
	assert.Empty(t, string(loaded.OnboardingState), "onboarding blob must not be hydrated")
}

func TestGetBusinessByCustomURLProjectsPublicColumns(t *testing.T) {
	rec := &publicBizSQLRecorder{Interface: logger.Default.LogMode(logger.Silent)}
	setupPublicBizProjectionDB(t, rec)

	biz, _ := seedPublicFatBusiness(t, "public-bistro", "QR-CUSTOMURL-1")

	rec.reset()
	loaded, err := GetBusinessByCustomURL("public-bistro")
	require.NoError(t, err)
	require.NotNil(t, loaded)

	assertPublicBusinessProjectionShape(t, rec, biz, loaded)
}

func TestGetActiveTableWithBusinessByCodeProjectsPublicColumns(t *testing.T) {
	rec := &publicBizSQLRecorder{Interface: logger.Default.LogMode(logger.Silent)}
	setupPublicBizProjectionDB(t, rec)

	biz, _ := seedPublicFatBusiness(t, "public-bistro-qr", "QR-TABLE-1")

	rec.reset()
	table, loaded, err := GetActiveTableWithBusinessByCode("QR-TABLE-1")
	require.NoError(t, err)
	require.NotNil(t, table)
	require.NotNil(t, loaded)

	// Table's own fields must remain intact.
	assert.Equal(t, "Patio", table.Name)
	assert.Equal(t, "QR-TABLE-1", table.TableCode)
	assert.Equal(t, 4, table.Capacity)

	assertPublicBusinessProjectionShape(t, rec, biz, loaded)
}

// TestGetActiveTableWithBusinessByCodeIsCaseInsensitive proves a lowercase or
// mixed-case deep link (/t/abc123) resolves the same uppercase-stored table —
// codes are always generated uppercase, so the lookup normalizes its input.
func TestGetActiveTableWithBusinessByCodeIsCaseInsensitive(t *testing.T) {
	rec := &publicBizSQLRecorder{Interface: logger.Default.LogMode(logger.Silent)}
	setupPublicBizProjectionDB(t, rec)

	seedPublicFatBusiness(t, "case-insensitive-qr", "QRTABLE1")

	for _, code := range []string{"qrtable1", "QrTable1", "  qrtable1  "} {
		table, _, err := GetActiveTableWithBusinessByCode(code)
		require.NoError(t, err, "code %q should resolve", code)
		require.NotNil(t, table)
		assert.Equal(t, "QRTABLE1", table.TableCode)
	}

	// A genuinely unknown code still 404s with the sentinel.
	_, _, err := GetActiveTableWithBusinessByCode("nope999")
	assert.ErrorIs(t, err, ErrTableNotFound)
}

func TestGetActiveTableWithBusinessByCodeSupportsLegacyLowercaseStoredCodes(t *testing.T) {
	rec := &publicBizSQLRecorder{Interface: logger.Default.LogMode(logger.Silent)}
	setupPublicBizProjectionDB(t, rec)

	seedPublicFatBusiness(t, "legacy-lowercase-qr", "legacy-table-1")

	for _, code := range []string{"legacy-table-1", "LEGACY-TABLE-1", " Legacy-Table-1 "} {
		table, _, err := GetActiveTableWithBusinessByCode(code)
		require.NoError(t, err, "code %q should resolve legacy lowercase rows", code)
		require.NotNil(t, table)
		assert.Equal(t, "legacy-table-1", table.TableCode)
	}
}

// --- OF-04 lock-state parity regression ---------------------------------
//
// The column-mention assertions above can share a blind spot with
// publicBusinessColumns (both are hand-maintained lists). These tests instead
// prove the SECURITY-critical property directly: the billing/lock gate computed
// off a PROJECTED guest load must equal the gate computed off a FULL-ROW load,
// for every subscription state — so a dropped subscription column (e.g.
// subscription_end_date / cancel_at_period_end) can't silently fail-open guest
// ordering + AI on a lapsed account or fail-close a paying one. This FAILS if
// either column is removed from publicBusinessColumns (verified RED).

// seedLockStateBusiness creates an active business plus a table, so both
// projected loaders can read it. All other public
// columns are left at defaults — only the lock-state inputs matter here.
func seedLockStateBusiness(t testing.TB, customURL, tableCode string) *Business {
	t.Helper()

	business := &Business{
		BusinessId:          fmt.Sprintf("lock-%d", time.Now().UnixNano()),
		Name:                "Lock Bistro",
		OwnerAddress:        fmt.Sprintf("0xlock%x", time.Now().UnixNano()),
		CustomURL:           customURL,
		IsActive:            true,
		BusinessPageEnabled: true,
		AiSettings:          BusinessAiSettings{AiEnabled: true},
	}
	require.NoError(t, db.Create(business).Error)

	table := &Table{
		BusinessID: business.ID,
		TableCode:  tableCode,
		Name:       "T1",
		Capacity:   2,
		IsActive:   true,
	}
	require.NoError(t, db.Create(table).Error)

	return business
}

// assertLockStateParity loads the business three ways — full row, projected
// custom-URL loader, projected QR/table loader — and asserts all three compute
// the IDENTICAL admin status, the expected status, and identical
// IsBusinessOperational / IsAIWaiterAvailable gates.
func assertLockStateParity(t *testing.T, customURL, tableCode string, biz *Business, wantStatus string, wantActive bool) {
	t.Helper()

	fullRow, err := GetBusinessByID(biz.ID)
	require.NoError(t, err)

	viaCustomURL, err := GetBusinessByCustomURL(customURL)
	require.NoError(t, err)

	_, viaTable, err := GetActiveTableWithBusinessByCode(tableCode)
	if wantStatus == BusinessStatusSuspended {
		// The QR loader refuses a suspended (is_active=false) venue outright,
		// which is a stricter lock than the projected gate itself.
		require.ErrorIs(t, err, ErrTableNotFound, "QR/table loader must refuse a suspended venue")
		viaTable = nil
	} else {
		require.NoError(t, err)
	}

	fullState := AdminBusinessStatus(fullRow)

	// The expected outcome — anchors the test so it can't silently agree on a
	// wrong value.
	assert.Equal(t, wantStatus, fullState, "full-row admin status")
	assert.Equal(t, wantActive, IsBusinessOperational(fullRow), "full-row IsBusinessOperational")

	// PARITY: projected loaders must match the full-row gate exactly. This is
	// the assertion that catches a dropped lifecycle column — the projected
	// load would zero is_active / closed_at and diverge.
	assert.Equal(t, fullState, AdminBusinessStatus(viaCustomURL),
		"custom-URL projected loader must compute SAME admin status as full row")
	if viaTable != nil {
		assert.Equal(t, fullState, AdminBusinessStatus(viaTable),
			"QR/table projected loader must compute SAME admin status as full row")
		assert.Equal(t, IsBusinessOperational(fullRow), IsBusinessOperational(viaTable),
			"QR/table projected IsBusinessOperational must match full row")
		assert.Equal(t, IsAIWaiterAvailable(fullRow), IsAIWaiterAvailable(viaTable),
			"QR/table projected IsAIWaiterAvailable must match full row")
	}

	assert.Equal(t, IsBusinessOperational(fullRow), IsBusinessOperational(viaCustomURL),
		"custom-URL projected IsBusinessOperational must match full row")

	assert.Equal(t, IsAIWaiterAvailable(fullRow), IsAIWaiterAvailable(viaCustomURL),
		"custom-URL projected IsAIWaiterAvailable must match full row")
}

// TestProjectedLoadersComputeIdenticalLockState covers the admin lifecycle
// lock (the only lock): the projected loaders must carry is_active and
// closed_at or a suspended/closed venue keeps taking guest orders.
func TestProjectedLoadersComputeIdenticalLockState(t *testing.T) {
	tests := []struct {
		name       string
		suspend    bool
		close      bool
		wantStatus string
		wantActive bool
	}{
		{name: "active", wantStatus: BusinessStatusActive, wantActive: true},
		{name: "suspended", suspend: true, wantStatus: BusinessStatusSuspended, wantActive: false},
		{name: "closed", close: true, wantStatus: BusinessStatusClosed, wantActive: false},
	}

	for i, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			rec := &publicBizSQLRecorder{Interface: logger.Default.LogMode(logger.Silent)}
			setupPublicBizProjectionDB(t, rec)

			customURL := fmt.Sprintf("lock-biz-%d", i)
			tableCode := fmt.Sprintf("LOCK-TABLE-%d", i)
			biz := seedLockStateBusiness(t, customURL, tableCode)
			if tc.suspend {
				require.NoError(t, db.Model(&Business{}).Where("id = ?", biz.ID).UpdateColumn("is_active", false).Error)
			}
			if tc.close {
				require.NoError(t, db.Model(&Business{}).Where("id = ?", biz.ID).Update("closed_at", time.Now().Add(-time.Hour)).Error)
			}

			assertLockStateParity(t, customURL, tableCode, biz, tc.wantStatus, tc.wantActive)
			if tc.close {
				_, viaTable, err := GetActiveTableWithBusinessByCode(tableCode)
				require.NoError(t, err)
				assert.False(t, IsAIWaiterAvailable(viaTable), "locked venue must not offer the AI waiter")
			}
		})
	}
}

// BenchmarkGetBusinessByCustomURL measures the public storefront loader over a
// fully-populated business.
func BenchmarkGetBusinessByCustomURL(b *testing.B) {
	rec := &publicBizSQLRecorder{Interface: logger.Default.LogMode(logger.Silent)}
	setupPublicBizProjectionDB(b, rec)
	seedPublicFatBusiness(b, "bench-bistro", "QR-BENCH-CUSTOMURL")

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := GetBusinessByCustomURL("bench-bistro"); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkGetActiveTableWithBusinessByCode measures the QR/table guest loader
// over a fully-populated business.
func BenchmarkGetActiveTableWithBusinessByCode(b *testing.B) {
	rec := &publicBizSQLRecorder{Interface: logger.Default.LogMode(logger.Silent)}
	setupPublicBizProjectionDB(b, rec)
	seedPublicFatBusiness(b, "bench-bistro-qr", "QR-BENCH-TABLE")

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, _, err := GetActiveTableWithBusinessByCode("QR-BENCH-TABLE"); err != nil {
			b.Fatal(err)
		}
	}
}
