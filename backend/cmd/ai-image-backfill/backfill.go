package main

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"path"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/stdevmac/payverge/backend/internal/services"
)

// defaultAIGeneratedPrefix is where GenerateMenuImage and menu AI generators
// write objects (see internal/services/ai.go and menu_ai_service.go).
const defaultAIGeneratedPrefix = "menu_items/ai_generated"

// backupPrefixRoot holds pre-overwrite originals so rollback does not require
// S3 versioning. Keys map 1:1 under this root.
const backupPrefixRoot = "menu_items/ai_generated_backup"

// skipIfAtMostBytes: objects already at or under this size after a JPEG sniff
// are treated as optimized (NEW-8 targets well under 350KB).
const skipIfAtMostBytes = 400_000

// objectStore is the S3 surface the backfill needs. Tests inject fakes.
type objectStore interface {
	List(ctx context.Context, prefix string) ([]objectInfo, error)
	Get(ctx context.Context, key string) ([]byte, string, error) // body, contentType
	Put(ctx context.Context, key string, body []byte, contentType string) error
	Copy(ctx context.Context, srcKey, dstKey string) error
}

type objectInfo struct {
	Key  string
	Size int64
}

type backfillOptions struct {
	Prefix      string
	Apply       bool
	Concurrency int
	MaxKeys     int
	Out         io.Writer
}

type backfillSummary struct {
	Mode      string
	Listed    int
	Skipped   int
	Processed int
	Failed    int
}

// decision is the pure skip/process choice for one object.
// Content-Type for the live overwrite is NOT decided here: OptimizeAIGeneratedImageBytes
// may keep original PNG/WebP bytes when JPEG would not shrink (no resize needed), so
// the write path must use opt.MIMEType after optimize.
type decision struct {
	Action    string // "skip" | "process"
	Reason    string
	BackupKey string
}

// decideObject chooses skip vs process from size + bytes sniff. Pure helper
// unit-tested with no S3.
func decideObject(key string, size int64, body []byte) decision {
	if !isAIGeneratedKey(key) {
		return decision{Action: "skip", Reason: "outside ai_generated prefix or is backup"}
	}
	if strings.HasPrefix(key, backupPrefixRoot+"/") {
		return decision{Action: "skip", Reason: "backup object"}
	}

	sniff := http.DetectContentType(body)
	// Already small JPEG/WebP → leave alone (idempotent re-run).
	if size > 0 && size <= skipIfAtMostBytes && (strings.HasPrefix(sniff, "image/jpeg") || strings.HasPrefix(sniff, "image/webp")) {
		return decision{Action: "skip", Reason: fmt.Sprintf("already optimized (%s %d bytes)", sniff, size)}
	}
	// Tiny non-image junk — skip rather than fail the run.
	if len(body) == 0 {
		return decision{Action: "skip", Reason: "empty object"}
	}
	if !strings.HasPrefix(sniff, "image/") {
		return decision{Action: "skip", Reason: fmt.Sprintf("not an image (%s)", sniff)}
	}

	return decision{
		Action:    "process",
		Reason:    fmt.Sprintf("optimize %s %d bytes", sniff, size),
		BackupKey: backupKeyFor(key),
	}
}

// isAIGeneratedKey reports whether key is under the live ai_generated tree
// (not the backup tree).
func isAIGeneratedKey(key string) bool {
	key = strings.TrimPrefix(key, "/")
	if strings.HasPrefix(key, backupPrefixRoot+"/") {
		return false
	}
	return key == defaultAIGeneratedPrefix ||
		strings.HasPrefix(key, defaultAIGeneratedPrefix+"/")
}

// backupKeyFor maps a live key to its backup sibling.
// menu_items/ai_generated/foo.png → menu_items/ai_generated_backup/foo.png
func backupKeyFor(key string) string {
	key = strings.TrimPrefix(key, "/")
	rel := strings.TrimPrefix(key, defaultAIGeneratedPrefix)
	rel = strings.TrimPrefix(rel, "/")
	if rel == "" {
		rel = path.Base(key)
	}
	return backupPrefixRoot + "/" + rel
}

func runBackfill(ctx context.Context, store objectStore, opts backfillOptions) (backfillSummary, error) {
	if opts.Out == nil {
		opts.Out = io.Discard
	}
	if opts.Concurrency < 1 {
		opts.Concurrency = 1
	}
	prefix := opts.Prefix
	if prefix == "" {
		prefix = defaultAIGeneratedPrefix
	}

	mode := "dry-run"
	if opts.Apply {
		mode = "apply"
	}

	objects, err := store.List(ctx, prefix)
	if err != nil {
		return backfillSummary{}, fmt.Errorf("list %q: %w", prefix, err)
	}
	if opts.MaxKeys > 0 && len(objects) > opts.MaxKeys {
		objects = objects[:opts.MaxKeys]
	}

	summary := backfillSummary{Mode: mode, Listed: len(objects)}
	var skipped, processed, failed atomic.Int64
	var failMu sync.Mutex
	var failMsgs []string

	sem := make(chan struct{}, opts.Concurrency)
	var wg sync.WaitGroup

	for _, obj := range objects {
		obj := obj
		// Skip backup keys at list time (prefix filter may still return them if
		// someone points --prefix at the bucket root).
		if strings.HasPrefix(obj.Key, backupPrefixRoot+"/") {
			skipped.Add(1)
			_, _ = fmt.Fprintf(opts.Out, "skip  %s  reason=backup\n", obj.Key)
			continue
		}

		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer wg.Done()
			defer func() { <-sem }()

			if err := ctx.Err(); err != nil {
				failed.Add(1)
				failMu.Lock()
				failMsgs = append(failMsgs, fmt.Sprintf("%s: %v", obj.Key, err))
				failMu.Unlock()
				return
			}

			body, _, getErr := store.Get(ctx, obj.Key)
			if getErr != nil {
				failed.Add(1)
				failMu.Lock()
				failMsgs = append(failMsgs, fmt.Sprintf("%s: get: %v", obj.Key, getErr))
				failMu.Unlock()
				_, _ = fmt.Fprintf(opts.Out, "fail  %s  get: %v\n", obj.Key, getErr)
				return
			}
			size := obj.Size
			if size == 0 {
				size = int64(len(body))
			}

			d := decideObject(obj.Key, size, body)
			if d.Action == "skip" {
				skipped.Add(1)
				_, _ = fmt.Fprintf(opts.Out, "skip  %s  reason=%s\n", obj.Key, d.Reason)
				return
			}

			opt, optErr := services.OptimizeAIGeneratedImageBytes(body)
			if optErr != nil {
				failed.Add(1)
				failMu.Lock()
				failMsgs = append(failMsgs, fmt.Sprintf("%s: optimize: %v", obj.Key, optErr))
				failMu.Unlock()
				_, _ = fmt.Fprintf(opts.Out, "fail  %s  optimize: %v\n", obj.Key, optErr)
				return
			}
			// Honor the optimizer's MIME: JPEG when it re-encodes, or original
			// PNG/WebP when JPEG does not shrink and dimensions are unchanged
			// (generated_image_optimize.go keep-source branch).
			contentType := opt.MIMEType
			if contentType == "" {
				contentType = http.DetectContentType(opt.Bytes)
			}

			_, _ = fmt.Fprintf(opts.Out, "%s %s  %d→%d bytes  backup=%s  content-type=%s\n",
				map[bool]string{true: "apply", false: "plan "}[opts.Apply],
				obj.Key, opt.SourceBytes, opt.OutputBytes, d.BackupKey, contentType)

			if !opts.Apply {
				processed.Add(1)
				return
			}

			// Backup original bytes first, then overwrite same key with optimized
			// bytes + the optimizer's Content-Type (not a hardcoded image/jpeg).
			if err := store.Put(ctx, d.BackupKey, body, http.DetectContentType(body)); err != nil {
				// Prefer Put of the bytes we already hold so we never re-GET.
				failed.Add(1)
				failMu.Lock()
				failMsgs = append(failMsgs, fmt.Sprintf("%s: backup put: %v", obj.Key, err))
				failMu.Unlock()
				_, _ = fmt.Fprintf(opts.Out, "fail  %s  backup: %v\n", obj.Key, err)
				return
			}
			if err := store.Put(ctx, obj.Key, opt.Bytes, contentType); err != nil {
				failed.Add(1)
				failMu.Lock()
				failMsgs = append(failMsgs, fmt.Sprintf("%s: overwrite: %v", obj.Key, err))
				failMu.Unlock()
				_, _ = fmt.Fprintf(opts.Out, "fail  %s  overwrite: %v\n", obj.Key, err)
				return
			}
			processed.Add(1)
		}()
	}
	wg.Wait()

	summary.Skipped = int(skipped.Load())
	summary.Processed = int(processed.Load())
	summary.Failed = int(failed.Load())
	if len(failMsgs) > 0 {
		_, _ = fmt.Fprintf(opts.Out, "failures (%d):\n", len(failMsgs))
		for _, m := range failMsgs {
			_, _ = fmt.Fprintf(opts.Out, "  - %s\n", m)
		}
	}
	return summary, nil
}
