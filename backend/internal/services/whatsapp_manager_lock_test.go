//go:build whatsapp

package services

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// staticDeviceStore hands back one prebuilt client from NewClient.
type staticDeviceStore struct {
	client whatsAppClient
}

func (s staticDeviceStore) NewClient() (whatsAppClient, error) { return s.client, nil }

func (s staticDeviceStore) ClientForJID(context.Context, string) (whatsAppClient, error) {
	return nil, errors.New("device not found")
}

func (s staticDeviceStore) ListDeviceJIDs(context.Context) ([]string, error) { return nil, nil }

// blockingConnectClient's Connect waits until release is closed.
type blockingConnectClient struct {
	*fakeWhatsAppClient
	entered     chan struct{}
	release     chan struct{}
	once        sync.Once
	disconnects atomic.Int32
}

func (c *blockingConnectClient) Connect() error {
	c.once.Do(func() { close(c.entered) })
	<-c.release
	return nil
}

func (c *blockingConnectClient) Disconnect() {
	c.disconnects.Add(1)
	if c.fakeWhatsAppClient != nil {
		c.fakeWhatsAppClient.Disconnect()
	}
}

func closeOnce(ch chan struct{}) func() {
	var once sync.Once
	return func() { once.Do(func() { close(ch) }) }
}

// Concurrent ConnectBusiness calls for one business must share a single dial.
func TestWhatsAppManager_ConcurrentConnectOneDial(t *testing.T) {
	var dials atomic.Int32
	release := make(chan struct{})
	started := make(chan struct{})
	var startOnce sync.Once
	orig := whatsappDial
	whatsappDial = func(context.Context, whatsAppClient) error {
		dials.Add(1)
		startOnce.Do(func() { close(started) })
		<-release
		return nil
	}
	unlock := closeOnce(release)
	t.Cleanup(func() { whatsappDial = orig })
	t.Cleanup(unlock)

	wm := newWhatsAppManagerForTest(newFakeDeviceStore(), nil)
	t.Cleanup(wm.Stop)

	const n = 8
	const businessID uint = 42
	type connectResult struct {
		ch  <-chan string
		err error
	}
	errs := make(chan connectResult, n)
	for i := 0; i < n; i++ {
		go func() {
			ch, err := wm.ConnectBusiness(businessID)
			errs <- connectResult{ch: ch, err: err}
		}()
	}
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("dial never started")
	}
	time.Sleep(100 * time.Millisecond)
	if got := dials.Load(); got != 1 {
		t.Fatalf("in-flight dials = %d, want 1", got)
	}
	unlock()
	var qr <-chan string
	for i := 0; i < n; i++ {
		res := <-errs
		if res.err != nil {
			t.Fatalf("ConnectBusiness: %v", res.err)
		}
		if res.ch != nil {
			qr = res.ch
		}
	}
	if got := dials.Load(); got != 1 {
		t.Fatalf("dials = %d, want 1", got)
	}
	// The callers share one channel. Drain it so the pairing forwarder — which
	// reads whatsAppConnectTimeout while tearing the attempt down — is finished
	// before the next test shrinks that timeout.
	if qr != nil && !drainedWithin(qr, 2*time.Second) {
		t.Fatal("pairing forwarder leaked after the shared dial")
	}
}

// The manager mutex must not be held across the dial: a blocked dial still
// lets a status read return.
func TestWhatsAppManager_DialDoesNotHoldManagerLock(t *testing.T) {
	release := make(chan struct{})
	started := make(chan struct{})
	orig := whatsappDial
	whatsappDial = func(context.Context, whatsAppClient) error {
		close(started)
		<-release
		return nil
	}
	unlock := closeOnce(release)
	t.Cleanup(func() { whatsappDial = orig })
	t.Cleanup(unlock)

	const businessID uint = 77
	wm := newWhatsAppManagerForTest(newFakeDeviceStore(), nil)
	t.Cleanup(wm.Stop)

	type connectResult struct {
		ch  <-chan string
		err error
	}
	connectDone := make(chan connectResult, 1)
	go func() {
		ch, err := wm.ConnectBusiness(businessID)
		connectDone <- connectResult{ch: ch, err: err}
	}()
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("dial never started")
	}

	statusDone := make(chan struct{})
	go func() {
		_ = wm.HandlerRegistrationCount(businessID)
		_ = wm.GetStatusDetail(businessID)
		_ = wm.PendingQRCode(businessID)
		close(statusDone)
	}()
	select {
	case <-statusDone:
	case <-time.After(time.Second):
		t.Error("status read blocked while dial was in progress")
	}
	unlock()
	var res connectResult
	select {
	case res = <-connectDone:
	case <-time.After(2 * time.Second):
		t.Fatal("connect did not finish after dial was released")
	}
	if res.ch != nil && !drainedWithin(res.ch, 2*time.Second) {
		t.Fatal("pairing forwarder leaked after dial")
	}
	if t.Failed() {
		t.Fatal("manager lock was held during dial")
	}
}

// A dial that never returns is abandoned at whatsAppConnectTimeout, and the
// late Connect is disconnected instead of left running.
func TestWhatsAppManager_DialTimesOutAndDisconnectsOrphan(t *testing.T) {
	origTimeout := whatsAppConnectTimeout
	const timeout = 80 * time.Millisecond
	whatsAppConnectTimeout = timeout
	t.Cleanup(func() { whatsAppConnectTimeout = origTimeout })

	release := make(chan struct{})
	unlock := closeOnce(release)
	t.Cleanup(unlock)

	client := &blockingConnectClient{
		fakeWhatsAppClient: &fakeWhatsAppClient{},
		entered:            make(chan struct{}),
		release:            release,
	}
	wm := newWhatsAppManagerForTest(staticDeviceStore{client: client}, nil)
	t.Cleanup(wm.Stop)

	started := time.Now()
	errCh := make(chan error, 1)
	go func() {
		_, err := wm.ConnectBusiness(81)
		errCh <- err
	}()
	select {
	case <-client.entered:
	case <-time.After(2 * time.Second):
		t.Fatal("dial never started")
	}
	select {
	case err := <-errCh:
		elapsed := time.Since(started)
		if !errors.Is(err, errWhatsAppDialTimeout) {
			t.Fatalf("ConnectBusiness err = %v, want timeout", err)
		}
		if elapsed < timeout/2 {
			t.Fatalf("dial returned in %s, before the %s timeout", elapsed, timeout)
		}
		if elapsed > 2*time.Second {
			t.Fatalf("dial took %s, timeout did not bound it", elapsed)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("dial did not time out")
	}
	if liveClient(wm, 81) {
		t.Fatal("timed-out dial published a client")
	}

	unlock()
	deadline := time.Now().Add(2 * time.Second)
	for client.disconnects.Load() == 0 {
		if time.Now().After(deadline) {
			t.Fatal("late connect was not disconnected")
		}
		time.Sleep(5 * time.Millisecond)
	}
}
