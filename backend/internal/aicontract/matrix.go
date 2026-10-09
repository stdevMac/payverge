package aicontract

import (
	"fmt"
	"sync"
)

// ScenarioRunner exercises one hermetic scenario against real production packages.
// Runners must drive shipped entry points (handlers/services/DB), not reimplement contracts.
type ScenarioRunner func(sc Scenario) (ScenarioRunResult, error)

// ScenarioRunResult is the observable outcome of a runner for expect matching.
type ScenarioRunResult struct {
	Code          string
	Text          string
	Effects       map[string]int
	ProviderCalls int
	ServedModel   string
	PrivacyClass  string
	ZDR           bool
}

var (
	runnerMu sync.RWMutex
	runners  = map[string]ScenarioRunner{}
)

// RegisterScenarioRunner binds a scenario id to a production-path runner.
// Called from adapter init() or TestMain setup in the aicontract test package.
func RegisterScenarioRunner(id string, fn ScenarioRunner) {
	if id == "" || fn == nil {
		panic("aicontract: RegisterScenarioRunner requires id and fn")
	}
	runnerMu.Lock()
	defer runnerMu.Unlock()
	if _, exists := runners[id]; exists {
		panic(fmt.Sprintf("aicontract: duplicate runner for %q", id))
	}
	runners[id] = fn
}

// RegisteredScenarioIDs returns sorted scenario ids with runners (for tests).
func RegisteredScenarioIDs() []string {
	runnerMu.RLock()
	defer runnerMu.RUnlock()
	out := make([]string, 0, len(runners))
	for id := range runners {
		out = append(out, id)
	}
	return out
}

// GetScenarioRunner returns the runner for id, if any.
func GetScenarioRunner(id string) (ScenarioRunner, bool) {
	runnerMu.RLock()
	defer runnerMu.RUnlock()
	fn, ok := runners[id]
	return fn, ok
}

// MatchExpect checks runner results against the scenario contract.
func MatchExpect(sc Scenario, res ScenarioRunResult) error {
	if sc.Expect.Code != "" && res.Code != "" && sc.Expect.Code != res.Code {
		return fmt.Errorf("code want %q got %q", sc.Expect.Code, res.Code)
	}
	for _, needle := range sc.Expect.TextContains {
		if needle != "" && !containsFold(res.Text, needle) {
			return fmt.Errorf("text missing %q in %q", needle, truncate(res.Text, 120))
		}
	}
	for _, bad := range sc.Expect.TextExcludes {
		if bad != "" && containsFold(res.Text, bad) {
			return fmt.Errorf("text must not contain %q", bad)
		}
	}
	for k, want := range sc.Expect.Effects {
		got := 0
		if res.Effects != nil {
			got = res.Effects[k]
		}
		if got != want {
			return fmt.Errorf("effect %s want %d got %d", k, want, got)
		}
	}
	if sc.Expect.ProviderCalls != 0 || res.ProviderCalls != 0 {
		// Only enforce when either side is non-zero, or expect explicitly zero via effects.
		if sc.Expect.ProviderCalls > 0 && res.ProviderCalls != sc.Expect.ProviderCalls {
			return fmt.Errorf("provider_calls want %d got %d", sc.Expect.ProviderCalls, res.ProviderCalls)
		}
	}
	if sc.Expect.ZDR != nil && *sc.Expect.ZDR && !res.ZDR {
		return fmt.Errorf("expected zdr=true")
	}
	if sc.Expect.NoOperationalMutation {
		for _, key := range []string{"menu_mutation", "bill_mutation", "director_apply"} {
			if res.Effects != nil && res.Effects[key] != 0 {
				return fmt.Errorf("operational mutation %s=%d", key, res.Effects[key])
			}
		}
	}
	return nil
}

func containsFold(hay, needle string) bool {
	return len(needle) == 0 || (len(hay) > 0 && (indexFold(hay, needle) >= 0))
}

func indexFold(s, sub string) int {
	// simple case-insensitive substring search
	ls, lsub := len(s), len(sub)
	if lsub == 0 {
		return 0
	}
	for i := 0; i+lsub <= ls; i++ {
		if equalFoldASCII(s[i:i+lsub], sub) {
			return i
		}
	}
	return -1
}

func equalFoldASCII(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := 0; i < len(a); i++ {
		ca, cb := a[i], b[i]
		if ca >= 'A' && ca <= 'Z' {
			ca += 'a' - 'A'
		}
		if cb >= 'A' && cb <= 'Z' {
			cb += 'a' - 'A'
		}
		if ca != cb {
			return false
		}
	}
	return true
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
