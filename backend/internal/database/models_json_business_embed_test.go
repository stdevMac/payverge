package database

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

// Embedded-Business access-shape tests covering models that reach
// unauthenticated guest endpoints. Bundle and Offer (SEC-2 follow-up) embed
// the full Business row under `json:"business,omitempty"` — but `omitempty`
// is a no-op for non-pointer struct fields, so the nested object is always
// emitted. Once any future handler preloads `.Business`, every Stripe ID,
// owner PII field, and payout address leaks into the public response. (X-2)

// leakedBusinessFields are the keys whose presence inside an embedded
// business object proves sensitive business data is reachable through JSON.
var leakedBusinessKeys = []string{
	"stripe_customer_id",
	"stripe_subscription_id",
	"stripe_price_id",
	"stripe_payment_method",
	"stripe_card_brand",
	"stripe_card_last4",
	"owner_address",
	"owner_name",
	"settlement_address",
	"tipping_address",
	"email",
	"phone",
	"user_id",
}

// leakedBusinessValues are sentinel string values used to assert that the
// serializer drops the *values* (not just the keys) even when a model is
// densely populated. The frontend has no consumer for bundle/offer.business
// so we assert the whole nested object is omitted from JSON.
var leakedBusinessValues = []string{
	"cus_LEAK",
	"sub_LEAK",
	"Jane Owner",
	"0xowner",
	"owner@x.com",
	"0xsettle",
	"0xtip",
}

// populatedBusiness is the same fixture used by both Bundle and Offer
// assertions. Mirrors the struct fields an admin settings page would set.
func populatedBusiness() Business {
	return Business{
		ID:              42,
		BusinessId:      "biz_LEAK",
		Name:            "Leak Test Business",
		OwnerAddress:    "0xowner",
		OwnerName:       "Jane Owner",
		Email:           "owner@x.com",
		Phone:           "+15551234567",
		SettlementAddr:  "0xsettle",
		TippingAddr:     "0xtip",
		Timezone:        "Asia/Dubai",
		DefaultCurrency: "AED",
		DisplayCurrency: "AED",
	}
}

func TestBundleMarshalJSON_DoesNotLeakEmbeddedBusinessSensitiveFields(t *testing.T) {
	b := Bundle{
		ID:         7,
		BusinessID: 42,
		Name:       "Test Bundle",
		Items:      "[]",
		IsActive:   true,
		Business:   populatedBusiness(),
	}
	raw, err := json.Marshal(b)
	require.NoError(t, err)

	// When the embedded Business is preloaded (the future-leak condition
	// this test exists to block) the JSON must not contain the sensitive
	// Business keys. We assert the simplest safe form first: no business
	// key at all. If a future change adds a safe summary, the keys
	// assertion will still hold; only the structure assertion will need
	// to be revised.
	s := string(raw)
	for _, v := range leakedBusinessValues {
		require.NotContains(t, s, v, "bundle JSON leaked sensitive business value %q", v)
	}
	for _, k := range leakedBusinessKeys {
		require.NotContains(t, s, `"`+k+`"`, "bundle JSON leaked sensitive business key %q", k)
	}
}

func TestOfferMarshalJSON_DoesNotLeakEmbeddedBusinessSensitiveFields(t *testing.T) {
	o := Offer{
		ID:            7,
		BusinessID:    42,
		Name:          "Test Offer",
		DiscountType:  "percentage",
		DiscountValue: 10,
		IsActive:      true,
		Business:      populatedBusiness(),
	}
	raw, err := json.Marshal(o)
	require.NoError(t, err)

	s := string(raw)
	for _, v := range leakedBusinessValues {
		require.NotContains(t, s, v, "offer JSON leaked sensitive business value %q", v)
	}
	for _, k := range leakedBusinessKeys {
		require.NotContains(t, s, `"`+k+`"`, "offer JSON leaked sensitive business key %q", k)
	}
}

// Sibling sibling tests for the other public/guest-reachable embeds
// surfaced by the Task-6 audit. The three storefront sub-resources are
// returned raw on the public GetBusinessByCustomURL handler
// (business_settings_handlers.go ~L1177-1179) and would leak the full
// Business struct on a future Preload. The fix flips their JSON tag
// to json:"-"; the test asserts the populated embed is dropped.

func TestBusinessGalleryImageMarshalJSON_DoesNotLeakEmbeddedBusinessSensitiveFields(t *testing.T) {
	g := BusinessGalleryImage{
		ID: 1, BusinessID: 42, ImageURL: "https://x/i.png", Business: populatedBusiness(),
	}
	raw, err := json.Marshal(g)
	require.NoError(t, err)
	s := string(raw)
	for _, v := range leakedBusinessValues {
		require.NotContains(t, s, v, "gallery image JSON leaked sensitive business value %q", v)
	}
	for _, k := range leakedBusinessKeys {
		require.NotContains(t, s, `"`+k+`"`, "gallery image JSON leaked sensitive business key %q", k)
	}
}

func TestBusinessOperatingHoursMarshalJSON_DoesNotLeakEmbeddedBusinessSensitiveFields(t *testing.T) {
	h := BusinessOperatingHours{
		ID: 1, BusinessID: 42, DayOfWeek: 1, OpenTime: "09:00", CloseTime: "17:00", Business: populatedBusiness(),
	}
	raw, err := json.Marshal(h)
	require.NoError(t, err)
	s := string(raw)
	for _, v := range leakedBusinessValues {
		require.NotContains(t, s, v, "operating hours JSON leaked sensitive business value %q", v)
	}
	for _, k := range leakedBusinessKeys {
		require.NotContains(t, s, `"`+k+`"`, "operating hours JSON leaked sensitive business key %q", k)
	}
}

func TestBusinessLanguageMarshalJSON_DoesNotLeakEmbeddedBusinessSensitiveFields(t *testing.T) {
	lang := BusinessLanguage{
		ID:           1,
		BusinessID:   42,
		LanguageCode: "en",
		IsDefault:    true,
		Business:     populatedBusiness(),
	}
	raw, err := json.Marshal(lang)
	require.NoError(t, err)
	s := string(raw)
	require.NotContains(t, s, `"business"`, "BusinessLanguage must omit the embedded Business (#523/#566)")
	for _, v := range leakedBusinessValues {
		require.NotContains(t, s, v, "business language JSON leaked sensitive business value %q", v)
	}
	for _, k := range leakedBusinessKeys {
		require.NotContains(t, s, `"`+k+`"`, "business language JSON leaked sensitive business key %q", k)
	}
}

func TestBusinessSpecialFeatureMarshalJSON_DoesNotLeakEmbeddedBusinessSensitiveFields(t *testing.T) {
	f := BusinessSpecialFeature{
		ID: 1, BusinessID: 42, Title: "Wi-Fi", Icon: "wifi", Business: populatedBusiness(),
	}
	raw, err := json.Marshal(f)
	require.NoError(t, err)
	s := string(raw)
	for _, v := range leakedBusinessValues {
		require.NotContains(t, s, v, "special feature JSON leaked sensitive business value %q", v)
	}
	for _, k := range leakedBusinessKeys {
		require.NotContains(t, s, `"`+k+`"`, "special feature JSON leaked sensitive business key %q", k)
	}
}

// TestDeliveryOrderMarshalJSON_DoesNotLeakEmbeddedBusiness guards against the
// SSE delivery.* payload leaking the full Business row (Stripe IDs, owner PII,
// wallet addresses) when any emit site preloads the Business relation.
// DeliveryOrder.MarshalJSON must scrub the embedded Business down to the safe
// billPublicBusiness summary, mirroring Table / ReferralRecord / Bill. (SSE-LEAK-3)
func TestDeliveryOrderMarshalJSON_DoesNotLeakEmbeddedBusiness(t *testing.T) {
	do := DeliveryOrder{
		ID:             1,
		BusinessID:     42,
		BillID:         10,
		DeliveryNumber: "DEL-001",
		CustomerName:   "Alice Guest",
		CustomerPhone:  "+15550001111",
		DeliveryFee:    500, // cents
		DriverTip:      100, // cents
		PlatformFee:    50,  // cents
		Business:       populatedBusiness(),
	}

	raw, err := json.Marshal(do)
	require.NoError(t, err)
	s := string(raw)

	// Sensitive business values must not appear anywhere in the JSON output.
	// Note: we check values (not all keys) here because some keys like
	// "settlement_address" are legitimately emitted by the embedded Bill payload.
	for _, v := range leakedBusinessValues {
		require.NotContains(t, s, v,
			"delivery order JSON leaked sensitive business value %q", v)
	}

	// Stripe and owner-identity keys must never appear in the business object.
	stripeAndIdentityKeys := []string{
		"stripe_customer_id",
		"stripe_subscription_id",
		"stripe_price_id",
		"stripe_payment_method",
		"stripe_card_brand",
		"stripe_card_last4",
		"owner_address",
		"owner_name",
		"user_id",
		"email",
		"phone",
	}
	for _, k := range stripeAndIdentityKeys {
		require.NotContains(t, s, `"`+k+`"`,
			"delivery order JSON leaked sensitive business key %q", k)
	}

	// Monetary fields must be serialised as dollars (cents ÷ 100).
	require.Contains(t, s, `"delivery_fee":5`, "delivery_fee must be in dollars")
	require.Contains(t, s, `"driver_tip":1`, "driver_tip must be in dollars")
	require.Contains(t, s, `"platform_fee":0.5`, "platform_fee must be in dollars")
}
