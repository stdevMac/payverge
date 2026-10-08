package dailyquota

import (
	"fmt"
	"sync"
	"testing"
	"time"
)

func TestTake_CapsPerKeyPerDay(t *testing.T) {
	c := New(100)
	day := time.Date(2026, 10, 3, 23, 0, 0, 0, time.UTC)
	c.SetClock(func() time.Time { return day })
	for i := 0; i < 3; i++ {
		if ok, _ := c.Take(Limit{Key: "ip:a", Max: 3}); !ok {
			t.Fatalf("event %d refused under the cap", i+1)
		}
	}
	ok, key := c.Take(Limit{Key: "ip:a", Max: 3})
	if ok || key != "ip:a" {
		t.Fatalf("4th event must be refused, got ok=%v key=%q", ok, key)
	}
	if ok, _ := c.Take(Limit{Key: "ip:b", Max: 3}); !ok {
		t.Fatal("another key must be unaffected")
	}
	// The next UTC day starts a fresh window.
	c.SetClock(func() time.Time { return day.Add(2 * time.Hour) })
	if ok, _ := c.Take(Limit{Key: "ip:a", Max: 3}); !ok {
		t.Fatal("a new UTC day must reset the window")
	}
}

func TestTake_AllOrNothingAcrossLimits(t *testing.T) {
	c := New(100)
	if ok, _ := c.Take(Limit{Key: "dev:x", Max: 1}); !ok {
		t.Fatal("first device event refused")
	}
	ok, key := c.Take(Limit{Key: "ip:a", Max: 10}, Limit{Key: "dev:x", Max: 1})
	if ok || key != "dev:x" {
		t.Fatalf("device cap must refuse, got ok=%v key=%q", ok, key)
	}
	if n := c.Count("ip:a"); n != 0 {
		t.Fatalf("a refused event must not burn the IP quota, count=%d", n)
	}
}

func TestTake_DisabledAndEmptyLimitsAreIgnored(t *testing.T) {
	c := New(10)
	for i := 0; i < 20; i++ {
		if ok, _ := c.Take(Limit{Key: "", Max: 1}, Limit{Key: "k", Max: 0}); !ok {
			t.Fatal("empty key / non-positive max must never refuse")
		}
	}
}

func TestTake_ConcurrentNeverOvershoots(t *testing.T) {
	c := New(100)
	var wg sync.WaitGroup
	var mu sync.Mutex
	admitted := 0
	for i := 0; i < 200; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if ok, _ := c.Take(Limit{Key: "ip:hot", Max: 25}); ok {
				mu.Lock()
				admitted++
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	if admitted != 25 {
		t.Fatalf("admitted %d, want exactly 25", admitted)
	}
}

func TestCounter_MemoryIsBounded(t *testing.T) {
	c := New(50)
	for i := 0; i < 1000; i++ {
		c.Take(Limit{Key: fmt.Sprintf("ip:%d", i), Max: 5})
	}
	if n := c.cache.Len(); n > 50 {
		t.Fatalf("cache grew to %d entries, cap 50", n)
	}
}

func TestNetworkKey(t *testing.T) {
	for in, want := range map[string]string{
		"203.0.113.7":          "203.0.113.7",
		"::ffff:203.0.113.7":   "203.0.113.7",
		"2001:db8:1:2:3:4:5:6": "2001:db8:1:2::/64",
		"2001:db8:1:2:ffff::1": "2001:db8:1:2::/64",
		" 198.51.100.1 ":       "198.51.100.1",
		"":                     "",
		"not-an-ip":            "not-an-ip",
	} {
		if got := NetworkKey(in); got != want {
			t.Errorf("NetworkKey(%q) = %q, want %q", in, got, want)
		}
	}
}
