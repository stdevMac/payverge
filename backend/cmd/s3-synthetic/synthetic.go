package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/google/uuid"
	storage "github.com/stdevmac/payverge/backend/internal/s3"
)

// boundedHTTPClient mirrors internal/s3.boundedS3HTTPClient: the SDK sets no
// overall operation deadline, and a stalled DNS/dial/TLS or slow first byte
// against a third-party S3-compatible store could otherwise hang forever.
func boundedHTTPClient() *http.Client {
	return &http.Client{
		Timeout: 30 * time.Second,
		Transport: &http.Transport{
			DialContext:           (&net.Dialer{Timeout: 5 * time.Second, KeepAlive: 30 * time.Second}).DialContext,
			TLSHandshakeTimeout:   5 * time.Second,
			ResponseHeaderTimeout: 15 * time.Second,
			ExpectContinueTimeout: 1 * time.Second,
			IdleConnTimeout:       90 * time.Second,
			MaxIdleConns:          100,
			MaxIdleConnsPerHost:   10,
		},
	}
}

// syntheticPrefix is the key namespace every probe object lives under so an
// operator can identify (and, if a probe ever crashes mid-run, sweep) synthetic
// objects. Kept deliberately narrow and obvious.
const syntheticPrefix = "synthetic/s3-probe/"

// objectStore is the minimal S3 surface the roundtrip probe needs. The real
// implementation wraps aws-sdk-go-v2; unit tests use an in-memory fake.
type objectStore interface {
	Put(ctx context.Context, key string, body []byte) error
	Get(ctx context.Context, key string) ([]byte, error)
	Delete(ctx context.Context, key string) error
}

// bucketConfig is the resolved, validated configuration for one bucket.
// It intentionally carries credentials but never exposes them via summary().
type bucketConfig struct {
	label     string // "public" | "protected" — for messages, never a secret
	name      string
	accessKey string
	secretKey string
	region    string
	endpoint  string // optional (custom S3-compatible endpoint)
}

// summary renders a human-safe, secret-free description for logs.
func (c bucketConfig) summary() string {
	ep := c.endpoint
	if ep == "" {
		ep = "(aws default)"
	}
	return fmt.Sprintf("bucket=%s label=%s region=%s endpoint=%s", c.name, c.label, c.region, ep)
}

// envVarNames returns the env var names for a bucket label. It is the single
// source of truth so error messages and reads never drift apart.
func envVarNames(label string) (bucketVar, accessVar, secretVar, endpointVar string, err error) {
	switch label {
	case "public":
		return "S3_BUCKET", "AWS_ACCESS_KEY", "AWS_SECRET_KEY", "S3_ENDPOINT", nil
	case "protected":
		return "S3_PROTECTED_BUCKET", "AWS_PROTECTED_ACCESS_KEY", "AWS_PROTECTED_SECRET_KEY", "S3_PROTECTED_ENDPOINT", nil
	default:
		return "", "", "", "", fmt.Errorf("unknown bucket label %q", label)
	}
}

// loadBucketConfig resolves and validates config for the named bucket from the
// provided getenv. It reports MISSING VARIABLE NAMES on error and never places a
// secret value into the returned error. Region falls back to us-east-1 (the same
// default the backend's --aws-region flag uses); a custom endpoint is optional.
func loadBucketConfig(label string, getenv func(string) string) (bucketConfig, error) {
	bucketVar, accessVar, secretVar, endpointVar, err := envVarNames(label)
	if err != nil {
		return bucketConfig{}, err
	}
	trim := func(k string) string { return strings.TrimSpace(getenv(k)) }

	cfg := bucketConfig{
		label:     label,
		name:      trim(bucketVar),
		accessKey: trim(accessVar),
		secretKey: trim(secretVar),
		region:    trim("AWS_REGION"),
		endpoint:  trim(endpointVar),
	}
	if cfg.region == "" {
		cfg.region = "us-east-1"
	}
	cfg.region = storage.NormalizeRegionForEndpoint(cfg.region, cfg.endpoint)

	var missing []string
	if cfg.name == "" {
		missing = append(missing, bucketVar)
	}
	if cfg.accessKey == "" {
		missing = append(missing, accessVar)
	}
	if cfg.secretKey == "" {
		missing = append(missing, secretVar)
	}
	if len(missing) > 0 {
		return bucketConfig{}, fmt.Errorf("%s bucket config incomplete; set: %s", label, strings.Join(missing, ", "))
	}
	if cfg.endpoint != "" {
		parsed, parseErr := url.Parse(cfg.endpoint)
		if parseErr != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
			return bucketConfig{}, fmt.Errorf("%s bucket config invalid; %s must be an absolute HTTP(S) URL", label, endpointVar)
		}
	}
	return cfg, nil
}

// selectBuckets maps the --bucket flag to the ordered set of bucket labels.
func selectBuckets(sel string) ([]string, error) {
	switch sel {
	case "both", "":
		return []string{"public", "protected"}, nil
	case "public":
		return []string{"public"}, nil
	case "protected":
		return []string{"protected"}, nil
	default:
		return nil, fmt.Errorf("--bucket must be public|protected|both, got %q", sel)
	}
}

// generateKey returns a unique, prefixed key for one probe object.
func generateKey() string {
	return syntheticPrefix + uuid.NewString() + ".txt"
}

// roundtrip performs Put -> Get -> verify -> Delete against store. Cleanup
// (Delete) always runs after a successful Put, even when Get or verification
// fails, so a probe never leaves synthetic objects behind. If the primary
// path succeeds but cleanup fails, that is surfaced (a leaked object is a real
// operational concern); if the primary path fails, that error is returned and
// the delete is best-effort.
func roundtrip(ctx context.Context, store objectStore, key string, payload []byte) error {
	if err := store.Put(ctx, key, payload); err != nil {
		return fmt.Errorf("put %q: %w", key, err)
	}

	primary := verifyGet(ctx, store, key, payload)

	delErr := store.Delete(ctx, key)
	if primary != nil {
		// Primary failure is the more actionable signal; delete was best-effort.
		return primary
	}
	if delErr != nil {
		return fmt.Errorf("cleanup: failed to delete synthetic object %q: %w", key, delErr)
	}
	return nil
}

func verifyGet(ctx context.Context, store objectStore, key string, payload []byte) error {
	got, err := store.Get(ctx, key)
	if err != nil {
		return fmt.Errorf("get %q: %w", key, err)
	}
	if !bytes.Equal(got, payload) {
		return fmt.Errorf("roundtrip payload mismatch for %q: wrote %d bytes, read %d bytes", key, len(payload), len(got))
	}
	return nil
}

// --- real aws-sdk-go-v2 backed objectStore ---

type s3ObjectStore struct {
	client *s3.Client
	bucket string
}

// newObjectStore builds a live S3 client for cfg, mirroring the bounded HTTP
// client and static-credentials/endpoint-resolver shape used by internal/s3.
func newObjectStore(ctx context.Context, cfg bucketConfig) (*s3ObjectStore, error) {
	opts := []func(*awsconfig.LoadOptions) error{
		awsconfig.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(cfg.accessKey, cfg.secretKey, "")),
		awsconfig.WithRegion(cfg.region),
		awsconfig.WithHTTPClient(boundedHTTPClient()),
	}
	if cfg.endpoint != "" {
		resolver := aws.EndpointResolverWithOptionsFunc(func(service, region string, _ ...interface{}) (aws.Endpoint, error) {
			return aws.Endpoint{URL: cfg.endpoint, SigningRegion: region}, nil
		})
		opts = append(opts, awsconfig.WithEndpointResolverWithOptions(resolver))
	}
	awsCfg, err := awsconfig.LoadDefaultConfig(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("load aws config: %w", err)
	}
	// Build the client exactly like internal/s3 (Init) does — from config
	// only, WITHOUT forcing UsePathStyle. The production client uses the SDK's
	// default virtual-host addressing (bucket in the host) against the same
	// custom endpoint; forcing path-style here would exercise a different
	// request path and could false-pass/false-fail relative to the real app.
	client := s3.NewFromConfig(awsCfg)
	return &s3ObjectStore{client: client, bucket: cfg.name}, nil
}

func (s *s3ObjectStore) Put(ctx context.Context, key string, body []byte) error {
	_, err := s.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:      aws.String(s.bucket),
		Key:         aws.String(key),
		Body:        bytes.NewReader(body),
		ContentType: aws.String("text/plain"),
	})
	return err
}

func (s *s3ObjectStore) Get(ctx context.Context, key string) ([]byte, error) {
	out, err := s.client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return nil, err
	}
	defer func() { _ = out.Body.Close() }()
	return io.ReadAll(out.Body)
}

func (s *s3ObjectStore) Delete(ctx context.Context, key string) error {
	_, err := s.client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	})
	return err
}

// probePayload is the content written and verified for a probe. It embeds the
// key and a timestamp so a stray object is self-describing.
func probePayload(key string, now time.Time) []byte {
	return []byte(fmt.Sprintf("payverge s3 synthetic probe\nkey=%s\nutc=%s\n", key, now.UTC().Format(time.RFC3339)))
}
