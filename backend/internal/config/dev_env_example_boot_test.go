package config

import (
	"bufio"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// composeJWTDefaultRE captures the dev-stack interpolation in the root
// docker-compose.yml: JWT_SECRET_KEY=${JWT_SECRET_KEY:-<default>}.
var composeJWTDefaultRE = regexp.MustCompile(`JWT_SECRET_KEY=\$\{JWT_SECRET_KEY:-([^}]*)\}`)

// TestRootEnvExampleBootsDevCompose reproduces `cp .env.example .env &&
// docker compose config`: compose resolves ${JWT_SECRET_KEY:-default} from the
// copied .env, and the backend must then pass ValidateConfig in development.
// A placeholder assignment in .env.example would shadow the compose default and
// stop the documented quick start from booting.
func TestRootEnvExampleBootsDevCompose(t *testing.T) {
	envPath := repoRootEnvExample(t)
	root := filepath.Dir(envPath)

	compose, err := os.ReadFile(filepath.Join(root, "docker-compose.yml"))
	if err != nil {
		t.Fatal(err)
	}
	m := composeJWTDefaultRE.FindSubmatch(compose)
	if m == nil {
		t.Fatal("root docker-compose.yml no longer defaults JWT_SECRET_KEY via ${JWT_SECRET_KEY:-...}")
	}
	resolved := string(m[1])

	f, err := os.Open(envPath)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if strings.HasPrefix(line, "#") {
			continue
		}
		if v, ok := strings.CutPrefix(line, "JWT_SECRET_KEY="); ok {
			// ${VAR:-default} keeps the default only when VAR is unset or empty.
			if v = strings.Trim(strings.TrimSpace(v), `"'`); v != "" {
				resolved = v
			}
		}
	}
	if err := sc.Err(); err != nil {
		t.Fatal(err)
	}

	for _, k := range []string{"ENV", "APP_ENV", "GO_ENV"} {
		t.Setenv(k, "development")
	}
	t.Setenv("JWT_SECRET_KEY", resolved)
	for _, e := range ValidateConfig("db", "payverge", "dev-db-password", "payverge", false) {
		if e.Field == "JWT_SECRET_KEY" {
			t.Fatalf("copied .env.example resolves JWT_SECRET_KEY to %q, which fails ValidateConfig: %s", resolved, e.Message)
		}
	}
}
