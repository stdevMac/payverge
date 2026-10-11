package schedclaim

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestClaim_exactlyOneWinnerUnderContention(t *testing.T) {
	db := setup(t)
	c := New(db)
	var wins, errs int64
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ok, err := c.Claim(context.Background(), "jobs", "done_at", "id", uint(1))
			if err != nil {
				atomic.AddInt64(&errs, 1)
				return
			}
			if ok {
				atomic.AddInt64(&wins, 1)
			}
		}()
	}
	wg.Wait()
	// Assert no goroutine errored too — otherwise wins==1 could hide a run where
	// 19 callers errored and only one happened to claim, which is not the
	// exactly-one-winner guarantee this test is the canonical proof of.
	require.EqualValues(t, 0, errs, "no claim attempt may error")
	require.EqualValues(t, 1, wins, "exactly one goroutine may win the claim")
}
