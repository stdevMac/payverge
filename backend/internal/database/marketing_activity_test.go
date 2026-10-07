package database

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func newMarketingActivityDB(t *testing.T) *DB {
	t.Helper()
	prev := db
	t.Cleanup(func() { SetTestDB(prev) })
	dsn := "file:" + t.Name() + "?mode=memory&cache=shared"
	gormDB, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := gormDB.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, gormDB.AutoMigrate(&Business{}, &MarketingActivity{}))
	SetTestDB(gormDB)
	require.NoError(t, db.Create(&Business{ID: 1, BusinessId: "biz-1"}).Error)
	return GetDBWrapper()
}

func TestEnsureInventoryHiddenMarketingActivity_InsertsOnceAndSkipsPosted(t *testing.T) {
	d := newMarketingActivityDB(t)
	require.NoError(t, d.EnsureInventoryHiddenMarketingActivity(MarketingActivity{
		BusinessID: 1, SuggestionID: "1:featured_dish:steak", Play: "featured_dish",
		Title: "Feature your star: Steak Plate", TargetName: "Steak Plate",
	}))
	rows, total, err := d.ListMarketingActivities(MarketingActivityQuery{BusinessID: 1, Status: "dismissed"})
	require.NoError(t, err)
	require.Equal(t, int64(1), total)
	require.Equal(t, "Steak Plate", rows[0].TargetName)
	require.Equal(t, "inventory", rows[0].CreatedBy)

	require.NoError(t, d.EnsureInventoryHiddenMarketingActivity(MarketingActivity{
		BusinessID: 1, SuggestionID: "1:featured_dish:steak", Play: "featured_dish",
		Title: "ignored overwrite", TargetName: "ignored",
	}))
	rows, total, err = d.ListMarketingActivities(MarketingActivityQuery{BusinessID: 1, Status: "dismissed"})
	require.NoError(t, err)
	require.Equal(t, int64(1), total)
	require.Equal(t, "Steak Plate", rows[0].TargetName)

	posted, err := d.RecordMarketingActivity(MarketingActivity{
		BusinessID: 1, SuggestionID: "1:featured_dish:bowl", Play: "featured_dish",
		Title: "Feature bowl", TargetName: "Harvest Bowl", Status: "posted",
	})
	require.NoError(t, err)
	require.Equal(t, "posted", posted.Status)
	require.NoError(t, d.EnsureInventoryHiddenMarketingActivity(MarketingActivity{
		BusinessID: 1, SuggestionID: "1:featured_dish:bowl", Play: "featured_dish",
		Title: "should not clobber", TargetName: "Harvest Bowl",
	}))
	var bowl MarketingActivity
	require.NoError(t, d.GetGorm().Where("suggestion_id = ?", "1:featured_dish:bowl").First(&bowl).Error)
	require.Equal(t, "posted", bowl.Status)
}

func TestRecordMarketingActivity_UpsertsOnSuggestionID(t *testing.T) {
	d := newMarketingActivityDB(t)
	in := MarketingActivity{
		BusinessID: 1, SuggestionID: "1:featured_dish:m1", Play: "featured_dish",
		Title: "Feature your star", TargetName: "Carbonara", Status: "posted",
		ImageURL: "https://cdn/c.jpg", Caption: "Fresh pasta", CreatedBy: "owner:0x1",
	}
	first, err := d.RecordMarketingActivity(in)
	require.NoError(t, err)
	require.NotZero(t, first.ID)
	require.NotNil(t, first.PostedAt)

	// Same suggestion_id transitions status (no duplicate row).
	in.Status = "dismissed"
	second, err := d.RecordMarketingActivity(in)
	require.NoError(t, err)
	require.Equal(t, first.ID, second.ID, "must upsert the same row, not create a second")
	require.Equal(t, "dismissed", second.Status)
	require.NotNil(t, second.DismissedAt)

	var count int64
	require.NoError(t, d.GetGorm().Model(&MarketingActivity{}).Count(&count).Error)
	require.Equal(t, int64(1), count)
}

func TestMarketingCreativeSnapshotRoundTrip(t *testing.T) {
	d := newMarketingActivityDB(t)
	legacy, err := d.RecordMarketingActivity(MarketingActivity{
		BusinessID: 1, SuggestionID: "1:featured_dish:m42", Play: "featured_dish",
		Status: "dismissed",
	})
	require.NoError(t, err)
	require.Nil(t, legacy.CreativeSnapshot, "legacy activity has no captured creative")

	want := &MarketingCreativeSnapshot{
		Caption:     "Tonight's handmade pasta.",
		ImageURL:    "https://cdn.example.com/marketing/pasta.jpg",
		ImageSource: "generated",
		Template:    "editorial",
		Aspect:      "4:5",
		FontFamily:  "Serif",
		Slots: map[string]string{
			"headline": "Handmade tonight",
			"cta":      "Reserve a table",
		},
		Crop: MarketingCrop{X: 0.42, Y: 0.31, Zoom: 1.35},
	}

	recorded, err := d.RecordMarketingActivity(MarketingActivity{
		BusinessID: 1, SuggestionID: "1:featured_dish:m42", Play: "featured_dish",
		Status: "posted", CreativeSnapshot: want,
	})
	require.NoError(t, err)
	require.Equal(t, legacy.ID, recorded.ID, "snapshot updates the existing activity row")
	require.Equal(t, want, recorded.CreativeSnapshot)

	rows, total, err := d.ListMarketingActivities(MarketingActivityQuery{
		BusinessID: 1, Page: 1, PerPage: 20,
	})
	require.NoError(t, err)
	require.Equal(t, int64(1), total)
	require.Len(t, rows, 1)
	require.Equal(t, want, rows[0].CreativeSnapshot)
}

func TestRestoreMarketingActivity_PreviouslyPostedDismissalReturnsToPosted(t *testing.T) {
	d := newMarketingActivityDB(t)
	want := &MarketingCreativeSnapshot{
		Caption:     "Exact approved caption",
		ImageURL:    "https://cdn.example.com/approved.jpg",
		ImageSource: "generated",
		Template:    "editorial",
		Aspect:      "4:5",
		Slots:       map[string]string{"headline": "Tonight only", "cta": "Book now"},
		Crop:        MarketingCrop{X: 0.42, Y: 0.31, Zoom: 1.35},
	}
	posted, err := d.RecordMarketingActivity(MarketingActivity{
		BusinessID: 1, SuggestionID: "1:offer:preserve", Play: "offer",
		Status: "posted", ImageURL: "https://cdn.example.com/activity.jpg",
		Caption: "Activity caption", CreativeSnapshot: want,
	})
	require.NoError(t, err)
	require.NotNil(t, posted.PostedAt)
	oldPostedAt := time.Now().UTC().Add(-31 * 24 * time.Hour).Truncate(time.Second)
	require.NoError(t, d.GetGorm().Model(&MarketingActivity{}).
		Where("id = ?", posted.ID).Update("posted_at", oldPostedAt).Error)

	dismissed, err := d.RecordMarketingActivity(MarketingActivity{
		BusinessID: 1, SuggestionID: "1:offer:preserve", Play: "offer", Status: "dismissed",
		ImageURL: "https://cdn.example.com/activity.jpg", Caption: "Activity caption",
	})
	require.NoError(t, err)
	require.Equal(t, posted.ID, dismissed.ID)
	require.Equal(t, "dismissed", dismissed.Status)
	require.Equal(t, want, dismissed.CreativeSnapshot)
	require.Equal(t, oldPostedAt, dismissed.PostedAt.UTC())
	require.NotNil(t, dismissed.DismissedAt)
	require.Equal(t, "https://cdn.example.com/activity.jpg", dismissed.ImageURL)
	require.Equal(t, "Activity caption", dismissed.Caption)

	restored, err := d.RestoreMarketingActivity(1, "1:offer:preserve")
	require.NoError(t, err)
	require.True(t, restored)

	var restoredRow MarketingActivity
	require.NoError(t, d.GetGorm().First(&restoredRow, posted.ID).Error)
	require.Equal(t, "posted", restoredRow.Status)
	require.Nil(t, restoredRow.DismissedAt)
	require.Equal(t, oldPostedAt, restoredRow.PostedAt.UTC(), "restore must retain the original cooldown timestamp")
	require.Equal(t, want, restoredRow.CreativeSnapshot)
	require.Equal(t, "https://cdn.example.com/activity.jpg", restoredRow.ImageURL)
	require.Equal(t, "Activity caption", restoredRow.Caption)

	restoredAgain, err := d.RestoreMarketingActivity(1, "1:offer:preserve")
	require.NoError(t, err)
	require.False(t, restoredAgain, "a concurrent or repeated second restore is a harmless no-op")

	var count int64
	require.NoError(t, d.GetGorm().Model(&MarketingActivity{}).Count(&count).Error)
	require.Equal(t, int64(1), count)
}

func TestMarketingCreativeSnapshotScanner(t *testing.T) {
	snapshot := MarketingCreativeSnapshot{Caption: "stale"}
	require.NoError(t, snapshot.Scan(nil))
	require.Equal(t, MarketingCreativeSnapshot{}, snapshot, "SQL NULL clears to no snapshot")

	err := snapshot.Scan(int64(1))
	require.ErrorContains(t, err, "cannot scan int64")
}

func TestMarketingCreativeSnapshotScannerAcceptsLegacyJSONWithoutFont(t *testing.T) {
	var snapshot MarketingCreativeSnapshot
	require.NoError(t, snapshot.Scan([]byte(`{"caption":"Legacy","slots":{},"crop":{"x":0.5,"y":0.5,"zoom":1}}`)))
	require.Equal(t, "", snapshot.FontFamily)
}

func TestMarketingCreativeSnapshotScannerRejectsInvalidJSONWithoutMutation(t *testing.T) {
	tests := []struct {
		name  string
		value any
	}{
		{name: "JSON null bytes", value: []byte("null")},
		{name: "JSON null string with whitespace", value: "  \nnull\t"},
		{name: "malformed JSON bytes", value: []byte(`{"caption":"changed"`)},
		{name: "malformed JSON string", value: `{"caption":"changed"`},
		{name: "decode error after valid fields", value: []byte(`{"caption":"changed","crop":{"x":"not-a-number"}}`)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			want := MarketingCreativeSnapshot{
				Caption: "unchanged",
				Slots:   map[string]string{"headline": "keep me"},
				Crop:    MarketingCrop{X: 0.42, Y: 0.31, Zoom: 1.35},
			}
			snapshot := MarketingCreativeSnapshot{
				Caption: "unchanged",
				Slots:   map[string]string{"headline": "keep me"},
				Crop:    MarketingCrop{X: 0.42, Y: 0.31, Zoom: 1.35},
			}

			err := snapshot.Scan(tt.value)

			require.Error(t, err)
			require.Equal(t, want, snapshot, "failed scans must not partially overwrite the receiver")
		})
	}
}

func TestMarketingCreativeSnapshotLegacyJSONIsExplicitNull(t *testing.T) {
	data, err := json.Marshal(MarketingActivity{})
	require.NoError(t, err)
	var activity map[string]any
	require.NoError(t, json.Unmarshal(data, &activity))
	snapshot, ok := activity["creative_snapshot"]
	require.True(t, ok, "legacy JSON must include the nullable snapshot field")
	require.Nil(t, snapshot)
}

func TestMarketingCreativeSnapshotCarriesMotionFields(t *testing.T) {
	snapshot := MarketingCreativeSnapshot{
		Caption:      "Milanesa night",
		ImageURL:     "https://cdn.example.com/a.jpg",
		ImageSource:  "menu",
		Template:     "editorial",
		Aspect:       "4:5",
		Slots:        map[string]string{"dishName": "Milanesa"},
		Crop:         MarketingCrop{X: 0.5, Y: 0.5, Zoom: 1},
		Kit:          "editorial",
		Composition:  "photoBottomStack",
		MediaKind:    "video",
		MotionPreset: "pushIn",
	}

	encoded, err := snapshot.Value()
	require.NoError(t, err)

	var decoded MarketingCreativeSnapshot
	require.NoError(t, decoded.Scan(encoded))
	require.Equal(t, "video", decoded.MediaKind)
	require.Equal(t, "pushIn", decoded.MotionPreset)
	require.Equal(t, snapshot, decoded, "round trip must be lossless")
}

func TestMarketingCreativeSnapshotLegacyJSONHasNoMediaKind(t *testing.T) {
	var snapshot MarketingCreativeSnapshot
	require.NoError(t, snapshot.Scan([]byte(
		`{"caption":"Legacy","image_url":"https://cdn.example.com/a.jpg",`+
			`"image_source":"menu","template":"editorial","aspect":"4:5",`+
			`"slots":{},"crop":{"x":0.5,"y":0.5,"zoom":1}}`)))
	require.Equal(t, "", snapshot.MediaKind, "absent media_kind means a legacy still image")
	require.Equal(t, "", snapshot.MotionPreset)
}

func TestMarketingCreativeSnapshotOmitsEmptyMotionFields(t *testing.T) {
	snapshot := MarketingCreativeSnapshot{
		Caption:     "Still",
		ImageSource: "menu",
		Template:    "editorial",
		Aspect:      "4:5",
		Slots:       map[string]string{},
		Crop:        MarketingCrop{X: 0.5, Y: 0.5, Zoom: 1},
	}
	encoded, err := snapshot.Value()
	require.NoError(t, err)
	require.NotContains(t, string(encoded.([]byte)), "media_kind",
		"a still snapshot must not grow bytes in every stored row")
	require.NotContains(t, string(encoded.([]byte)), "motion_preset")
}

func TestHandledMarketingActivityRestore_DeletesDismissedButNotPosted(t *testing.T) {
	d := newMarketingActivityDB(t)
	_, err := d.RecordMarketingActivity(MarketingActivity{
		BusinessID: 1, SuggestionID: "1:happy_hour:", Play: "happy_hour", Status: "dismissed",
	})
	require.NoError(t, err)
	deleted, err := d.RestoreMarketingActivity(1, "1:happy_hour:")
	require.NoError(t, err)
	require.True(t, deleted, "a dismissed row must be deletable")
	deletedAgain, err := d.RestoreMarketingActivity(1, "1:happy_hour:")
	require.NoError(t, err)
	require.False(t, deletedAgain, "a repeated restore after deletion is harmless")
	var count int64
	require.NoError(t, d.GetGorm().Model(&MarketingActivity{}).Count(&count).Error)
	require.Equal(t, int64(0), count)

	// Posted rows are permanent: restore is a no-op (not deleted).
	_, err = d.RecordMarketingActivity(MarketingActivity{
		BusinessID: 1, SuggestionID: "1:offer:o5", Play: "offer", Status: "posted",
	})
	require.NoError(t, err)
	deleted, err = d.RestoreMarketingActivity(1, "1:offer:o5")
	require.NoError(t, err)
	require.False(t, deleted, "a posted row is permanent and must not be deleted")
	require.NoError(t, d.GetGorm().Model(&MarketingActivity{}).Count(&count).Error)
	require.Equal(t, int64(1), count)
}

func TestListMarketingActivities_FilterAndPaginate(t *testing.T) {
	d := newMarketingActivityDB(t)
	base := time.Date(2026, 7, 1, 12, 0, 0, 0, time.UTC)
	for i := 0; i < 5; i++ {
		status := "posted"
		if i%2 == 1 {
			status = "dismissed"
		}
		row := MarketingActivity{
			BusinessID: 1, SuggestionID: "1:offer:o" + string(rune('0'+i)), Play: "offer",
			Status: status, CreatedAt: base.Add(time.Duration(i) * time.Hour),
		}
		require.NoError(t, d.GetGorm().Create(&row).Error)
	}
	// Newest-first, no filter, page 1 size 2.
	page, total, err := d.ListMarketingActivities(MarketingActivityQuery{
		BusinessID: 1, Page: 1, PerPage: 2,
	})
	require.NoError(t, err)
	require.Equal(t, int64(5), total)
	require.Len(t, page, 2)
	require.True(t, page[0].CreatedAt.After(page[1].CreatedAt), "newest first")

	// Status filter.
	posted, total, err := d.ListMarketingActivities(MarketingActivityQuery{
		BusinessID: 1, Status: "posted", Page: 1, PerPage: 50,
	})
	require.NoError(t, err)
	require.Equal(t, int64(3), total)
	require.Len(t, posted, 3)
	for _, r := range posted {
		require.Equal(t, "posted", r.Status)
	}
}

func TestListMarketingActivities_PerPageClamped(t *testing.T) {
	d := newMarketingActivityDB(t)
	_ = d
	q := MarketingActivityQuery{BusinessID: 1, Page: 0, PerPage: 9999}.Normalize()
	require.Equal(t, 1, q.Page, "page floors at 1")
	require.Equal(t, marketingActivityMaxPerPage, q.PerPage, "per_page clamps to max")
}

func TestListMarketingActivities_HandledFromUsesLifecycleTimestampAndTotal(t *testing.T) {
	d := newMarketingActivityDB(t)
	now := time.Date(2026, 7, 15, 12, 0, 0, 0, time.UTC)
	cutoff := now.Add(-30 * 24 * time.Hour)
	recent := now.Add(-time.Hour)
	old := now.Add(-31 * 24 * time.Hour)
	createdOld := now.Add(-90 * 24 * time.Hour)
	rows := []MarketingActivity{
		{BusinessID: 1, SuggestionID: "recent-post", Status: "posted", CreatedAt: createdOld, UpdatedAt: recent, PostedAt: &recent},
		{BusinessID: 1, SuggestionID: "recent-dismiss", Status: "dismissed", CreatedAt: createdOld, UpdatedAt: recent, DismissedAt: &recent},
		{BusinessID: 1, SuggestionID: "legacy-post", Status: "posted", CreatedAt: createdOld, UpdatedAt: recent},
		{BusinessID: 1, SuggestionID: "legacy-dismiss", Status: "dismissed", CreatedAt: createdOld, UpdatedAt: recent},
		{BusinessID: 1, SuggestionID: "effective-transition", Status: "posted", CreatedAt: createdOld, UpdatedAt: recent, PostedAt: &old},
		{BusinessID: 1, SuggestionID: "old-post", Status: "posted", CreatedAt: now, UpdatedAt: old, PostedAt: &old},
		{BusinessID: 1, SuggestionID: "old-dismiss", Status: "dismissed", CreatedAt: now, UpdatedAt: old, DismissedAt: &old},
		{BusinessID: 1, SuggestionID: "pending", Status: "pending", CreatedAt: now, UpdatedAt: recent},
		{BusinessID: 2, SuggestionID: "other-business", Status: "posted", CreatedAt: createdOld, UpdatedAt: recent, PostedAt: &recent},
	}
	require.NoError(t, d.GetGorm().Create(&rows).Error)

	page, total, err := d.ListMarketingActivities(MarketingActivityQuery{
		BusinessID: 1, HandledFrom: &cutoff, Page: 1, PerPage: 1,
	})
	require.NoError(t, err)
	require.Equal(t, int64(5), total, "total must count every recently handled row, including effective transitions, not only the page")
	require.Len(t, page, 1)

	createdFrom := now.Add(-24 * time.Hour)
	createdPage, createdTotal, err := d.ListMarketingActivities(MarketingActivityQuery{
		BusinessID: 1, From: &createdFrom, Page: 1, PerPage: 50,
	})
	require.NoError(t, err)
	require.Equal(t, int64(3), createdTotal, "from must retain its original created_at meaning")
	require.Len(t, createdPage, 3)
}

func TestHandledMarketingActivities_ReturnsDismissedAndRecentPostedOnly(t *testing.T) {
	d := newMarketingActivityDB(t)
	now := time.Date(2026, 7, 14, 12, 0, 0, 0, time.UTC)
	recent := now.Add(-30*24*time.Hour + time.Second)
	expired := now.Add(-30 * 24 * time.Hour)

	require.NoError(t, d.GetGorm().Create(&MarketingActivity{
		BusinessID: 1, SuggestionID: "1:offer:dismissed", Play: "offer", Status: "dismissed",
	}).Error)
	require.NoError(t, d.GetGorm().Create(&MarketingActivity{
		BusinessID: 1, SuggestionID: "1:offer:recent", Play: "offer", Status: "posted", PostedAt: &recent,
	}).Error)
	require.NoError(t, d.GetGorm().Create(&MarketingActivity{
		BusinessID: 1, SuggestionID: "1:offer:expired", Play: "offer", Status: "posted", PostedAt: &expired,
	}).Error)

	rows, err := d.HandledMarketingActivities(1, []string{
		"1:offer:dismissed", "1:offer:recent", "1:offer:expired",
	}, now.Add(-30*24*time.Hour))
	require.NoError(t, err)
	require.ElementsMatch(t, []HandledMarketingActivity{
		{SuggestionID: "1:offer:dismissed", Status: "dismissed"},
		{SuggestionID: "1:offer:recent", Status: "posted", PostedAt: &recent},
	}, rows, "dismissals are durable and only posts inside the cooldown are handled")
}

func TestHandledMarketingActivities_IsBoundedToCandidateIDs(t *testing.T) {
	d := newMarketingActivityDB(t)
	require.NoError(t, d.GetGorm().Create(&MarketingActivity{
		BusinessID: 1, SuggestionID: "1:offer:requested", Play: "offer", Status: "dismissed",
	}).Error)
	require.NoError(t, d.GetGorm().Create(&MarketingActivity{
		BusinessID: 1, SuggestionID: "1:offer:unrelated", Play: "offer", Status: "dismissed",
	}).Error)

	rows, err := d.HandledMarketingActivities(1, []string{"1:offer:requested"}, time.Date(2026, 6, 14, 12, 0, 0, 0, time.UTC))
	require.NoError(t, err)
	require.Equal(t, []HandledMarketingActivity{{SuggestionID: "1:offer:requested", Status: "dismissed"}}, rows)
}

// S3-Loop: Director closed-loop reads recent mark_posted rows (count, titles,
// optional freeform channel). Not a schedule — only historical posted_at.
func TestRecentMarketingPosts_CountsPeriodAndSurfacesChannels(t *testing.T) {
	d := newMarketingActivityDB(t)
	now := time.Date(2026, 7, 26, 12, 0, 0, 0, time.UTC)
	recent := now.Add(-2 * 24 * time.Hour)
	old := now.Add(-30 * 24 * time.Hour)
	since := now.Add(-7 * 24 * time.Hour)

	require.NoError(t, d.GetGorm().Create(&MarketingActivity{
		BusinessID: 1, SuggestionID: "1:featured:a", Play: "featured_dish",
		Title: "Feature ribeye", Status: "posted", PostedAt: &recent,
		CreativeSnapshot: &MarketingCreativeSnapshot{
			Caption: "x", ImageURL: "https://cdn.example.com/a.jpg",
			ImageSource: "menu", Template: "bold", Aspect: "1:1",
			PostedChannel: "Instagram Stories",
		},
	}).Error)
	require.NoError(t, d.GetGorm().Create(&MarketingActivity{
		BusinessID: 1, SuggestionID: "1:offer:b", Play: "offer",
		Title: "Happy hour", Status: "posted", PostedAt: &recent,
		CreativeSnapshot: &MarketingCreativeSnapshot{
			Caption: "y", ImageURL: "https://cdn.example.com/b.jpg",
			ImageSource: "offer", Template: "minimal", Aspect: "4:5",
			DestinationID: "ig_feed",
		},
	}).Error)
	require.NoError(t, d.GetGorm().Create(&MarketingActivity{
		BusinessID: 1, SuggestionID: "1:offer:old", Play: "offer",
		Title: "Stale", Status: "posted", PostedAt: &old,
	}).Error)
	require.NoError(t, d.GetGorm().Create(&MarketingActivity{
		BusinessID: 1, SuggestionID: "1:offer:dismissed", Play: "offer",
		Title: "Hidden", Status: "dismissed",
	}).Error)
	// Other business must not leak.
	require.NoError(t, d.GetGorm().Create(&Business{ID: 2, BusinessId: "biz-2"}).Error)
	require.NoError(t, d.GetGorm().Create(&MarketingActivity{
		BusinessID: 2, SuggestionID: "2:offer:x", Play: "offer",
		Title: "Other", Status: "posted", PostedAt: &recent,
	}).Error)

	period, err := d.RecentMarketingPosts(1, since, 5)
	require.NoError(t, err)
	require.Equal(t, int64(2), period.Count)
	require.Len(t, period.Recent, 2)
	require.ElementsMatch(t, []string{"Feature ribeye", "Happy hour"}, period.RecentTitles)
	// Freeform channel preferred; destination_id fills when freeform absent.
	require.Contains(t, period.Channels, "Instagram Stories")
	require.Contains(t, period.Channels, "ig_feed")
}

func TestRecentMarketingPosts_EmptyWhenNonePosted(t *testing.T) {
	d := newMarketingActivityDB(t)
	period, err := d.RecentMarketingPosts(1, time.Now().UTC().AddDate(0, 0, -7), 5)
	require.NoError(t, err)
	require.Equal(t, int64(0), period.Count)
	require.Empty(t, period.Recent)
	require.Empty(t, period.RecentTitles)
	require.Empty(t, period.Channels)
}
