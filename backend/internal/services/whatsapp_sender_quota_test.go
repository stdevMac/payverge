package services

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestTakeWhatsAppSenderQuotaCapsOneSenderPerDay(t *testing.T) {
	whatsAppSenderQuota.Reset()
	t.Cleanup(whatsAppSenderQuota.Reset)
	t.Setenv(envWhatsAppDailyPerSender, "3")

	const abuser = "15551230000@s.whatsapp.net"
	for i := 0; i < 3; i++ {
		require.Truef(t, takeWhatsAppSenderQuota(abuser), "turn %d is within the ceiling", i+1)
	}
	require.False(t, takeWhatsAppSenderQuota(abuser), "fourth turn of the day is refused")
	require.False(t, takeWhatsAppSenderQuota(" "+abuser+" "), "whitespace does not mint a new key")

	require.True(t, takeWhatsAppSenderQuota("15551239999@s.whatsapp.net"), "another sender is unaffected")
	require.False(t, takeWhatsAppSenderQuota(""), "an unattributable sender is refused")
}

func TestTakeWhatsAppSenderQuotaNoticeOncePerSenderPerDay(t *testing.T) {
	whatsAppSenderQuota.Reset()
	t.Cleanup(whatsAppSenderQuota.Reset)
	day := time.Date(2026, 10, 3, 23, 0, 0, 0, time.UTC)
	whatsAppSenderQuota.SetClock(func() time.Time { return day })
	t.Cleanup(func() { whatsAppSenderQuota.SetClock(time.Now) })

	const abuser = "15551230000@s.whatsapp.net"
	require.True(t, takeWhatsAppSenderQuotaNotice(abuser), "first refusal of the day is announced")
	for i := 0; i < 5; i++ {
		require.False(t, takeWhatsAppSenderQuotaNotice(abuser), "later refusals the same day are silent")
	}
	require.False(t, takeWhatsAppSenderQuotaNotice(" "+abuser), "whitespace does not mint a new notice")
	require.True(t, takeWhatsAppSenderQuotaNotice("15551239999@s.whatsapp.net"), "another sender gets its own notice")
	require.False(t, takeWhatsAppSenderQuotaNotice(""), "an unattributable sender never gets a reply")

	day = day.Add(2 * time.Hour) // next UTC day
	require.True(t, takeWhatsAppSenderQuotaNotice(abuser), "the notice resets with the UTC day")
}

func TestWhatsAppDailyPerSenderDefaultsAndRejectsInvalid(t *testing.T) {
	t.Setenv(envWhatsAppDailyPerSender, "")
	require.EqualValues(t, defaultWhatsAppDailyPerSender, whatsAppDailyPerSender())
	for _, bad := range []string{"0", "-4", "lots"} {
		t.Setenv(envWhatsAppDailyPerSender, bad)
		require.EqualValuesf(t, defaultWhatsAppDailyPerSender, whatsAppDailyPerSender(), "invalid %q falls back", bad)
	}
}
