//go:build whatsapp

package services

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/guardrails"
	"github.com/stdevmac/payverge/backend/internal/llm"
	"github.com/stdevmac/payverge/backend/internal/logger"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/store/sqlstore"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	waLog "go.mau.fi/whatsmeow/util/log"
	"golang.org/x/sync/singleflight"
	"google.golang.org/protobuf/proto"
)

// WhatsApp inbound-AI abuse controls. The inbound path drives model calls on
// the business's account, so it must be bounded just like the HTTP waiter
// (maxAIWaiterMessageBytes + BusinessRateLimit there).
const (
	maxWhatsAppMessageBytes = 4096
	whatsAppRateWindow      = time.Minute
	whatsAppRateMax         = 10 // AI replies per sender per window
)

type WhatsAppManager struct {
	db        *database.DB
	container *sqlstore.Container // retained for production sqlstore construction
	devices   whatsAppDeviceStore
	clients   map[uint]whatsAppClient
	handlers  map[uint]uint32 // businessID -> registered handler id (dedupe restore)
	pendingQR map[uint]string // businessID -> latest unscanned pairing code
	// busGen bumps when Disconnect, Stop, or a superseding attempt drops a
	// business's client. An in-flight dial captured the old value and must
	// not publish over the newer state. Guarded by mu.
	busGen map[uint]uint64
	mu     sync.RWMutex
	// connectFlight collapses concurrent ConnectBusiness calls per business
	// so they share one dial instead of constructing two clients.
	connectFlight singleflight.Group
	aiService     *AIService
	classifier    guardrails.InputClassifier // inbound-turn screen; AllowAll until WithClassifier
	costGate      *llm.AICostGate            // per-business daily USD ceiling; nil-safe

	rateMu   sync.Mutex
	rateHits map[string][]time.Time // sender JID -> recent allowed timestamps

	done     chan struct{} // closed by Stop() to signal the sweep loop to exit
	stopOnce sync.Once     // guards done against a double close

	// now is injectable for tests; defaults to time.Now.
	now func() time.Time
}

// allowSender applies a per-sender sliding-window rate limit so a single
// chat/number cannot drive unbounded AI calls. Process-local (matches the
// existing in-memory limiter); a Redis-backed limiter is needed for multi-replica.
func (wm *WhatsAppManager) allowSender(sender string) bool {
	now := time.Now()
	cutoff := now.Add(-whatsAppRateWindow)

	wm.rateMu.Lock()
	defer wm.rateMu.Unlock()
	if wm.rateHits == nil {
		wm.rateHits = make(map[string][]time.Time)
	}
	kept := wm.rateHits[sender][:0]
	for _, ts := range wm.rateHits[sender] {
		if ts.After(cutoff) {
			kept = append(kept, ts)
		}
	}
	if len(kept) >= whatsAppRateMax {
		wm.rateHits[sender] = kept
		return false
	}
	wm.rateHits[sender] = append(kept, now)
	return true
}

// sweepOnce prunes expired timestamps from every sender's slice and deletes
// keys whose window has fully drained, so the rateHits map cannot grow one
// entry per unique inbound JID forever. Mirrors auth.RateLimiter.sweepLoop's
// per-window eviction; reuses rateMu so it never races allowSender.
func (wm *WhatsAppManager) sweepOnce() {
	cutoff := time.Now().Add(-whatsAppRateWindow)

	wm.rateMu.Lock()
	defer wm.rateMu.Unlock()
	for sender, hits := range wm.rateHits {
		kept := hits[:0]
		for _, ts := range hits {
			if ts.After(cutoff) {
				kept = append(kept, ts)
			}
		}
		if len(kept) == 0 {
			delete(wm.rateHits, sender)
		} else {
			wm.rateHits[sender] = kept
		}
	}
}

// sweepLoop runs sweepOnce every window until Stop() closes wm.done, mirroring
// auth.RateLimiter.sweepLoop. Watching done lets graceful shutdown reclaim this
// goroutine instead of leaking it for the lifetime of the process.
func (wm *WhatsAppManager) sweepLoop() {
	ticker := time.NewTicker(whatsAppRateWindow)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			wm.sweepOnce()
		case <-wm.done:
			return
		}
	}
}

// Stop signals the sweep goroutine to exit and disconnects all active WhatsApp
// clients so a graceful shutdown leaves no leaked goroutine or open socket.
// Safe to call multiple times (the done close is guarded by stopOnce).
// Clients are snapshotted under the lock; Disconnect runs after it is released.
func (wm *WhatsAppManager) Stop() {
	wm.stopOnce.Do(func() {
		close(wm.done)
	})
	wm.mu.Lock()
	clients := make([]whatsAppClient, 0, len(wm.clients))
	for id, client := range wm.clients {
		if client != nil {
			clients = append(clients, client)
		}
		delete(wm.clients, id)
		delete(wm.handlers, id)
		delete(wm.pendingQR, id)
		wm.bumpBusGenLocked(id)
	}
	wm.mu.Unlock()

	if len(clients) == 0 {
		return
	}
	var wg sync.WaitGroup
	for _, client := range clients {
		wg.Add(1)
		go func(c whatsAppClient) {
			defer wg.Done()
			disconnectBounded(c, whatsAppConnectTimeout)
		}(client)
	}
	waitGroupBounded(&wg, whatsAppConnectTimeout+time.Second)
}

func NewWhatsAppManager(db *database.DB, aiService *AIService) (*WhatsAppManager, error) {
	// Initialize SQL store
	sqlDB, err := db.GetDB().DB()
	if err != nil {
		return nil, fmt.Errorf("failed to get sql db: %w", err)
	}

	// Create container for session storage
	// Using "postgres" as the dialect. The store name is configurable via
	// WHATSAPP_STORE_NAME (B14); the default keeps existing deployments intact.
	storeName := os.Getenv("WHATSAPP_STORE_NAME")
	if storeName == "" {
		storeName = "payverge_whatsapp_store"
	}
	container, err := sqlstore.New(context.Background(), "postgres", storeName, waLog.Stdout("database", "DEBUG", true))
	if err != nil {
		// Provide a fallback if direct container creation fails (e.g. if we need to pass the *sql.DB directly)
		// Note: The library usually expects a connection string or pre-configured logic.
		// Since we have an existing *sql.DB, we might need a custom wrapper or just use the dsn style if possible.
		// But `sqlstore.New` connects internally. Let's try to pass the existing connection if the library allows,
		// otherwise we have to share the DSN. GORM DSN is likely what we want.
		// For now, let's assume standard New works, but if we need to reuse the connection object, we might need to look at `sqlstore.NewWithDB`.
		// Let's assume NewWithDB exists or we can inject it. If not, we fall back to generic New.
		// Actually, standard whatsmeow sqlstore.New takes a DSN. We don't want to open a second pool if possible,
		// but simple logic suggests using the same DSN string is safest.
		// However, to keep it simple and safe within this file without knowing the DSN string (struct has it),
		// let's try to assume we can get the DSN or we use a wrapper.
		// Actually, `sqlstore.NewWithDB` is available in newer versions.
		container = sqlstore.NewWithDB(sqlDB, "postgres", waLog.Stdout("database", "INFO", true))
	}

	manager := &WhatsAppManager{
		db:         db,
		container:  container,
		devices:    &sqlstoreDeviceStore{container: container},
		clients:    make(map[uint]whatsAppClient),
		handlers:   make(map[uint]uint32),
		pendingQR:  make(map[uint]string),
		aiService:  aiService,
		classifier: guardrails.AllowAll{},
		rateHits:   make(map[string][]time.Time),
		done:       make(chan struct{}),
		now:        time.Now,
	}

	// Evict idle sender keys so rateHits cannot grow one entry per unique
	// inbound JID forever. Process-lifetime goroutine; no stop needed.
	logger.SafeGo(manager.sweepLoop)

	return manager, nil
}

// newWhatsAppManagerForTest builds a manager with injectable fakes (no live store).
func newWhatsAppManagerForTest(devices whatsAppDeviceStore, ai *AIService) *WhatsAppManager {
	return &WhatsAppManager{
		devices:    devices,
		clients:    make(map[uint]whatsAppClient),
		handlers:   make(map[uint]uint32),
		pendingQR:  make(map[uint]string),
		aiService:  ai,
		classifier: guardrails.AllowAll{},
		rateHits:   make(map[string][]time.Time),
		done:       make(chan struct{}),
		now:        time.Now,
	}
}

// NewWhatsAppManagerForHermetic builds a manager with an injectable device store
// for hermetic scenario contracts (no live WhatsApp account).
func NewWhatsAppManagerForHermetic(devices HermeticDeviceStore, ai *AIService) *WhatsAppManager {
	return newWhatsAppManagerForTest(devices, ai)
}

// HandlerRegistrationCount returns how many event handlers are registered for a
// business (used by hermetic restore scenarios to assert no duplicates).
func (wm *WhatsAppManager) HandlerRegistrationCount(businessID uint) int {
	wm.mu.RLock()
	defer wm.mu.RUnlock()
	if _, ok := wm.handlers[businessID]; ok {
		return 1
	}
	return 0
}

const (
	whatsAppSendTimeout     = 10 * time.Second
	whatsAppRestoreWorkers  = 4
	whatsAppMaxRetryBackoff = 15 * time.Minute
	// whatsAppHandlerPending reserves a handler slot without calling into the
	// client while wm.mu is held. AddEventHandler fills the real id after.
	whatsAppHandlerPending uint32 = ^uint32(0)
)

// whatsAppConnectTimeout bounds every WhatsApp dial, logout, and disconnect
// wait. Tests shrink it.
var whatsAppConnectTimeout = 20 * time.Second

// errWhatsAppDialTimeout is returned when Connect does not finish before
// whatsAppConnectTimeout. The late Connect is disconnected when it returns.
var errWhatsAppDialTimeout = errors.New("whatsapp connect timed out")

// whatsappDial is the connect seam. Production bounds client.Connect; tests
// replace it to count, block, or hang a dial. Callers must not hold wm.mu.
var whatsappDial = func(ctx context.Context, client whatsAppClient) error {
	return dialWhatsApp(ctx, client, whatsAppConnectTimeout)
}

// dialWhatsApp runs client.Connect without the manager lock. Connect has no
// context parameter, so the call sits in a goroutine and this stops waiting
// after timeout (or ctx's deadline, whichever is sooner). A Connect that
// finishes late is disconnected so it cannot leave an orphaned socket.
func dialWhatsApp(ctx context.Context, client whatsAppClient, timeout time.Duration) error {
	if client == nil {
		return fmt.Errorf("whatsapp client is nil")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if timeout <= 0 {
		timeout = 20 * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	errCh := make(chan error, 1)
	go func() {
		errCh <- client.Connect()
	}()
	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		go func() {
			<-errCh
			client.Disconnect()
		}()
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return errWhatsAppDialTimeout
		}
		return ctx.Err()
	}
}

func isWhatsAppDialTimeout(err error) bool {
	return errors.Is(err, errWhatsAppDialTimeout) ||
		errors.Is(err, context.DeadlineExceeded) ||
		errors.Is(err, context.Canceled)
}

// logoutBounded runs Logout outside the manager lock and stops waiting after
// timeout. A Logout that ignores cancellation is disconnected once it returns.
func logoutBounded(client whatsAppClient, timeout time.Duration) error {
	if client == nil {
		return nil
	}
	if timeout <= 0 {
		timeout = whatsAppConnectTimeout
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	errCh := make(chan error, 1)
	go func() {
		errCh <- client.Logout(ctx)
	}()
	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		go func() {
			<-errCh
			client.Disconnect()
		}()
		return errWhatsAppDialTimeout
	}
}

// disconnectBounded waits for Disconnect up to timeout. Disconnect cannot be
// cancelled; the caller stops waiting and the call finishes in the background.
func disconnectBounded(client whatsAppClient, timeout time.Duration) {
	if client == nil {
		return
	}
	if timeout <= 0 {
		timeout = whatsAppConnectTimeout
	}
	done := make(chan struct{})
	go func() {
		client.Disconnect()
		close(done)
	}()
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case <-done:
	case <-timer.C:
	}
}

func waitGroupBounded(wg *sync.WaitGroup, timeout time.Duration) {
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case <-done:
	case <-timer.C:
	}
}

// RestoreAllSessions reconnects mapped business devices after process restart.
// Only rows in whatsapp_business_devices are restored — unmapped whatsmeow
// devices are never guessed by business name.
func (wm *WhatsAppManager) RestoreAllSessions() error {
	return wm.RestoreAllSessionsCtx(context.Background())
}

// RestoreAllSessionsCtx is the context-aware restore entry point.
func (wm *WhatsAppManager) RestoreAllSessionsCtx(ctx context.Context) error {
	if wm.devices == nil {
		return fmt.Errorf("whatsapp device store not configured")
	}
	now := wm.clock()
	mappings, err := database.ListWhatsAppBusinessDevicesForRestore(now, 500)
	if err != nil {
		return err
	}
	if len(mappings) == 0 {
		return nil
	}

	// Bounded worker pool — max 4 concurrent restores.
	jobs := make(chan database.WhatsAppBusinessDevice)
	var wg sync.WaitGroup
	workers := whatsAppRestoreWorkers
	if workers > len(mappings) {
		workers = len(mappings)
	}
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for m := range jobs {
				wm.restoreOne(ctx, m)
			}
		}()
	}
	for _, m := range mappings {
		select {
		case <-ctx.Done():
			close(jobs)
			wg.Wait()
			return ctx.Err()
		case jobs <- m:
		}
	}
	close(jobs)
	wg.Wait()

	// Diagnostic: unmapped store devices are reported once, never auto-linked.
	if storeJIDs, lerr := wm.devices.ListDeviceJIDs(ctx); lerr == nil {
		mapped := make(map[string]bool, len(mappings))
		for _, m := range mappings {
			mapped[m.DeviceJID] = true
		}
		for _, jid := range storeJIDs {
			if !mapped[jid] {
				log.Printf("whatsapp: ignoring unmapped store device jid_ref=%s", whatsappSenderRef(jid))
			}
		}
	}
	return nil
}

func (wm *WhatsAppManager) restoreOne(ctx context.Context, mapping database.WhatsAppBusinessDevice) {
	businessID := mapping.BusinessID
	if wm.isStopped() {
		return
	}
	// Skip if already live with a handler (idempotent restore). Snapshot under
	// the lock; IsConnected / the dial run after it is released.
	wm.mu.RLock()
	existing, hasClient := wm.clients[businessID]
	_, hasHandler := wm.handlers[businessID]
	gen := wm.busGen[businessID]
	wm.mu.RUnlock()
	if hasClient && hasHandler && existing != nil && existing.IsConnected() && existing.IsLoggedIn() {
		return
	}

	client, err := wm.devices.ClientForJID(ctx, mapping.DeviceJID)
	if err != nil {
		log.Printf("whatsapp restore: business=%d jid missing err=%v", businessID, err)
		_ = database.MarkWhatsAppDeviceStatus(businessID, database.WhatsAppDeviceStatusDegraded, "device_missing", nil, wm.nextRetryAt(mapping.ReconnectAttempts+1), int64Ptr(mapping.ReconnectAttempts+1))
		return
	}

	wm.registerHandlerOnce(businessID, client)

	if err := whatsappDial(ctx, client); err != nil {
		attempts := mapping.ReconnectAttempts + 1
		if isWhatsAppDialTimeout(err) {
			_ = database.MarkWhatsAppDeviceStatus(businessID, database.WhatsAppDeviceStatusDegraded, "connect_timeout", nil, wm.nextRetryAt(attempts), &attempts)
			return
		}
		log.Printf("whatsapp restore: business=%d connect failed: %v", businessID, err)
		_ = database.MarkWhatsAppDeviceStatus(businessID, database.WhatsAppDeviceStatusDegraded, "connect_failed", nil, wm.nextRetryAt(attempts), &attempts)
		return
	}

	if !wm.installRestoredClient(businessID, client, gen) {
		return
	}

	now := wm.clock()
	zero := int64(0)
	_ = database.MarkWhatsAppDeviceStatus(businessID, database.WhatsAppDeviceStatusConnected, "", &now, nil, &zero)
}

// installRestoredClient publishes client only when gen is still the snapshot
// taken before the dial and no other attempt owns the business.
func (wm *WhatsAppManager) installRestoredClient(businessID uint, client whatsAppClient, gen uint64) bool {
	wm.mu.Lock()
	superseded := wm.isStopped() || wm.busGen[businessID] != gen
	if !superseded {
		if current, ok := wm.clients[businessID]; ok && current != nil && current != client {
			superseded = true
		}
	}
	if superseded {
		wm.mu.Unlock()
		disconnectBounded(client, whatsAppConnectTimeout)
		return false
	}
	if wm.clients == nil {
		wm.clients = make(map[uint]whatsAppClient)
	}
	wm.clients[businessID] = client
	wm.mu.Unlock()
	return true
}

func (wm *WhatsAppManager) registerHandlerOnce(businessID uint, client whatsAppClient) {
	wm.mu.Lock()
	if wm.handlers == nil {
		wm.handlers = make(map[uint]uint32)
	}
	if _, ok := wm.handlers[businessID]; ok {
		wm.mu.Unlock()
		return
	}
	// Reserve the slot, then drop the lock before calling into the client.
	wm.handlers[businessID] = whatsAppHandlerPending
	wm.mu.Unlock()

	id := client.AddEventHandler(func(evt any) {
		wm.handleEvent(businessID, evt)
	})

	wm.mu.Lock()
	if cur, ok := wm.handlers[businessID]; ok && cur == whatsAppHandlerPending {
		wm.handlers[businessID] = id
	}
	wm.mu.Unlock()
}

// bumpBusGenLocked invalidates in-flight dials for businessID. Caller holds wm.mu.
func (wm *WhatsAppManager) bumpBusGenLocked(businessID uint) {
	if wm.busGen == nil {
		wm.busGen = make(map[uint]uint64)
	}
	wm.busGen[businessID]++
}

func (wm *WhatsAppManager) isStopped() bool {
	if wm == nil || wm.done == nil {
		return false
	}
	select {
	case <-wm.done:
		return true
	default:
		return false
	}
}

func (wm *WhatsAppManager) nextRetryAt(attempt int64) *time.Time {
	// Exponential: 30s * 2^(attempt-1), capped at 15m.
	if attempt < 1 {
		attempt = 1
	}
	backoff := time.Duration(30*(1<<min64(attempt-1, 5))) * time.Second
	if backoff > whatsAppMaxRetryBackoff {
		backoff = whatsAppMaxRetryBackoff
	}
	t := wm.clock().Add(backoff)
	return &t
}

func (wm *WhatsAppManager) clock() time.Time {
	if wm.now != nil {
		return wm.now()
	}
	return time.Now().UTC()
}

func int64Ptr(v int64) *int64 { return &v }

// ConnectBusiness initializes a client for a business.
// Returns a channel for QR codes when pairing is required.
// Concurrent calls for the same business share one dial.
func (wm *WhatsAppManager) ConnectBusiness(businessID uint) (<-chan string, error) {
	key := strconv.FormatUint(uint64(businessID), 10)
	v, err, _ := wm.connectFlight.Do(key, func() (any, error) {
		ch, cerr := wm.connectBusinessOnce(businessID)
		if cerr != nil {
			return nil, cerr
		}
		if ch == nil {
			return nil, nil
		}
		return ch, nil
	})
	if err != nil {
		return nil, err
	}
	if v == nil {
		return nil, nil
	}
	ch, ok := v.(<-chan string)
	if !ok || ch == nil {
		return nil, nil
	}
	return ch, nil
}

// connectBusinessOnce is the singleflight body for ConnectBusiness. It never
// holds wm.mu across Connect, Disconnect, Logout, or the QR-channel wait.
func (wm *WhatsAppManager) connectBusinessOnce(businessID uint) (<-chan string, error) {
	if wm.isStopped() {
		return nil, fmt.Errorf("whatsapp manager stopped")
	}

	wm.mu.Lock()
	expected := wm.busGen[businessID]
	existing := wm.clients[businessID]
	wm.mu.Unlock()

	var localBumps uint64
	if existing != nil {
		if existing.IsConnected() && existing.IsLoggedIn() {
			wm.mu.Lock()
			still := wm.clients[businessID] == existing && wm.busGen[businessID] == expected && !wm.isStopped()
			wm.mu.Unlock()
			if still {
				return nil, fmt.Errorf("business %d already connected", businessID)
			}
		}
		log.Printf("Cleaning up stale client for business %d", businessID)
		wm.mu.Lock()
		if current, ok := wm.clients[businessID]; ok && current == existing {
			delete(wm.clients, businessID)
			delete(wm.handlers, businessID)
			delete(wm.pendingQR, businessID)
			wm.bumpBusGenLocked(businessID)
			localBumps++
		}
		wm.mu.Unlock()
		disconnectBounded(existing, whatsAppConnectTimeout)
	}
	expected += localBumps

	if wm.isStopped() {
		return nil, fmt.Errorf("whatsapp manager stopped")
	}
	if wm.devices == nil {
		return nil, fmt.Errorf("whatsapp device store not configured")
	}

	log.Printf("Creating new WhatsApp device for business %d...", businessID)
	client, err := wm.devices.NewClient()
	if err != nil {
		return nil, err
	}
	handlerID := client.AddEventHandler(func(evt any) {
		wm.handleEvent(businessID, evt)
	})

	if client.StoreID() == nil {
		log.Printf("Getting QR channel for business %d...", businessID)
		qrChan, qerr := client.GetQRChannel(context.Background())
		if qerr != nil {
			disconnectBounded(client, whatsAppConnectTimeout)
			return nil, qerr
		}
		if err := whatsappDial(context.Background(), client); err != nil {
			log.Printf("Failed to connect client for business %d: %v", businessID, err)
			return nil, err
		}
		if err := wm.publishConnectedClient(businessID, client, handlerID, expected); err != nil {
			return nil, err
		}
		_ = database.MarkWhatsAppDeviceStatus(businessID, database.WhatsAppDeviceStatusPairing, "", nil, nil, nil)
		return wm.forwardPairingQR(businessID, client, qrChan), nil
	}

	log.Printf("Connecting with existing session for business %d...", businessID)
	if err := whatsappDial(context.Background(), client); err != nil {
		log.Printf("Failed to connect existing session for business %d: %v", businessID, err)
		return nil, err
	}
	if err := wm.publishConnectedClient(businessID, client, handlerID, expected); err != nil {
		return nil, err
	}
	wm.persistPairedDevice(businessID, client)
	return nil, nil
}

// publishConnectedClient stores client if expected is still the bus generation
// observed before the dial. Otherwise the client is an orphan and is disconnected
// after the lock is released.
func (wm *WhatsAppManager) publishConnectedClient(businessID uint, client whatsAppClient, handlerID uint32, expected uint64) error {
	wm.mu.Lock()
	superseded := wm.isStopped() || wm.busGen[businessID] != expected
	if !superseded {
		if current, ok := wm.clients[businessID]; ok && current != nil && current != client {
			superseded = true
		}
	}
	if superseded {
		wm.mu.Unlock()
		disconnectBounded(client, whatsAppConnectTimeout)
		return fmt.Errorf("business %d connect superseded", businessID)
	}
	if wm.clients == nil {
		wm.clients = make(map[uint]whatsAppClient)
	}
	if wm.handlers == nil {
		wm.handlers = make(map[uint]uint32)
	}
	wm.clients[businessID] = client
	wm.handlers[businessID] = handlerID
	wm.mu.Unlock()
	return nil
}

// forwardPairingQR drains whatsmeow's QR channel without holding wm.mu.
// The returned channel is buffered and fed without blocking: the connect
// handler reads the first code, and every later one only lands in pendingQR
// for GET /whatsapp/qr. A blocking send stalled the loop on the second code,
// so a slow scan never persisted the device mapping and leaked this goroutine.
func (wm *WhatsAppManager) forwardPairingQR(businessID uint, client whatsAppClient, qrChan <-chan whatsmeow.QRChannelItem) <-chan string {
	qrStringChan := make(chan string, 1)
	logger.SafeGo(func() {
		defer close(qrStringChan)
		// Last non-code event ("success", "timeout", "error", err-*), or ""
		// when the channel closed without one.
		outcome := ""
		for evt := range qrChan {
			if evt.Event == "code" {
				log.Printf("QR Code received for business %d", businessID)
				wm.setPendingQR(businessID, client, evt.Code)
				select {
				case qrStringChan <- evt.Code:
				default:
				}
				continue
			}
			log.Printf("QR Event for business %d: %s", businessID, evt.Event)
			outcome = evt.Event
			wm.setPendingQR(businessID, client, "")
			// Successful pair: persist mapping once store identity exists.
			if evt.Event == "success" {
				wm.persistPairedDevice(businessID, client)
			}
		}
		if wm.endPairingAttempt(businessID, client, outcome) {
			// Also try after channel closes (some adapters only close).
			wm.persistPairedDevice(businessID, client)
		}
	})
	return qrStringChan
}

// setPendingQR records (or, with code == "", clears) the latest pairing code
// for businessID. It is a no-op once client is no longer the business's active
// client, so a superseded pairing attempt cannot overwrite a newer code.
func (wm *WhatsAppManager) setPendingQR(businessID uint, client whatsAppClient, code string) {
	wm.mu.Lock()
	defer wm.mu.Unlock()
	if current, ok := wm.clients[businessID]; !ok || current != client {
		return
	}
	if code == "" {
		delete(wm.pendingQR, businessID)
		return
	}
	if wm.pendingQR == nil {
		wm.pendingQR = make(map[uint]string)
	}
	wm.pendingQR[businessID] = code
}

// endPairingAttempt runs once the QR channel closes, with the last non-code
// event it carried (outcome), and reports whether the attempt is still the
// business's current one and paired. It always drops the pending code.
//
// An attempt that did not pair is torn down: its client leaves wm.clients and
// is disconnected, and the mapping row ConnectBusiness marked "pairing" goes
// back to "disconnected" (a first pairing has no row, so that write is a no-op
// there). whatsmeow disconnects by itself only when it ran out of codes or the
// pair handshake failed; on ClientOutdated or an unexpected event
// (ConnectFailure, LoggedOut, TemporaryBan, ...) it closes the channel and
// leaves the socket up, unpaired. GetStatusDetail trusts a live connected
// client over the row, so keeping that client made status read "pairing"
// forever and the dashboard polled for a code that would never come.
//
// Success needs a store identity (StoreID, not IsLoggedIn: whatsmeow
// reconnects right after a pair succeeds, so IsLoggedIn can briefly read false
// on a device that did pair) and no pair "error": whatsmeow sets the store ID
// before saving it, so a pair that fails on that save (or on the confirmation
// send) reports "error" with an ID already set. Other events that arrive with
// an ID mean the pair went through, as before.
//
// The network and database calls run after wm.mu is released so a slow write
// never blocks status reads. A Connect that starts a new attempt in that gap
// may see its "pairing" row overwritten with "disconnected"; status still
// reads "pairing" from the new live client, and the new attempt rewrites the
// row when it ends.
func (wm *WhatsAppManager) endPairingAttempt(businessID uint, client whatsAppClient, outcome string) bool {
	// StoreID takes the client lock. Read it before wm.mu so a client callback
	// that needs the manager lock cannot deadlock against this teardown.
	var storeID string
	if client != nil {
		if id := client.StoreID(); id != nil {
			storeID = id.String()
		}
	}
	wm.mu.Lock()
	if current, ok := wm.clients[businessID]; !ok || current != client {
		wm.mu.Unlock()
		return false
	}
	delete(wm.pendingQR, businessID)
	if storeID != "" && outcome != whatsmeow.QRChannelEventError {
		wm.mu.Unlock()
		return true
	}
	delete(wm.clients, businessID)
	delete(wm.handlers, businessID)
	wm.mu.Unlock()

	log.Printf("whatsapp: pairing attempt for business %d ended unpaired (%s)", businessID, outcome)
	disconnectBounded(client, whatsAppConnectTimeout)
	_ = database.MarkWhatsAppDeviceStatus(businessID, database.WhatsAppDeviceStatusDisconnected, "", nil, nil, nil)
	return false
}

// PendingQRCode returns the pairing code the operator should scan now, or ""
// when the business is not mid-pairing (no live client, already logged in, or
// whatsmeow has not issued a code yet). Anyone who scans it links their
// WhatsApp account to the business, so only expose it to settings:write.
func (wm *WhatsAppManager) PendingQRCode(businessID uint) string {
	wm.mu.RLock()
	client, ok := wm.clients[businessID]
	code := wm.pendingQR[businessID]
	wm.mu.RUnlock()
	if !ok || client == nil || code == "" || !client.IsConnected() || client.IsLoggedIn() {
		return ""
	}
	return code
}

// persistPairedDevice writes the business↔JID mapping only when a real store
// identity exists. Blank/provisional JIDs are never persisted.
func (wm *WhatsAppManager) persistPairedDevice(businessID uint, client whatsAppClient) {
	if client == nil {
		return
	}
	id := client.StoreID()
	if id == nil || id.String() == "" {
		return
	}
	now := wm.clock()
	zero := int64(0)
	device := &database.WhatsAppBusinessDevice{
		BusinessID:        businessID,
		DeviceJID:         id.String(),
		Status:            database.WhatsAppDeviceStatusConnected,
		LastConnectedAt:   &now,
		ReconnectAttempts: 0,
	}
	if err := database.UpsertWhatsAppBusinessDevice(device); err != nil {
		log.Printf("whatsapp: failed to persist device mapping business=%d: %v", businessID, err)
		return
	}
	_ = database.MarkWhatsAppDeviceStatus(businessID, database.WhatsAppDeviceStatusConnected, "", &now, nil, &zero)
}

func (wm *WhatsAppManager) DisconnectBusiness(businessID uint) error {
	wm.mu.Lock()
	client := wm.clients[businessID]
	delete(wm.clients, businessID)
	delete(wm.handlers, businessID)
	delete(wm.pendingQR, businessID)
	wm.bumpBusGenLocked(businessID)
	wm.mu.Unlock()

	if client != nil {
		// Logout and Disconnect are network I/O. The snapshot above is the
		// only work done under wm.mu; a hung logout cannot pin status reads.
		if err := logoutBounded(client, whatsAppConnectTimeout); err != nil {
			log.Printf("Error logging out business %d: %v", businessID, err)
			if !errors.Is(err, errWhatsAppDialTimeout) {
				disconnectBounded(client, whatsAppConnectTimeout)
			}
		} else {
			disconnectBounded(client, whatsAppConnectTimeout)
		}
	}
	// Explicit disconnect removes the mapping so the next restart does not reconnect.
	if err := database.DeleteWhatsAppBusinessDevice(businessID); err != nil {
		log.Printf("whatsapp: delete mapping business=%d: %v", businessID, err)
	}
	return nil
}

// GetStatusDetail returns structured lifecycle state for the dashboard.
func (wm *WhatsAppManager) GetStatusDetail(businessID uint) WhatsAppStatus {
	wm.mu.RLock()
	client, ok := wm.clients[businessID]
	wm.mu.RUnlock()

	if ok && client != nil {
		if client.IsConnected() {
			if client.IsLoggedIn() {
				return WhatsAppStatus{Status: database.WhatsAppDeviceStatusConnected}
			}
			return WhatsAppStatus{Status: database.WhatsAppDeviceStatusPairing}
		}
	}

	// Fall back to durable mapping when the process lost the in-memory client.
	if mapping, err := database.GetWhatsAppBusinessDevice(businessID); err == nil && mapping != nil {
		return WhatsAppStatus{
			Status:          mapping.Status,
			LastErrorCode:   mapping.LastErrorCode,
			LastConnectedAt: mapping.LastConnectedAt,
			RetryAt:         mapping.NextRetryAt,
		}
	}
	return WhatsAppStatus{Status: database.WhatsAppDeviceStatusDisconnected}
}

func (wm *WhatsAppManager) handleEvent(businessID uint, evt interface{}) {
	switch v := evt.(type) {
	case *events.Message:
		if v.Info.IsFromMe {
			return
		}

		// Extract text
		text := v.Message.GetConversation()
		if text == "" {
			text = v.Message.GetExtendedTextMessage().GetText()
		}
		if text == "" {
			return // Ignore non-text for now
		}

		// SafeGo: processMessage does DB + network work on inbound messages; a
		// raw `go` would crash the whole process on any panic inside it.
		logger.SafeGo(func() { wm.processMessage(businessID, v, text) })
	}
}

func (wm *WhatsAppManager) processMessage(businessID uint, v *events.Message, text string) {
	// Bound the whole inbound handling to the waiter_whatsapp feature ceiling so
	// a stalled provider cannot pin a whatsmeow goroutine for ~3 min (M2/M4).
	ctx, cancel := context.WithTimeout(context.Background(), llm.FeatureTimeout("waiter_whatsapp"))
	defer cancel()

	sender := v.Info.Sender.String()
	// Best-effort locale for early terminal replies (before conversation load).
	earlyLocale := ResolveWaiterConversationLocale(text, "en")

	// Cap message length and rate-limit per sender BEFORE any work, so an
	// abusive chat cannot drive unbounded model calls or DB writes. Mirrors
	// the HTTP waiter's maxAIWaiterMessageBytes + per-business rate limit.
	if len(text) > maxWhatsAppMessageBytes {
		log.Printf("WhatsApp message from sender_ref=%s exceeds %d bytes; ignoring", whatsappSenderRef(sender), maxWhatsAppMessageBytes)
		_ = wm.sendMessage(ctx, businessID, v.Info.Chat, WhatsAppTerminalReply(earlyLocale, WhatsAppTooLong))
		return
	}
	if !wm.allowSender(sender) {
		log.Printf("WhatsApp AI rate limit hit for sender_ref=%s (business %d); dropping message", whatsappSenderRef(sender), businessID)
		_ = wm.sendMessage(ctx, businessID, v.Info.Chat, WhatsAppTerminalReply(earlyLocale, WhatsAppRateLimited))
		return
	}
	if nonAD := v.Info.Sender.ToNonAD().String(); !takeWhatsAppSenderQuota(nonAD) {
		// Reply once per sender per UTC day, then drop silently: a refusal
		// reply per message would let a flooding number drive outbound sends.
		if takeWhatsAppSenderQuotaNotice(nonAD) {
			log.Printf("WhatsApp daily per-sender ceiling hit for sender_ref=%s (business %d); further messages today are dropped silently", whatsappSenderRef(sender), businessID)
			_ = wm.sendMessage(ctx, businessID, v.Info.Chat, WhatsAppTerminalReply(earlyLocale, WhatsAppBudgetReached))
		}
		return
	}

	// Resolve business settings and gate on the operational state AND the
	// owner's AI toggle BEFORE any DB write, mirroring the HTTP waiter. Gating
	// first also ensures a suspended/closed business does not accumulate
	// conversation/message rows for inbound traffic.
	settings, err := database.GetBusinessByID(businessID)
	if err != nil {
		log.Printf("Failed to get business %d: %v", businessID, err)
		return
	}
	if !database.IsBusinessOperational(settings) {
		log.Printf("WhatsApp AI call skipped for business %d: business suspended or closed", businessID)
		_ = wm.sendMessage(ctx, businessID, v.Info.Chat, WhatsAppTerminalReply(earlyLocale, WhatsAppAIUnavailable))
		return
	}
	if !settings.AiSettings.AiEnabled {
		log.Printf("WhatsApp AI call skipped for business %d: AI disabled by owner", businessID)
		_ = wm.sendMessage(ctx, businessID, v.Info.Chat, WhatsAppTerminalReply(earlyLocale, WhatsAppAIUnavailable))
		return
	}
	if wm.aiService == nil {
		log.Printf("WhatsApp AI call skipped for business %d: AI service not configured", businessID)
		_ = wm.sendMessage(ctx, businessID, v.Info.Chat, WhatsAppTerminalReply(earlyLocale, WhatsAppAIUnavailable))
		return
	}

	// Resolve conversation first so locale can be retained across turns before
	// guardrails and terminal replies.
	sessionID := fmt.Sprintf("whatsapp:%s", sender)
	conv, err := database.GetOrCreateAiWaiterConversation(sessionID, businessID, "WhatsApp", "en", "ordering")
	if err != nil {
		log.Printf("Failed to sync conversation for business %d: %v", businessID, err)
		return
	}
	locale := ResolveWaiterConversationLocale(text, conv.Language)
	if locale != conv.Language {
		_ = database.UpdateAiWaiterConversationLanguage(conv.ID, locale)
		conv.Language = locale
	}

	// Guardrail screen on the inbound guest turn (HTTP-waiter parity, fail-open).
	if reply, blocked := wm.screenWhatsAppTurn(ctx, businessID, locale, text); blocked {
		_ = wm.sendMessage(ctx, businessID, v.Info.Chat, reply)
		return
	}

	// Per-business daily message-count + USD budget gate (HTTP-waiter parity).
	// Count is served from the in-memory daily counter (seeded once per UTC day)
	// to avoid a JOIN+COUNT over ai_waiter_messages on every inbound message.
	now := time.Now()
	if database.CountAiWaiterMessagesTodayCached(businessID, now) >= whatsAppDailyMessageBudget() ||
		wm.costGate.OverBudget(businessID) {
		log.Printf("WhatsApp AI over budget for business %d; dropping message", businessID)
		_ = wm.sendMessage(ctx, businessID, v.Info.Chat, WhatsAppTerminalReply(locale, WhatsAppBudgetReached))
		return
	}

	// Always save user message (PII redacted at ingest, matching the HTTP waiter).
	if err := database.SaveAiWaiterMessage(conv.ID, "user", redactWhatsAppUserText(text), ""); err != nil {
		log.Printf("WARNING: failed to save AI waiter message for conversation %d: %v", conv.ID, err)
	} else {
		database.IncrementDailyAiWaiterCount(businessID, now)
	}

	// Check if paused
	if conv.IsPaused {
		log.Printf("AI is paused for conversation %d (Staff takeover)", conv.ID)
		_ = wm.sendMessage(ctx, businessID, v.Info.Chat, WhatsAppTerminalReply(locale, WhatsAppHumanTakeover))
		return
	}

	// Shared runtime context (same stock/orderable/promo grounding as web waiter).

	rtCtx, ctxErr := BuildWaiterRuntimeContext(ctx, settings, locale)
	if ctxErr != nil {
		log.Printf("WhatsApp context build failed business=%d: %v", businessID, ctxErr)
		_ = wm.sendMessage(ctx, businessID, v.Info.Chat, WhatsAppTerminalReply(locale, WhatsAppContextUnavailable))
		return
	}

	// Conversation history — load recent messages from DB
	history := []WaiterMessage{}
	if recentMsgs, err := database.GetRecentAiWaiterMessages(conv.ID, 20); err == nil {
		for _, m := range recentMsgs {
			history = append(history, WaiterMessage{
				ID:        m.ID,
				Role:      m.Role,
				Content:   m.Content,
				CreatedAt: m.CreatedAt.Unix(),
			})
		}
	} else {
		log.Printf("WARNING: failed to load conversation history for conv %d: %v", conv.ID, err)
	}

	// Call AI with shared context (locale retained from conversation).
	resp, err := wm.aiService.ChatWithWaiterWhatsApp(ctx, WaiterWhatsAppParams{
		AIName:              rtCtx.AIName,
		AIPriority:          rtCtx.AIPriority,
		SpecialInstructions: rtCtx.SpecialInstructions,
		BusinessName:        rtCtx.BusinessName,
		BusinessDescription: rtCtx.BusinessDescription,
		BusinessAddress:     rtCtx.BusinessAddress,
		ReservationContext:  rtCtx.ReservationContext,
		DeliveryContext:     rtCtx.DeliveryContext,
		MenuData:            rtCtx.MenuJSON,
		Language:            locale,
		History:             history,
		BusinessID:          businessID,
	})
	if err != nil {
		log.Printf("AI Error business=%d: %v", businessID, err)
		_ = wm.sendMessage(ctx, businessID, v.Info.Chat, WhatsAppTerminalReply(locale, WhatsAppProviderFailed))
		return
	}

	// 5. Send Response
	// Extract text from the model response
	responseText := ""
	if resp != nil && resp.Text != "" {
		responseText = resp.Text
	}

	if responseText == "" {
		_ = wm.sendMessage(ctx, businessID, v.Info.Chat, WhatsAppTerminalReply(locale, WhatsAppProviderFailed))
		return
	}
	responseText = whatsAppAllergenPostCheck(locale, responseText)
	if err := database.SaveAiWaiterMessage(conv.ID, "assistant", responseText, ""); err != nil {
		log.Printf("WARNING: failed to save AI waiter message for conversation %d: %v", conv.ID, err)
	}
	_ = wm.sendMessage(ctx, businessID, v.Info.Chat, responseText)
}

func (wm *WhatsAppManager) sendMessage(parent context.Context, businessID uint, chatID types.JID, text string) error {
	wm.mu.RLock()
	client, ok := wm.clients[businessID]
	wm.mu.RUnlock()

	if !ok || client == nil {
		return fmt.Errorf("whatsapp client not connected")
	}

	ctx, cancel := context.WithTimeout(parent, whatsAppSendTimeout)
	defer cancel()
	_, err := client.SendMessage(ctx, chatID, &waE2E.Message{
		Conversation: proto.String(text),
	})
	if err != nil {
		log.Printf("Failed to send WA message business=%d sender_ref=%s: %v", businessID, whatsappSenderRef(chatID.String()), err)
		return err
	}
	return nil
}

// SendMessageManual sends a message initiated by staff.
func (wm *WhatsAppManager) SendMessageManual(businessID uint, chatID types.JID, text string) error {
	return wm.sendMessage(context.Background(), businessID, chatID, text)
}

// WhatsAppBuilt reports whether this binary was compiled with the whatsmeow
// (GPL-3.0) integration. Default builds are false; see whatsapp_stub.go.
const WhatsAppBuilt = true

// SendManualToJID parses a raw JID string and sends a staff-initiated message.
// Callers outside this file use it so they never import go.mau.fi/* directly.
func (wm *WhatsAppManager) SendManualToJID(businessID uint, jid, text string) error {
	parsed, err := types.ParseJID(jid)
	if err != nil {
		return err
	}
	return wm.SendMessageManual(businessID, parsed, text)
}

// WithClassifier installs the inbound-message guardrail classifier (wired in
// main.go to the Gemini classifier). nil restores the fail-open AllowAll
// default. Mirrors DirectorConsoleService.WithClassifier.
func (wm *WhatsAppManager) WithClassifier(c guardrails.InputClassifier) *WhatsAppManager {
	if c == nil {
		wm.classifier = guardrails.AllowAll{}
		return wm
	}
	wm.classifier = c
	return wm
}

// WithCostGate installs the per-business daily USD ceiling gate. nil disables it.
func (wm *WhatsAppManager) WithCostGate(g *llm.AICostGate) *WhatsAppManager {
	wm.costGate = g
	return wm
}

// screenWhatsAppTurn runs the same inbound guardrail screen the HTTP waiter
// uses (the shared classifier, hard 2s ceiling). Failure mode follows the
// classifier's strict flag: fail-closed in production, fail-open in dev.
// When blocked it returns the localized redirect/decline reply to send back.
func (wm *WhatsAppManager) screenWhatsAppTurn(parent context.Context, businessID uint, locale, text string) (reply string, blocked bool) {
	if strings.TrimSpace(text) == "" || wm.classifier == nil {
		return "", false
	}
	gctx, gcancel := context.WithTimeout(parent, llm.FeatureTimeout("guardrail"))
	defer gcancel()
	verdict, err := wm.classifier.Classify(gctx, guardrails.ClassifyRequest{
		Surface: guardrails.SurfaceAIWaiter, BusinessID: businessID, Locale: locale, Text: text,
	})
	if err != nil || verdict.Allowed { // Classify returns nil err; strict mode sets Allowed=false on failure
		return "", false
	}
	if verdict.Category == guardrails.CategoryAbuse {
		return WaiterAbuseDecline(locale), true
	}
	return WaiterOffTopicRedirect(locale), true
}
