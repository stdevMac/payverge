package workflowcontract

// Contracts for the public GitHub Actions workflow set: which workflows exist,
// how they are hardened, and what each release lane must keep doing.
//
// scripts/ci/d10_hygiene_contract.test.mjs repeats the cheapest of these
// checks (workflow set, SHA pins) without a YAML parser, so the repository
// contract job reports them even if this Go module fails to build.

import (
	"encoding/json"
	"fmt"
	"net/netip"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// publicWorkflows is the complete set of files under .github/workflows/.
var publicWorkflows = []string{"acceptance.yml", "ci.yml", "codeql.yml", "e2e.yml", "release.yml", "scorecard.yml"}

// writePermissionAllowList is every job that may hold a write scope, and the
// scopes it may hold. Top-level permissions are always contents: read.
var writePermissionAllowList = map[string][]string{
	"codeql.yml/analyze":         {"security-events"},
	"scorecard.yml/analysis":     {"id-token", "security-events"},
	"release.yml/release-please": {"contents", "issues", "pull-requests"},
	"release.yml/build":          {"packages"},
	"release.yml/publish":        {"id-token", "packages"},
	"release.yml/assets":         {"contents", "id-token"},
}

// timeoutCeiling raises the 60-minute job ceiling for the few jobs that wait
// on another workflow rather than doing work of their own.
var timeoutCeiling = map[string]int{
	// Polls ci.yml on the release commit; CI starts with the release run and
	// its slowest lane has a 60-minute timeout of its own after queueing.
	"release.yml/ci-verdict": 120,
}

// hostedRunners are the only runner labels a job may request: GitHub-hosted,
// so no workflow depends on a self-hosted machine that fork pull requests
// could reach.
var hostedRunners = []string{"ubuntu-24.04", "ubuntu-24.04-arm"}

type ghStep struct {
	ID               string            `yaml:"id"`
	Name             string            `yaml:"name"`
	If               string            `yaml:"if"`
	Uses             string            `yaml:"uses"`
	Run              string            `yaml:"run"`
	WorkingDirectory string            `yaml:"working-directory"`
	With             map[string]any    `yaml:"with"`
	Env              map[string]string `yaml:"env"`
}

type ghService struct {
	Image string `yaml:"image"`
}

type ghStrategy struct {
	FailFast *bool          `yaml:"fail-fast"`
	Matrix   map[string]any `yaml:"matrix"`
}

type ghJob struct {
	Name           string               `yaml:"name"`
	If             string               `yaml:"if"`
	Uses           string               `yaml:"uses"`
	Needs          StringOrSlice        `yaml:"needs"`
	RunsOn         string               `yaml:"runs-on"`
	TimeoutMinutes int                  `yaml:"timeout-minutes"`
	Permissions    map[string]string    `yaml:"permissions"`
	Strategy       ghStrategy           `yaml:"strategy"`
	Services       map[string]ghService `yaml:"services"`
	Env            map[string]string    `yaml:"env"`
	Outputs        map[string]string    `yaml:"outputs"`
	Steps          []ghStep             `yaml:"steps"`
	Environment    any                  `yaml:"environment"`
}

type ghWorkflow struct {
	Name        string            `yaml:"name"`
	On          map[string]any    `yaml:"on"`
	Permissions map[string]string `yaml:"permissions"`
	Concurrency map[string]any    `yaml:"concurrency"`
	Env         map[string]string `yaml:"env"`
	Defaults    map[string]any    `yaml:"defaults"`
	Jobs        map[string]ghJob  `yaml:"jobs"`
}

func repoPath(parts ...string) string {
	return filepath.Join(append([]string{"..", "..", ".."}, parts...)...)
}

func loadPublicWorkflow(t *testing.T, name string) ghWorkflow {
	t.Helper()
	var wf ghWorkflow
	if err := yaml.Unmarshal([]byte(readRepoFile(t, ".github", "workflows", name)), &wf); err != nil {
		t.Fatalf("parse .github/workflows/%s: %v", name, err)
	}
	if len(wf.Jobs) == 0 {
		t.Fatalf(".github/workflows/%s has no jobs", name)
	}
	return wf
}

func requirePublicJob(t *testing.T, workflow string, wf ghWorkflow, name string) ghJob {
	t.Helper()
	job, ok := wf.Jobs[name]
	if !ok {
		t.Fatalf("%s is missing job %q", workflow, name)
	}
	return job
}

// runScript joins every run: block of a job.
func runScript(job ghJob) string {
	var parts []string
	for _, step := range job.Steps {
		if step.Run != "" {
			parts = append(parts, step.Run)
		}
	}
	return strings.Join(parts, "\n")
}

func stepsUsing(job ghJob, action string) []ghStep {
	var out []ghStep
	for _, step := range job.Steps {
		if strings.HasPrefix(step.Uses, action+"@") {
			out = append(out, step)
		}
	}
	return out
}

func requireContainsAll(t *testing.T, where, source string, required ...string) {
	t.Helper()
	for _, want := range required {
		if !strings.Contains(source, want) {
			t.Errorf("%s is missing %q", where, want)
		}
	}
}

func matrixValues(t *testing.T, job ghJob, key string) []string {
	t.Helper()
	raw, ok := job.Strategy.Matrix[key].([]any)
	if !ok {
		t.Fatalf("strategy.matrix.%s is not a list", key)
	}
	out := make([]string, 0, len(raw))
	for _, v := range raw {
		out = append(out, fmt.Sprint(v))
	}
	return out
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func TestPublicWorkflowSetIsExact(t *testing.T) {
	entries, err := os.ReadDir(repoPath(".github", "workflows"))
	if err != nil {
		t.Fatalf("read .github/workflows: %v", err)
	}
	var got []string
	for _, entry := range entries {
		got = append(got, entry.Name())
	}
	sort.Strings(got)
	if !slices.Equal(got, publicWorkflows) {
		t.Fatalf(".github/workflows = %v, want exactly %v (update publicWorkflows and the hygiene contract deliberately)", got, publicWorkflows)
	}
}

// TestPublicWorkflowsAreHardened enforces the rules listed at the top of
// ci.yml on every workflow.
func TestPublicWorkflowsAreHardened(t *testing.T) {
	pinned := regexp.MustCompile(`^[A-Za-z0-9_.-]+/[A-Za-z0-9_./-]+@[0-9a-f]{40}$`)
	usesLine := regexp.MustCompile(`^\s*(?:-\s+)?uses:\s*(\S+)(.*)$`)
	versionComment := regexp.MustCompile(`^\s+#\s*v\d+\.\d+\.\d+\s*$`)
	writeLine := regexp.MustCompile(`^\s+[a-z-]+:\s*write\b(.*)$`)
	secretRef := regexp.MustCompile(`(?:^|[^A-Za-z0-9_-])secrets\.([A-Za-z0-9_]+)`)

	for _, name := range publicWorkflows {
		source := readRepoFile(t, ".github", "workflows", name)
		wf := loadPublicWorkflow(t, name)

		// Triggers: nothing that runs fork code with a privileged token.
		for _, trigger := range []string{"pull_request_target", "workflow_run"} {
			if _, ok := wf.On[trigger]; ok {
				t.Errorf("%s: trigger %s is forbidden", name, trigger)
			}
		}

		// Read-only default token; writes are granted per job below.
		if len(wf.Permissions) != 1 || wf.Permissions["contents"] != "read" {
			t.Errorf("%s: top-level permissions = %v, want exactly {contents: read}", name, wf.Permissions)
		}
		if group, _ := wf.Concurrency["group"].(string); group == "" {
			t.Errorf("%s: needs a top-level concurrency group", name)
		}

		for _, jobName := range sortedKeys(wf.Jobs) {
			job := wf.Jobs[jobName]
			where := name + "/" + jobName

			ceiling := 60
			if c, ok := timeoutCeiling[where]; ok {
				ceiling = c
			}
			if job.TimeoutMinutes <= 0 || job.TimeoutMinutes > ceiling {
				t.Errorf("%s: timeout-minutes = %d, want 1..%d", where, job.TimeoutMinutes, ceiling)
			}

			switch {
			case slices.Contains(hostedRunners, job.RunsOn):
			case job.RunsOn == "${{ matrix.runner }}":
				include, _ := job.Strategy.Matrix["include"].([]any)
				if len(include) == 0 {
					t.Errorf("%s: runs-on uses matrix.runner without a matrix include list", where)
				}
				for _, entry := range include {
					runner, _ := entry.(map[string]any)["runner"].(string)
					if !slices.Contains(hostedRunners, runner) {
						t.Errorf("%s: matrix runner %q is not a GitHub-hosted label %v", where, runner, hostedRunners)
					}
				}
			default:
				t.Errorf("%s: runs-on %q is not a GitHub-hosted label %v", where, job.RunsOn, hostedRunners)
			}

			var writes []string
			for scope, level := range job.Permissions {
				switch level {
				case "read", "none":
				case "write":
					writes = append(writes, scope)
				default:
					t.Errorf("%s: permission %s: %q, want read, write or none", where, scope, level)
				}
			}
			sort.Strings(writes)
			if want := writePermissionAllowList[where]; !slices.Equal(writes, want) {
				t.Errorf("%s: write scopes = %v, want %v (writePermissionAllowList)", where, writes, want)
			}

			if job.Uses != "" && !strings.HasPrefix(job.Uses, "./") && !pinned.MatchString(job.Uses) {
				t.Errorf("%s: reusable workflow %q is not pinned to a commit SHA", where, job.Uses)
			}
			for i, step := range job.Steps {
				stepWhere := fmt.Sprintf("%s step %d (%s)", where, i+1, step.Name+step.Uses)
				if step.Uses != "" && !strings.HasPrefix(step.Uses, "./") && !pinned.MatchString(step.Uses) {
					t.Errorf("%s: action %q is not pinned to a full commit SHA", stepWhere, step.Uses)
				}
				if strings.HasPrefix(step.Uses, "actions/checkout@") {
					if v, ok := step.With["persist-credentials"].(bool); !ok || v {
						t.Errorf("%s: checkout must set persist-credentials: false", stepWhere)
					}
				}
				// Expressions reach scripts through env:, never by template
				// substitution into the shell source.
				if strings.Contains(step.Run, "${{") {
					t.Errorf("%s: run: block interpolates ${{ ... }}; pass the value through env:", stepWhere)
				}
			}
		}

		for i, line := range strings.Split(source, "\n") {
			if m := usesLine.FindStringSubmatch(line); m != nil && !strings.HasPrefix(m[1], "./") {
				if !versionComment.MatchString(m[2]) {
					t.Errorf("%s:%d: pinned action %s needs a trailing `# vX.Y.Z` comment", name, i+1, m[1])
				}
			}
			if m := writeLine.FindStringSubmatch(line); m != nil && !strings.Contains(m[1], "#") {
				t.Errorf("%s:%d: write permission needs an inline `# reason` comment", name, i+1)
			}
			if strings.Contains(line, "payverge.io") {
				t.Errorf("%s:%d: workflows must not reference the private deployment", name, i+1)
			}
			for _, m := range secretRef.FindAllStringSubmatch(line, -1) {
				// The only secret is optional and falls back to github.token.
				if m[1] != "RELEASE_PLEASE_TOKEN" || name != "release.yml" ||
					!strings.Contains(line, "secrets.RELEASE_PLEASE_TOKEN || github.token") {
					t.Errorf("%s:%d: secrets.%s: public workflows must run without repository secrets", name, i+1, m[1])
				}
			}
		}
	}
}

func TestCIRunsOnPullRequestsAndMain(t *testing.T) {
	wf := loadPublicWorkflow(t, "ci.yml")
	if _, ok := wf.On["pull_request"]; !ok {
		t.Error("ci.yml must run on pull_request")
	}
	push, _ := wf.On["push"].(map[string]any)
	branches, _ := push["branches"].([]any)
	if len(branches) != 1 || branches[0] != "main" {
		t.Errorf("ci.yml push.branches = %v, want [main]", branches)
	}
	if _, ok := wf.On["schedule"]; !ok {
		t.Error("ci.yml needs the nightly schedule that runs the race detector")
	}
	cancel, _ := wf.Concurrency["cancel-in-progress"].(string)
	if cancel != "${{ github.event_name == 'pull_request' }}" {
		t.Errorf("ci.yml cancel-in-progress = %q, want superseded pull-request runs cancelled and nothing else", cancel)
	}
	// A concurrency group keeps one running and one pending run; a newer
	// pending run replaces the older one even without cancel-in-progress.
	// Outside pull requests the group must therefore be unique per commit, or
	// a burst of pushes to main leaves commits without a CI verdict.
	group, _ := wf.Concurrency["group"].(string)
	if group != "ci-${{ github.event_name }}-${{ github.event_name == 'pull_request' && github.ref || github.sha }}" {
		t.Errorf("ci.yml concurrency.group = %q, want one group per PR and one per commit otherwise", group)
	}
}

// TestCIOKAggregatesEveryJob keeps ci-ok the single required status check:
// it must depend on every other job and fail unless all of them succeeded.
func TestCIOKAggregatesEveryJob(t *testing.T) {
	wf := loadPublicWorkflow(t, "ci.yml")
	ok := requirePublicJob(t, "ci.yml", wf, "ci-ok")
	if ok.If != "always()" {
		t.Errorf("ci-ok.if = %q, want always() so a failed dependency still reports", ok.If)
	}
	var others []string
	for name := range wf.Jobs {
		if name != "ci-ok" {
			others = append(others, name)
		}
	}
	sort.Strings(others)
	needs := slices.Clone([]string(ok.Needs))
	sort.Strings(needs)
	if !slices.Equal(needs, others) {
		t.Errorf("ci-ok.needs = %v, want every other job %v", needs, others)
	}
	if len(ok.Steps) != 1 || ok.Steps[0].Env["NEEDS"] != "${{ toJSON(needs) }}" {
		t.Fatal("ci-ok must read the dependency results from env NEEDS: ${{ toJSON(needs) }}")
	}
	requireContainsAll(t, "ci-ok", ok.Steps[0].Run, `.value.result != "success"`, "exit 1")

	for _, name := range others {
		if job := wf.Jobs[name]; len(job.Needs) != 0 || job.If != "" {
			t.Errorf("ci.yml %s: needs=%v if=%q; lanes run in parallel and unconditionally, ci-ok aggregates them", name, []string(job.Needs), job.If)
		}
	}
}

func TestCIBackendLaneShardsVetAndShortTests(t *testing.T) {
	wf := loadPublicWorkflow(t, "ci.yml")
	backend := requirePublicJob(t, "ci.yml", wf, "backend")
	if got := matrixValues(t, backend, "shard"); !slices.Equal(got, []string{"1", "2", "3"}) {
		t.Errorf("backend shards = %v, want [1 2 3]", got)
	}
	run := runScript(backend)
	requireContainsAll(t, "ci.yml backend", run,
		"scripts/ci/backend-env.sh go build ./...",
		`bash scripts/ci/go-test-shard.sh "$SHARD" 3`,
		`scripts/ci/backend-env.sh go vet "${pkgs[@]}"`,
		`scripts/ci/backend-env.sh go test -short -count=1 "${race[@]}" "${pkgs[@]}"`,
	)

	var race string
	for _, step := range backend.Steps {
		if v, ok := step.Env["RACE"]; ok {
			race = v
		}
	}
	// -race on every push to main and on the nightly schedule, never on PRs.
	requireContainsAll(t, "backend RACE expression", race,
		"github.event_name == 'schedule'",
		"github.event_name == 'push' && github.ref == 'refs/heads/main'",
	)
	if strings.Contains(race, "pull_request") {
		t.Errorf("backend RACE = %q must not enable the race detector for pull requests", race)
	}
}

// TestCIBackendLaneCompilesEveryTagGatedTest: a test behind a build tag that
// no job runs (provider sandbox tests, ARCA homologation) still has to
// compile, or it rots unnoticed. Every tag used in backend/ must be in the
// shard-1 vet step, except whatsapp, which the whatsapp job builds.
func TestCIBackendLaneCompilesEveryTagGatedTest(t *testing.T) {
	wf := loadPublicWorkflow(t, "ci.yml")
	run := runScript(requirePublicJob(t, "ci.yml", wf, "backend"))
	m := regexp.MustCompile(`scripts/ci/backend-env\.sh go vet -tags ([\w,]+) \./\.\.\.`).FindStringSubmatch(run)
	if m == nil {
		t.Fatal("ci.yml backend must run `scripts/ci/backend-env.sh go vet -tags <tags> ./...`")
	}
	vetted := strings.Split(m[1], ",")

	constraint := regexp.MustCompile(`(?m)^//go:build (.+)$`)
	ident := regexp.MustCompile(`[A-Za-z_][A-Za-z0-9_]*`)
	tags := map[string]bool{}
	err := filepath.WalkDir(repoPath("backend"), func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if d.Name() == "vendor" || d.Name() == "node_modules" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") {
			return nil
		}
		src, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		for _, c := range constraint.FindAllStringSubmatch(string(src), -1) {
			for _, tag := range ident.FindAllString(c[1], -1) {
				tags[tag] = true
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, known := range []string{"integration", "afip_homo"} {
		if !tags[known] {
			t.Fatalf("tag discovery found no %q constraint in backend/; the scan is out of date", known)
		}
	}
	for tag := range tags {
		if tag == "whatsapp" || slices.Contains(vetted, tag) {
			continue
		}
		t.Errorf("backend uses build tag %q but ci.yml never compiles it; add it to the shard-1 go vet -tags list", tag)
	}
}

// TestReusablePostgresJobRunsDurableLLMBudgetTests keeps the Postgres lane on
// a real postgres:18 service and keeps the package discovery from dropping
// the suites that only run against it.
func TestReusablePostgresJobRunsDurableLLMBudgetTests(t *testing.T) {
	wf := loadPublicWorkflow(t, "ci.yml")
	pg := requirePublicJob(t, "ci.yml", wf, "backend-pg")
	if img := pg.Services["postgres"].Image; !regexp.MustCompile(`^postgres:18@sha256:[0-9a-f]{64}$`).MatchString(img) {
		t.Errorf("backend-pg postgres service image = %q, want postgres:18 pinned by digest", img)
	}
	if got := pg.Env["TEST_DATABASE_URL"]; got != "postgres://test:test@localhost:5432/test?sslmode=disable" {
		t.Errorf("backend-pg TEST_DATABASE_URL = %q", got)
	}
	var tags string
	for _, step := range pg.Steps {
		if v, ok := step.Env["GO_TEST_SHARD_TAGS"]; ok {
			tags = v
		}
	}
	if tags != "integration,integration_postgres" {
		t.Errorf("backend-pg GO_TEST_SHARD_TAGS = %q, want integration,integration_postgres", tags)
	}
	requireContainsAll(t, "ci.yml backend-pg", runScript(pg),
		"bash scripts/ci/go-pg-packages.sh",
		`bash scripts/ci/go-test-shard.sh "$SHARD" 2 "${pg[@]}"`,
		`go test -count=1 -p 1 -tags "$GO_TEST_SHARD_TAGS"`,
	)

	out, err := exec.Command("bash", repoPath("scripts", "ci", "go-pg-packages.sh")).Output()
	if err != nil {
		t.Fatalf("scripts/ci/go-pg-packages.sh: %v", err)
	}
	pkgs := strings.Fields(string(out))
	for _, want := range []string{
		"./internal/llm",      // durable budget store: concurrency, restart, midnight rollover
		"./internal/database", // migration rollback rehearsal and demo-seed idempotency
		"./internal/server",   // integration-tagged handler suites
		"./internal/tests",
	} {
		if !slices.Contains(pkgs, want) {
			t.Errorf("go-pg-packages.sh no longer lists %s: %v", want, pkgs)
		}
	}
	for _, live := range []string{"./internal/plugins/paypal", "./internal/plugins/mercadopago"} {
		if slices.Contains(pkgs, live) {
			t.Errorf("go-pg-packages.sh lists %s, whose integration tests call live provider sandboxes", live)
		}
	}
}

func TestCIWhatsAppTagCompilesAndDefaultBuildStaysGPLFree(t *testing.T) {
	wf := loadPublicWorkflow(t, "ci.yml")
	whatsapp := requirePublicJob(t, "ci.yml", wf, "whatsapp")
	requireContainsAll(t, "ci.yml whatsapp", runScript(whatsapp), "go build -tags whatsapp ./...")

	guard := requirePublicJob(t, "ci.yml", wf, "gpl-guard")
	run := runScript(guard)
	requireContainsAll(t, "ci.yml gpl-guard", run, "go list -deps", `go\.mau\.fi`, "go list -deps -tags whatsapp ./cmd/app")

	// The guard must cover every binary the backend image builds.
	dockerfile := readRepoFile(t, "backend", "Dockerfile")
	cmds := regexp.MustCompile(`\./cmd/([a-z0-9-]+)`).FindAllStringSubmatch(dockerfile, -1)
	if len(cmds) == 0 {
		t.Fatal("backend/Dockerfile builds no ./cmd/ package; update this contract")
	}
	for _, m := range cmds {
		if !strings.Contains(run, "./cmd/"+m[1]) {
			t.Errorf("gpl-guard does not check ./cmd/%s, which backend/Dockerfile ships", m[1])
		}
	}
}

// TestReleaseImagesNeverCarryWhatsAppTag: the whatsapp build tag links
// GPL-3.0 code and must never reach a published image.
func TestReleaseImagesNeverCarryWhatsAppTag(t *testing.T) {
	release := readRepoFile(t, ".github", "workflows", "release.yml")
	for _, forbidden := range []string{"-tags whatsapp", "GO_TAGS", "--build-arg"} {
		if strings.Contains(release, forbidden) {
			t.Errorf("release.yml contains %q; published images use the default (untagged) backend build", forbidden)
		}
	}
	wf := loadPublicWorkflow(t, "release.yml")
	build := requirePublicJob(t, "release.yml", wf, "build")
	requireContainsAll(t, "release.yml build", runScript(build),
		"go version -m",
		`go\.mau\.fi/`,
		"-tags=.*whatsapp",
	)
	args := stepsUsing(build, "docker/build-push-action")
	if len(args) != 1 {
		t.Fatalf("release.yml build must have exactly one build-push-action step, found %d", len(args))
	}
	if got := fmt.Sprint(args[0].With["build-args"]); got != "${{ steps.args.outputs.build-args }}" {
		t.Errorf("build-push-action build-args = %q, want only the metadata computed in the args step", got)
	}

	tagged := regexp.MustCompile(`GO_TAGS\s*[:=]\s*["']?[^"'\n]*\bwhatsapp\b`)
	files := []string{}
	for _, name := range publicWorkflows {
		files = append(files, filepath.Join(".github", "workflows", name))
	}
	compose, _ := filepath.Glob(repoPath("docker-compose*.yml"))
	for _, path := range compose {
		files = append(files, filepath.Base(path))
	}
	for _, rel := range files {
		if loc := tagged.FindString(readRepoFile(t, strings.Split(rel, string(filepath.Separator))...)); loc != "" {
			t.Errorf("%s sets a whatsapp build tag: %q", rel, loc)
		}
	}
}

func TestCIFrontendLanes(t *testing.T) {
	wf := loadPublicWorkflow(t, "ci.yml")

	static := requirePublicJob(t, "ci.yml", wf, "frontend-static")
	requireContainsAll(t, "ci.yml frontend-static", runScript(static), "npm run lint", "npm run typecheck")

	unit := requirePublicJob(t, "ci.yml", wf, "frontend-unit")
	if got := matrixValues(t, unit, "shard"); !slices.Equal(got, []string{"1", "2", "3", "4"}) {
		t.Errorf("frontend-unit shards = %v, want [1 2 3 4]", got)
	}
	requireContainsAll(t, "ci.yml frontend-unit", runScript(unit), `--shard="${SHARD}/4"`, "--testTimeout=", "--ci")

	// next build sees exactly the release build arguments: a build that only
	// passes with deployment URLs would ship a release image that fails.
	build := requirePublicJob(t, "ci.yml", wf, "frontend-build")
	var buildStep ghStep
	for _, step := range build.Steps {
		if strings.Contains(step.Run, "npm run build") {
			buildStep = step
		}
	}
	requireContainsAll(t, "ci.yml frontend-build", buildStep.Run,
		`args="$(bash ../scripts/ci/image-build-args.sh frontend 0.0.0-ci)"`,
		`done <<<"$args"`,
		"npm run build",
	)
	if strings.Contains(buildStep.Run, "< <(") {
		t.Error("frontend-build must not read the build arguments through process substitution: a failing script would go unnoticed")
	}
	for key := range buildStep.Env {
		if strings.HasPrefix(key, "NEXT_PUBLIC_") {
			t.Errorf("frontend-build sets %s; next build gets only scripts/ci/image-build-args.sh output, as a release does", key)
		}
	}
	for key := range build.Env {
		if strings.HasPrefix(key, "NEXT_PUBLIC_") {
			t.Errorf("frontend-build job env sets %s", key)
		}
	}
	for key := range wf.Env {
		if strings.HasPrefix(key, "NEXT_PUBLIC_") {
			t.Errorf("ci.yml env sets %s", key)
		}
	}

	// Every job that installs frontend dependencies does so from the lockfile.
	for _, name := range sortedKeys(wf.Jobs) {
		for _, step := range wf.Jobs[name].Steps {
			if strings.Contains(step.Run, "npm install") {
				t.Errorf("ci.yml %s runs npm install; use npm ci", name)
			}
		}
	}
}

// TestCII18nLaneRunsEveryPreCommitValidator keeps CI a superset of the
// pre-commit hook, which contributors may skip with --no-verify.
func TestCII18nLaneRunsEveryPreCommitValidator(t *testing.T) {
	wf := loadPublicWorkflow(t, "ci.yml")
	run := runScript(requirePublicJob(t, "ci.yml", wf, "i18n"))
	hook := readRepoFile(t, "scripts", "hooks", "pre-commit")

	validators := regexp.MustCompile(`node "\$repo_root/(frontend/scripts/check-[a-z-]+\.js)"((?: --[a-z]+)*)`).FindAllStringSubmatch(hook, -1)
	if len(validators) < 4 {
		t.Fatalf("found %d validators in scripts/hooks/pre-commit; the parser is out of date", len(validators))
	}
	for _, m := range validators {
		requireContainsAll(t, "ci.yml i18n", run, "node "+m[1]+m[2])
	}
	if strings.Contains(hook, "i18n:check") {
		requireContainsAll(t, "ci.yml i18n", run, "npm run i18n:check")
	}
	requireContainsAll(t, "ci.yml i18n", run,
		"node frontend/scripts/check-guest-locales.js --critical",
		"node frontend/scripts/check-guest-locales.js --strict",
		"node frontend/scripts/check-guest-locales.js --keys",
	)
}

func TestCIAIOfflineLaneRunsTheRecordedSuites(t *testing.T) {
	wf := loadPublicWorkflow(t, "ci.yml")
	job := requirePublicJob(t, "ci.yml", wf, "ai-offline")
	run := runScript(job)
	requireContainsAll(t, "ci.yml ai-offline", run,
		"go test ./internal/llmeval/... -run 'TestEvalOffline|TestSuitesWellFormed'",
		"./internal/aicontract/...",
		"./internal/assistantcontract/...",
	)
	for _, line := range strings.Split(run, "\n") {
		if strings.Contains(line, "go test") && !strings.Contains(line, "scripts/ci/backend-env.sh go test") {
			t.Errorf("ai-offline must run every suite through the hermetic env (no provider keys): %q", strings.TrimSpace(line))
		}
	}
}

// scriptTestRunners are the trees whose JavaScript/TypeScript tests run under
// their own runner instead of a ci.yml line each. Shell tests get no such
// pass anywhere: every *_test.sh and *.test.sh must be named in a workflow.
var scriptTestRunners = map[string]string{
	"frontend/": "jest (ci.yml frontend-unit)",
	"tools/":    "each tool's own npm test",
	"evals/":    "promptfoo suites, which call hosted models",
}

// listScriptTests returns every tracked or new (not ignored) shell, Node or
// TypeScript test file in the repository, repo-relative.
func listScriptTests(t *testing.T) []string {
	t.Helper()
	cmd := exec.Command("git", "ls-files", "-z", "--cached", "--others", "--exclude-standard")
	cmd.Dir = repoPath()
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("git ls-files: %v", err)
	}
	shell := regexp.MustCompile(`(?:_test|\.test)\.sh$`)
	node := regexp.MustCompile(`\.test\.(?:mjs|js|ts)$`)
	var tests []string
	for _, file := range strings.Split(string(out), "\x00") {
		if file == "" {
			continue
		}
		if _, err := os.Stat(repoPath(strings.Split(file, "/")...)); err != nil {
			continue // deleted in the working tree
		}
		ownRunner := false
		for prefix := range scriptTestRunners {
			ownRunner = ownRunner || strings.HasPrefix(file, prefix)
		}
		if shell.MatchString(file) || (node.MatchString(file) && !ownRunner) {
			tests = append(tests, file)
		}
	}
	sort.Strings(tests)
	return tests
}

// TestCIContractsLaneRunsEveryScriptTest: a shell or Node contract test that
// CI never runs protects nothing.
func TestCIContractsLaneRunsEveryScriptTest(t *testing.T) {
	var workflows strings.Builder
	for _, name := range publicWorkflows {
		workflows.WriteString(readRepoFile(t, ".github", "workflows", name))
	}
	ci := readRepoFile(t, ".github", "workflows", "ci.yml")

	// The old Caddy harness needs a custom Caddy image built from caddy/.
	// scripts/ci/deploy-caddy-edge_test.sh replaces it (stock caddy:2 with the
	// deploy/ Caddyfiles) and ci.yml runs that one, so the exemption holds
	// only while the replacement is absent; once both exist, the old harness
	// must go or run.
	exempt := map[string]bool{}
	if _, err := os.Stat(repoPath("scripts", "ci", "deploy-caddy-edge_test.sh")); err != nil {
		exempt["scripts/ci/caddy-security-behavior_test.sh"] = true
	}

	tests := listScriptTests(t)
	if len(tests) < 15 {
		t.Fatalf("found only %d script tests; the discovery rules are out of date", len(tests))
	}
	for _, test := range tests {
		if exempt[test] {
			continue
		}
		if !strings.Contains(workflows.String(), test) {
			t.Errorf("%s exists but no workflow runs it", test)
		}
	}
	requireContainsAll(t, "ci.yml", ci,
		"scripts/ci/workflowcontract/go.sum",
		"(cd scripts/ci/workflowcontract && go test -count=1 ./...)",
		"bash scripts/ci/deploy-caddy-edge_test.sh",
	)
	// Node tool packages with their own jest suite.
	contracts := runScript(requirePublicJob(t, "ci.yml", loadPublicWorkflow(t, "ci.yml"), "contracts"))
	for _, dir := range []string{"tools/payverge-admin-mcp", "tools/screenshot-extension"} {
		if !strings.Contains(ci, "working-directory: "+dir) {
			t.Errorf("ci.yml runs no step in %s", dir)
		}
	}
	requireContainsAll(t, "ci.yml contracts", contracts, "npm ci --ignore-scripts --no-audit --no-fund\nnpm test")
	requireContainsAll(t, "ci.yml contracts", contracts, "git ls-files -z '*.sh' | xargs -0 shellcheck -S warning")
	requireContainsAll(t, "ci.yml contracts", contracts,
		"node site/tools/check.mjs\n",
		"node site/tools/check.mjs --self-test",
		"bash tools/oss-export/test.sh",
	)
}

// TestCIContractsLaneGuardsUpstreamDomain: a self-hosted instance must never
// link, mail or redirect to the upstream deployment, so neither the backend
// nor the frontend runtime code may hard-code its domain (full scan, no
// --backend-only / --frontend-only narrowing).
func TestCIContractsLaneGuardsUpstreamDomain(t *testing.T) {
	wf := loadPublicWorkflow(t, "ci.yml")
	contracts := requirePublicJob(t, "ci.yml", wf, "contracts")
	run := runScript(contracts)
	requireContainsAll(t, "ci.yml contracts", run,
		"bash scripts/check-hardcoded-domain.test.sh",
		"./scripts/check-hardcoded-domain.sh\n",
		// A release-please PR must also pass the release gate, so the
		// release cannot be tagged while KNOWN_DEBT remains.
		`if [[ "$HEAD_REF" == release-please--* ]]; then
  ./scripts/check-hardcoded-domain.sh --release`,
	)
	if strings.Contains(strings.ReplaceAll(run, "check-hardcoded-domain.sh --release", ""), "check-hardcoded-domain.sh --") {
		t.Error("ci.yml contracts must run the full domain scan, not a narrowed one")
	}
}

func TestE2EWorkflowRunsChromiumJourneysOnLabelAndNightly(t *testing.T) {
	wf := loadPublicWorkflow(t, "e2e.yml")
	pr, _ := wf.On["pull_request"].(map[string]any)
	types, _ := pr["types"].([]any)
	if !slices.Contains(types, any("labeled")) {
		t.Errorf("e2e.yml pull_request.types = %v, must include labeled", types)
	}
	if _, ok := wf.On["schedule"]; !ok {
		t.Error("e2e.yml must run nightly")
	}
	journeys := requirePublicJob(t, "e2e.yml", wf, "journeys")
	requireContainsAll(t, "e2e.yml journeys.if", journeys.If,
		"github.event_name != 'pull_request'",
		"contains(github.event.pull_request.labels.*.name, 'e2e')",
		"github.event.action != 'labeled' || github.event.label.name == 'e2e'",
	)
	// Concurrency is resolved before the job `if`. A `labeled` event for any
	// other label must land in a group of its own, or it cancels the journeys
	// already running for the PR and then skips its own job.
	if cancel, _ := wf.Concurrency["cancel-in-progress"].(bool); !cancel {
		t.Error("e2e.yml must cancel a run superseded by a newer push")
	}
	group, _ := wf.Concurrency["group"].(string)
	// The event name keeps a manual dispatch from cancelling the nightly run
	// on the same ref, and the reverse.
	if group != "e2e-${{ github.event_name }}-${{ github.ref }}-${{ (github.event.action == 'labeled' && github.event.label.name != 'e2e') && github.run_id || 'journeys' }}" {
		t.Errorf("e2e.yml concurrency.group = %q, want one group per event and ref, unrelated labeled events isolated by run_id", group)
	}
	run := runScript(journeys)
	requireContainsAll(t, "e2e.yml journeys", run,
		"cp .github/e2e/ci.env.fixture .env",
		"bash scripts/ci/rotate-ci-secrets.sh .env",
		"docker compose --env-file .env up -d --build",
		"bash scripts/ci/wait-for-stack.sh",
	)

	ci := loadPublicWorkflow(t, "ci.yml")
	if _, ok := ci.Jobs["journeys"]; ok {
		t.Error("the compose journeys are label-gated in e2e.yml, not part of every ci.yml run")
	}
}

func TestCodeQLAnalyzesGoAndTypeScript(t *testing.T) {
	wf := loadPublicWorkflow(t, "codeql.yml")
	job := requirePublicJob(t, "codeql.yml", wf, "analyze")
	include, _ := job.Strategy.Matrix["include"].([]any)
	var languages []string
	for _, entry := range include {
		languages = append(languages, fmt.Sprint(entry.(map[string]any)["language"]))
	}
	sort.Strings(languages)
	if !slices.Equal(languages, []string{"go", "javascript-typescript"}) {
		t.Errorf("codeql.yml languages = %v, want [go javascript-typescript]", languages)
	}
	if len(stepsUsing(job, "github/codeql-action/init")) != 1 || len(stepsUsing(job, "github/codeql-action/analyze")) != 1 {
		t.Error("codeql.yml must init and analyze exactly once per language")
	}
	for _, step := range job.Steps {
		if strings.Contains(step.Run, "go build") && step.WorkingDirectory != "backend" {
			t.Error("codeql.yml must build the Go module from backend/")
		}
	}
}

// TestScorecardMeetsPublishResultsRestrictions mirrors the conditions under
// which the Scorecard API accepts published results.
func TestScorecardMeetsPublishResultsRestrictions(t *testing.T) {
	wf := loadPublicWorkflow(t, "scorecard.yml")
	if len(wf.Env) != 0 || len(wf.Defaults) != 0 {
		t.Error("scorecard.yml must not set top-level env or defaults when publish_results is true")
	}
	if len(wf.Jobs) != 1 {
		t.Fatalf("scorecard.yml must have exactly one job, found %d", len(wf.Jobs))
	}
	job := requirePublicJob(t, "scorecard.yml", wf, "analysis")
	want := map[string]string{"contents": "read", "actions": "read", "security-events": "write", "id-token": "write"}
	if fmt.Sprint(job.Permissions) != fmt.Sprint(want) {
		t.Errorf("scorecard analysis permissions = %v, want %v", job.Permissions, want)
	}
	if len(job.Env) != 0 {
		t.Error("scorecard analysis must not set job env")
	}
	for i, step := range job.Steps {
		if step.Uses == "" || step.Run != "" {
			t.Errorf("scorecard step %d must be a pinned uses: step with no run:", i+1)
		}
	}
	scorecard := stepsUsing(job, "ossf/scorecard-action")
	if len(scorecard) != 1 || scorecard[0].With["publish_results"] != true {
		t.Error("scorecard.yml must run ossf/scorecard-action with publish_results: true")
	}
	if len(stepsUsing(job, "github/codeql-action/upload-sarif")) != 1 {
		t.Error("scorecard.yml must upload its SARIF to code scanning")
	}
}

func TestReleasePleaseUsesConfigUnderGitHubDir(t *testing.T) {
	wf := loadPublicWorkflow(t, "release.yml")
	job := requirePublicJob(t, "release.yml", wf, "release-please")
	requireContainsAll(t, "release-please.if", job.If, "github.event_name == 'push'", "github.repository == 'stdevMac/payverge'")
	steps := stepsUsing(job, "googleapis/release-please-action")
	if len(steps) != 1 {
		t.Fatalf("release-please job must run googleapis/release-please-action once, found %d", len(steps))
	}
	with := steps[0].With
	if with["config-file"] != ".github/release-please-config.json" || with["manifest-file"] != ".github/.release-please-manifest.json" {
		t.Errorf("release-please config-file/manifest-file = %v / %v", with["config-file"], with["manifest-file"])
	}

	var config struct {
		IncludeVInTag    bool                      `json:"include-v-in-tag"`
		Draft            bool                      `json:"draft"`
		ForceTagCreation bool                      `json:"force-tag-creation"`
		ChangelogPath    string                    `json:"changelog-path"`
		Packages         map[string]map[string]any `json:"packages"`
	}
	if err := json.Unmarshal([]byte(readRepoFile(t, ".github", "release-please-config.json")), &config); err != nil {
		t.Fatalf("parse .github/release-please-config.json: %v", err)
	}
	if !config.IncludeVInTag {
		t.Error("release tags must be vX.Y.Z (include-v-in-tag: true); release.yml resolves only that form")
	}
	if _, ok := config.Packages["."]; !ok {
		t.Error("release-please-config.json must release the repository root package \".\"")
	}
	// The release stays a draft until the assets job has uploaded the install
	// files, so releases/latest/download/install.sh never 404s. GitHub tags a
	// draft only when it is published, and resolve needs the tag right away.
	if !config.Draft || !config.ForceTagCreation {
		t.Errorf("release-please-config.json draft = %v, force-tag-creation = %v; want both true", config.Draft, config.ForceTagCreation)
	}
	for name, pkg := range config.Packages {
		for _, key := range []string{"draft", "force-tag-creation"} {
			if v, ok := pkg[key]; ok && v != true {
				t.Errorf("release-please-config.json package %q overrides %s = %v", name, key, v)
			}
		}
	}
	if !strings.Contains(config.ChangelogPath, "/") {
		t.Errorf("changelog-path = %q; the root is a closed set, keep the changelog under docs/", config.ChangelogPath)
	}

	var manifest map[string]string
	if err := json.Unmarshal([]byte(readRepoFile(t, ".github", ".release-please-manifest.json")), &manifest); err != nil {
		t.Fatalf("parse .github/.release-please-manifest.json: %v", err)
	}
	if !regexp.MustCompile(`^\d+\.\d+\.\d+$`).MatchString(manifest["."]) {
		t.Errorf("release-please manifest version = %q", manifest["."])
	}
}

// The deploy files default PAYVERGE_VERSION to a release tag, never latest.
// That default must equal the release-please manifest version (kept current by
// the generic extra-files markers), so a copy of the deploy files names images
// that release.yml has published for the same release.
func TestDeployImageDefaultsMatchReleaseManifest(t *testing.T) {
	var manifest map[string]string
	if err := json.Unmarshal([]byte(readRepoFile(t, ".github", ".release-please-manifest.json")), &manifest); err != nil {
		t.Fatalf("parse .github/.release-please-manifest.json: %v", err)
	}
	want := manifest["."]
	versionRef := regexp.MustCompile(`\$\{PAYVERGE_VERSION:-([^}]*)\}|^PAYVERGE_VERSION=(\S*)`)
	for _, file := range [][]string{
		{"deploy", "docker-compose.yml"},
		{"deploy", ".env.example"},
		{"deploy", "platforms", "coolify", "docker-compose.yml"},
	} {
		name := filepath.Join(file...)
		found := 0
		for _, line := range strings.Split(readRepoFile(t, file...), "\n") {
			m := versionRef.FindStringSubmatch(line)
			if m == nil {
				continue
			}
			found++
			if got := m[1] + m[2]; got != want {
				t.Errorf("%s: PAYVERGE_VERSION default %q, want the manifest version %q: %s", name, got, want, strings.TrimSpace(line))
			}
		}
		if found == 0 {
			t.Errorf("%s: no PAYVERGE_VERSION default found", name)
		}
	}
}

func TestReleaseBuildsNativeMultiArchImagesWithAttestations(t *testing.T) {
	for i, line := range strings.Split(readRepoFile(t, ".github", "workflows", "release.yml"), "\n") {
		if code, _, _ := strings.Cut(line, "#"); strings.Contains(strings.ToLower(code), "qemu") {
			t.Errorf("release.yml:%d: each platform builds natively, never under QEMU", i+1)
		}
	}
	wf := loadPublicWorkflow(t, "release.yml")
	if wf.Env["IMAGE_PREFIX"] != "ghcr.io/stdevmac/payverge" {
		t.Errorf("IMAGE_PREFIX = %q, want ghcr.io/stdevmac/payverge", wf.Env["IMAGE_PREFIX"])
	}

	build := requirePublicJob(t, "release.yml", wf, "build")
	if got := matrixValues(t, build, "image"); !slices.Equal(got, []string{"backend", "frontend"}) {
		t.Errorf("build images = %v", got)
	}
	if got := matrixValues(t, build, "arch"); !slices.Equal(got, []string{"amd64", "arm64"}) {
		t.Errorf("build arches = %v", got)
	}
	runners := map[string]string{}
	include, _ := build.Strategy.Matrix["include"].([]any)
	for _, entry := range include {
		m := entry.(map[string]any)
		runners[fmt.Sprint(m["arch"])] = fmt.Sprint(m["runner"])
	}
	if runners["amd64"] != "ubuntu-24.04" || runners["arm64"] != "ubuntu-24.04-arm" {
		t.Errorf("build runners = %v, want amd64 on ubuntu-24.04 and arm64 on ubuntu-24.04-arm", runners)
	}
	if build.RunsOn != "${{ matrix.runner }}" {
		t.Errorf("build runs-on = %q, want ${{ matrix.runner }}", build.RunsOn)
	}
	push := stepsUsing(build, "docker/build-push-action")
	if len(push) != 1 {
		t.Fatalf("build must have one build-push-action step")
	}
	with := push[0].With
	if with["provenance"] != "mode=max" || with["sbom"] != true {
		t.Errorf("build-push-action provenance=%v sbom=%v, want mode=max and true", with["provenance"], with["sbom"])
	}
	if with["platforms"] != "linux/${{ matrix.arch }}" {
		t.Errorf("build-push-action platforms = %v", with["platforms"])
	}
	requireContainsAll(t, "build-push-action outputs", fmt.Sprint(with["outputs"]), "push-by-digest=true", "push=true")
	if build.Steps[0].With["ref"] != "${{ needs.resolve.outputs.sha }}" {
		t.Error("build must check out the resolved release commit")
	}

	publish := requirePublicJob(t, "release.yml", wf, "publish")
	if len(stepsUsing(publish, "sigstore/cosign-installer")) != 1 {
		t.Error("publish must install cosign")
	}
	requireContainsAll(t, "release.yml publish", runScript(publish),
		"docker buildx imagetools create",
		`cosign sign --yes "$IMAGE@$DIGEST"`,
	)
	meta := stepsUsing(publish, "docker/metadata-action")
	if len(meta) != 1 {
		t.Fatal("publish must compute tags with docker/metadata-action")
	}
	tags := fmt.Sprint(meta[0].With["tags"])
	// metadata-action rewrites every non-{{raw}} semver pattern to {{version}}
	// for a prerelease, so pattern=v{{version}} would tag 1.2.3-rc.1 only.
	// The v-prefixed tag is the resolved git tag itself, stable or not.
	if strings.Contains(tags, "pattern=v{{version}}") {
		t.Error("publish tags: pattern=v{{version}} drops the v-prefixed tag on prereleases; use type=raw with the resolved tag")
	}
	requireContainsAll(t, "publish tags", tags,
		"pattern={{version}}",
		"type=raw,value=${{ needs.resolve.outputs.tag }}\n",
		"pattern={{major}}.{{minor}}",
		// docs/governance/RELEASING.md promises X.Y.Z, X.Y and X. A 0.x major
		// tag would float across breaking minors, so X starts at 1.0.0.
		"type=semver,pattern={{major}},value=${{ needs.resolve.outputs.tag }},enable=${{ !startsWith(needs.resolve.outputs.tag, 'v0.') }}",
		"type=raw,value=latest,enable=${{ needs.resolve.outputs.stable == 'true' }}",
	)
	if meta[0].With["flavor"] != "latest=false" {
		t.Error("publish must disable metadata-action's implicit latest; latest is for stable releases only")
	}
}

func TestReleaseAssetsShipDeployFilesAndLicences(t *testing.T) {
	wf := loadPublicWorkflow(t, "release.yml")
	assets := requirePublicJob(t, "release.yml", wf, "assets")
	needs := slices.Clone([]string(assets.Needs))
	sort.Strings(needs)
	if !slices.Equal(needs, []string{"publish", "resolve"}) {
		t.Errorf("assets.needs = %v, want [publish resolve]: assets never point at unpublished images", needs)
	}
	run := runScript(assets)
	requireContainsAll(t, "release.yml assets", run,
		`bash scripts/ci/release-assets.sh "$VERSION" "$RUNNER_TEMP/release-assets"`,
		`gh release upload "$TAG"`,
		"--verify-tag --draft",
		// release-please drafts the release; only a push-triggered run, after
		// the upload, publishes it.
		`gh release edit "$TAG" --draft=false`,
		`if [[ "$EVENT_NAME" != push ]]; then`,
	)
	if strings.Index(run, `gh release edit "$TAG" --draft=false`) < strings.Index(run, `gh release upload "$TAG"`) {
		t.Error("release.yml assets must publish the draft release only after uploading the assets")
	}

	script := readRepoFile(t, "scripts", "ci", "release-assets.sh")
	requireContainsAll(t, "scripts/ci/release-assets.sh", script,
		"deploy/install.sh", "deploy/docker-compose.yml", "deploy/Caddyfile", "deploy/.env.example",
		"bash scripts/licenses/generate-third-party.sh",
		"THIRD_PARTY_LICENSES.md",
		"SHA256SUMS",
		// The deploy tarball carries the licence and NOTICE (Apache-2.0, 4(a)/(d)).
		"legal=(LICENSE NOTICE)",
		"--add-file=",
	)
}

// TestReleaseWaitsForGreenCIOnTheReleaseCommit keeps release.yml from
// publishing signed images and install files from a commit ci.yml did not
// pass: a release pull request merged while CI was red or still running, or a
// dispatched tag on an untested commit.
func TestReleaseWaitsForGreenCIOnTheReleaseCommit(t *testing.T) {
	wf := loadPublicWorkflow(t, "release.yml")
	gate := requirePublicJob(t, "release.yml", wf, "ci-verdict")
	if !slices.Equal([]string(gate.Needs), []string{"resolve"}) {
		t.Errorf("ci-verdict.needs = %v, want [resolve]", []string(gate.Needs))
	}
	if len(gate.Permissions) != 2 || gate.Permissions["actions"] != "read" || gate.Permissions["contents"] != "read" {
		t.Errorf("ci-verdict permissions = %v, want exactly {actions: read, contents: read}", gate.Permissions)
	}
	var waits []ghStep
	for _, step := range gate.Steps {
		if strings.Contains(step.Run, "scripts/ci/wait-for-ci.sh") {
			waits = append(waits, step)
		}
	}
	if len(waits) != 1 {
		t.Fatalf("ci-verdict must run scripts/ci/wait-for-ci.sh once, found %d", len(waits))
	}
	if strings.TrimSpace(waits[0].Run) != `bash scripts/ci/wait-for-ci.sh "$SHA"` {
		t.Errorf("ci-verdict run = %q", waits[0].Run)
	}
	// The resolved commit, never the tag name: a tag can move after CI ran.
	if waits[0].Env["SHA"] != "${{ needs.resolve.outputs.sha }}" || waits[0].Env["GH_TOKEN"] != "${{ github.token }}" {
		t.Errorf("ci-verdict env = %v, want SHA from resolve and GH_TOKEN from github.token", waits[0].Env)
	}

	build := requirePublicJob(t, "release.yml", wf, "build")
	if !slices.Contains([]string(build.Needs), "ci-verdict") {
		t.Errorf("build.needs = %v; nothing may build before ci-verdict passes", []string(build.Needs))
	}

	// The gate counts only runs on the commit itself, and only ci.yml.
	script := readRepoFile(t, "scripts", "ci", "wait-for-ci.sh")
	requireContainsAll(t, "scripts/ci/wait-for-ci.sh", script,
		`CI_WORKFLOW:-ci.yml`,
		`actions/workflows/$workflow/runs?head_sha=$sha`,
		`select(.event == "push" or .event == "schedule")]`,
		`.status == "completed" and .conclusion == "success"`,
	)
	// A dispatched CI run may execute another branch's ci.yml.
	if strings.Contains(script, `.event == "workflow_dispatch"`) {
		t.Error("wait-for-ci.sh must not count workflow_dispatch runs of ci.yml")
	}
}

// TestReleasePublishesOnlyFromMain (SC-01): a dispatch runs the copy of
// release.yml on the chosen branch, and a tag may point at a commit main never
// contained. Either would publish signed images from unreviewed code, so the
// workflow refuses non-main refs and non-main tags, and the jobs holding the
// signing identity and release write access sit in the main-only `release`
// environment.
func TestReleasePublishesOnlyFromMain(t *testing.T) {
	wf := loadPublicWorkflow(t, "release.yml")
	push, _ := wf.On["push"].(map[string]any)
	if branches := fmt.Sprint(push["branches"]); branches != "[main]" {
		t.Errorf("release.yml push branches = %s, want [main] only", branches)
	}

	resolve := requirePublicJob(t, "release.yml", wf, "resolve")
	if len(resolve.Steps) < 2 || !strings.HasPrefix(resolve.Steps[0].Uses, "actions/checkout@") || fmt.Sprint(resolve.Steps[0].With["fetch-depth"]) != "0" {
		t.Fatal("resolve must first check out full history (fetch-depth: 0) for the ancestry check")
	}
	var tagStep *ghStep
	for i := range resolve.Steps {
		if resolve.Steps[i].ID == "tag" {
			tagStep = &resolve.Steps[i]
		}
	}
	if tagStep == nil {
		t.Fatal("resolve has no tag step")
	}
	if tagStep.Env["RUN_REF"] != "${{ github.ref }}" {
		t.Errorf("resolve tag step RUN_REF = %q, want ${{ github.ref }}", tagStep.Env["RUN_REF"])
	}
	requireContainsAll(t, "release.yml resolve", tagStep.Run,
		`if [[ "$RUN_REF" != refs/heads/main ]]; then`,
		`git merge-base --is-ancestor "$sha" "$GITHUB_SHA"`,
	)
	if strings.Index(tagStep.Run, "refs/heads/main") > strings.Index(tagStep.Run, `tag="$DISPATCH_TAG"`) {
		t.Error("resolve must reject a non-main ref before it reads the dispatched tag")
	}

	for _, name := range []string{"build", "publish", "assets"} {
		job := requirePublicJob(t, "release.yml", wf, name)
		if !strings.Contains(job.If, "github.ref == 'refs/heads/main'") {
			t.Errorf("release.yml %s.if = %q, want a github.ref == 'refs/heads/main' guard", name, job.If)
		}
	}
	for _, name := range []string{"publish", "assets"} {
		if env := requirePublicJob(t, "release.yml", wf, name).Environment; env != "release" {
			t.Errorf("release.yml %s.environment = %v, want release", name, env)
		}
	}

	identity := "--certificate-identity https://github.com/stdevMac/payverge/.github/workflows/release.yml@refs/heads/main"
	for _, parts := range [][]string{
		{".github", "workflows", "release.yml"},
		{"deploy", "README.md"},
		{"docs", "governance", "RELEASING.md"},
	} {
		src := readRepoFile(t, parts...)
		if !strings.Contains(src, identity) {
			t.Errorf("%s must document the exact cosign identity %q", filepath.Join(parts...), identity)
		}
		if strings.Contains(src, "--certificate-identity-regexp") {
			t.Errorf("%s documents a certificate identity regexp; any branch's release.yml would match it", filepath.Join(parts...))
		}
	}
}

// TestReleaseJobsSurviveASkippedReleasePlease: on workflow_dispatch the
// release-please job is skipped, and GitHub skips every job downstream of a
// skipped job unless that job carries its own status check. Without one, a
// dispatched release (the hand-tagged first release, or a recovery run)
// would resolve its tag and then publish nothing.
func TestReleaseJobsSurviveASkippedReleasePlease(t *testing.T) {
	wf := loadPublicWorkflow(t, "release.yml")
	for _, name := range sortedKeys(wf.Jobs) {
		job := wf.Jobs[name]
		if name == "release-please" {
			continue
		}
		if !strings.Contains(job.If, "!cancelled()") {
			t.Errorf("release.yml %s.if = %q, want !cancelled() so a skipped release-please does not skip it", name, job.If)
		}
		if name == "resolve" {
			continue
		}
		for _, need := range job.Needs {
			if want := "needs." + need + ".result == 'success'"; !strings.Contains(job.If, want) {
				t.Errorf("release.yml %s.if = %q, want %q", name, job.If, want)
			}
		}
	}
}

// TestCIImagesBuildLikeTheRelease keeps one list of image build arguments,
// scripts/ci/image-build-args.sh, shared by release.yml and the ci.yml jobs
// that must fail before a tag does: a frontend that only builds with
// deployment URLs, or a Dockerfile that only builds with extra arguments,
// would otherwise first break while publishing a release.
func TestCIImagesBuildLikeTheRelease(t *testing.T) {
	const script = "bash scripts/ci/image-build-args.sh"

	release := loadPublicWorkflow(t, "release.yml")
	build := requirePublicJob(t, "release.yml", release, "build")
	requireContainsAll(t, "release.yml build", runScript(build),
		script+` "$IMAGE" "$VERSION"`,
		"build-args<<BUILD_ARGS_EOF",
	)

	ci := loadPublicWorkflow(t, "ci.yml")
	images := requirePublicJob(t, "ci.yml", ci, "images")
	if got := matrixValues(t, images, "image"); !slices.Equal(got, []string{"backend", "frontend"}) {
		t.Errorf("ci.yml images matrix = %v, want [backend frontend]", got)
	}
	requireContainsAll(t, "ci.yml images", runScript(images),
		script+` "$IMAGE" 0.0.0-ci`,
		"build-args<<BUILD_ARGS_EOF",
	)
	push := stepsUsing(images, "docker/build-push-action")
	if len(push) != 1 {
		t.Fatalf("ci.yml images must have exactly one build-push-action step, found %d", len(push))
	}
	with := push[0].With
	if with["push"] != false {
		t.Errorf("ci.yml images push = %v, want false: CI never publishes", with["push"])
	}
	if with["context"] != "${{ matrix.image }}" || with["file"] != "${{ matrix.image }}/Dockerfile" {
		t.Errorf("ci.yml images context/file = %v / %v, want the release Dockerfiles", with["context"], with["file"])
	}
	if got := fmt.Sprint(with["build-args"]); got != "${{ steps.args.outputs.build-args }}" {
		t.Errorf("ci.yml images build-args = %q, want only the script output", got)
	}
	if with["no-cache"] != true {
		t.Error("ci.yml images must build from scratch like the release (no-cache: true)")
	}
	for _, key := range []string{"cache-from", "cache-to", "outputs", "tags"} {
		if _, ok := with[key]; ok {
			t.Errorf("ci.yml images sets %s; the CI build is a throwaway", key)
		}
	}
	for _, step := range images.Steps {
		if strings.HasPrefix(step.Uses, "docker/login-action@") {
			t.Error("ci.yml images logs in to a registry; it must not need credentials")
		}
	}

	// No workflow passes a NEXT_PUBLIC_* value of its own outside comments.
	for _, name := range []string{"ci.yml", "release.yml"} {
		for i, line := range strings.Split(readRepoFile(t, ".github", "workflows", name), "\n") {
			if code, _, _ := strings.Cut(line, "#"); strings.Contains(code, "NEXT_PUBLIC_") {
				t.Errorf("%s:%d: sets a NEXT_PUBLIC_* value directly; add it to scripts/ci/image-build-args.sh if every image needs it", name, i+1)
			}
		}
	}

	// Every argument the script emits must be declared by the Dockerfile,
	// or BuildKit drops it with only a warning.
	cmd := exec.Command("bash", "scripts/ci/image-build-args.sh", "frontend", "0.0.0-ci")
	cmd.Dir = repoPath()
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("scripts/ci/image-build-args.sh frontend: %v", err)
	}
	dockerfile := readRepoFile(t, "frontend", "Dockerfile")
	lines := strings.Fields(string(out))
	if len(lines) == 0 {
		t.Fatal("scripts/ci/image-build-args.sh frontend printed nothing")
	}
	for _, line := range lines {
		key, _, _ := strings.Cut(line, "=")
		if !regexp.MustCompile(`(?m)^ARG ` + regexp.QuoteMeta(key) + `(=|$)`).MatchString(dockerfile) {
			t.Errorf("frontend/Dockerfile does not declare ARG %s, which image-build-args.sh passes", key)
		}
	}
	cmd = exec.Command("bash", "scripts/ci/image-build-args.sh", "backend", "0.0.0-ci")
	cmd.Dir = repoPath()
	if out, err := cmd.Output(); err != nil || len(out) != 0 {
		t.Errorf("scripts/ci/image-build-args.sh backend = %q, %v; the backend image takes no build arguments", out, err)
	}
}

// The Render Blueprint is applied as-is, so it must name images release.yml
// published for this release (never latest, which would let a redeploy move
// one service to a newer release than the other) and must connect to its
// database without a hand-edited start command: the server reads DB_* from
// the environment when no --db-* flag is given.
func TestRenderBlueprintPinsReleaseImagesAndReadsDatabaseFromEnv(t *testing.T) {
	var manifest map[string]string
	if err := json.Unmarshal([]byte(readRepoFile(t, ".github", ".release-please-manifest.json")), &manifest); err != nil {
		t.Fatalf("parse .github/.release-please-manifest.json: %v", err)
	}
	want := manifest["."]

	raw := readRepoFile(t, "deploy", "platforms", "render", "render.yaml")
	var blueprint struct {
		Databases []struct {
			Name string `yaml:"name"`
		} `yaml:"databases"`
		Services []struct {
			Type          string `yaml:"type"`
			Name          string `yaml:"name"`
			DockerCommand string `yaml:"dockerCommand"`
			Image         struct {
				URL string `yaml:"url"`
			} `yaml:"image"`
			EnvVars []struct {
				Key          string `yaml:"key"`
				Value        string `yaml:"value"`
				FromDatabase *struct {
					Name     string `yaml:"name"`
					Property string `yaml:"property"`
				} `yaml:"fromDatabase"`
			} `yaml:"envVars"`
		} `yaml:"services"`
	}
	if err := yaml.Unmarshal([]byte(raw), &blueprint); err != nil {
		t.Fatalf("parse render.yaml: %v", err)
	}
	if strings.Contains(raw, ":latest") {
		t.Error("render.yaml must not reference a :latest image")
	}
	if strings.Contains(raw, "DATABASE_INTERNAL_HOST") {
		t.Error("render.yaml still carries the DATABASE_INTERNAL_HOST placeholder")
	}
	if len(blueprint.Databases) != 1 {
		t.Fatalf("render.yaml databases = %d, want 1", len(blueprint.Databases))
	}
	db := blueprint.Databases[0].Name

	images := map[string]string{}
	for _, svc := range blueprint.Services {
		images[svc.Name] = svc.Image.URL
		if svc.Name != "payverge-backend" {
			continue
		}
		if svc.DockerCommand != "" {
			t.Errorf("payverge-backend dockerCommand = %q; the image entrypoint and DB_* env are enough", svc.DockerCommand)
		}
		env := map[string]string{}
		for _, v := range svc.EnvVars {
			if v.FromDatabase != nil {
				if v.FromDatabase.Name != db {
					t.Errorf("%s fromDatabase.name = %q, want %q", v.Key, v.FromDatabase.Name, db)
				}
				env[v.Key] = "fromDatabase:" + v.FromDatabase.Property
			} else {
				env[v.Key] = v.Value
			}
		}
		for key, property := range map[string]string{
			"DB_HOST": "host", "DB_PORT": "port", "DB_USER": "user",
			"DB_NAME": "database", "DB_PASSWORD": "password",
		} {
			if env[key] != "fromDatabase:"+property {
				t.Errorf("payverge-backend %s = %q, want fromDatabase %s", key, env[key], property)
			}
		}
		if env["APP_ENV"] != "production" {
			t.Errorf("payverge-backend APP_ENV = %q; without --production it is what turns production mode on", env["APP_ENV"])
		}
	}
	for name, image := range map[string]string{
		"payverge-backend":  "ghcr.io/stdevmac/payverge-backend:" + want,
		"payverge-frontend": "ghcr.io/stdevmac/payverge-frontend:" + want,
	} {
		if images[name] != image {
			t.Errorf("%s image = %q, want %q (the release-please manifest version)", name, images[name], image)
		}
	}
	if !strings.Contains(readRepoFile(t, ".github", "release-please-config.json"), `"path": "deploy/platforms/render/render.yaml"`) {
		t.Error("release-please-config.json must bump the render.yaml image tags (generic extra-files entry)")
	}
	if got := strings.Count(raw, "x-release-please-start-version"); got != 2 {
		t.Errorf("render.yaml has %d x-release-please-start-version markers, want 2 (one per image)", got)
	}

	// The blueprint relies on these env fallbacks in the server's flag defaults.
	mainGo := readRepoFile(t, "backend", "cmd", "app", "main.go")
	for _, fallback := range []string{
		`flag.String("db-host", envOrDefault("DB_HOST",`,
		`flag.String("db-port", envOrDefault("DB_PORT",`,
		`flag.String("db-user", envOrDefault("DB_USER",`,
		`flag.String("db-name", envOrDefault("DB_NAME",`,
		`flag.String("db-sslmode", envOrDefault("DB_SSLMODE",`,
		`os.Getenv("DB_PASSWORD")`,
	} {
		if !strings.Contains(mainGo, fallback) {
			t.Errorf("backend/cmd/app/main.go no longer has %s; render.yaml passes the database only through env", fallback)
		}
	}
}

// The backend's TRUSTED_PROXIES default is loopback only
// (config.DefaultTrustedProxies). Every platform template puts the frontend on
// a private network in front of the backend, so each one must set the value
// explicitly to private ranges, or every visitor shares the frontend's address
// for rate limits and audit logs.
func TestPlatformTemplatesSetTrustedProxiesExplicitly(t *testing.T) {
	defaults := readRepoFile(t, "backend", "internal", "config", "deploy_defaults.go")
	if !strings.Contains(defaults, `DefaultTrustedProxies = "127.0.0.0/8,::1"`) {
		t.Fatal("config.DefaultTrustedProxies changed; revisit the platform templates' TRUSTED_PROXIES")
	}

	coolify := readRepoFile(t, "deploy", "platforms", "coolify", "docker-compose.yml")
	render := readRepoFile(t, "deploy", "platforms", "render", "render.yaml")
	railway := readRepoFile(t, "deploy", "platforms", "railway", "README.md")

	values := map[string]string{}
	if m := regexp.MustCompile(`(?m)^\s*- TRUSTED_PROXIES=\$\{TRUSTED_PROXIES:-([^}]*)\}\s*$`).FindStringSubmatch(coolify); m != nil {
		values["coolify"] = m[1]
	}
	if m := regexp.MustCompile(`(?m)^\s*- key: TRUSTED_PROXIES\n\s*value: (\S+)\s*$`).FindStringSubmatch(render); m != nil {
		values["render"] = m[1]
	}
	if m := regexp.MustCompile("(?m)^\\| `TRUSTED_PROXIES` \\| `([^`]+)` \\|$").FindStringSubmatch(railway); m != nil {
		values["railway"] = m[1]
	}

	private := []netip.Prefix{
		netip.MustParsePrefix("10.0.0.0/8"),
		netip.MustParsePrefix("172.16.0.0/12"),
		netip.MustParsePrefix("192.168.0.0/16"),
		netip.MustParsePrefix("fc00::/7"),
	}
	for _, platform := range []string{"coolify", "render", "railway"} {
		raw, ok := values[platform]
		if !ok || strings.TrimSpace(raw) == "" {
			t.Errorf("%s template must set TRUSTED_PROXIES to a non-empty value (the backend default is loopback only)", platform)
			continue
		}
		for _, item := range strings.Split(raw, ",") {
			prefix, err := netip.ParsePrefix(strings.TrimSpace(item))
			if err != nil {
				t.Errorf("%s TRUSTED_PROXIES entry %q is not a CIDR: %v", platform, item, err)
				continue
			}
			inside := false
			for _, p := range private {
				if p.Bits() <= prefix.Bits() && p.Contains(prefix.Addr()) {
					inside = true
				}
			}
			if !inside {
				t.Errorf("%s TRUSTED_PROXIES entry %q is not inside a private range", platform, item)
			}
		}
	}
	for name, raw := range map[string]string{"coolify": coolify, "render": render, "railway": railway} {
		if strings.Contains(raw, "defaults to private ranges") || strings.Contains(raw, "default private ranges") || strings.Contains(raw, "Leave `TRUSTED_PROXIES` unset") {
			t.Errorf("%s template still claims the backend defaults TRUSTED_PROXIES to private ranges", name)
		}
	}
}

// `make test` is what contributors run before opening a PR, so it must match
// the CI backend lane (-short skips the Testcontainers suites). The full run,
// Docker included, lives behind test-docker.
func TestBackendMakeTestMatchesTheCISuite(t *testing.T) {
	makefile := readRepoFile(t, "backend", "Makefile")
	recipe := func(target string) string {
		m := regexp.MustCompile(`(?m)^` + regexp.QuoteMeta(target) + `:\n((?:\t.*\n)+)`).FindStringSubmatch(makefile)
		if m == nil {
			t.Fatalf("backend/Makefile has no %s target", target)
		}
		return m[1]
	}
	for _, target := range []string{"test", "quick-test"} {
		if !regexp.MustCompile(`(?m)^\tgo test -short\b.* \./\.\.\.$`).MatchString(recipe(target)) {
			t.Errorf("backend/Makefile %s must run go test -short ./...:\n%s", target, recipe(target))
		}
	}
	docker := recipe("test-docker")
	if !strings.Contains(docker, "go test") || strings.Contains(docker, "-short") {
		t.Errorf("backend/Makefile test-docker must run the full suite without -short:\n%s", docker)
	}
}
