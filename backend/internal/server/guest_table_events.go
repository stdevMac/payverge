package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/events"
	"github.com/stdevmac/payverge/backend/internal/middleware"

	"github.com/gin-gonic/gin"
)

// guestTableStreamsPerIP is generous on purpose: venue Wi-Fi NATs every diner
// behind one address. It matches the table hub's own per-IP cap
// (events.publicTableMaxPerIP) so neither guard is tighter than the other.
// Guests that hit it fall back to polling.
const guestTableStreamsPerIP = 60

// streamIPLimiter caps concurrent live streams per client IP so one client
// cannot hold the whole global pool. Venue Wi-Fi NATs every diner behind one
// IP, so the cap is generous; guests that hit it fall back to polling.
type streamIPLimiter struct {
	mu    sync.Mutex
	max   int
	count map[string]int
}

func newStreamIPLimiter(max int) *streamIPLimiter {
	return &streamIPLimiter{max: max, count: make(map[string]int)}
}

// acquire reserves one live stream for ip. ok is false when that IP is already
// at max (max <= 0 means unlimited). release is idempotent and drops the key
// once the count reaches zero.
func (l *streamIPLimiter) acquire(ip string) (release func(), ok bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.count == nil {
		l.count = make(map[string]int)
	}
	if l.max > 0 && l.count[ip] >= l.max {
		return nil, false
	}
	l.count[ip]++
	var once sync.Once
	release = func() {
		once.Do(func() {
			l.mu.Lock()
			defer l.mu.Unlock()
			if l.count[ip] <= 1 {
				delete(l.count, ip)
				return
			}
			l.count[ip]--
		})
	}
	return release, true
}

var guestTableStreamIPLimiter = newStreamIPLimiter(guestTableStreamsPerIP)

// GuestTableEvents exposes only table-scoped active-bill lifecycle signals.
func GuestTableEvents(c *gin.Context) {
	table, business, err := loadPublicGuestTableContext(c.Param("code"))
	if err != nil {
		respondPublicGuestTableLookupError(c, err)
		return
	}
	if code, message, locked := database.BusinessLockDenial(business); locked {
		writeGuestTableTerminal(c, code, message)
		return
	}
	clientIP := middleware.ClientRateLimitKey(c)
	release, ok := guestTableStreamIPLimiter.acquire(clientIP)
	if !ok {
		c.JSON(http.StatusTooManyRequests, gin.H{"error": "Too many live connections"})
		return
	}
	defer release()
	updates, cancel, err := events.GetTableHub().Subscribe(table.ID, business.ID, clientIP)
	if err != nil {
		// Subscribe's only failure is ErrTableConnectionLimit.
		writeGuestTableTerminal(c, "capacity", "Too many live connections")
		return
	}
	defer cancel()

	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache, no-store")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")
	flusher, ok := c.Writer.(http.Flusher)
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Streaming unsupported"})
		return
	}
	write := func(name string, payload events.TableBillChanged) bool {
		body, marshalErr := json.Marshal(payload)
		if marshalErr != nil {
			return false
		}
		if _, writeErr := fmt.Fprintf(c.Writer, "event: %s\ndata: %s\n\n", name, body); writeErr != nil {
			return false
		}
		flusher.Flush()
		return true
	}
	active, lookupErr := database.HasActiveBillForTableID(table.ID)
	if lookupErr != nil || !write("connected", events.TableBillChanged{HasActiveBill: active}) {
		return
	}

	heartbeat := time.NewTicker(20 * time.Second)
	defer heartbeat.Stop()
	for {
		select {
		case <-c.Request.Context().Done():
			return
		case update, open := <-updates:
			if !open || !write("table.bill_changed", update) {
				return
			}
		case <-heartbeat.C:
			if _, err := fmt.Fprint(c.Writer, ": heartbeat\n\n"); err != nil {
				return
			}
			flusher.Flush()
		}
	}
}

// writeGuestTableTerminal emits one SSE error frame and a 60s retry so
// EventSource does not reconnect immediately on a permanent gate or a full hub.
func writeGuestTableTerminal(c *gin.Context, code, message string) {
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache, no-store")
	c.Header("X-Accel-Buffering", "no")
	c.Status(http.StatusOK)
	payload, err := json.Marshal(struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	}{Code: code, Message: message})
	if err != nil {
		payload = []byte(`{"code":"error","message":"unavailable"}`)
	}
	_, _ = fmt.Fprintf(c.Writer, "retry: 60000\nevent: error\ndata: %s\n\n", payload)
	if flusher, ok := c.Writer.(http.Flusher); ok {
		flusher.Flush()
	}
}
