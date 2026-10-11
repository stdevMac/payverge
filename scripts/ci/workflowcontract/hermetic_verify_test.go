package workflowcontract

import (
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"
	"testing"
)

func TestHermeticVerificationEntryPointsAreExecutable(t *testing.T) {
	for _, name := range []string{"verify-backend.sh", "verify-frontend.sh"} {
		path := filepath.Join("..", "..", "..", "scripts", name)
		info, err := os.Stat(path)
		if err != nil {
			t.Fatalf("stat %s: %v", path, err)
		}
		if info.Mode().Perm()&0o111 == 0 {
			t.Errorf("%s mode %04o is not executable", path, info.Mode().Perm())
		}
	}
}

func TestHermeticBackendVerificationEntryPoint(t *testing.T) {
	makefile := readRepoFile(t, "Makefile")
	if !strings.Contains(makefile, `verify\:backend:`) {
		t.Fatal("root Makefile must expose make verify:backend")
	}

	script := readRepoFile(t, "scripts", "verify-backend.sh")
	for _, required := range []string{
		"unset FRONTEND_URL BASE_URL NEXT_PUBLIC_BASE_URL APP_BASE_URL",
		"unset ALLOWED_REDIRECT_DOMAINS APP_DOMAIN API_DOMAIN COOKIE_DOMAIN DOMAIN",
		"unset ENV APP_ENV NODE_ENV NEXT_PUBLIC_ENV",
		"unset GOOGLE_TRANSLATE_API_KEY GEMINI_API_KEY OPENROUTER_API_KEY FIRECRAWL_API_KEY LIFI_API_KEY",
		"export PLUGIN_SECRET_KEY=payverge-hermetic-test-key-00000",
		"go test -count=1 -race ./...",
		"go test -count=1 -tags integration ./internal/server/...",
		"go test -count=1 -tags integration_postgres ./internal/database -run 'Test(RollbackRehearsal_EachMigration|DemoSeed_PostgresIdempotentAndVerified)'",
		"go vet ./...",
		"go build ./...",
	} {
		if !strings.Contains(script, required) {
			t.Errorf("backend verification entry point missing %q", required)
		}
	}
}

func TestHermeticFrontendVerificationEntryPoint(t *testing.T) {
	makefile := readRepoFile(t, "Makefile")
	if !strings.Contains(makefile, `verify\:frontend:`) {
		t.Fatal("root Makefile must expose make verify:frontend")
	}

	script := readRepoFile(t, "scripts", "verify-frontend.sh")
	for _, required := range []string{
		"unset FRONTEND_URL BASE_URL NEXT_PUBLIC_BASE_URL",
		"npm ci",
		"npm run lint -- --max-warnings 0",
		"npm run typecheck",
		"npm run i18n:check",
		"npm run i18n:validate",
		"npm run build",
		"npm exec -- jest --watchman=false --runInBand",
	} {
		if !strings.Contains(script, required) {
			t.Errorf("frontend verification entry point missing %q", required)
		}
	}
}

func TestReleaseToolchainsUseExactRepositoryVersions(t *testing.T) {
	const nodeVersion = "26.10.0"
	const npmVersion = "11.19.1"
	const goVersion = "1.27.2"

	if got := strings.TrimSpace(readRepoFile(t, ".nvmrc")); got != nodeVersion {
		t.Fatalf(".nvmrc = %q, want %q", got, nodeVersion)
	}
	backendModule := readRepoFile(t, "backend", "go.mod")
	if !strings.Contains(backendModule, "toolchain go"+goVersion) {
		t.Fatalf("backend go.mod must pin toolchain go%s", goVersion)
	}

	frontendPackage := readRepoFile(t, "frontend", "package.json")
	if !strings.Contains(frontendPackage, `"packageManager": "npm@`+npmVersion+`"`) {
		t.Fatalf("frontend packageManager must pin npm %s", npmVersion)
	}
	// The root package.json only wraps npx eval scripts; a non-npm
	// packageManager there makes Corepack demand pnpm in an npm repository.
	rootPackage := readRepoFile(t, "package.json")
	if strings.Contains(rootPackage, `"packageManager"`) && !strings.Contains(rootPackage, `"packageManager": "npm@`) {
		t.Fatalf("root package.json must not declare a non-npm packageManager")
	}
	frontendDockerfile := readRepoFile(t, "frontend", "Dockerfile")
	if got := strings.Count(frontendDockerfile, "FROM node:"+nodeVersion+"-alpine"); got != 3 {
		t.Fatalf("frontend Dockerfile exact Node base count = %d, want 3", got)
	}
	backendDockerfile := readRepoFile(t, "backend", "Dockerfile")
	if !strings.Contains(backendDockerfile, "FROM golang:"+goVersion+"-bookworm") {
		t.Fatalf("backend Dockerfile must use Go %s", goVersion)
	}

	// Workflows read the toolchain from the repository, never a literal: one
	// version bump (go.mod, .nvmrc) moves CI, images and contributors together.
	for _, name := range publicWorkflows {
		wf := loadPublicWorkflow(t, name)
		for _, jobName := range sortedKeys(wf.Jobs) {
			for _, step := range wf.Jobs[jobName].Steps {
				where := name + "/" + jobName
				switch {
				case strings.HasPrefix(step.Uses, "actions/setup-go@"):
					if step.With["go-version-file"] != "backend/go.mod" || step.With["go-version"] != nil {
						t.Errorf("%s: setup-go must use go-version-file: backend/go.mod and no go-version", where)
					}
				case strings.HasPrefix(step.Uses, "actions/setup-node@"):
					if step.With["node-version-file"] != ".nvmrc" || step.With["node-version"] != nil {
						t.Errorf("%s: setup-node must use node-version-file: .nvmrc and no node-version", where)
					}
				}
			}
		}
	}
}

func TestNightlyE2EProvesSeedIdempotencyAndAlwaysRetainsDiagnostics(t *testing.T) {
	workflow := readRepoFile(t, ".github", "workflows", "e2e.yml")
	// Either two literal applications or one loop that runs twice.
	twice := regexp.MustCompile(`(?s)for _ in 1 2; do\n.*< backend/scripts/demo_seed\.sql\n.*< backend/scripts/verify_demo_seed\.sql\n\s*done`)
	if !twice.MatchString(workflow) {
		if got := strings.Count(workflow, "< backend/scripts/demo_seed.sql"); got < 2 {
			t.Fatalf("nightly E2E must apply the demo seed twice; found %d applications", got)
		}
		if got := strings.Count(workflow, "< backend/scripts/verify_demo_seed.sql"); got < 2 {
			t.Fatalf("nightly E2E must verify fixture invariants after both seed applications; found %d", got)
		}
	}
	for _, stepName := range []string{
		"Collect compose diagnostics",
		"Upload compose diagnostics",
		"Upload Playwright diagnostics",
	} {
		stepPattern := regexp.MustCompile(`(?s)- name: ` + regexp.QuoteMeta(stepName) + `\n\s+if: always\(\)`)
		if !stepPattern.MatchString(workflow) {
			t.Errorf("%s must run under if: always()", stepName)
		}
	}
}

// unsetVars returns the variable names of every top-level `unset` line.
func unsetVars(script string) []string {
	var vars []string
	for _, line := range strings.Split(script, "\n") {
		if fields := strings.Fields(line); len(fields) > 1 && fields[0] == "unset" {
			vars = append(vars, fields[1:]...)
		}
	}
	sort.Strings(vars)
	return vars
}

// TestBackendEnvMatchesVerifyBackend keeps the CI wrapper and the local
// `make verify:backend` entry point on one hermetic environment, so a test
// that passes locally cannot depend on a variable CI leaves set, or the
// reverse.
func TestBackendEnvMatchesVerifyBackend(t *testing.T) {
	ci := readRepoFile(t, "scripts", "ci", "backend-env.sh")
	local := readRepoFile(t, "scripts", "verify-backend.sh")

	ciVars, localVars := unsetVars(ci), unsetVars(local)
	if !slices.Equal(ciVars, localVars) {
		t.Errorf("unset lists differ:\n  scripts/ci/backend-env.sh: %v\n  scripts/verify-backend.sh: %v", ciVars, localVars)
	}
	for _, name := range []string{
		"FRONTEND_URL", "BASE_URL", "NEXT_PUBLIC_BASE_URL", "APP_BASE_URL", "ALLOWED_REDIRECT_DOMAINS",
		"APP_DOMAIN", "API_DOMAIN", "COOKIE_DOMAIN", "DOMAIN", "ENV", "APP_ENV", "NODE_ENV", "NEXT_PUBLIC_ENV",
		"GOOGLE_TRANSLATE_API_KEY", "GEMINI_API_KEY", "OPENROUTER_API_KEY", "FIRECRAWL_API_KEY", "LIFI_API_KEY",
		"NANOBANANA_API_KEY", "PUBLIC_URL", "STORAGE_DRIVER", "EMAIL_PROVIDER",
		"REGISTRATION_MODE", "LLM_BASE_URL", "LLM_API_KEY",
	} {
		if !slices.Contains(ciVars, name) {
			t.Errorf("scripts/ci/backend-env.sh must unset %s", name)
		}
	}
	for _, export := range []string{"export PLUGIN_SECRET_KEY=payverge-hermetic-test-key-00000", "export GO_ENV=test"} {
		if !strings.Contains(ci, export) {
			t.Errorf("scripts/ci/backend-env.sh is missing %q", export)
		}
	}
	if !strings.Contains(local, "export PLUGIN_SECRET_KEY=payverge-hermetic-test-key-00000") {
		t.Error("scripts/verify-backend.sh must export the same PLUGIN_SECRET_KEY as CI")
	}
	info, err := os.Stat(repoPath("scripts", "ci", "backend-env.sh"))
	if err != nil || info.Mode().Perm()&0o111 == 0 {
		t.Errorf("scripts/ci/backend-env.sh must exist and be executable (%v)", err)
	}
}
