package llm

import (
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// FIND-027 regression: docker-compose / .env.example once defaulted chat+director
// to google/gemini-2.0-flash-001 (OpenRouter 404) while LoadModelConfig already
// defaulted to google/gemini-2.5-flash. Keep the three sources of truth aligned.
func TestOpenRouterModelDefaults_ComposeAndEnvExampleMatchLoadModelConfig(t *testing.T) {
	_, thisFile, _, ok := runtime.Caller(0)
	require.True(t, ok)
	repoRoot := filepath.Clean(filepath.Join(filepath.Dir(thisFile), "..", "..", ".."))

	// Authoritative Go defaults (empty env).
	for _, key := range []string{
		"OPENROUTER_MODEL_CHAT",
		"OPENROUTER_MODEL_MENU",
		"OPENROUTER_MODEL_DIRECTOR",
		"OPENROUTER_MODEL_IMAGE",
		"OPENROUTER_MODEL_GUARDRAIL",
	} {
		t.Setenv(key, "")
	}
	cfg := LoadModelConfig()

	want := map[string]string{
		"OPENROUTER_MODEL_CHAT":      cfg.Chat,
		"OPENROUTER_MODEL_MENU":      cfg.Menu,
		"OPENROUTER_MODEL_DIRECTOR":  cfg.Director,
		"OPENROUTER_MODEL_IMAGE":     cfg.Image,
		"OPENROUTER_MODEL_GUARDRAIL": cfg.Guardrail,
	}

	// docker-compose default form: OPENROUTER_MODEL_CHAT=${OPENROUTER_MODEL_CHAT:-google/...}
	composeDefault := regexp.MustCompile(
		`OPENROUTER_MODEL_(CHAT|MENU|DIRECTOR|IMAGE|GUARDRAIL)=\$\{OPENROUTER_MODEL_(?:CHAT|MENU|DIRECTOR|IMAGE|GUARDRAIL):-([^}]+)\}`,
	)
	// .env.example form: OPENROUTER_MODEL_CHAT=google/...
	envExampleLine := regexp.MustCompile(
		`(?m)^OPENROUTER_MODEL_(CHAT|MENU|DIRECTOR|IMAGE|GUARDRAIL)=(\S+)\s*$`,
	)

	composeFiles := composeFilesWithModelDefaults(t, repoRoot)
	for _, rel := range composeFiles {
		path := filepath.Join(repoRoot, rel)
		raw, err := os.ReadFile(path)
		require.NoError(t, err, path)
		body := string(raw)
		found := map[string]string{}
		for _, m := range composeDefault.FindAllStringSubmatch(body, -1) {
			found["OPENROUTER_MODEL_"+m[1]] = m[2]
		}
		for key, wantModel := range want {
			got, ok := found[key]
			require.Truef(t, ok, "%s missing default for %s", rel, key)
			require.Equalf(t, wantModel, got, "%s default for %s", rel, key)
		}
	}

	envPath := filepath.Join(repoRoot, ".env.example")
	envRaw, err := os.ReadFile(envPath)
	require.NoError(t, err, envPath)
	foundEnv := map[string]string{}
	for _, m := range envExampleLine.FindAllStringSubmatch(string(envRaw), -1) {
		foundEnv["OPENROUTER_MODEL_"+m[1]] = m[2]
	}
	for key, wantModel := range want {
		got, ok := foundEnv[key]
		require.Truef(t, ok, ".env.example missing %s", key)
		require.Equalf(t, wantModel, got, ".env.example %s", key)
	}

	// Stale model that OpenRouter 404'd on (FIND-027) must not reappear as a default.
	stale := "google/gemini-2.0-flash-001"
	for _, rel := range append(composeFiles, ".env.example") {
		raw, err := os.ReadFile(filepath.Join(repoRoot, rel))
		require.NoError(t, err)
		// Only flag when used as a *default assignment*, not docs/comments about prices.
		if rel == ".env.example" {
			require.NotRegexp(t,
				regexp.MustCompile(`(?m)^OPENROUTER_MODEL_(CHAT|DIRECTOR)=`+regexp.QuoteMeta(stale)+`\s*$`),
				string(raw),
				"%s must not assign stale chat/director model", rel,
			)
		} else {
			require.NotContains(t, string(raw), "OPENROUTER_MODEL_CHAT=${OPENROUTER_MODEL_CHAT:-"+stale+"}", rel)
			require.NotContains(t, string(raw), "OPENROUTER_MODEL_DIRECTOR=${OPENROUTER_MODEL_DIRECTOR:-"+stale+"}", rel)
		}
	}
}

// composeFilesWithModelDefaults returns the local compose file plus any
// deployment compose files that set OpenRouter model defaults, so a new
// self-host compose file is held to the same defaults automatically.
func composeFilesWithModelDefaults(t *testing.T, repoRoot string) []string {
	t.Helper()
	files := []string{"docker-compose.yml"}
	for _, pattern := range []string{"deploy/*.yml", "deploy/*/*.yml", "deploy/*.yaml", "deploy/*/*.yaml"} {
		matches, err := filepath.Glob(filepath.Join(repoRoot, pattern))
		require.NoError(t, err)
		for _, match := range matches {
			raw, err := os.ReadFile(match)
			require.NoError(t, err, match)
			if !strings.Contains(string(raw), "OPENROUTER_MODEL_") {
				continue
			}
			rel, err := filepath.Rel(repoRoot, match)
			require.NoError(t, err)
			files = append(files, rel)
		}
	}
	return files
}
