package marketing

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sort"
	"strings"
	"time"

	"github.com/stdevmac/payverge/backend/internal/analytics"
	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/locales"
	"github.com/stdevmac/payverge/backend/internal/services/menuengineering"
)

// MenuEngineeringFn produces a menu-engineering report for a business/period.
// Wired in main.go by reusing the construction in
// internal/services/director_tools/tool_menu_engineering.go (foodcost report →
// menuengineering.Classify); stubbed in tests.
type MenuEngineeringFn func(businessID uint, period string, loc *time.Location) (menuengineering.Report, error)

// Settings mirrors database.MarketingSettings without importing the shaped
// type across the layer boundary. enabled=false pauses the whole engine.
type Settings struct {
	Enabled       bool
	DisabledPlays []string
}

// SettingsLoaderFn loads a business's automation settings.
type SettingsLoaderFn func(businessID uint) (Settings, error)

// HandledLoaderFn loads durable dismissals and posts still inside their
// cooldown. postedAfter is the exact lower bound for active post cooldowns.
type HandledLoaderFn func(businessID uint, suggestionIDs []string, postedAfter time.Time) ([]database.HandledMarketingActivity, error)

// ClockFn supplies the engine's current time. Production uses UTC wall time;
// tests pin it to exercise cooldown boundaries without sleeping.
type ClockFn func() time.Time

// EmptyReason explains why a successful suggestion batch is empty.
type EmptyReason string

const (
	EmptyReasonNone           EmptyReason = ""
	EmptyReasonPaused         EmptyReason = "paused"
	EmptyReasonNoData         EmptyReason = "no_data"
	EmptyReasonNoEnabledPlays EmptyReason = "no_enabled_plays"
	EmptyReasonAllHandled     EmptyReason = "all_handled"
	postedSuggestionCooldown              = 30 * 24 * time.Hour
)

// SuggestionBatch is the truthful stateful response for the suggestion feed.
type SuggestionBatch struct {
	Suggestions []CampaignSuggestion `json:"suggestions"`
	Paused      bool                 `json:"paused"`
	EmptyReason EmptyReason          `json:"empty_reason"`
	// InventoryBlocked lists dish names we refused to campaign because they
	// are 86'd or recipe-out (issue #200). Honest "blocked by inventory"
	// copy lives on the FE.
	InventoryBlocked []string `json:"inventory_blocked,omitempty"`
}

// Engine builds ranked campaign suggestions from a business's own signals.
type Engine struct {
	db             *database.DB
	analytics      *analytics.AnalyticsService
	menuEng        MenuEngineeringFn
	loadSettings   SettingsLoaderFn
	loadHandled    HandledLoaderFn
	loadUnmakeable UnmakeableLoaderFn
	now            ClockFn
}

// NewEngine constructs the engine. analytics may be nil in tests that don't
// exercise the popular-items path.
func NewEngine(db *database.DB, an *analytics.AnalyticsService, menuEng MenuEngineeringFn) *Engine {
	e := &Engine{db: db, analytics: an, menuEng: menuEng, now: func() time.Time { return time.Now().UTC() }}
	e.loadSettings = func(businessID uint) (Settings, error) {
		s, err := db.GetMarketingSettings(businessID)
		if err != nil {
			return Settings{Enabled: true}, err
		}
		return Settings{Enabled: s.Enabled, DisabledPlays: s.DisabledPlays}, nil
	}
	e.loadHandled = func(businessID uint, suggestionIDs []string, postedAfter time.Time) ([]database.HandledMarketingActivity, error) {
		return db.HandledMarketingActivities(businessID, suggestionIDs, postedAfter)
	}
	return e
}

// SetSettingsLoader overrides the settings seam (tests).
func (e *Engine) SetSettingsLoader(fn SettingsLoaderFn) { e.loadSettings = fn }

// SetHandledLoader overrides the handled-activity seam (tests).
func (e *Engine) SetHandledLoader(fn HandledLoaderFn) { e.loadHandled = fn }

// SetClock overrides the engine clock (tests).
func (e *Engine) SetClock(fn ClockFn) {
	if fn != nil {
		e.now = fn
	}
}

func (e *Engine) currentTime() time.Time {
	if e.now == nil {
		return time.Now().UTC()
	}
	return e.now().UTC()
}

// maxSuggestions bounds the feed so one signal can't crowd it.
const maxSuggestions = 8

// SuggestionBatch gathers signals and maps them to ranked suggestions, after
// applying automation settings and handled-opportunity lifecycle rules.
func (e *Engine) SuggestionBatch(ctx context.Context, businessID uint, loc *time.Location, locale string) (SuggestionBatch, error) {
	const period = "week"
	empty := make([]CampaignSuggestion, 0)

	// 0. Settings FIRST — a paused engine skips ALL signal queries (perf win).
	settings := Settings{Enabled: true}
	if e.loadSettings != nil {
		s, err := e.loadSettings(businessID)
		if err != nil {
			return SuggestionBatch{}, fmt.Errorf("load marketing settings: %w", err)
		}
		settings = s
	}
	if !settings.Enabled {
		return SuggestionBatch{Suggestions: empty, Paused: true, EmptyReason: EmptyReasonPaused}, nil
	}
	disabled := make(map[string]struct{}, len(settings.DisabledPlays))
	for _, p := range settings.DisabledPlays {
		disabled[p] = struct{}{}
	}
	if allMarketingPlaysDisabled(disabled) {
		return SuggestionBatch{Suggestions: empty, EmptyReason: EmptyReasonNoEnabledPlays}, nil
	}

	// 1. Menu-engineering report (margin/popularity quadrants) via the seam.
	var menuRep menuengineering.Report
	if e.menuEng != nil {
		r, err := e.menuEng(businessID, period, loc)
		if err != nil {
			return SuggestionBatch{}, fmt.Errorf("load menu engineering signals: %w", err)
		}
		menuRep = r
	}

	// 2. Popular items (for naming a hero in happy-hour / win-back).
	var popular []analytics.ItemStats
	if e.analytics != nil {
		var perr error
		if popular, perr = e.analytics.GetPopularItems(businessID, 5, period, loc); perr != nil {
			return SuggestionBatch{}, fmt.Errorf("load popular item signals: %w", perr)
		}
	}
	heroName := popularTopName(popular)

	// 3. Weakest daypart (single bounded query).
	daypart, err := e.weakestDaypart(ctx, businessID, loc, locale)
	if err != nil {
		return SuggestionBatch{}, fmt.Errorf("load weakest daypart signals: %w", err)
	}
	if daypart.HeroItemName == "" {
		daypart.HeroItemName = heroName
	}

	// 4. Lapsed-regular counts (single bounded query each).
	lapsed, err := e.lapsedRegularCount(ctx, businessID)
	if err != nil {
		return SuggestionBatch{}, fmt.Errorf("load lapsed regular signals: %w", err)
	}
	marketableLapsed, err := e.lapsedMarketableCount(ctx, businessID)
	if err != nil {
		return SuggestionBatch{}, fmt.Errorf("load marketable lapsed signals: %w", err)
	}

	// 5. Active bundles (single bounded query).
	bundles, err := e.activeBundles(ctx, businessID)
	if err != nil {
		return SuggestionBatch{}, fmt.Errorf("load active bundle signals: %w", err)
	}

	// 6. Active offers (single bounded query).
	offers, err := e.activeOffers(ctx, businessID, loc)
	if err != nil {
		return SuggestionBatch{}, fmt.Errorf("load active offer signals: %w", err)
	}

	// 7. Menu item metadata (single bounded read of the active menu JSON).
	menuMeta, err := e.menuItemMetaIndex(ctx, businessID)
	if err != nil {
		return SuggestionBatch{}, fmt.Errorf("load menu metadata: %w", err)
	}

	// 8. Inventory / recipe OOS set — same grounding AI Waiter uses so Marketing
	// never drives demand to a dish the kitchen cannot produce (warn-mode OOS).
	unmakeable, err := e.unmakeableMenuItemIDs(ctx, businessID)
	if err != nil {
		return SuggestionBatch{}, fmt.Errorf("load unmakeable menu items: %w", err)
	}

	now := e.currentTime()
	if loc == nil {
		loc = time.UTC
	}
	localNow := now.In(loc)
	blockedIDs := blockedMenuItemIDs(menuMeta, unmakeable)
	var inventoryBlocked []string
	var hiddenIdeas []*CampaignSuggestion
	for _, o := range offers {
		if !offerTargetsBlocked(o, blockedIDs) {
			continue
		}
		name := strings.TrimSpace(o.Name)
		if o.TargetID != nil {
			if item, ok := menuMeta.byID[strings.TrimSpace(*o.TargetID)]; ok {
				if n := strings.TrimSpace(item.Name); n != "" {
					name = n
				}
			}
		}
		inventoryBlocked = appendUniqueName(inventoryBlocked, name)
		if sugs := offerSuggestions([]database.Offer{o}, 1, localNow); len(sugs) > 0 {
			s := sugs[0]
			s.ID = fmt.Sprintf("%d:%s:%s", businessID, s.Play, s.TargetItemID)
			hiddenIdeas = append(hiddenIdeas, s)
		}
	}
	for _, b := range bundles {
		if bundleContainsBlocked(b.Items, blockedIDs) {
			inventoryBlocked = appendUniqueName(inventoryBlocked, strings.TrimSpace(b.Name))
			if sugs := comboSuggestions([]database.Bundle{b}, 1, nil); len(sugs) > 0 {
				s := sugs[0]
				s.ID = fmt.Sprintf("%d:%s:%s", businessID, s.Play, s.TargetItemID)
				hiddenIdeas = append(hiddenIdeas, s)
			}
		}
	}
	candidates := []*CampaignSuggestion{
		featuredDishSuggestion(menuRep),
		moveItemSuggestion(menuRep),
		attachHappyHourOffer(happyHourSuggestion(daypart), offersNotTargetingBlocked(offers, blockedIDs), localNow),
		winBackSuggestion(marketableLapsed, lapsed, heroName),
	}
	for _, s := range comboSuggestions(bundles, maxPromoSuggestions, blockedIDs) {
		candidates = append(candidates, s)
	}
	for _, s := range offerSuggestions(offers, maxPromoSuggestions, localNow) {
		candidates = append(candidates, s)
	}

	// Give menu-driven cards their dish photo and description when available.
	// ID-targeted plays (featured_dish, move_item) resolve by TargetItemID; plays
	// that only carry a hero item name (happy_hour, win_back) resolve by name so a
	// single-dish post with a real menu photo isn't stuck on "add a photo".
	// Also: suppress 86'd menu targets and clear unavailable name-only heroes (S2).
	menuImages := menuImageURLSet(menuMeta)
	enabledCandidates := make([]*CampaignSuggestion, 0, len(candidates))
	suggestionIDs := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		if candidate == nil {
			continue
		}
		if _, off := disabled[string(candidate.Play)]; off {
			continue
		}

		var meta menuItemMeta
		var ok bool
		if candidate.Play == PlayHappyHour {
			// Never resolve a leftover dish name for happy hour — captions and
			// photos must come from the attached offer (or its item target).
			if mid, linked := offerLinkedMenuItemID(candidate); linked {
				meta, ok = menuMeta.byID[mid]
			}
		} else {
			meta, ok = menuMeta.byID[candidate.TargetItemID]
			if !ok && candidate.TargetItemID == "" {
				meta, ok = menuMeta.lookupByName(candidate.TargetName)
			}
		}
		if ok {
			if candidate.ImageURL == "" && meta.ImageURL != "" {
				candidate.ImageURL = meta.ImageURL
				candidate.ImageSource = ImageSourceMenu
			}
			if candidate.TargetDescription == "" && meta.Description != "" {
				candidate.TargetDescription = meta.Description
			}
		}

		candidate.ID = fmt.Sprintf("%d:%s:%s", businessID, candidate.Play, candidate.TargetItemID)
		// S2: do not market dishes the menu marks unavailable or inventory cannot make.
		if shouldSuppressUnavailable(candidate, menuMeta) ||
			shouldSuppressUnmakeable(candidate, menuMeta, unmakeable) {
			inventoryBlocked = appendUniqueName(inventoryBlocked, inventoryBlockName(candidate, menuMeta))
			hiddenIdeas = append(hiddenIdeas, candidate)
			continue
		}
		clearUnavailableHero(candidate, menuMeta, unmakeable)
		stripUnattributedPhoto(candidate, menuImages)

		// Photo-ready boost for menu-enriched images (offers/combos already
		// apply it in their pure builders when ImageURL is set on the model).
		if candidate.ImageURL != "" && !hasWhyFactor(candidate, "photo_ready") {
			applyImageBoost(candidate)
		}
		stampRankingVersion(candidate)

		enabledCandidates = append(enabledCandidates, candidate)
		suggestionIDs = append(suggestionIDs, candidate.ID)
	}

	cutoff := now.Add(-postedSuggestionCooldown)
	handled := make(map[string]struct{})
	if e.loadHandled != nil {
		rows, err := e.loadHandled(businessID, suggestionIDs, cutoff)
		if err != nil {
			return SuggestionBatch{}, fmt.Errorf("load handled marketing activity: %w", err)
		}
		for _, row := range rows {
			switch row.Status {
			case "dismissed", "ready", "approved":
				// Soft hide + S2-E handoff states: off the live feed, no cooldown clock.
				handled[row.SuggestionID] = struct{}{}
			case "posted":
				if row.PostedAt != nil && row.PostedAt.After(cutoff) {
					handled[row.SuggestionID] = struct{}{}
				}
			}
		}
	}

	out := make([]CampaignSuggestion, 0, len(enabledCandidates))
	for _, c := range enabledCandidates {
		if _, gone := handled[c.ID]; gone {
			continue
		}
		out = append(out, *c)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Rank > out[j].Rank })
	// S2: collapse multiple plays targeting the same menu item (keep highest rank).
	out = dedupeMenuTargets(out)
	dedupeSharedPhotos(out)
	if len(out) > maxSuggestions {
		out = out[:maxSuggestions]
	}
	reason := EmptyReasonNone
	if len(out) == 0 {
		if len(enabledCandidates) > 0 {
			reason = EmptyReasonAllHandled
		} else {
			reason = EmptyReasonNoData
		}
	}
	e.persistInventoryHidden(businessID, hiddenIdeas)
	return SuggestionBatch{Suggestions: out, EmptyReason: reason, InventoryBlocked: inventoryBlocked}, nil
}

func (e *Engine) persistInventoryHidden(businessID uint, ideas []*CampaignSuggestion) {
	if e == nil || e.db == nil || len(ideas) == 0 {
		return
	}
	for _, s := range ideas {
		if s == nil || strings.TrimSpace(s.ID) == "" {
			continue
		}
		if err := e.db.EnsureInventoryHiddenMarketingActivity(database.MarketingActivity{
			BusinessID:   businessID,
			SuggestionID: s.ID,
			Play:         string(s.Play),
			Title:        s.Title,
			TargetName:   s.TargetName,
			Status:       "dismissed",
			ImageURL:     s.ImageURL,
			CreatedBy:    "inventory",
		}); err != nil {
			log.Printf("marketing: persist inventory-hidden %s: %v", s.ID, err)
		}
	}
}

func allMarketingPlaysDisabled(disabled map[string]struct{}) bool {
	plays := [...]Play{PlayHappyHour, PlayFeaturedDish, PlayMoveItem, PlayWinBack, PlayComboDeal, PlayOffer}
	for _, play := range plays {
		if _, off := disabled[string(play)]; !off {
			return false
		}
	}
	return true
}

// weakestDaypart buckets recent bills by weekday and hour (in business tz) and
// finds the weakest occupied window relative to the mean. One bounded SELECT.
func (e *Engine) weakestDaypart(ctx context.Context, businessID uint, loc *time.Location, locale string) (DaypartLoad, error) {
	if loc == nil {
		loc = time.UTC
	}
	type row struct {
		CreatedAt   time.Time
		TotalAmount int64
	}
	var bills []row
	since := e.currentTime().AddDate(0, 0, -28)
	// Count only bills that produced revenue from a served customer. Open tabs
	// and voided bills carry a total_amount but aren't real traffic, so counting
	// them would distort which window reads as "weak".
	settled := []database.BillStatus{database.BillStatusPaid, database.BillStatusClosed, database.BillStatusPartial}
	if err := e.db.GetGorm().WithContext(ctx).
		Model(&database.Bill{}).
		Select("created_at", "total_amount").
		Where("business_id = ? AND status IN ? AND created_at >= ?", businessID, settled, since).
		Find(&bills).Error; err != nil {
		return DaypartLoad{}, err
	}
	if len(bills) == 0 {
		return DaypartLoad{HasData: false}, nil
	}
	var sum [7][24]int64
	var cnt [7][24]int
	var total int64
	for _, b := range bills {
		t := b.CreatedAt.In(loc)
		wd := int(t.Weekday())
		h := t.Hour()
		sum[wd][h] += b.TotalAmount
		cnt[wd][h]++
		total += b.TotalAmount
	}
	occupied := 0
	for wd := 0; wd < 7; wd++ {
		for h := 0; h < 24; h++ {
			if cnt[wd][h] > 0 {
				occupied++
			}
		}
	}
	if occupied < 2 {
		return DaypartLoad{HasData: false}, nil
	}
	mean := float64(total) / float64(occupied)
	weakestWD, weakestH := -1, -1
	weakestRev := int64(1 << 62)
	for wd := 0; wd < 7; wd++ {
		for h := 0; h < 24; h++ {
			if cnt[wd][h] > 0 && sum[wd][h] < weakestRev {
				weakestWD, weakestH, weakestRev = wd, h, sum[wd][h]
			}
		}
	}
	pct := 0.0
	if mean > 0 {
		pct = 1 - float64(weakestRev)/mean
	}
	if pct <= 0 || weakestWD < 0 {
		return DaypartLoad{HasData: false}, nil
	}
	label := fmt.Sprintf("%s %s", weekdayShortLocalized(time.Weekday(weakestWD), locale), daypartLabel(weakestH, locale))
	return DaypartLoad{HasData: true, WeakestLabel: label, PctBelowMean: pct}, nil
}

// isSpanishLocale reports whether a language tag resolves to a Spanish prompt
// family (es, es_ar) in the locale registry. Detection is delegated to
// internal/locales so the es/en split has a single source of truth; unknown
// tags resolve to the registry default (English).
func isSpanishLocale(locale string) bool {
	fam := promptFamilyOf(locale)
	return fam == "es" || fam == "es_ar"
}

// promptFamilyOf resolves a language tag to its registry prompt family,
// tolerating underscore/case variants ("es_AR", "es-ar") the same way the
// services-layer resolvePromptLocale does.
func promptFamilyOf(locale string) string {
	if l, ok := locales.Lookup(locale); ok {
		return l.PromptFamily
	}
	normalized := strings.ToLower(strings.ReplaceAll(strings.TrimSpace(locale), "_", "-"))
	for _, l := range locales.AllLocales() {
		if normalized == strings.ToLower(l.Canonical) || normalized == l.PathSegment {
			return l.PromptFamily
		}
	}
	return locales.Default().PromptFamily
}

// weekdayShortLocalized renders a short weekday name in the operator's language.
// Only English and Spanish are distinguished today; any other/unknown locale
// falls back to English. (The locale registry has no short-weekday facility, so
// the name tables live here; only Spanish DETECTION is registry-driven.)
func weekdayShortLocalized(wd time.Weekday, locale string) string {
	es := []string{"Dom", "Lun", "Mar", "Mié", "Jue", "Vie", "Sáb"}
	en := []string{"Sun", "Mon", "Tue", "Wed", "Thu", "Fri", "Sat"}
	names := en
	if isSpanishLocale(locale) {
		names = es
	}
	if int(wd) < 0 || int(wd) >= len(names) {
		// Effectively unreachable: wd comes from a validated 0-6 SQL bucket.
		// Return an obvious sentinel rather than a plausible-but-wrong weekday.
		return "Day"
	}
	return names[wd]
}

// daypartLabel renders an hour as a readable window. English uses am/pm
// (17 -> "5–6pm"); Spanish restaurant copy uses 24h "h" windows
// (17 -> "17–18h"), matching the operator-facing style.
func daypartLabel(hour int, locale string) string {
	if isSpanishLocale(locale) {
		// % 24 exists for the hour+1 wrap (23 -> "23–0h"); it's a no-op for
		// the in-range hour itself.
		fmtH := func(h int) int { return h % 24 }
		return fmt.Sprintf("%d–%dh", fmtH(hour), fmtH(hour+1))
	}
	fmtH := func(h int) string {
		ap := "am"
		hh := h % 24
		if hh >= 12 {
			ap = "pm"
		}
		d := hh % 12
		if d == 0 {
			d = 12
		}
		return fmt.Sprintf("%d%s", d, ap)
	}
	return fmt.Sprintf("%s–%s", fmtH(hour), fmtH(hour+1))
}

// lapsedRegularCount counts customers who used to visit (>=2) but not in 30 days.
func (e *Engine) lapsedRegularCount(ctx context.Context, businessID uint) (int, error) {
	cutoff := e.currentTime().AddDate(0, 0, -30)
	var n int64
	if err := e.db.GetGorm().WithContext(ctx).
		Model(&database.CustomerBusiness{}).
		Where("business_id = ? AND visit_count >= 2 AND last_visit_at IS NOT NULL AND last_visit_at < ?", businessID, cutoff).
		Count(&n).Error; err != nil {
		return 0, err
	}
	return int(n), nil
}

// lapsedMarketableCount counts lapsed regulars who opted in to marketing.
func (e *Engine) lapsedMarketableCount(ctx context.Context, businessID uint) (int, error) {
	cutoff := e.currentTime().AddDate(0, 0, -30)
	var n int64
	if err := e.db.GetGorm().WithContext(ctx).
		Model(&database.CustomerBusiness{}).
		Where("business_id = ? AND visit_count >= 2 AND last_visit_at IS NOT NULL AND last_visit_at < ? AND opt_in_marketing = ?", businessID, cutoff, true).
		Count(&n).Error; err != nil {
		return 0, err
	}
	return int(n), nil
}

// activeBundles loads active bundles (bounded; newest first).
func (e *Engine) activeBundles(ctx context.Context, businessID uint) ([]database.Bundle, error) {
	var bundles []database.Bundle
	if err := e.db.GetGorm().WithContext(ctx).
		Select("id", "name", "price", "image", "items", "is_active").
		Where("business_id = ? AND is_active = ?", businessID, true).
		Order("created_at DESC").
		Limit(10).
		Find(&bundles).Error; err != nil {
		return nil, err
	}
	return bundles, nil
}

// activeOffers loads active, in-date, in-window offers (bounded; newest first).
// Narrow projection — never selects the json:"-" Business relation. Recurring
// weekday/minute windows are evaluated in loc (UTC when loc is nil).
func (e *Engine) activeOffers(ctx context.Context, businessID uint, loc *time.Location) ([]database.Offer, error) {
	now := e.currentTime()
	if loc == nil {
		loc = time.UTC
	}
	local := now.In(loc)
	var offers []database.Offer
	if err := e.db.GetGorm().WithContext(ctx).
		Select("id", "name", "image", "discount_type", "discount_value", "code", "start_date", "end_date", "created_at", "applicable_to", "target_id", "weekday_mask", "start_minute", "end_minute").
		Where("business_id = ? AND is_active = ?", businessID, true).
		Where("start_date IS NULL OR start_date <= ?", now).
		Where("end_date IS NULL OR end_date >= ?", now).
		Order("created_at DESC").
		Limit(5).
		Find(&offers).Error; err != nil {
		return nil, err
	}
	return database.FilterOffersActiveAt(offers, local, loc.String()), nil
}

// UnmakeableLoaderFn returns menu item IDs that must not be promoted (recipe OOS
// / BlocksSale). Production wires database.UnrecommendableMenuItemIDs; tests stub it.
type UnmakeableLoaderFn func(businessID uint) (map[string]bool, error)

// SetUnmakeableLoader overrides the inventory grounding seam (tests).
func (e *Engine) SetUnmakeableLoader(fn UnmakeableLoaderFn) {
	e.loadUnmakeable = fn
}

func (e *Engine) unmakeableMenuItemIDs(ctx context.Context, businessID uint) (map[string]bool, error) {
	_ = ctx
	if e.loadUnmakeable != nil {
		return e.loadUnmakeable(businessID)
	}
	if e.db == nil {
		return map[string]bool{}, nil
	}
	ids, err := database.UnrecommendableMenuItemIDs(businessID)
	if err != nil {
		// Fail closed — same posture as AI Waiter. Returning an empty set would
		// let OOS / unmakeable dishes reappear in suggestions when inventory
		// grounding is unavailable.
		return nil, err
	}
	return ids, nil
}

// menuItemMeta maps menu item IDs to image URL, description, and availability.
type menuItemMeta struct {
	ID          string
	Name        string
	ImageURL    string
	Description string
	// Available is true unless the menu JSON explicitly sets is_available:false.
	// Missing is_available is treated as available (matches guest FE semantics).
	Available bool
	// Seen is true when the item appears on the active menu (even with no photo).
	Seen bool
}

// menuMetaIndex holds two views of the active menu's items: keyed by item ID and
// keyed by normalized (lower-cased, trimmed) name. Plays that target an item by
// ID (featured_dish, move_item) resolve via byID; plays that only know a hero
// item's display name (happy_hour, win_back) resolve via byName.
type menuMetaIndex struct {
	byID   map[string]menuItemMeta
	byName map[string]menuItemMeta
}

// lookupByName resolves an item's metadata from its display name.
func (m menuMetaIndex) lookupByName(name string) (menuItemMeta, bool) {
	key := strings.ToLower(strings.TrimSpace(name))
	if key == "" {
		return menuItemMeta{}, false
	}
	meta, ok := m.byName[key]
	return meta, ok
}

// idByName resolves a menu item ID from its display name.
func (m menuMetaIndex) idByName(name string) string {
	meta, ok := m.lookupByName(name)
	if !ok {
		return ""
	}
	return meta.ID
}

// rawMenuItem is used only when indexing the active menu so we can tell
// "is_available omitted" (→ available) from "is_available:false" (→ 86'd).
// database.MenuItem's bool field cannot distinguish those cases.
type rawMenuItem struct {
	ID               string   `json:"id"`
	Name             string   `json:"name"`
	Description      string   `json:"description"`
	Image            string   `json:"image"`
	Images           []string `json:"images"`
	CompositionImage string   `json:"composition_image"`
	IsAvailable      *bool    `json:"is_available"`
}

type rawMenuCategory struct {
	Items []rawMenuItem `json:"items"`
}

// stockOutMenuItemIDs returns recipe-linked dishes that cannot make a serving
// when inventory sync is enabled (warn / hard_block). Failures and missing
// inventory tables fail open so marketing still works without inventory.
func (e *Engine) stockOutMenuItemIDs(ctx context.Context, businessID uint) map[string]bool {
	var settings database.InventorySettings
	err := e.db.GetGorm().WithContext(ctx).
		Where("business_id = ?", businessID).
		First(&settings).Error
	if err != nil || !settings.InventoryEnabled {
		return nil
	}
	if strings.TrimSpace(settings.AvailabilitySyncMode) == "" {
		settings.AvailabilitySyncMode = database.InventoryAvailabilityModeWarn
	}
	if settings.AvailabilitySyncMode == database.InventoryAvailabilityModeManual {
		return nil
	}
	oos, err := database.OutOfStockMenuItemIDs(businessID)
	if err != nil || len(oos) == 0 {
		return nil
	}
	return oos
}

// menuItemMetaIndex maps menu item IDs and names to metadata. One bounded read of
// the active menu (items live inside the menu's Categories JSON blob).
func (e *Engine) menuItemMetaIndex(ctx context.Context, businessID uint) (menuMetaIndex, error) {
	idx := menuMetaIndex{byID: map[string]menuItemMeta{}, byName: map[string]menuItemMeta{}}
	var menus []database.Menu
	if err := e.db.GetGorm().WithContext(ctx).
		Select("id", "categories").
		Where("business_id = ? AND is_active = ?", businessID, true).
		Order("version DESC").
		Limit(1).
		Find(&menus).Error; err != nil {
		return idx, err
	}
	if len(menus) == 0 || menus[0].Categories == "" {
		return idx, nil
	}
	menu := menus[0]
	var cats []rawMenuCategory
	if err := json.Unmarshal([]byte(menu.Categories), &cats); err != nil {
		return idx, err
	}
	// Issue #200: also treat zero-stock recipe dishes as unavailable so
	// marketing never cheerfully promotes an 86'd plate.
	stockOut := e.stockOutMenuItemIDs(ctx, businessID)
	for _, c := range cats {
		for _, it := range c.Items {
			available := true
			if it.IsAvailable != nil {
				available = *it.IsAvailable
			}
			if stockOut[strings.TrimSpace(it.ID)] {
				available = false
			}
			meta := menuItemMeta{
				ID:          it.ID,
				Name:        strings.TrimSpace(it.Name),
				ImageURL:    bestRawItemImage(it),
				Description: strings.TrimSpace(it.Description),
				Available:   available,
				Seen:        true,
			}
			// Always index seen items (even without image/description) so the
			// availability suppress path can 86 a sold-out dish.
			if it.ID != "" {
				idx.byID[it.ID] = meta
			}
			if name := strings.ToLower(strings.TrimSpace(it.Name)); name != "" {
				// Don't overwrite an earlier item that already carries an image
				// with a later same-named item that has none. Prefer unavailable
				// flags when names collide (safer not to market a sold-out dish).
				prev, exists := idx.byName[name]
				if !exists {
					idx.byName[name] = meta
				} else if !meta.Available {
					idx.byName[name] = meta
				} else if prev.ImageURL == "" && meta.ImageURL != "" {
					idx.byName[name] = meta
				}
			}
		}
	}
	return idx, nil
}

// bestRawItemImage prefers the primary image, then the first gallery image, then
// the AI composition image.
func bestRawItemImage(it rawMenuItem) string {
	if it.Image != "" {
		return it.Image
	}
	for _, img := range it.Images {
		if img != "" {
			return img
		}
	}
	return it.CompositionImage
}
