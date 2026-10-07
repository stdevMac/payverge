package config

import (
	"bufio"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// repoSecretPathspecs selects every tracked file that can carry a literal
// secret assignment: compose files, env templates and CI fixtures, workflow
// YAML, shell scripts, Node release scripts and their tests, and Makefiles.
// Globbing (rather than a fixed list) means a secret committed to a new
// script or workflow fails TestKnownUnsafeSecretsCoverRepoLiterals.
// frontend/.env* is excluded: the frontend workspace owns its env template
// and the backend reads no secret from it.
var repoSecretPathspecs = []string{
	":(glob)**/docker-compose*.yml",
	":(glob)**/.env*",
	":(glob)**/*.env.example",
	":(glob)**/*.env.fixture",
	":(glob).github/**/*.yml",
	":(glob)**/*.sh",
	":(glob)scripts/**/*.mjs",
	":(glob)**/Makefile",
	":(exclude,glob)frontend/.env*",
	":(exclude,glob)**/node_modules/**",
}

// repoSecretFiles is the fallback scan set when git is unavailable (for
// example a source tarball without .git).
var repoSecretFiles = []string{
	".env.example",
	".github/e2e/ci.env.fixture",
	".github/workflows/perf-bench.yml",
	"backend/.env.example",
	"backend/perf/staging/.env.example",
	"backend/perf/staging/docker-compose.staging-perf.yml",
	"backend/scripts/run-restore-drill.sh",
	"docker-compose.yml",
	"docker-compose.production.yml",
	"docker-compose.prod-local.yml",
	"docker-compose.digest-deploy.yml",
	"docker-compose.release-smoke.yml",
	"infra/monitoring/docker-compose.yml",
	"scripts/validate-production-config.test.mjs",
	"scripts/verify-backend.sh",
}

var (
	repoSecretKeyRE      = regexp.MustCompile(`(PASSWORD|SECRET|TOKEN|API_KEY|ACCESS_KEY|PRIVATE_KEY|_KEY$|PASS$)`)
	repoSecretKeyExclude = regexp.MustCompile(`(PUBLIC|PUBLISHABLE|_TTL|_URL|_FILE|_PATH|_ID$|_EXPIR|_LENGTH|_HEADER|_ENABLED|_BACKFILL|^CADDY_TLS)`)
	repoEnvLineRE        = regexp.MustCompile(`^(?:export\s+)?([A-Z][A-Z0-9_]*)=(.*)$`)
	repoListLineRE       = regexp.MustCompile(`^-\s*["']?([A-Z][A-Z0-9_]*)=(.*?)["']?$`)
	repoMapLineRE        = regexp.MustCompile(`^["']?([A-Z][A-Z0-9_]*)["']?:\s*(.*)$`)
	repoInterpDefaultRE  = regexp.MustCompile(`\$\{([A-Z][A-Z0-9_]*):?-([^}]*)\}`)
	// repoInlineAssignRE finds KEY=value tokens inside a command line
	// (docker run -e KEY=value, KEY=value go test, env KEY=value cmd).
	repoInlineAssignRE  = regexp.MustCompile(`(?:^|[\s;(])([A-Z][A-Z0-9_]*)=("[^"]*"|'[^']*'|[^\s"'\\;)]+)`)
	repoTrailingComment = regexp.MustCompile(`\s+#.*$`)
	repoLiteralHasAlnum = regexp.MustCompile(`[A-Za-z0-9]`)
)

type repoSecretLiteral struct {
	file  string
	line  int
	key   string
	value string
}

// findRepoRoot walks up from the package directory to the monorepo root.
func findRepoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	require.NoError(t, err)
	for {
		if _, err := os.Stat(filepath.Join(dir, "docker-compose.yml")); err == nil {
			if _, err := os.Stat(filepath.Join(dir, "backend", "go.mod")); err == nil {
				return dir
			}
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Skip("repository root not found; secret literal scan needs the source tree")
		}
		dir = parent
	}
}

// listRepoSecretFiles returns the tracked files matching repoSecretPathspecs,
// or the static fallback list when git cannot list them.
func listRepoSecretFiles(t *testing.T, root string) []string {
	t.Helper()
	args := append([]string{"-C", root, "ls-files", "-z", "--"}, repoSecretPathspecs...)
	out, err := exec.Command("git", args...).Output()
	if err != nil || len(out) == 0 {
		t.Logf("git ls-files unavailable (%v); scanning the fallback file list", err)
		return repoSecretFiles
	}
	var files []string
	for _, rel := range strings.Split(string(out), "\x00") {
		if rel != "" {
			files = append(files, rel)
		}
	}
	return files
}

// cleanRepoSecretValue strips quoting, trailing comments, line continuations
// and JS/YAML separators. Values built from other variables or expressions
// (anything containing "$") are not literals and are skipped.
func cleanRepoSecretValue(raw string) (string, bool) {
	value := repoTrailingComment.ReplaceAllString(strings.TrimSpace(raw), "")
	value = strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(value), `\`))
	value = strings.TrimSpace(strings.TrimSuffix(value, ","))
	value = strings.Trim(value, `"'`)
	// Values built from other variables or printf verbs are templates.
	// Values with no letter or digit are syntax, not secrets: shell case
	// patterns (`KEY=)`) and sed/regex fragments (`s|^KEY=.*|...|`).
	if strings.Contains(value, "$") || strings.Contains(value, "%s") || !repoLiteralHasAlnum.MatchString(value) {
		return "", false
	}
	return value, true
}

// scanRepoSecretLiterals extracts every literal value assigned to a
// secret-shaped variable: KEY=value (env files, compose list form, and
// commented examples), KEY: value (compose/workflow map form and JS object
// literals), inline KEY=value tokens on command lines, and ${KEY:-default}
// interpolation defaults. Values built from other variables are skipped.
func scanRepoSecretLiterals(t *testing.T, root string) []repoSecretLiteral {
	t.Helper()
	var out []repoSecretLiteral
	scanned := 0
	for _, rel := range listRepoSecretFiles(t, root) {
		f, err := os.Open(filepath.Join(root, rel))
		if os.IsNotExist(err) {
			t.Logf("skipping missing %s", rel)
			continue
		}
		require.NoError(t, err)
		scanned++
		scanner := bufio.NewScanner(f)
		scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
		lineNo := 0
		for scanner.Scan() {
			lineNo++
			line := strings.TrimSpace(scanner.Text())
			line = strings.TrimSpace(strings.TrimLeft(line, "#"))
			type pair struct{ key, value string }
			var pairs []pair
			seen := map[pair]bool{}
			add := func(key, raw string) {
				value, ok := cleanRepoSecretValue(raw)
				p := pair{key, value}
				if ok && !seen[p] {
					seen[p] = true
					pairs = append(pairs, p)
				}
			}
			for _, m := range repoInterpDefaultRE.FindAllStringSubmatch(line, -1) {
				add(m[1], m[2])
			}
			m := repoEnvLineRE.FindStringSubmatch(line)
			if m == nil {
				m = repoListLineRE.FindStringSubmatch(line)
			}
			if m == nil {
				m = repoMapLineRE.FindStringSubmatch(line)
			}
			if m != nil {
				add(m[1], m[2])
			} else {
				for _, im := range repoInlineAssignRE.FindAllStringSubmatch(line, -1) {
					add(im[1], im[2])
				}
			}
			for _, p := range pairs {
				if !repoSecretKeyRE.MatchString(p.key) || repoSecretKeyExclude.MatchString(p.key) {
					continue
				}
				switch strings.ToLower(p.value) {
				case "", "true", "false", "0", "1":
					continue
				}
				out = append(out, repoSecretLiteral{file: rel, line: lineNo, key: p.key, value: p.value})
			}
		}
		require.NoError(t, scanner.Err())
		_ = f.Close()
	}
	if scanned == 0 {
		t.Skip("no env/compose files found to scan")
	}
	return out
}

// docSecretPathspecs selects tracked Markdown. READMEs, runbooks, plans and
// the benchmark log publish copy-paste commands, and a secret shown in one is
// as public as a compose default.
var docSecretPathspecs = []string{
	":(glob)**/*.md",
	":(exclude,glob)**/node_modules/**",
}

var (
	// docSecretKeyRE narrows the Markdown scan to variables that feed the
	// always-required production secret slots (JWT, plugin key, database and
	// admin passwords). Prose names many other secret-shaped variables
	// (error codes such as AUTH_TOKEN_MISSING, "sk-or-..." key placeholders),
	// so the broad repoSecretKeyRE would only flag noise there.
	docSecretKeyRE = regexp.MustCompile(`(JWT_SECRET_KEY|PLUGIN_SECRET_KEY|DB_PASSWORD|POSTGRES_PASSWORD|ADMIN_PASSWORD)$`)
	// docInlineAssignRE finds KEY=value tokens anywhere in a Markdown line:
	// shell blocks, inline code spans, list items and quoted compose entries.
	// Only the KEY=value form is read; "KEY: value" is too often prose. A
	// <placeholder> is captured whole, spaces included.
	docInlineAssignRE = regexp.MustCompile(`(?:^|[^A-Za-z0-9_$])([A-Z][A-Z0-9_]*)=("[^"]*"|'[^']*'|<[^>]*>|[^\s"'\\;)|]+)`)
)

// scanDocSecretLiterals extracts every literal assigned to an always-required
// secret slot in tracked Markdown. Code-span backticks are treated as
// whitespace; values built from other variables are skipped.
func scanDocSecretLiterals(t *testing.T, root string) []repoSecretLiteral {
	t.Helper()
	args := append([]string{"-C", root, "ls-files", "-z", "--"}, docSecretPathspecs...)
	out, err := exec.Command("git", args...).Output()
	if err != nil || len(out) == 0 {
		t.Skipf("git ls-files unavailable (%v); the Markdown secret scan needs a git checkout", err)
	}
	var literals []repoSecretLiteral
	for _, rel := range strings.Split(string(out), "\x00") {
		if rel == "" {
			continue
		}
		f, err := os.Open(filepath.Join(root, rel))
		if os.IsNotExist(err) {
			continue
		}
		require.NoError(t, err)
		scanner := bufio.NewScanner(f)
		scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
		lineNo := 0
		for scanner.Scan() {
			lineNo++
			line := strings.ReplaceAll(scanner.Text(), "`", " ")
			for _, m := range docInlineAssignRE.FindAllStringSubmatch(line, -1) {
				if !docSecretKeyRE.MatchString(m[1]) {
					continue
				}
				value, ok := cleanRepoSecretValue(m[2])
				if !ok || value == "" {
					continue
				}
				literals = append(literals, repoSecretLiteral{file: rel, line: lineNo, key: m[1], value: value})
			}
		}
		require.NoError(t, scanner.Err())
		_ = f.Close()
	}
	return literals
}

func secretDigest(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

// TestKnownUnsafeSecretsCoverRepoLiterals is the public-secret guard: every secret
// value committed to an env example, CI fixture, or compose file is public,
// so production must refuse it. A failure names the file, line, variable, and
// the digest to add to knownPublicSecretDigests — never the value itself.
func TestKnownUnsafeSecretsCoverRepoLiterals(t *testing.T) {
	root := findRepoRoot(t)
	literals := scanRepoSecretLiterals(t, root)
	require.NotEmpty(t, literals, "scanner found no committed secret literals; the parser is probably broken")

	for _, lit := range literals {
		if !IsKnownUnsafeSecret(lit.value) {
			t.Errorf("%s:%d %s holds a committed secret that production would accept; add sha256 %s to knownPublicSecretDigests",
				lit.file, lit.line, lit.key, secretDigest(lit.value))
		}
	}
}

// TestKnownUnsafeSecretsCoverDocExamples extends the public-secret guard to Markdown:
// a JWT secret, plugin key, or database/admin password shown in a README,
// runbook or plan is published with the source, so production must refuse it
// even when it is long enough to pass the length checks.
func TestKnownUnsafeSecretsCoverDocExamples(t *testing.T) {
	root := findRepoRoot(t)
	literals := scanDocSecretLiterals(t, root)
	for _, lit := range literals {
		if !IsKnownUnsafeSecret(lit.value) {
			t.Errorf("%s:%d %s publishes a secret that production would accept; add sha256 %s to knownPublicSecretDigests",
				lit.file, lit.line, lit.key, secretDigest(lit.value))
		}
	}
}

// TestDocSecretScanParsesMarkdownForms pins the Markdown parser on the shapes
// docs use, so a parser regression cannot make the doc guard vacuous.
func TestDocSecretScanParsesMarkdownForms(t *testing.T) {
	cases := map[string]string{
		"JWT_SECRET_KEY=alpha-value make quick-test":                      "alpha-value",
		"Run: `cd backend && JWT_SECRET_KEY=bravo-value make quick-test`": "bravo-value",
		"export JWT_SECRET_KEY=charlie-value   # required by the bench":   "charlie-value",
		`- "DB_PASSWORD=delta-value"`:                                     "delta-value",
		"`LOCAL_ADMIN_PASSWORD=echo-value`":                               "echo-value",
		"| PLUGIN_SECRET_KEY=foxtrot-value | dev only |":                  "foxtrot-value",
		"- `JWT_SECRET_KEY=<new secret>`;":                                "<new secret>",
	}
	for line, want := range cases {
		var got []string
		for _, m := range docInlineAssignRE.FindAllStringSubmatch(strings.ReplaceAll(line, "`", " "), -1) {
			if !docSecretKeyRE.MatchString(m[1]) {
				continue
			}
			if value, ok := cleanRepoSecretValue(m[2]); ok {
				got = append(got, value)
			}
		}
		assert.Equal(t, []string{want}, got, line)
	}

	for _, line := range []string{
		"JWT_SECRET_KEY=$(openssl rand -hex 32)",
		"DB_PASSWORD=${DB_PASSWORD}",
		"AUTH_TOKEN_MISSING=401",
		"$JWT_SECRET_KEY=x is not an assignment",
		// sed substitution in .claude/skills/setup-and-deploy/SKILL.md
		`  -e "s|^DB_PASSWORD=.*|DB_PASSWORD=$(openssl rand -hex 24)|" \`,
		// shell case pattern in deploy/install.sh
		"\t\t\tADMIN_PASSWORD=)",
	} {
		for _, m := range docInlineAssignRE.FindAllStringSubmatch(line, -1) {
			if value, ok := cleanRepoSecretValue(m[2]); ok && docSecretKeyRE.MatchString(m[1]) {
				t.Errorf("%q yielded literal %q", line, value)
			}
		}
	}
}

// TestRepoSecretScanCoversScriptsAndWorkflows pins the scan set to globs:
// the literals that once slipped past a fixed file list (workflow env,
// docker run -e flags, Node test fixtures, skill env examples) stay covered.
func TestRepoSecretScanCoversScriptsAndWorkflows(t *testing.T) {
	root := findRepoRoot(t)
	files := map[string]bool{}
	for _, rel := range listRepoSecretFiles(t, root) {
		files[rel] = true
		assert.False(t, strings.HasPrefix(rel, "frontend/.env"), "frontend env files are out of scope: %s", rel)
	}
	for _, want := range []string{
		".github/e2e/ci.env.fixture",
		".github/workflows/perf-bench.yml",
		"backend/scripts/run-restore-drill.sh",
		"scripts/validate-production-config.test.mjs",
		"docker-compose.yml",
		"scripts/verify-backend.sh",
	} {
		if _, err := os.Stat(filepath.Join(root, want)); err != nil {
			continue
		}
		assert.True(t, files[want], "%s must be scanned for committed secrets", want)
	}

	literals := scanRepoSecretLiterals(t, root)
	found := map[string]bool{}
	for _, lit := range literals {
		found[lit.file+" "+lit.key] = true
	}
	for _, want := range []string{
		".github/workflows/perf-bench.yml JWT_SECRET_KEY",
		"backend/scripts/run-restore-drill.sh JWT_SECRET_KEY",
		"scripts/validate-production-config.test.mjs PLUGIN_SECRET_KEY",
	} {
		file := strings.SplitN(want, " ", 2)[0]
		if !files[file] {
			continue
		}
		assert.True(t, found[want], "parser no longer extracts %s", want)
	}
}

// TestValidateProduction_RejectsEveryRepoLiteralInSecretSlots proves the
// denylist is wired into the fatal preflight for the always-required secrets.
func TestValidateProduction_RejectsEveryRepoLiteralInSecretSlots(t *testing.T) {
	root := findRepoRoot(t)
	literals := scanRepoSecretLiterals(t, root)
	require.NotEmpty(t, literals)

	slots := map[string]func(*ProductionInputs, string){
		"JWT_SECRET_KEY":    func(in *ProductionInputs, v string) { in.JWTSecretKey = v },
		"PLUGIN_SECRET_KEY": func(in *ProductionInputs, v string) { in.PluginSecretKey = v },
		"DB_PASSWORD":       func(in *ProductionInputs, v string) { in.DBPassword = v },
		"ADMIN_PASSWORD":    func(in *ProductionInputs, v string) { in.AdminPassword = v },
	}
	for _, lit := range literals {
		for slot, set := range slots {
			in := validProductionInputs()
			set(&in, lit.value)
			report := ValidateProduction(in)
			if report.OK() {
				t.Errorf("%s:%d %s accepted as %s in production (sha256 %s)", lit.file, lit.line, lit.key, slot, secretDigest(lit.value))
			}
			for _, issue := range report.Issues() {
				if strings.Contains(issue.Message, lit.value) {
					t.Errorf("%s message echoes the %s literal from %s:%d", issue.Code, lit.key, lit.file, lit.line)
				}
			}
		}
	}
}

func TestIsKnownUnsafeSecret(t *testing.T) {
	legacySeed := "payverge-local-plugin-secret-key"
	legacyKey := sha256.Sum256([]byte(legacySeed))

	unsafe := []string{
		"payverge_local_dev_jwt_secret_2026",
		"payverge_local_dev_jwt_secret_2026_min32",
		"PAYVERGE_LOCAL_DEV_anything",
		"payverge_password",
		"Password",
		" postgres ",
		"replace_with_strong_secret",
		"changeme",
		legacySeed,
		string(legacyKey[:]),
		base64.StdEncoding.EncodeToString(legacyKey[:]),
		base64.RawStdEncoding.EncodeToString(legacyKey[:]),
		"pw",
		"<password>",
		"<at-least-32-random-bytes>",
		"<new secret>",
	}
	for _, v := range unsafe {
		assert.True(t, IsKnownUnsafeSecret(v), "expected unsafe (sha256 %s)", secretDigest(v))
	}

	safe := []string{
		"",
		"   ",
		"a1b2c3d4e5f6g7h8i9j0k1l2m3n4o5p6q7r8s9t0u1v2w3x4y5z6A7B8C9",
		base64.StdEncoding.EncodeToString([]byte(unitPluginKeyRaw)),
		"unit-db-password-8c4f1d907e2a",
		"<a1b2c3d4e5f6g7h8i9j0",
		"a1b2c3d4e5f6g7h8i9j0>",
	}
	for _, v := range safe {
		assert.False(t, IsKnownUnsafeSecret(v), "expected safe: %q", v)
	}
}

// TestIsKnownUnsafePluginKey checks that a published key is refused in every
// spelling the cipher accepts, not only the one that was committed.
func TestIsKnownUnsafePluginKey(t *testing.T) {
	legacyKey := sha256.Sum256([]byte("payverge-local-plugin-secret-key"))
	for name, material := range map[string][]byte{
		"sequential hex": []byte("0123456789abcdef0123456789abcdef"),
		"hermetic test":  []byte("payverge-hermetic-test-key-00000"),
		"legacy":         legacyKey[:],
	} {
		for form, value := range map[string]string{
			"raw":               string(material),
			"base64":            base64.StdEncoding.EncodeToString(material),
			"unpadded base64":   base64.RawStdEncoding.EncodeToString(material),
			"padded with space": "  " + base64.StdEncoding.EncodeToString(material) + " ",
			"hex":               hex.EncodeToString(material),
			"upper-case hex":    strings.ToUpper(hex.EncodeToString(material)),
		} {
			assert.True(t, IsKnownUnsafePluginKey(value), "%s key as %s must be refused", name, form)
		}
	}

	fresh := base64.StdEncoding.EncodeToString([]byte(unitPluginKeyRaw))
	assert.False(t, IsKnownUnsafePluginKey(fresh))
	assert.False(t, IsKnownUnsafePluginKey(unitPluginKeyRaw))
	assert.False(t, IsKnownUnsafePluginKey(hex.EncodeToString([]byte(unitPluginKeyRaw))))
	assert.False(t, IsKnownUnsafePluginKey(""))

	in := validProductionInputs()
	in.PluginSecretKey = base64.StdEncoding.EncodeToString([]byte("0123456789abcdef0123456789abcdef"))
	assert.Equal(t, []string{"plugin_secret.unsafe"}, ValidateProduction(in).Codes())
}

func TestKnownPublicSecretDigestsAreWellFormed(t *testing.T) {
	for digest, label := range knownPublicSecretDigests {
		decoded, err := hex.DecodeString(digest)
		assert.NoError(t, err, label)
		assert.Len(t, decoded, sha256.Size, label)
		assert.Equal(t, strings.ToLower(digest), digest, label)
		assert.NotEmpty(t, label)
	}
}
