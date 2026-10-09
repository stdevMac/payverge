// Command ai-image-backfill re-optimizes historical AI-generated menu images on
// the public S3 bucket that were uploaded before NEW-8 (multi-MB PNGs under
// menu_items/ai_generated/). New uploads already go through
// services.OptimizeAIGeneratedImageBytes; this tool is the one-shot ops path.
//
// Safety:
//
//	--dry-run is the DEFAULT (prints planned changes; no writes)
//	--apply is required to copy originals to a backup/ prefix and overwrite
//
// Strategy: same-key overwrite with Content-Type image/jpeg so every DB/content
// URL that still ends in .png keeps working (browsers use Content-Type, not
// extension). Originals are copied to menu_items/ai_generated_backup/ before
// overwrite so rollback does not depend on bucket versioning.
//
// Env (same as backend public bucket): S3_BUCKET, AWS_ACCESS_KEY, AWS_SECRET_KEY,
// AWS_REGION (default us-east-1), S3_ENDPOINT (optional).
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"
)

func main() {
	apply := flag.Bool("apply", false, "perform backup+overwrite (default is dry-run)")
	dryRun := flag.Bool("dry-run", true, "print planned changes without writing (default true; ignored when --apply)")
	prefix := flag.String("prefix", defaultAIGeneratedPrefix, "S3 key prefix to scan")
	concurrency := flag.Int("concurrency", 4, "max concurrent object workers")
	maxKeys := flag.Int("max-keys", 0, "optional cap on objects listed (0 = no cap)")
	timeout := flag.Duration("timeout", 30*time.Minute, "overall deadline")
	flag.Parse()

	// --apply turns dry-run off; otherwise dry-run stays the default.
	doApply := *apply
	if doApply {
		*dryRun = false
	} else {
		*dryRun = true
	}

	cfg, err := loadPublicBucketConfig(os.Getenv)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ai-image-backfill: %v\n", err)
		os.Exit(1)
	}

	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()

	// Dry-run still lists + downloads to report before/after sizes; both modes
	// need a live client when credentials are present.
	store, err := newS3Store(ctx, cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ai-image-backfill: s3 client: %v\n", err)
		os.Exit(1)
	}

	summary, err := runBackfill(ctx, store, backfillOptions{
		Prefix:      *prefix,
		Apply:       doApply,
		Concurrency: *concurrency,
		MaxKeys:     *maxKeys,
		Out:         os.Stdout,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "ai-image-backfill: %v\n", err)
		os.Exit(1)
	}
	_, _ = fmt.Fprintf(os.Stdout, "ai-image-backfill: done mode=%s listed=%d skipped=%d processed=%d failed=%d\n",
		summary.Mode, summary.Listed, summary.Skipped, summary.Processed, summary.Failed)
	if summary.Failed > 0 {
		os.Exit(2)
	}
}
