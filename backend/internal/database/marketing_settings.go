package database

import (
	"encoding/json"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/stdevmac/payverge/backend/internal/locales"
)

// knownMarketingPlays is the canonical set of play keys the automation settings
// may disable. Kept in sync with internal/services/marketing/suggestion.go's
// Play constants. Duplicated here (not imported) to avoid a database->services
// import cycle.
var knownMarketingPlays = map[string]struct{}{
	"happy_hour":    {},
	"featured_dish": {},
	"move_item":     {},
	"win_back":      {},
	"combo_deal":    {},
	"offer":         {},
}

var (
	knownMarketingVisualMoods = map[string]struct{}{
		"natural": {}, "bright": {}, "moody": {}, "editorial": {}, "rustic": {},
	}
	knownMarketingCTAStyles = map[string]struct{}{
		"soft": {}, "direct": {}, "urgent": {},
	}
	knownMarketingHashtagBehaviors = map[string]struct{}{
		"none": {}, "light": {}, "standard": {},
	}
	knownMarketingTones = map[string]struct{}{
		"warm": {}, "playful": {}, "elegant": {}, "punchy": {},
	}
)

const (
	maxMarketingAvoidPhrases      = 10
	maxMarketingAvoidPhraseLength = 60
	// Audience and voice feed directly into generation prompts, so each is
	// bounded to 200 Unicode characters after surrounding whitespace is trimmed.
	maxMarketingAudienceLength = 200
	maxMarketingVoiceLength    = 200
)

// MarketingCreativeProfile holds optional creative overrides. Empty values
// deliberately remain empty so callers can derive defaults from the business.
type MarketingCreativeProfile struct {
	Audience        string   `json:"audience"`
	Voice           string   `json:"voice"`
	VisualMood      string   `json:"visual_mood"`
	CTAStyle        string   `json:"cta_style"`
	HashtagBehavior string   `json:"hashtag_behavior"`
	AvoidPhrases    []string `json:"avoid_phrases"`
	DefaultLanguage string   `json:"default_language"`
	DefaultTone     string   `json:"default_tone"`
}

// MarketingSettings is the shaped view of businesses.marketing_settings.
type MarketingSettings struct {
	Enabled         bool                     `json:"enabled"`
	DisabledPlays   []string                 `json:"disabled_plays"`
	CreativeProfile MarketingCreativeProfile `json:"creative_profile"`
}

// DefaultMarketingSettings returns the enabled-with-nothing-disabled default,
// used when the column is empty/NULL (older rows, or a race before backfill).
func DefaultMarketingSettings() MarketingSettings {
	return MarketingSettings{
		Enabled:         true,
		DisabledPlays:   []string{},
		CreativeProfile: MarketingCreativeProfile{AvoidPhrases: []string{}},
	}
}

// Sanitized returns a copy with disabled_plays trimmed of empties and deduped,
// preserving first-seen order.
func (s MarketingSettings) Sanitized() MarketingSettings {
	seen := make(map[string]struct{}, len(s.DisabledPlays))
	out := make([]string, 0, len(s.DisabledPlays))
	for _, p := range s.DisabledPlays {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if _, dup := seen[p]; dup {
			continue
		}
		seen[p] = struct{}{}
		out = append(out, p)
	}
	s.DisabledPlays = out
	s.CreativeProfile = s.CreativeProfile.sanitized()
	return s
}

func (p MarketingCreativeProfile) sanitized() MarketingCreativeProfile {
	p.Audience = strings.TrimSpace(p.Audience)
	p.Voice = strings.TrimSpace(p.Voice)
	p.VisualMood = strings.TrimSpace(p.VisualMood)
	p.CTAStyle = strings.TrimSpace(p.CTAStyle)
	p.HashtagBehavior = strings.TrimSpace(p.HashtagBehavior)
	p.DefaultLanguage = strings.TrimSpace(p.DefaultLanguage)
	p.DefaultTone = strings.TrimSpace(p.DefaultTone)

	phrases := make([]string, 0, len(p.AvoidPhrases))
	for _, phrase := range p.AvoidPhrases {
		phrase = strings.TrimSpace(phrase)
		if phrase == "" || containsFold(phrases, phrase) {
			continue
		}
		phrases = append(phrases, phrase)
	}
	p.AvoidPhrases = phrases
	return p
}

func containsFold(values []string, candidate string) bool {
	for _, value := range values {
		if strings.EqualFold(value, candidate) {
			return true
		}
	}
	return false
}

// ValidateMarketingSettings rejects any disabled play that is not a known play.
func ValidateMarketingSettings(s MarketingSettings) error {
	for _, p := range s.DisabledPlays {
		if p == "" {
			continue
		}
		if _, ok := knownMarketingPlays[p]; !ok {
			return fmt.Errorf("unknown marketing play %q", p)
		}
	}
	return ValidateMarketingCreativeProfile(s.CreativeProfile)
}

// ValidateMarketingCreativeProfile enforces the bounded creative choices and
// phrase limits. All profile fields are optional.
func ValidateMarketingCreativeProfile(p MarketingCreativeProfile) error {
	if utf8.RuneCountInString(strings.TrimSpace(p.Audience)) > maxMarketingAudienceLength {
		return fmt.Errorf("marketing audience must contain at most %d characters", maxMarketingAudienceLength)
	}
	if utf8.RuneCountInString(strings.TrimSpace(p.Voice)) > maxMarketingVoiceLength {
		return fmt.Errorf("marketing voice must contain at most %d characters", maxMarketingVoiceLength)
	}
	if err := validateMarketingChoice("mood", p.VisualMood, knownMarketingVisualMoods); err != nil {
		return err
	}
	if err := validateMarketingChoice("cta", p.CTAStyle, knownMarketingCTAStyles); err != nil {
		return err
	}
	if err := validateMarketingChoice("hashtags", p.HashtagBehavior, knownMarketingHashtagBehaviors); err != nil {
		return err
	}
	if err := validateMarketingChoice("tone", p.DefaultTone, knownMarketingTones); err != nil {
		return err
	}
	if p.DefaultLanguage != "" && !locales.IsGuestLocale(p.DefaultLanguage) {
		return fmt.Errorf("unknown marketing language %q", p.DefaultLanguage)
	}
	if len(p.AvoidPhrases) > maxMarketingAvoidPhrases {
		return fmt.Errorf("marketing avoid phrases must contain at most %d entries", maxMarketingAvoidPhrases)
	}
	for _, phrase := range p.AvoidPhrases {
		if utf8.RuneCountInString(phrase) > maxMarketingAvoidPhraseLength {
			return fmt.Errorf("marketing avoid phrase must contain at most %d characters", maxMarketingAvoidPhraseLength)
		}
	}
	return nil
}

func validateMarketingChoice(field, value string, allowed map[string]struct{}) error {
	if value == "" {
		return nil
	}
	if _, ok := allowed[value]; !ok {
		return fmt.Errorf("unknown marketing %s %q", field, value)
	}
	return nil
}

// GetMarketingSettings reads and shapes the business's settings blob. A blank
// or unparseable blob yields the enabled default (never an error) so the engine
// and UI degrade to "on".
func (d *DB) GetMarketingSettings(businessID uint) (MarketingSettings, error) {
	var raw struct {
		MarketingSettings JSONRawMessage `gorm:"column:marketing_settings"`
	}
	if err := d.GetGorm().Model(&Business{}).
		Select("marketing_settings").
		Where("id = ?", businessID).
		Take(&raw).Error; err != nil {
		return DefaultMarketingSettings(), err
	}
	if len(raw.MarketingSettings) == 0 {
		return DefaultMarketingSettings(), nil
	}
	var s MarketingSettings
	if err := json.Unmarshal(raw.MarketingSettings, &s); err != nil {
		return DefaultMarketingSettings(), nil
	}
	if s.DisabledPlays == nil {
		s.DisabledPlays = []string{}
	}
	if s.CreativeProfile.AvoidPhrases == nil {
		s.CreativeProfile.AvoidPhrases = []string{}
	}
	return s, nil
}

// UpdateMarketingSettings validates, sanitizes, and persists the settings blob,
// returning the stored value. Callers should have already validated at the
// handler boundary; this re-validates as a safety net.
func (d *DB) UpdateMarketingSettings(businessID uint, s MarketingSettings) (MarketingSettings, error) {
	s = s.Sanitized()
	if err := ValidateMarketingSettings(s); err != nil {
		return MarketingSettings{}, err
	}
	blob, err := json.Marshal(s)
	if err != nil {
		return MarketingSettings{}, err
	}
	if err := d.GetGorm().Model(&Business{}).
		Where("id = ?", businessID).
		Update("marketing_settings", JSONRawMessage(blob)).Error; err != nil {
		return MarketingSettings{}, err
	}
	return s, nil
}
