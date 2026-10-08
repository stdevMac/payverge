package server

import (
	"reflect"
	"strings"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/demomode"
)

// demoStorefrontTextMaxRunes caps the storefront free text a public-demo
// visitor can write; see demomode.MaxStorefrontTextRunes.
const demoStorefrontTextMaxRunes = demomode.MaxStorefrontTextRunes

const demoStorefrontIdentityAction = "changing the venue name, logo, address, contact details, images, website, social links, custom URL or payout wallet addresses"

var demoStorefrontTextAction = demomode.StorefrontTextAction

// demoEditableBusinessFields is the DEMO_MODE allowlist for UpdateBusiness,
// keyed by the UpdateBusinessRequest JSON tag. Only these fields may change in
// the public demo. Every string among them is capped at
// demoStorefrontTextMaxRunes whenever the request sets it, changed or not, so
// a long value written some other way cannot be re-saved through here either.
//
// Any request field that is not listed here is refused when it changes,
// including fields added to UpdateBusinessRequest later. Listing a field in
// demoFrozenBusinessFields documents that the refusal is deliberate;
// TestDemoBusinessFieldPolicyCoversEveryRequestField fails on a field that is
// in neither map, so each new field gets a decision.
var demoEditableBusinessFields = map[string]struct{}{
	"tax_rate":                      {},
	"service_fee_rate":              {},
	"tax_inclusive":                 {},
	"service_inclusive":             {},
	"show_reviews":                  {},
	"google_reviews_enabled":        {},
	"default_currency":              {},
	"display_currency":              {},
	"timezone":                      {},
	"service_day_start_minute":      {},
	"business_type":                 {},
	"show_welcome_message":          {},
	"show_about_story":              {},
	"show_gallery":                  {},
	"show_operating_hours":          {},
	"show_special_features":         {},
	"ai_enabled":                    {},
	"ai_name":                       {}, // NormalizeAndValidateAiName bounds it
	"ai_priority":                   {},
	"business_page_ai_enabled":      {},
	"default_qr_foreground_color":   {},
	"default_qr_background_color":   {},
	"default_qr_logo_size":          {},
	"default_qr_show_business_name": {},
	"default_qr_show_table_name":    {},
	"default_qr_text_font":          {},

	"description":             {},
	"welcome_message":         {},
	"about_story":             {},
	"ai_special_instructions": {},
}

// demoFrozenBusinessFields are the fields deliberately refused in DEMO_MODE:
// the venue's identity, contact details, outbound links and images, and the
// payout wallets. They show on the public storefront and the demo banner, and
// the public demo hands the same owner session to everyone. Each func reports
// whether the request changes the stored value; it runs only for a field the
// request sets. An unchanged value passes, because the settings form re-sends
// every field on save.
var demoFrozenBusinessFields = map[string]func(*UpdateBusinessRequest, *database.Business) bool{
	"name":    func(r *UpdateBusinessRequest, b *database.Business) bool { return r.Name != b.Name },
	"logo":    func(r *UpdateBusinessRequest, b *database.Business) bool { return r.Logo != b.Logo },
	"address": func(r *UpdateBusinessRequest, b *database.Business) bool { return *r.Address != b.Address },
	"settlement_address": func(r *UpdateBusinessRequest, b *database.Business) bool {
		return walletDiffers(r.SettlementAddr, b.SettlementAddr)
	},
	"tipping_address": func(r *UpdateBusinessRequest, b *database.Business) bool {
		return walletDiffers(r.TippingAddr, b.TippingAddr)
	},
	// Turning the public page off would take the shared storefront down for
	// every visitor until the nightly reset.
	"business_page_enabled": func(r *UpdateBusinessRequest, b *database.Business) bool {
		return *r.BusinessPageEnabled != b.BusinessPageEnabled
	},
	"custom_url":    func(r *UpdateBusinessRequest, b *database.Business) bool { return *r.CustomURL != b.CustomURL },
	"phone":         func(r *UpdateBusinessRequest, b *database.Business) bool { return *r.Phone != b.Phone },
	"website":       func(r *UpdateBusinessRequest, b *database.Business) bool { return *r.Website != b.Website },
	"social_media":  func(r *UpdateBusinessRequest, b *database.Business) bool { return *r.SocialMedia != b.SocialMedia },
	"banner_images": func(r *UpdateBusinessRequest, b *database.Business) bool { return *r.BannerImages != b.BannerImages },
	"default_qr_logo_url": func(r *UpdateBusinessRequest, b *database.Business) bool {
		return *r.DefaultQRLogoURL != b.DefaultQRLogoURL
	},
}

func walletDiffers(next, current string) bool {
	return !strings.EqualFold(strings.TrimSpace(next), strings.TrimSpace(current))
}

// businessRequestFieldTag returns the JSON name of an UpdateBusinessRequest field.
func businessRequestFieldTag(f reflect.StructField) string {
	name, _, _ := strings.Cut(f.Tag.Get("json"), ",")
	if name == "" {
		return f.Name
	}
	return name
}

// demoStorefrontRefusal reports whether an UpdateBusiness request must be
// refused in DEMO_MODE, and which action the refusal names. It walks every
// field the request sets: allowlisted fields pass (free text up to the cap),
// frozen fields pass only unchanged, and any other set field is refused.
func demoStorefrontRefusal(req *UpdateBusinessRequest, b *database.Business) (demomode.Kind, string, bool) {
	v := reflect.ValueOf(req).Elem()
	t := v.Type()
	for i := 0; i < t.NumField(); i++ {
		fv := v.Field(i)
		var text string
		switch fv.Kind() {
		case reflect.Pointer:
			if fv.IsNil() {
				continue
			}
			if e := fv.Elem(); e.Kind() == reflect.String {
				text = e.String()
			}
		case reflect.String:
			// Plain string fields treat "" as "leave unchanged" in UpdateBusiness.
			if fv.String() == "" {
				continue
			}
			text = fv.String()
		default:
			if fv.IsZero() {
				continue
			}
		}

		tag := businessRequestFieldTag(t.Field(i))
		if _, ok := demoEditableBusinessFields[tag]; ok {
			if demomode.TextTooLong(text) {
				return demomode.KindStorefront, demoStorefrontTextAction, true
			}
			continue
		}
		if changed, ok := demoFrozenBusinessFields[tag]; ok && !changed(req, b) {
			continue
		}
		return demomode.KindStorefront, demoStorefrontIdentityAction, true
	}
	return "", "", false
}

// demoHospitalityTextRefusal applies the same cap to the hospitality-settings
// route, which writes the welcome message and about story outside
// UpdateBusiness.
func demoHospitalityTextRefusal(welcomeMessage, aboutStory string) bool {
	return demomode.TextTooLong(welcomeMessage) || demomode.TextTooLong(aboutStory)
}

// demoSpecialFeaturesTextRefusal caps each special feature's title and
// description, the storefront "Why choose us" texts, in DEMO_MODE.
func demoSpecialFeaturesTextRefusal(features []database.BusinessSpecialFeature) bool {
	for _, f := range features {
		if demomode.TextTooLong(f.Title) || demomode.TextTooLong(f.Description) || demomode.TextTooLong(f.Icon) {
			return true
		}
	}
	return false
}

// demoQRLogoAllowed reports whether a QR logo URL may be written in DEMO_MODE.
// Table QR codes and the bulk "apply to all" branding write the same outbound
// image URL as default_qr_logo_url, so a visitor may only clear it, keep it,
// or reuse the venue's own frozen logo or QR default; never a new URL.
func demoQRLogoAllowed(next string, b *database.Business, current string) bool {
	return next == "" || next == current || next == b.DefaultQRLogoURL || next == b.Logo
}
