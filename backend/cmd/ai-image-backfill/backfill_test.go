package main

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/color"
	"image/png"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/stdevmac/payverge/backend/internal/services"
)

func TestBackupKeyFor(t *testing.T) {
	assert.Equal(t, "menu_items/ai_generated_backup/foo.png",
		backupKeyFor("menu_items/ai_generated/foo.png"))
	assert.Equal(t, "menu_items/ai_generated_backup/dish/uuid.png",
		backupKeyFor("menu_items/ai_generated/dish/uuid.png"))
	assert.Equal(t, "menu_items/ai_generated_backup/x",
		backupKeyFor("/menu_items/ai_generated/x"))
}

func TestIsAIGeneratedKey(t *testing.T) {
	assert.True(t, isAIGeneratedKey("menu_items/ai_generated/a.png"))
	assert.True(t, isAIGeneratedKey("menu_items/ai_generated/sub/a.png"))
	assert.False(t, isAIGeneratedKey("menu_items/ai_generated_backup/a.png"))
	assert.False(t, isAIGeneratedKey("menu_items/other/a.png"))
}

func TestDecideObject_SkipAlreadyOptimizedJPEG(t *testing.T) {
	// Minimal JPEG SOI/EOI + enough bytes for DetectContentType.
	jpegish := []byte{
		0xff, 0xd8, 0xff, 0xe0, 0x00, 0x10, 0x4a, 0x46, 0x49, 0x46, 0x00, 0x01,
		0x01, 0x00, 0x00, 0x01, 0x00, 0x01, 0x00, 0x00, 0xff, 0xd9,
	}
	// Pad so size is realistic but under threshold.
	body := make([]byte, 10_000)
	copy(body, jpegish)

	d := decideObject("menu_items/ai_generated/small.jpg", int64(len(body)), body)
	assert.Equal(t, "skip", d.Action)
	assert.Contains(t, d.Reason, "already optimized")
}

func TestDecideObject_ProcessLargePNG(t *testing.T) {
	body := makeLargePNG(t, 1600, 1200)
	// PNG may compress small; decision keys off MIME + non-JPEG/WebP, not a
	// minimum byte floor. Pass an oversized Size so skip-if-small cannot apply.
	d := decideObject("menu_items/ai_generated/big.png", skipIfAtMostBytes+1, body)
	assert.Equal(t, "process", d.Action)
	assert.Equal(t, "menu_items/ai_generated_backup/big.png", d.BackupKey)
}

func TestDecideObject_SkipBackupAndEmpty(t *testing.T) {
	d := decideObject("menu_items/ai_generated_backup/x.png", 100, []byte{1, 2, 3})
	assert.Equal(t, "skip", d.Action)

	d2 := decideObject("menu_items/ai_generated/empty.png", 0, nil)
	assert.Equal(t, "skip", d2.Action)
}

type fakeStore struct {
	mu      sync.Mutex
	objects map[string][]byte
	types   map[string]string
	failGet map[string]bool
	failPut map[string]bool
}

func newFakeStore() *fakeStore {
	return &fakeStore{
		objects: map[string][]byte{},
		types:   map[string]string{},
		failGet: map[string]bool{},
		failPut: map[string]bool{},
	}
}

func (f *fakeStore) List(_ context.Context, prefix string) ([]objectInfo, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []objectInfo
	for k, v := range f.objects {
		if strings.HasPrefix(k, prefix) {
			out = append(out, objectInfo{Key: k, Size: int64(len(v))})
		}
	}
	return out, nil
}

func (f *fakeStore) Get(_ context.Context, key string) ([]byte, string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.failGet[key] {
		return nil, "", errors.New("get boom")
	}
	b, ok := f.objects[key]
	if !ok {
		return nil, "", errors.New("not found")
	}
	cp := make([]byte, len(b))
	copy(cp, b)
	return cp, f.types[key], nil
}

func (f *fakeStore) Put(_ context.Context, key string, body []byte, contentType string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.failPut[key] {
		return errors.New("put boom")
	}
	cp := make([]byte, len(body))
	copy(cp, body)
	f.objects[key] = cp
	f.types[key] = contentType
	return nil
}

func (f *fakeStore) Copy(_ context.Context, srcKey, dstKey string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	b, ok := f.objects[srcKey]
	if !ok {
		return errors.New("copy source missing")
	}
	cp := make([]byte, len(b))
	copy(cp, b)
	f.objects[dstKey] = cp
	return nil
}

func TestRunBackfill_DryRunDefaultDoesNotWrite(t *testing.T) {
	store := newFakeStore()
	pngBody := makeLargePNG(t, 1400, 1000)
	key := "menu_items/ai_generated/legacy.png"
	store.objects[key] = pngBody
	store.types[key] = "image/png"

	var buf bytes.Buffer
	summary, err := runBackfill(context.Background(), store, backfillOptions{
		Prefix:      defaultAIGeneratedPrefix,
		Apply:       false,
		Concurrency: 2,
		Out:         &buf,
	})
	require.NoError(t, err)
	assert.Equal(t, "dry-run", summary.Mode)
	assert.Equal(t, 1, summary.Listed)
	assert.Equal(t, 1, summary.Processed)
	assert.Equal(t, 0, summary.Failed)

	// Original untouched; no backup written.
	assert.Equal(t, pngBody, store.objects[key])
	_, hasBackup := store.objects[backupKeyFor(key)]
	assert.False(t, hasBackup)
	out := buf.String()
	t.Logf("dry-run transcript:\n%s", out)
	assert.Contains(t, out, "plan ")
	assert.Contains(t, out, key)
	assert.Contains(t, out, "backup="+backupKeyFor(key))
	assert.Contains(t, out, "content-type=image/jpeg")
}

func TestRunBackfill_ApplyBacksUpAndOverwritesJPEG(t *testing.T) {
	store := newFakeStore()
	// Large enough to force resize (max side 1280) so the optimizer emits JPEG
	// rather than keeping the source PNG.
	pngBody := makeLargePNG(t, 1400, 1000)
	key := "menu_items/ai_generated/legacy.png"
	store.objects[key] = pngBody
	store.types[key] = "image/png"

	var buf bytes.Buffer
	summary, err := runBackfill(context.Background(), store, backfillOptions{
		Prefix:      defaultAIGeneratedPrefix,
		Apply:       true,
		Concurrency: 2,
		Out:         &buf,
	})
	require.NoError(t, err)
	assert.Equal(t, "apply", summary.Mode)
	assert.Equal(t, 1, summary.Processed)
	assert.Equal(t, 0, summary.Failed)

	// Backup has original PNG bytes.
	backup := store.objects[backupKeyFor(key)]
	require.NotNil(t, backup)
	assert.Equal(t, pngBody, backup)

	// Live key overwritten with JPEG bytes and matching content-type.
	live := store.objects[key]
	require.NotNil(t, live)
	assert.Equal(t, "image/jpeg", store.types[key])
	require.GreaterOrEqual(t, len(live), 2)
	assert.Equal(t, byte(0xff), live[0])
	assert.Equal(t, byte(0xd8), live[1])
}

// TestRunBackfill_ApplyKeepSourcePNGUsesPNGContentType covers the optimizer
// branch that keeps original PNG bytes when JPEG encode does not shrink and no
// resize is needed (flat small PNG). The live object Content-Type must be
// image/png, not a hard-coded image/jpeg that would mislabel the bytes.
func TestRunBackfill_ApplyKeepSourcePNGUsesPNGContentType(t *testing.T) {
	store := newFakeStore()
	// Flat 200×200 PNG: no resize; JPEG often larger → keep-source PNG path.
	pngBody := makeFlatPNG(t, 200, 200)
	key := "menu_items/ai_generated/flat.png"
	store.objects[key] = pngBody
	store.types[key] = "image/png"

	// Confirm optimizer keep-source path on this fixture (drives the shipped path).
	opt, err := services.OptimizeAIGeneratedImageBytes(pngBody)
	require.NoError(t, err)
	require.Equal(t, "image/png", opt.MIMEType, "fixture must hit keep-source PNG branch")
	require.Equal(t, pngBody, opt.Bytes)

	var buf bytes.Buffer
	summary, err := runBackfill(context.Background(), store, backfillOptions{
		Prefix:      defaultAIGeneratedPrefix,
		Apply:       true,
		Concurrency: 1,
		Out:         &buf,
	})
	require.NoError(t, err)
	assert.Equal(t, 1, summary.Processed)
	assert.Equal(t, 0, summary.Failed)

	// Live key must retain PNG bytes with Content-Type image/png.
	live := store.objects[key]
	require.NotNil(t, live)
	assert.Equal(t, pngBody, live)
	assert.Equal(t, "image/png", store.types[key], "must not mislabel kept PNG as image/jpeg")
	assert.Contains(t, buf.String(), "content-type=image/png")
}

func TestRunBackfill_IdempotentSkipAfterOptimize(t *testing.T) {
	store := newFakeStore()
	// Seed a small JPEG under threshold.
	jpegish := []byte{
		0xff, 0xd8, 0xff, 0xe0, 0x00, 0x10, 0x4a, 0x46, 0x49, 0x46, 0x00, 0x01,
		0x01, 0x00, 0x00, 0x01, 0x00, 0x01, 0x00, 0x00, 0xff, 0xd9,
	}
	body := make([]byte, 50_000)
	copy(body, jpegish)
	key := "menu_items/ai_generated/already.jpg"
	store.objects[key] = body

	summary, err := runBackfill(context.Background(), store, backfillOptions{
		Prefix: defaultAIGeneratedPrefix, Apply: true, Concurrency: 1, Out: &bytes.Buffer{},
	})
	require.NoError(t, err)
	assert.Equal(t, 1, summary.Skipped)
	assert.Equal(t, 0, summary.Processed)
}

func TestRunBackfill_ContinuesOnPerObjectError(t *testing.T) {
	store := newFakeStore()
	goodKey := "menu_items/ai_generated/good.png"
	badKey := "menu_items/ai_generated/bad.png"
	store.objects[goodKey] = makeLargePNG(t, 1400, 900)
	store.objects[badKey] = makeLargePNG(t, 1400, 900)
	store.failGet[badKey] = true

	summary, err := runBackfill(context.Background(), store, backfillOptions{
		Prefix: defaultAIGeneratedPrefix, Apply: false, Concurrency: 2, Out: &bytes.Buffer{},
	})
	require.NoError(t, err)
	assert.Equal(t, 1, summary.Processed)
	assert.Equal(t, 1, summary.Failed)
}

func TestLoadPublicBucketConfig(t *testing.T) {
	_, err := loadPublicBucketConfig(func(string) string { return "" })
	require.Error(t, err)
	assert.Contains(t, err.Error(), "S3_BUCKET")

	cfg, err := loadPublicBucketConfig(func(k string) string {
		return map[string]string{
			"S3_BUCKET":      "pv-images",
			"AWS_ACCESS_KEY": "AKIA",
			"AWS_SECRET_KEY": "secret",
			"AWS_REGION":     "eu-west-1",
		}[k]
	})
	require.NoError(t, err)
	assert.Equal(t, "pv-images", cfg.name)
	assert.Equal(t, "eu-west-1", cfg.region)
	assert.NotContains(t, cfg.summary(), "secret")
}

func makeLargePNG(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	// Non-uniform pixels so PNG does not compress to almost nothing.
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, color.RGBA{R: uint8(x), G: uint8(y), B: uint8(x + y), A: 255})
		}
	}
	var buf bytes.Buffer
	require.NoError(t, png.Encode(&buf, img))
	return buf.Bytes()
}

// makeFlatPNG encodes a solid-color PNG that compresses tightly so the
// optimizer's keep-source branch (JPEG does not shrink, no resize) is reliable.
func makeFlatPNG(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, color.RGBA{R: 240, G: 240, B: 240, A: 255})
		}
	}
	var buf bytes.Buffer
	require.NoError(t, png.Encode(&buf, img))
	return buf.Bytes()
}
