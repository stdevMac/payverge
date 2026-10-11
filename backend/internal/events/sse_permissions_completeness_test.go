package events

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// publishCallRe matches `PublishJSON(<expr>, "event.type"` and `Publish(<expr>, "event.type"`,
// including multi-line struct-literal sites like
// `Publish(events.BusinessEvent{BusinessID: ..., Type: "order.created", ...})`.
// Event types are dot/underscore-separated lowercase tokens. The first argument
// (business ID) is often a wrapped expression like `uint(businessID)`, so we
// don't try to parse it strictly — we just require the FIRST quoted string
// after the call opens to be the event type, within a bounded window.
//
// The window is 400 non-quote characters (was 80): the real struct-literal
// sites (handlers/orders.go, server/guest_handlers.go) already sat at ~70-75
// chars, so one longer variable name or an extra field before Type: would have
// silently dropped them from the scan. 400 is generous enough that no
// realistic argument list before the type literal falls out, while the
// `[^"]` class still hard-stops at the first quote so an earlier string
// literal can never be skipped over. The canary fixture below
// (TestScannerFindsStructLiteralPublishBeyondOldWindow) pins this width.
//
// LIMITATION (be honest about it): this only catches publish sites whose event
// type is a quoted LITERAL at the call. Sites that pass a VARIABLE (the
// publishDeliveryEvent and publishAlert wrappers) are invisible to it, so each
// such wrapper gets its own explicit anchor below — never rely on a
// coincidental nearby literal happening to fall inside the window.
var publishCallRe = regexp.MustCompile(`(?s)Publish(?:JSON)?\([^"]{0,400}"([a-z][a-z0-9_.]+)"`)

// wrapperCallRe anchors on known helper wrappers around hub publishes whose
// call sites pass the event type as a quoted literal argument. Today that is
// DeliveryHandler.publishDeliveryEvent (internal/handlers/delivery_handlers.go),
// called with "delivery.updated"/"delivery.cancelled" as its 4th argument. Add
// any new such wrapper to this alternation.
var wrapperCallRe = regexp.MustCompile(`(?s)publishDeliveryEvent\([^"]{0,120}"([a-z][a-z0-9_.]+)"`)

// alertLiteralRe harvests the concrete alert.* event names. The operational
// alerts service publishes via publishAlert(eventName, ...) where eventName
// flows through eventNameForEventType's return literals, so no call-anchored
// regex can see them; instead every "alert.x" literal in a file that contains a
// direct hub publish call counts as published. The gate on
// `events.GetHub().Publish` (see collectPublishedEventTypes) keeps this
// package's own allBusinessEventTypes enumeration from circularly counting.
var alertLiteralRe = regexp.MustCompile(`"(alert\.[a-z0-9_.]+)"`)

// Print wakeups select a permission-scoped event name through
// printWakeEventType before publishing. Harvest those closed return literals
// only from the file that also contains the direct hub publish.
var printWakeLiteralRe = regexp.MustCompile(`"(print\.(?:bill|receipt|kitchen)_available)"`)

// exemptTypes are stream-control frames, not business events.
var exemptTypes = map[string]bool{"connected": true, "ping": true, "error": true, "sync.reset": true}

func collectPublishedEventTypes(t *testing.T) map[string]bool {
	t.Helper()
	published := map[string]bool{}
	root := filepath.Join("..", "..") // backend/
	// Walk both internal/ and cmd/: main.go wires services and has historically
	// grown inline glue (schedulers, sweepers) — a publish site added there must
	// not be invisible to the completeness scan just because of its directory.
	for _, dir := range []string{"internal", "cmd"} {
		err := filepath.Walk(filepath.Join(root, dir), func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if info.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return nil
			}
			raw, readErr := os.ReadFile(path)
			if readErr != nil {
				return readErr
			}
			src := string(raw)
			for _, m := range publishCallRe.FindAllStringSubmatch(src, -1) {
				published[m[1]] = true
			}
			for _, m := range wrapperCallRe.FindAllStringSubmatch(src, -1) {
				published[m[1]] = true
			}
			// alert.* names live in return literals feeding a variable-typed publish;
			// only harvest them from files that demonstrably publish to the hub.
			if strings.Contains(src, "events.GetHub().Publish") {
				for _, m := range alertLiteralRe.FindAllStringSubmatch(src, -1) {
					published[m[1]] = true
				}
				if strings.Contains(src, "printWakeEventType") {
					for _, m := range printWakeLiteralRe.FindAllStringSubmatch(src, -1) {
						published[m[1]] = true
					}
				}
			}
			return nil
		})
		if err != nil {
			t.Fatalf("walk backend/%s: %v", dir, err)
		}
	}
	if len(published) < 15 {
		t.Fatalf("suspiciously few publish sites found (%d) — regex or layout drifted", len(published))
	}
	return published
}

// Every event type published to the hub must have an explicit RBAC gate, or a
// topic-scoped (non-owner) subscriber will silently never receive it — that is
// exactly how notification.new was invisible to staff for weeks.
func TestEveryPublishedEventTypeIsPermissionMapped(t *testing.T) {
	published := collectPublishedEventTypes(t)
	for et := range published {
		if exemptTypes[et] || strings.HasPrefix(et, "alert.") {
			continue
		}
		if _, ok := eventTypePermissions[et]; !ok {
			t.Errorf("event type %q is published but missing from eventTypePermissions — scoped subscribers will never receive it", et)
		}
	}
}

// Phantom entries hide real coverage gaps: reservation.created sat in the map
// for months while nothing published it.
func TestEveryPermissionMapEntryIsPublished(t *testing.T) {
	published := collectPublishedEventTypes(t)
	for et := range eventTypePermissions {
		if !published[et] {
			t.Errorf("eventTypePermissions has %q but nothing publishes it (phantom entry)", et)
		}
	}
}

// canaryStructLiteralPublish is a fixture, NOT real code: a struct-literal
// publish site shaped like the ones in handlers/orders.go and
// server/guest_handlers.go, but with deliberately long field names so the
// quoted event type sits MORE than 80 characters after `Publish(`. The
// original publishCallRe window was `[^"]{0,80}`, which put real struct-literal
// sites (~70-75 chars) one long variable name away from silently dropping out
// of the scan. This canary pins the widened window: if it ever shrinks back,
// TestScannerFindsStructLiteralPublishBeyondOldWindow fails before any real
// publish site can go invisible. (This lives in a _test.go file, which
// collectPublishedEventTypes skips, so the canary type never pollutes the
// published set.)
const canaryStructLiteralPublish = `events.GetHub().Publish(events.BusinessEvent{
	BusinessID: someDeliberatelyVeryLongBusinessIdentifierVariableName,
	Priority:   someOtherFieldValueThatPrecedesTheTypeField,
	Type:       "canary.struct_literal.published",
	Timestamp:  time.Now(),
})`

func TestScannerFindsStructLiteralPublishBeyondOldWindow(t *testing.T) {
	open := strings.Index(canaryStructLiteralPublish, "Publish(")
	quote := strings.Index(canaryStructLiteralPublish, `"`)
	if open == -1 || quote == -1 {
		t.Fatal("canary fixture malformed")
	}
	if dist := quote - (open + len("Publish(")); dist <= 80 {
		t.Fatalf("canary fixture no longer exercises the widened window: type literal only %d chars after Publish( — keep it beyond the old 80-char limit", dist)
	}
	m := publishCallRe.FindStringSubmatch(canaryStructLiteralPublish)
	if m == nil {
		t.Fatal("publishCallRe missed a struct-literal publish whose event type sits >80 chars after Publish( — the scan window regressed and real publish sites can silently drop out of the completeness check")
	}
	if m[1] != "canary.struct_literal.published" {
		t.Fatalf("publishCallRe captured %q, want %q", m[1], "canary.struct_literal.published")
	}
}

// The alert.* prefix family is exempt from the map (requiredPermission handles
// the prefix), but its concrete names are hand-enumerated in
// allBusinessEventTypes — and a scoped subscriber only receives topics in that
// list. So a newly published alert.x that is missing from the enumeration
// silently vanishes for every non-owner, exactly like an unmapped type would.
func TestEveryPublishedAlertTypeIsEnumerated(t *testing.T) {
	published := collectPublishedEventTypes(t)
	enumerated := map[string]bool{}
	for _, et := range allBusinessEventTypes {
		enumerated[et] = true
	}
	sawAlert := false
	for et := range published {
		if !strings.HasPrefix(et, "alert.") {
			continue
		}
		sawAlert = true
		if !enumerated[et] {
			t.Errorf("alert type %q is published but missing from allBusinessEventTypes — scoped subscribers will never receive it", et)
		}
	}
	if !sawAlert {
		t.Fatal("no published alert.* literals found — the alert scanner drifted from the operational_alerts publish path")
	}
}
