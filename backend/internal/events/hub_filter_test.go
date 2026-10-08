package events

import (
	"encoding/json"
	"fmt"
	"testing"
)

func splitEvent(billNumber string) BusinessEvent {
	return BusinessEvent{
		BusinessID: 7,
		Type:       "bill.split.updated",
		Data:       json.RawMessage(fmt.Sprintf(`{"bill_number":%q}`, billNumber)),
	}
}

func billNumberFilter(want string) func(BusinessEvent) bool {
	return func(ev BusinessEvent) bool {
		var p struct {
			BillNumber string `json:"bill_number"`
		}
		return json.Unmarshal(ev.Data, &p) == nil && p.BillNumber == want
	}
}

// A flood of other bills' split frames must never take this stream's buffer
// slots, so the target bill's frame still arrives and nothing is dropped.
func TestSubscribeGuestFiltered_OtherBillFloodDoesNotDropTargetFrames(t *testing.T) {
	hub := newTestHub()
	ch, _, dropped, cancel, ok := hub.SubscribeGuestFiltered(7, 0, "203.0.113.1", []string{"bill.split.updated"}, billNumberFilter("TARGET"))
	if !ok {
		t.Fatal("subscribe rejected")
	}
	defer cancel()

	for i := 0; i < subscriberBufferSize*4; i++ {
		hub.Publish(splitEvent(fmt.Sprintf("OTHER-%d", i)))
	}
	hub.Publish(splitEvent("TARGET"))

	select {
	case <-dropped:
		t.Fatal("drop signalled for frames this stream filters out")
	default:
	}
	if len(ch) != 1 {
		t.Fatalf("buffered %d frames, want only the target's 1", len(ch))
	}
	if ev := <-ch; !billNumberFilter("TARGET")(ev) {
		t.Fatalf("got %s, want TARGET frame", ev.Data)
	}
}

func TestSubscribeGuestFiltered_OverflowSignalsDrop(t *testing.T) {
	hub := newTestHub()
	_, _, dropped, cancel, ok := hub.SubscribeGuestFiltered(7, 0, "203.0.113.2", []string{"bill.split.updated"}, billNumberFilter("TARGET"))
	if !ok {
		t.Fatal("subscribe rejected")
	}
	defer cancel()

	for i := 0; i < subscriberBufferSize+5; i++ {
		hub.Publish(splitEvent("TARGET"))
	}
	select {
	case <-dropped:
	default:
		t.Fatal("overflow did not signal a drop")
	}
}

func TestSubscribeGuestFiltered_ReplayIsFiltered(t *testing.T) {
	hub := newTestHub()
	hub.Publish(splitEvent("OTHER"))
	hub.Publish(splitEvent("TARGET"))
	_, replayed, _, cancel, ok := hub.SubscribeGuestFiltered(7, 0, "203.0.113.3", []string{"bill.split.updated"}, billNumberFilter("TARGET"))
	if !ok {
		t.Fatal("subscribe rejected")
	}
	defer cancel()
	if len(replayed) != 1 || !billNumberFilter("TARGET")(replayed[0]) {
		t.Fatalf("replayed %d events, want only TARGET", len(replayed))
	}
}
