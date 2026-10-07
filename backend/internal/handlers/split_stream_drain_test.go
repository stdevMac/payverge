package handlers

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/stdevmac/payverge/backend/internal/events"
)

// On a dropped-frame signal the split stream drains every queued snapshot
// before it reads and sends sync.reset, so no older full snapshot can be
// written after the reset.
func TestDrainBusinessEvents_DiscardsQueuedFramesWithoutBlocking(t *testing.T) {
	ch := make(chan events.BusinessEvent, 4)
	for i := 0; i < 3; i++ {
		ch <- events.BusinessEvent{ID: uint64(i + 1), Type: "bill.split.updated"}
	}
	require.Equal(t, 3, drainBusinessEvents(ch))
	require.Len(t, ch, 0)

	require.Equal(t, 0, drainBusinessEvents(ch), "an empty channel returns at once")

	closed := make(chan events.BusinessEvent, 1)
	closed <- events.BusinessEvent{ID: 9}
	close(closed)
	require.Equal(t, 1, drainBusinessEvents(closed), "a closed channel stops the drain")
}
