package services

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/stdevmac/payverge/backend/internal/database"

	"gorm.io/gorm"
)

// #871: the Director had NO delivery evidence in its grounding. Delivery is not
// a plugin, so the plugin block the snapshot already carries says nothing about
// it, and the model filled the hole with "you have no delivery partners, go to
// Plugins" — on a venue whose Delivery settings already list PedidosYa and
// Rappi with a configured fee. The host and the guest storefront were giving
// opposite answers about whether the venue delivers at all.

// directorDeliveryConfig is the Director's delivery evidence: the same
// toggles, partner list and money the Delivery settings screen shows.
//
// Known distinguishes "this venue does not deliver" from "we could not read
// the delivery settings" — only the first is safe to state as fact.
type directorDeliveryConfig struct {
	Known             bool `json:"known"`
	Enabled           bool `json:"enabled"`
	InHouseEnabled    bool `json:"in_house_enabled"`
	ThirdPartyEnabled bool `json:"third_party_enabled"`
	// Partners are the marketplaces the operator already configured
	// (PedidosYa, Rappi, Uber Eats…). Names only — never the partner URLs,
	// which are operator config the model has no reason to echo.
	Partners []string `json:"partners"`
	// Money is in the venue currency, dollars on the wire (cents in the DB).
	FlatFee      float64 `json:"flat_fee"`
	MinimumOrder float64 `json:"minimum_order"`
	RadiusKm     float64 `json:"radius_km"`
}

// directorDeliveryPartnerNameCap bounds how many partner names ride in the
// prompt. External partner links are already capped at 5 by the settings
// validator; the cap here is belt-and-braces against legacy rows.
const directorDeliveryPartnerNameCap = 10

// directorExternalPartnerLink is the stored shape of one external partner
// link. Only the display name is read — the URL stays out of the snapshot.
type directorExternalPartnerLink struct {
	Name string `json:"name"`
}

// loadDirectorDeliveryConfig reads the venue's delivery configuration.
//
// Access shape: ONE indexed single-row read against delivery_settings
// (business_id is unique) with an explicit column projection — no SELECT *, no
// preloads, no per-partner queries. A missing row is a configured answer
// ("this venue does not deliver"), not an error.
func loadDirectorDeliveryConfig(db *gorm.DB, businessID uint) directorDeliveryConfig {
	if db == nil {
		return directorDeliveryConfig{Partners: []string{}}
	}

	var row struct {
		DeliveryEnabled        bool
		InHouseDeliveryEnabled bool
		ThirdPartyEnabled      bool
		UberEatsEnabled        bool
		DoordashEnabled        bool
		GrubhubEnabled         bool
		DefaultDeliveryFee     int64
		MinimumOrderAmount     int64
		MaxDeliveryRadius      float64
		ExternalPartnerLinks   []byte
	}

	err := db.Model(&database.DeliverySettings{}).
		Select("delivery_enabled", "in_house_delivery_enabled", "third_party_enabled",
			"uber_eats_enabled", "doordash_enabled", "grubhub_enabled",
			"default_delivery_fee", "minimum_order_amount", "max_delivery_radius",
			"external_partner_links").
		Where("business_id = ?", businessID).
		Take(&row).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			// No settings row at all: the venue has never configured delivery.
			// That is a known answer, not an unreadable one.
			return directorDeliveryConfig{Known: true, Partners: []string{}}
		}
		return directorDeliveryConfig{Partners: []string{}}
	}

	cfg := directorDeliveryConfig{
		Known:             true,
		Enabled:           row.DeliveryEnabled,
		InHouseEnabled:    row.InHouseDeliveryEnabled,
		ThirdPartyEnabled: row.ThirdPartyEnabled,
		FlatFee:           centsToDollarsFloat(row.DefaultDeliveryFee),
		MinimumOrder:      centsToDollarsFloat(row.MinimumOrderAmount),
		RadiusKm:          row.MaxDeliveryRadius,
		Partners:          directorDeliveryPartners(row.ExternalPartnerLinks, row.UberEatsEnabled, row.DoordashEnabled, row.GrubhubEnabled),
	}
	return cfg
}

// directorDeliveryPartners unions the operator's external partner links with
// the built-in marketplace toggles, de-duplicated case-insensitively so a
// venue that both toggled Uber Eats and pasted an Uber Eats link is listed
// once.
func directorDeliveryPartners(links []byte, uberEats, doordash, grubhub bool) []string {
	out := make([]string, 0, directorDeliveryPartnerNameCap)
	seen := map[string]struct{}{}
	add := func(name string) {
		name = SanitizePromptField(strings.TrimSpace(name), 60)
		if name == "" || len(out) >= directorDeliveryPartnerNameCap {
			return
		}
		key := strings.ToLower(name)
		if _, dup := seen[key]; dup {
			return
		}
		seen[key] = struct{}{}
		out = append(out, name)
	}

	if len(links) > 0 {
		var parsed []directorExternalPartnerLink
		if err := json.Unmarshal(links, &parsed); err == nil {
			for _, link := range parsed {
				add(link.Name)
			}
		}
	}
	builtin := []string{}
	if uberEats {
		builtin = append(builtin, "Uber Eats")
	}
	if doordash {
		builtin = append(builtin, "DoorDash")
	}
	if grubhub {
		builtin = append(builtin, "Grubhub")
	}
	sort.Strings(builtin)
	for _, name := range builtin {
		add(name)
	}
	return out
}

// directorDeliveryProse renders the delivery evidence as operator prose, in the
// venue currency. English like the rest of the grounding block — the model
// translates into the ask's locale.
func directorDeliveryProse(cfg directorDeliveryConfig, currency string) string {
	if !cfg.Known {
		return "Delivery: the delivery settings could not be read right now, so do not claim this venue does or does not deliver."
	}
	if !cfg.Enabled {
		return "Delivery: turned off in Delivery settings for this venue."
	}

	parts := []string{"Delivery: enabled."}
	switch {
	case cfg.InHouseEnabled && len(cfg.Partners) > 0:
		parts = append(parts, fmt.Sprintf("In-house delivery is on and these marketplace partners are ALREADY configured: %s.", strings.Join(cfg.Partners, ", ")))
	case len(cfg.Partners) > 0:
		parts = append(parts, fmt.Sprintf("These marketplace partners are ALREADY configured: %s.", strings.Join(cfg.Partners, ", ")))
	case cfg.InHouseEnabled:
		parts = append(parts, "In-house delivery is on; no marketplace partner is configured.")
	default:
		parts = append(parts, "No marketplace partner is configured yet.")
	}
	parts = append(parts, fmt.Sprintf("Delivery fee %s, minimum order %s, radius %.1f km.",
		directorMoney(currency, cfg.FlatFee), directorMoney(currency, cfg.MinimumOrder), cfg.RadiusKm))
	if len(cfg.Partners) > 0 {
		parts = append(parts, "Never tell this operator they have no delivery partners or send them to Plugins to add one — delivery is configured in Delivery settings, not Plugins.")
	}
	return strings.Join(parts, " ")
}
