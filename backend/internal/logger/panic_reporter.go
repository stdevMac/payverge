package logger

import (
	"runtime/debug"
	"sync/atomic"
)

// panicReporterFunc reports a recovered panic to an out-of-process sink
// (Sentry in production). name identifies the goroutine/loop, r is the
// recovered value, and stack is the goroutine stack captured at recover time.
type panicReporterFunc func(name string, r interface{}, stack []byte)

// panicReporter holds the installed reporter behind an atomic pointer so the
// recover blocks (which run on arbitrary goroutines) and SetPanicReporter
// (called once at startup, and swapped by tests) never race. The logger
// package intentionally does NOT import sentry-go; SetPanicReporter is wired
// once from main after sentry.Init.
var panicReporter atomic.Pointer[panicReporterFunc]

func init() {
	noop := panicReporterFunc(func(string, interface{}, []byte) {})
	panicReporter.Store(&noop)
}

// SetPanicReporter installs the process-wide panic reporter. Call once at
// startup (cmd/app/main.go) after Sentry is initialized. A nil fn is ignored.
func SetPanicReporter(fn func(name string, r interface{}, stack []byte)) {
	if fn != nil {
		f := panicReporterFunc(fn)
		panicReporter.Store(&f)
	}
}

// reportPanic logs the recovered panic at Error level (preserving the existing
// observable log line) and forwards it to the installed reporter.
func reportPanic(name string, r interface{}) {
	stack := debug.Stack()
	Logger.Errorf("recovered panic in %s: %v\nstack: %s", name, r, stack)
	(*panicReporter.Load())(name, r, stack)
}
