package logger

// SafeTick runs fn with panic recovery and returns normally even if fn panics.
//
// Unlike SafeGo, it does NOT spawn a goroutine: it is meant to wrap a single
// iteration of a long-lived worker/scheduler loop (`for { select { case
// <-ticker.C: ... } }`). Such loops run in their own goroutines, so a panic in
// one iteration is NOT caught by gin.Recovery and would unwind the loop and
// crash the entire process — and because every scheduler also runs its check
// immediately on startup, a persistent bad row would crash-loop on boot rather
// than self-heal. SafeTick contains the panic, logs it with a full stack at
// Error level (so it stays observable), and lets the caller's loop continue to
// the next iteration.
//
// Use SafeGo for short-lived fire-and-forget goroutines; use SafeTick for one
// iteration of a loop that must keep running.
func SafeTick(name string, fn func()) {
	defer func() {
		if r := recover(); r != nil {
			reportPanic(name+" loop iteration", r)
		}
	}()
	fn()
}
