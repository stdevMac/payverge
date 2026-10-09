package services

import (
	"sync"
	"testing"
	"time"
)

// TestExchangeRateLastFetchedNoRace hammers the throttle field from many
// goroutines the way the request path + ticker do. With -race this fails
// until lastFetched is atomic/guarded.
func TestExchangeRateLastFetchedNoRace(t *testing.T) {
	s := &ExchangeRateService{cacheDuration: time.Minute}
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 200; j++ {
				// read the throttle timestamp
				_ = s.recentlyFetched()
				// write it
				s.markFetched(time.Now())
			}
		}()
	}
	wg.Wait()
}
