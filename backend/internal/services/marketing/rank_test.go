package marketing

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/services/menuengineering"
)

func TestShouldSuppressUnavailable(t *testing.T) {
	meta := menuMetaIndex{
		byID: map[string]menuItemMeta{
			"m1": {Seen: true, Available: false, ImageURL: "https://cdn/x.jpg"},
			"m2": {Seen: true, Available: true, ImageURL: "https://cdn/y.jpg"},
		},
	}
	require.True(t, shouldSuppressUnavailable(&CampaignSuggestion{
		Play: PlayFeaturedDish, TargetItemID: "m1",
	}, meta), "explicitly 86'd menu item is suppressed")
	require.False(t, shouldSuppressUnavailable(&CampaignSuggestion{
		Play: PlayFeaturedDish, TargetItemID: "m2",
	}, meta), "available item is not suppressed")
	require.False(t, shouldSuppressUnavailable(&CampaignSuggestion{
		Play: PlayFeaturedDish, TargetItemID: "missing",
	}, meta), "unknown item is not suppressed (no data ≠ out of stock)")
	require.False(t, shouldSuppressUnavailable(&CampaignSuggestion{
		Play: PlayOffer, TargetItemID: "offer:4",
	}, meta), "offer targets never use menu availability")
	require.False(t, shouldSuppressUnavailable(&CampaignSuggestion{
		Play: PlayComboDeal, TargetItemID: "bundle:1",
	}, meta), "bundle targets never use menu availability")
	require.True(t, shouldSuppressUnavailable(&CampaignSuggestion{
		Play: PlayHappyHour, Metrics: map[string]any{"offer_target_item_id": "m1"},
	}, meta), "item-scoped happy-hour offer for an 86'd dish is suppressed")
}

func TestClearUnavailableHero(t *testing.T) {
	meta := menuMetaIndex{
		byName: map[string]menuItemMeta{
			"steak plate": {Seen: true, Available: false, ImageURL: "https://cdn/s.jpg"},
			"pasta":       {Seen: true, Available: true, ImageURL: "https://cdn/p.jpg"},
		},
	}
	s := &CampaignSuggestion{Play: PlayHappyHour, TargetName: "Steak Plate", ImageURL: "https://cdn/s.jpg", ImageSource: ImageSourceMenu}
	clearUnavailableHero(s, meta, nil)
	require.Empty(t, s.TargetName, "86'd hero name is cleared")
	require.Empty(t, s.ImageURL)
	require.Empty(t, s.ImageSource)

	keep := &CampaignSuggestion{Play: PlayHappyHour, TargetName: "Pasta", ImageURL: "https://cdn/p.jpg"}
	clearUnavailableHero(keep, meta, nil)
	require.Equal(t, "Pasta", keep.TargetName)
	require.Equal(t, "https://cdn/p.jpg", keep.ImageURL)
}

func TestShouldSuppressUnmakeable(t *testing.T) {
	meta := menuMetaIndex{
		byID: map[string]menuItemMeta{
			"steak": {ID: "steak", Seen: true, Available: true},
		},
		byName: map[string]menuItemMeta{
			"steak plate": {ID: "steak", Seen: true, Available: true},
		},
	}
	unmakeable := map[string]bool{"steak": true}
	require.True(t, shouldSuppressUnmakeable(&CampaignSuggestion{
		Play: PlayFeaturedDish, TargetItemID: "steak",
	}, meta, unmakeable))
	require.True(t, shouldSuppressUnmakeable(&CampaignSuggestion{
		Play: PlayHappyHour, TargetName: "Steak Plate",
	}, meta, unmakeable), "name-only hero for OOS dish is suppressed")
	tid := "steak"
	require.True(t, shouldSuppressUnmakeable(&CampaignSuggestion{
		Play: PlayOffer, TargetItemID: "offer:9",
		Metrics: map[string]any{"offer_target_item_id": tid},
	}, meta, unmakeable), "item-scoped offer for OOS dish is suppressed")
	require.False(t, shouldSuppressUnmakeable(&CampaignSuggestion{
		Play: PlayFeaturedDish, TargetItemID: "pasta",
	}, meta, unmakeable))
	require.False(t, shouldSuppressUnmakeable(&CampaignSuggestion{
		Play: PlayFeaturedDish, TargetItemID: "steak",
	}, meta, nil), "nil unmakeable set never suppresses")
}

func TestDedupeMenuTargets_KeepsHighestRank(t *testing.T) {
	in := []CampaignSuggestion{
		{ID: "1:move_item:m1", Play: PlayMoveItem, TargetItemID: "m1", Rank: 50},
		{ID: "1:featured_dish:m1", Play: PlayFeaturedDish, TargetItemID: "m1", Rank: 90},
		{ID: "1:offer:offer:9", Play: PlayOffer, TargetItemID: "offer:9", Rank: 55},
		{ID: "1:featured_dish:m2", Play: PlayFeaturedDish, TargetItemID: "m2", Rank: 70},
		{ID: "1:happy_hour:", Play: PlayHappyHour, TargetItemID: "", Rank: 80},
	}
	out := dedupeMenuTargets(in)
	// m1 collapsed to featured (90); m2 kept; offer kept; happy_hour kept.
	require.Len(t, out, 4)
	ids := make([]string, len(out))
	for i, s := range out {
		ids[i] = s.ID
	}
	require.Equal(t, []string{
		"1:featured_dish:m1",
		"1:happy_hour:",
		"1:featured_dish:m2",
		"1:offer:offer:9",
	}, ids)
}

func TestApplyImageBoost(t *testing.T) {
	s := &CampaignSuggestion{Rank: 60, ImageURL: "https://cdn/x.jpg"}
	applyImageBoost(s)
	require.InDelta(t, 65, s.Rank, 0.001)
	require.True(t, hasWhyFactor(s, "photo_ready"))

	bare := &CampaignSuggestion{Rank: 60}
	applyImageBoost(bare)
	require.InDelta(t, 60, bare.Rank, 0.001)
	require.False(t, hasWhyFactor(bare, "photo_ready"))
}

func TestHappyHourSuggestion_WeakWindowSuppressed(t *testing.T) {
	require.Nil(t, happyHourSuggestion(DaypartLoad{
		HasData: true, WeakestLabel: "Tue 3–4pm", PctBelowMean: 0.05,
	}), "pct below mean under 15% is noise")
	s := happyHourSuggestion(DaypartLoad{
		HasData: true, WeakestLabel: "Tue 5–7pm", PctBelowMean: 0.20,
	})
	require.NotNil(t, s)
	require.Equal(t, RankingVersionS2, s.RankingVersion)
	require.NotEmpty(t, s.WhyFactors)
	require.True(t, hasWhyFactor(s, "pct_below_mean"))
}

func TestMoveItemSuggestion_WeakMarginSuppressed(t *testing.T) {
	rep := menuengineering.Report{Dishes: []menuengineering.DishClass{
		{MenuItemID: "m1", MenuItemName: "Side", Action: "promote", MarginPerUnit: 0.25, QtySold: 5},
	}}
	require.Nil(t, moveItemSuggestion(rep), "sub-$1 margin promote is a weak play")
}

func TestMoveItemSuggestion_VelocityGapRanksHigher(t *testing.T) {
	rep := menuengineering.Report{
		MedianQtySold: 40,
		Dishes: []menuengineering.DishClass{
			{MenuItemID: "m1", MenuItemName: "Osso Buco", Action: "promote", MarginPerUnit: 14, QtySold: 5, AvgPrice: 28},
		},
	}
	s := moveItemSuggestion(rep)
	require.NotNil(t, s)
	require.True(t, hasWhyFactor(s, "velocity_gap"))
	// base 45 + margin 14 + gap min(35,30)=30 → 89
	require.InDelta(t, 89, s.Rank, 0.001)
	require.Equal(t, RankingVersionS2, s.RankingVersion)
}

func TestFeaturedDishSuggestion_ZeroSoldSuppressed(t *testing.T) {
	rep := menuengineering.Report{Dishes: []menuengineering.DishClass{
		{MenuItemID: "m1", MenuItemName: "Ghost Star", Quadrant: menuengineering.QuadrantStar, QtySold: 0, MarginPerUnit: 9},
	}}
	require.Nil(t, featuredDishSuggestion(rep), "zero-sold stars are classification noise")
}

func TestFeaturedDishSuggestion_WhyFactorsAndVersion(t *testing.T) {
	rep := menuengineering.Report{Dishes: []menuengineering.DishClass{
		{MenuItemID: "m1", MenuItemName: "Carbonara", Quadrant: menuengineering.QuadrantStar, QtySold: 50, MarginPerUnit: 9, AvgPrice: 12.99},
	}}
	s := featuredDishSuggestion(rep)
	require.NotNil(t, s)
	require.Equal(t, RankingVersionS2, s.RankingVersion)
	require.True(t, hasWhyFactor(s, "qty_sold"))
	require.True(t, hasWhyFactor(s, "margin_per_unit"))
	// 60 + min(50,100) + min(9,20) = 119
	require.InDelta(t, 119, s.Rank, 0.001)
}

func TestOfferSuggestions_SuppressesExpiredAndZero(t *testing.T) {
	now := time.Date(2026, 7, 14, 12, 0, 0, 0, time.UTC)
	past := now.Add(-24 * time.Hour)
	futureStart := now.Add(48 * time.Hour)
	endingSoon := now.Add(48 * time.Hour)

	offers := []database.Offer{
		{ID: 1, Name: "Expired", DiscountType: "percentage", DiscountValue: 20, EndDate: &past},
		{ID: 2, Name: "Zero", DiscountType: "percentage", DiscountValue: 0},
		{ID: 3, Name: "Future", DiscountType: "percentage", DiscountValue: 15, StartDate: &futureStart},
		{ID: 4, Name: "Live Soon", DiscountType: "percentage", DiscountValue: 25, Image: "https://cdn/o.jpg", EndDate: &endingSoon},
	}
	got := offerSuggestions(offers, 5, now)
	require.Len(t, got, 1)
	require.Equal(t, "Live Soon", got[0].TargetName)
	require.True(t, hasWhyFactor(got[0], "ending_soon"), "ending within 7 days gets urgency factor")
	require.True(t, hasWhyFactor(got[0], "photo_ready"))
	require.Greater(t, got[0].Rank, 55.0, "urgency + image boost above base")
}

func TestOfferIsLive_Table(t *testing.T) {
	now := time.Date(2026, 7, 14, 12, 0, 0, 0, time.UTC)
	past := now.Add(-time.Hour)
	future := now.Add(time.Hour)

	tests := []struct {
		name string
		o    database.Offer
		want bool
	}{
		{name: "live open-ended", o: database.Offer{DiscountValue: 10}, want: true},
		{name: "zero discount", o: database.Offer{DiscountValue: 0}, want: false},
		{name: "expired", o: database.Offer{DiscountValue: 10, EndDate: &past}, want: false},
		{name: "not started", o: database.Offer{DiscountValue: 10, StartDate: &future}, want: false},
		{name: "in window", o: database.Offer{DiscountValue: 10, StartDate: &past, EndDate: &future}, want: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, offerIsLive(tt.o, now))
		})
	}
}

func TestWinBackSuggestion_WhyFactorsAndOptInShare(t *testing.T) {
	s := winBackSuggestion(20, 40, "Margherita")
	require.NotNil(t, s)
	require.True(t, hasWhyFactor(s, "marketable_lapsed"))
	require.True(t, hasWhyFactor(s, "opt_in_share"))
	// 50 + 20 + 0.5*10 = 75
	require.InDelta(t, 75, s.Rank, 0.001)
	require.InDelta(t, 0.5, s.Metrics["opt_in_share"], 0.001)
}

func TestIsPlainMenuItemID(t *testing.T) {
	require.True(t, isPlainMenuItemID("m1"))
	require.False(t, isPlainMenuItemID(""))
	require.False(t, isPlainMenuItemID("offer:4"))
	require.False(t, isPlainMenuItemID("bundle:7"))
}

func TestStripUnattributedPhoto_GenericOfferDropsMenuDish(t *testing.T) {
	menuImages := map[string]struct{}{"https://cdn/bowl.jpg": {}}
	generic := &CampaignSuggestion{
		Play: PlayOffer, TargetItemID: "offer:1", TargetName: "Weekday Lunch 15% Off",
		ImageURL: "https://cdn/bowl.jpg", ImageSource: ImageSourceOffer, Rank: 60,
		WhyFactors: []WhyFactor{{Key: "photo_ready", Value: "1", Weight: imageRankBoost}},
	}
	stripUnattributedPhoto(generic, menuImages)
	require.Empty(t, generic.ImageURL, "generic offer must not borrow Harvest Bowl's plate photo")
	require.Empty(t, generic.ImageSource)
	require.False(t, hasWhyFactor(generic, "photo_ready"))
	require.InDelta(t, 55, generic.Rank, 0.001)

	itemOffer := &CampaignSuggestion{
		Play: PlayOffer, TargetItemID: "offer:2", TargetName: "$5 Off the Steak Plate",
		ImageURL: "https://cdn/steak.jpg", ImageSource: ImageSourceOffer,
		Metrics: map[string]any{"offer_target_item_id": "steak"},
	}
	stripUnattributedPhoto(itemOffer, map[string]struct{}{"https://cdn/steak.jpg": {}})
	require.Equal(t, "https://cdn/steak.jpg", itemOffer.ImageURL, "item-scoped offer keeps its dish photo")
}

func TestDedupeSharedPhotos_KeepsFirstClearsLater(t *testing.T) {
	in := []CampaignSuggestion{
		{ID: "offer", Play: PlayOffer, ImageURL: "https://cdn/steak.jpg", ImageSource: ImageSourceOffer, Rank: 70},
		{ID: "hh", Play: PlayHappyHour, ImageURL: "https://cdn/steak.jpg", ImageSource: ImageSourceOffer, Rank: 50},
		{ID: "bowl", Play: PlayFeaturedDish, ImageURL: "https://cdn/bowl.jpg", ImageSource: ImageSourceMenu, Rank: 40},
	}
	dedupeSharedPhotos(in)
	require.Equal(t, "https://cdn/steak.jpg", in[0].ImageURL)
	require.Empty(t, in[1].ImageURL, "happy hour must not reuse the steak tray already on the offer card")
	require.Equal(t, "https://cdn/bowl.jpg", in[2].ImageURL)
}

func TestInventoryBlockName_PrefersDishOverOfferTitle(t *testing.T) {
	meta := menuMetaIndex{byID: map[string]menuItemMeta{
		"demo-steak": {ID: "demo-steak", Name: "Steak Plate", Seen: true, Available: false},
	}}
	require.Equal(t, "Steak Plate", inventoryBlockName(&CampaignSuggestion{
		Play: PlayOffer, TargetName: "$5 Off the Steak Plate",
		Metrics: map[string]any{"offer_target_item_id": "demo-steak"},
	}, meta))
}
