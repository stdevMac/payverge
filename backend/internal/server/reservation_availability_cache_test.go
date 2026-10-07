package server

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/services"
)

func TestReservationAvailabilityCacheHitsAndInvalidation(t *testing.T) {
	prevCompute := computeReservationAvailability
	prevTTL := reservationAvailabilityCacheTTL
	t.Cleanup(func() {
		computeReservationAvailability = prevCompute
		reservationAvailabilityCacheTTL = prevTTL
		resetReservationAvailabilityCache()
	})
	resetReservationAvailabilityCache()
	reservationAvailabilityCacheTTL = 20 * time.Second

	var calls atomic.Int32
	var seen []int
	date := time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC)
	business := &database.Business{ID: 42}
	computeReservationAvailability = func(_ *database.Business, _ time.Time, partySize int) (*services.ReservationAvailabilityDTO, error) {
		calls.Add(1)
		seen = append(seen, partySize)
		return &services.ReservationAvailabilityDTO{Date: "2030-01-01", PartySize: partySize}, nil
	}

	lookup := func(partySize int) {
		t.Helper()
		got, err := reservationAvailabilityFromCache(business, date, "2030-01-01", partySize)
		require.NoError(t, err)
		require.Equal(t, partySize, got.PartySize, "party size must reach the service unchanged")
	}

	lookup(2)
	lookup(2)
	require.Equal(t, int32(1), calls.Load(), "same key within TTL computes once")

	lookup(4)
	require.Equal(t, int32(2), calls.Load(), "a different party size computes again")

	// A venue may set max_party_size above any fixed clamp (banquet halls);
	// a party of 80 must be answered for 80, not silently for a smaller party.
	lookup(80)
	require.Equal(t, int32(3), calls.Load(), "a large party size is its own key")
	require.Equal(t, []int{2, 4, 80}, seen)

	invalidateReservationAvailability(99)
	lookup(4)
	require.Equal(t, int32(3), calls.Load(), "another business does not drop this cache")

	invalidateReservationAvailability(business.ID)
	lookup(4)
	require.Equal(t, int32(4), calls.Load(), "invalidate forces the next lookup to recompute")

	reservationAvailabilityCacheTTL = time.Millisecond
	time.Sleep(5 * time.Millisecond)
	lookup(4)
	require.Equal(t, int32(5), calls.Load(), "expired entries recompute")
}

func TestReservationAvailabilityCacheDoesNotCacheErrors(t *testing.T) {
	prevCompute := computeReservationAvailability
	t.Cleanup(func() {
		computeReservationAvailability = prevCompute
		resetReservationAvailabilityCache()
	})
	resetReservationAvailabilityCache()

	var calls atomic.Int32
	computeReservationAvailability = func(_ *database.Business, _ time.Time, _ int) (*services.ReservationAvailabilityDTO, error) {
		calls.Add(1)
		return nil, errors.New("party size must be between 1 and 20")
	}
	business := &database.Business{ID: 7}
	date := time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC)
	for i := 0; i < 3; i++ {
		_, err := reservationAvailabilityFromCache(business, date, "2030-01-01", 0)
		require.Error(t, err)
	}
	require.Equal(t, int32(3), calls.Load(), "errors (e.g. out-of-range party size) must not be cached")
	reservationAvailabilityCacheMu.Lock()
	defer reservationAvailabilityCacheMu.Unlock()
	require.Empty(t, reservationAvailabilityCache, "a rejected party size must not occupy a cache key")
}

func TestGetReservationAvailability_PassesLargePartySizeThrough(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupPublicBusinessHandlerTestDB(t)
	business := createPublicBusinessRouteTestBusiness(t, "avail-large-party-venue", true, true)
	services.InvalidateBusinessCustomURL(business.CustomURL)

	prevCompute := computeReservationAvailability
	t.Cleanup(func() {
		computeReservationAvailability = prevCompute
		resetReservationAvailabilityCache()
	})
	resetReservationAvailabilityCache()

	var got atomic.Int32
	computeReservationAvailability = func(_ *database.Business, _ time.Time, partySize int) (*services.ReservationAvailabilityDTO, error) {
		got.Store(int32(partySize))
		return &services.ReservationAvailabilityDTO{
			Date:           "2030-01-01",
			PartySize:      partySize,
			AvailableSlots: []services.ReservationAvailabilitySlotDTO{},
		}, nil
	}

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "customUrl", Value: business.CustomURL}}
	c.Request = httptest.NewRequest(http.MethodGet, "/?date=2030-01-01&party_size=80", nil)
	GetReservationAvailability(c)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	require.Equal(t, int32(80), got.Load(), "handler must not clamp a venue-valid party size")
}

func TestGetReservationAvailability_CachesWithinTTL(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupPublicBusinessHandlerTestDB(t)
	business := createPublicBusinessRouteTestBusiness(t, "avail-cache-venue", true, true)
	services.InvalidateBusinessCustomURL(business.CustomURL)

	prevCompute := computeReservationAvailability
	prevTTL := reservationAvailabilityCacheTTL
	t.Cleanup(func() {
		computeReservationAvailability = prevCompute
		reservationAvailabilityCacheTTL = prevTTL
		resetReservationAvailabilityCache()
	})
	resetReservationAvailabilityCache()
	reservationAvailabilityCacheTTL = 20 * time.Second

	var calls atomic.Int32
	computeReservationAvailability = func(_ *database.Business, _ time.Time, partySize int) (*services.ReservationAvailabilityDTO, error) {
		calls.Add(1)
		return &services.ReservationAvailabilityDTO{
			Date:           "2030-01-01",
			PartySize:      partySize,
			AvailableSlots: []services.ReservationAvailabilitySlotDTO{},
		}, nil
	}

	call := func() {
		t.Helper()
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Params = gin.Params{{Key: "customUrl", Value: business.CustomURL}}
		c.Request = httptest.NewRequest(http.MethodGet, "/?date=2030-01-01&party_size=2", nil)
		GetReservationAvailability(c)
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	}
	call()
	call()
	require.Equal(t, int32(1), calls.Load())
}

func TestReservationAvailabilityCacheSweepDoesNotEvictOtherVenues(t *testing.T) {
	prevCompute := computeReservationAvailability
	t.Cleanup(func() {
		computeReservationAvailability = prevCompute
		resetReservationAvailabilityCache()
	})
	resetReservationAvailabilityCache()

	var calls atomic.Int32
	computeReservationAvailability = func(_ *database.Business, _ time.Time, partySize int) (*services.ReservationAvailabilityDTO, error) {
		calls.Add(1)
		return &services.ReservationAvailabilityDTO{PartySize: partySize}, nil
	}
	date := time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC)
	victim := &database.Business{ID: 1}
	attacker := &database.Business{ID: 2}

	_, err := reservationAvailabilityFromCache(victim, date, "2030-01-01", 2)
	require.NoError(t, err)

	// Sweep far past the per-venue bound with distinct dates.
	for i := 0; i < reservationAvailabilityCacheMaxPerBusiness+200; i++ {
		day := date.AddDate(0, 0, i)
		_, err := reservationAvailabilityFromCache(attacker, day, day.Format("2006-01-02"), 2)
		require.NoError(t, err)
	}

	reservationAvailabilityCacheMu.Lock()
	attackerEntries := reservationAvailabilityBusinessCount[attacker.ID]
	total := len(reservationAvailabilityCache)
	reservationAvailabilityCacheMu.Unlock()
	require.Equal(t, reservationAvailabilityCacheMaxPerBusiness, attackerEntries, "one venue is bounded to its share")
	require.Equal(t, reservationAvailabilityCacheMaxPerBusiness+1, total)

	before := calls.Load()
	_, err = reservationAvailabilityFromCache(victim, date, "2030-01-01", 2)
	require.NoError(t, err)
	require.Equal(t, before, calls.Load(), "another venue's fresh entry must survive the sweep")

	invalidateReservationAvailability(attacker.ID)
	reservationAvailabilityCacheMu.Lock()
	_, stillCounted := reservationAvailabilityBusinessCount[attacker.ID]
	remaining := len(reservationAvailabilityCache)
	reservationAvailabilityCacheMu.Unlock()
	require.False(t, stillCounted, "invalidate must clear the per-venue count")
	require.Equal(t, 1, remaining)
}

func TestReservationAvailabilityCacheCollapsesConcurrentMisses(t *testing.T) {
	prevCompute := computeReservationAvailability
	t.Cleanup(func() {
		computeReservationAvailability = prevCompute
		resetReservationAvailabilityCache()
	})
	resetReservationAvailabilityCache()

	var calls atomic.Int32
	release := make(chan struct{})
	computeReservationAvailability = func(_ *database.Business, _ time.Time, partySize int) (*services.ReservationAvailabilityDTO, error) {
		calls.Add(1)
		<-release
		return &services.ReservationAvailabilityDTO{PartySize: partySize}, nil
	}
	business := &database.Business{ID: 9}
	date := time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC)

	const callers = 25
	errs := make(chan error, callers)
	for i := 0; i < callers; i++ {
		go func() {
			_, err := reservationAvailabilityFromCache(business, date, "2030-01-01", 2)
			errs <- err
		}()
	}
	require.Eventually(t, func() bool { return calls.Load() == 1 }, time.Second, time.Millisecond)
	time.Sleep(20 * time.Millisecond)
	close(release)
	for i := 0; i < callers; i++ {
		require.NoError(t, <-errs)
	}
	require.Equal(t, int32(1), calls.Load(), "concurrent misses for one key share one computation")
}

func TestReservationAvailabilityInvalidateDuringComputeStartsFreshFlight(t *testing.T) {
	prevCompute := computeReservationAvailability
	t.Cleanup(func() {
		computeReservationAvailability = prevCompute
		resetReservationAvailabilityCache()
	})
	resetReservationAvailabilityCache()

	var calls atomic.Int32
	firstStarted := make(chan struct{})
	releaseFirst := make(chan struct{})
	computeReservationAvailability = func(_ *database.Business, _ time.Time, partySize int) (*services.ReservationAvailabilityDTO, error) {
		n := calls.Add(1)
		if n == 1 {
			close(firstStarted)
			<-releaseFirst
			return &services.ReservationAvailabilityDTO{PartySize: partySize, TotalSlots: 1}, nil
		}
		return &services.ReservationAvailabilityDTO{PartySize: partySize, TotalSlots: 2}, nil
	}
	business := &database.Business{ID: 11}
	date := time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC)

	done := make(chan *services.ReservationAvailabilityDTO, 1)
	go func() {
		got, _ := reservationAvailabilityFromCache(business, date, "2030-01-01", 2)
		done <- got
	}()
	<-firstStarted
	invalidateReservationAvailability(business.ID)

	fresh, err := reservationAvailabilityFromCache(business, date, "2030-01-01", 2)
	require.NoError(t, err)
	require.Equal(t, 2, fresh.TotalSlots, "a caller after invalidation must not join the stale flight")

	close(releaseFirst)
	stale := <-done
	require.Equal(t, 1, stale.TotalSlots)

	cached, err := reservationAvailabilityFromCache(business, date, "2030-01-01", 2)
	require.NoError(t, err)
	require.Equal(t, 2, cached.TotalSlots, "the pre-invalidation result must not overwrite the cache")
	require.Equal(t, int32(2), calls.Load())
}

// TestReservationAvailabilityInvalidationWiring pins every owner/staff write
// that changes bookable slots to the availability-cache invalidation.
func TestReservationAvailabilityInvalidationWiring(t *testing.T) {
	cases := map[string][]string{
		"business_settings_handlers.go": {"UpdateBusinessOperatingHours", "UpdateBusinessOperatingExceptions"},
		"table_handlers.go":             {"UpdateTable", "CreateTableWithQR", "UpdateTableDetails", "DeleteTableSoft"},
		"space_handlers.go":             {"PublishSpaceLayout"},
		"business_handlers.go":          {"UpdateBusiness"},
		"table_floor_handlers.go":       {"SeatTable", "ClearTable", "TransferTable", "MergeTable", "seatReservationFromFloor"},
	}
	for file, funcs := range cases {
		src, err := os.ReadFile(file)
		require.NoError(t, err)
		text := string(src)
		for _, fn := range funcs {
			start := strings.Index(text, "\nfunc "+fn+"(")
			require.GreaterOrEqualf(t, start, 0, "%s: %s not found", file, fn)
			body := text[start+1:]
			if end := strings.Index(body, "\n}\n"); end >= 0 {
				body = body[:end]
			}
			require.Containsf(t, body, "invalidateReservationAvailability(", "%s: %s must invalidate reservation availability", file, fn)
		}
	}
}
