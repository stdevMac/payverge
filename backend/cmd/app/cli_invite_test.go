package main

import (
	"context"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/stdevmac/payverge/backend/internal/runtimecontrol"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

var inviteCodeLine = regexp.MustCompile(`(?m)^([0-9a-f]{64})$`)

func newInviteCLITestDB(t *testing.T) *gorm.DB {
	t.Helper()
	return newCLITestDB(t, &runtimecontrol.InviteBatch{}, &runtimecontrol.AuditEvent{}, &runtimecontrol.InviteClaim{})
}

// ADM-2: in invite mode an operator must be able to mint a code from the
// server itself (distroless image, no admin invite screen), and the code must
// actually admit a signup.
func TestInviteCreateMintsAWorkingSingleShowCode(t *testing.T) {
	db := newInviteCLITestDB(t)
	h := newCLIHarness(t, db, "", map[string]string{
		"ADMIN_EMAIL": "owner@example.com",
		"PUBLIC_URL":  "https://pay.example.com/",
	})

	before := time.Now().UTC()
	require.Equal(t, cliExitOK, h.run("invite", "create", "--uses", "3", "--days", "7", "--name", "Opening week"), h.stderr.String())
	require.Empty(t, h.stderr.String(), "invite mode (the default) needs no note")

	m := inviteCodeLine.FindStringSubmatch(h.stdout.String())
	require.NotNil(t, m, "stdout must carry the plaintext code once: %s", h.stdout.String())
	code := m[1]
	require.Contains(t, h.stdout.String(), "Signup link: https://pay.example.com/dashboard?invite_code="+code)
	require.Contains(t, h.stdout.String(), "3 use(s)")

	var batch runtimecontrol.InviteBatch
	require.NoError(t, db.First(&batch).Error)
	require.Equal(t, "Opening week", batch.Name)
	require.Equal(t, 3, batch.CohortCap)
	require.True(t, batch.Active)
	require.Equal(t, "owner@example.com", batch.Owner, "owner defaults to ADMIN_EMAIL")
	require.Equal(t, cliInviteActor, batch.CreatedBy)
	require.NotEqual(t, code, batch.CodeDigest, "only the hash may be stored")
	require.WithinDuration(t, before.Add(7*24*time.Hour), batch.ExpiresAt, time.Minute)

	var audit runtimecontrol.AuditEvent
	require.NoError(t, db.Where("event_type = ?", "invite_batch_created").First(&audit).Error)
	require.Equal(t, batch.ID, *audit.InviteBatchID)
	require.Equal(t, cliInviteActor, audit.Actor)

	created := false
	require.NoError(t, runtimecontrol.New(db).RegisterWithInvite(context.Background(), "guest@example.com", code, func(*gorm.DB) error {
		created = true
		return nil
	}))
	require.True(t, created, "the minted code must admit a signup")
}

func TestInviteCreateDefaultsAndModeNote(t *testing.T) {
	db := newInviteCLITestDB(t)
	h := newCLIHarness(t, db, "", map[string]string{"REGISTRATION_MODE": "open"})
	require.Equal(t, cliExitOK, h.run("invite", "create"), h.stderr.String())
	require.NotContains(t, h.stdout.String(), "Signup link", "no PUBLIC_URL, no link")
	require.Contains(t, h.stderr.String(), `REGISTRATION_MODE is "open"`)

	var batch runtimecontrol.InviteBatch
	require.NoError(t, db.First(&batch).Error)
	require.Equal(t, 1, batch.CohortCap, "a code admits one account unless --uses says otherwise")
	require.Equal(t, "operator", batch.Owner)
	require.WithinDuration(t, time.Now().UTC().Add(cliInviteDefaultDays*24*time.Hour), batch.ExpiresAt, time.Minute)
}

func TestInviteCreateRejectsBadFlagsBeforeTouchingTheDatabase(t *testing.T) {
	for _, tc := range []struct {
		name string
		args []string
		want string
	}{
		{"zero uses", []string{"--uses", "0"}, "--uses must be between"},
		{"too many days", []string{"--days", "400"}, "--days must be between"},
		{"empty name", []string{"--name", "  "}, "--name and --reason must not be empty"},
		{"stray positional", []string{"secret-looking-value"}, "unexpected positional argument"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := newCLIHarness(t, nil, "", nil)
			require.Equal(t, cliExitUsage, h.run(append([]string{"invite", "create"}, tc.args...)...))
			require.Contains(t, h.stderr.String(), tc.want)
			require.NotContains(t, h.stderr.String(), "secret-looking-value")
			require.Zero(t, h.opened)
		})
	}
}

func TestInviteLink(t *testing.T) {
	require.Equal(t, "https://pay.example.com/dashboard?invite_code=abc", inviteLink(" https://pay.example.com// ", "abc"))
	require.Equal(t, "http://localhost:3000/dashboard?invite_code=abc", inviteLink("http://localhost:3000", "abc"))
	for _, bad := range []string{"", "pay.example.com", "javascript:alert(1)", "ftp://x"} {
		require.Empty(t, inviteLink(bad, "abc"), bad)
	}
	require.True(t, isCLIInvocation([]string{"invite", "create"}))
	require.False(t, strings.Contains(inviteLink("https://a.example", "x y"), " "))
}
