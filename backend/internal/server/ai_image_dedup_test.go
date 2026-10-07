package server

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"golang.org/x/sync/singleflight"
)

func TestImageJobDedup_TwoConcurrentReserveOnce(t *testing.T) {
	var group singleflight.Group
	var reserveCount int32

	blocker := make(chan struct{})

	body := func() (interface{}, error) {
		atomic.AddInt32(&reserveCount, 1)
		<-blocker
		return "https://cdn/x.png", nil
	}

	key := imageJobBriefKey(imageJobBrief{BusinessID: 7, Tool: "regenerate", Name: "Margherita Pizza"})

	var wg sync.WaitGroup
	var results [2]string
	var errs [2]error
	wg.Add(2)

	go func() {
		defer wg.Done()
		v, err, _ := group.Do(key, body)
		results[0] = v.(string)
		errs[0] = err
	}()
	go func() {
		defer wg.Done()
		v, err, _ := group.Do(key, body)
		results[1] = v.(string)
		errs[1] = err
	}()

	time.Sleep(10 * time.Millisecond)
	close(blocker)
	wg.Wait()

	require.NoError(t, errs[0])
	require.NoError(t, errs[1])
	assert.Equal(t, int32(1), atomic.LoadInt32(&reserveCount), "two concurrent identical jobs must reserve exactly one credit")
	assert.Equal(t, results[0], results[1], "both callers receive the same shared result")
}

func TestImageJobDedupKey_DistinguishesToolAndItem(t *testing.T) {
	assert.NotEqual(t, imageJobBriefKey(imageJobBrief{BusinessID: 7, Tool: "regenerate", Name: "Pizza"}), imageJobBriefKey(imageJobBrief{BusinessID: 7, Tool: "enhance", Name: "Pizza"}))
	assert.NotEqual(t, imageJobBriefKey(imageJobBrief{BusinessID: 7, Tool: "regenerate", Name: "Pizza"}), imageJobBriefKey(imageJobBrief{BusinessID: 8, Tool: "regenerate", Name: "Pizza"}))
	assert.NotEqual(t, imageJobBriefKey(imageJobBrief{BusinessID: 7, Tool: "regenerate", Name: "Pizza"}), imageJobBriefKey(imageJobBrief{BusinessID: 7, Tool: "regenerate", Name: "Burger"}))
}

func BenchmarkImageJobDedupKey(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = imageJobBriefKey(imageJobBrief{BusinessID: 7, Tool: "regenerate", Name: "Margherita Pizza"})
	}
}
