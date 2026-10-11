package s3

import (
	"bytes"
	"context"
	"errors"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// snapshotState saves and restores the whole package wiring (stores, URLs,
// publicHost) around a test that calls Init.
func snapshotState(t *testing.T) {
	t.Helper()
	stateMu.Lock()
	prev, prevHost := state, publicHost
	stateMu.Unlock()
	t.Cleanup(func() {
		stateMu.Lock()
		state, publicHost = prev, prevHost
		stateMu.Unlock()
	})
}

func TestConfigFromEnv(t *testing.T) {
	t.Setenv(EnvStorageDriver, "")
	t.Setenv(EnvStorageDir, "")
	t.Setenv(EnvS3ForcePathStyle, "")
	t.Setenv(EnvPublicURL, "")

	cfg, err := ConfigFromEnv(false, S3Settings{})
	require.NoError(t, err)
	require.Equal(t, DriverLocal, cfg.Driver, "local is the zero-config default")
	require.Equal(t, DefaultStorageDirDevelopment, cfg.Dir)
	require.Empty(t, cfg.PublicURL)

	cfg, err = ConfigFromEnv(true, S3Settings{})
	require.NoError(t, err)
	require.Equal(t, DefaultStorageDirProduction, cfg.Dir)

	t.Setenv(EnvStorageDriver, " S3 ")
	t.Setenv(EnvStorageDir, "/srv/storage")
	t.Setenv(EnvS3ForcePathStyle, "true")
	for _, other := range []string{"FRONTEND_URL", "BASE_URL", "APP_BASE_URL"} {
		t.Setenv(other, "https://other.example.test/")
	}
	cfg, err = ConfigFromEnv(true, S3Settings{Bucket: "b"})
	require.NoError(t, err)
	require.Equal(t, DriverS3, cfg.Driver)
	require.Equal(t, "/srv/storage", cfg.Dir)
	require.True(t, cfg.S3.ForcePathStyle)
	require.Equal(t, "b", cfg.S3.Bucket)
	require.Empty(t, cfg.PublicURL, "FRONTEND_URL/BASE_URL/APP_BASE_URL never make media URLs absolute")
	require.Empty(t, cfg.Warnings)

	t.Setenv(EnvPublicURL, "https://pos.example.test/")
	cfg, err = ConfigFromEnv(true, S3Settings{})
	require.NoError(t, err)
	require.Equal(t, "https://pos.example.test", cfg.PublicURL, "PUBLIC_URL is the only absolute-origin opt-in")

	t.Setenv(EnvPublicURL, "pos.example.test")
	cfg, err = ConfigFromEnv(true, S3Settings{})
	require.NoError(t, err)
	require.Empty(t, cfg.PublicURL)
	require.Len(t, cfg.Warnings, 1, "a malformed PUBLIC_URL is reported, not silently dropped")
	require.Contains(t, cfg.Warnings[0], EnvPublicURL)
	t.Setenv(EnvPublicURL, "")

	t.Setenv(EnvStorageDriver, "gcs")
	_, err = ConfigFromEnv(false, S3Settings{})
	require.ErrorContains(t, err, EnvStorageDriver)

	t.Setenv(EnvStorageDriver, "local")
	t.Setenv(EnvS3ForcePathStyle, "sometimes")
	_, err = ConfigFromEnv(false, S3Settings{})
	require.ErrorContains(t, err, EnvS3ForcePathStyle)
}

func TestInitLocalNeedsNothingS3(t *testing.T) {
	snapshotState(t)
	dir := filepath.Join(t.TempDir(), "storage")
	rep, err := Init(context.Background(), Config{Driver: DriverLocal, Dir: dir, PublicURL: "https://pos.example.test"})
	require.NoError(t, err)
	require.Equal(t, DriverLocal, rep.Driver)
	require.Equal(t, dir, rep.Dir)
	require.Empty(t, rep.Warnings)
	require.Equal(t, DriverLocal, ActiveDriver())
	require.Equal(t, "https://pos.example.test/media/a.png", PublicURL("a.png"))
	require.Empty(t, PublicHost(), "no S3 host is trusted for server-side fetches")
	for _, sub := range []string{"public", "protected"} {
		fi, err := os.Stat(filepath.Join(dir, sub))
		require.NoError(t, err)
		require.True(t, fi.IsDir())
	}
	require.NotNil(t, ProtectedStore())
	require.NotSame(t, PublicStore(), ProtectedStore())
}

// TestMediaURLsStayRelativeWithoutPublicURL pins the persisted URL shape for
// the zero-config install: legacy origin variables set (as in the shipped
// compose), PUBLIC_URL unset -> "/media/<key>", which survives a host change.
func TestMediaURLsStayRelativeWithoutPublicURL(t *testing.T) {
	snapshotState(t)
	t.Setenv(EnvStorageDriver, "")
	t.Setenv(EnvStorageDir, t.TempDir())
	t.Setenv(EnvS3ForcePathStyle, "")
	t.Setenv(EnvPublicURL, "")
	t.Setenv("FRONTEND_URL", "http://localhost:3000")
	t.Setenv("BASE_URL", "https://pos.example.test")
	t.Setenv("APP_BASE_URL", "https://app.example.test")
	cfg, err := ConfigFromEnv(true, S3Settings{})
	require.NoError(t, err)
	_, err = Init(context.Background(), cfg)
	require.NoError(t, err)

	location, err := UploadBytes([]byte("png"), "a.png", "businesses/1/menu_items", "image/png")
	require.NoError(t, err)
	require.Equal(t, "/media/businesses/1/menu_items/a.png", location)
	require.True(t, IsOwnMediaURL(location))
	require.False(t, IsOwnMediaURL("http://localhost:3000/media/businesses/1/menu_items/a.png"),
		"FRONTEND_URL is not an own-media origin")
}

func TestInitLocalKeepsTrustingLegacyCDNHostAndWarnsAboutBucket(t *testing.T) {
	snapshotState(t)
	rep, err := Init(context.Background(), Config{
		Driver: DriverLocal,
		Dir:    t.TempDir(),
		S3:     S3Settings{Bucket: "payverge-prod", PublicBaseURL: "https://images.example.test"},
	})
	require.NoError(t, err)
	require.Equal(t, "images.example.test", PublicHost(), "old rows on the CDN stay fetchable")
	require.Equal(t, "/media/a.png", PublicURL("a.png"), "new uploads go to /media, not the bucket")
	require.Len(t, rep.Warnings, 1)
	require.Contains(t, rep.Warnings[0], "S3_BUCKET")
}

func TestInitLocalUnwritableDirIsAnError(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root can write anywhere")
	}
	snapshotState(t)
	parent := t.TempDir()
	require.NoError(t, os.Chmod(parent, 0o500))
	t.Cleanup(func() { _ = os.Chmod(parent, 0o700) })
	_, err := Init(context.Background(), Config{Driver: DriverLocal, Dir: filepath.Join(parent, "storage")})
	require.Error(t, err)
}

func TestInitS3PrivateBucketPassesProbe(t *testing.T) {
	snapshotState(t)
	fake := newFakeS3(t)
	// An existing upload makes the direct-bucket check run (and be denied).
	fake.seed("/pub/businesses/1/logo.png", []byte("png"))
	rep, err := Init(context.Background(), Config{
		Driver:     DriverS3,
		Production: true,
		PublicURL:  "https://pos.example.test",
		S3: S3Settings{
			Bucket: "pub", AccessKey: "ak", SecretKey: "sk", Region: "us-east-1",
			Endpoint: fake.URL(), ProtectedBucket: "prot", ForcePathStyle: true,
		},
	})
	require.NoError(t, err)
	require.NotNil(t, rep.Probe)
	require.Empty(t, rep.Probe.Exposed)
	require.Empty(t, rep.Probe.Inconclusive)
	require.Equal(t, 2, fake.anonGets, "one anonymous GET of the protected probe, one anonymous HEAD of a public object")
	require.Equal(t, 1, fake.count(), "the protected probe object is cleaned up; the existing upload stays")
	requireNoPublicBucketWrites(t, fake, "pub")
	require.NotContains(t, strings.Join(rep.Warnings, "\n"), "S3_PUBLIC_BASE_URL=",
		"a private bucket has no direct-URL alternative to suggest")
	require.Equal(t, DriverS3, ActiveDriver())
	require.Equal(t, "https://pos.example.test/media/k.png", PublicURL("k.png"), "no CDN: served through /media")
	require.Equal(t, strings.TrimPrefix(fake.URL(), "http://"), PublicHost(), "path-style: the endpoint host")

	// Package wrappers go through the S3 stores.
	loc, err := UploadBytesProtected([]byte("%PDF"), "r.pdf", "fiscal-receipts/1/2", "application/pdf")
	require.NoError(t, err)
	require.Equal(t, "fiscal-receipts/1/2/r.pdf", loc)
	require.True(t, fake.has("/prot/fiscal-receipts/1/2/r.pdf"))
	got, err := DownloadFileProtected(loc)
	require.NoError(t, err)
	require.Equal(t, "%PDF", string(got))
}

// TestInitS3PublicBucketWithoutCDNNamesLegacyBase covers the upgrade path of
// the documented "leave S3_PUBLIC_BASE_URL empty" config (iDrive e2 / AWS with
// a public-read bucket): uploads move to /media, and startup names the exact
// S3_PUBLIC_BASE_URL that restores the old direct bucket URLs.
func TestInitS3PublicBucketWithoutCDNNamesLegacyBase(t *testing.T) {
	snapshotState(t)
	fake := newFakeS3(t)
	fake.publicBuckets = map[string]bool{"pub": true}
	fake.seed("/pub/businesses/1/logo.png", []byte("png"))
	rep, err := Init(context.Background(), Config{
		Driver:     DriverS3,
		Production: true,
		S3: S3Settings{
			Bucket: "pub", AccessKey: "ak", SecretKey: "sk", Region: "us-east-1",
			Endpoint: fake.URL(), ProtectedBucket: "prot", ForcePathStyle: true,
		},
	})
	require.NoError(t, err, "a public PUBLIC bucket is fine; only protected exposure is fatal")
	require.Empty(t, rep.Probe.Exposed)
	joined := strings.Join(rep.Warnings, "\n")
	require.Contains(t, joined, "S3_PUBLIC_BASE_URL="+fake.URL()+"/pub")
	require.Equal(t, "/media/k.png", PublicURL("k.png"))
	require.Equal(t, 1, fake.count(), "the protected probe object is cleaned up; the existing upload stays")
	requireNoPublicBucketWrites(t, fake, "pub")

	// Following the advice restores the pre-upgrade URL shape and still
	// resolves keys from those URLs.
	snapshotState(t)
	_, err = Init(context.Background(), Config{
		Driver: DriverS3,
		S3: S3Settings{
			Bucket: "pub", AccessKey: "ak", SecretKey: "sk", Region: "us-east-1",
			Endpoint: fake.URL(), ProtectedBucket: "prot", ForcePathStyle: true,
			PublicBaseURL: fake.URL() + "/pub",
		},
	})
	require.NoError(t, err)
	require.Equal(t, fake.URL()+"/pub/k.png", PublicURL("k.png"))
	key, err := KeyFromURL(fake.URL() + "/pub/businesses/1/k.png")
	require.NoError(t, err)
	require.Equal(t, "businesses/1/k.png", key)
}

// TestInitS3DirectBucketCheckNeverWritesThePublicBucket pins the read-only
// direct-bucket check: on an empty bucket (fresh install) or without
// s3:ListBucket it stays quiet, and no restart writes to the public bucket,
// so write-once (object-lock) buckets and Delete-denying policies never
// accumulate probe objects.
func TestInitS3DirectBucketCheckNeverWritesThePublicBucket(t *testing.T) {
	for _, tc := range []struct {
		name     string
		seed     bool
		denyList bool
	}{
		{name: "empty public bucket"},
		{name: "list denied", seed: true, denyList: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			snapshotState(t)
			fake := newFakeS3(t)
			fake.publicBuckets = map[string]bool{"pub": true}
			fake.denyList = tc.denyList
			if tc.seed {
				fake.seed("/pub/businesses/1/logo.png", []byte("png"))
			}
			for range 3 { // restarts
				rep, err := Init(context.Background(), Config{
					Driver:     DriverS3,
					Production: true,
					S3: S3Settings{
						Bucket: "pub", AccessKey: "ak", SecretKey: "sk", Region: "us-east-1",
						Endpoint: fake.URL(), ProtectedBucket: "prot", ForcePathStyle: true,
					},
				})
				require.NoError(t, err)
				require.NotContains(t, strings.Join(rep.Warnings, "\n"), "S3_PUBLIC_BASE_URL=")
			}
			requireNoPublicBucketWrites(t, fake, "pub")
		})
	}
}

func TestSampleObjectKeysSkipsReservedAndFolderKeys(t *testing.T) {
	fake := newFakeS3(t)
	fake.seed("/shared/protected/contracts/1/c.pdf", []byte("pdf"))
	fake.seed("/shared/menu/", nil)
	fake.seed("/shared/businesses/1/logo.png", []byte("png"))
	pub := newFakeS3Store(t, fake, "shared", "", SharedBucketProtectedPrefix)
	keys, err := pub.sampleObjectKeys(context.Background(), 20)
	require.NoError(t, err)
	require.Equal(t, []string{"businesses/1/logo.png"}, keys)

	prot := newFakeS3Store(t, fake, "shared", SharedBucketProtectedPrefix)
	keys, err = prot.sampleObjectKeys(context.Background(), 20)
	require.NoError(t, err)
	require.Equal(t, []string{"contracts/1/c.pdf"}, keys, "keys come back relative to the store prefix")
}

func requireNoPublicBucketWrites(t *testing.T, fake *fakeS3, bucket string) {
	t.Helper()
	for _, w := range fake.writes() {
		_, path, _ := strings.Cut(w, " ")
		require.False(t, strings.HasPrefix(path, "/"+bucket+"/"), "startup wrote to the public bucket: %s", w)
	}
}

func TestDirectBucketBaseVirtualHosted(t *testing.T) {
	client, err := newS3Client("menus", "ak", "sk", "us-east-1", "https://s3.e2.example.test", false)
	require.NoError(t, err)
	base, err := newS3Store(client, "menus", "").anonymousBaseURL(context.Background())
	require.NoError(t, err)
	require.Equal(t, "https://menus.s3.e2.example.test", base, "matches the SDK Location the old code persisted")
}

func TestInitS3ExposedProtectedBucketIsFatalInProduction(t *testing.T) {
	snapshotState(t)
	fake := newFakeS3(t)
	fake.publicRead = true
	cfg := Config{
		Driver:     DriverS3,
		Production: true,
		S3: S3Settings{
			Bucket: "pub", AccessKey: "ak", SecretKey: "sk", Region: "us-east-1",
			Endpoint: fake.URL(), ProtectedBucket: "prot", ForcePathStyle: true,
		},
	}
	rep, err := Init(context.Background(), cfg)
	require.ErrorIs(t, err, ErrProtectedExposed)
	require.NotNil(t, rep.Probe)
	require.NotEmpty(t, rep.Probe.Exposed)
	require.Zero(t, fake.count(), "probe object removed even when exposed")

	// Outside production it is a loud warning, not a crash.
	cfg.Production = false
	rep, err = Init(context.Background(), cfg)
	require.NoError(t, err)
	require.NotEmpty(t, rep.Warnings)
	require.Contains(t, strings.Join(rep.Warnings, "\n"), "anonymously readable")
}

func TestInitS3SharedBucketProbesTheCDNBase(t *testing.T) {
	snapshotState(t)
	fake := newFakeS3(t)
	// The bucket itself is private, but a public CDN/custom domain fronts it
	// (Cloudflare R2 public domain): protected/ objects leak through the CDN.
	var cdnHits []string
	cdn := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cdnHits = append(cdnHits, r.URL.Path)
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(cdn.Close)

	rep, err := Init(context.Background(), Config{
		Driver:     DriverS3,
		Production: true,
		S3: S3Settings{
			Bucket: "shared", AccessKey: "ak", SecretKey: "sk", Region: "us-east-1",
			Endpoint: fake.URL(), PublicBaseURL: cdn.URL, ForcePathStyle: true,
		},
	})
	require.ErrorIs(t, err, ErrProtectedExposed)
	require.Len(t, cdnHits, 1)
	require.True(t, strings.HasPrefix(cdnHits[0], "/protected/storage-probe/"), cdnHits[0])
	require.Contains(t, strings.Join(rep.Warnings, "\n"), "S3_PROTECTED_BUCKET is empty")
}

func TestInitS3NetworkFailureIsInconclusiveNotFatal(t *testing.T) {
	snapshotState(t)
	fake := newFakeS3(t)
	rep, err := Init(context.Background(), Config{
		Driver:     DriverS3,
		Production: true,
		S3: S3Settings{
			Bucket: "pub", AccessKey: "ak", SecretKey: "sk", Region: "us-east-1",
			Endpoint: fake.URL(), ProtectedBucket: "prot", ForcePathStyle: true,
			ProtectedBaseURL: "http://127.0.0.1:1", // nothing listens here
		},
	})
	require.NoError(t, err)
	require.NotEmpty(t, rep.Probe.Inconclusive)
	require.Contains(t, strings.Join(rep.Warnings, "\n"), "probe inconclusive")
}

func TestInitS3RejectsIncompleteConfig(t *testing.T) {
	snapshotState(t)
	_, err := Init(context.Background(), Config{Driver: DriverS3, S3: S3Settings{Region: "us-east-1"}})
	require.ErrorContains(t, err, "bucket name is required")
	_, err = Init(context.Background(), Config{Driver: "gcs"})
	require.Error(t, err)
}

// TestPackageWrappersRoundTripOnLocal covers the helpers the ~15 importers
// call (UploadFile/UploadBytes/DeleteFile, the protected trio) on the local
// driver: upload → serve through /media → delete.
func TestPackageWrappersRoundTripOnLocal(t *testing.T) {
	snapshotState(t)
	_, err := Init(context.Background(), Config{Driver: DriverLocal, Dir: t.TempDir(), PublicURL: "https://pos.example.test"})
	require.NoError(t, err)
	r := mediaRouter()

	// Multipart upload (menu image handler path).
	fh := multipartFile(t, "dish.png", []byte("png-bytes"))
	location, err := UploadFile(fh, "0123456789abcdef_dish.png", "businesses/9/menu_items", WithContentType("image/png"))
	require.NoError(t, err)
	require.Equal(t, "https://pos.example.test/media/businesses/9/menu_items/0123456789abcdef_dish.png", location)
	key, err := KeyFromURL(location)
	require.NoError(t, err)

	w := doMedia(r, http.MethodGet, "/media/"+key, nil)
	require.Equal(t, http.StatusOK, w.Code)
	require.Equal(t, "png-bytes", w.Body.String())
	require.Equal(t, "image/png", w.Header().Get("Content-Type"))

	exists, err := ObjectExists(key)
	require.NoError(t, err)
	require.True(t, exists)
	data, ct, err := DownloadPublicAsset(context.Background(), location)
	require.NoError(t, err, "own /media URLs are read from the store, not fetched")
	require.Equal(t, "png-bytes", string(data))
	require.Equal(t, "image/png", ct)

	require.NoError(t, DeleteFile(key))
	w = doMedia(r, http.MethodGet, "/media/"+key, nil)
	require.Equal(t, http.StatusNotFound, w.Code)
	exists, err = ObjectExists(key)
	require.NoError(t, err)
	require.False(t, exists)

	// Bytes with metadata (AI-generated images).
	location, err = UploadBytesWithMetadata([]byte("webp"), "x.webp", "menu_items/ai_generated/9", "image/webp", map[string]string{"ai-generated": "true"})
	require.NoError(t, err)
	key, _ = KeyFromURL(location)
	info, err := PublicStore().Stat(context.Background(), key)
	require.NoError(t, err)
	require.Equal(t, "true", info.Metadata["ai-generated"])

	// Protected: never under /media, readable only through the helpers.
	fh = multipartFile(t, "contract.pdf", []byte("%PDF-contract"))
	prot, err := UploadFileProtected(fh, "contract.pdf", "businesses/9/contracts", WithContentType("application/pdf"))
	require.NoError(t, err)
	require.Equal(t, "businesses/9/contracts/contract.pdf", prot)
	w = doMedia(r, http.MethodGet, "/media/"+prot, nil)
	require.Equal(t, http.StatusNotFound, w.Code)
	got, err := DownloadFileProtected(prot)
	require.NoError(t, err)
	require.Equal(t, "%PDF-contract", string(got))
	require.NoError(t, DeleteFileProtected(prot))
	_, err = DownloadFileProtected(prot)
	require.True(t, errors.Is(err, ErrNotFound), "got %v", err)

	// The asset reader refuses protected key spaces even via /media URLs.
	_, _, err = DownloadPublicAsset(context.Background(), "/media/fiscal-receipts/1/a.pdf")
	require.ErrorIs(t, err, ErrDisallowedAssetURL)
}

func TestProtectedKeyResolvesLegacyLocations(t *testing.T) {
	snapshotState(t)
	updateState(func(s *storageState) { s.protectedBaseURL = "https://prot.example.test" })
	cases := map[string]string{
		"ai/menu-extraction/1/2/page.png":                                  "ai/menu-extraction/1/2/page.png",
		"/ai/menu-extraction/1/2/page.png":                                 "ai/menu-extraction/1/2/page.png",
		"https://prot.example.test/businesses/1/contracts/c.pdf":           "businesses/1/contracts/c.pdf",
		"https://prot-bucket.s3.amazonaws.com/fiscal-receipts/1/a.pdf?X=1": "fiscal-receipts/1/a.pdf",
	}
	for in, want := range cases {
		got, err := protectedKey(in)
		require.NoError(t, err, in)
		require.Equal(t, want, got, in)
	}
}

func multipartFile(t *testing.T, filename string, data []byte) *multipart.FileHeader {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	part, err := mw.CreateFormFile("file", filename)
	require.NoError(t, err)
	_, err = part.Write(data)
	require.NoError(t, err)
	require.NoError(t, mw.Close())
	req := httptest.NewRequest(http.MethodPost, "/upload", &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	require.NoError(t, req.ParseMultipartForm(1<<20))
	return req.MultipartForm.File["file"][0]
}
