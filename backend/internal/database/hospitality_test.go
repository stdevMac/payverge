package database

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestValidateBusinessOperatingHoursRejectsInvalidDayRange(t *testing.T) {
	err := ValidateBusinessOperatingHours([]BusinessOperatingHours{
		{DayOfWeek: 7, OpenTime: "09:00", CloseTime: "17:00"},
	})

	require.Error(t, err)
	require.Contains(t, err.Error(), "day_of_week must be between 0 and 6")
}

func TestValidateBusinessOperatingHoursAcceptsSplitShifts(t *testing.T) {
	// Lunch 11–15 + dinner 19–23 on the same day (split shift).
	err := ValidateBusinessOperatingHours([]BusinessOperatingHours{
		{DayOfWeek: 1, OpenTime: "11:00", CloseTime: "15:00"},
		{DayOfWeek: 1, OpenTime: "19:00", CloseTime: "23:00"},
		{DayOfWeek: 6, IsClosed: true},
	})
	require.NoError(t, err)
}

func TestValidateBusinessOperatingHoursRejectsMoreThanTwoOpenPeriods(t *testing.T) {
	err := ValidateBusinessOperatingHours([]BusinessOperatingHours{
		{DayOfWeek: 1, OpenTime: "08:00", CloseTime: "10:00"},
		{DayOfWeek: 1, OpenTime: "12:00", CloseTime: "14:00"},
		{DayOfWeek: 1, OpenTime: "18:00", CloseTime: "22:00"},
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "at most 2 open periods")
}

func TestValidateBusinessOperatingHoursRejectsClosedPlusOpenSameDay(t *testing.T) {
	err := ValidateBusinessOperatingHours([]BusinessOperatingHours{
		{DayOfWeek: 1, IsClosed: true},
		{DayOfWeek: 1, OpenTime: "11:00", CloseTime: "15:00"},
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "closed day cannot also have open periods")
}

func TestValidateBusinessOperatingHoursAcceptsOneRowPerDay(t *testing.T) {
	err := ValidateBusinessOperatingHours([]BusinessOperatingHours{
		{DayOfWeek: 0, OpenTime: "09:00", CloseTime: "17:00"},
		{DayOfWeek: 6, IsClosed: true},
	})

	require.NoError(t, err)
}

func newGalleryDB(t *testing.T) *DB {
	t.Helper()
	prev := db
	t.Cleanup(func() { SetTestDB(prev) })
	dsn := "file:" + t.Name() + "?mode=memory&cache=shared"
	gormDB, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := gormDB.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, gormDB.AutoMigrate(&Business{}, &BusinessGalleryImage{}))
	SetTestDB(gormDB)
	require.NoError(t, db.Create(&Business{ID: 1, BusinessId: "biz-gallery-1"}).Error)
	return GetDBWrapper()
}

// TestUpdateBusinessGalleryImages_PreservesRowIDs asserts the write path
// upserts in place instead of delete-all + re-insert (which minted fresh IDs
// and forced a full 21-locale caption re-translation on every typo fix).
func TestUpdateBusinessGalleryImages_PreservesRowIDs(t *testing.T) {
	_ = newGalleryDB(t)

	seed := []BusinessGalleryImage{
		{BusinessID: 1, ImageURL: "https://cdn.example.com/a.jpg", Caption: "A", DisplayOrder: 0, IsActive: true},
		{BusinessID: 1, ImageURL: "https://cdn.example.com/b.jpg", Caption: "B", DisplayOrder: 1, IsActive: true},
	}
	result, err := UpdateBusinessGalleryImages(1, seed)
	require.NoError(t, err)
	require.True(t, result.CaptionsChanged, "first insert of captioned rows is a caption change")

	existing, err := GetBusinessGalleryImages(1)
	require.NoError(t, err)
	require.Len(t, existing, 2)
	idA, idB := existing[0].ID, existing[1].ID
	require.NotZero(t, idA)
	require.NotZero(t, idB)

	// Reorder + typo fix on A; keep B caption. IDs must survive.
	updated := []BusinessGalleryImage{
		{ID: idB, ImageURL: "https://cdn.example.com/b.jpg", Caption: "B", DisplayOrder: 0, IsActive: true},
		{ID: idA, ImageURL: "https://cdn.example.com/a.jpg", Caption: "A fixed", DisplayOrder: 1, IsActive: true},
	}
	result, err = UpdateBusinessGalleryImages(1, updated)
	require.NoError(t, err)
	require.True(t, result.CaptionsChanged)

	after, err := GetBusinessGalleryImages(1)
	require.NoError(t, err)
	require.Len(t, after, 2)
	// Order is display_order ASC
	require.Equal(t, idB, after[0].ID, "row B id must be preserved")
	require.Equal(t, idA, after[1].ID, "row A id must be preserved")
	require.Equal(t, "B", after[0].Caption)
	require.Equal(t, "A fixed", after[1].Caption)
}

// TestUpdateBusinessGalleryImages_SkipsCaptionChangeWhenUnchanged covers the
// handler's translation short-circuit signal: display-order / is_active only
// edits must not report CaptionsChanged.
func TestUpdateBusinessGalleryImages_SkipsCaptionChangeWhenUnchanged(t *testing.T) {
	_ = newGalleryDB(t)

	seed := []BusinessGalleryImage{
		{BusinessID: 1, ImageURL: "https://cdn.example.com/a.jpg", Caption: "Same", DisplayOrder: 0, IsActive: true},
	}
	_, err := UpdateBusinessGalleryImages(1, seed)
	require.NoError(t, err)
	existing, err := GetBusinessGalleryImages(1)
	require.NoError(t, err)
	require.Len(t, existing, 1)

	// Reorder-only (single row) + flip is_active — captions identical.
	result, err := UpdateBusinessGalleryImages(1, []BusinessGalleryImage{
		{ID: existing[0].ID, ImageURL: existing[0].ImageURL, Caption: "Same", DisplayOrder: 0, IsActive: false},
	})
	require.NoError(t, err)
	require.False(t, result.CaptionsChanged, "caption text unchanged → no translation fan-out")
	require.Equal(t, existing[0].ID, result.PreservedIDs[0])
}

// TestUpdateBusinessGalleryImages_DeletesMissingRows ensures removed images
// are dropped while remaining IDs stay stable.
func TestUpdateBusinessGalleryImages_DeletesMissingRows(t *testing.T) {
	_ = newGalleryDB(t)

	_, err := UpdateBusinessGalleryImages(1, []BusinessGalleryImage{
		{ImageURL: "https://cdn.example.com/keep.jpg", Caption: "Keep", IsActive: true},
		{ImageURL: "https://cdn.example.com/drop.jpg", Caption: "Drop", IsActive: true},
	})
	require.NoError(t, err)
	existing, err := GetBusinessGalleryImages(1)
	require.NoError(t, err)
	require.Len(t, existing, 2)
	keepID := existing[0].ID

	result, err := UpdateBusinessGalleryImages(1, []BusinessGalleryImage{
		{ID: keepID, ImageURL: "https://cdn.example.com/keep.jpg", Caption: "Keep", IsActive: true},
	})
	require.NoError(t, err)
	require.False(t, result.CaptionsChanged)
	require.Contains(t, result.DeletedIDs, existing[1].ID)

	after, err := GetBusinessGalleryImages(1)
	require.NoError(t, err)
	require.Len(t, after, 1)
	require.Equal(t, keepID, after[0].ID)
}

// galleryWriteDeleteAll is the pre-Stream-9 write path kept only for
// benchmark baselines (delete-all + re-insert, fresh IDs every save).
func galleryWriteDeleteAll(businessID uint, images []BusinessGalleryImage) error {
	db := GetDBWrapper()
	tx := db.GetGorm().Begin()
	if tx.Error != nil {
		return tx.Error
	}
	if err := tx.Where("business_id = ?", businessID).Delete(&BusinessGalleryImage{}).Error; err != nil {
		tx.Rollback()
		return err
	}
	now := time.Now()
	for i, image := range images {
		image.ID = 0
		image.BusinessID = businessID
		image.DisplayOrder = i
		image.CreatedAt = now
		image.UpdatedAt = now
		if err := tx.Create(&image).Error; err != nil {
			tx.Rollback()
			return err
		}
	}
	return tx.Commit().Error
}

var galleryBenchSeq int64

func seedGalleryForBench(b *testing.B, n int) []BusinessGalleryImage {
	b.Helper()
	prev := db
	b.Cleanup(func() { SetTestDB(prev) })
	// Unique DSN per call so -count=N and parallel benches never share tables.
	galleryBenchSeq++
	dsn := "file:bench-gallery-" + b.Name() + "-" + itoa(int(galleryBenchSeq)) + "?mode=memory&cache=shared"
	gormDB, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(b, err)
	sqlDB, err := gormDB.DB()
	require.NoError(b, err)
	sqlDB.SetMaxOpenConns(1)
	require.NoError(b, gormDB.AutoMigrate(&Business{}, &BusinessGalleryImage{}))
	SetTestDB(gormDB)
	require.NoError(b, db.Create(&Business{ID: 1, BusinessId: "biz-bench-gallery-" + itoa(int(galleryBenchSeq))}).Error)

	seed := make([]BusinessGalleryImage, n)
	for i := 0; i < n; i++ {
		seed[i] = BusinessGalleryImage{
			ImageURL:     "https://cdn.example.com/img-" + itoa(i) + ".jpg",
			Caption:      "Caption " + itoa(i),
			DisplayOrder: i,
			IsActive:     true,
		}
	}
	_, err = UpdateBusinessGalleryImages(1, seed)
	require.NoError(b, err)
	existing, err := GetBusinessGalleryImages(1)
	require.NoError(b, err)
	return existing
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	var buf [12]byte
	pos := len(buf)
	n := i
	for n > 0 {
		pos--
		buf[pos] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[pos:])
}

// BenchmarkGalleryWrite_CaptionTypoFix measures a single-caption edit of a
// 12-image gallery (typical operator typo fix) under the upsert path vs the
// legacy delete-all path.
func BenchmarkGalleryWrite_CaptionTypoFix_Upsert(b *testing.B) {
	existing := seedGalleryForBench(b, 12)
	payload := make([]BusinessGalleryImage, len(existing))
	copy(payload, existing)
	payload[0].Caption = "Typo fixed"

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		payload[0].Caption = "Typo fixed " + itoa(i)
		if _, err := UpdateBusinessGalleryImages(1, payload); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkGalleryWrite_CaptionTypoFix_DeleteAll(b *testing.B) {
	existing := seedGalleryForBench(b, 12)
	payload := make([]BusinessGalleryImage, len(existing))
	copy(payload, existing)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// Strip IDs to simulate the old full-replace payload.
		for j := range payload {
			payload[j].ID = 0
			payload[j].Caption = existing[j].Caption
		}
		payload[0].Caption = "Typo fixed " + itoa(i)
		if err := galleryWriteDeleteAll(1, payload); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkGalleryWrite_ReorderOnly_Upsert is the short-circuit case: no
// caption text change — handler skips translation fan-out entirely.
func BenchmarkGalleryWrite_ReorderOnly_Upsert(b *testing.B) {
	existing := seedGalleryForBench(b, 12)
	payload := make([]BusinessGalleryImage, len(existing))
	// Reverse order each iteration.
	for j := range existing {
		payload[j] = existing[len(existing)-1-j]
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// Flip order every iteration so work stays real.
		if i%2 == 0 {
			for j := range existing {
				payload[j] = existing[len(existing)-1-j]
			}
		} else {
			copy(payload, existing)
		}
		if _, err := UpdateBusinessGalleryImages(1, payload); err != nil {
			b.Fatal(err)
		}
	}
}
