package server

import (
	"strconv"
	"sync"
	"time"

	"golang.org/x/sync/singleflight"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/services"
)

// reservationAvailabilityCacheMaxEntries bounds the whole cache and
// reservationAvailabilityCacheMaxPerBusiness bounds one venue's share of it, so
// a date/party-size sweep against one venue cannot evict other venues' entries.
// When a bound is reached after dropping expired rows, the new result is served
// but not stored; nothing fresh is ever flushed to make room.
const (
	reservationAvailabilityCacheMaxEntries     = 10000
	reservationAvailabilityCacheMaxPerBusiness = 512
)

// reservationAvailabilityCacheTTL is how long a successful public availability
// response is reused. Tests shorten it; freshness is measured from storedAt
// against the current value, so a shorter TTL expires entries already stored.
var reservationAvailabilityCacheTTL = 20 * time.Second

// computeReservationAvailability is the availability seam. Tests replace it.
var computeReservationAvailability = func(business *database.Business, date time.Time, partySize int) (*services.ReservationAvailabilityDTO, error) {
	return getReservationService().GetPublicAvailability(business, date, partySize)
}

type reservationAvailabilityCacheKey struct {
	businessID uint
	date       string
	partySize  int
}

type reservationAvailabilityCacheEntry struct {
	value    *services.ReservationAvailabilityDTO
	storedAt time.Time
}

var (
	reservationAvailabilityCacheMu sync.Mutex
	reservationAvailabilityCache   = map[reservationAvailabilityCacheKey]reservationAvailabilityCacheEntry{}
	// reservationAvailabilityEpoch bumps on invalidate so an in-flight compute
	// cannot repopulate a business it just dropped.
	reservationAvailabilityEpoch = map[uint]uint64{}
	// reservationAvailabilityBusinessCount tracks entries per business.
	reservationAvailabilityBusinessCount = map[uint]int{}
	// reservationAvailabilityFlight collapses concurrent misses for one key.
	reservationAvailabilityFlight singleflight.Group
)

func reservationAvailabilityFresh(entry reservationAvailabilityCacheEntry, now time.Time) bool {
	return now.Sub(entry.storedAt) < reservationAvailabilityCacheTTL
}

// reservationAvailabilityFromCache returns a cached successful availability
// result for the business, calendar date, and party size. The party size is
// passed through unchanged: the service rejects sizes outside the venue's
// min/max (MaxPartySize is owner-defined and may exceed any fixed clamp), and
// errors are never cached, so only in-range sizes can occupy cache keys.
func reservationAvailabilityFromCache(business *database.Business, requestedDate time.Time, dateKey string, partySize int) (*services.ReservationAvailabilityDTO, error) {
	key := reservationAvailabilityCacheKey{
		businessID: business.ID,
		date:       dateKey,
		partySize:  partySize,
	}
	now := time.Now()

	reservationAvailabilityCacheMu.Lock()
	epoch := reservationAvailabilityEpoch[business.ID]
	if entry, ok := reservationAvailabilityCache[key]; ok && reservationAvailabilityFresh(entry, now) {
		reservationAvailabilityCacheMu.Unlock()
		return entry.value, nil
	}
	reservationAvailabilityCacheMu.Unlock()

	// The epoch is part of the flight key so a caller arriving after an
	// invalidation never joins a computation that started before it.
	flightKey := strconv.FormatUint(uint64(business.ID), 10) + "|" + dateKey + "|" +
		strconv.Itoa(partySize) + "|" + strconv.FormatUint(epoch, 10)
	value, err, _ := reservationAvailabilityFlight.Do(flightKey, func() (any, error) {
		result, err := computeReservationAvailability(business, requestedDate, partySize)
		if err != nil {
			return nil, err
		}
		storeReservationAvailability(key, epoch, result)
		return result, nil
	})
	if err != nil {
		return nil, err
	}
	return value.(*services.ReservationAvailabilityDTO), nil
}

// storeReservationAvailability caches result unless the business was
// invalidated while it was computed or a bound is reached.
func storeReservationAvailability(key reservationAvailabilityCacheKey, epoch uint64, result *services.ReservationAvailabilityDTO) {
	reservationAvailabilityCacheMu.Lock()
	defer reservationAvailabilityCacheMu.Unlock()
	if reservationAvailabilityEpoch[key.businessID] != epoch {
		return
	}
	now := time.Now()
	if _, exists := reservationAvailabilityCache[key]; !exists {
		if reservationAvailabilityBusinessCount[key.businessID] >= reservationAvailabilityCacheMaxPerBusiness {
			pruneExpiredReservationAvailability(now, key.businessID, true)
			if reservationAvailabilityBusinessCount[key.businessID] >= reservationAvailabilityCacheMaxPerBusiness {
				return
			}
		}
		if len(reservationAvailabilityCache) >= reservationAvailabilityCacheMaxEntries {
			pruneExpiredReservationAvailability(now, 0, false)
			if len(reservationAvailabilityCache) >= reservationAvailabilityCacheMaxEntries {
				return
			}
		}
		reservationAvailabilityBusinessCount[key.businessID]++
	}
	reservationAvailabilityCache[key] = reservationAvailabilityCacheEntry{
		value:    result,
		storedAt: now,
	}
}

// pruneExpiredReservationAvailability drops expired entries, optionally only
// those of one business. The caller holds reservationAvailabilityCacheMu.
func pruneExpiredReservationAvailability(now time.Time, businessID uint, onlyBusiness bool) {
	for cachedKey, entry := range reservationAvailabilityCache {
		if onlyBusiness && cachedKey.businessID != businessID {
			continue
		}
		if !reservationAvailabilityFresh(entry, now) {
			deleteReservationAvailabilityEntry(cachedKey)
		}
	}
}

// deleteReservationAvailabilityEntry removes one key and keeps the per-business
// count in step. The caller holds reservationAvailabilityCacheMu.
func deleteReservationAvailabilityEntry(key reservationAvailabilityCacheKey) {
	if _, ok := reservationAvailabilityCache[key]; !ok {
		return
	}
	delete(reservationAvailabilityCache, key)
	if reservationAvailabilityBusinessCount[key.businessID] <= 1 {
		delete(reservationAvailabilityBusinessCount, key.businessID)
		return
	}
	reservationAvailabilityBusinessCount[key.businessID]--
}

// invalidateReservationAvailability drops every cached availability row for a business.
func invalidateReservationAvailability(businessID uint) {
	reservationAvailabilityCacheMu.Lock()
	defer reservationAvailabilityCacheMu.Unlock()
	reservationAvailabilityEpoch[businessID]++
	if reservationAvailabilityBusinessCount[businessID] == 0 {
		return
	}
	for key := range reservationAvailabilityCache {
		if key.businessID == businessID {
			deleteReservationAvailabilityEntry(key)
		}
	}
}

// resetReservationAvailabilityCache drops the whole cache. Tests use it.
func resetReservationAvailabilityCache() {
	reservationAvailabilityCacheMu.Lock()
	defer reservationAvailabilityCacheMu.Unlock()
	reservationAvailabilityCache = map[reservationAvailabilityCacheKey]reservationAvailabilityCacheEntry{}
	reservationAvailabilityEpoch = map[uint]uint64{}
	reservationAvailabilityBusinessCount = map[uint]int{}
}
