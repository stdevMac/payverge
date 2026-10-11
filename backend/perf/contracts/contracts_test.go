package contracts

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"
)

type capacityContract struct {
	SchemaVersion   int `json:"schema_version"`
	ProofMultiplier int `json:"proof_multiplier"`
	Flows           map[string]struct {
		P50MS     float64 `json:"p50_ms"`
		P95MS     float64 `json:"p95_ms"`
		P99MS     float64 `json:"p99_ms"`
		ErrorRate float64 `json:"error_rate"`
	} `json:"flows"`
	Limits          map[string]float64 `json:"limits"`
	RuntimeContract map[string]float64 `json:"runtime_contract"`
	ProjectedPeak   struct {
		RequestsPerSecond int `json:"requests_per_second"`
		ConcurrentSSE     int `json:"concurrent_sse"`
	} `json:"projected_peak"`
}

type loadProfile struct {
	Duration       string         `json:"duration"`
	ExternalOnly   bool           `json:"external_only"`
	PeakMultiplier float64        `json:"peak_multiplier"`
	Flows          map[string]int `json:"flows"`
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(b)
}

func decodeJSON[T any](t *testing.T, path string) T {
	t.Helper()
	var value T
	if err := json.Unmarshal([]byte(readFile(t, path)), &value); err != nil {
		t.Fatalf("decode %s: %v", path, err)
	}
	return value
}

func TestCapacityContractCoversEveryD5Surface(t *testing.T) {
	contract := decodeJSON[capacityContract](t, filepath.Join("..", "capacity-contract.json"))
	if contract.SchemaVersion != 1 {
		t.Fatalf("schema_version = %d, want 1", contract.SchemaVersion)
	}
	if contract.ProofMultiplier < 5 {
		t.Fatalf("proof_multiplier = %d, want at least 5", contract.ProofMultiplier)
	}

	for _, flow := range []string{
		"anonymous_menu", "operator_action", "guest_quote", "guest_order",
		"callback_webhook", "sse",
	} {
		budget, ok := contract.Flows[flow]
		if !ok {
			t.Errorf("missing flow budget %q", flow)
			continue
		}
		if budget.P50MS <= 0 || budget.P95MS <= budget.P50MS || budget.P99MS <= budget.P95MS {
			t.Errorf("flow %q must define increasing positive p50/p95/p99 budgets: %+v", flow, budget)
		}
		if budget.ErrorRate <= 0 || budget.ErrorRate > 0.01 {
			t.Errorf("flow %q error_rate must be conservative and non-zero (<=1%%): %v", flow, budget.ErrorRate)
		}
	}

	for _, limit := range []string{
		"cpu_percent", "memory_bytes", "disk_percent", "goroutines",
		"postgres_open_connections", "postgres_waiting_locks",
		"postgres_lock_wait_ms", "background_job_lag_seconds", "queue_depth",
		"sse_lag_ms",
	} {
		if contract.Limits[limit] <= 0 && limit != "postgres_waiting_locks" {
			t.Errorf("missing positive limit %q", limit)
		}
	}
	if got := contract.Limits["postgres_waiting_locks"]; got != 0 {
		t.Errorf("postgres_waiting_locks = %v, want fail-closed limit 0", got)
	}
}

func TestProfilesIncludeBurstOneHourSoakAndExternalFiveXCandidate(t *testing.T) {
	smoke := decodeJSON[loadProfile](t, filepath.Join("..", "k6", "profiles", "smoke.json"))
	burst := decodeJSON[loadProfile](t, filepath.Join("..", "k6", "profiles", "burst.json"))
	soak := decodeJSON[loadProfile](t, filepath.Join("..", "k6", "profiles", "soak_1h.json"))
	candidate := decodeJSON[loadProfile](t, filepath.Join("..", "k6", "profiles", "candidate_5x.json"))

	smokeDuration, err := time.ParseDuration(smoke.Duration)
	if err != nil || smokeDuration <= 0 || smokeDuration > time.Minute || smoke.ExternalOnly || len(smoke.Flows) < 6 {
		t.Fatalf("smoke must be a local, all-surface profile <=1m: %+v", smoke)
	}
	burstDuration, err := time.ParseDuration(burst.Duration)
	if err != nil || burstDuration <= 0 || burstDuration > 10*time.Minute {
		t.Fatalf("burst duration %q must be >0 and <=10m", burst.Duration)
	}
	if len(burst.Flows) < 6 {
		t.Errorf("burst must exercise all request surfaces, got %d", len(burst.Flows))
	}
	if soak.Duration != "1h" {
		t.Errorf("soak duration = %q, want 1h", soak.Duration)
	}
	if candidate.Duration != "1h" || candidate.PeakMultiplier < 5 || !candidate.ExternalOnly {
		t.Errorf("candidate profile must be a guarded external-only 1h 5x proof: %+v", candidate)
	}
	contract := decodeJSON[capacityContract](t, filepath.Join("..", "capacity-contract.json"))
	for name, profile := range map[string]loadProfile{"burst": burst, "soak_1h": soak, "candidate_5x": candidate} {
		requestRate := 0
		for flow, value := range profile.Flows {
			if flow != "sse" {
				requestRate += value
			}
		}
		wantRequests := int(float64(contract.ProjectedPeak.RequestsPerSecond) * profile.PeakMultiplier)
		wantSSE := int(float64(contract.ProjectedPeak.ConcurrentSSE) * profile.PeakMultiplier)
		if requestRate != wantRequests || profile.Flows["sse"] != wantSSE {
			t.Errorf("%s does not match declared peak multiplier: requests=%d/%d sse=%d/%d", name, requestRate, wantRequests, profile.Flows["sse"], wantSSE)
		}
	}
}

func TestK6HarnessHasTaggedThresholdsTelemetryAndFailClosedAuth(t *testing.T) {
	thresholds := readFile(t, filepath.Join("..", "k6", "lib", "thresholds.js"))
	contract := decodeJSON[capacityContract](t, filepath.Join("..", "capacity-contract.json"))
	for _, flow := range []string{
		"anonymous_menu", "operator_action", "guest_quote", "guest_order",
		"callback_webhook", "sse",
	} {
		if !strings.Contains(thresholds, flow+":") {
			t.Errorf("thresholds missing tagged flow %q", flow)
		}
		budget := contract.Flows[flow]
		want := regexp.MustCompile(regexp.QuoteMeta(flow) + `\s*:\s*\{\s*p50:\s*` +
			regexp.QuoteMeta(trimFloat(budget.P50MS)) + `,\s*p95:\s*` + regexp.QuoteMeta(trimFloat(budget.P95MS)) +
			`,\s*p99:\s*` + regexp.QuoteMeta(trimFloat(budget.P99MS)) + `,\s*errors:\s*` +
			regexp.QuoteMeta(trimFloat(budget.ErrorRate)))
		if !want.MatchString(thresholds) {
			t.Errorf("k6 threshold budget for %q drifted from capacity-contract.json", flow)
		}
	}
	if !strings.Contains(thresholds, "flow:${flow}") {
		t.Error("threshold keys are not tagged by flow")
	}
	for _, percentile := range []string{"p(50)", "p(95)", "p(99)"} {
		if !strings.Contains(thresholds, percentile) {
			t.Errorf("thresholds missing %s", percentile)
		}
	}

	capacity := readFile(t, filepath.Join("..", "k6", "scenarios", "capacity.js"))
	for _, token := range []string{
		"setupOperator", "guestQuote", "guestOrder", "callbackWebhook", "sse",
		"summaryTrendStats", "sse_event_lag_ms", "capacity-summary.json", "k6/x/sse", "sseClient.open",
	} {
		if !strings.Contains(capacity, token) {
			t.Errorf("capacity scenario missing %q", token)
		}
	}
	auth := readFile(t, filepath.Join("..", "k6", "lib", "auth.js"))
	if strings.Contains(auth, "TODO(perf)") || !strings.Contains(auth, "/api/v1/staff/verify-login-code") {
		t.Error("authenticated fixture must use the real staff login-code route without TODO fallback")
	}
	if !strings.Contains(auth, "throw new Error") {
		t.Error("auth setup must fail closed when fixtures/token acquisition are absent")
	}
	builder := readFile(t, filepath.Join("..", "k6", "scripts", "build-k6.sh"))
	for _, pin := range []string{"xk6_version=\"v1.4.8\"", "k6_version=\"v1.8.0\"", "sse_version=\"v0.1.12\""} {
		if !strings.Contains(builder, pin) {
			t.Errorf("pinned k6 builder missing %s", pin)
		}
	}
}

func trimFloat(value float64) string {
	return strings.TrimRight(strings.TrimRight(strconv.FormatFloat(value, 'f', 6, 64), "0"), ".")
}

func TestSeedProvidesRepeatableAuthAndActiveOrderFixtures(t *testing.T) {
	seed := readFile(t, filepath.Join("..", "seed", "active.go"))
	for _, token := range []string{"StaffLoginCode", "BillStatusOpen", "perfLoginCode", "activeBillNumber"} {
		if !strings.Contains(seed, token) {
			t.Errorf("active fixture seeder missing %q", token)
		}
	}
	main := readFile(t, filepath.Join("..", "seed", "main.go"))
	if !strings.Contains(main, "seedActiveFixtures") {
		t.Error("seedAll does not install active/authenticated fixtures")
	}
}

func TestSystemObserverCapturesRequiredCapacitySignals(t *testing.T) {
	observer := readFile(t, filepath.Join("..", "observe", "capture.sh"))
	for _, token := range []string{
		"cpu_percent", "memory_bytes", "disk_percent", "postgres_open_connections",
		"postgres_waiting_locks", "postgres_lock_wait_ms", "go_goroutines",
		"background_job_lag_seconds", "queue_depth", "sse_dropped",
	} {
		if !strings.Contains(observer, token) {
			t.Errorf("observer missing %q", token)
		}
	}
	if !strings.Contains(observer, "capacity-observations.csv") {
		t.Error("observer must emit a stable capacity-observations.csv artifact")
	}
}

func TestBenchmarkComparisonRejectsMissingAndUnrelatedBaselines(t *testing.T) {
	// A benchmark comparison must never report success against an absent,
	// empty or unrelated baseline; validate-inputs.sh runs before benchstat.
	validator := readFile(t, filepath.Join("..", "bench", "scripts", "validate-inputs.sh"))
	if !regexp.MustCompile(`\[\[ -s "\$\{file\}" \]\]`).MatchString(validator) {
		t.Error("benchmark validator must reject an empty/missing baseline")
	}
	for _, token := range []string{"MINIMUM_BENCHMARKS", "MINIMUM_OVERLAP_PERCENT", "absent or empty"} {
		if !strings.Contains(validator, token) {
			t.Errorf("benchmark validator missing %q", token)
		}
	}
}

func TestStagingPerfLimitsAreExplicitAndConservative(t *testing.T) {
	contract := decodeJSON[capacityContract](t, filepath.Join("..", "capacity-contract.json"))
	compose := readFile(t, filepath.Join("..", "staging", "docker-compose.staging-perf.yml"))
	for _, token := range []string{
		"max_connections=100", "GLOBAL_RATE_LIMIT_REQUESTS_PER_MINUTE=1200",
		"SSE_MAX_SUBSCRIBERS_PER_BUSINESS=200", "SSE_MAX_SUBSCRIBERS_PER_IP=30",
		"cpus:", "mem_limit:",
	} {
		if !strings.Contains(compose, token) {
			t.Errorf("staging perf contract missing %q", token)
		}
	}
	dbConfig := readFile(t, filepath.Join("..", "..", "internal", "database", "db_config.go"))
	pool := trimFloat(contract.RuntimeContract["application_db_pool"])
	if !strings.Contains(dbConfig, "SetMaxOpenConns("+pool+")") {
		t.Errorf("application DB pool drifted from capacity contract (%s)", pool)
	}
}
