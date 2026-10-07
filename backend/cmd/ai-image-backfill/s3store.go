package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

// publicBucketConfig is the resolved public-bucket config (never logs secrets).
type publicBucketConfig struct {
	name      string
	accessKey string
	secretKey string
	region    string
	endpoint  string
}

func (c publicBucketConfig) summary() string {
	ep := c.endpoint
	if ep == "" {
		ep = "(aws default)"
	}
	return fmt.Sprintf("bucket=%s region=%s endpoint=%s", c.name, c.region, ep)
}

func loadPublicBucketConfig(getenv func(string) string) (publicBucketConfig, error) {
	trim := func(k string) string { return strings.TrimSpace(getenv(k)) }
	cfg := publicBucketConfig{
		name:      trim("S3_BUCKET"),
		accessKey: trim("AWS_ACCESS_KEY"),
		secretKey: trim("AWS_SECRET_KEY"),
		region:    trim("AWS_REGION"),
		endpoint:  trim("S3_ENDPOINT"),
	}
	if cfg.region == "" {
		cfg.region = "us-east-1"
	}
	var missing []string
	if cfg.name == "" {
		missing = append(missing, "S3_BUCKET")
	}
	if cfg.accessKey == "" {
		missing = append(missing, "AWS_ACCESS_KEY")
	}
	if cfg.secretKey == "" {
		missing = append(missing, "AWS_SECRET_KEY")
	}
	if len(missing) > 0 {
		return publicBucketConfig{}, fmt.Errorf("public bucket config incomplete; set: %s", strings.Join(missing, ", "))
	}
	return cfg, nil
}

func boundedHTTPClient() *http.Client {
	return &http.Client{
		Timeout: 60 * time.Second, // large historical PNGs
		Transport: &http.Transport{
			DialContext: (&net.Dialer{
				Timeout:   5 * time.Second,
				KeepAlive: 30 * time.Second,
			}).DialContext,
			TLSHandshakeTimeout:   5 * time.Second,
			ResponseHeaderTimeout: 30 * time.Second,
			ExpectContinueTimeout: 1 * time.Second,
			IdleConnTimeout:       90 * time.Second,
			MaxIdleConns:          100,
			MaxIdleConnsPerHost:   10,
		},
	}
}

type s3Store struct {
	client *s3.Client
	bucket string
}

func newS3Store(ctx context.Context, cfg publicBucketConfig) (*s3Store, error) {
	loadOpts := []func(*awsconfig.LoadOptions) error{
		awsconfig.WithRegion(cfg.region),
		awsconfig.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(cfg.accessKey, cfg.secretKey, "")),
		awsconfig.WithHTTPClient(boundedHTTPClient()),
	}
	awsCfg, err := awsconfig.LoadDefaultConfig(ctx, loadOpts...)
	if err != nil {
		return nil, fmt.Errorf("aws config: %w", err)
	}
	var clientOpts []func(*s3.Options)
	if cfg.endpoint != "" {
		clientOpts = append(clientOpts, func(o *s3.Options) {
			o.BaseEndpoint = aws.String(cfg.endpoint)
			o.UsePathStyle = true
		})
	}
	return &s3Store{
		client: s3.NewFromConfig(awsCfg, clientOpts...),
		bucket: cfg.name,
	}, nil
}

func (s *s3Store) List(ctx context.Context, prefix string) ([]objectInfo, error) {
	var out []objectInfo
	var token *string
	for {
		resp, err := s.client.ListObjectsV2(ctx, &s3.ListObjectsV2Input{
			Bucket:            aws.String(s.bucket),
			Prefix:            aws.String(prefix),
			ContinuationToken: token,
		})
		if err != nil {
			return nil, err
		}
		for _, obj := range resp.Contents {
			if obj.Key == nil {
				continue
			}
			// Skip "directory" placeholders.
			if strings.HasSuffix(*obj.Key, "/") {
				continue
			}
			var size int64
			if obj.Size != nil {
				size = *obj.Size
			}
			out = append(out, objectInfo{Key: *obj.Key, Size: size})
		}
		if resp.IsTruncated == nil || !*resp.IsTruncated {
			break
		}
		token = resp.NextContinuationToken
	}
	return out, nil
}

func (s *s3Store) Get(ctx context.Context, key string) ([]byte, string, error) {
	resp, err := s.client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return nil, "", err
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, "", err
	}
	ct := ""
	if resp.ContentType != nil {
		ct = *resp.ContentType
	}
	return body, ct, nil
}

func (s *s3Store) Put(ctx context.Context, key string, body []byte, contentType string) error {
	input := &s3.PutObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
		Body:   bytes.NewReader(body),
	}
	if contentType != "" {
		input.ContentType = aws.String(contentType)
	}
	_, err := s.client.PutObject(ctx, input)
	return err
}

func (s *s3Store) Copy(ctx context.Context, srcKey, dstKey string) error {
	_, err := s.client.CopyObject(ctx, &s3.CopyObjectInput{
		Bucket:     aws.String(s.bucket),
		CopySource: aws.String(s.bucket + "/" + srcKey),
		Key:        aws.String(dstKey),
	})
	return err
}
