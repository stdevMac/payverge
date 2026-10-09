package logger

// SafeGo launches fn in a new goroutine with panic recovery. Any panic is
// logged and reported to the installed panic reporter; the process survives.
func SafeGo(fn func()) {
	SafeGoNamed("background goroutine", fn)
}

// SafeGoNamed is SafeGo with a human name used in the log + Sentry report.
// The panic reporter installed at the time of the panic (not at call time)
// receives the report.
func SafeGoNamed(name string, fn func()) {
	go func() {
		defer func() {
			if r := recover(); r != nil {
				reportPanic(name, r)
			}
		}()
		fn()
	}()
}
