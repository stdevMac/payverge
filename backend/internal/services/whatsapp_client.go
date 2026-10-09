//go:build whatsapp

package services

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/store"
	"go.mau.fi/whatsmeow/store/sqlstore"
	"go.mau.fi/whatsmeow/types"
	waLog "go.mau.fi/whatsmeow/util/log"
)

// whatsAppClient is the narrow surface WhatsAppManager needs so unit tests can
// inject fakes without a live WhatsApp account or real whatsmeow container.
type whatsAppClient interface {
	AddEventHandler(func(any)) uint32
	Connect() error
	Disconnect()
	Logout(context.Context) error
	IsConnected() bool
	IsLoggedIn() bool
	SendMessage(ctx context.Context, to types.JID, msg *waE2E.Message, extras ...whatsmeow.SendRequestExtra) (whatsmeow.SendResponse, error)
	StoreID() *types.JID
	GetQRChannel(ctx context.Context) (<-chan whatsmeow.QRChannelItem, error)
}

// whatsAppDeviceStore resolves or creates devices for the manager.
type whatsAppDeviceStore interface {
	NewClient() (whatsAppClient, error)
	ClientForJID(ctx context.Context, jid string) (whatsAppClient, error)
	// ListDeviceJIDs returns JIDs known to the underlying store (for diagnostics).
	ListDeviceJIDs(ctx context.Context) ([]string, error)
}

// whatsmeowClientAdapter wraps *whatsmeow.Client behind whatsAppClient.
type whatsmeowClientAdapter struct {
	client *whatsmeow.Client
}

func (a *whatsmeowClientAdapter) AddEventHandler(fn func(any)) uint32 {
	return a.client.AddEventHandler(fn)
}

func (a *whatsmeowClientAdapter) Connect() error {
	return a.client.Connect()
}

func (a *whatsmeowClientAdapter) Disconnect() {
	a.client.Disconnect()
}

func (a *whatsmeowClientAdapter) Logout(ctx context.Context) error {
	return a.client.Logout(ctx)
}

func (a *whatsmeowClientAdapter) IsConnected() bool {
	return a.client.IsConnected()
}

func (a *whatsmeowClientAdapter) IsLoggedIn() bool {
	return a.client.IsLoggedIn()
}

func (a *whatsmeowClientAdapter) SendMessage(ctx context.Context, to types.JID, msg *waE2E.Message, extras ...whatsmeow.SendRequestExtra) (whatsmeow.SendResponse, error) {
	return a.client.SendMessage(ctx, to, msg, extras...)
}

func (a *whatsmeowClientAdapter) StoreID() *types.JID {
	if a.client == nil || a.client.Store == nil {
		return nil
	}
	return a.client.Store.ID
}

func (a *whatsmeowClientAdapter) GetQRChannel(ctx context.Context) (<-chan whatsmeow.QRChannelItem, error) {
	return a.client.GetQRChannel(ctx)
}

// sqlstoreDeviceStore adapts *sqlstore.Container to whatsAppDeviceStore.
type sqlstoreDeviceStore struct {
	container *sqlstore.Container
}

func (s *sqlstoreDeviceStore) NewClient() (whatsAppClient, error) {
	if s.container == nil {
		return nil, fmt.Errorf("whatsapp store not initialized")
	}
	device := s.container.NewDevice()
	client := whatsmeow.NewClient(device, waLog.Stdout("client", "INFO", true))
	return &whatsmeowClientAdapter{client: client}, nil
}

func (s *sqlstoreDeviceStore) ClientForJID(ctx context.Context, jid string) (whatsAppClient, error) {
	if s.container == nil {
		return nil, fmt.Errorf("whatsapp store not initialized")
	}
	parsed, err := types.ParseJID(jid)
	if err != nil {
		return nil, fmt.Errorf("invalid device jid: %w", err)
	}
	device, err := s.container.GetDevice(ctx, parsed)
	if err != nil {
		return nil, err
	}
	if device == nil {
		return nil, fmt.Errorf("device not found for jid")
	}
	client := whatsmeow.NewClient(device, waLog.Stdout("client", "INFO", true))
	return &whatsmeowClientAdapter{client: client}, nil
}

func (s *sqlstoreDeviceStore) ListDeviceJIDs(ctx context.Context) ([]string, error) {
	if s.container == nil {
		return nil, fmt.Errorf("whatsapp store not initialized")
	}
	devices, err := s.container.GetAllDevices(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(devices))
	for _, d := range devices {
		if d != nil && d.ID != nil {
			out = append(out, d.ID.String())
		}
	}
	return out, nil
}

// Ensure store.Device remains referenced for compile-time coupling.
var _ = (*store.Device)(nil)

// fakeWhatsAppClient is a test double for whatsAppClient.
type fakeWhatsAppClient struct {
	mu              sync.Mutex
	connected       bool
	loggedIn        bool
	storeID         *types.JID
	handlers        []func(any)
	connectErr      error
	sendErr         error
	sendDelay       time.Duration
	logoutErr       error
	connectCalls    int
	handlerRegs     int
	sendCalls       int
	lastSendTimeout time.Duration
	qrCodes         []string
	// qrEvents, when set, replaces qrCodes with an exact QR-channel script.
	qrEvents []whatsmeow.QRChannelItem
	// pairJID becomes the store identity (and the client logs in) just before
	// a scripted "success" event is emitted, as a real scan does.
	pairJID *types.JID
	// pairFailsAfterStoreID makes a scripted "error" event first set the store
	// identity to pairJID without logging in, as whatsmeow does when saving
	// the paired device fails after it assigned Store.ID.
	pairFailsAfterStoreID bool
	// qrHoldOpen keeps the QR channel open after the script until Disconnect
	// or Logout, like whatsmeow while a code still waits to be scanned.
	qrHoldOpen bool
	qrClosed   chan struct{}
}

func (f *fakeWhatsAppClient) AddEventHandler(fn func(any)) uint32 {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.handlers = append(f.handlers, fn)
	f.handlerRegs++
	return uint32(len(f.handlers))
}

func (f *fakeWhatsAppClient) Connect() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.connectCalls++
	if f.connectErr != nil {
		return f.connectErr
	}
	f.connected = true
	return nil
}

func (f *fakeWhatsAppClient) Disconnect() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.connected = false
	f.releaseQRLocked()
}

// releaseQRLocked closes a held-open QR channel. Caller holds f.mu.
func (f *fakeWhatsAppClient) releaseQRLocked() {
	if f.qrClosed == nil {
		return
	}
	select {
	case <-f.qrClosed:
	default:
		close(f.qrClosed)
	}
}

func (f *fakeWhatsAppClient) Logout(context.Context) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.logoutErr != nil {
		return f.logoutErr
	}
	f.loggedIn = false
	f.connected = false
	f.storeID = nil
	f.releaseQRLocked()
	return nil
}

func (f *fakeWhatsAppClient) IsConnected() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.connected
}

func (f *fakeWhatsAppClient) IsLoggedIn() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.loggedIn
}

func (f *fakeWhatsAppClient) SendMessage(ctx context.Context, _ types.JID, _ *waE2E.Message, _ ...whatsmeow.SendRequestExtra) (whatsmeow.SendResponse, error) {
	f.mu.Lock()
	f.sendCalls++
	if dl, ok := ctx.Deadline(); ok {
		f.lastSendTimeout = time.Until(dl)
	}
	delay := f.sendDelay
	err := f.sendErr
	f.mu.Unlock()
	if delay > 0 {
		select {
		case <-time.After(delay):
		case <-ctx.Done():
			return whatsmeow.SendResponse{}, ctx.Err()
		}
	}
	if err != nil {
		return whatsmeow.SendResponse{}, err
	}
	if err := ctx.Err(); err != nil {
		return whatsmeow.SendResponse{}, err
	}
	return whatsmeow.SendResponse{}, nil
}

func (f *fakeWhatsAppClient) StoreID() *types.JID {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.storeID
}

func (f *fakeWhatsAppClient) GetQRChannel(ctx context.Context) (<-chan whatsmeow.QRChannelItem, error) {
	f.mu.Lock()
	items := append([]whatsmeow.QRChannelItem(nil), f.qrEvents...)
	if len(items) == 0 {
		for _, code := range f.qrCodes {
			items = append(items, whatsmeow.QRChannelItem{Event: "code", Code: code})
		}
	}
	if f.qrHoldOpen && f.qrClosed == nil {
		f.qrClosed = make(chan struct{})
	}
	held := f.qrClosed
	f.mu.Unlock()

	ch := make(chan whatsmeow.QRChannelItem, len(items)+1)
	go func() {
		timedOut := false
		defer func() {
			close(ch)
			// whatsmeow disconnects the client after closing the channel on
			// a "timeout" (it ran out of codes before a scan).
			if timedOut {
				f.Disconnect()
			}
		}()
		for _, item := range items {
			if item.Event == "success" {
				f.completePairing()
			}
			if item.Event == "error" && f.pairFailsAfterStoreID {
				f.assignStoreIDOnly()
			}
			select {
			case ch <- item:
			case <-ctx.Done():
				return
			}
			timedOut = item.Event == "timeout"
		}
		if timedOut {
			return
		}
		if held != nil {
			select {
			case <-held:
			case <-ctx.Done():
			}
		}
	}()
	return ch, nil
}

func (f *fakeWhatsAppClient) completePairing() {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.pairJID != nil {
		f.storeID = f.pairJID
		f.loggedIn = true
	}
}

func (f *fakeWhatsAppClient) assignStoreIDOnly() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.storeID = f.pairJID
}

// NewFakePairingWhatsAppClient builds an unpaired hermetic client whose QR
// channel emits codes in order and then stays open until Disconnect, like a
// phone that has not scanned yet.
func NewFakePairingWhatsAppClient(codes ...string) *FakeWhatsAppClient {
	return &fakeWhatsAppClient{qrCodes: codes, qrHoldOpen: true}
}

// QueueNewClient makes the next NewClient call (a fresh pairing) return c.
func (s *FakeDeviceStore) QueueNewClient(c *FakeWhatsAppClient) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.newQueue = append(s.newQueue, c)
}

// fakeDeviceStore maps JIDs to preconfigured fake clients for tests.
type fakeDeviceStore struct {
	mu       sync.Mutex
	byJID    map[string]*fakeWhatsAppClient
	newQueue []*fakeWhatsAppClient
	newErr   error
}

// HermeticDeviceStore is the injectable device store used by hermetic restore
// scenarios (Wave 6). Implemented by FakeDeviceStore.
type HermeticDeviceStore interface {
	whatsAppDeviceStore
}

// FakeDeviceStore maps JIDs to preconfigured fake clients for hermetic tests.
type FakeDeviceStore = fakeDeviceStore

// FakeWhatsAppClient is the injectable whatsmeow client double for hermetic tests.
type FakeWhatsAppClient = fakeWhatsAppClient

func newFakeDeviceStore() *fakeDeviceStore {
	return &fakeDeviceStore{byJID: make(map[string]*fakeWhatsAppClient)}
}

// NewFakeDeviceStore creates an empty hermetic device store.
func NewFakeDeviceStore() *FakeDeviceStore {
	return newFakeDeviceStore()
}

// RegisterDevice maps a JID to a preconfigured fake client.
func (s *FakeDeviceStore) RegisterDevice(jid string, c *FakeWhatsAppClient) {
	s.register(jid, c)
}

// HandlerRegs returns how many times AddEventHandler was called on the client.
func (f *FakeWhatsAppClient) HandlerRegs() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.handlerRegs
}

// NewFakeWhatsAppClient builds a hermetic client with login/store identity set.
func NewFakeWhatsAppClient(jid string, loggedIn bool) (*FakeWhatsAppClient, error) {
	parsed, err := types.ParseJID(jid)
	if err != nil {
		return nil, err
	}
	return &fakeWhatsAppClient{loggedIn: loggedIn, storeID: &parsed}, nil
}

func (s *fakeDeviceStore) NewClient() (whatsAppClient, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.newErr != nil {
		return nil, s.newErr
	}
	if len(s.newQueue) == 0 {
		return &fakeWhatsAppClient{}, nil
	}
	c := s.newQueue[0]
	s.newQueue = s.newQueue[1:]
	return c, nil
}

func (s *fakeDeviceStore) ClientForJID(_ context.Context, jid string) (whatsAppClient, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	c, ok := s.byJID[strings.TrimSpace(jid)]
	if !ok || c == nil {
		return nil, fmt.Errorf("device not found for jid")
	}
	return c, nil
}

func (s *fakeDeviceStore) ListDeviceJIDs(context.Context) ([]string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]string, 0, len(s.byJID))
	for jid := range s.byJID {
		out = append(out, jid)
	}
	return out, nil
}

func (s *fakeDeviceStore) register(jid string, c *fakeWhatsAppClient) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if c.storeID == nil {
		parsed, _ := types.ParseJID(jid)
		c.storeID = &parsed
	}
	s.byJID[jid] = c
}
