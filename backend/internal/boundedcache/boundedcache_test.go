package boundedcache

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestSetGet_hitBeforeTTL(t *testing.T) {
	c := New[string, int](100, time.Minute)
	c.Set("a", 1)
	v, ok := c.Get("a")
	require.True(t, ok)
	require.Equal(t, 1, v)
}

func TestGet_missAfterTTL(t *testing.T) {
	c := New[string, int](100, 10*time.Millisecond)
	c.Set("a", 1)
	time.Sleep(20 * time.Millisecond)
	_, ok := c.Get("a")
	require.False(t, ok, "entry must expire after TTL")
}

func TestSet_evictsLeastRecentlyUsedAtCap(t *testing.T) {
	c := New[string, int](2, time.Minute)
	c.Set("a", 1)
	c.Set("b", 2)
	_, _ = c.Get("a") // touch a so b is now LRU
	c.Set("c", 3)     // over cap -> evict b
	_, okA := c.Get("a")
	_, okB := c.Get("b")
	_, okC := c.Get("c")
	require.True(t, okA)
	require.False(t, okB, "LRU entry b must be evicted")
	require.True(t, okC)
}

func TestInvalidate_and_InvalidatePrefix(t *testing.T) {
	c := New[string, int](100, time.Minute)
	c.Set("biz:1:menu", 1)
	c.Set("biz:1:offers", 2)
	c.Set("biz:2:menu", 3)
	c.Invalidate("biz:1:menu")
	_, ok := c.Get("biz:1:menu")
	require.False(t, ok)
	c.InvalidatePrefix("biz:1:")
	_, ok = c.Get("biz:1:offers")
	require.False(t, ok)
	_, ok = c.Get("biz:2:menu")
	require.True(t, ok, "other-business keys must survive prefix invalidation")
}

func TestInvalidateFuncRemovesMatchingValues(t *testing.T) {
	c := New[string, int](10, time.Minute)
	c.Set("a", 1)
	c.Set("b", 2)
	c.Set("c", 1)

	c.InvalidateFunc(func(v int) bool { return v == 1 })

	_, okA := c.Get("a")
	_, okB := c.Get("b")
	_, okC := c.Get("c")
	require.False(t, okA)
	require.True(t, okB)
	require.False(t, okC)
	require.Equal(t, 1, c.Len())
}
