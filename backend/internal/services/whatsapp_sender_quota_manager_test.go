//go:build whatsapp

package services

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
)

// processMessage refuses a sender past the daily ceiling before any DB work.
// The business id below does not exist, so an admitted turn stops silently at
// the business lookup (no reply), while a refused turn gets the capacity reply.
func TestWhatsAppProcessMessage_DailyPerSenderCeiling(t *testing.T) {
	setupWhatsAppManagerTestDB(t)
	whatsAppSenderQuota.Reset()
	t.Cleanup(whatsAppSenderQuota.Reset)
	t.Setenv(envWhatsAppDailyPerSender, "2")

	const businessID = 4242
	chat, err := types.ParseJID("15551230000@s.whatsapp.net")
	require.NoError(t, err)
	fake := &fakeWhatsAppClient{connected: true, loggedIn: true, storeID: &chat}
	wm := newWhatsAppManagerForTest(newFakeDeviceStore(), nil)
	wm.clients[businessID] = fake

	msgFrom := func(device uint16) *events.Message {
		sender := chat
		sender.Device = device
		return &events.Message{Info: types.MessageInfo{MessageSource: types.MessageSource{Chat: chat, Sender: sender}}}
	}

	wm.processMessage(businessID, msgFrom(0), "hello")
	wm.processMessage(businessID, msgFrom(0), "hello again")
	require.Equal(t, 0, fake.sendCalls, "turns within the ceiling reach the business lookup")

	wm.processMessage(businessID, msgFrom(0), "third")
	require.Equal(t, 1, fake.sendCalls, "the third turn of the day gets the capacity reply")

	wm.processMessage(businessID, msgFrom(0), "fourth")
	wm.processMessage(businessID, msgFrom(7), "from a linked device")
	require.Equal(t, 1, fake.sendCalls,
		"later refused turns (linked devices included) are dropped without another outbound reply")
	require.False(t, takeWhatsAppSenderQuota(chat.ToNonAD().String()), "linked devices of one number share the ceiling")
}
