package database

import (
	"testing"
	"time"
)

func TestIsAIWaiterAvailable(t *testing.T) {
	closed := time.Now()
	cases := []struct {
		name string
		biz  Business
		ai   bool
		want bool
	}{
		{"active+enabled", Business{IsActive: true}, true, true},
		{"active+disabled", Business{IsActive: true}, false, false},
		{"suspended+enabled", Business{IsActive: false}, true, false},
		{"closed+enabled", Business{IsActive: true, ClosedAt: &closed}, true, false},
		{"demo+suspended+enabled", Business{IsActive: false, IsDemo: true}, true, true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			b := tc.biz
			b.AiSettings.AiEnabled = tc.ai
			if got := IsAIWaiterAvailable(&b); got != tc.want {
				t.Fatalf("IsAIWaiterAvailable(%s) = %v, want %v", tc.name, got, tc.want)
			}
		})
	}

	if IsAIWaiterAvailable(nil) {
		t.Fatal("IsAIWaiterAvailable(nil) = true, want false")
	}
}
