package services

import (
	"strings"

	"github.com/stdevmac/payverge/backend/internal/database"
)

type OrderabilityState string

const (
	OrderabilityAvailable      OrderabilityState = "available"
	OrderabilityManualDisabled OrderabilityState = "manual_disabled"
	OrderabilityInventoryOut   OrderabilityState = "inventory_out"
	OrderabilityInventoryWarn  OrderabilityState = "inventory_warning"
	OrderabilityBusinessClosed OrderabilityState = "business_closed"
	OrderabilityOrderingOff    OrderabilityState = "ordering_disabled"
)

type OrderabilityContext uint8

const (
	OrderabilityContextGuest OrderabilityContext = iota
	OrderabilityContextOperator
)

type Orderability struct {
	Orderable bool              `json:"orderable"`
	State     OrderabilityState `json:"state"`
}

type OrderabilityFacts struct {
	Context              OrderabilityContext
	ManualAvailable      bool
	InventoryEnabled     bool
	InventoryMode        string
	InventoryOut         bool
	InventoryWarning     bool
	BusinessOpen         bool
	GuestOrderingEnabled bool
}

func ResolveOrderability(facts OrderabilityFacts) Orderability {
	if !facts.ManualAvailable {
		return Orderability{State: OrderabilityManualDisabled}
	}
	// Guest hours / kitchen gates must win over inventory "still sellable"
	// honesty states (warn-mode low stock). Otherwise a closed dining
	// room can still project orderable:true lines (e.g. inventory_warning)
	// and the guest UI never shows a closed banner.
	if facts.Context == OrderabilityContextGuest {
		if !facts.BusinessOpen {
			return Orderability{State: OrderabilityBusinessClosed}
		}
		if !facts.GuestOrderingEnabled {
			return Orderability{State: OrderabilityOrderingOff}
		}
	}
	if facts.InventoryEnabled {
		switch facts.InventoryMode {
		case database.InventoryAvailabilityModeHardBlock, database.InventoryAvailabilityModeWarn:
			// Dinner-service bar: zero stock must 86 the dish on live menu,
			// guest orderability, and open-check adds — warn and hard_block
			// both refuse sale when recipe ingredients cannot make a serving.
			// Warn still surfaces low-stock honesty without blocking.
			if facts.InventoryOut {
				return Orderability{State: OrderabilityInventoryOut}
			}
			if facts.InventoryMode == database.InventoryAvailabilityModeWarn && facts.InventoryWarning {
				return Orderability{Orderable: true, State: OrderabilityInventoryWarn}
			}
		}
	}
	return Orderability{Orderable: true, State: OrderabilityAvailable}
}

// ProjectOrderability evaluates every menu item from one inventory summary.
// The summary performs a constant set of batched reads and avoids per-item DB
// calls. Inventory failures fail open while manual and guest business gates
// remain authoritative.
func ProjectOrderability(
	business *database.Business,
	categories []database.MenuCategory,
	context OrderabilityContext,
	businessOpen bool,
) map[string]Orderability {
	projection := make(map[string]Orderability)
	if business == nil {
		return projection
	}

	statusByID := make(map[string]database.InventoryMenuItemStatus)
	settings := database.InventorySettings{}
	if business.ID != 0 {
		// Settings first: with inventory disabled the summary's statuses are
		// never consulted (ResolveOrderability ignores them), so skip the
		// inventory_items / inventory_recipes / menu reads entirely. That is
		// the common case on every guest menu and table poll.
		if loaded, err := database.GetInventorySettings(business.ID); err == nil && loaded != nil && loaded.InventoryEnabled {
			if summary, err := database.GetInventorySummaryWithSettings(business.ID, loaded); err == nil && summary != nil {
				settings = summary.Settings
				for _, status := range summary.MenuItemStatuses {
					statusByID[status.MenuItemID] = status
				}
			}
		}
	}

	guestOrderingEnabled := business.KitchenEnabled && business.OrdersEnabled && database.IsBusinessOperational(business)
	for ci := range categories {
		for ii := range categories[ci].Items {
			item := &categories[ci].Items[ii]
			status := statusByID[item.ID]
			// Survive guest hours: ResolveOrderability returns business_closed
			// even when InventoryOut is true. Stamp the catalog so callers that
			// only see the projection cannot resurrect an 86 as a dish card.
			// Manual sync never 86s from stock — don't stamp those rows.
			if status.Status == "out_of_stock" && settings.InventoryEnabled &&
				(settings.AvailabilitySyncMode == database.InventoryAvailabilityModeHardBlock ||
					settings.AvailabilitySyncMode == database.InventoryAvailabilityModeWarn) {
				item.InventoryStatus = status.Status
			} else {
				// Serve-time stamp only. Clear a leftover out_of_stock so
				// Menu Builder cannot 86 a dish the live summary no longer
				// blocks (Harvest Bowl after beef was remapped to steak).
				item.InventoryStatus = ""
			}
			projection[item.ID] = ResolveOrderability(OrderabilityFacts{
				Context:              context,
				ManualAvailable:      item.IsAvailable,
				InventoryEnabled:     settings.InventoryEnabled,
				InventoryMode:        settings.AvailabilitySyncMode,
				InventoryOut:         status.Status == "out_of_stock",
				InventoryWarning:     status.Status == "low_stock",
				BusinessOpen:         businessOpen,
				GuestOrderingEnabled: guestOrderingEnabled,
			})
		}
	}
	return projection
}

// ApplyInventorySellability turns the catalog's stored is_available flag into
// the effective one: a dish inventory has 86'd reads is_available:false. That
// is already the answer guests get, so applying it to the operator menu too is
// what stops the two audiences disagreeing about whether a dish is sellable —
// the disagreement WAS #727 (Menu Builder painted the blocked dish available
// next to an "Unavailable · inventory" pill, while guests could not order it).
//
// stampManual preserves the stored flag in manual_available. Operator reads
// need it because Menu Builder and Kitchen still EDIT that flag; guests never
// do, so their payload is left alone.
func ApplyInventorySellability(
	categories []database.MenuCategory,
	projection map[string]Orderability,
	stampManual bool,
) {
	for ci := range categories {
		for ii := range categories[ci].Items {
			item := &categories[ci].Items[ii]
			if stampManual {
				manual := item.IsAvailable
				item.ManualAvailable = &manual
			}
			// inventory_status survives a guest-hours remap to business_closed,
			// so check the stamp before the projection.
			if item.InventoryStatus == "out_of_stock" {
				item.IsAvailable = false
				continue
			}
			id := strings.TrimSpace(item.ID)
			if id == "" {
				continue
			}
			if decision, ok := projection[id]; ok && decision.State == OrderabilityInventoryOut {
				item.IsAvailable = false
			}
		}
	}
}
