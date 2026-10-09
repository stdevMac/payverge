package ops_guides

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
)

// TabRecord is one canonical dashboard area shared by Ops and the operator UI.
type TabRecord struct {
	Key                string `json:"key"`
	Group              string `json:"group"`
	RequiredPermission string `json:"required_permission"`
	RouteKind          string `json:"route_kind"`
	StaffLockBehavior  string `json:"staff_lock_behavior"`
}

// CanonicalTabKeys is the ordered 23-area set Ops and the sidebar must share.
var CanonicalTabKeys = []string{
	"overview", "bills", "cash-register", "printers", "kitchen", "reservations",
	"menu", "tables", "ai-waiter", "director-console", "marketing", "analytics",
	"crm", "delivery", "counter", "inventory", "staff", "schedule",
	"business-page", "accounting", "fiscal", "plugins", "settings",
}

var (
	tabOnce sync.Once
	tabMap  map[string]TabRecord
	tabErr  error
)

func loadTabs() {
	tabOnce.Do(func() {
		tabMap = make(map[string]TabRecord, len(CanonicalTabKeys))
		// Prefer embedded fallbacks so unit tests and production both work
		// without depending on CWD; try repo config path first.
		data, err := readDashboardTabsJSON()
		if err != nil {
			tabErr = err
			return
		}
		var rows []TabRecord
		if err := json.Unmarshal(data, &rows); err != nil {
			tabErr = err
			return
		}
		for _, r := range rows {
			tabMap[r.Key] = r
		}
	})
}

func readDashboardTabsJSON() ([]byte, error) {
	// Walk from this file up to repo root config/dashboard-tabs.json.
	_, thisFile, _, ok := runtime.Caller(0)
	if ok {
		dir := filepath.Dir(thisFile)
		for i := 0; i < 8; i++ {
			candidate := filepath.Join(dir, "config", "dashboard-tabs.json")
			if b, err := os.ReadFile(candidate); err == nil {
				return b, nil
			}
			// Also try from agents/ops_guides → payverge root (4 levels up).
			parent := filepath.Dir(dir)
			if parent == dir {
				break
			}
			dir = parent
		}
	}
	// Fallback: relative from cwd (backend or repo root).
	for _, p := range []string{
		"config/dashboard-tabs.json",
		"../config/dashboard-tabs.json",
		"../../config/dashboard-tabs.json",
	} {
		if b, err := os.ReadFile(p); err == nil {
			return b, nil
		}
	}
	return nil, fmt.Errorf("dashboard-tabs.json not found")
}

// LookupTab returns the registry record for a key.
func LookupTab(key string) (TabRecord, bool) {
	loadTabs()
	r, ok := tabMap[key]
	return r, ok
}

// AllTabs returns every registry record in canonical order.
func AllTabs() []TabRecord {
	loadTabs()
	out := make([]TabRecord, 0, len(CanonicalTabKeys))
	for _, k := range CanonicalTabKeys {
		if r, ok := tabMap[k]; ok {
			out = append(out, r)
		}
	}
	return out
}

// RegistryLoadError exposes load failures for tests.
func RegistryLoadError() error {
	loadTabs()
	return tabErr
}

// MaxActiveTabLen bounds the client-supplied active_tab before it is matched.
const MaxActiveTabLen = 64

// NormalizeActiveTab returns the canonical dashboard tab key for a
// client-supplied active_tab, or "" when the value is not a known tab.
// active_tab arrives in the request body and is interpolated into assistant
// system prompts, so only registry keys (config/dashboard-tabs.json, kept in
// parity with CanonicalTabKeys) may pass (SEC-AI-01).
func NormalizeActiveTab(raw string) string {
	if len(raw) > 4*MaxActiveTabLen {
		return ""
	}
	key := strings.ToLower(strings.TrimSpace(raw))
	if key == "" || len(key) > MaxActiveTabLen {
		return ""
	}
	if _, ok := LookupTab(key); ok {
		return key
	}
	for _, k := range CanonicalTabKeys {
		if k == key {
			return key
		}
	}
	return ""
}
