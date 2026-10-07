package s3

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// Environment variables owned by the storage layer. The S3_* / AWS_* bucket
// and credential variables keep their existing names and are resolved by
// cmd/app (flags > env) into S3Settings.
const (
	EnvStorageDriver    = "STORAGE_DRIVER"
	EnvStorageDir       = "STORAGE_DIR"
	EnvS3ForcePathStyle = "S3_FORCE_PATH_STYLE"
	EnvPublicURL        = "PUBLIC_URL"
)

// DefaultStorageDirProduction is the container volume mount point.
const DefaultStorageDirProduction = "/data/storage"

// DefaultStorageDirDevelopment is relative to the backend working directory.
const DefaultStorageDirDevelopment = "./data/storage"

// ErrProtectedExposed means the startup probe read a protected object
// without credentials.
var ErrProtectedExposed = errors.New("storage: protected objects are anonymously readable")

// Config is the storage wiring for one process.
type Config struct {
	Driver     string
	Dir        string
	PublicURL  string
	Production bool
	S3         S3Settings
	// Warnings found while reading the environment; Init reports them.
	Warnings []string
}

// Report summarizes what Init configured, for startup logging.
type Report struct {
	Driver    string
	Dir       string
	PublicURL string
	Warnings  []string
	Probe     *ProbeResult
}

// DefaultStorageDir returns the STORAGE_DIR default for the mode.
func DefaultStorageDir(production bool) string {
	if production {
		return DefaultStorageDirProduction
	}
	return DefaultStorageDirDevelopment
}

// ResolvePublicURL returns the absolute public origin used to build media
// URLs: PUBLIC_URL when it is an absolute http(s) origin, else "" (relative
// "/media/<key>" URLs). APP_BASE_URL is deliberately NOT consulted: media URLs are persisted in rows, and an
// implicit absolute origin would pin them to whatever host was configured at
// upload time. Relative URLs survive a host change; operators who need
// absolute URLs opt in with PUBLIC_URL.
func ResolvePublicURL() string {
	return normalizePublicURL(os.Getenv(EnvPublicURL))
}

func normalizePublicURL(raw string) string {
	raw = strings.TrimRight(strings.TrimSpace(raw), "/")
	if raw == "" {
		return ""
	}
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return ""
	}
	return raw
}

// ConfigFromEnv reads STORAGE_DRIVER (local|s3, default local), STORAGE_DIR,
// S3_FORCE_PATH_STYLE and the public URL, layering them over the already
// resolved S3 settings.
func ConfigFromEnv(production bool, settings S3Settings) (Config, error) {
	driver := strings.ToLower(strings.TrimSpace(os.Getenv(EnvStorageDriver)))
	if driver == "" {
		driver = DriverLocal
	}
	if driver != DriverLocal && driver != DriverS3 {
		return Config{}, fmt.Errorf("%s=%q is not supported (use %q or %q)", EnvStorageDriver, driver, DriverLocal, DriverS3)
	}
	dir := strings.TrimSpace(os.Getenv(EnvStorageDir))
	if dir == "" {
		dir = DefaultStorageDir(production)
	}
	if raw := strings.TrimSpace(os.Getenv(EnvS3ForcePathStyle)); raw != "" {
		v, err := strconv.ParseBool(raw)
		if err != nil {
			return Config{}, fmt.Errorf("%s=%q is not a boolean", EnvS3ForcePathStyle, raw)
		}
		settings.ForcePathStyle = v
	}
	cfg := Config{
		Driver:     driver,
		Dir:        dir,
		PublicURL:  ResolvePublicURL(),
		Production: production,
		S3:         settings,
	}
	if raw := strings.TrimSpace(os.Getenv(EnvPublicURL)); raw != "" && cfg.PublicURL == "" {
		cfg.Warnings = append(cfg.Warnings, fmt.Sprintf(
			"%s is not an absolute http(s) origin without credentials, query or fragment; media URLs stay relative (/media/<key>)",
			EnvPublicURL))
	}
	return cfg, nil
}

func isPlaceholder(v string) bool {
	v = strings.ToLower(strings.TrimSpace(v))
	return v == "" || strings.HasPrefix(v, "replace_with") || strings.HasPrefix(v, "your-") ||
		strings.HasPrefix(v, "your_") || strings.HasPrefix(v, "changeme") || strings.HasPrefix(v, "<")
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

// Init installs the public and protected stores. For the s3 driver it also
// runs the protected-exposure probe; an exposed protected store is an error
// in production and a warning elsewhere.
func Init(ctx context.Context, cfg Config) (Report, error) {
	rep := Report{Driver: cfg.Driver, PublicURL: cfg.PublicURL}
	rep.Warnings = append(rep.Warnings, cfg.Warnings...)
	switch cfg.Driver {
	case "", DriverLocal:
		rep.Driver = DriverLocal
		dir := cfg.Dir
		if dir == "" {
			dir = DefaultStorageDir(cfg.Production)
		}
		abs, err := filepath.Abs(dir)
		if err != nil {
			return rep, fmt.Errorf("storage: resolve %s=%q: %w", EnvStorageDir, dir, err)
		}
		rep.Dir = abs
		pub, err := NewLocalStore(filepath.Join(abs, "public"))
		if err != nil {
			return rep, err
		}
		prot, err := NewLocalStore(filepath.Join(abs, "protected"))
		if err != nil {
			return rep, err
		}
		cdn := strings.TrimRight(strings.TrimSpace(cfg.S3.PublicBaseURL), "/")
		installState(storageState{
			public:    pub,
			protected: prot,
		}, cfg.PublicURL)
		// Rows written while the instance ran on S3 keep their CDN URLs; keep
		// trusting that host for reads (AI waiter images, enhancement
		// sources) without routing new writes there.
		if cdn != "" && !isPlaceholder(cdn) {
			configurePublicHost(cfg.S3.Bucket, cfg.S3.Region, cfg.S3.Endpoint, cdn)
		} else {
			setPublicHost("")
		}
		if !isPlaceholder(cfg.S3.Bucket) {
			rep.Warnings = append(rep.Warnings, fmt.Sprintf(
				"S3_BUCKET is set but %s=local: new uploads are written under %s; set %s=s3 to keep using the bucket",
				EnvStorageDriver, abs, EnvStorageDriver))
		}
		return rep, nil

	case DriverS3:
		st := cfg.S3
		pubClient, err := newS3Client(st.Bucket, st.AccessKey, st.SecretKey, st.Region, st.Endpoint, st.ForcePathStyle)
		if err != nil {
			return rep, fmt.Errorf("storage: public S3 store: %w", err)
		}
		var pub, prot *S3Store
		shared := strings.TrimSpace(st.ProtectedBucket) == ""
		if shared {
			pub = newS3Store(pubClient, st.Bucket, "", SharedBucketProtectedPrefix)
			prot = newS3Store(pubClient, st.Bucket, SharedBucketProtectedPrefix)
			rep.Warnings = append(rep.Warnings, fmt.Sprintf(
				"S3_PROTECTED_BUCKET is empty: protected objects share bucket %q under %q; the bucket must not allow anonymous reads",
				st.Bucket, SharedBucketProtectedPrefix))
		} else {
			protClient, err := newS3Client(
				st.ProtectedBucket,
				firstNonEmpty(st.ProtectedAccessKey, st.AccessKey),
				firstNonEmpty(st.ProtectedSecretKey, st.SecretKey),
				st.Region,
				firstNonEmpty(st.ProtectedEndpoint, st.Endpoint),
				st.ForcePathStyle,
			)
			if err != nil {
				return rep, fmt.Errorf("storage: protected S3 store: %w", err)
			}
			pub = newS3Store(pubClient, st.Bucket, "")
			prot = newS3Store(protClient, st.ProtectedBucket, "")
		}
		cdn := strings.TrimRight(strings.TrimSpace(st.PublicBaseURL), "/")
		endpoint := normalizeEndpoint(st.Endpoint)
		installState(storageState{
			public:           pub,
			protected:        prot,
			cdnBaseURL:       cdn,
			protectedBaseURL: strings.TrimRight(strings.TrimSpace(st.ProtectedBaseURL), "/"),
			s3EndpointHost:   hostOf(endpoint),
			s3Bucket:         st.Bucket,
		}, cfg.PublicURL)
		configurePublicHostStyle(st.Bucket, st.Region, endpoint, cdn, st.ForcePathStyle)

		extra := []string{st.ProtectedBaseURL}
		if shared {
			extra = append(extra, cdn)
		}
		res := probeProtectedExposure(ctx, prot, extra, newProbeHTTPClient())
		rep.Probe = &res
		for _, msg := range res.Inconclusive {
			rep.Warnings = append(rep.Warnings, "protected-storage probe inconclusive: "+msg)
		}
		if len(res.Exposed) > 0 {
			err := fmt.Errorf("%w via %s", ErrProtectedExposed, strings.Join(res.Exposed, ", "))
			if cfg.Production {
				return rep, err
			}
			rep.Warnings = append(rep.Warnings, err.Error())
		}
		if cdn == "" {
			if w := directBucketURLWarning(ctx, pub, cfg.PublicURL, newProbeHTTPClient()); w != "" {
				rep.Warnings = append(rep.Warnings, w)
			}
		}
		return rep, nil
	}
	return rep, fmt.Errorf("%s=%q is not supported", EnvStorageDriver, cfg.Driver)
}

// directBucketSampleSize bounds the one ListObjectsV2 page the direct-bucket
// check reads; keys under reserved prefixes are skipped, so take a few.
const directBucketSampleSize = 20

// directBucketURLWarning explains the one URL-shape change the s3 driver has
// for existing installs. Before pluggable storage, an empty S3_PUBLIC_BASE_URL
// meant "persist the bucket's own object URL" (e.g. iDrive e2
// https://<bucket>.<endpoint>/<key>), which only works for an anonymously
// readable bucket. Now it means "serve through the backend at /media/<key>",
// which also works for private buckets. When the public bucket IS anonymously
// readable, the operator almost certainly relied on the old behavior, so name
// the exact S3_PUBLIC_BASE_URL that restores it. Private buckets (where /media
// is the only option) and inconclusive checks stay quiet.
//
// The check is read-only so restarts never leave objects behind on write-once
// (object-lock) buckets or under policies that deny DeleteObject: it lists one
// page of existing public objects and HEADs one anonymously. An empty bucket
// (a fresh install has no old URLs to keep) or a denied list stays quiet.
func directBucketURLWarning(ctx context.Context, pub *S3Store, publicURL string, client httpDoer) string {
	keys, err := pub.sampleObjectKeys(ctx, directBucketSampleSize)
	if err != nil || len(keys) == 0 {
		return ""
	}
	// Bucket access is policy-wide (uploads never set per-object ACLs), so
	// one object answers for the bucket.
	if !pub.anonymouslyReadable(ctx, keys[0], client) {
		return ""
	}
	base, err := pub.anonymousBaseURL(ctx)
	if err != nil {
		return ""
	}
	return fmt.Sprintf(
		"S3_PUBLIC_BASE_URL is empty, so new public objects are proxied by the backend at %s%s<key>. "+
			"Bucket %q is anonymously readable: to keep persisting direct bucket URLs as before this release "+
			"(no proxy hop), set S3_PUBLIC_BASE_URL=%s. Existing rows keep working either way.",
		publicURL, MediaPathPrefix, pub.bucket, base)
}

// installState replaces the whole storage wiring (Init is the only caller in
// production; it runs once at startup).
func installState(next storageState, publicURL string) {
	publicURL = normalizePublicURL(publicURL)
	next.publicURL = publicURL
	next.publicURLHost = hostOf(publicURL)
	next.publicURLPath = pathOf(publicURL)
	stateMu.Lock()
	state = next
	stateMu.Unlock()
}

func pathOf(raw string) string {
	if raw == "" {
		return ""
	}
	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	return strings.TrimRight(u.Path, "/")
}
