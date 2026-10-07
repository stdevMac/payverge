package services

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/stdevmac/payverge/backend/internal/database"
)

// auditedFalseHoursClaim is the production incident fixture: a slowest-window
// insight was rewritten into a public opening-hours claim that was not true.
const auditedFalseHoursClaim = "We're open from 8am–9am every Saturday"

func TestGuardMarketingClaims_RejectsAuditedFalseOpeningHours(t *testing.T) {
	// Business is open Sat 11:00–22:00 — not 8–9am. The audited caption must
	// never be publishable under this record.
	facts := MarketingClaimFacts{
		Hours: []MarketingHourWindow{
			{DayOfWeek: 6, OpenTime: "11:00", CloseTime: "22:00"},
		},
	}
	ok, violations := GuardMarketingClaims(auditedFalseHoursClaim, facts)
	require.False(t, ok, "false opening-hours claim must be rejected")
	require.NotEmpty(t, violations)
	assert.Equal(t, "hours", violations[0].Kind)
	assert.Contains(t, strings.ToLower(violations[0].Claim), "open")
}

func TestGuardMarketingClaims_RejectsHoursWhenNoHoursConfigured(t *testing.T) {
	// Fail closed: any hours claim without ground-truth hours is invented.
	ok, violations := GuardMarketingClaims(auditedFalseHoursClaim, MarketingClaimFacts{})
	require.False(t, ok)
	require.NotEmpty(t, violations)
	assert.Equal(t, "hours", violations[0].Kind)
}

func TestGuardMarketingClaims_AcceptsMatchingOpeningHours(t *testing.T) {
	facts := MarketingClaimFacts{
		Hours: []MarketingHourWindow{
			{DayOfWeek: 6, OpenTime: "08:00", CloseTime: "09:00"},
		},
	}
	ok, violations := GuardMarketingClaims(auditedFalseHoursClaim, facts)
	require.True(t, ok, "matching hours must pass; violations=%v", violations)
	assert.Empty(t, violations)
}

func TestGuardMarketingClaims_RejectsClosedDayHoursClaim(t *testing.T) {
	facts := MarketingClaimFacts{
		Hours: []MarketingHourWindow{
			{DayOfWeek: 6, IsClosed: true},
		},
	}
	ok, _ := GuardMarketingClaims("Join us Saturday — open from 5pm to 11pm", facts)
	require.False(t, ok)
}

func TestGuardMarketingClaims_RejectsFalsePrice(t *testing.T) {
	facts := MarketingClaimFacts{
		Prices: []float64{14.50, 22.00},
	}
	// Caption invents a $9 price that is not on the menu.
	ok, violations := GuardMarketingClaims("Try our risotto for only $9 tonight! #dinner", facts)
	require.False(t, ok)
	require.NotEmpty(t, violations)
	assert.Equal(t, "price", violations[0].Kind)
}

func TestGuardMarketingClaims_AcceptsMenuPrice(t *testing.T) {
	facts := MarketingClaimFacts{
		Prices: []float64{14.50, 22.00},
	}
	ok, violations := GuardMarketingClaims("Tonight's risotto is $14.50 — come hungry. #dinner", facts)
	require.True(t, ok, "violations=%v", violations)
}

func TestGuardMarketingClaims_RejectsPriceWhenMenuEmpty(t *testing.T) {
	// Fail closed: a priced claim with no known prices is invented.
	ok, violations := GuardMarketingClaims("Only €12 this week!", MarketingClaimFacts{})
	require.False(t, ok)
	assert.Equal(t, "price", violations[0].Kind)
}

func TestGuardMarketingClaims_IgnoresNonPriceNumbers(t *testing.T) {
	// "18 times" / customer counts must not be treated as prices.
	ok, violations := GuardMarketingClaims(
		"Our medialunas sold 18 times this week — guests love them. #pastry",
		MarketingClaimFacts{},
	)
	require.True(t, ok, "non-price counts must not trip the guard; violations=%v", violations)
}

func TestGuardMarketingClaims_RejectsFalseOfferPercent(t *testing.T) {
	facts := MarketingClaimFacts{
		Offers: []MarketingOfferFact{
			{Name: "Lunch Deal", DiscountType: "percentage", DiscountValue: 15},
		},
	}
	ok, violations := GuardMarketingClaims("Happy hour: 50% off all drinks!", facts)
	require.False(t, ok)
	assert.Equal(t, "offer", violations[0].Kind)
}

func TestGuardMarketingClaims_AcceptsMatchingOfferPercent(t *testing.T) {
	facts := MarketingClaimFacts{
		Offers: []MarketingOfferFact{
			{Name: "Lunch Deal", DiscountType: "percentage", DiscountValue: 15},
		},
	}
	ok, violations := GuardMarketingClaims("Lunch Deal is live: 15% off selected plates. #lunch", facts)
	require.True(t, ok, "violations=%v", violations)
}

func TestGuardMarketingClaims_RejectsOfferWhenNoneActive(t *testing.T) {
	ok, violations := GuardMarketingClaims("Get 20% off this weekend only!", MarketingClaimFacts{})
	require.False(t, ok)
	assert.Equal(t, "offer", violations[0].Kind)
}

func TestGuardMarketingClaims_RejectsFalsePhone(t *testing.T) {
	facts := MarketingClaimFacts{Phone: "+1 415 555 0100"}
	ok, violations := GuardMarketingClaims("Reserve now: call +1 212 555 9999", facts)
	require.False(t, ok)
	assert.Equal(t, "phone", violations[0].Kind)
}

func TestGuardMarketingClaims_AcceptsMatchingPhone(t *testing.T) {
	facts := MarketingClaimFacts{Phone: "+1 (415) 555-0100"}
	ok, violations := GuardMarketingClaims("Questions? Call us at +1 415 555 0100", facts)
	require.True(t, ok, "violations=%v", violations)
}

func TestGuardMarketingClaims_RejectsFalseAddress(t *testing.T) {
	facts := MarketingClaimFacts{
		Street: "100 Main Street",
		City:   "Rosario",
	}
	ok, violations := GuardMarketingClaims(
		"Find us at 999 Fake Boulevard, Buenos Aires this Saturday",
		facts,
	)
	require.False(t, ok)
	assert.Equal(t, "address", violations[0].Kind)
}

func TestGuardMarketingClaims_AcceptsMatchingAddressFragment(t *testing.T) {
	facts := MarketingClaimFacts{
		Street: "100 Main Street",
		City:   "Rosario",
	}
	ok, violations := GuardMarketingClaims(
		"Visit us at 100 Main Street in Rosario — walk-ins welcome.",
		facts,
	)
	require.True(t, ok, "violations=%v", violations)
}

func TestGuardMarketingClaims_AllowsCopyWithoutFactualClaims(t *testing.T) {
	ok, violations := GuardMarketingClaims(
		"Flaky, warm, and made this morning. Come taste the difference. #bakery",
		MarketingClaimFacts{},
	)
	require.True(t, ok)
	assert.Empty(t, violations)
}

func TestGuardMarketingClaims_SpanishHoursRejectedWhenMismatch(t *testing.T) {
	facts := MarketingClaimFacts{
		Hours: []MarketingHourWindow{
			{DayOfWeek: 6, OpenTime: "11:00", CloseTime: "23:00"},
		},
	}
	ok, violations := GuardMarketingClaims(
		"Abrimos de 8am a 9am todos los sábados. #brunch",
		facts,
	)
	require.False(t, ok)
	assert.Equal(t, "hours", violations[0].Kind)
}

func TestBuildMarketingClaimFacts_FromBusinessRecord(t *testing.T) {
	facts := BuildMarketingClaimFacts(
		"+15550100",
		database.BusinessAddress{Street: "12 Oak Ave", City: "Austin", State: "TX", PostalCode: "78701", Country: "US"},
		[]database.BusinessOperatingHours{
			{DayOfWeek: 1, OpenTime: "09:00", CloseTime: "17:00"},
			{DayOfWeek: 0, IsClosed: true},
		},
		[]database.Offer{
			{Name: "Happy Hour", DiscountType: "percentage", DiscountValue: 20, IsActive: true},
			{Name: "Draft", DiscountType: "fixed", DiscountValue: 5, IsActive: false},
		},
		[]float64{9.5, 18},
	)
	assert.Equal(t, "+15550100", facts.Phone)
	assert.Equal(t, "12 Oak Ave", facts.Street)
	assert.Equal(t, "Austin", facts.City)
	require.Len(t, facts.Hours, 2)
	require.Len(t, facts.Offers, 1, "only active offers")
	assert.Equal(t, "Happy Hour", facts.Offers[0].Name)
	assert.Equal(t, []float64{9.5, 18}, facts.Prices)
}

func TestStripMarketingClaimSentences_DropsViolatingSentence(t *testing.T) {
	// When a multi-sentence caption has one bad claim, drop that sentence and
	// keep the rest so the operator still gets usable copy.
	caption := "Flaky croissants, baked fresh. " + auditedFalseHoursClaim + ". See you soon!"
	facts := MarketingClaimFacts{
		Hours: []MarketingHourWindow{
			{DayOfWeek: 6, OpenTime: "11:00", CloseTime: "22:00"},
		},
	}
	cleaned, ok := SanitizeMarketingCaptionClaims(caption, facts)
	require.True(t, ok, "remaining copy should be publishable")
	assert.NotContains(t, cleaned, "8am")
	assert.Contains(t, cleaned, "Flaky croissants")
	assert.Contains(t, cleaned, "See you soon")
}

func TestSanitizeMarketingCaptionClaims_RejectsWhenOnlyClaimRemains(t *testing.T) {
	facts := MarketingClaimFacts{}
	cleaned, ok := SanitizeMarketingCaptionClaims(auditedFalseHoursClaim, facts)
	require.False(t, ok)
	assert.Empty(t, cleaned)
}
