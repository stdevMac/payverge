package server_test

import (
	"encoding/json"
	"os"
	"regexp"
	"strings"
	"testing"
)

// backendInsightTypes is the authoritative list of insight types the backend's
// buildProactiveInsights (director_console_handler.go) can emit. Add to this
// list when you add a new insight. This test then asserts the frontend knows
// about each type in three places:
//   - ProactiveInsights.tsx KNOWN_INSIGHT_TYPES set (else the insight is filtered out)
//   - en/businessDashboard.json proactive.types.<kind>.title_one
//   - es/businessDashboard.json proactive.types.<kind>.title_one
//
// This is the interim parity guard for plan task 5.2 — the full shared
// registry is deferred; until then, keep this list in lockstep with the
// backend emitter.
var backendInsightTypes = []string{
	"inventory_out_of_stock",
	"inventory_low_stock",
	"stale_open_bills",
	"ai_conversations_pending",
	"food_cost_high",
	"waste_high",
	"labor_high",
}

// repoRoot is the path from this test file (backend/internal/server) up to
// the repo root. Each segment steps out of one directory.
const repoRoot = "../../.."

func TestProactiveInsightParity_FrontendKnowsEveryBackendType(t *testing.T) {
	tsxPath := repoRoot + "/frontend/src/components/business/overview/ProactiveInsights.tsx"
	tsxSrc, err := os.ReadFile(tsxPath)
	if err != nil {
		t.Fatalf("read ProactiveInsights.tsx: %v", err)
	}
	knownSet := extractKnownInsightTypes(t, string(tsxSrc))

	enPath := repoRoot + "/frontend/src/i18n/messages/en/businessDashboard.json"
	esPath := repoRoot + "/frontend/src/i18n/messages/es/businessDashboard.json"

	enRaw, err := os.ReadFile(enPath)
	if err != nil {
		t.Fatalf("read %s: %v", enPath, err)
	}
	esRaw, err := os.ReadFile(esPath)
	if err != nil {
		t.Fatalf("read %s: %v", esPath, err)
	}
	enTypes := extractProactiveTypes(t, enRaw, "en")
	esTypes := extractProactiveTypes(t, esRaw, "es")

	for _, typeName := range backendInsightTypes {
		if !knownSet[typeName] {
			t.Errorf(
				"backend emits insight type %q but it is missing from KNOWN_INSIGHT_TYPES in %s — the UI filter will drop it. Add %q to the Set.",
				typeName, tsxPath, typeName,
			)
		}
		assertTitleOne(t, enTypes, typeName, enPath)
		assertTitleOne(t, esTypes, typeName, esPath)
	}
}

// assertTitleOne fails the test if the given insight type is missing from the
// i18n bundle, or has no title_one key under proactive.types.<typeName>.
func assertTitleOne(t *testing.T, types map[string]any, typeName, path string) {
	t.Helper()
	entryAny, ok := types[typeName]
	if !ok {
		t.Errorf(
			"backend emits insight type %q but %s has no overview.proactive.types.%s entry — add translations there.",
			typeName, path, typeName,
		)
		return
	}
	entry, ok := entryAny.(map[string]any)
	if !ok {
		t.Errorf(
			"overview.proactive.types.%s in %s is not a JSON object (got %T) — expected an object with at least title_one.",
			typeName, path, entryAny,
		)
		return
	}
	if _, ok := entry["title_one"]; !ok {
		t.Errorf(
			"overview.proactive.types.%s in %s is missing required key title_one — add a singular title string.",
			typeName, path,
		)
	}
}

// extractKnownInsightTypes parses the KNOWN_INSIGHT_TYPES = new Set([...]) literal
// from ProactiveInsights.tsx. The regex tolerates an optional TypeScript type
// argument (e.g. new Set<string>([...])), whitespace, and trailing commas.
func extractKnownInsightTypes(t *testing.T, src string) map[string]bool {
	t.Helper()
	re := regexp.MustCompile(`KNOWN_INSIGHT_TYPES\s*=\s*new\s+Set(?:<[^>]+>)?\s*\(\s*\[([\s\S]*?)\]`)
	m := re.FindStringSubmatch(src)
	if len(m) < 2 {
		t.Fatal("could not locate KNOWN_INSIGHT_TYPES = new Set([...]) literal in ProactiveInsights.tsx")
	}
	body := m[1]
	out := map[string]bool{}
	// Match quoted strings (single or double quotes). Avoids mis-parsing
	// comments or array metadata tokens like `as const`.
	stringRe := regexp.MustCompile(`["']([^"']+)["']`)
	for _, match := range stringRe.FindAllStringSubmatch(body, -1) {
		s := strings.TrimSpace(match[1])
		if s != "" {
			out[s] = true
		}
	}
	if len(out) == 0 {
		t.Fatal("extracted KNOWN_INSIGHT_TYPES body but found no string entries — regex likely needs tuning")
	}
	return out
}

// extractProactiveTypes walks the JSON and returns the map at
// dashboard.proactive.types. Fails the test with a clear error if the path
// is missing.
func extractProactiveTypes(t *testing.T, raw []byte, label string) map[string]any {
	t.Helper()
	var parsed map[string]any
	if err := json.Unmarshal(raw, &parsed); err != nil {
		t.Fatalf("parse %s businessDashboard.json: %v", label, err)
	}
	overview, ok := parsed["overview"].(map[string]any)
	if !ok {
		t.Fatalf("%s businessDashboard.json: missing top-level 'overview' object", label)
	}
	proactive, ok := overview["proactive"].(map[string]any)
	if !ok {
		t.Fatalf("%s businessDashboard.json: missing 'overview.proactive' object", label)
	}
	types, ok := proactive["types"].(map[string]any)
	if !ok {
		t.Fatalf("%s businessDashboard.json: missing 'overview.proactive.types' object", label)
	}
	return types
}
