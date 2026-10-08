package director_actions

import "time"

// Kind enumerates the v1 mutation kinds.
type Kind string

const (
	KindAdjustPrices    Kind = "menu.adjust_prices"
	KindSetAvailability Kind = "menu.set_availability"
	KindEditContent     Kind = "menu.edit_content"
)

// Tunable caps (spec §2).
const (
	PriceReconfirmSwingPct = 50.0
	DescriptionMaxRunes    = 500
	ProposalTTL            = 60 * time.Minute
)

// ProposedAction is the server-truth DTO surfaced to the client (assembled from
// the persisted proposal row, never parsed from model free-text).
type ProposedAction struct {
	ID                string        `json:"id"` // proposal public_id
	Kind              Kind          `json:"kind"`
	Title             string        `json:"title"`
	Description       string        `json:"description"`
	Preview           ActionPreview `json:"preview"`
	Warnings          []string      `json:"warnings"`
	MenuVersion       uint          `json:"menu_version"`
	RequiresReconfirm bool          `json:"requires_reconfirm"`
	ExpiresAt         time.Time     `json:"expires_at"`
}

// ActionPreview is the server-computed dry-run diff.
type ActionPreview struct {
	AffectedCount int              `json:"affected_count"`
	Examples      []PreviewExample `json:"examples"`
	Summary       string           `json:"summary"`
}

// PreviewExample is one before/after pair (max 5 surfaced).
type PreviewExample struct {
	Name   string `json:"name"`
	Before any    `json:"before"`
	After  any    `json:"after"`
}

// ItemSnapshot captures the mutable fields of one item for undo support.
// It deliberately excludes Allergens — the AI may never touch them.
type ItemSnapshot struct {
	CategoryID  string   `json:"category_id"`
	ItemID      string   `json:"item_id"`
	Price       float64  `json:"price"`
	IsAvailable bool     `json:"is_available"`
	Description string   `json:"description"`
	DietaryTags []string `json:"dietary_tags"`
}

const maxPreviewExamples = 5
