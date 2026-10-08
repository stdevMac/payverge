package workflowcontract

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// Release-supply-chain and self-host hardening contracts (SC-02..06,
// trusted-proxy subnet, Postgres app role, hosted-admin env, alert rules).

const (
	postgresImage = "postgres:18.6-alpine@sha256:77f585114c32fbca283dc835b0596f4e52b51b4c6662d7810b2f4084f60a1873"
	releaseSigner = "https://github.com/stdevMac/payverge/.github/workflows/release.yml@refs/heads/main"
	githubIssuer  = "https://token.actions.githubusercontent.com"
)

// SC-02: the release-please token is an environment secret, so the job that
// reads it must run in the main-only `release` environment.
func TestReleasePleaseRunsInTheReleaseEnvironment(t *testing.T) {
	wf := loadPublicWorkflow(t, "release.yml")
	job := requirePublicJob(t, "release.yml", wf, "release-please")
	if env, _ := job.Environment.(string); env != "release" {
		t.Fatalf("release.yml/release-please must declare environment: release, got %v", job.Environment)
	}
}

// SC-03: the CI verdict is read for the exact commit being released.
func TestReleaseCIVerdictChecksOutTheReleaseCommit(t *testing.T) {
	wf := loadPublicWorkflow(t, "release.yml")
	job := requirePublicJob(t, "release.yml", wf, "ci-verdict")
	checkouts := stepsUsing(job, "actions/checkout")
	if len(checkouts) == 0 {
		t.Fatal("release.yml/ci-verdict has no checkout step")
	}
	for _, step := range checkouts {
		if ref, _ := step.With["ref"].(string); ref != "${{ needs.resolve.outputs.sha }}" {
			t.Errorf("release.yml/ci-verdict checkout ref = %q, want the resolved release sha", ref)
		}
	}
}

// SC-04: SHA256SUMS is signed keyless after staging and before upload, and
// the installer can verify that signature against the same identity.
func TestReleaseSignsChecksumsAndInstallerVerifies(t *testing.T) {
	wf := loadPublicWorkflow(t, "release.yml")
	job := requirePublicJob(t, "release.yml", wf, "assets")
	if job.Permissions["id-token"] != "write" {
		t.Error("release.yml/assets needs id-token: write for the keyless signature")
	}
	stage, sign, upload := -1, -1, -1
	for i, step := range job.Steps {
		switch {
		case strings.Contains(step.Run, "scripts/ci/release-assets.sh"):
			stage = i
		case strings.Contains(step.Run, "cosign sign-blob"):
			sign = i
			requireContainsAll(t, "release.yml sign step", step.Run,
				"--bundle SHA256SUMS.sigstore.json SHA256SUMS", "cosign verify-blob",
				releaseSigner, githubIssuer)
		case strings.Contains(step.Run, "gh release upload"):
			upload = i
		}
	}
	if stage < 0 || sign < 0 || upload < 0 || !(stage < sign && sign < upload) {
		t.Fatalf("release.yml/assets must stage, then sign, then upload (stage=%d sign=%d upload=%d)", stage, sign, upload)
	}
	if len(stepsUsing(job, "sigstore/cosign-installer")) != 1 {
		t.Error("release.yml/assets must install cosign once")
	}
	install := readRepoFile(t, "deploy", "install.sh")
	requireContainsAll(t, "deploy/install.sh", install,
		"cosign verify-blob", "SHA256SUMS.sigstore.json", releaseSigner, githubIssuer,
		"--require-signature", "PAYVERGE_REQUIRE_SIGNATURE")
	harness := readRepoFile(t, "scripts", "ci", "deploy-install_test.sh")
	requireContainsAll(t, "scripts/ci/deploy-install_test.sh", harness,
		"signature bad: nothing started", "require-signature, no cosign: no download")
}

// The bundled backend and frontend trust only the compose edge subnet,
// never a whole private range.
func TestDeployTrustsOnlyTheEdgeSubnet(t *testing.T) {
	compose := readRepoFile(t, "deploy", "docker-compose.yml")
	requireContainsAll(t, "deploy/docker-compose.yml", compose,
		"- subnet: ${EDGE_SUBNET:-172.30.0.0/24}",
		"TRUSTED_PROXIES=${TRUSTED_PROXIES:-${EDGE_SUBNET:-172.30.0.0/24},127.0.0.1}",
		"FRONTEND_TRUSTED_PROXIES: ${FRONTEND_TRUSTED_PROXIES:-${EDGE_SUBNET:-172.30.0.0/24}}")
	for _, wide := range []string{"172.16.0.0/12", "10.0.0.0/8", "192.168.0.0/16"} {
		if strings.Contains(compose, wide) {
			t.Errorf("deploy/docker-compose.yml must not trust the whole %s range", wide)
		}
	}
}

// The app connects as a non-superuser owner; the superuser has no password.
func TestDeployPostgresAppRoleIsNotSuperuser(t *testing.T) {
	compose := readRepoFile(t, "deploy", "docker-compose.yml")
	requireContainsAll(t, "deploy/docker-compose.yml", compose,
		"POSTGRES_USER: postgres",
		"PAYVERGE_DB_USER: ${DB_USER:-payverge}",
		"./postgres/init-app-role.sql:/docker-entrypoint-initdb.d/10-app-role.sql:ro")
	initSQL := readRepoFile(t, "deploy", "postgres", "init-app-role.sql")
	requireContainsAll(t, "deploy/postgres/init-app-role.sql", initSQL,
		"NOSUPERUSER", "NOCREATEROLE", "NOBYPASSRLS", "ALTER DATABASE", "OWNER TO",
		"PASSWORD NULL", "must not be")
	install := readRepoFile(t, "deploy", "install.sh")
	requireContainsAll(t, "deploy/install.sh", install, "postgres/init-app-role.sql", "monitoring/alerts.yml")
}

// SC-05/06: every Postgres image the repo runs is the deploy digest, and the
// backup image runs unprivileged.
func TestPostgresAndGoImagesArePinnedByDigest(t *testing.T) {
	for _, path := range [][]string{
		{"deploy", "docker-compose.yml"},
		{"docker-compose.yml"},
		{"backend", "scripts", "backup.Dockerfile"},
		{"backend", "scripts", "generate-genesis-schema.sh"},
	} {
		source := readRepoFile(t, path...)
		if !strings.Contains(source, postgresImage) {
			t.Errorf("%s must use %s", filepath.Join(path...), postgresImage)
		}
		for _, line := range strings.Split(source, "\n") {
			trimmed := strings.TrimSpace(line)
			if strings.HasPrefix(trimmed, "#") || strings.HasPrefix(trimmed, "1.") {
				continue
			}
			if regexp.MustCompile(`(image:|FROM|docker run).*\bpostgres:[0-9][^\s"]*`).MatchString(trimmed) &&
				!strings.Contains(trimmed, "@sha256:") && !strings.Contains(trimmed, "GENESIS_POSTGRES_IMAGE") {
				t.Errorf("%s: unpinned postgres image: %s", filepath.Join(path...), trimmed)
			}
		}
	}
	if !regexp.MustCompile(`image: golang:[0-9.]+-bookworm@sha256:[0-9a-f]{64}`).MatchString(readRepoFile(t, "docker-compose.yml")) {
		t.Error("docker-compose.yml golang image must be pinned by digest")
	}
	backup := readRepoFile(t, "backend", "scripts", "backup.Dockerfile")
	if !regexp.MustCompile(`(?m)^USER nobody$`).MatchString(backup) {
		t.Error("backend/scripts/backup.Dockerfile must run as USER nobody")
	}
	dependabot := readRepoFile(t, ".github", "dependabot.yml")
	requireContainsAll(t, ".github/dependabot.yml", dependabot, "      - /backend/scripts\n", "      - /deploy\n      - /\n")
}

// Hosted-only bootstrap admins are not part of any shipped configuration.
func TestHostedAdminEnvIsNotShipped(t *testing.T) {
	banned := []string{"BOOTSTRAP_DEFAULT_ADMINS", "DEFAULT_ADMIN_EMAIL", "DEFAULT_ADMIN_WALLET"}
	files := []string{
		"docker-compose.yml", ".env.example",
		filepath.Join(".github", "e2e", "ci.env.fixture"),
		filepath.Join("backend", "scripts", "run-restore-drill.sh"),
	}
	err := filepath.WalkDir(repoPath("deploy"), func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			rel, _ := filepath.Rel(repoPath(), path)
			files = append(files, rel)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk deploy/: %v", err)
	}
	for _, file := range files {
		source := readRepoFile(t, file)
		for _, name := range banned {
			if strings.Contains(source, name) {
				t.Errorf("%s must not mention %s", file, name)
			}
		}
	}
}

// OBS-007: shipped alert rules only read metrics the backend exports, and CI
// runs their promtool unit tests.
func TestDeployAlertRulesReadExportedMetrics(t *testing.T) {
	rules := readRepoFile(t, "deploy", "monitoring", "alerts.yml")
	exported := readRepoFile(t, "backend", "internal", "metrics", "metrics.go")
	used := regexp.MustCompile(`\bpayverge_[a-z0-9_]+`).FindAllString(rules, -1)
	if len(used) == 0 {
		t.Fatal("deploy/monitoring/alerts.yml reads no payverge_ metric")
	}
	for _, metric := range used {
		if !strings.Contains(exported, `"`+metric+`"`) {
			t.Errorf("deploy/monitoring/alerts.yml reads %s, which backend/internal/metrics does not export", metric)
		}
	}
	ci := readRepoFile(t, ".github", "workflows", "ci.yml")
	requireContainsAll(t, ".github/workflows/ci.yml", ci,
		"check rules /m/alerts.yml", "test rules /m/alerts.test.yml",
		"bash scripts/ci/deploy-postgres-role_test.sh")
	if !regexp.MustCompile(`prom/prometheus:v[0-9.]+@sha256:[0-9a-f]{64}`).MatchString(ci) {
		t.Error("ci.yml must run promtool from a digest-pinned prom/prometheus image")
	}
}
