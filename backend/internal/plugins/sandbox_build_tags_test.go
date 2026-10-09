package plugins

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Live payment-provider sandbox tests call real provider APIs with real
// credentials. They must sit behind their own opt-in build tag so neither a
// default `go test ./...` nor the generic `integration` tag used by CI and
// scripts/verify-backend.sh ever compiles them.
func TestSandboxIntegrationTestsRequireProviderBuildTag(t *testing.T) {
	wantTags := map[string]string{
		"paypal":      "paypal_sandbox",
		"mercadopago": "mercadopago_sandbox",
	}
	found := 0
	for dir, tag := range wantTags {
		matches, err := filepath.Glob(filepath.Join(dir, "*sandbox*_test.go"))
		if err != nil {
			t.Fatalf("glob %s: %v", dir, err)
		}
		for _, path := range matches {
			found++
			constraint := firstBuildConstraint(t, path)
			if constraint != "//go:build "+tag {
				t.Errorf("%s: build constraint %q, want %q", path, constraint, "//go:build "+tag)
			}
			body, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read %s: %v", path, err)
			}
			if strings.Contains(string(body), "payverge.io") {
				t.Errorf("%s: sandbox redirect URLs must point at localhost, not a production domain", path)
			}
		}
	}
	if found == 0 {
		t.Fatal("no sandbox integration tests found; update this guard if they moved")
	}
}

func firstBuildConstraint(t *testing.T, path string) string {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("open %s: %v", path, err)
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "//go:build") {
			return line
		}
		if strings.HasPrefix(line, "package ") {
			break
		}
	}
	return ""
}
