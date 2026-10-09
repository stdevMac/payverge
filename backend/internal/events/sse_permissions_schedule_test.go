package events

import "testing"

// TestScheduleSSEEventsRegistered guards that the Slice-2 scheduling event types
// are mapped to schedule:read in the SSE permission table. Without an explicit
// mapping, requiredPermission falls back deny-by-default to financial:read, which
// would hide the staff's own published schedule / shift assignments from them.
func TestScheduleSSEEventsRegistered(t *testing.T) {
	for _, et := range []string{"schedule.published", "shift.assigned", "shift.updated"} {
		if got := requiredPermission(et); got != "schedule:read" {
			t.Errorf("requiredPermission(%q) = %q, want schedule:read", et, got)
		}
	}
	// And they must be enumerated in the whole-business topic list so a
	// schedule:read subscriber is actually subscribed to them.
	want := map[string]bool{"schedule.published": false, "shift.assigned": false, "shift.updated": false}
	for _, et := range allBusinessEventTypes {
		if _, ok := want[et]; ok {
			want[et] = true
		}
	}
	for et, found := range want {
		if !found {
			t.Errorf("event %q missing from allBusinessEventTypes", et)
		}
	}
}
