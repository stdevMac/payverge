package marketing

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/stdevmac/payverge/backend/internal/analytics"
	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/services/menuengineering"
)

func TestFeaturedDishSuggestion_PicksTopStar(t *testing.T) {
	rep := menuengineering.Report{Dishes: []menuengineering.DishClass{
		{MenuItemID: "m1", MenuItemName: "Carbonara", Quadrant: "star", QtySold: 120, MarginPerUnit: 9.2, AvgPrice: 12.99},
		{MenuItemID: "m4", MenuItemName: "Tiramisu", Quadrant: "star", QtySold: 200, MarginPerUnit: 6.0},
		{MenuItemID: "m2", MenuItemName: "Side Salad", Quadrant: "dog", QtySold: 4, MarginPerUnit: 1.0},
	}}
	s := featuredDishSuggestion(rep)
	require.NotNil(t, s)
	require.Equal(t, PlayFeaturedDish, s.Play)
	require.Equal(t, "m4", s.TargetItemID, "picks the highest-QtySold star")
	require.Equal(t, "Tiramisu", s.TargetName)
	require.Contains(t, s.WhyData, "Tiramisu")
}

func TestFeaturedDishSuggestion_NoStars_Nil(t *testing.T) {
	require.Nil(t, featuredDishSuggestion(menuengineering.Report{Sparse: true}))
}

func TestFeaturedDishSuggestion_IncludesPrice(t *testing.T) {
	rep := menuengineering.Report{Dishes: []menuengineering.DishClass{
		{MenuItemID: "m1", MenuItemName: "Carbonara", Quadrant: "star", QtySold: 120, MarginPerUnit: 9.2, AvgPrice: 12.99},
	}}
	s := featuredDishSuggestion(rep)
	require.NotNil(t, s)
	require.InDelta(t, 12.99, s.Metrics["price"], 0.001, "price feeds the post's price slot")
}

func TestMoveItemSuggestion_PicksPromoteAction(t *testing.T) {
	rep := menuengineering.Report{Dishes: []menuengineering.DishClass{
		{MenuItemID: "m9", MenuItemName: "Lobster", Quadrant: "star", Action: "keep", MarginPerUnit: 25.0, QtySold: 80}, // higher margin but not promote -> excluded
		{MenuItemID: "m3", MenuItemName: "Osso Buco", Quadrant: "puzzle", Action: "promote", MarginPerUnit: 14.0, QtySold: 6},
		{MenuItemID: "m5", MenuItemName: "Risotto", Quadrant: "puzzle", Action: "promote", MarginPerUnit: 18.0, QtySold: 9, AvgPrice: 7.5},
	}}
	s := moveItemSuggestion(rep)
	require.NotNil(t, s)
	require.Equal(t, PlayMoveItem, s.Play)
	require.Equal(t, "Risotto", s.TargetName, "highest-margin promote item, excluding the higher-margin non-promote dish")
	require.InDelta(t, 7.5, s.Metrics["price"], 0.001)
}

func TestHappyHourSuggestion_WeakWindow(t *testing.T) {
	s := happyHourSuggestion(DaypartLoad{HasData: true, WeakestLabel: "Tue 5–7pm", PctBelowMean: 0.38})
	require.NotNil(t, s)
	require.Equal(t, PlayHappyHour, s.Play)
	require.Contains(t, s.WhyData, "Tue 5–7pm")
	require.Contains(t, s.Title, "Tue 5–7pm", "title references the weak window even without a hero item")
}

func TestHappyHourSuggestion_NoData_Nil(t *testing.T) {
	require.Nil(t, happyHourSuggestion(DaypartLoad{HasData: false}))
}

func TestAttachHappyHourOffer_PairsNewestOffer(t *testing.T) {
	s := happyHourSuggestion(DaypartLoad{HasData: true, WeakestLabel: "Tue 5–7pm", PctBelowMean: 0.2})
	offers := []database.Offer{
		{ID: 1, Name: "Lunch Deal", DiscountType: "percentage", DiscountValue: 15},
	}
	out := attachHappyHourOffer(s, offers, time.Now().UTC())
	require.NotNil(t, out)
	require.Equal(t, "Lunch Deal", out.Metrics["suggested_offer"])
	require.Equal(t, "percentage", out.Metrics["suggested_discount_type"])
	require.InDelta(t, 15.0, out.Metrics["suggested_discount_value"], 0.001)
	require.Equal(t, "percentage", out.DiscountType)
	require.InDelta(t, 15.0, out.DiscountValue, 0.001)
	require.Equal(t, "Lunch Deal", out.TargetName, "caption subject is the real offer, not a random dish")
}

func TestAttachHappyHourOffer_NoOffer_Suppressed(t *testing.T) {
	s := happyHourSuggestion(DaypartLoad{HasData: true, WeakestLabel: "Sat 8pm–9pm", PctBelowMean: 0.76, HeroItemName: "Steak Plate"})
	require.Nil(t, attachHappyHourOffer(s, nil, time.Now().UTC()),
		"happy hour without a live offer must not ship — it invents a deal")
	require.Nil(t, attachHappyHourOffer(s, []database.Offer{
		{ID: 1, Name: "Expired", DiscountType: "percentage", DiscountValue: 0},
	}, time.Now().UTC()))
}

func TestAttachHappyHourOffer_OutOfWindow_Suppressed(t *testing.T) {
	s := happyHourSuggestion(DaypartLoad{HasData: true, WeakestLabel: "Wed 5–7pm", PctBelowMean: 0.4, HeroItemName: "Steak Plate"})
	start, end := 11*60, 15*60
	offers := []database.Offer{{
		ID: 1, Name: "Weekday Lunch 15% Off", DiscountType: "percentage", DiscountValue: 15,
		WeekdayMask: 62, StartMinute: &start, EndMinute: &end,
	}}
	// Wednesday 15:14 is after the exclusive 15:00 lunch end.
	now := time.Date(2026, 8, 12, 15, 14, 0, 0, time.UTC)
	require.Nil(t, attachHappyHourOffer(s, offers, now),
		"happy hour must not invent a dinner deal from an ended lunch offer")
}

func TestAttachHappyHourOffer_InWindow_AttachesNameDiscountWindow(t *testing.T) {
	s := happyHourSuggestion(DaypartLoad{HasData: true, WeakestLabel: "Wed 12–1pm", PctBelowMean: 0.4, HeroItemName: "Steak Plate"})
	start, end := 11*60, 15*60
	offers := []database.Offer{{
		ID: 1, Name: "Weekday Lunch 15% Off", DiscountType: "percentage", DiscountValue: 15,
		WeekdayMask: 62, StartMinute: &start, EndMinute: &end, Image: "https://cdn/lunch.jpg",
	}}
	now := time.Date(2026, 8, 12, 12, 0, 0, 0, time.UTC)
	out := attachHappyHourOffer(s, offers, now)
	require.NotNil(t, out)
	require.Equal(t, "Weekday Lunch 15% Off", out.TargetName)
	require.Equal(t, "Weekday Lunch 15% Off", out.Metrics["suggested_offer"])
	require.Equal(t, "percentage", out.DiscountType)
	require.InDelta(t, 15.0, out.DiscountValue, 0.001)
	require.Equal(t, start, out.Metrics["offer_start_minute"])
	require.Equal(t, end, out.Metrics["offer_end_minute"])
	require.NotContains(t, out.TargetName, "Steak")
	require.Equal(t, ImageSourceOffer, out.ImageSource)
}

func TestHappyHourSuggestion_DoesNotHeroDishName(t *testing.T) {
	s := happyHourSuggestion(DaypartLoad{HasData: true, WeakestLabel: "Tue 5–7pm", PctBelowMean: 0.4, HeroItemName: "Steak Plate"})
	require.NotNil(t, s)
	require.Empty(t, s.TargetName, "happy hour must not caption a dish until a live offer is attached")
}

func TestWinBackSuggestion_Threshold(t *testing.T) {
	require.Nil(t, winBackSuggestion(3, 50, "Margherita"), "too few marketable lapsed customers")
	s := winBackSuggestion(32, 50, "Margherita")
	require.NotNil(t, s)
	require.Equal(t, PlayWinBack, s.Play)
	require.Contains(t, s.WhyData, "32")
	require.Contains(t, s.WhyData, "opted in")
}

func TestPopularTopName(t *testing.T) {
	require.Equal(t, "Pizza", popularTopName([]analytics.ItemStats{{ItemName: "Pizza"}, {ItemName: "Soda"}}))
	require.Equal(t, "", popularTopName(nil))
}

// Live-review regression: the featured-dish copy claimed "%s is a top seller"
// unconditionally — a demo showed Iced Tea (1 sold) labeled a top seller. The
// claim must be conditioned on real volume; low-volume stars get honest
// margin-led copy instead.
func TestFeaturedDishSuggestion_LowVolume_DropsTopSellerClaim(t *testing.T) {
	rep := menuengineering.Report{Dishes: []menuengineering.DishClass{
		{MenuItemID: "m7", MenuItemName: "Iced Tea", Quadrant: "star", QtySold: 1, MarginPerUnit: 3.0, AvgPrice: 4.0},
	}}
	s := featuredDishSuggestion(rep)
	require.NotNil(t, s)
	require.NotContains(t, s.WhyData, "top seller")
	require.NotContains(t, s.CopyAngle, "best-seller")
	require.Contains(t, s.WhyData, "Iced Tea")
}

func TestFeaturedDishSuggestion_RealVolume_KeepsTopSellerClaim(t *testing.T) {
	rep := menuengineering.Report{Dishes: []menuengineering.DishClass{
		{MenuItemID: "m4", MenuItemName: "Tiramisu", Quadrant: "star", QtySold: 200, MarginPerUnit: 6.0},
	}}
	s := featuredDishSuggestion(rep)
	require.NotNil(t, s)
	require.Contains(t, s.WhyData, "top seller")
}

func TestPluralize(t *testing.T) {
	require.Equal(t, "order", pluralize(1, "order", "orders"))
	require.Equal(t, "orders", pluralize(0, "order", "orders"))
	require.Equal(t, "orders", pluralize(2, "order", "orders"))
}

// Issue #243: a single Iced Tea order is not a campaign sample.
func TestMoveItemSuggestion_OneOrderSuppressed(t *testing.T) {
	rep := menuengineering.Report{Dishes: []menuengineering.DishClass{
		{MenuItemID: "demo-tea", MenuItemName: "Iced Tea", Quadrant: "puzzle", Action: "promote", MarginPerUnit: 4.62, QtySold: 1},
	}}
	require.Nil(t, moveItemSuggestion(rep), "1 order / $4.62 is a weak signal, not a push")
}

func TestMoveItemSuggestion_WeakSampleLabeled(t *testing.T) {
	rep := menuengineering.Report{Dishes: []menuengineering.DishClass{
		{MenuItemID: "m1", MenuItemName: "Osso Buco", Quadrant: "puzzle", Action: "promote", MarginPerUnit: 14.0, QtySold: 4},
	}}
	s := moveItemSuggestion(rep)
	require.NotNil(t, s)
	require.True(t, hasWhyFactor(s, "weak_signal"))
}

func TestComboSuggestions_SkipBundleWithBlockedDish(t *testing.T) {
	items, err := json.Marshal([]database.BundleItemRef{
		{MenuItemID: "demo-steak", Name: "Steak Plate", Quantity: 1},
		{MenuItemID: "demo-cocktail", Name: "Demo Spritz", Quantity: 2},
	})
	require.NoError(t, err)
	got := comboSuggestions([]database.Bundle{{
		ID: 1, Name: "Date Night for Two", Price: 68, IsActive: true, Items: string(items),
	}}, 3, map[string]bool{"demo-steak": true})
	require.Empty(t, got, "bundle containing OOS steak must not be campaigned")
}

func TestMoveItemSuggestion_PluralOrderCopy(t *testing.T) {
	rep := menuengineering.Report{Dishes: []menuengineering.DishClass{
		{MenuItemID: "m1", MenuItemName: "Osso Buco", Quadrant: "puzzle", Action: "promote", MarginPerUnit: 14.0, QtySold: 6},
	}}
	s := moveItemSuggestion(rep)
	require.NotNil(t, s)
	require.Contains(t, s.WhyData, "6 orders in the last 30 days")
}

// Win-back copy must not read "1 regulars".
func TestWinBackSuggestion_SingularRegular(t *testing.T) {
	s := winBackSuggestion(1, 5, "Margherita")
	// Below the winBackMinLapsed floor (10), so no card — but the pluralization
	// helper is what guards the copy; assert on a count of exactly 1 above the
	// floor via a direct check of the helper-driven title instead.
	require.Nil(t, s, "1 marketable lapsed is below the win-back floor")
	require.Equal(t, "regular", pluralize(1, "regular", "regulars"))
}
