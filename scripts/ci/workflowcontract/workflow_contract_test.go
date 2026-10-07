package workflowcontract

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// StringOrSlice unmarshals a YAML scalar or sequence into []string.
// GitHub Actions `needs:` accepts either form.
type StringOrSlice []string

func (s *StringOrSlice) UnmarshalYAML(value *yaml.Node) error {
	switch value.Kind {
	case yaml.ScalarNode:
		var single string
		if err := value.Decode(&single); err != nil {
			return err
		}
		*s = []string{single}
		return nil
	case yaml.SequenceNode:
		var many []string
		if err := value.Decode(&many); err != nil {
			return err
		}
		*s = many
		return nil
	case yaml.AliasNode:
		return s.UnmarshalYAML(value.Alias)
	default:
		// Empty / null needs
		*s = nil
		return nil
	}
}

func readRepoFile(t *testing.T, parts ...string) string {
	t.Helper()
	pathParts := append([]string{"..", "..", ".."}, parts...)
	path := filepath.Join(pathParts...)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(data)
}

func TestPWARolloutFlagRemovedFromBuildConfiguration(t *testing.T) {
	paths := [][]string{
		{".env.example"},
		{"frontend", ".env.example"},
		{"frontend", "Dockerfile"},
		{"docker-compose.yml"},
	}
	for _, parts := range paths {
		content := readRepoFile(t, parts...)
		for _, forbidden := range []string{
			"NEXT_PUBLIC_PWA_FLOATING_PROMPT_ENABLED",
			"PWA_FLOATING_PROMPT_ENABLED",
			"pwa-prompt-off",
			"pwa-prompt-on",
			"Staged PWA rollout",
			"weekly-card rollout",
		} {
			if strings.Contains(content, forbidden) {
				t.Errorf("%s still contains retired PWA rollout mechanism %q", filepath.Join(parts...), forbidden)
			}
		}
	}
}

// TestRestoreDrillStaysOffHostWithCandidateAuthSmoke keeps the restore drill
// bound to an ephemeral loopback Postgres plus an authenticated smoke of the
// candidate backend, so it can never touch a live database host.
func TestRestoreDrillStaysOffHostWithCandidateAuthSmoke(t *testing.T) {
	drill := readRepoFile(t, "backend", "scripts", "run-restore-drill.sh")
	for _, required := range []string{
		"ephemeral_loopback_postgres",
		"production_host_contacted\": false",
		"production_database_contacted\": false",
		"docker network create",
		"127.0.0.1:${BACKEND_HOST_PORT}:8080",
		"/api/v1/health/live",
		"/api/v1/health/ready",
		"/api/v1/auth/login",
		"/api/v1/auth/me",
		"response_body_retained\": false",
		"RTO_SECONDS",
		"MAX_RTO_SECONDS",
	} {
		if !strings.Contains(drill, required) {
			t.Errorf("restore drill missing off-host authentication contract %q", required)
		}
	}
}

func TestAdvertisedJourneysAreMandatoryReleaseGates(t *testing.T) {
	wf := loadPublicWorkflow(t, "ci.yml")
	static := runScript(requirePublicJob(t, "ci.yml", wf, "frontend-static"))
	for _, required := range []string{
		"check-advertised-feature-contract.ts --strict-tests",
		"check-advertised-contract-journeys.ts --require-live-coverage",
	} {
		if !strings.Contains(static, required) {
			t.Errorf("ci.yml frontend-static is missing mandatory advertised-journey flag %q", required)
		}
	}

	// The default Playwright config is the release configuration: it must
	// select the release journeys.
	config := readRepoFile(t, "frontend", "playwright.config.ts")
	if !strings.Contains(config, `testMatch: "**/*.spec.ts"`) {
		t.Error("frontend/playwright.config.ts must select every spec, including release/")
	}
}

func TestDeployCaddyShipsCloudflareCIDRs(t *testing.T) {
	cloudflare := readRepoFile(t, "deploy", "Caddyfile.cloudflare")
	if !strings.Contains(cloudflare, "import cloudflare-cidrs.caddy") {
		t.Fatal("deploy/Caddyfile.cloudflare must import the checked-in Cloudflare CIDR snapshot")
	}
	if strings.Contains(readRepoFile(t, "deploy", "Caddyfile"), "cloudflare-cidrs.caddy") {
		t.Fatal("deploy/Caddyfile (EDGE=none) must not trust Cloudflare ranges")
	}

	compose := readRepoFile(t, "deploy", "docker-compose.yml")
	for _, mount := range []string{
		"./Caddyfile:/etc/caddy/Caddyfile:ro",
		"./Caddyfile.cloudflare:/etc/caddy/Caddyfile.cloudflare:ro",
		"./payverge.caddy:/etc/caddy/payverge.caddy:ro",
		"./cloudflare-cidrs.caddy:/etc/caddy/cloudflare-cidrs.caddy:ro",
	} {
		if !strings.Contains(compose, mount) {
			t.Errorf("deploy/docker-compose.yml must mount %s into the stock Caddy image", mount)
		}
	}
}

func TestCaddyTrustAndClientIPContract(t *testing.T) {
	cidrs := readRepoFile(t, "deploy", "cloudflare-cidrs.caddy")
	if !strings.Contains(cidrs, "trusted_proxies static") {
		t.Fatal("Caddy must derive client_ip only from the checked-in Cloudflare CIDR allowlist")
	}
	if !strings.Contains(cidrs, "client_ip_headers CF-Connecting-IP") {
		t.Fatal("Caddy must prefer Cloudflare's authenticated client IP header")
	}

	site := readRepoFile(t, "deploy", "payverge.caddy")
	clientIP := caddySnippet(t, site, "payverge_client_ip")
	if !strings.Contains(clientIP, "header_up X-Forwarded-For {client_ip}") {
		t.Fatal("the payverge_client_ip snippet must replace X-Forwarded-For with the verified {client_ip}")
	}
	// Every other client-identity header is removed, so neither app can read
	// a client-chosen address (Gin also reads X-Real-IP).
	for _, header := range []string{"X-Real-IP", "CF-Connecting-IP", "True-Client-IP", "X-Client-IP", "Forwarded"} {
		if !strings.Contains(clientIP, "header_up -"+header+"\n") {
			t.Errorf("payverge_client_ip must remove the client-supplied %s header before proxying", header)
		}
	}
	proxies := regexp.MustCompile(`(?m)^\s*reverse_proxy \S+ \{$`).FindAllStringIndex(site, -1)
	if got := strings.Count(site, "import payverge_client_ip"); got < 2 || got != len(proxies) {
		t.Fatalf("every reverse_proxy (backend and frontend) must import payverge_client_ip; %d imports for %d proxies", got, len(proxies))
	}
	for _, spoofable := range []string{"{http.request.header.CF-Connecting-IP}", "{http.request.header.X-Forwarded-For}", "{header.X-Forwarded-For}"} {
		if strings.Contains(site, spoofable) {
			t.Fatalf("payverge.caddy must never key on the spoofable request header %s", spoofable)
		}
	}
}

// caddySnippet returns the body of the Caddyfile snippet "(name) { ... }".
func caddySnippet(t *testing.T, caddyfile, name string) string {
	t.Helper()
	start := strings.Index(caddyfile, "("+name+") {")
	if start < 0 {
		t.Fatalf("snippet (%s) not found", name)
	}
	end := strings.Index(caddyfile[start:], "\n}\n")
	if end < 0 {
		t.Fatalf("snippet (%s) is not closed", name)
	}
	return caddyfile[start : start+end+1]
}

// A load balancer in front of Caddy appends to X-Forwarded-For. Without
// trusted_proxies_strict Caddy takes the LEFT-most entry, which the client
// wrote, so trusting a proxy must always come with strict mode.
func TestDeployCaddyTrustedProxiesAreStrict(t *testing.T) {
	plain := readRepoFile(t, "deploy", "Caddyfile")
	if !strings.Contains(plain, "{$CADDY_TRUSTED_PROXIES_OPTION}") || !strings.Contains(plain, "{$CADDY_TRUSTED_PROXIES_STRICT_OPTION}") {
		t.Fatal("deploy/Caddyfile must emit both the trusted_proxies and the trusted_proxies_strict placeholders")
	}
	compose := readRepoFile(t, "deploy", "docker-compose.yml")
	for _, line := range []string{
		"CADDY_TRUSTED_PROXIES_OPTION: ${TRUSTED_PROXY_CIDRS:+trusted_proxies static ${TRUSTED_PROXY_CIDRS:-}}",
		"CADDY_TRUSTED_PROXIES_STRICT_OPTION: ${TRUSTED_PROXY_CIDRS:+trusted_proxies_strict}",
	} {
		if !strings.Contains(compose, line) {
			t.Errorf("deploy/docker-compose.yml must set %q so TRUSTED_PROXY_CIDRS always turns on strict mode", line)
		}
	}
	cloudflare := readRepoFile(t, "deploy", "Caddyfile.cloudflare")
	if !strings.Contains(cloudflare, "import cloudflare-cidrs.caddy\n\t\ttrusted_proxies_strict\n") {
		t.Fatal("deploy/Caddyfile.cloudflare must enable trusted_proxies_strict next to the Cloudflare ranges (X-Forwarded-For fallback)")
	}
}

// The edge owns Strict-Transport-Security: it replaces the apps' value and
// never forces includeSubDomains on a domain whose other hosts it cannot see.
func TestDeployCaddyHSTSIsOperatorControlled(t *testing.T) {
	headers := caddySnippet(t, readRepoFile(t, "deploy", "payverge.caddy"), "payverge_security_headers")
	if !strings.Contains(headers, "\tStrict-Transport-Security \"{$CADDY_HSTS:max-age=31536000}\"\n") {
		t.Fatal("payverge.caddy must SET (not ?default) Strict-Transport-Security from CADDY_HSTS, default max-age=31536000")
	}
	if strings.Contains(headers, "includeSubDomains") {
		t.Fatal("the edge must not force includeSubDomains; operators opt in with HSTS_POLICY")
	}
	if !strings.Contains(readRepoFile(t, "deploy", "docker-compose.yml"), "CADDY_HSTS: ${HSTS_POLICY:-max-age=31536000}") {
		t.Fatal("deploy/docker-compose.yml must pass HSTS_POLICY to Caddy as CADDY_HSTS")
	}
	if !strings.Contains(readRepoFile(t, "deploy", ".env.example"), "# HSTS_POLICY=max-age=31536000") {
		t.Fatal("deploy/.env.example must document HSTS_POLICY")
	}
}

func TestDeployCaddyEdgeHarnessCoversAntiSpoofing(t *testing.T) {
	script := readRepoFile(t, "scripts", "ci", "deploy-caddy-edge_test.sh")
	for _, required := range []string{"CF-Connecting-IP", "X-Forwarded-For", "X-Real-IP", "cf=[] real=[] fwd=[]", "trusted_proxies_strict", "198.51.100.20", "strict-transport-security: max-age=31536000", "413", "CADDY_IMAGE", "Caddyfile.cloudflare", "caddy validate"} {
		if !strings.Contains(script, required) {
			t.Errorf("deploy Caddy edge test missing %q", required)
		}
	}
}

// Third-party images in the self-host stack are pinned to a digest, so an
// install cannot silently pick up a re-pushed tag. Payverge's own images follow
// PAYVERGE_VERSION. MINIO_IMAGE stays an operator choice, but its default must
// be pinned too (Chainguard's free MinIO image is published as :latest only,
// so the default is latest@sha256:<digest>).
func TestDeployThirdPartyImagesArePinnedByDigest(t *testing.T) {
	compose := readRepoFile(t, "deploy", "docker-compose.yml")
	images := regexp.MustCompile(`(?m)^\s+image:\s*(\S+)\s*$`).FindAllStringSubmatch(compose, -1)
	if len(images) == 0 {
		t.Fatal("no image: lines found in deploy/docker-compose.yml")
	}
	pinned := regexp.MustCompile(`^[a-z0-9./_-]+:[A-Za-z0-9._-]+@sha256:[0-9a-f]{64}$`)
	postgres := map[string]bool{}
	for _, match := range images {
		image := match[1]
		switch {
		case strings.Contains(image, "${PAYVERGE_VERSION"):
			continue
		case strings.HasPrefix(image, "${MINIO_IMAGE"):
			def := strings.TrimSuffix(strings.TrimPrefix(image, "${MINIO_IMAGE:-"), "}")
			if def == image || !pinned.MatchString(def) {
				t.Errorf("deploy/docker-compose.yml MINIO_IMAGE default in %q must be pinned as name:tag@sha256:<digest>", image)
			}
			continue
		case !pinned.MatchString(image):
			t.Errorf("deploy/docker-compose.yml image %q must be pinned as name:tag@sha256:<digest>", image)
		}
		if strings.HasPrefix(image, "postgres:") {
			postgres[image] = true
		}
	}
	// db, backup and restore must run the same PostgreSQL build: pg_dump and
	// pg_restore refuse a newer server or archive format.
	if len(postgres) != 1 {
		t.Errorf("db, backup and restore must use one identical postgres image, found %d: %v", len(postgres), postgres)
	}
}

// Every directory Dependabot watches must exist, and the self-host compose
// file (the third-party image pins) must be watched.
func TestDependabotWatchesExistingDirectoriesAndDeployCompose(t *testing.T) {
	var config struct {
		Updates []struct {
			Ecosystem   string   `yaml:"package-ecosystem"`
			Directory   string   `yaml:"directory"`
			Directories []string `yaml:"directories"`
		} `yaml:"updates"`
	}
	if err := yaml.Unmarshal([]byte(readRepoFile(t, ".github", "dependabot.yml")), &config); err != nil {
		t.Fatalf("parse .github/dependabot.yml: %v", err)
	}
	deployCompose := false
	for _, update := range config.Updates {
		dirs := update.Directories
		if update.Directory != "" {
			dirs = append(dirs, update.Directory)
		}
		for _, dir := range dirs {
			path := filepath.Join("..", "..", "..", strings.TrimPrefix(dir, "/"))
			if info, err := os.Stat(path); err != nil || !info.IsDir() {
				t.Errorf("dependabot %s entry watches %s, which does not exist", update.Ecosystem, dir)
			}
			if update.Ecosystem == "docker-compose" && strings.TrimSuffix(dir, "/") == "/deploy" {
				deployCompose = true
			}
		}
	}
	if !deployCompose {
		t.Error("dependabot must watch /deploy with the docker-compose ecosystem so the pinned self-host images get updates")
	}
}

// deploy/README.md ships into every install directory, so its links to the
// self-hosting guides are absolute. Each guide arrives on its own branch; once
// docs/self-hosting exists in this tree, every guide the README names must be
// in it, so a release never ships a README with dead links.
func TestDeployReadmeSelfHostingLinksResolve(t *testing.T) {
	dir := filepath.Join("..", "..", "..", "docs", "self-hosting")
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		t.Skip("docs/self-hosting is not in this tree yet; the guides land with their own branches")
	}
	readme := readRepoFile(t, "deploy", "README.md")
	links := regexp.MustCompile(`https://github\.com/stdevMac/payverge/blob/main/docs/self-hosting/([A-Za-z0-9._-]+\.md)`).FindAllStringSubmatch(readme, -1)
	if len(links) == 0 {
		t.Fatal("deploy/README.md links no self-hosting guide")
	}
	for _, link := range links {
		if _, err := os.Stat(filepath.Join(dir, link[1])); err != nil {
			t.Errorf("deploy/README.md links docs/self-hosting/%s, which is not in this tree; merge the branch that adds it before releasing", link[1])
		}
	}
}

// install.sh must never stop silently, must read every outcome the backend
// logs for the bootstrap admin, and must back up before an upgrade changes
// anything. The hermetic harness covers each of those.
func TestDeployInstallerContract(t *testing.T) {
	install := readRepoFile(t, "deploy", "install.sh")
	for _, required := range []string{
		`trap 'on_error $? $LINENO "$BASH_COMMAND"' ERR`,
		`Bootstrap admin (NOT (created|applied)|for .* failed|skipped|.* created \(user id|.* took it over)|ADMIN_PASSWORD ignored`,
		"docker compose up -d failed (exit",
		"compose run --rm -T backup once",
		"--no-backup",
		"BACKUP_UID",
	} {
		if !strings.Contains(install, required) {
			t.Errorf("deploy/install.sh must contain %q", required)
		}
	}
	mainBody := install[strings.Index(install, "\nmain() {"):]
	backup := strings.Index(mainBody, "\tpre_upgrade_backup\n")
	fetch := strings.Index(mainBody, "fetch_release \"$version\"")
	if backup < 0 || fetch < 0 || backup > fetch {
		t.Error("install.sh main must take the pre-upgrade backup before it fetches or copies new release files")
	}

	harness := readRepoFile(t, "scripts", "ci", "deploy-install_test.sh")
	for _, required := range []string{
		"STUB_UP_STATUS", "docker compose up -d failed (exit 1)", "does not resolve yet",
		"Bootstrap admin NOT created:", "Bootstrap admin NOT applied:", "took it over", "ADMIN_PASSWORD ignored",
		"backup-version=", "STUB_BACKUP_STATUS", "--no-backup", "BACKUP_UID",
	} {
		if !strings.Contains(harness, required) {
			t.Errorf("deploy install test missing %q", required)
		}
	}
}

// Backups must be readable by the installing user, survive files changing
// under tar, and say when a volume is missing from a set.
func TestDeployBackupContract(t *testing.T) {
	compose := readRepoFile(t, "deploy", "docker-compose.yml")
	for _, required := range []string{"BACKUP_UID: ${BACKUP_UID:-}", "BACKUP_GID: ${BACKUP_GID:-}", "BACKUP_ARCHIVE_ATTEMPTS: ${BACKUP_ARCHIVE_ATTEMPTS:-3}", "      - CHOWN\n",
		"BACKUP_MAX_AGE_HOURS: ${BACKUP_MAX_AGE_HOURS:-26}", `test: ["CMD", "/bin/sh", "/scripts/backup.sh", "health"]`} {
		if !strings.Contains(compose, required) {
			t.Errorf("deploy/docker-compose.yml backup service must contain %q", required)
		}
	}
	backup := readRepoFile(t, "deploy", "backup", "backup.sh")
	for _, required := range []string{"archives_failed=", "--exclude=.minio.sys/tmp", "--exclude=.minio.sys/multipart", "chown -R \"$BACKUP_UID:$BACKUP_GID\"",
		"STATUS_FILE=$BACKUP_ROOT/.status", "trap record_outcome EXIT", "check_health"} {
		if !strings.Contains(backup, required) {
			t.Errorf("deploy/backup/backup.sh must contain %q", required)
		}
	}
	if !strings.Contains(readRepoFile(t, "deploy", "backup", "restore.sh"), "archives_failed") {
		t.Error("deploy/backup/restore.sh must warn when the set it restores is missing an archive")
	}
	harness := readRepoFile(t, "scripts", "ci", "deploy-backup_test.sh")
	for _, required := range []string{
		"initdb", "pg_restore", "restore: database is back", "restore: uploads are back", "other connection(s)",
		"checksum mismatch", "trying again (1/3)", "archives_failed=minio", "this set was taken without: minio",
		"minio.sys/(tmp|multipart)", "BACKUP_UID must be a numeric user id",
		"status: a failed run records failed", "health: ok older than 26h fails", "health: no status fails",
	} {
		if !strings.Contains(harness, required) {
			t.Errorf("deploy backup test missing %q", required)
		}
	}
}

func TestReleaseSmokeUsesRealProtectedRoutesAndPublicBillToken(t *testing.T) {
	spec := readRepoFile(t, "frontend", "tests", "release", "critical-release-smoke.spec.ts")

	protectedWithoutInside := regexp.MustCompile("`\\$\\{API_BASE\\}/businesses/")
	if protectedWithoutInside.MatchString(spec) {
		t.Fatal("release smoke calls a protected /businesses route without /inside")
	}
	if !strings.Contains(spec, "`${API_BASE}/inside/businesses") {
		t.Fatal("release smoke must exercise real /api/v1/inside/businesses routes")
	}
	if !strings.Contains(spec, ".bill?.public_token") || !strings.Contains(spec, "guest/bill/${publicToken}") {
		t.Fatal("release smoke must resolve guest bills with the opaque public_token")
	}
	if strings.Contains(spec, "guest/bill/${billNumber}") {
		t.Fatal("release smoke must not use bill_number as a public guest capability")
	}
	if !strings.Contains(spec, "X-Request-Id") {
		t.Fatal("release smoke must exercise an idempotent checkout/session seam")
	}
	if !strings.Contains(spec, "ignoreHTTPSErrors: true") {
		t.Fatal("release smoke API context must trust the ephemeral self-signed Caddy certificate")
	}
	if !strings.Contains(spec, "`${API_BASE}/inside/businesses/${businessNumericId}`") {
		t.Fatal("release smoke cleanup must use the real protected business route")
	}
}

func TestPerfBenchMakeTargetUsesBashForPipefail(t *testing.T) {
	makefile := readRepoFile(t, "backend", "Makefile")
	if !strings.Contains(makefile, "bash -o pipefail -c") {
		t.Fatal("bench-ci must run the piped benchmark command under bash; /bin/sh on GitHub rejects `set -o pipefail`")
	}
	if strings.Contains(makefile, "\n\t@set -o pipefail; go test") {
		t.Fatal("bench-ci must not invoke `set -o pipefail` through make's default /bin/sh")
	}
}

func TestPerfBenchMakeTargetIsBoundedForCI(t *testing.T) {
	makefile := readRepoFile(t, "backend", "Makefile")
	if !strings.Contains(makefile, "-benchtime=100ms") {
		t.Fatal("bench-ci must set a short explicit benchtime so the GitHub job stays under its timeout as benchmarks accumulate")
	}
}

func TestDemoSeedMatchesCurrentReservationSettingsSchema(t *testing.T) {
	seed := readRepoFile(t, "backend", "scripts", "demo_seed.sql")
	for _, retiredColumn := range []string{
		"require_confirmation",
		"deposit_enabled",
		"deposit_amount",
		"deposit_party_size",
	} {
		if strings.Contains(seed, retiredColumn) {
			t.Fatalf("demo seed references retired reservation_settings column %q", retiredColumn)
		}
	}
	if !strings.Contains(seed, "approval_mode") {
		t.Fatal("demo seed must populate reservation_settings.approval_mode")
	}

	counterBillStart := strings.Index(seed, "IF (v_business_key = 'core' AND v_i > 62)")
	if counterBillStart == -1 {
		t.Fatal("demo seed counter-bill block not found")
	}
	counterBillEnd := strings.Index(seed[counterBillStart:], "END IF;")
	if counterBillEnd == -1 {
		t.Fatal("demo seed counter-bill block is unterminated")
	}
	counterBillBlock := seed[counterBillStart : counterBillStart+counterBillEnd]
	if !strings.Contains(counterBillBlock, "v_table_id := NULL;") {
		t.Fatal("counter bills must clear table_id to preserve the one-active-bill-per-table invariant")
	}

	// Migration 000001 dropped the SaaS billing tables; the seed must not touch them.
	for _, droppedTable := range []string{"subscription_payments", "stripe_subscription", "subscription_plan", "stripe_price_"} {
		if strings.Contains(seed, droppedTable) {
			t.Fatalf("demo seed references dropped SaaS billing schema %q", droppedTable)
		}
	}
}

func TestRequiredSecurityWorkflowScansRepositoryHistoryForSecrets(t *testing.T) {
	wf := loadPublicWorkflow(t, "ci.yml")
	audit := requirePublicJob(t, "ci.yml", wf, "audit")
	checkout := stepsUsing(audit, "actions/checkout")
	if len(checkout) != 1 || fmt.Sprint(checkout[0].With["fetch-depth"]) != "0" {
		t.Error("ci.yml audit must check out the full history (fetch-depth: 0) so both secret scanners walk every commit")
	}
	run := runScript(audit)
	for _, required := range []string{
		"node --test scripts/scan-secrets.test.mjs",
		"node scripts/scan-secrets.mjs --history",
		"--config /repo/.github/gitleaks.toml --redact",
		"go run golang.org/x/vuln/cmd/govulncheck@",
		"node scripts/check-npm-audit.js --level high",
	} {
		if !strings.Contains(run, required) {
			t.Errorf("ci.yml audit is missing %q", required)
		}
	}
	// Every scanner reports even when an earlier one failed.
	for _, step := range audit.Steps[1:] {
		if step.Run != "" && step.Name != "Secret scanner self-test and history scan" && step.If != "${{ !cancelled() }}" {
			t.Errorf("ci.yml audit step %q must run with if: ${{ !cancelled() }}", step.Name)
		}
	}
	// The production-dependency audit gate audits --omit=dev itself and only
	// accepts advisories listed (with a review date) in its allowlist.
	auditGate := readRepoFile(t, "frontend", "scripts", "check-npm-audit.js")
	if !strings.Contains(auditGate, `"--omit=dev"`) {
		t.Error("frontend/scripts/check-npm-audit.js must audit production dependencies (--omit=dev)")
	}
	if _, err := os.Stat(repoPath("frontend", "scripts", "npm-audit-allowlist.json")); err != nil {
		t.Errorf("frontend/scripts/npm-audit-allowlist.json: %v", err)
	}
	if _, err := os.Stat(repoPath(".github", "gitleaks.toml")); err != nil {
		t.Errorf(".github/gitleaks.toml: %v", err)
	}

	hook := readRepoFile(t, "scripts", "hooks", "pre-commit")
	if !strings.Contains(hook, "scripts/scan-secrets.mjs") {
		t.Error("pre-commit hook is missing scripts/scan-secrets.mjs")
	}

	ignore := readRepoFile(t, ".gitignore")
	for _, required := range []string{".env.bak*", "*.dump", "*.sql.gz", "credential-exports/"} {
		if !strings.Contains(ignore, required) {
			t.Errorf("secret/artifact ignore policy is missing %q", required)
		}
	}
}

// An allowlist entry that names a file which no longer exists silently stops
// covering the renamed file, and the public gitleaks job goes red. Every
// fully anchored literal path in .github/gitleaks.toml must exist.
func TestGitleaksAllowlistPathsExist(t *testing.T) {
	config := readRepoFile(t, ".github", "gitleaks.toml")
	entry := regexp.MustCompile(`'''\^((?:[^'\\]|\\.)*)\$'''`)
	meta := regexp.MustCompile(`[\[\](){}*+?|^$]`)
	checked := 0
	for _, m := range entry.FindAllStringSubmatch(config, -1) {
		pattern := m[1]
		if meta.MatchString(strings.ReplaceAll(pattern, `\.`, "")) {
			continue // a real pattern, not a literal file path
		}
		path := strings.ReplaceAll(pattern, `\.`, ".")
		if strings.Contains(path, `\`) {
			continue
		}
		checked++
		if _, err := os.Stat(repoPath(strings.Split(path, "/")...)); err != nil {
			t.Errorf(".github/gitleaks.toml allowlists %q, which does not exist: %v", path, err)
		}
	}
	if checked < 4 {
		t.Fatalf("expected at least 4 literal allowlist paths in .github/gitleaks.toml, found %d", checked)
	}
}
