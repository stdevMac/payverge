package main

import (
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// TestComposeRoutesMediaThroughTheFrontend pins the zero-config wiring that
// makes persisted "/media/<key>" URLs resolvable with the shipped compose:
// the backend defaults to the local driver and relative media URLs, and the
// frontend proxies /media to the backend because BACKEND_INTERNAL_URL
// defaults to the backend service. Dropping either default reintroduces
// broken images on a fresh install (the frontend /media route 404s when
// BACKEND_INTERNAL_URL is unset).
func TestComposeRoutesMediaThroughTheFrontend(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join(storageTestRepoRoot(t), "docker-compose.yml"))
	if err != nil {
		t.Fatalf("read docker-compose.yml: %v", err)
	}
	// Other services use the map form of environment:, so decode lazily.
	var doc struct {
		Services map[string]yaml.Node `yaml:"services"`
	}
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("parse docker-compose.yml: %v", err)
	}
	type service struct {
		Environment []string `yaml:"environment"`
		Volumes     []string `yaml:"volumes"`
	}
	load := func(name string) service {
		node, ok := doc.Services[name]
		if !ok {
			t.Fatalf("docker-compose.yml has no %q service", name)
		}
		var svc service
		if err := node.Decode(&svc); err != nil {
			t.Fatalf("decode %s service: %v", name, err)
		}
		return svc
	}
	env := func(svc service) map[string]string {
		out := map[string]string{}
		for _, kv := range svc.Environment {
			k, v, _ := strings.Cut(kv, "=")
			out[k] = v
		}
		return out
	}

	backendSvc := load("backend")
	backend := env(backendSvc)
	for key, want := range map[string]string{
		"STORAGE_DRIVER": "${STORAGE_DRIVER:-local}",
		"STORAGE_DIR":    "${STORAGE_DIR:-/data/storage}",
		"PUBLIC_URL":     "${PUBLIC_URL:-http://localhost:3000}",
	} {
		if got := backend[key]; got != want {
			t.Errorf("backend %s = %q, want %q", key, got, want)
		}
	}
	mounted := false
	for _, v := range backendSvc.Volumes {
		if strings.HasSuffix(v, ":/data/storage") {
			mounted = true
		}
	}
	if !mounted {
		t.Error("backend must mount a volume at /data/storage (STORAGE_DIR default)")
	}

	frontend := env(load("frontend"))
	if got, want := frontend["BACKEND_INTERNAL_URL"], "${BACKEND_INTERNAL_URL:-http://backend:8080}"; got != want {
		t.Errorf("frontend BACKEND_INTERNAL_URL = %q, want %q: without it the Next /media proxy is off and every /media/<key> image 404s", got, want)
	}
	if _, ok := frontend["PUBLIC_URL"]; !ok {
		t.Error("frontend must receive PUBLIC_URL (compose only forwards declared variables)")
	}
}

// TestComposeMediaURLsHaveAFrontendRoute guards the merge order with the
// fe-runtime workstream. The storage driver persists relative "/media/<key>"
// URLs, and the shipped compose has guests' browsers load every page from
// the frontend origin, so each image request reaches the Next server. That
// works only when the frontend has a /media route handler that proxies to
// the backend, and the middleware matcher leaves /media/* to it (otherwise
// the locale middleware redirects /media/x to /<locale>/media/x). Both come
// from fe-runtime. On the storage branch alone this test fails by design:
// it ships uploads whose URLs 404.
func TestComposeMediaURLsHaveAFrontendRoute(t *testing.T) {
	const hint = "relative /media/<key> URLs need the frontend same-origin media proxy " +
		"from the fe-runtime workstream: merge fe-runtime before, or together with, storage"
	root := storageTestRepoRoot(t)

	route := filepath.Join("frontend", "src", "app", "media", "[...path]", "route.ts")
	src, err := os.ReadFile(filepath.Join(root, route))
	if err != nil {
		t.Fatalf("%s is missing (%v): %s", route, err, hint)
	}
	for _, method := range []string{"GET", "HEAD"} {
		export := regexp.MustCompile(`export\s+(?:async\s+)?(?:function\s+|const\s+)` + method + `\b`)
		if !export.Match(src) {
			t.Errorf("%s does not export %s: %s", route, method, hint)
		}
	}

	mw := filepath.Join("frontend", "src", "middleware.ts")
	mwSrc, err := os.ReadFile(filepath.Join(root, mw))
	if err != nil {
		t.Fatalf("read %s: %v", mw, err)
	}
	_, matcher, found := strings.Cut(string(mwSrc), "matcher:")
	if !found {
		t.Fatalf("%s has no config.matcher", mw)
	}
	matcher, _, _ = strings.Cut(matcher, "]")
	excludesMedia := regexp.MustCompile(`\(\?!(?:[^"]*\|)?media/(?:\||\))`)
	if !excludesMedia.MatchString(matcher) {
		t.Errorf("%s matcher does not exclude media/, so the locale middleware intercepts /media/<key>: %s", mw, hint)
	}
}

func storageTestRepoRoot(t *testing.T) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(thisFile), "..", "..", ".."))
}
