package workflowcontract

import (
	"regexp"
	"strings"
	"testing"
)

func TestBrowserReleaseConfigsAdvertiseEverySupportedEngine(t *testing.T) {
	for _, config := range []string{"playwright.config.ts"} {
		source := readRepoFile(t, "frontend", config)
		for _, project := range []string{"chromium", "firefox", "webkit", "iphone-webkit"} {
			if !strings.Contains(source, `name: "`+project+`"`) {
				t.Errorf("%s: missing %s project", config, project)
			}
		}
		if !strings.Contains(source, "no-skipped-tests-reporter") {
			t.Errorf("%s: CI must fail when an advertised project skips a test", config)
		}
	}
}

// TestBrowserReleaseWorkflowsInstallEveryEngine: the public E2E workflow runs
// the compose journeys in Chromium only. Every `playwright test` names the
// project, so a new config project cannot start running without a browser
// install. Firefox and WebKit stay available locally through the config.
func TestBrowserReleaseWorkflowsInstallEveryEngine(t *testing.T) {
	source := readRepoFile(t, ".github", "workflows", "e2e.yml")
	if !strings.Contains(source, "npx playwright install --with-deps chromium\n") {
		t.Error("e2e.yml must install Chromium with its system dependencies")
	}
	runs := regexp.MustCompile(`npx playwright test(?:[^\n]*\\\n)*[^\n]*`).FindAllString(source, -1)
	if len(runs) == 0 {
		t.Fatal("e2e.yml runs no playwright test")
	}
	for _, run := range runs {
		if !strings.Contains(run, "--project=chromium") {
			t.Errorf("e2e.yml: %q must pass --project=chromium", run)
		}
	}
}

func TestC10RunsInEveryReleaseBrowser(t *testing.T) {
	config := readRepoFile(t, "frontend", "playwright.config.ts")
	if !strings.Contains(config, `testMatch: "**/*.spec.ts"`) || !strings.Contains(config, `name: "mobile-chrome"`) {
		t.Fatal("every release browser project, including mobile-chrome, must execute the C10 browser contract")
	}
}

func TestC10DeclaresItsProductionJourneyTimeout(t *testing.T) {
	spec := readRepoFile(t, "frontend", "tests", "release", "c10-cross-stack-user-contracts.spec.ts")
	if !strings.Contains(spec, "test.describe.configure({ timeout: 300_000 })") {
		t.Fatal("C10 must declare its five-minute journey timeout in source; a local CLI override does not protect the release workflow")
	}
}

func TestCriticalReleaseSmokeCoversRTLAndPaymentProviderReturn(t *testing.T) {
	config := readRepoFile(t, "frontend", "playwright.config.ts")
	for _, project := range []string{"chromium", "firefox", "webkit", "iphone-webkit"} {
		projectMarker := `name: "` + project + `"`
		if !strings.Contains(config, projectMarker) {
			t.Fatalf("release config must execute critical journeys in %s", project)
		}
	}

	spec := readRepoFile(t, "frontend", "tests", "release", "critical-release-smoke.spec.ts")
	for _, required := range []string{
		`provider-handoff`,
		`/payment-plugins`,
		`/plugin-payment`,
		`sessionStorage.getItem("payverge_payment")`,
		`payment=cancelled`,
		`?lang=ar`,
		`toHaveAttribute("lang", "ar")`,
		`toHaveAttribute("dir", "rtl")`,
	} {
		if !strings.Contains(spec, required) {
			t.Errorf("critical release smoke missing browser contract %q", required)
		}
	}
}
