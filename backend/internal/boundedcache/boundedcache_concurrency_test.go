package boundedcache

import (
	"fmt"
	"sync"
	"testing"
	"time"
)

func TestCache_concurrentSetGetIsRaceFree(t *testing.T) {
	c := New[string, int](64, time.Minute)
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			k := fmt.Sprintf("k:%d", n%16)
			c.Set(k, n)
			_, _ = c.Get(k)
			if n%4 == 0 {
				c.InvalidatePrefix("k:")
			}
		}(i)
	}
	wg.Wait()
}
