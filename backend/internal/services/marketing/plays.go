package marketing

import (
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/stdevmac/payverge/backend/internal/analytics"
	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/services/menuengineering"
)

// winBackMinLapsed is the floor below which a win-back campaign isn't worth it.
const winBackMinLapsed = 10

// topSellerMinQty is the sales floor for calling an item a "top seller" in
// generated copy — below it the featured-dish play uses margin-led copy.
const topSellerMinQty = 15

// happyHourMinPctBelow drops daypart noise: windows only slightly below the
// mean are not worth a happy-hour campaign (S2 weak-play suppress).
const happyHourMinPctBelow = 0.15

// moveItemMinMarginUSD requires meaningful unit margin before we recommend a
// "give it a push" spotlight (S2 weak-play suppress).
const moveItemMinMarginUSD = 1.0

// moveItemMinQtySold is the sample floor for a move-item campaign. One order
// is classification noise, not a trend (#243).
const moveItemMinQtySold = 3

// moveItemWeakSampleQty labels "give it a push" ideas that barely clear the
// floor so the operator can see the signal is thin.
const moveItemWeakSampleQty = 8

// offerUrgencyWindowDays boosts offers ending within this many days.
const offerUrgencyWindowDays = 7

// DaypartLoad is the precomputed weakest-window signal for the happy_hour play.
type DaypartLoad struct {
	HasData      bool
	WeakestLabel string  // e.g. "Tue 5–7pm"
	PctBelowMean float64 // 0..1; how far below the mean the weakest window is
	HeroItemName string  // a high-margin item to feature, if known
}

func popularTopName(items []analytics.ItemStats) string {
	if len(items) == 0 {
		return ""
	}
	return items[0].ItemName
}

// pluralize returns singular when n == 1 and plural otherwise, so generated copy
// reads "1 order" / "2 orders" instead of "1 orders". Used for all count-bearing
// play copy (orders, regulars, etc.).
func pluralize(n int, singular, plural string) string {
	if n == 1 {
		return singular
	}
	return plural
}

// featuredDishSuggestion features the strongest "star" (popular + high margin).
func featuredDishSuggestion(rep menuengineering.Report) *CampaignSuggestion {
	var best *menuengineering.DishClass
	for i := range rep.Dishes {
		d := &rep.Dishes[i]
		if d.Quadrant != menuengineering.QuadrantStar {
			continue
		}
		// Weak suppress: zero-sold "stars" are classification noise.
		if d.QtySold <= 0 {
			continue
		}
		if best == nil || d.QtySold > best.QtySold {
			best = d
		}
	}
	if best == nil {
		return nil
	}
	// Honesty gate: a "star" in a low-volume period can have sold a handful of
	// units — only claim "top seller" when the volume actually supports it.
	why := fmt.Sprintf("%s is one of your highest-margin dishes — a spotlight puts that margin to work.", best.MenuItemName)
	angle := "feature the house favorite; appetite-forward, confident"
	if best.QtySold >= topSellerMinQty {
		why = fmt.Sprintf("%s is a top seller and one of your highest-margin dishes.", best.MenuItemName)
		angle = "celebrate the signature best-seller; appetite-forward, confident"
	}

	// Rank: base + capped qty + capped margin (S2 signal quality).
	qtyComponent := math.Min(float64(best.QtySold), 100)
	marginComponent := math.Min(best.MarginPerUnit, 20)
	rank := 60 + qtyComponent + marginComponent

	factors := []WhyFactor{
		{Key: "qty_sold", Value: formatCount(best.QtySold), Weight: qtyComponent},
		{Key: "margin_per_unit", Value: formatMoney(best.MarginPerUnit), Weight: marginComponent},
		{Key: "quadrant", Value: string(menuengineering.QuadrantStar), Weight: 0},
	}

	s := &CampaignSuggestion{
		Play:           PlayFeaturedDish,
		PlayKey:        string(PlayFeaturedDish),
		Title:          fmt.Sprintf("Feature your star: %s", best.MenuItemName),
		WhyData:        why,
		WhyFactors:     factors,
		RankingVersion: RankingVersionS2,
		Source:         "menu_engineering",
		TargetItemID:   best.MenuItemID,
		TargetName:     best.MenuItemName,
		CopyAngle:      angle,
		Metrics:        map[string]any{"qty_sold": best.QtySold, "margin_per_unit": best.MarginPerUnit, "price": best.AvgPrice},
		Rank:           rank,
	}
	return s
}

// moveItemSuggestion promotes a high-margin-but-underselling item (puzzle).
func moveItemSuggestion(rep menuengineering.Report) *CampaignSuggestion {
	var best *menuengineering.DishClass
	for i := range rep.Dishes {
		d := &rep.Dishes[i]
		if d.Action != "promote" {
			continue
		}
		// Weak suppress: thin-margin "promote" items are not worth a spotlight.
		if d.MarginPerUnit < moveItemMinMarginUSD {
			continue
		}
		// Weak suppress: 1–2 orders is not a campaign sample (#243).
		if d.QtySold < moveItemMinQtySold {
			continue
		}
		if best == nil || d.MarginPerUnit > best.MarginPerUnit {
			best = d
		}
	}
	if best == nil {
		return nil
	}

	// Velocity gap: how far below menu-median popularity the dish sits.
	// Larger gap → stronger "underselling" signal for ranking.
	velocityGap := 0.0
	if rep.MedianQtySold > 0 {
		velocityGap = rep.MedianQtySold - float64(best.QtySold)
		if velocityGap < 0 {
			velocityGap = 0
		}
		velocityGap = math.Min(velocityGap, 30)
	}
	marginComponent := math.Min(best.MarginPerUnit, 25)
	rank := 45 + marginComponent + velocityGap

	why := fmt.Sprintf(
		"%s earns a strong margin but sells slowly — only %d %s in the last 30 days, so a spotlight could move it.",
		best.MenuItemName, best.QtySold, pluralize(best.QtySold, "order", "orders"),
	)

	factors := []WhyFactor{
		{Key: "margin_per_unit", Value: formatMoney(best.MarginPerUnit), Weight: marginComponent},
		{Key: "qty_sold", Value: formatCount(best.QtySold), Weight: 0},
	}
	if velocityGap > 0 {
		factors = append(factors, WhyFactor{
			Key:    "velocity_gap",
			Value:  formatCount(int(math.Round(velocityGap))),
			Weight: velocityGap,
		})
	}
	if best.QtySold < moveItemWeakSampleQty {
		factors = append(factors, WhyFactor{
			Key:   "weak_signal",
			Value: formatCount(best.QtySold),
		})
	}

	return &CampaignSuggestion{
		Play:           PlayMoveItem,
		PlayKey:        string(PlayMoveItem),
		Title:          fmt.Sprintf("Give %s a push", best.MenuItemName),
		WhyData:        why,
		WhyFactors:     factors,
		RankingVersion: RankingVersionS2,
		Source:         "menu_engineering",
		TargetItemID:   best.MenuItemID,
		TargetName:     best.MenuItemName,
		CopyAngle:      "intrigue and discovery; position as a hidden gem worth trying",
		Metrics: map[string]any{
			"qty_sold":        best.QtySold,
			"margin_per_unit": best.MarginPerUnit,
			"price":           best.AvgPrice,
			"median_qty_sold": rep.MedianQtySold,
			"velocity_gap":    velocityGap,
		},
		Rank: rank,
	}
}

// happyHourSuggestion fills the weakest daypart.
func happyHourSuggestion(d DaypartLoad) *CampaignSuggestion {
	if !d.HasData {
		return nil
	}
	// Weak suppress: tiny dips below the mean are noise, not a happy-hour case.
	if d.PctBelowMean < happyHourMinPctBelow {
		return nil
	}
	title := "Fill your slowest window"
	if d.WeakestLabel != "" {
		title = fmt.Sprintf("Fill %s with a happy hour", d.WeakestLabel)
	}
	pctWeight := d.PctBelowMean * 50
	rank := 40 + pctWeight

	return &CampaignSuggestion{
		Play:       PlayHappyHour,
		PlayKey:    string(PlayHappyHour),
		DaypartKey: d.WeakestLabel,
		Title:      title,
		WhyData:    fmt.Sprintf("%s is your weakest window — %.0f%% below your average.", d.WeakestLabel, d.PctBelowMean*100),
		WhyFactors: []WhyFactor{
			{Key: "weakest_window", Value: d.WeakestLabel, Weight: 0},
			{Key: "pct_below_mean", Value: formatPct(d.PctBelowMean), Weight: pctWeight},
		},
		RankingVersion: RankingVersionS2,
		Source:         "slow_dayparts",
		// Caption subject is the attached live offer, never a leftover dish
		// name (prod #242 captioned OOS Steak Plate with no discount).
		TargetName: "",
		CopyAngle:  "limited-time, time-bound urgency; reason to come in during a quiet window",
		Metrics:    map[string]any{"weakest_window": d.WeakestLabel, "pct_below_mean": d.PctBelowMean},
		Rank:       rank,
	}
}

// attachHappyHourOffer pairs the slowest window with a live offer. Happy-hour
// plays are suppressed when no live offer exists — posting "happy hour" without
// a real discount invents a deal guests cannot redeem (#242).
func attachHappyHourOffer(s *CampaignSuggestion, offers []database.Offer, now time.Time) *CampaignSuggestion {
	if s == nil {
		return nil
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	var o *database.Offer
	for i := range offers {
		if offerIsLive(offers[i], now) && strings.TrimSpace(offers[i].Name) != "" {
			o = &offers[i]
			break
		}
	}
	if o == nil {
		return nil
	}
	if s.Metrics == nil {
		s.Metrics = map[string]any{}
	}
	s.Metrics["suggested_offer"] = o.Name
	s.Metrics["suggested_discount_type"] = o.DiscountType
	s.Metrics["suggested_discount_value"] = o.DiscountValue
	if o.StartMinute != nil {
		s.Metrics["offer_start_minute"] = *o.StartMinute
	}
	if o.EndMinute != nil {
		s.Metrics["offer_end_minute"] = *o.EndMinute
	}
	if o.WeekdayMask != 0 {
		s.Metrics["offer_weekday_mask"] = o.WeekdayMask
	}
	s.DiscountType = o.DiscountType
	s.DiscountValue = o.DiscountValue
	// Prefer the offer name as the caption subject so the model does not invent
	// a "Steak Plate special" with no discount attached.
	if strings.TrimSpace(o.Name) != "" {
		s.TargetName = o.Name
		if o.Image != "" {
			s.ImageURL = o.Image
			s.ImageSource = ImageSourceOffer
		}
	}
	if strings.EqualFold(strings.TrimSpace(o.ApplicableTo), "item") && o.TargetID != nil {
		if tid := strings.TrimSpace(*o.TargetID); tid != "" {
			s.Metrics["offer_target_item_id"] = tid
		}
	}
	s.WhyFactors = append(s.WhyFactors, WhyFactor{
		Key:   "suggested_offer",
		Value: o.Name,
	})
	s.WhyFactors = append(s.WhyFactors, WhyFactor{
		Key:   "discount",
		Value: formatOfferDiscount(*o),
	})
	s.CopyAngle = "limited-time happy hour with a real discount; state the offer and window clearly"
	s.Title = fmt.Sprintf("Fill %s with \"%s\"", strings.TrimSpace(s.DaypartKey), o.Name)
	if strings.TrimSpace(s.DaypartKey) == "" {
		s.Title = fmt.Sprintf("Run a happy hour with \"%s\"", o.Name)
	}
	return s
}

// winBackSuggestion targets lapsed regulars who opted in to marketing.
func winBackSuggestion(marketableLapsed int, totalLapsed int, heroItem string) *CampaignSuggestion {
	if marketableLapsed < winBackMinLapsed {
		return nil
	}
	// Opt-in share of all lapsed regulars — higher share → cleaner CRM signal.
	optInShare := 0.0
	if totalLapsed > 0 {
		optInShare = float64(marketableLapsed) / float64(totalLapsed)
		if optInShare > 1 {
			optInShare = 1
		}
	}
	shareBonus := optInShare * 10
	countComponent := float64(marketableLapsed)
	rank := 50 + countComponent + shareBonus

	return &CampaignSuggestion{
		Play:    PlayWinBack,
		PlayKey: string(PlayWinBack),
		Title:   fmt.Sprintf("Win back %d opted-in %s", marketableLapsed, pluralize(marketableLapsed, "regular", "regulars")),
		WhyData: fmt.Sprintf("%d %s opted in to hear from you and haven't visited in over a month.", marketableLapsed, pluralize(marketableLapsed, "regular has", "regulars have")),
		WhyFactors: []WhyFactor{
			{Key: "marketable_lapsed", Value: formatCount(marketableLapsed), Weight: countComponent},
			{Key: "total_lapsed", Value: formatCount(totalLapsed), Weight: 0},
			{Key: "opt_in_share", Value: formatPct(optInShare), Weight: shareBonus},
		},
		RankingVersion: RankingVersionS2,
		Source:         "crm_segment",
		TargetName:     heroItem,
		CopyAngle:      "warm, we-miss-you tone; a small comeback incentive",
		Metrics: map[string]any{
			"marketable_lapsed": marketableLapsed,
			"total_lapsed":      totalLapsed,
			"opt_in_share":      optInShare,
		},
		Rank: rank,
	}
}

const maxPromoSuggestions = 3

func offerTargetsBlocked(o database.Offer, blocked map[string]bool) bool {
	if len(blocked) == 0 {
		return false
	}
	if !strings.EqualFold(strings.TrimSpace(o.ApplicableTo), "item") || o.TargetID == nil {
		return false
	}
	id := strings.TrimSpace(*o.TargetID)
	return id != "" && blocked[id]
}

func offersNotTargetingBlocked(offers []database.Offer, blocked map[string]bool) []database.Offer {
	if len(blocked) == 0 || len(offers) == 0 {
		return offers
	}
	out := make([]database.Offer, 0, len(offers))
	for _, o := range offers {
		if offerTargetsBlocked(o, blocked) {
			continue
		}
		out = append(out, o)
	}
	return out
}

func bundleItemIDs(itemsJSON string) []string {
	itemsJSON = strings.TrimSpace(itemsJSON)
	if itemsJSON == "" {
		return nil
	}
	var refs []database.BundleItemRef
	if err := json.Unmarshal([]byte(itemsJSON), &refs); err != nil {
		return nil
	}
	out := make([]string, 0, len(refs))
	for _, ref := range refs {
		if id := strings.TrimSpace(ref.MenuItemID); id != "" {
			out = append(out, id)
		}
	}
	return out
}

func bundleContainsBlocked(itemsJSON string, blocked map[string]bool) bool {
	if len(blocked) == 0 {
		return false
	}
	for _, id := range bundleItemIDs(itemsJSON) {
		if blocked[id] {
			return true
		}
	}
	return false
}

// comboSuggestions promotes active bundles (newest first, up to limit).
func comboSuggestions(bundles []database.Bundle, limit int, blocked map[string]bool) []*CampaignSuggestion {
	if limit <= 0 {
		limit = 1
	}
	out := make([]*CampaignSuggestion, 0, limit)
	for i := range bundles {
		b := bundles[i]
		if !b.IsActive {
			continue
		}
		// Weak suppress: nameless bundles aren't postable.
		if strings.TrimSpace(b.Name) == "" {
			continue
		}
		// Do not campaign a combo that contains an 86'd / OOS dish (#200).
		if bundleContainsBlocked(b.Items, blocked) {
			continue
		}
		rank := 35.0 - float64(len(out))*0.5
		factors := []WhyFactor{
			{Key: "bundle_price", Value: formatMoney(b.Price), Weight: 0},
		}
		if b.Image != "" {
			rank += imageRankBoost
			// Numeric/flag only — FE owns "has a photo" copy (L4-22).
			factors = append(factors, WhyFactor{
				Key:    "photo_ready",
				Value:  "1",
				Weight: imageRankBoost,
			})
		}
		out = append(out, &CampaignSuggestion{
			Play:           PlayComboDeal,
			PlayKey:        string(PlayComboDeal),
			Title:          fmt.Sprintf("Promote your \"%s\" deal", b.Name),
			WhyData:        "A bundle gives guests an easy, higher-value order — worth putting in front of them.",
			WhyFactors:     factors,
			RankingVersion: RankingVersionS2,
			Source:         "bundles",
			TargetItemID:   fmt.Sprintf("bundle:%d", b.ID),
			TargetName:     b.Name,
			CopyAngle:      "value and convenience; make the deal feel generous",
			Metrics:        map[string]any{"price": b.Price},
			Rank:           rank,
			ImageURL:       b.Image,
			ImageSource:    ImageSourceBundle,
		})
		if len(out) >= limit {
			break
		}
	}
	return out
}

// offerIsLive reports whether an offer is currently usable for marketing:
// named, positive discount, within optional date bounds, and inside its
// recurring weekday/minute window (OfferActiveAt). IsActive is enforced by
// the SQL loader (activeOffers); pure unit fixtures often omit the bool zero
// value, so we do not re-require it here. Expired / not-yet-started / zero
// discount / out-of-window offers are suppressed.
func offerIsLive(o database.Offer, now time.Time) bool {
	if o.DiscountValue <= 0 {
		return false
	}
	if o.StartDate != nil && o.StartDate.After(now) {
		return false
	}
	if o.EndDate != nil && o.EndDate.Before(now) {
		return false
	}
	return database.OfferActiveAt(o, now)
}

// offerUrgencyBoost returns a rank bonus for offers ending soon (0..8).
func offerUrgencyBoost(o database.Offer, now time.Time) float64 {
	if o.EndDate == nil {
		return 0
	}
	hoursLeft := o.EndDate.Sub(now).Hours()
	if hoursLeft < 0 {
		return 0
	}
	daysLeft := hoursLeft / 24
	if daysLeft > offerUrgencyWindowDays {
		return 0
	}
	// Ending today → +8; ending on day 7 → ~+1.
	return math.Max(0, float64(offerUrgencyWindowDays)+1-daysLeft)
}

// offerSuggestions promotes live offers (newest first, up to limit). now is
// used to suppress expired / not-yet-started / zero-value offers and to boost
// urgency for ending-soon promos (S2).
func offerSuggestions(offers []database.Offer, limit int, now time.Time) []*CampaignSuggestion {
	if len(offers) == 0 || limit <= 0 {
		return nil
	}
	out := make([]*CampaignSuggestion, 0, limit)
	for i := 0; i < len(offers) && len(out) < limit; i++ {
		o := offers[i]
		if !offerIsLive(o, now) {
			continue
		}
		urgency := offerUrgencyBoost(o, now)
		// Newer list position still wins among equal urgency; small index penalty.
		rank := 55 - float64(len(out)) + urgency

		factors := []WhyFactor{
			{Key: "discount", Value: formatOfferDiscount(o), Weight: 0},
		}
		if urgency > 0 {
			// Days window as bare number — FE formats "Ends within N days" (L4-22).
			factors = append(factors, WhyFactor{
				Key:    "ending_soon",
				Value:  formatCount(offerUrgencyWindowDays),
				Weight: urgency,
			})
		}
		if o.Image != "" {
			// Image boost applied here for offers that already carry creative;
			// engine applyImageBoost is idempotent-ish only if ImageURL set
			// later — stamp weight once here so pure tests see it.
			rank += imageRankBoost
			factors = append(factors, WhyFactor{
				Key:    "photo_ready",
				Value:  "1",
				Weight: imageRankBoost,
			})
		}

		metrics := map[string]any{
			"discount_type":  o.DiscountType,
			"discount_value": o.DiscountValue,
			"urgency_boost":  urgency,
		}
		if strings.EqualFold(strings.TrimSpace(o.ApplicableTo), "item") && o.TargetID != nil {
			if tid := strings.TrimSpace(*o.TargetID); tid != "" {
				metrics["offer_target_item_id"] = tid
			}
		}
		out = append(out, &CampaignSuggestion{
			Play:           PlayOffer,
			PlayKey:        string(PlayOffer),
			Title:          fmt.Sprintf("Promote your \"%s\" offer", o.Name),
			WhyData:        "You have a live offer running — put it in front of guests while it lasts.",
			WhyFactors:     factors,
			RankingVersion: RankingVersionS2,
			Source:         "offers",
			TargetItemID:   fmt.Sprintf("offer:%d", o.ID),
			TargetName:     o.Name,
			CopyAngle:      "highlight the deal; clear savings and a reason to act now",
			Metrics:        metrics,
			Rank:           rank,
			ImageURL:       o.Image,
			ImageSource:    ImageSourceOffer,
			DiscountType:   o.DiscountType,
			DiscountValue:  o.DiscountValue,
		})
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// formatOfferDiscount emits a numeric/wire token only (L4-22). FE owns
// "off"/"% off" localization via why.factors.discount_* keys.
// percentage → "12.5%" ; fixed/other → bare money number from formatMoney.
func formatOfferDiscount(o database.Offer) string {
	switch o.DiscountType {
	case "percentage":
		return formatMoney(o.DiscountValue) + "%"
	default:
		return formatMoney(o.DiscountValue)
	}
}
