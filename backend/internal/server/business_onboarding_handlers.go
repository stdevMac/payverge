// First-run onboarding: data-driven setup status + idempotent completion
// anchor. The IMP-06 per-step JSON state blob (GET/PUT onboarding-state) was
// removed 2026-07 — it never gained a non-test consumer; the dashboard hub
// derives progress from GetSetupStatus and only writes the completion stamp.
package server

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/services"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// onboardingStateResponse is the shape returned by CompleteOnboarding.
type onboardingStateResponse struct {
	CompletedAt *time.Time `json:"completed_at"`
}

type setupStatusInput struct {
	businessName         string
	addressCity          string
	defaultCurrency      string
	tableCount           int64
	menuCategories       int
	menuItems            int
	staffCount           int64
	activePluginCount    int64
	cryptoPluginCount    int64
	hasSettlementAddress bool
	hasFirstPaidBill     bool
	qrPreviewed          bool
	// publishedSpaceCount is advisory only — layout is never required for go-live.
	publishedSpaceCount int64
}

type businessProfileStep struct {
	Done        bool `json:"done"`
	HasName     bool `json:"has_name"`
	HasAddress  bool `json:"has_address"`
	HasCurrency bool `json:"has_currency"`
}

type countStep struct {
	Done  bool  `json:"done"`
	Count int64 `json:"count"`
}

// paymentStep also reports whether the payout wallet exists, so the dashboard
// can tell the owner WHY the step is incomplete ("add your payout wallet or
// connect Stripe") instead of just showing an unticked chip.
type paymentStep struct {
	Done                 bool  `json:"done"`
	Count                int64 `json:"count"`
	HasSettlementAddress bool  `json:"has_settlement_address"`
}

type menuStep struct {
	Done       bool `json:"done"`
	Categories int  `json:"categories"`
	Items      int  `json:"items"`
}

type setupSteps struct {
	BusinessProfile businessProfileStep `json:"business_profile"`
	Tables          countStep           `json:"tables"`
	Menu            menuStep            `json:"menu"`
	Staff           countStep           `json:"staff"`
	Payment         paymentStep         `json:"payment"`
	// Layout is optional (spaces floor plan). It never gates required_done /
	// all_done / completed_count — profile + tables + menu remain go-live.
	Layout countStep `json:"layout"`
}

type setupStatusResponse struct {
	Steps            setupSteps `json:"steps"`
	CompletedCount   int        `json:"completed_count"`
	TotalCount       int        `json:"total_count"`
	RequiredDone     bool       `json:"required_done"`
	AllDone          bool       `json:"all_done"`
	HasFirstPaidBill bool       `json:"has_first_paid_bill"`
	QRPreviewed      bool       `json:"qr_previewed"`
}

// computeSetupStatus maps the shared services-level SetupStatus (the single
// source of truth also used by the lifecycle scheduler) to this handler's HTTP
// response shape, so the completion rules live in exactly one place.
func computeSetupStatus(in setupStatusInput) setupStatusResponse {
	status := services.ComputeSetupStatusFromInput(services.SetupStatusInput{
		BusinessName:         in.businessName,
		AddressCity:          in.addressCity,
		DefaultCurrency:      in.defaultCurrency,
		TableCount:           in.tableCount,
		MenuCategories:       in.menuCategories,
		MenuItems:            in.menuItems,
		StaffCount:           in.staffCount,
		ActivePluginCount:    in.activePluginCount,
		CryptoPluginCount:    in.cryptoPluginCount,
		HasSettlementAddress: in.hasSettlementAddress,
		QRPreviewed:          in.qrPreviewed,
	})

	return setupStatusResponse{
		Steps: setupSteps{
			BusinessProfile: businessProfileStep{
				Done:        status.ProfileDone,
				HasName:     status.HasName,
				HasAddress:  status.HasAddress,
				HasCurrency: status.HasCurrency,
			},
			Tables:  countStep{Done: status.TablesDone, Count: in.tableCount},
			Menu:    menuStep{Done: status.MenuDone, Categories: in.menuCategories, Items: in.menuItems},
			Staff:   countStep{Done: status.StaffDone, Count: in.staffCount},
			Payment: paymentStep{Done: status.PaymentDone, Count: in.activePluginCount, HasSettlementAddress: in.hasSettlementAddress},
			Layout:  countStep{Done: in.publishedSpaceCount > 0, Count: in.publishedSpaceCount},
		},
		CompletedCount:   status.CompletedCount,
		TotalCount:       status.TotalCount,
		RequiredDone:     status.RequiredDone,
		AllDone:          status.AllDone,
		HasFirstPaidBill: in.hasFirstPaidBill,
		QRPreviewed:      status.QRPreviewed,
	}
}

func GetSetupStatus(c *gin.Context) {
	business, ok := requireBusinessAccess(c, "id")
	if !ok {
		return
	}

	db := database.GetDB()

	var tableCount int64
	if err := db.Model(&database.Table{}).Where("business_id = ? AND is_active = ?", business.ID, true).Count(&tableCount).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to query tables"})
		return
	}

	var menu database.Menu
	menuCategories := 0
	menuItems := 0
	if err := db.Where("business_id = ? AND is_active = ?", business.ID, true).First(&menu).Error; err == nil && menu.Categories != "" {
		var cats []database.MenuCategory
		if json.Unmarshal([]byte(menu.Categories), &cats) == nil {
			menuCategories = len(cats)
			for _, cat := range cats {
				menuItems += len(cat.Items)
			}
		}
	} else if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to query menu"})
		return
	}

	var staffCount int64
	if err := db.Model(&database.Staff{}).Where("business_id = ? AND is_active = ?", business.ID, true).Count(&staffCount).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to query staff"})
		return
	}

	var activePluginCount int64
	if err := db.Model(&database.BusinessPlugin{}).
		Joins("JOIN plugins ON plugins.id = business_plugins.plugin_id").
		Where("business_plugins.business_id = ? AND business_plugins.is_enabled = ? AND plugins.is_active = ?", business.ID, true, true).
		Count(&activePluginCount).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to query plugins"})
		return
	}

	var cryptoPluginCount int64
	if err := db.Model(&database.BusinessPlugin{}).
		Joins("JOIN plugins ON plugins.id = business_plugins.plugin_id").
		Where("business_plugins.business_id = ? AND business_plugins.is_enabled = ? AND plugins.is_active = ? AND plugins.name IN ?",
			business.ID, true, true, []string{services.PluginNameUSDCPayment, services.PluginNameCrossChainPayment}).
		Count(&cryptoPluginCount).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to query plugins"})
		return
	}

	hasFirstPaidBill, err := database.BusinessHasPaidBill(business.ID)
	if err != nil {
		// Fail-quiet: this flag drives an advisory "no orders yet" banner. On a
		// lookup error report true (banner hidden) rather than nagging a
		// business that already has orders.
		log.Printf("setup-status: paid-bill lookup failed for business %d: %v", business.ID, err)
		hasFirstPaidBill = true
	}

	// Optional layout signal (published spaces). Fail-quiet to 0 so go-live
	// is never blocked by spaces table issues. Soft-deleted rows are excluded
	// by GORM's DeletedAt scope on RestaurantSpace.
	var publishedSpaceCount int64
	if err := db.Model(&database.RestaurantSpace{}).
		Where("business_id = ? AND status = ?", business.ID, "published").
		Count(&publishedSpaceCount).Error; err != nil {
		log.Printf("setup-status: published-space lookup failed for business %d: %v", business.ID, err)
		publishedSpaceCount = 0
	}

	result := computeSetupStatus(setupStatusInput{
		businessName:         business.Name,
		addressCity:          business.Address.City,
		defaultCurrency:      business.DefaultCurrency,
		tableCount:           tableCount,
		menuCategories:       menuCategories,
		menuItems:            menuItems,
		staffCount:           staffCount,
		activePluginCount:    activePluginCount,
		cryptoPluginCount:    cryptoPluginCount,
		hasSettlementAddress: strings.TrimSpace(business.SettlementAddr) != "",
		hasFirstPaidBill:     hasFirstPaidBill,
		qrPreviewed:          business.QRPreviewedAt != nil,
		publishedSpaceCount:  publishedSpaceCount,
	})

	c.JSON(http.StatusOK, result)
}

type markQRPreviewedRequest struct {
	TableID uint `json:"table_id" binding:"required"`
}

// MarkQRPreviewed records the server-authoritative first QR preview. The write
// is monotonic and idempotent: later previews retain the original timestamp.
func MarkQRPreviewed(c *gin.Context) {
	business, ok := requireBusinessAccess(c, "id")
	if !ok {
		return
	}

	var req markQRPreviewedRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		RespondBindError(c, err)
		return
	}

	var table database.Table
	if err := database.GetDB().Select("id", "business_id").
		Where("id = ? AND business_id = ? AND is_active = ?", req.TableID, business.ID, true).
		First(&table).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			RespondWithError(c, http.StatusNotFound, ErrCodeBusinessNotFound, "Table not found")
			return
		}
		RespondWithError(c, http.StatusInternalServerError, ErrCodeInternal, "Failed to record QR preview")
		return
	}

	now := time.Now().UTC()
	if err := database.GetDB().Model(&database.Business{}).
		Where("id = ? AND qr_previewed_at IS NULL", business.ID).
		Update("qr_previewed_at", now).Error; err != nil {
		RespondWithError(c, http.StatusInternalServerError, ErrCodeInternal, "Failed to record QR preview")
		return
	}

	var refreshed database.Business
	if err := database.GetDB().Select("qr_previewed_at").First(&refreshed, business.ID).Error; err != nil {
		RespondWithError(c, http.StatusInternalServerError, ErrCodeInternal, "Failed to load QR preview milestone")
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"qr_previewed":    true,
		"qr_previewed_at": refreshed.QRPreviewedAt,
	})
}

// CompleteOnboarding marks the wizard as finished. Idempotent: calling it
// again after the first time is a no-op (we don't reset the timestamp). This
// shares a code path with the "Skip for now" link in the wizard footer.
func CompleteOnboarding(c *gin.Context) {
	business, ok := requireBusinessAccess(c, "id")
	if !ok {
		return
	}

	if business.OnboardingCompletedAt == nil {
		now := time.Now().UTC()
		db := database.GetDB()
		res := db.Model(&database.Business{}).
			Where("id = ? AND onboarding_completed_at IS NULL", business.ID).
			Update("onboarding_completed_at", now)
		if res.Error != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to complete onboarding"})
			return
		}
		if res.RowsAffected == 1 {
			business.OnboardingCompletedAt = &now
		} else {
			// Lost the race — return the persisted stamp, not a nil.
			var refreshed database.Business
			if err := db.Select("onboarding_completed_at").First(&refreshed, business.ID).Error; err == nil {
				business.OnboardingCompletedAt = refreshed.OnboardingCompletedAt
			}
		}
	}

	c.JSON(http.StatusOK, onboardingStateResponse{
		CompletedAt: business.OnboardingCompletedAt,
	})
}
