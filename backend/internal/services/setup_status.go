package services

import (
	"encoding/json"
	"errors"
	"strings"

	"github.com/stdevmac/payverge/backend/internal/database"

	"gorm.io/gorm"
)

// SetupStatusInput carries the raw, already-queried setup signals for a business.
// It is the pure input to ComputeSetupStatusFromInput so the boolean logic can be
// unit-tested without a database (the server onboarding handler feeds it the same
// values it renders in the setup-status API).
type SetupStatusInput struct {
	BusinessName      string
	AddressCity       string
	DefaultCurrency   string
	TableCount        int64
	MenuCategories    int
	MenuItems         int
	StaffCount        int64
	ActivePluginCount int64
	// CryptoPluginCount is how many of ActivePluginCount are crypto wallet rails
	// (USDC / cross-chain). Those only let guests pay once a settlement (payout)
	// address exists — both the guest UI and the payment API fail closed without
	// one — so they only complete the payment step together with
	// HasSettlementAddress.
	CryptoPluginCount    int64
	HasSettlementAddress bool
	// QRPreviewed is a persisted business milestone, never inferred from a
	// client event. It is the final first-value setup step.
	QRPreviewed bool
}

// SetupStatus is the derived onboarding progress for a business. It is the single
// source of truth shared by the server onboarding handler (HTTP response) and the
// lifecycle scheduler (setup-summary sentence + first missing step in nudge
// emails).
type SetupStatus struct {
	Input SetupStatusInput

	ProfileDone bool
	TablesDone  bool
	MenuDone    bool
	StaffDone   bool
	PaymentDone bool
	QRPreviewed bool

	HasName     bool
	HasAddress  bool
	HasCurrency bool

	CompletedCount int
	TotalCount     int
	RequiredDone   bool
	AllDone        bool
}

// ComputeSetupStatusFromInput applies the setup-completion rules to already-fetched
// signals. Required-to-go-live steps are profile + tables + menu (RequiredDone);
// staff and payments round out the five-step total.
func ComputeSetupStatusFromInput(in SetupStatusInput) SetupStatus {
	hasName := strings.TrimSpace(in.BusinessName) != ""
	// City is informational only (P3 2026-07-16): registration marks it
	// optional and nothing in guest ordering consumes it, so it must not gate
	// the required profile step. HasAddress remains in the payload for display.
	hasAddress := strings.TrimSpace(in.AddressCity) != ""
	hasCurrency := strings.TrimSpace(in.DefaultCurrency) != ""
	profileDone := hasName && hasCurrency

	tablesDone := in.TableCount > 0
	menuDone := in.MenuCategories > 0 && in.MenuItems > 0
	staffDone := in.StaffCount > 0
	// Payment step means "guests can actually pay digitally": a fiat plugin
	// (Stripe etc.) counts on its own; crypto rails count only once the payout
	// wallet is set.
	fiatPluginCount := in.ActivePluginCount - in.CryptoPluginCount
	paymentDone := fiatPluginCount > 0 || (in.CryptoPluginCount > 0 && in.HasSettlementAddress)
	requiredDone := profileDone && tablesDone && menuDone

	completed := 0
	for _, done := range []bool{profileDone, tablesDone, menuDone, staffDone, paymentDone} {
		if done {
			completed++
		}
	}

	return SetupStatus{
		Input:          in,
		ProfileDone:    profileDone,
		TablesDone:     tablesDone,
		MenuDone:       menuDone,
		StaffDone:      staffDone,
		PaymentDone:    paymentDone,
		QRPreviewed:    in.QRPreviewed,
		HasName:        hasName,
		HasAddress:     hasAddress,
		HasCurrency:    hasCurrency,
		CompletedCount: completed,
		TotalCount:     5,
		RequiredDone:   requiredDone,
		AllDone:        completed == 5,
	}
}

// ComputeSetupStatus runs the same narrow, per-domain count queries the setup
// status API uses (one COUNT per domain plus a single active-menu read), then
// derives the status. Projections stay narrow: no full-Business hydration and the
// menu read only pulls the categories JSON.
func ComputeSetupStatus(businessID uint) (SetupStatus, error) {
	db := database.GetDB()

	var biz database.Business
	if err := db.Model(&database.Business{}).
		Select("name", "city", "default_currency", "settlement_addr", "qr_previewed_at").
		Where("id = ?", businessID).
		First(&biz).Error; err != nil {
		return SetupStatus{}, err
	}

	var tableCount int64
	if err := db.Model(&database.Table{}).
		Where("business_id = ? AND is_active = ?", businessID, true).
		Count(&tableCount).Error; err != nil {
		return SetupStatus{}, err
	}

	menuCategories, menuItems := 0, 0
	var menu database.Menu
	if err := db.Select("categories").
		Where("business_id = ? AND is_active = ?", businessID, true).
		First(&menu).Error; err == nil && menu.Categories != "" {
		var cats []database.MenuCategory
		if json.Unmarshal([]byte(menu.Categories), &cats) == nil {
			menuCategories = len(cats)
			for _, cat := range cats {
				menuItems += len(cat.Items)
			}
		}
	} else if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return SetupStatus{}, err
	}

	var staffCount int64
	if err := db.Model(&database.Staff{}).
		Where("business_id = ? AND is_active = ?", businessID, true).
		Count(&staffCount).Error; err != nil {
		return SetupStatus{}, err
	}

	var activePluginCount int64
	if err := db.Model(&database.BusinessPlugin{}).
		Joins("JOIN plugins ON plugins.id = business_plugins.plugin_id").
		Where("business_plugins.business_id = ? AND business_plugins.is_enabled = ? AND plugins.is_active = ?", businessID, true, true).
		Count(&activePluginCount).Error; err != nil {
		return SetupStatus{}, err
	}

	var cryptoPluginCount int64
	if err := db.Model(&database.BusinessPlugin{}).
		Joins("JOIN plugins ON plugins.id = business_plugins.plugin_id").
		Where("business_plugins.business_id = ? AND business_plugins.is_enabled = ? AND plugins.is_active = ? AND plugins.name IN ?",
			businessID, true, true, []string{PluginNameUSDCPayment, PluginNameCrossChainPayment}).
		Count(&cryptoPluginCount).Error; err != nil {
		return SetupStatus{}, err
	}

	return ComputeSetupStatusFromInput(SetupStatusInput{
		BusinessName:         biz.Name,
		AddressCity:          biz.Address.City,
		DefaultCurrency:      biz.DefaultCurrency,
		TableCount:           tableCount,
		MenuCategories:       menuCategories,
		MenuItems:            menuItems,
		StaffCount:           staffCount,
		ActivePluginCount:    activePluginCount,
		CryptoPluginCount:    cryptoPluginCount,
		HasSettlementAddress: strings.TrimSpace(biz.SettlementAddr) != "",
		QRPreviewed:          biz.QRPreviewedAt != nil,
	}), nil
}

// FirstMissingRequiredStep returns the first required (go-live) step that is not
// yet done, in priority order menu → tables → profile (the menu is the single
// biggest blocker to a first order, so it is surfaced first). Returns "" when all
// required steps are complete.
func (s SetupStatus) FirstMissingRequiredStep() string {
	switch {
	case !s.MenuDone:
		return "menu"
	case !s.TablesDone:
		return "tables"
	case !s.ProfileDone:
		return "profile"
	default:
		return ""
	}
}
