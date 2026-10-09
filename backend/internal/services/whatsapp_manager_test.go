//go:build whatsapp

package services

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stdevmac/payverge/backend/internal/database"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/types"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// TestWhatsAppManager_StopIsIdempotentAndExitsSweep verifies the B4 graceful
// shutdown behavior: Stop() closes the done channel (exiting sweepLoop) and is
// safe to call more than once (stopOnce guards the close). It exercises only the
// goroutine lifecycle, so no DB/container is required — sweepOnce touches only
// rateMu/rateHits, both initialized here.
func TestWhatsAppManager_StopIsIdempotentAndExitsSweep(t *testing.T) {
	wm := &WhatsAppManager{
		clients:  make(map[uint]whatsAppClient),
		handlers: make(map[uint]uint32),
		rateHits: make(map[string][]time.Time),
		done:     make(chan struct{}),
	}

	go wm.sweepLoop()

	wm.Stop()
	wm.Stop() // must not panic: the second close is guarded by stopOnce

	select {
	case <-wm.done:
		// done was closed by Stop(); sweepLoop will observe this and return.
	default:
		t.Fatal("done channel not closed after Stop()")
	}
}

func setupWhatsAppManagerTestDB(t *testing.T) {
	t.Helper()
	// A per-run name: shared-cache memory DBs outlive the test, so reusing
	// t.Name() made -count>1 trip over the previous run's rows.
	gormDB, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:wa-mgr-%s-%d?mode=memory&cache=shared", t.Name(), time.Now().UnixNano())), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := gormDB.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	database.SetTestDB(gormDB)
	require.NoError(t, gormDB.AutoMigrate(&database.Business{}, &database.WhatsAppBusinessDevice{}))
}

func TestWhatsAppManager_RestoreMappedDeviceOnce(t *testing.T) {
	setupWhatsAppManagerTestDB(t)
	biz := &database.Business{
		BusinessId: "wa-restore-1", Name: "WA", OwnerAddress: "0x", SettlementAddr: "0x1", TippingAddr: "0x2", IsActive: true,
	}
	require.NoError(t, database.GetDB().Create(biz).Error)

	jid := "15551234567@s.whatsapp.net"
	parsed, err := types.ParseJID(jid)
	require.NoError(t, err)
	fake := &fakeWhatsAppClient{loggedIn: true, storeID: &parsed}
	store := newFakeDeviceStore()
	store.register(jid, fake)

	require.NoError(t, database.UpsertWhatsAppBusinessDevice(&database.WhatsAppBusinessDevice{
		BusinessID: biz.ID,
		DeviceJID:  jid,
		Status:     database.WhatsAppDeviceStatusConnected,
	}))

	wm := newWhatsAppManagerForTest(store, nil)
	require.NoError(t, wm.RestoreAllSessionsCtx(context.Background()))
	require.NoError(t, wm.RestoreAllSessionsCtx(context.Background())) // second restore must not double-register

	assert.Equal(t, 1, fake.connectCalls)
	assert.Equal(t, 1, fake.handlerRegs, "exactly one event handler per restored client")
	assert.True(t, fake.IsConnected())
	assert.Equal(t, database.WhatsAppDeviceStatusConnected, wm.GetStatusDetail(biz.ID).Status)
}

func TestWhatsAppManager_RestoreMissingDeviceDegraded(t *testing.T) {
	setupWhatsAppManagerTestDB(t)
	biz := &database.Business{
		BusinessId: "wa-restore-miss", Name: "WA", OwnerAddress: "0x", SettlementAddr: "0x1", TippingAddr: "0x2", IsActive: true,
	}
	require.NoError(t, database.GetDB().Create(biz).Error)
	require.NoError(t, database.UpsertWhatsAppBusinessDevice(&database.WhatsAppBusinessDevice{
		BusinessID: biz.ID,
		DeviceJID:  "missing@s.whatsapp.net",
		Status:     database.WhatsAppDeviceStatusConnected,
	}))

	wm := newWhatsAppManagerForTest(newFakeDeviceStore(), nil)
	require.NoError(t, wm.RestoreAllSessionsCtx(context.Background()))

	detail := wm.GetStatusDetail(biz.ID)
	assert.Equal(t, database.WhatsAppDeviceStatusDegraded, detail.Status)
	assert.Equal(t, "device_missing", detail.LastErrorCode)
	require.NotNil(t, detail.RetryAt)
}

func TestWhatsAppManager_DisconnectDeletesMapping(t *testing.T) {
	setupWhatsAppManagerTestDB(t)
	biz := &database.Business{
		BusinessId: "wa-disc", Name: "WA", OwnerAddress: "0x", SettlementAddr: "0x1", TippingAddr: "0x2", IsActive: true,
	}
	require.NoError(t, database.GetDB().Create(biz).Error)
	jid := "1999@s.whatsapp.net"
	parsed, _ := types.ParseJID(jid)
	fake := &fakeWhatsAppClient{connected: true, loggedIn: true, storeID: &parsed}
	store := newFakeDeviceStore()
	store.register(jid, fake)
	require.NoError(t, database.UpsertWhatsAppBusinessDevice(&database.WhatsAppBusinessDevice{
		BusinessID: biz.ID, DeviceJID: jid, Status: database.WhatsAppDeviceStatusConnected,
	}))

	wm := newWhatsAppManagerForTest(store, nil)
	wm.clients[biz.ID] = fake
	wm.handlers[biz.ID] = 1
	require.NoError(t, wm.DisconnectBusiness(biz.ID))

	_, err := database.GetWhatsAppBusinessDevice(biz.ID)
	require.Error(t, err)
	assert.Equal(t, database.WhatsAppDeviceStatusDisconnected, wm.GetStatusDetail(biz.ID).Status)
}

func TestWhatsAppSendMessage_BoundedTimeout(t *testing.T) {
	parsed, _ := types.ParseJID("1@s.whatsapp.net")
	fake := &fakeWhatsAppClient{
		connected: true,
		loggedIn:  true,
		storeID:   &parsed,
		sendDelay: 50 * time.Millisecond,
	}
	wm := newWhatsAppManagerForTest(newFakeDeviceStore(), nil)
	wm.clients[7] = fake

	err := wm.sendMessage(context.Background(), 7, parsed, "hello")
	require.NoError(t, err)
	assert.Greater(t, fake.lastSendTimeout, time.Duration(0))
	assert.LessOrEqual(t, fake.lastSendTimeout, whatsAppSendTimeout+time.Second)
}

func TestWhatsAppSenderRef_NotRawJID(t *testing.T) {
	jid := "15551234567@s.whatsapp.net"
	ref := whatsappSenderRef(jid)
	assert.NotContains(t, ref, "1555")
	assert.NotContains(t, ref, "@")
	sum := sha256.Sum256([]byte(strings.TrimSpace(jid)))
	assert.Equal(t, hex.EncodeToString(sum[:6]), ref)
}

func TestWhatsAppTerminalReply_EnglishAndSpanish(t *testing.T) {
	en := WhatsAppTerminalReply("en", WhatsAppHumanTakeover)
	es := WhatsAppTerminalReply("es", WhatsAppHumanTakeover)
	assert.NotEmpty(t, en)
	assert.NotEmpty(t, es)
	assert.NotEqual(t, en, es)
	// Unknown locale falls back to English.
	assert.Equal(t, en, WhatsAppTerminalReply("zz", WhatsAppHumanTakeover))
}

// drainedWithin reports whether ch is closed within d, i.e. the QR forwarder
// goroutine finished instead of leaking.
func drainedWithin(ch <-chan string, d time.Duration) bool {
	deadline := time.After(d)
	for {
		select {
		case _, ok := <-ch:
			if !ok {
				return true
			}
		case <-deadline:
			return false
		}
	}
}

// whatsmeow rotates pairing codes and reports "success" on the same channel.
// The forwarder must keep draining it: the latest code stays available for
// GET /whatsapp/qr, and a scan after a rotation still persists the mapping.
// The old blocking send stalled on the second code, so neither happened.
func TestWhatsAppManager_PairingAfterRotationPersistsDevice(t *testing.T) {
	setupWhatsAppManagerTestDB(t)
	biz := &database.Business{
		BusinessId: "wa-pair-rotate", Name: "WA", OwnerAddress: "0x", SettlementAddr: "0x1", TippingAddr: "0x2", IsActive: true,
	}
	require.NoError(t, database.GetDB().Create(biz).Error)

	jid, err := types.ParseJID("15550001111@s.whatsapp.net")
	require.NoError(t, err)
	fake := &fakeWhatsAppClient{
		qrEvents: []whatsmeow.QRChannelItem{
			{Event: "code", Code: "code-A"},
			{Event: "code", Code: "code-B"},
			{Event: "code", Code: "code-C"},
			{Event: "success"},
		},
		pairJID: &jid,
	}
	store := newFakeDeviceStore()
	store.newQueue = append(store.newQueue, fake)
	wm := newWhatsAppManagerForTest(store, nil)
	t.Cleanup(wm.Stop)

	codes, err := wm.ConnectBusiness(biz.ID)
	require.NoError(t, err)
	require.NotNil(t, codes)
	// Read only the first code, exactly like POST /whatsapp/connect does.
	select {
	case first := <-codes:
		assert.Equal(t, "code-A", first)
	case <-time.After(2 * time.Second):
		t.Fatal("connect never returned the first code")
	}

	require.Eventually(t, func() bool {
		mapping, err := database.GetWhatsAppBusinessDevice(biz.ID)
		return err == nil && mapping.DeviceJID == jid.String()
	}, 2*time.Second, 10*time.Millisecond, "paired device mapping was never persisted: the forwarder is stuck on a rotated code")
	require.True(t, drainedWithin(codes, 2*time.Second), "QR forwarder never finished")
	assert.Equal(t, database.WhatsAppDeviceStatusConnected, wm.GetStatusDetail(biz.ID).Status)
	assert.Empty(t, wm.PendingQRCode(biz.ID), "a paired business has no code to scan")
}

// While the phone has not scanned, the latest code is served; Disconnect ends
// the attempt, clears the code and lets the forwarder exit.
func TestWhatsAppManager_PendingQRCodeTracksLatestUntilDisconnect(t *testing.T) {
	setupWhatsAppManagerTestDB(t)
	const businessID = 7
	fake := NewFakePairingWhatsAppClient("code-A", "code-B", "code-C")
	store := newFakeDeviceStore()
	store.QueueNewClient(fake)
	wm := newWhatsAppManagerForTest(store, nil)
	t.Cleanup(wm.Stop)

	assert.Empty(t, wm.PendingQRCode(businessID), "no attempt yet")
	codes, err := wm.ConnectBusiness(businessID)
	require.NoError(t, err)
	select {
	case first := <-codes:
		assert.Equal(t, "code-A", first)
	case <-time.After(2 * time.Second):
		t.Fatal("connect never returned the first code")
	}
	require.Eventually(t, func() bool { return wm.PendingQRCode(businessID) == "code-C" },
		2*time.Second, 10*time.Millisecond, "the rotated code was never exposed")
	assert.Equal(t, database.WhatsAppDeviceStatusPairing, wm.GetStatusDetail(businessID).Status)

	require.NoError(t, wm.DisconnectBusiness(businessID))
	assert.Empty(t, wm.PendingQRCode(businessID))
	assert.True(t, drainedWithin(codes, 2*time.Second), "QR forwarder leaked after Disconnect")
}

// Reconnecting abandons the previous attempt; its late codes must not
// overwrite the new attempt's code.
func TestWhatsAppManager_SupersededPairingCannotOverwriteCode(t *testing.T) {
	setupWhatsAppManagerTestDB(t)
	const businessID = 8
	first := NewFakePairingWhatsAppClient("old-code")
	second := NewFakePairingWhatsAppClient("new-code")
	store := newFakeDeviceStore()
	store.QueueNewClient(first)
	store.QueueNewClient(second)
	wm := newWhatsAppManagerForTest(store, nil)
	t.Cleanup(wm.Stop)

	oldCodes, err := wm.ConnectBusiness(businessID)
	require.NoError(t, err)
	require.Eventually(t, func() bool { return wm.PendingQRCode(businessID) == "old-code" },
		2*time.Second, 10*time.Millisecond)

	_, err = wm.ConnectBusiness(businessID)
	require.NoError(t, err)
	require.True(t, drainedWithin(oldCodes, 2*time.Second), "superseded forwarder leaked")
	require.Eventually(t, func() bool { return wm.PendingQRCode(businessID) == "new-code" },
		2*time.Second, 10*time.Millisecond)

	// Even a straggling write from the superseded client is ignored.
	wm.setPendingQR(businessID, first, "old-code")
	assert.Equal(t, "new-code", wm.PendingQRCode(businessID))
}

// A re-pair (the business already has a device row) whose codes all expire
// must not stay "pairing": ConnectBusiness marked the row "pairing", whatsmeow
// then disconnects the client, and status falls back to that row. Before the
// fix the card kept polling for a code that would never come and offered no
// Connect button until a restart.
func TestWhatsAppManager_ExpiredRepairReturnsToDisconnected(t *testing.T) {
	setupWhatsAppManagerTestDB(t)
	biz := &database.Business{
		BusinessId: "wa-pair-expire", Name: "WA", OwnerAddress: "0x", SettlementAddr: "0x1", TippingAddr: "0x2", IsActive: true,
	}
	require.NoError(t, database.GetDB().Create(biz).Error)
	require.NoError(t, database.UpsertWhatsAppBusinessDevice(&database.WhatsAppBusinessDevice{
		BusinessID:    biz.ID,
		DeviceJID:     "15550002222@s.whatsapp.net",
		Status:        database.WhatsAppDeviceStatusDegraded,
		LastErrorCode: "device_missing",
	}))

	fake := &fakeWhatsAppClient{qrEvents: []whatsmeow.QRChannelItem{
		{Event: "code", Code: "code-A"},
		{Event: "code", Code: "code-B"},
		{Event: "timeout"},
	}}
	store := newFakeDeviceStore()
	store.QueueNewClient(fake)
	wm := newWhatsAppManagerForTest(store, nil)
	t.Cleanup(wm.Stop)

	codes, err := wm.ConnectBusiness(biz.ID)
	require.NoError(t, err)
	require.True(t, drainedWithin(codes, 2*time.Second), "QR forwarder never finished")

	require.Eventually(t, func() bool {
		return wm.GetStatusDetail(biz.ID).Status == database.WhatsAppDeviceStatusDisconnected
	}, 2*time.Second, 10*time.Millisecond, "an expired re-pair is still reported as %q", wm.GetStatusDetail(biz.ID).Status)
	assert.Empty(t, wm.PendingQRCode(biz.ID), "an expired attempt has nothing to scan")
	mapping, err := database.GetWhatsAppBusinessDevice(biz.ID)
	require.NoError(t, err)
	assert.Equal(t, database.WhatsAppDeviceStatusDisconnected, mapping.Status)
}

// The same expiry on a superseded attempt must not touch the row the newer
// attempt now owns.
func TestWhatsAppManager_SupersededAttemptEndingKeepsNewerPairing(t *testing.T) {
	setupWhatsAppManagerTestDB(t)
	biz := &database.Business{
		BusinessId: "wa-pair-supersede-end", Name: "WA", OwnerAddress: "0x", SettlementAddr: "0x1", TippingAddr: "0x2", IsActive: true,
	}
	require.NoError(t, database.GetDB().Create(biz).Error)
	require.NoError(t, database.UpsertWhatsAppBusinessDevice(&database.WhatsAppBusinessDevice{
		BusinessID: biz.ID,
		DeviceJID:  "15550003333@s.whatsapp.net",
		Status:     database.WhatsAppDeviceStatusDegraded,
	}))
	first := NewFakePairingWhatsAppClient("old-code")
	second := NewFakePairingWhatsAppClient("new-code")
	store := newFakeDeviceStore()
	store.QueueNewClient(first)
	store.QueueNewClient(second)
	wm := newWhatsAppManagerForTest(store, nil)
	t.Cleanup(wm.Stop)

	oldCodes, err := wm.ConnectBusiness(biz.ID)
	require.NoError(t, err)
	_, err = wm.ConnectBusiness(biz.ID)
	require.NoError(t, err)
	require.True(t, drainedWithin(oldCodes, 2*time.Second), "superseded forwarder leaked")

	// The old attempt's exit ran after the new attempt took over.
	assert.False(t, wm.endPairingAttempt(biz.ID, first, "timeout"), "a superseded attempt never reports paired")
	mapping, err := database.GetWhatsAppBusinessDevice(biz.ID)
	require.NoError(t, err)
	assert.Equal(t, database.WhatsAppDeviceStatusPairing, mapping.Status)
	assert.Equal(t, database.WhatsAppDeviceStatusPairing, wm.GetStatusDetail(biz.ID).Status)
	require.Eventually(t, func() bool { return wm.PendingQRCode(biz.ID) == "new-code" },
		2*time.Second, 10*time.Millisecond)
}

// liveClient reports whether businessID still has an in-memory client.
func liveClient(wm *WhatsAppManager, businessID uint) bool {
	wm.mu.RLock()
	defer wm.mu.RUnlock()
	_, ok := wm.clients[businessID]
	return ok
}

// whatsmeow disconnects the client by itself only when it runs out of codes
// or the pair handshake fails. On ClientOutdated or an unexpected connection
// event it closes the QR channel and leaves the socket up, unpaired. Status
// trusted that live client, so the card read "pairing" and polled /qr forever.
// The attempt must end as disconnected, with the client dropped and closed.
func TestWhatsAppManager_UnpairedCloseWithoutDisconnectEndsAttempt(t *testing.T) {
	for _, tc := range []struct {
		event  string
		repair bool // the business already has a device row (re-pair)
	}{
		{event: "err-client-outdated", repair: true},
		{event: "err-unexpected-state", repair: true},
		{event: "error", repair: false},
		{event: "", repair: true}, // channel closed with no terminal event
	} {
		t.Run(fmt.Sprintf("%q/repair=%v", tc.event, tc.repair), func(t *testing.T) {
			setupWhatsAppManagerTestDB(t)
			biz := &database.Business{
				BusinessId: "wa-pair-close", Name: "WA", OwnerAddress: "0x", SettlementAddr: "0x1", TippingAddr: "0x2", IsActive: true,
			}
			require.NoError(t, database.GetDB().Create(biz).Error)
			if tc.repair {
				require.NoError(t, database.UpsertWhatsAppBusinessDevice(&database.WhatsAppBusinessDevice{
					BusinessID: biz.ID,
					DeviceJID:  "15550004444@s.whatsapp.net",
					Status:     database.WhatsAppDeviceStatusDegraded,
				}))
			}

			script := []whatsmeow.QRChannelItem{{Event: "code", Code: "code-A"}}
			if tc.event != "" {
				script = append(script, whatsmeow.QRChannelItem{Event: tc.event})
			}
			fake := &fakeWhatsAppClient{qrEvents: script}
			store := newFakeDeviceStore()
			store.QueueNewClient(fake)
			wm := newWhatsAppManagerForTest(store, nil)
			t.Cleanup(wm.Stop)

			codes, err := wm.ConnectBusiness(biz.ID)
			require.NoError(t, err)
			require.True(t, drainedWithin(codes, 2*time.Second), "QR forwarder never finished")

			require.Eventually(t, func() bool {
				return wm.GetStatusDetail(biz.ID).Status == database.WhatsAppDeviceStatusDisconnected
			}, 2*time.Second, 10*time.Millisecond, "an ended attempt is still reported as %q", wm.GetStatusDetail(biz.ID).Status)
			assert.False(t, liveClient(wm, biz.ID), "the unpaired client must leave the manager")
			assert.False(t, fake.IsConnected(), "the unpaired client must be disconnected")
			assert.Equal(t, 0, wm.HandlerRegistrationCount(biz.ID))
			assert.Empty(t, wm.PendingQRCode(biz.ID))
			if tc.repair {
				mapping, err := database.GetWhatsAppBusinessDevice(biz.ID)
				require.NoError(t, err)
				assert.Equal(t, database.WhatsAppDeviceStatusDisconnected, mapping.Status)
			}
		})
	}
}

// whatsmeow assigns Store.ID before it saves the paired device, so a pair that
// fails on that save reports "error" with an ID already set. That device must
// not be persisted as connected.
func TestWhatsAppManager_PairErrorAfterStoreIDIsNotPersisted(t *testing.T) {
	setupWhatsAppManagerTestDB(t)
	biz := &database.Business{
		BusinessId: "wa-pair-error-id", Name: "WA", OwnerAddress: "0x", SettlementAddr: "0x1", TippingAddr: "0x2", IsActive: true,
	}
	require.NoError(t, database.GetDB().Create(biz).Error)

	jid, err := types.ParseJID("15550005555@s.whatsapp.net")
	require.NoError(t, err)
	fake := &fakeWhatsAppClient{
		qrEvents:              []whatsmeow.QRChannelItem{{Event: "code", Code: "code-A"}, {Event: "error"}},
		pairJID:               &jid,
		pairFailsAfterStoreID: true,
	}
	store := newFakeDeviceStore()
	store.QueueNewClient(fake)
	wm := newWhatsAppManagerForTest(store, nil)
	t.Cleanup(wm.Stop)

	codes, err := wm.ConnectBusiness(biz.ID)
	require.NoError(t, err)
	require.True(t, drainedWithin(codes, 2*time.Second), "QR forwarder never finished")

	require.Eventually(t, func() bool { return !liveClient(wm, biz.ID) }, 2*time.Second, 10*time.Millisecond)
	require.NotNil(t, fake.StoreID(), "the fake must model the ID-then-error path")
	_, err = database.GetWhatsAppBusinessDevice(biz.ID)
	assert.ErrorIs(t, err, gorm.ErrRecordNotFound, "a failed pair must not persist a device mapping")
	assert.Equal(t, database.WhatsAppDeviceStatusDisconnected, wm.GetStatusDetail(biz.ID).Status)
}
