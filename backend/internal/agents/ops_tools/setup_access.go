package ops_tools

import (
	"encoding/json"
	"errors"
	"strings"

	"github.com/stdevmac/payverge/backend/internal/agents"
	"github.com/stdevmac/payverge/backend/internal/database"

	"gorm.io/gorm"
)

type tabLock struct {
	Locked bool
	Hidden bool
}

func tabLockMeta(tab string, env agents.ToolEnv) tabLock {
	tab = strings.TrimSpace(strings.ToLower(tab))
	if env.IsSuspended && tab != "settings" {
		return tabLock{Locked: true, Hidden: env.IsStaffUser}
	}
	return tabLock{}
}

func allTabKeys() []string {
	return []string{
		"overview", "menu", "tables", "bills", "kitchen", "counter", "reservations",
		"delivery", "analytics", "crm", "plugins", "staff", "settings",
		"business-page", "ai-waiter", "director-console", "inventory", "accounting",
	}
}

func computeSetupStatusForBusiness(db *gorm.DB, businessID uint) (map[string]any, error) {
	var biz database.Business
	if err := db.First(&biz, businessID).Error; err != nil {
		return nil, err
	}
	var tableCount int64
	_ = db.Model(&database.Table{}).Where("business_id = ? AND is_active = ?", businessID, true).Count(&tableCount)

	menuCategories := 0
	menuItems := 0
	var menu database.Menu
	if err := db.Where("business_id = ? AND is_active = ?", businessID, true).First(&menu).Error; err == nil && menu.Categories != "" {
		var cats []database.MenuCategory
		if json.Unmarshal([]byte(menu.Categories), &cats) == nil {
			menuCategories = len(cats)
			for _, cat := range cats {
				menuItems += len(cat.Items)
			}
		}
	} else if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}

	var staffCount int64
	_ = db.Model(&database.Staff{}).Where("business_id = ? AND is_active = ?", businessID, true).Count(&staffCount)
	var pluginCount int64
	_ = db.Model(&database.BusinessPlugin{}).
		Joins("JOIN plugins ON plugins.id = business_plugins.plugin_id").
		Where("business_plugins.business_id = ? AND business_plugins.is_enabled = ? AND plugins.is_active = ?", businessID, true, true).
		Count(&pluginCount)

	hasName := strings.TrimSpace(biz.Name) != ""
	hasAddress := strings.TrimSpace(biz.Address.City) != ""
	hasCurrency := strings.TrimSpace(biz.DefaultCurrency) != ""
	profileDone := hasName && hasAddress && hasCurrency
	tablesDone := tableCount > 0
	menuDone := menuCategories > 0 && menuItems > 0
	staffDone := staffCount > 0
	paymentDone := pluginCount > 0
	completed := 0
	for _, d := range []bool{profileDone, tablesDone, menuDone, staffDone, paymentDone} {
		if d {
			completed++
		}
	}
	return map[string]any{
		"completed_count": completed,
		"total_count":     5,
		"required_done":   profileDone && tablesDone && menuDone,
		"all_done":        completed == 5,
		"steps": map[string]any{
			"business_profile": map[string]any{"done": profileDone},
			"tables":           map[string]any{"done": tablesDone, "count": tableCount},
			"menu":             map[string]any{"done": menuDone, "categories": menuCategories, "items": menuItems},
			"staff":            map[string]any{"done": staffDone, "count": staffCount},
			"payment":          map[string]any{"done": paymentDone, "count": pluginCount},
		},
	}, nil
}
