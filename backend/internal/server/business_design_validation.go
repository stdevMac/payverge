package server

import (
	"fmt"
	"regexp"

	"github.com/stdevmac/payverge/backend/internal/database"
)

// The public page interpolates these values into a <style> tag and CSS
// custom properties, so anything beyond a strict hex color or a known enum
// member is a CSS-injection vector on a public, unauthenticated page.
var hexColorPattern = regexp.MustCompile(`^#(?:[0-9a-fA-F]{3}|[0-9a-fA-F]{6})$`)

var designEnumAllowed = map[string][]string{
	"font_family":        {"Inter", "Sans", "Serif"},
	"theme":              {"light", "dark"},
	"menu_layout":        {"grid", "list"},
	"header_style":       {"banner", "minimal"},
	"corner_radius":      {"none", "small", "medium", "large"},
	"shadow_intensity":   {"none", "subtle", "medium", "strong"},
	"background_pattern": {"none", "stripes", "dots", "grid", "waves", "mandala", "geometric", "hexagon", "circles"},
	"hero_layout":        {"centered", "split-left", "split-right"},
	"section_density":    {"compact", "comfortable"},
}

// applyDesignSettingsDefaults maps empty strings to the model defaults so a
// partial save validates.
func applyDesignSettingsDefaults(s *database.BusinessDesignSettings) {
	if s.PrimaryColor == "" {
		s.PrimaryColor = "#1f2937"
	}
	if s.SecondaryColor == "" {
		s.SecondaryColor = "#3b82f6"
	}
	if s.FontFamily == "" {
		s.FontFamily = "Inter"
	}
	if s.Theme == "" {
		s.Theme = "light"
	}
	if s.MenuLayout == "" {
		s.MenuLayout = "grid"
	}
	if s.HeaderStyle == "" {
		s.HeaderStyle = "banner"
	}
	if s.CornerRadius == "" {
		s.CornerRadius = "medium"
	}
	if s.ShadowIntensity == "" {
		s.ShadowIntensity = "subtle"
	}
	if s.BackgroundPattern == "" {
		s.BackgroundPattern = "none"
	}
	if s.HeroLayout == "" {
		s.HeroLayout = "centered"
	}
	if s.SectionDensity == "" {
		s.SectionDensity = "comfortable"
	}
}

func validateBusinessDesignSettings(s *database.BusinessDesignSettings) error {
	if !hexColorPattern.MatchString(s.PrimaryColor) {
		return fmt.Errorf("primary_color must be a hex color like #1a6b6a")
	}
	if !hexColorPattern.MatchString(s.SecondaryColor) {
		return fmt.Errorf("secondary_color must be a hex color like #1a6b6a")
	}
	enumChecks := []struct {
		field string
		value string
	}{
		{"font_family", s.FontFamily},
		{"theme", s.Theme},
		{"menu_layout", s.MenuLayout},
		{"header_style", s.HeaderStyle},
		{"corner_radius", s.CornerRadius},
		{"shadow_intensity", s.ShadowIntensity},
		{"background_pattern", s.BackgroundPattern},
		{"hero_layout", s.HeroLayout},
		{"section_density", s.SectionDensity},
	}
	for _, check := range enumChecks {
		allowed := designEnumAllowed[check.field]
		ok := false
		for _, candidate := range allowed {
			if check.value == candidate {
				ok = true
				break
			}
		}
		if !ok {
			return fmt.Errorf("%s must be one of %v", check.field, allowed)
		}
	}
	if s.PatternOpacity < 0 || s.PatternOpacity > 1 {
		return fmt.Errorf("pattern_opacity must be between 0 and 1")
	}
	return nil
}
