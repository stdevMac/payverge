package database

import (
	"os"
	"strings"
	"testing"
)

func TestDemoSeedDoesNotReferenceDummyimageDotCom(t *testing.T) {
	raw, err := os.ReadFile("../../scripts/demo_seed.sql")
	if err != nil {
		t.Fatalf("read seed: %v", err)
	}
	if strings.Contains(string(raw), "dummyimage.com") {
		t.Fatalf("demo_seed.sql still references dummyimage.com (not on production CSP allowlist)")
	}
}
