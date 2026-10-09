package s3

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awshttp "github.com/aws/aws-sdk-go-v2/aws/transport/http"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/feature/s3/manager"
	awss3 "github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
)

// SharedBucketProtectedPrefix is the key prefix protected objects get when no
// separate S3_PROTECTED_BUCKET is configured and both stores share one bucket.
// The public store refuses every key under it, /media 404s it, and the startup
// probe verifies it is not anonymously readable.
const SharedBucketProtectedPrefix = "protected/"

// S3Settings is the resolved S3 configuration (flags/env already applied by
// cmd/app). The Protected* fields are optional: an empty ProtectedBucket means
// "same bucket, protected/ prefix", and empty protected credentials/endpoint
// fall back to the public ones.
type S3Settings struct {
	Bucket        string
	AccessKey     string
	SecretKey     string
	Region        string
	Endpoint      string
	PublicBaseURL string

	ProtectedBucket    string
	ProtectedAccessKey string
	ProtectedSecretKey string
	ProtectedEndpoint  string
	ProtectedBaseURL   string

	// ForcePathStyle (S3_FORCE_PATH_STYLE) addresses objects as
	// <endpoint>/<bucket>/<key> — required by MinIO/Garage/SeaweedFS without
	// wildcard DNS.
	ForcePathStyle bool
}

// S3Store is the S3-compatible driver (AWS S3, Cloudflare R2, MinIO, Garage,
// iDrive e2, ...).
type S3Store struct {
	client   *awss3.Client
	uploader *manager.Uploader
	bucket   string
	// prefix is prepended to every key ("protected/" for the protected store
	// in shared-bucket mode).
	prefix string
	// reserved are key prefixes this store refuses (the public store refuses
	// "protected/" in shared-bucket mode).
	reserved []string
}

func (s *S3Store) Driver() string { return DriverS3 }

// Bucket returns the bucket name.
func (s *S3Store) Bucket() string { return s.bucket }

func normalizeEndpoint(endpoint string) string {
	endpoint = strings.TrimRight(strings.TrimSpace(endpoint), "/")
	if endpoint != "" && !strings.HasPrefix(endpoint, "http://") && !strings.HasPrefix(endpoint, "https://") {
		endpoint = "https://" + endpoint
	}
	return endpoint
}

// newS3Client builds an SDK client with the bounded HTTP client, the R2 region
// coercion, an optional custom endpoint and optional path-style addressing.
func newS3Client(bucketName, accessKey, secretKey, region, endpointURL string, forcePathStyle bool) (*awss3.Client, error) {
	if bucketName == "" {
		return nil, fmt.Errorf("bucket name is required")
	}
	if accessKey == "" || secretKey == "" {
		return nil, fmt.Errorf("AWS credentials are required")
	}
	if region == "" {
		return nil, fmt.Errorf("AWS region is required")
	}
	endpointURL = normalizeEndpoint(endpointURL)

	// Cloudflare R2 rejects AWS region names (InvalidRegionName); coerce to
	// "auto" when the endpoint is an R2 host and the region is not R2-accepted.
	region = normalizeRegionForEndpoint(region, endpointURL)

	opts := []func(*config.LoadOptions) error{
		config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(accessKey, secretKey, "")),
		config.WithRegion(region),
		config.WithHTTPClient(boundedS3HTTPClient()),
	}
	if endpointURL != "" {
		resolver := aws.EndpointResolverWithOptionsFunc(func(service, resolveRegion string, options ...interface{}) (aws.Endpoint, error) {
			return aws.Endpoint{URL: endpointURL, SigningRegion: region}, nil
		})
		//nolint:staticcheck // legacy resolver kept: it is what production has always signed with.
		opts = append(opts, config.WithEndpointResolverWithOptions(resolver))
		// Third-party S3 implementations vary in their support for the SDK's
		// default flexible checksums (aws-chunked trailers); only send them
		// when the operation requires one.
		opts = append(opts,
			config.WithRequestChecksumCalculation(aws.RequestChecksumCalculationWhenRequired),
			config.WithResponseChecksumValidation(aws.ResponseChecksumValidationWhenRequired),
		)
	}
	cfg, err := config.LoadDefaultConfig(context.Background(), opts...)
	if err != nil {
		return nil, fmt.Errorf("unable to load SDK config: %v", err)
	}
	return awss3.NewFromConfig(cfg, func(o *awss3.Options) {
		o.UsePathStyle = forcePathStyle
	}), nil
}

func newS3Store(client *awss3.Client, bucketName, prefix string, reserved ...string) *S3Store {
	return &S3Store{
		client:   client,
		uploader: manager.NewUploader(client),
		bucket:   bucketName,
		prefix:   prefix,
		reserved: reserved,
	}
}

func (s *S3Store) fullKey(key string) (string, error) {
	if err := validateLooseKey(key); err != nil {
		return "", err
	}
	for _, r := range s.reserved {
		if strings.HasPrefix(key, r) {
			return "", ErrInvalidKey
		}
	}
	return s.prefix + key, nil
}

func (s *S3Store) Put(ctx context.Context, key string, body io.Reader, opts PutOptions) error {
	full, err := s.fullKey(key)
	if err != nil {
		return err
	}
	if body == nil {
		body = strings.NewReader("")
	}
	input := &awss3.PutObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(full),
		Body:   body,
	}
	if opts.ContentType != "" {
		input.ContentType = aws.String(opts.ContentType)
	}
	if opts.ContentDisposition != "" {
		input.ContentDisposition = aws.String(opts.ContentDisposition)
	}
	if len(opts.Metadata) > 0 {
		input.Metadata = copyMetadata(opts.Metadata)
	}
	if _, err := s.uploader.Upload(ctx, input); err != nil {
		return fmt.Errorf("failed to upload object: %w", err)
	}
	return nil
}

func (s *S3Store) Get(ctx context.Context, key string) (*Object, error) {
	full, err := s.fullKey(key)
	if err != nil {
		return nil, err
	}
	out, err := s.client.GetObject(ctx, &awss3.GetObjectInput{Bucket: aws.String(s.bucket), Key: aws.String(full)})
	if err != nil {
		return nil, mapS3Error(err, "get object")
	}
	info := ObjectInfo{
		Key:                key,
		Size:               aws.ToInt64(out.ContentLength),
		ContentType:        aws.ToString(out.ContentType),
		ContentDisposition: aws.ToString(out.ContentDisposition),
		ETag:               aws.ToString(out.ETag),
		Metadata:           copyMetadata(out.Metadata),
	}
	if out.LastModified != nil {
		info.ModTime = out.LastModified.UTC()
	}
	return &Object{ObjectInfo: info, Body: out.Body}, nil
}

func (s *S3Store) Stat(ctx context.Context, key string) (*ObjectInfo, error) {
	full, err := s.fullKey(key)
	if err != nil {
		return nil, err
	}
	out, err := s.client.HeadObject(ctx, &awss3.HeadObjectInput{Bucket: aws.String(s.bucket), Key: aws.String(full)})
	if err != nil {
		return nil, mapS3Error(err, "head object")
	}
	info := &ObjectInfo{
		Key:                key,
		Size:               aws.ToInt64(out.ContentLength),
		ContentType:        aws.ToString(out.ContentType),
		ContentDisposition: aws.ToString(out.ContentDisposition),
		ETag:               aws.ToString(out.ETag),
		Metadata:           copyMetadata(out.Metadata),
	}
	if out.LastModified != nil {
		info.ModTime = out.LastModified.UTC()
	}
	return info, nil
}

func (s *S3Store) Exists(ctx context.Context, key string) (bool, error) {
	_, err := s.Stat(ctx, key)
	if errors.Is(err, ErrNotFound) {
		return false, nil
	}
	return err == nil, err
}

func (s *S3Store) Delete(ctx context.Context, key string) error {
	full, err := s.fullKey(key)
	if err != nil {
		return err
	}
	if _, err := s.client.DeleteObject(ctx, &awss3.DeleteObjectInput{Bucket: aws.String(s.bucket), Key: aws.String(full)}); err != nil {
		return mapS3Error(err, "delete object")
	}
	return nil
}

func mapS3Error(err error, op string) error {
	var nsk *types.NoSuchKey
	var nf *types.NotFound
	if errors.As(err, &nsk) || errors.As(err, &nf) {
		return ErrNotFound
	}
	var respErr *awshttp.ResponseError
	if errors.As(err, &respErr) && respErr.HTTPStatusCode() == http.StatusNotFound {
		return ErrNotFound
	}
	return fmt.Errorf("failed to %s: %w", op, err)
}

// anonymousObjectURL is the unauthenticated URL of an object in this store, in
// the client's own addressing style (virtual-host or path): a presigned GET
// with its signature query stripped.
func (s *S3Store) anonymousObjectURL(ctx context.Context, key string) (string, error) {
	full, err := s.fullKey(key)
	if err != nil {
		return "", err
	}
	req, err := awss3.NewPresignClient(s.client).PresignGetObject(ctx, &awss3.GetObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(full),
	})
	if err != nil {
		return "", err
	}
	u, err := url.Parse(req.URL)
	if err != nil {
		return "", err
	}
	u.RawQuery = ""
	return u.String(), nil
}

// anonymousBaseURL is the bucket's direct object URL base in the client's
// addressing style ("https://<bucket>.<endpoint>" or "<endpoint>/<bucket>"),
// such that anonymousBaseURL + "/" + key is an object's unauthenticated URL.
// It needs no network: it is derived from a presigned URL.
func (s *S3Store) anonymousBaseURL(ctx context.Context) (string, error) {
	const marker = "storage-base-marker"
	u, err := s.anonymousObjectURL(ctx, marker)
	if err != nil {
		return "", err
	}
	suffix := "/" + s.prefix + marker
	if !strings.HasSuffix(u, suffix) {
		return "", fmt.Errorf("unexpected object url shape %q", redactURL(u))
	}
	return strings.TrimSuffix(u, suffix), nil
}

// sampleObjectKeys returns existing keys of this store (relative to its
// prefix; reserved, invalid and folder-marker keys skipped) from a single
// authenticated ListObjectsV2 page of at most limit entries. It never writes.
func (s *S3Store) sampleObjectKeys(ctx context.Context, limit int32) ([]string, error) {
	out, err := s.client.ListObjectsV2(ctx, &awss3.ListObjectsV2Input{
		Bucket:  aws.String(s.bucket),
		Prefix:  aws.String(s.prefix),
		MaxKeys: aws.Int32(limit),
	})
	if err != nil {
		return nil, mapS3Error(err, "list objects")
	}
	var keys []string
	for _, obj := range out.Contents {
		key := strings.TrimPrefix(aws.ToString(obj.Key), s.prefix)
		if key == "" || strings.HasSuffix(key, "/") {
			continue
		}
		if _, err := s.fullKey(key); err != nil {
			continue
		}
		keys = append(keys, key)
	}
	return keys, nil
}

// anonymouslyReadable reports whether an unauthenticated HEAD of an existing
// object succeeds through the store's own object URL. Errors count as "no".
func (s *S3Store) anonymouslyReadable(ctx context.Context, key string, client httpDoer) bool {
	u, err := s.anonymousObjectURL(ctx, key)
	if err != nil {
		return false
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodHead, u, nil)
	if err != nil {
		return false
	}
	resp, err := client.Do(req)
	if err != nil {
		return false
	}
	_ = resp.Body.Close()
	return resp.StatusCode >= 200 && resp.StatusCode < 300
}

// ProbeResult is the outcome of the protected-store exposure probe.
type ProbeResult struct {
	// Exposed lists URLs that served a protected probe object anonymously.
	Exposed []string
	// Inconclusive explains why the probe could not decide (network, upload).
	Inconclusive []string
}

type httpDoer interface {
	Do(*http.Request) (*http.Response, error)
}

func newProbeHTTPClient() *http.Client {
	return &http.Client{
		Timeout:       10 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
}

// probeProtectedExposure writes a throwaway object to the protected store and
// fetches it anonymously through every URL a misconfigured bucket could serve
// it from: the store's own object URL plus extraBases (S3_PROTECTED_BASE_URL,
// and the public CDN base in shared-bucket mode). Any 2xx means protected
// objects (contracts, fiscal receipts, ledger attachments) are world-readable.
func probeProtectedExposure(ctx context.Context, protected *S3Store, extraBases []string, client httpDoer) ProbeResult {
	var res ProbeResult
	var nonce [8]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		res.Inconclusive = append(res.Inconclusive, "probe nonce: "+err.Error())
		return res
	}
	key := "storage-probe/" + hex.EncodeToString(nonce[:]) + ".txt"
	if err := protected.Put(ctx, key, strings.NewReader("payverge protected-storage probe"), PutOptions{ContentType: "text/plain"}); err != nil {
		res.Inconclusive = append(res.Inconclusive, "probe upload failed: "+err.Error())
		return res
	}
	defer func() {
		_ = protected.Delete(context.Background(), key)
	}()

	var urls []string
	if u, err := protected.anonymousObjectURL(ctx, key); err == nil {
		urls = append(urls, u)
	} else {
		res.Inconclusive = append(res.Inconclusive, "probe url: "+err.Error())
	}
	for _, base := range extraBases {
		base = strings.TrimRight(strings.TrimSpace(base), "/")
		if base != "" {
			urls = append(urls, base+"/"+protected.prefix+key)
		}
	}
	for _, u := range urls {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
		if err != nil {
			res.Inconclusive = append(res.Inconclusive, "probe request: "+err.Error())
			continue
		}
		resp, err := client.Do(req)
		if err != nil {
			res.Inconclusive = append(res.Inconclusive, "anonymous GET "+redactURL(u)+": "+err.Error())
			continue
		}
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
		_ = resp.Body.Close()
		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			res.Exposed = append(res.Exposed, redactURL(u))
		}
	}
	return res
}

// redactURL drops userinfo and query so logs never carry credentials.
func redactURL(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return "<unparseable url>"
	}
	u.User = nil
	u.RawQuery = ""
	return u.String()
}

var _ Store = (*S3Store)(nil)
