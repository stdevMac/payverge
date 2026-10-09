package database

import (
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

func setupPublicHospitalityDB(t testing.TB, rec logger.Interface) {
	t.Helper()
	prev := db
	t.Cleanup(func() { SetTestDB(prev) })

	dsnName := strings.NewReplacer("/", "_", " ", "_").Replace(t.Name())
	dsn := fmt.Sprintf("file:%s-%d?mode=memory&cache=shared", dsnName, time.Now().UnixNano())
	cfg := &gorm.Config{}
	if rec != nil {
		cfg.Logger = rec
	}
	gormDB, err := gorm.Open(sqlite.Open(dsn), cfg)
	require.NoError(t, err)
	sqlDB, err := gormDB.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = sqlDB.Close() })

	SetTestDB(gormDB)
	require.NoError(t, gormDB.AutoMigrate(
		&Business{},
		&BusinessGalleryImage{},
		&BusinessOperatingHours{},
		&BusinessOperatingException{},
		&BusinessSpecialFeature{},
		&BusinessLanguage{},
		&SupportedLanguage{},
	))
}

func seedPublicHospitalityBusiness(t testing.TB) *Business {
	t.Helper()
	business := &Business{
		BusinessId:          fmt.Sprintf("pub-hosp-%d", time.Now().UnixNano()),
		Name:                "Public Hours",
		OwnerAddress:        fmt.Sprintf("0xpubhosp%x", time.Now().UnixNano()),
		CustomURL:           fmt.Sprintf("pub-hosp-%d", time.Now().UnixNano()),
		IsActive:            true,
		BusinessPageEnabled: true,
	}
	require.NoError(t, db.Create(business).Error)
	return business
}

func TestGetPublicBusinessOperatingExceptions_ExcludesPastAndBounds(t *testing.T) {
	setupPublicHospitalityDB(t, nil)
	biz := seedPublicHospitalityBusiness(t)
	today := time.Now().UTC().Truncate(24 * time.Hour)

	require.NoError(t, db.Create(&BusinessOperatingException{
		BusinessID: biz.ID, ExceptionDate: today.AddDate(-3, 0, 0), IsClosed: true, Label: "years-ago",
	}).Error)
	require.NoError(t, db.Create(&BusinessOperatingException{
		BusinessID: biz.ID, ExceptionDate: today, IsClosed: true, Label: "today",
	}).Error)
	require.NoError(t, db.Create(&BusinessOperatingException{
		BusinessID: biz.ID, ExceptionDate: today.AddDate(0, 0, 10), IsClosed: true, Label: "upcoming",
	}).Error)

	publicRows, err := GetPublicBusinessOperatingExceptions(biz.ID)
	require.NoError(t, err)
	labels := make([]string, 0, len(publicRows))
	for _, row := range publicRows {
		labels = append(labels, row.Label)
	}
	assert.NotContains(t, labels, "years-ago")
	assert.Contains(t, labels, "today")
	assert.Contains(t, labels, "upcoming")

	operatorRows, err := GetBusinessOperatingExceptions(biz.ID)
	require.NoError(t, err)
	operatorLabels := make([]string, 0, len(operatorRows))
	for _, row := range operatorRows {
		operatorLabels = append(operatorLabels, row.Label)
	}
	assert.Contains(t, operatorLabels, "years-ago", "operator editor must still see historical exceptions")
}

func TestGetPublicStorefrontHospitalityAccessShape(t *testing.T) {
	rec := &publicBizSQLRecorder{Interface: logger.Default.LogMode(logger.Silent)}
	setupPublicHospitalityDB(t, rec)
	biz := seedPublicHospitalityBusiness(t)

	require.NoError(t, db.Create(&BusinessGalleryImage{
		BusinessID: biz.ID, ImageURL: "https://cdn.example.com/a.jpg", IsActive: true,
	}).Error)
	require.NoError(t, db.Create(&BusinessOperatingHours{
		BusinessID: biz.ID, DayOfWeek: 1, OpenTime: "11:00", CloseTime: "23:00",
	}).Error)
	require.NoError(t, db.Create(&BusinessOperatingException{
		BusinessID: biz.ID, ExceptionDate: time.Now().UTC(), IsClosed: true, Label: "today",
	}).Error)
	require.NoError(t, db.Create(&BusinessSpecialFeature{
		BusinessID: biz.ID, Title: "Patio", IsActive: true,
	}).Error)
	require.NoError(t, db.Create(&BusinessLanguage{
		BusinessID: biz.ID, LanguageCode: "en", IsDefault: true,
	}).Error)
	require.NoError(t, db.Create(&SupportedLanguage{
		Code: "en", Name: "English", NativeName: "English", IsActive: true,
	}).Error)

	rec.reset()
	_, err := GetPublicBusinessGalleryImages(biz.ID)
	require.NoError(t, err)
	_, err = GetPublicBusinessOperatingHours(biz.ID)
	require.NoError(t, err)
	_, err = GetPublicBusinessOperatingExceptions(biz.ID)
	require.NoError(t, err)
	_, err = GetPublicBusinessSpecialFeatures(biz.ID)
	require.NoError(t, err)
	_, err = GetPublicBusinessLanguages(biz.ID)
	require.NoError(t, err)
	InvalidatePublicSupportedLanguages()
	_, err = GetCachedPublicSupportedLanguages()
	require.NoError(t, err)

	joined := strings.ToLower(strings.Join(rec.statements, "\n"))
	for _, table := range []string{
		"business_gallery_images",
		"business_operating_hours",
		"business_operating_exceptions",
		"business_special_features",
		"business_languages",
		"supported_languages",
	} {
		assert.NotContains(t, joined, "select * from `"+table+"`", "public %s must not SELECT *", table)
		assert.Contains(t, joined, "from `"+table+"`", "expected a SELECT from %s", table)
	}
	assert.Contains(t, joined, "limit ", "public hospitality reads must be bounded")
	assert.Contains(t, joined, "exception_date", "exceptions must date-filter")
}

func TestGetCachedPublicSupportedLanguages_DoesNotRequery(t *testing.T) {
	rec := &publicBizSQLRecorder{Interface: logger.Default.LogMode(logger.Silent)}
	setupPublicHospitalityDB(t, rec)
	require.NoError(t, db.Create(&SupportedLanguage{
		Code: "en", Name: "English", NativeName: "English", IsActive: true,
	}).Error)

	InvalidatePublicSupportedLanguages()
	rec.reset()
	first, err := GetCachedPublicSupportedLanguages()
	require.NoError(t, err)
	require.Len(t, first, 1)
	second, err := GetCachedPublicSupportedLanguages()
	require.NoError(t, err)
	require.Equal(t, first, second)

	count := 0
	for _, stmt := range rec.statements {
		normalized := strings.ToLower(stmt)
		if strings.Contains(normalized, "from `supported_languages`") ||
			strings.Contains(normalized, "from \"supported_languages\"") {
			count++
		}
	}
	assert.Equal(t, 1, count, "second call must be served from the process cache")
}
