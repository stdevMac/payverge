package server

import (
	"sync/atomic"
	"testing"
	"time"
)

// TestSSEStreamWriterHeartbeatStopsAndIsIdempotent locks the contract that
// stop waits out the heartbeat goroutine. A tick racing the handler's return
// used to call Comment on a gin writer after the handler had returned.
func TestSSEStreamWriterHeartbeatStopsAndIsIdempotent(t *testing.T) {
	var n atomic.Int64
	stop := startSSEHeartbeat(time.Millisecond, func() {
		n.Add(1)
		time.Sleep(2 * time.Millisecond)
	})

	stop()
	after := n.Load()
	time.Sleep(20 * time.Millisecond)
	if got := n.Load(); got != after {
		t.Fatalf("ping ran after stop returned: before=%d after=%d", after, got)
	}

	stop()
}
