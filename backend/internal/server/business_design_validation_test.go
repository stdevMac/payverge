package server

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/stdevmac/payverge/backend/internal/database"
)

func validSettings() database.BusinessDesignSettings {
	return database.BusinessDesignSettings{
		PrimaryColor:      "#1a6b6a",
		SecondaryColor:    "#2a8b8a",
		FontFamily:        "Inter",
		Theme:             "light",
		MenuLayout:        "grid",
		HeaderStyle:       "banner",
		CornerRadius:      "medium",
		ShadowIntensity:   "subtle",
		BackgroundPattern: "none",
		PatternOpacity:    0.1,
		HeroLayout:        "centered",
		SectionDensity:    "comfortable",
	}
}

func TestValidateBusinessDesignSettings(t *testing.T) {
	t.Run("accepts a fully valid payload", func(t *testing.T) {
		s := validSettings()
		applyDesignSettingsDefaults(&s)
		assert.NoError(t, validateBusinessDesignSettings(&s))
	})

	t.Run("rejects CSS injection via primary_color", func(t *testing.T) {
		s := validSettings()
		s.PrimaryColor = "#fff; } body { display:none } :root { --x: "
		assert.Error(t, validateBusinessDesignSettings(&s))
	})

	t.Run("rejects unknown enum values", func(t *testing.T) {
		s := validSettings()
		s.BackgroundPattern = "<script>"
		assert.Error(t, validateBusinessDesignSettings(&s))
	})

	t.Run("rejects out-of-range opacity", func(t *testing.T) {
		s := validSettings()
		s.PatternOpacity = 4
		assert.Error(t, validateBusinessDesignSettings(&s))
	})

	t.Run("empty strings are defaulted, then valid", func(t *testing.T) {
		s := database.BusinessDesignSettings{}
		applyDesignSettingsDefaults(&s)
		assert.NoError(t, validateBusinessDesignSettings(&s))
	})
}
