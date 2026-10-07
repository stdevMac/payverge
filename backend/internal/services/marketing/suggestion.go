// Package marketing turns a business's own sales/analytics signals into ranked
// campaign suggestions for the Marketing tab. It composes existing, already
// optimized reads (analytics, foodcost, menuengineering) and maps them into
// suggestions via pure functions — it issues no new aggregate queries of its own
// beyond bounded, single-shot reads.
package marketing

// Play is a marketing technique the engine can suggest.
type Play string

const (
	PlayHappyHour    Play = "happy_hour"
	PlayFeaturedDish Play = "featured_dish"
	PlayMoveItem     Play = "move_item"
	PlayWinBack      Play = "win_back"
	PlayComboDeal    Play = "combo_deal"
	PlayOffer        Play = "offer"
)

// Image source tags tell the frontend where a card's free image came from
// (so it can label it "Your photo" / "Your offer" / "Your combo").
const (
	ImageSourceMenu   = "menu"
	ImageSourceOffer  = "offer"
	ImageSourceBundle = "bundle"
)

// RankingVersionS2 tags suggestion payloads built with Season 2 signal-quality
// ranking (why_factors, availability suppress, weak-play gates). Absent or
// empty RankingVersion is treated as pre-S2 by clients.
const RankingVersionS2 = "s2"

// WhyFactor is one structured explainability atom for ranking (S2 why-this-post).
// Key is an i18n suffix under marketingDashboard.why.factors.*; Value is already
// formatted operator-facing text; Weight is the factor's contribution to Rank
// when known (optional, additive for display only).
type WhyFactor struct {
	Key    string  `json:"key"`
	Value  string  `json:"value"`
	Weight float64 `json:"weight,omitempty"`
}

// CampaignSuggestion is one ranked, operator-facing recommendation.
type CampaignSuggestion struct {
	ID   string `json:"id"`
	Play Play   `json:"play"`
	// PlayKey is a stable localization key (matches Play string). Title is the
	// English title stored on activity rows; the FE localizes from play_key.
	PlayKey string `json:"play_key,omitempty"`
	// DaypartKey is the localized daypart label (operator language) for
	// happy_hour (e.g. "Tue 5–7pm" / "Lun 17–18h") so the FE can interpolate
	// it into a localized title shell.
	DaypartKey string `json:"daypart_key,omitempty"`
	Title      string `json:"title"`
	// WhyData is the single-string explanation fed to caption prompts and
	// shown when WhyFactors is empty.
	WhyData string `json:"why_data"`
	// WhyFactors is the structured S2 explainability payload (additive).
	WhyFactors []WhyFactor `json:"why_factors,omitempty"`
	// RankingVersion identifies the ranker that produced Rank / WhyFactors.
	RankingVersion string         `json:"ranking_version,omitempty"`
	Source         string         `json:"source"`
	TargetItemID   string         `json:"target_item_id,omitempty"`
	TargetName     string         `json:"target_name,omitempty"`
	CopyAngle      string         `json:"copy_angle"`
	Metrics        map[string]any `json:"metrics,omitempty"`
	Rank           float64        `json:"rank"`

	// Visual + offer enrichment (v1.1 ready-to-post gallery). A blank
	// ImageURL means "no free image found" — the frontend brand-fills.
	ImageURL      string  `json:"image_url,omitempty"`      // menu/offer/bundle image, else ""
	ImageSource   string  `json:"image_source,omitempty"`   // "menu" | "offer" | "bundle" | ""
	DiscountType  string  `json:"discount_type,omitempty"`  // offer play only: "percentage" | "fixed"
	DiscountValue float64 `json:"discount_value,omitempty"` // offer play only

	// Menu enrichment for AI image generation and richer copy.
	TargetDescription string `json:"target_description,omitempty"`

	// GuestURL is an optional public storefront deep link (S2-D). Absent when
	// the business has no live custom_url page. Never a signed private asset.
	GuestURL string `json:"guest_url,omitempty"`
	// GuestURLKind is "business" | "menu" | "reservations" when GuestURL is set.
	GuestURLKind string `json:"guest_url_kind,omitempty"`
}
