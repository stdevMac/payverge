package workflowcontract

import (
	"encoding/json"
	"os/exec"
	"path"
	"regexp"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestBackendMakeTargetsAreHonest(t *testing.T) {
	makefile := readRepoFile(t, "backend", "Makefile")
	unit := makefileRecipe(t, makefile, "test-unit")
	if !strings.Contains(unit, "go test -short -race ./...") {
		t.Errorf("test-unit recipe = %q, want go test -short -race ./...", unit)
	}
	integration := makefileRecipe(t, makefile, "test-integration")
	requireContainsAll(t, "test-integration recipe", integration,
		"go-pg-packages.sh", "-tags integration,integration_postgres")
	if strings.Contains(integration, "./internal/tests/...") {
		t.Errorf("test-integration still runs ./internal/tests/...:\n%s", integration)
	}
	lint := makefileRecipe(t, makefile, "lint")
	if !strings.Contains(lint, "--new-from-rev=$(LINT_BASE)") {
		t.Errorf("lint recipe = %q, want --new-from-rev=$(LINT_BASE)", lint)
	}
	if !strings.Contains(makefile, "LINT_BASE ?= origin/main") {
		t.Error("backend/Makefile must set LINT_BASE ?= origin/main")
	}
}

func TestGolangciConfigHasNoBaseline(t *testing.T) {
	cfg := readRepoFile(t, "backend", ".golangci.yml")
	if regexp.MustCompile(`(?m)^\s*new-from-rev:`).MatchString(cfg) {
		t.Error("backend/.golangci.yml must not set new-from-rev; callers pass the baseline")
	}
	ci := readRepoFile(t, ".github", "workflows", "ci.yml")
	if !strings.Contains(ci, "--new-from-rev=") {
		t.Error(".github/workflows/ci.yml must still pass --new-from-rev=")
	}
}

func TestPlaywrightDefaultsToDevServer(t *testing.T) {
	cfg := readRepoFile(t, "frontend", "playwright.config.ts")
	if !strings.Contains(cfg, `"http://localhost:3000"`) {
		t.Error(`playwright.config.ts must default baseURL to "http://localhost:3000"`)
	}
	if strings.Contains(cfg, "localhost:3001") {
		t.Error("playwright.config.ts must not target localhost:3001")
	}
	raw := readRepoFile(t, "frontend", "package.json")
	var pkg struct {
		Scripts map[string]string `json:"scripts"`
	}
	if err := json.Unmarshal([]byte(raw), &pkg); err != nil {
		t.Fatalf("parse frontend/package.json: %v", err)
	}
	staff := pkg.Scripts["test:e2e:staff"]
	if !strings.Contains(staff, "--project=chromium") {
		t.Errorf("test:e2e:staff = %q, want --project=chromium", staff)
	}
}

func TestImagesDoNotUpgradeOrInstallUnpinnedPackages(t *testing.T) {
	frontend := readRepoFile(t, "frontend", "Dockerfile")
	if strings.Contains(frontend, "apk upgrade") {
		t.Error("frontend/Dockerfile must not run apk upgrade")
	}
	backend := readRepoFile(t, "backend", "Dockerfile")
	if strings.Contains(backend, "apt-get") {
		t.Error("backend/Dockerfile must not run apt-get")
	}
}

// SC-08: every tracked go.mod, and every tracked package.json that declares
// dependencies, has a Dependabot entry for its ecosystem and directory. A
// package.json with no dependencies (the repo root's script runner,
// frontend/ds) gives Dependabot nothing to update.
func TestDependabotCoversEveryManifest(t *testing.T) {
	var cfg struct {
		Updates []struct {
			Ecosystem   string   `yaml:"package-ecosystem"`
			Directory   string   `yaml:"directory"`
			Directories []string `yaml:"directories"`
		} `yaml:"updates"`
	}
	if err := yaml.Unmarshal([]byte(readRepoFile(t, ".github", "dependabot.yml")), &cfg); err != nil {
		t.Fatalf("parse .github/dependabot.yml: %v", err)
	}
	covered := map[string]bool{}
	for _, u := range cfg.Updates {
		for _, dir := range append([]string{u.Directory}, u.Directories...) {
			if dir != "" {
				covered[u.Ecosystem+" "+dir] = true
			}
		}
	}

	cmd := exec.Command("git", "ls-files", "-z", "--cached", "--others", "--exclude-standard", "--", "*go.mod", "*package.json")
	cmd.Dir = repoPath()
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("git ls-files: %v", err)
	}
	var missing []string
	for _, file := range strings.Split(string(out), "\x00") {
		ecosystem := ""
		switch path.Base(file) {
		case "go.mod":
			ecosystem = "gomod"
		case "package.json":
			ecosystem = "npm"
			var pkg struct {
				Dependencies    map[string]string `json:"dependencies"`
				DevDependencies map[string]string `json:"devDependencies"`
			}
			if err := json.Unmarshal([]byte(readRepoFile(t, strings.Split(file, "/")...)), &pkg); err != nil {
				t.Fatalf("parse %s: %v", file, err)
			}
			if len(pkg.Dependencies)+len(pkg.DevDependencies) == 0 {
				continue
			}
		default:
			continue
		}
		dir := "/" + strings.TrimSuffix(path.Dir(file), ".")
		dir = strings.TrimSuffix(dir, "/")
		if dir == "" {
			dir = "/"
		}
		if !covered[ecosystem+" "+dir] {
			missing = append(missing, ecosystem+" "+dir)
		}
	}
	if len(missing) > 0 {
		t.Errorf(".github/dependabot.yml has no entry for:\n%s", strings.Join(missing, "\n"))
	}
}

func makefileRecipe(t *testing.T, makefile, target string) string {
	t.Helper()
	lines := strings.Split(makefile, "\n")
	header := target + ":"
	for i, line := range lines {
		if line != header && !strings.HasPrefix(line, header+" ") && !strings.HasPrefix(line, header+"\t") {
			continue
		}
		var b strings.Builder
		for _, body := range lines[i+1:] {
			if body == "" || body[0] != '\t' {
				break
			}
			b.WriteString(body)
			b.WriteByte('\n')
		}
		if b.Len() == 0 {
			t.Fatalf("make target %s has an empty recipe", target)
		}
		return b.String()
	}
	t.Fatalf("make target %s not found", target)
	return ""
}
