package services

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"runtime/debug"
	"sync"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/events"
	"github.com/stdevmac/payverge/backend/internal/spaces"
	"github.com/stdevmac/payverge/backend/internal/spaces/scan"
)

const (
	spaceScanWorkerQueueSize = 32
	spaceScanWorkerPollEvery = 15 * time.Second
	spaceScanJobTimeout      = 3 * time.Minute
)

// SpaceScanWorker claims sessions in processing status, runs SpaceScanProcessor,
// writes result into the bound space draft (no auto-publish), emits SSE.
type SpaceScanWorker struct {
	store     scan.ArtifactStore
	spaces    *spaces.Service
	queue     chan uint
	now       func() time.Time
	startOnce sync.Once
	stopOnce  sync.Once
	cancel    context.CancelFunc
	wg        sync.WaitGroup
}

// GlobalSpaceScanWorker is set by StartSpaceScanWorker for handler enqueue.
var GlobalSpaceScanWorker *SpaceScanWorker

// NewSpaceScanWorker constructs a worker. store may be nil (uses DefaultArtifactStore).
func NewSpaceScanWorker(store scan.ArtifactStore) *SpaceScanWorker {
	if store == nil {
		store = scan.DefaultArtifactStore("")
	}
	return &SpaceScanWorker{
		store:  store,
		spaces: spaces.NewService(),
		queue:  make(chan uint, spaceScanWorkerQueueSize),
		now:    func() time.Time { return time.Now().UTC() },
	}
}

// StartSpaceScanWorker starts the global worker (called from main after DB init).
func StartSpaceScanWorker(ctx context.Context, store scan.ArtifactStore) *SpaceScanWorker {
	w := NewSpaceScanWorker(store)
	GlobalSpaceScanWorker = w
	w.Start(ctx)
	return w
}

// Start runs the background consumer until ctx is cancelled.
func (w *SpaceScanWorker) Start(ctx context.Context) {
	w.startOnce.Do(func() {
		runCtx, cancel := context.WithCancel(ctx)
		w.cancel = cancel
		w.wg.Add(1)
		go func() {
			defer w.wg.Done()
			w.loop(runCtx)
		}()
		// Startup recovery + periodic poll.
		w.wg.Add(1)
		go func() {
			defer w.wg.Done()
			w.pollLoop(runCtx)
		}()
	})
}

// Stop cancels the worker and waits for in-flight work.
func (w *SpaceScanWorker) Stop() {
	w.stopOnce.Do(func() {
		if w.cancel != nil {
			w.cancel()
		}
	})
	w.wg.Wait()
}

// Enqueue wakes the worker for sessionID.
func (w *SpaceScanWorker) Enqueue(sessionID uint) {
	if sessionID == 0 || w == nil {
		return
	}
	select {
	case w.queue <- sessionID:
	default:
		log.Printf("space scan worker queue full; session %d will rely on poll recovery", sessionID)
	}
}

// EnqueueSpaceScanSession is a package-level helper for handlers.
func EnqueueSpaceScanSession(sessionID uint) {
	if GlobalSpaceScanWorker != nil {
		GlobalSpaceScanWorker.Enqueue(sessionID)
	}
}

func (w *SpaceScanWorker) loop(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case id := <-w.queue:
			w.process(ctx, id)
		}
	}
}

func (w *SpaceScanWorker) pollLoop(ctx context.Context) {
	// Initial recovery.
	w.restorePending()
	ticker := time.NewTicker(spaceScanWorkerPollEvery)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			w.restorePending()
		}
	}
}

func (w *SpaceScanWorker) restorePending() {
	sessions, err := database.ListSpaceScanSessionsNeedingWork(50)
	if err != nil {
		log.Printf("space scan worker restore: %v", err)
		return
	}
	for _, s := range sessions {
		w.Enqueue(s.ID)
	}
}

func (w *SpaceScanWorker) process(parent context.Context, sessionID uint) {
	var businessID uint
	var spaceID *uint
	// resultStored flips once the review_ready result is persisted. A panic
	// after that point (draft apply, audit, SSE) must not downgrade a stored
	// scan to failed; it only flags the draft apply for manual re-apply.
	resultStored := false
	defer func() {
		if r := recover(); r != nil {
			log.Printf("space scan session=%d panicked: %v\n%s", sessionID, r, debug.Stack())
			if businessID == 0 {
				return
			}
			if resultStored {
				_ = database.MarkSpaceScanDraftApplyFailed(businessID, sessionID, "scan ready but draft apply failed — re-apply from review")
				return
			}
			w.fail(businessID, sessionID, spaceID, "internal_error", "scan processing failed")
		}
	}()

	ctx, cancel := context.WithTimeout(parent, spaceScanJobTimeout)
	defer cancel()

	session, err := database.GetSpaceScanSessionByIDUnscoped(sessionID)
	if err != nil || session == nil {
		// Already completed, cancelled, or unknown id — nothing to do.
		return
	}
	businessID = session.BusinessID
	spaceID = session.SpaceID
	// Only process sessions still in the processing queue state.
	if session.Status != database.ScanStatusProcessing {
		return
	}

	publishScanSSE(businessID, session.ID, session.SpaceID, database.ScanStatusProcessing, 55, "processing scan")

	uploads, err := database.ListSpaceScanUploads(businessID, sessionID)
	if err != nil {
		w.fail(businessID, sessionID, session.SpaceID, "load_uploads_failed", "failed to load uploads")
		return
	}

	// Prefer roomplan_json, then keyframes/metadata.
	var primary *database.SpaceScanUpload
	for i := range uploads {
		u := &uploads[i]
		if !u.IsComplete || u.S3Key == nil || *u.S3Key == "" {
			continue
		}
		if u.UploadKind == database.ScanUploadRoomPlanJSON {
			primary = u
			break
		}
		if primary == nil && (u.UploadKind == database.ScanUploadKeyframes || u.UploadKind == database.ScanUploadMetadata) {
			primary = u
		}
	}
	if primary == nil {
		w.fail(businessID, sessionID, session.SpaceID, "no_usable_upload", "no complete scan payload found")
		return
	}

	payload, err := w.store.Get(*primary.S3Key)
	if err != nil {
		w.fail(businessID, sessionID, session.SpaceID, "download_failed", "failed to load scan artifact")
		return
	}

	processor, err := scan.ProcessorForKind(primary.UploadKind)
	if err != nil {
		w.fail(businessID, sessionID, session.SpaceID, "unsupported_kind", err.Error())
		return
	}

	input := scan.ProcessInput{
		BusinessID:    businessID,
		SpaceID:       session.SpaceID,
		Kind:          primary.UploadKind,
		Payload:       payload,
		CalibrationMm: session.CalibrationMm,
		DeviceMeta:    json.RawMessage(session.DeviceMetaJSON),
	}

	// Optional keyframe expansion from metadata payload.
	if primary.UploadKind == database.ScanUploadKeyframes || primary.UploadKind == database.ScanUploadMetadata {
		var frames []scan.Keyframe
		if json.Unmarshal(payload, &frames) == nil {
			input.Frames = frames
		} else {
			var wrap struct {
				Frames []scan.Keyframe `json:"frames"`
			}
			if json.Unmarshal(payload, &wrap) == nil {
				input.Frames = wrap.Frames
			}
		}
	}

	result, err := processor.Process(ctx, input)
	if err != nil {
		w.fail(businessID, sessionID, session.SpaceID, "process_failed", "scan processing failed")
		log.Printf("space scan process session=%d: %v", sessionID, err)
		return
	}

	layoutBytes, err := spaces.MarshalLayoutDocument(result.Layout)
	if err != nil {
		w.fail(businessID, sessionID, session.SpaceID, "marshal_failed", "failed to serialize layout")
		return
	}

	// Write result onto session (review_ready — never auto-publish).
	// Keep the opaque token valid so phone/desktop can still GET result for review.
	// Token is revoked on cancel, complete, or expiry — not here.
	if err := database.SetSpaceScanSessionResult(businessID, sessionID, database.JSONRawMessage(layoutBytes), database.ScanStatusReviewReady); err != nil {
		w.fail(businessID, sessionID, session.SpaceID, "persist_failed", "failed to store scan result")
		return
	}
	resultStored = true

	// If bound to a space, write scan layout into draft (candidates allowed).
	// - success → clear sticky draft_apply_failed
	// - skip (scan/candidate draft left alone) → do NOT mark failed
	// - operator content / CAS conflict → draft_apply_failed for manual merge
	if session.SpaceID != nil && *session.SpaceID > 0 {
		if _, res, uerr := w.spaces.ApplyScanLayoutToDraft(businessID, *session.SpaceID, layoutBytes); uerr != nil {
			if spaces.IsScanDraftSkipped(uerr) {
				// Prior scan-owned or unlinked draft left intact; result_layout_json
				// still holds the new scan for review. Not a failure signal.
				log.Printf("space scan draft apply skipped session=%d: %v", sessionID, uerr)
			} else {
				log.Printf("space scan draft apply session=%d: %v issues=%+v warnings=%+v",
					sessionID, uerr, res.Issues, res.Warnings)
				msg := "scan ready but draft apply failed — re-apply from review"
				_ = database.MarkSpaceScanDraftApplyFailed(businessID, sessionID, msg)
			}
		} else {
			// Successful auto-apply: clear any sticky failure from earlier attempts.
			_ = database.ClearSpaceScanDraftApplyFailed(businessID, sessionID)
			src := result.Source
			_ = database.CreateSpaceLayoutAuditEvent(&database.SpaceLayoutAuditEvent{
				BusinessID: businessID,
				SpaceID:    session.SpaceID,
				Action:     "scan_draft_applied",
				DetailJSON: database.JSONRawMessage(fmt.Sprintf(
					`{"session_id":%d,"source":%q,"approximate":%v}`, sessionID, src, result.Approximate)),
			})
		}
	}

	msg := "review ready"
	publishScanSSE(businessID, sessionID, session.SpaceID, database.ScanStatusReviewReady, 100, msg)
}

func (w *SpaceScanWorker) fail(businessID, sessionID uint, spaceID *uint, code, message string) {
	_ = database.UpdateSpaceScanSessionStatus(businessID, sessionID, database.ScanStatusFailed, 0, &message, &code, &message)
	publishScanSSE(businessID, sessionID, spaceID, database.ScanStatusFailed, 0, message)
}

func publishScanSSE(businessID, sessionID uint, spaceID *uint, status string, progressPct int, progressMessage string) {
	payload := gin.H{
		"session_id":       sessionID,
		"status":           status,
		"progress_pct":     progressPct,
		"progress_message": progressMessage,
	}
	if spaceID != nil {
		payload["space_id"] = *spaceID
	} else {
		payload["space_id"] = nil
	}
	events.GetHub().PublishJSON(businessID, "space.scan.updated", payload)
}
