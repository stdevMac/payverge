package database

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func newMarketingSettingsDB(t *testing.T) *DB {
	t.Helper()
	prev := db
	t.Cleanup(func() { SetTestDB(prev) })
	dsn := "file:" + t.Name() + "?mode=memory&cache=shared"
	gormDB, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := gormDB.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, gormDB.AutoMigrate(&Business{}))
	SetTestDB(gormDB)
	// Seed a business with the default settings blob.
	require.NoError(t, db.Create(&Business{
		ID: 1, BusinessId: "biz-1",
		MarketingSettings: JSONRawMessage(`{"enabled": true, "disabled_plays": []}`),
	}).Error)
	return GetDBWrapper()
}

func TestGetMarketingSettings_DefaultsWhenBlank(t *testing.T) {
	d := newMarketingSettingsDB(t)
	// A business whose column is empty/NULL still yields the enabled default.
	require.NoError(t, db.Create(&Business{ID: 2, BusinessId: "biz-2"}).Error)
	s, err := d.GetMarketingSettings(2)
	require.NoError(t, err)
	require.True(t, s.Enabled)
	require.Empty(t, s.DisabledPlays)
}

func TestUpdateMarketingSettings_PersistsAndValidates(t *testing.T) {
	d := newMarketingSettingsDB(t)
	updated, err := d.UpdateMarketingSettings(1, MarketingSettings{
		Enabled: false, DisabledPlays: []string{"happy_hour", "offer"},
	})
	require.NoError(t, err)
	require.False(t, updated.Enabled)
	require.ElementsMatch(t, []string{"happy_hour", "offer"}, updated.DisabledPlays)

	got, err := d.GetMarketingSettings(1)
	require.NoError(t, err)
	require.False(t, got.Enabled)
	require.ElementsMatch(t, []string{"happy_hour", "offer"}, got.DisabledPlays)
}

func TestValidateMarketingSettings_RejectsUnknownPlay(t *testing.T) {
	err := ValidateMarketingSettings(MarketingSettings{
		Enabled: true, DisabledPlays: []string{"happy_hour", "not_a_play"},
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "not_a_play")

	require.NoError(t, ValidateMarketingSettings(MarketingSettings{
		Enabled: true,
		DisabledPlays: []string{
			"happy_hour", "featured_dish", "move_item", "win_back", "combo_deal", "offer",
		},
	}))
}

func TestValidateMarketingSettings_DedupesAndDropsEmpty(t *testing.T) {
	s := MarketingSettings{Enabled: true, DisabledPlays: []string{"offer", "offer", "", "happy_hour"}}
	cleaned := s.Sanitized()
	require.ElementsMatch(t, []string{"offer", "happy_hour"}, cleaned.DisabledPlays)
}

func TestMarketingSettingsCreativeProfile_OptionalAndSanitized(t *testing.T) {
	empty := DefaultMarketingSettings()
	require.NoError(t, ValidateMarketingSettings(empty))
	require.Equal(t, MarketingCreativeProfile{AvoidPhrases: []string{}}, empty.CreativeProfile)

	settings := MarketingSettings{
		Enabled:       true,
		DisabledPlays: []string{" offer ", "offer", ""},
		CreativeProfile: MarketingCreativeProfile{
			Audience:        "  neighborhood regulars  ",
			Voice:           "  friendly and concise  ",
			VisualMood:      " editorial ",
			CTAStyle:        " soft ",
			HashtagBehavior: " light ",
			AvoidPhrases:    []string{"  Best in town  ", "best IN TOWN", "", "  guaranteed  "},
			DefaultLanguage: " es-AR ",
			DefaultTone:     " warm ",
		},
	}.Sanitized()

	require.Equal(t, []string{"offer"}, settings.DisabledPlays)
	require.Equal(t, "neighborhood regulars", settings.CreativeProfile.Audience)
	require.Equal(t, "friendly and concise", settings.CreativeProfile.Voice)
	require.Equal(t, "editorial", settings.CreativeProfile.VisualMood)
	require.Equal(t, "soft", settings.CreativeProfile.CTAStyle)
	require.Equal(t, "light", settings.CreativeProfile.HashtagBehavior)
	require.Equal(t, []string{"Best in town", "guaranteed"}, settings.CreativeProfile.AvoidPhrases)
	require.Equal(t, "es-AR", settings.CreativeProfile.DefaultLanguage)
	require.Equal(t, "warm", settings.CreativeProfile.DefaultTone)
	require.NoError(t, ValidateMarketingSettings(settings))
}

func TestMarketingSettingsCreativeProfile_ValidatesBoundedEnumsAndLanguage(t *testing.T) {
	validValues := []MarketingCreativeProfile{
		{},
		{VisualMood: "natural", CTAStyle: "soft", HashtagBehavior: "none", DefaultTone: "warm", DefaultLanguage: "en"},
		{VisualMood: "bright", CTAStyle: "direct", HashtagBehavior: "light", DefaultTone: "playful", DefaultLanguage: "es"},
		{VisualMood: "moody", CTAStyle: "urgent", HashtagBehavior: "standard", DefaultTone: "elegant", DefaultLanguage: "es-AR"},
		{VisualMood: "editorial", DefaultTone: "punchy", DefaultLanguage: "fr"},
		{VisualMood: "rustic"},
	}
	for _, profile := range validValues {
		require.NoError(t, ValidateMarketingSettings(MarketingSettings{CreativeProfile: profile}), "%+v", profile)
	}

	invalidValues := map[string]MarketingCreativeProfile{
		"mood":     {VisualMood: "cinematic"},
		"cta":      {CTAStyle: "pushy"},
		"hashtags": {HashtagBehavior: "heavy"},
		"tone":     {DefaultTone: "formal"},
		"language": {DefaultLanguage: "xx-not-real"},
	}
	for field, profile := range invalidValues {
		t.Run(field, func(t *testing.T) {
			err := ValidateMarketingSettings(MarketingSettings{CreativeProfile: profile})
			require.Error(t, err)
			require.Contains(t, err.Error(), field)
		})
	}
}

func TestMarketingSettingsCreativeProfile_ValidatesAvoidPhraseLimits(t *testing.T) {
	sixtyCharacters := strings.Repeat("é", 60)
	tenPhrases := make([]string, 10)
	for i := range tenPhrases {
		tenPhrases[i] = fmt.Sprintf("phrase %d", i)
	}
	require.NoError(t, ValidateMarketingSettings(MarketingSettings{
		CreativeProfile: MarketingCreativeProfile{AvoidPhrases: append(append([]string{}, tenPhrases[:9]...), sixtyCharacters)},
	}.Sanitized()), "ten phrases of up to 60 Unicode characters are allowed")

	elevenPhrases := append(append([]string{}, tenPhrases...), "phrase 10")
	err := ValidateMarketingSettings(MarketingSettings{
		CreativeProfile: MarketingCreativeProfile{AvoidPhrases: elevenPhrases},
	}.Sanitized())
	require.Error(t, err)
	require.Contains(t, err.Error(), "10")

	err = ValidateMarketingSettings(MarketingSettings{
		CreativeProfile: MarketingCreativeProfile{AvoidPhrases: []string{strings.Repeat("é", 61)}},
	}.Sanitized())
	require.Error(t, err)
	require.Contains(t, err.Error(), "60")
}

func TestMarketingSettingsCreativeProfile_ValidatesAudienceAndVoiceRuneLimits(t *testing.T) {
	twoHundredCharacters := strings.Repeat("é", 200)
	twoHundredOneCharacters := strings.Repeat("é", 201)

	for _, field := range []string{"audience", "voice"} {
		t.Run(field+" accepts 200 Unicode characters after trimming", func(t *testing.T) {
			profile := MarketingCreativeProfile{}
			if field == "audience" {
				profile.Audience = "  " + twoHundredCharacters + "  "
			} else {
				profile.Voice = "  " + twoHundredCharacters + "  "
			}

			require.NoError(t, ValidateMarketingSettings(MarketingSettings{
				CreativeProfile: profile,
			}.Sanitized()))
		})

		t.Run(field+" rejects 201 Unicode characters after trimming", func(t *testing.T) {
			profile := MarketingCreativeProfile{}
			if field == "audience" {
				profile.Audience = "  " + twoHundredOneCharacters + "  "
			} else {
				profile.Voice = "  " + twoHundredOneCharacters + "  "
			}

			err := ValidateMarketingSettings(MarketingSettings{
				CreativeProfile: profile,
			}.Sanitized())
			require.Error(t, err)
			require.Contains(t, err.Error(), field)
			require.Contains(t, err.Error(), "200")
		})
	}
}

func TestMarketingSettingsCreativeProfile_PersistsRoundTrip(t *testing.T) {
	d := newMarketingSettingsDB(t)
	want := MarketingCreativeProfile{
		Audience:        "late-night diners",
		Voice:           "confident",
		VisualMood:      "moody",
		CTAStyle:        "direct",
		HashtagBehavior: "standard",
		AvoidPhrases:    []string{"best ever", "guaranteed"},
		DefaultLanguage: "es-AR",
		DefaultTone:     "punchy",
	}

	stored, err := d.UpdateMarketingSettings(1, MarketingSettings{
		Enabled: true, CreativeProfile: want,
	})
	require.NoError(t, err)
	require.Equal(t, want, stored.CreativeProfile)

	got, err := d.GetMarketingSettings(1)
	require.NoError(t, err)
	require.Equal(t, want, got.CreativeProfile)
}
