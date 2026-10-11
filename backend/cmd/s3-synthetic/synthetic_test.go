package main

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// fakeStore is an in-memory objectStore for unit tests. It can be told to fail
// a specific operation, and it records how many deletes ran so tests can assert
// cleanup happened even on the failure paths.
type fakeStore struct {
	objects   map[string][]byte
	failPut   bool
	failGet   bool
	failDel   bool
	getReturn []byte // when non-nil, Get returns this instead of the stored bytes
	deletes   int
}

func newFakeStore() *fakeStore { return &fakeStore{objects: map[string][]byte{}} }

func (f *fakeStore) Put(_ context.Context, key string, body []byte) error {
	if f.failPut {
		return errors.New("put boom")
	}
	cp := make([]byte, len(body))
	copy(cp, body)
	f.objects[key] = cp
	return nil
}

func (f *fakeStore) Get(_ context.Context, key string) ([]byte, error) {
	if f.failGet {
		return nil, errors.New("get boom")
	}
	if f.getReturn != nil {
		return f.getReturn, nil
	}
	b, ok := f.objects[key]
	if !ok {
		return nil, errors.New("not found")
	}
	return b, nil
}

func (f *fakeStore) Delete(_ context.Context, key string) error {
	f.deletes++
	if f.failDel {
		return errors.New("delete boom")
	}
	delete(f.objects, key)
	return nil
}

func TestRoundtripSuccessCleansUp(t *testing.T) {
	store := newFakeStore()
	if err := roundtrip(context.Background(), store, "synthetic/probe/x", []byte("hello")); err != nil {
		t.Fatalf("expected success, got %v", err)
	}
	if len(store.objects) != 0 {
		t.Fatalf("synthetic object leaked; store not empty: %v", store.objects)
	}
	if store.deletes != 1 {
		t.Fatalf("expected exactly one delete, got %d", store.deletes)
	}
}

func TestRoundtripPayloadMismatchIsError(t *testing.T) {
	store := newFakeStore()
	store.getReturn = []byte("tampered")
	err := roundtrip(context.Background(), store, "synthetic/probe/x", []byte("hello"))
	if err == nil || !strings.Contains(err.Error(), "mismatch") {
		t.Fatalf("expected mismatch error, got %v", err)
	}
	if store.deletes != 1 {
		t.Fatalf("cleanup must run even on mismatch; deletes=%d", store.deletes)
	}
}

func TestRoundtripGetFailureStillCleansUp(t *testing.T) {
	store := newFakeStore()
	store.failGet = true
	err := roundtrip(context.Background(), store, "synthetic/probe/x", []byte("hello"))
	if err == nil {
		t.Fatal("expected error when Get fails")
	}
	if store.deletes != 1 {
		t.Fatalf("cleanup must run even when Get fails; deletes=%d", store.deletes)
	}
}

func TestRoundtripPutFailureSkipsGetAndDelete(t *testing.T) {
	store := newFakeStore()
	store.failPut = true
	err := roundtrip(context.Background(), store, "synthetic/probe/x", []byte("hello"))
	if err == nil || !strings.Contains(err.Error(), "put") {
		t.Fatalf("expected put error, got %v", err)
	}
	// Nothing was written, so nothing should be deleted.
	if store.deletes != 0 {
		t.Fatalf("expected no delete when Put fails, got %d", store.deletes)
	}
}

func TestRoundtripCleanupFailureIsSurfaced(t *testing.T) {
	store := newFakeStore()
	store.failDel = true
	err := roundtrip(context.Background(), store, "synthetic/probe/x", []byte("hello"))
	if err == nil || !strings.Contains(err.Error(), "cleanup") {
		t.Fatalf("expected cleanup failure to surface, got %v", err)
	}
}

func TestLoadBucketConfigPublic(t *testing.T) {
	env := map[string]string{
		"S3_BUCKET":      "pv-public",
		"AWS_ACCESS_KEY": "AKIA_public",
		"AWS_SECRET_KEY": "secret_public_value",
		"S3_ENDPOINT":    "https://e2.example.com",
		"AWS_REGION":     "us-west-2",
	}
	cfg, err := loadBucketConfig("public", func(k string) string { return env[k] })
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.name != "pv-public" || cfg.region != "us-west-2" || cfg.endpoint != "https://e2.example.com" {
		t.Fatalf("unexpected config: %+v", cfg)
	}
}

func TestLoadBucketConfigNormalizesR2RegionLikeProduction(t *testing.T) {
	env := map[string]string{
		"S3_BUCKET":      "pv-public",
		"AWS_ACCESS_KEY": "AKIA_public",
		"AWS_SECRET_KEY": "secret_public_value",
		"S3_ENDPOINT":    "https://account.r2.cloudflarestorage.com",
		"AWS_REGION":     "us-west-2",
	}
	cfg, err := loadBucketConfig("public", func(k string) string { return env[k] })
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.region != "auto" {
		t.Fatalf("R2 synthetic region = %q, want auto to match production storage", cfg.region)
	}
}

func TestLoadBucketConfigRejectsProtocolRelativeEndpoint(t *testing.T) {
	env := map[string]string{
		"S3_BUCKET":      "pv-public",
		"AWS_ACCESS_KEY": "AKIA_public",
		"AWS_SECRET_KEY": "secret_public_value",
		"S3_ENDPOINT":    "//account.r2.cloudflarestorage.com",
		"AWS_REGION":     "auto",
	}
	_, err := loadBucketConfig("public", func(k string) string { return env[k] })
	if err == nil || !strings.Contains(err.Error(), "S3_ENDPOINT") {
		t.Fatalf("expected an actionable S3_ENDPOINT error, got %v", err)
	}
}

func TestLoadBucketConfigProtectedReadsProtectedVars(t *testing.T) {
	env := map[string]string{
		"S3_PROTECTED_BUCKET":      "pv-protected",
		"AWS_PROTECTED_ACCESS_KEY": "AKIA_prot",
		"AWS_PROTECTED_SECRET_KEY": "secret_prot_value",
		// no AWS_REGION -> should default
	}
	cfg, err := loadBucketConfig("protected", func(k string) string { return env[k] })
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.name != "pv-protected" {
		t.Fatalf("protected bucket not read from S3_PROTECTED_BUCKET: %+v", cfg)
	}
	if cfg.region != "us-east-1" {
		t.Fatalf("expected default region us-east-1, got %q", cfg.region)
	}
}

func TestLoadBucketConfigMissingVarsNamesThemWithoutLeakingSecret(t *testing.T) {
	env := map[string]string{
		"S3_BUCKET": "pv-public",
		// AWS_ACCESS_KEY missing
		"AWS_SECRET_KEY": "topsecret_should_not_appear",
	}
	_, err := loadBucketConfig("public", func(k string) string { return env[k] })
	if err == nil {
		t.Fatal("expected error for missing AWS_ACCESS_KEY")
	}
	if !strings.Contains(err.Error(), "AWS_ACCESS_KEY") {
		t.Fatalf("error should name the missing var, got %v", err)
	}
	if strings.Contains(err.Error(), "topsecret_should_not_appear") {
		t.Fatalf("error leaked a secret VALUE: %v", err)
	}
}

func TestBucketConfigSummaryOmitsSecrets(t *testing.T) {
	cfg := bucketConfig{
		label:     "public",
		name:      "pv-public",
		accessKey: "AKIA_leak_me",
		secretKey: "supersecret_leak_me",
		region:    "us-east-1",
		endpoint:  "https://e2.example.com",
	}
	s := cfg.summary()
	if strings.Contains(s, "AKIA_leak_me") || strings.Contains(s, "supersecret_leak_me") {
		t.Fatalf("summary leaked credentials: %q", s)
	}
	if !strings.Contains(s, "pv-public") || !strings.Contains(s, "public") {
		t.Fatalf("summary should include label and bucket name: %q", s)
	}
}

func TestSelectBuckets(t *testing.T) {
	cases := map[string][]string{
		"both":      {"public", "protected"},
		"public":    {"public"},
		"protected": {"protected"},
	}
	for in, want := range cases {
		got, err := selectBuckets(in)
		if err != nil {
			t.Fatalf("selectBuckets(%q) errored: %v", in, err)
		}
		if strings.Join(got, ",") != strings.Join(want, ",") {
			t.Fatalf("selectBuckets(%q)=%v want %v", in, got, want)
		}
	}
	if _, err := selectBuckets("nonsense"); err == nil {
		t.Fatal("expected error for invalid bucket selector")
	}
}

func TestGenerateKeyUniqueAndPrefixed(t *testing.T) {
	k1 := generateKey()
	k2 := generateKey()
	if !strings.HasPrefix(k1, syntheticPrefix) {
		t.Fatalf("key %q missing prefix %q", k1, syntheticPrefix)
	}
	if k1 == k2 {
		t.Fatalf("keys should be unique: %q == %q", k1, k2)
	}
}
