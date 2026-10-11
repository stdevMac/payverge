package workflowcontract

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// The WhatsApp channel links go.mau.fi/libsignal (GPL-3.0) and is opt-in via
// the `whatsapp` Go build tag. These contracts keep the default image GPL-free
// and keep anyone from publishing a tagged image by accident.

func TestBackendDockerfileGoTagsDefaultEmpty(t *testing.T) {
	dockerfile := readRepoFile(t, "backend", "Dockerfile")

	args := regexp.MustCompile(`(?m)^ARG GO_TAGS(=.*)?$`).FindAllString(dockerfile, -1)
	if len(args) != 1 || args[0] != `ARG GO_TAGS=""` {
		t.Fatalf(`backend Dockerfile must declare exactly one ARG GO_TAGS="" (empty default = no whatsapp); got %q`, args)
	}

	builds := strings.Count(dockerfile, "go build")
	tagged := strings.Count(dockerfile, `-tags "$GO_TAGS"`)
	if builds == 0 || builds != tagged {
		t.Fatalf(`every go build in backend/Dockerfile must pass -tags "$GO_TAGS" (go build=%d, tagged=%d)`, builds, tagged)
	}
}

func TestNoPublishedWhatsAppTaggedImage(t *testing.T) {
	forbidden := regexp.MustCompile(`GO_TAGS\s*[:=]\s*["']?[^"'\n]*\bwhatsapp\b`)
	var files [][]string
	workflows, err := os.ReadDir(filepath.Join("..", "..", "..", ".github", "workflows"))
	if err != nil {
		t.Fatalf("read workflows: %v", err)
	}
	for _, entry := range workflows {
		if strings.HasSuffix(entry.Name(), ".yml") || strings.HasSuffix(entry.Name(), ".yaml") {
			files = append(files, []string{".github", "workflows", entry.Name()})
		}
	}
	composeFiles, err := filepath.Glob(filepath.Join("..", "..", "..", "docker-compose*.yml"))
	if err != nil {
		t.Fatalf("glob compose files: %v", err)
	}
	for _, path := range composeFiles {
		files = append(files, []string{filepath.Base(path)})
	}
	if len(files) == 0 {
		t.Fatal("no workflow or compose files found")
	}
	for _, parts := range files {
		if loc := forbidden.FindString(readRepoFile(t, parts...)); loc != "" {
			t.Errorf("%s builds a whatsapp-tagged (GPL-3.0) image via %q; release images must use the empty GO_TAGS default", filepath.Join(parts...), loc)
		}
	}
}

// The backend reads WHATSAPP_ENABLED only from its process environment, and
// compose forwards only the variables a service lists. Without the pass-through
// the documented "set WHATSAPP_ENABLED=true in .env" never reaches a tagged
// backend, and an untagged one never logs the "ignored" warning.
func TestComposeBackendPassesWhatsAppSwitches(t *testing.T) {
	// backendList returns a list-form key (environment, build.args) of the
	// backend service; other services may use the map form, so stay untyped.
	backendList := func(name string, path ...string) []string {
		t.Helper()
		var doc map[string]any
		if err := yaml.Unmarshal([]byte(readRepoFile(t, name)), &doc); err != nil {
			t.Fatalf("parse %s: %v", name, err)
		}
		node := any(doc)
		for _, key := range append([]string{"services", "backend"}, path...) {
			m, ok := node.(map[string]any)
			if !ok {
				t.Fatalf("%s: services.backend.%s is not a mapping", name, strings.Join(path, "."))
			}
			node = m[key]
		}
		items, _ := node.([]any)
		out := make([]string, 0, len(items))
		for _, item := range items {
			if s, ok := item.(string); ok {
				out = append(out, s)
			}
		}
		return out
	}

	want := "WHATSAPP_ENABLED=${WHATSAPP_ENABLED:-false}"
	found := false
	for _, entry := range backendList("docker-compose.yml", "environment") {
		found = found || entry == want
	}
	if !found {
		t.Errorf("docker-compose.yml backend environment must list %q", want)
	}

	// The source-built stack takes GO_TAGS from .env, defaulting to empty
	// (the GPL-free image), so `up --build` keeps whatever the operator chose.
	if args := backendList("docker-compose.yml", "build", "args"); len(args) != 1 || args[0] != "GO_TAGS=${GO_TAGS:-}" {
		t.Errorf("docker-compose.yml backend build args must be exactly [GO_TAGS=${GO_TAGS:-}]; got %q", args)
	}
}

func TestGPLFreeGateEntryPoint(t *testing.T) {
	makefile := readRepoFile(t, "Makefile")
	if !regexp.MustCompile(`(?m)^check-gpl-free:\n(\t.*\n)*\t@bash \./scripts/ci/check-gpl-free\.sh$`).MatchString(makefile) {
		t.Fatal("root Makefile must expose make check-gpl-free running scripts/ci/check-gpl-free.sh")
	}

	path := filepath.Join("..", "..", "..", "scripts", "ci", "check-gpl-free.sh")
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat %s: %v", path, err)
	}
	if info.Mode().Perm()&0o111 == 0 {
		t.Errorf("%s mode %04o is not executable", path, info.Mode().Perm())
	}

	script := readRepoFile(t, "scripts", "ci", "check-gpl-free.sh")
	for _, required := range []string{
		`"go.mau.fi"`,
		`SHIPPED_PACKAGES=(./cmd/app ./cmd/email-smoke ./cmd/healthcheck)`,
		`CGO_ENABLED=0 GOOS=linux "$GO" list -tags "$tags" -deps`,
		`"$GO" build -tags whatsapp ./...`,
	} {
		if !strings.Contains(script, required) {
			t.Errorf("scripts/ci/check-gpl-free.sh lost %q", required)
		}
	}
}
