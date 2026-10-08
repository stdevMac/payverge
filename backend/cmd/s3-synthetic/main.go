// Command s3-synthetic exercises the real object-storage path the app depends on
// by round-tripping a tiny, uniquely-tagged object through each configured
// bucket: PUT -> GET -> verify bytes -> DELETE. It closes the S3 gap in the
// synthetic-monitors catalog, which previously had
// neither a synthetic nor an alert (S3 exposes no backend metric to alert on).
//
// It reads the SAME env vars the backend uses (internal/s3):
//
//	Public bucket:    S3_BUCKET, AWS_ACCESS_KEY, AWS_SECRET_KEY,
//	                  S3_ENDPOINT (optional)
//	Protected bucket: S3_PROTECTED_BUCKET, AWS_PROTECTED_ACCESS_KEY,
//	                  AWS_PROTECTED_SECRET_KEY, S3_PROTECTED_ENDPOINT (optional)
//	Shared:           AWS_REGION (default us-east-1)
//
// Flags:
//
//	--dry-run          validate configuration for the selected buckets without
//	                   any network call
//	--bucket <sel>     public | protected | both (default both)
//	--timeout <dur>    overall deadline for the live probe (default 30s)
//
// Exit codes: 0 on success, nonzero on any configuration or probe error.
// It never logs or prints credential values.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"
)

func main() {
	dryRun := flag.Bool("dry-run", false, "validate configuration without any network call")
	bucketSel := flag.String("bucket", "both", "which bucket(s) to probe: public|protected|both")
	timeout := flag.Duration("timeout", 30*time.Second, "overall deadline for the live probe")
	flag.Parse()

	if err := run(*dryRun, *bucketSel, *timeout, os.Getenv, os.Stdout); err != nil {
		fmt.Fprintf(os.Stderr, "s3-synthetic: %v\n", err)
		os.Exit(1)
	}
}

// run resolves config for the selected buckets and, unless --dry-run, performs
// a roundtrip against each. It is separated from main() so the getenv source and
// output sink are injectable.
func run(dryRun bool, bucketSel string, timeout time.Duration, getenv func(string) string, out interface{ Write([]byte) (int, error) }) error {
	labels, err := selectBuckets(bucketSel)
	if err != nil {
		return err
	}

	configs := make([]bucketConfig, 0, len(labels))
	for _, label := range labels {
		cfg, err := loadBucketConfig(label, getenv)
		if err != nil {
			return err
		}
		configs = append(configs, cfg)
	}

	if dryRun {
		for _, cfg := range configs {
			fmt.Fprintf(out, "s3-synthetic: dry-run OK — %s\n", cfg.summary())
		}
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	for _, cfg := range configs {
		store, err := newObjectStore(ctx, cfg)
		if err != nil {
			return fmt.Errorf("%s: %w", cfg.label, err)
		}
		key := generateKey()
		payload := probePayload(key, time.Now())
		if err := roundtrip(ctx, store, key, payload); err != nil {
			return fmt.Errorf("%s (%s): %w", cfg.label, cfg.summary(), err)
		}
		fmt.Fprintf(out, "s3-synthetic: %s roundtrip OK — %s key=%s\n", cfg.label, cfg.summary(), key)
	}
	return nil
}
