package marketing

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestValidateHandoffTransition_Matrix(t *testing.T) {
	t.Parallel()
	// action, current, isOwner, wantOK, wantCode
	cases := []struct {
		name    string
		action  string
		current string
		owner   bool
		ok      bool
		code    string
	}{
		// mark_ready
		{"ready from draft", ActivityActionMarkReady, "", true, true, ""},
		{"ready from draft staff", ActivityActionMarkReady, "", false, true, ""},
		{"ready re-save", ActivityActionMarkReady, ActivityStatusReady, false, true, ""},
		{"ready from approved rework", ActivityActionMarkReady, ActivityStatusApproved, false, true, ""},
		{"ready blocked from posted", ActivityActionMarkReady, ActivityStatusPosted, true, false, "invalid_handoff_transition"},
		{"ready blocked from dismissed", ActivityActionMarkReady, ActivityStatusDismissed, true, false, "invalid_handoff_transition"},

		// approve — owner only, from ready
		{"approve owner ready", ActivityActionApprove, ActivityStatusReady, true, true, ""},
		{"approve staff blocked", ActivityActionApprove, ActivityStatusReady, false, false, "owner_required"},
		{"approve from draft blocked", ActivityActionApprove, "", true, false, "invalid_handoff_transition"},
		{"approve from approved blocked", ActivityActionApprove, ActivityStatusApproved, true, false, "invalid_handoff_transition"},
		{"approve from posted blocked", ActivityActionApprove, ActivityStatusPosted, true, false, "invalid_handoff_transition"},

		// post — owner flexible; staff needs approved
		{"post owner from ready", ActivityActionPost, ActivityStatusReady, true, true, ""},
		{"post owner from approved", ActivityActionPost, ActivityStatusApproved, true, true, ""},
		{"post owner from draft", ActivityActionPost, "", true, true, ""},
		{"post staff from approved", ActivityActionPost, ActivityStatusApproved, false, true, ""},
		{"post staff from ready blocked", ActivityActionPost, ActivityStatusReady, false, false, "approval_required"},
		{"post staff from draft blocked", ActivityActionPost, "", false, false, "approval_required"},
		{"post from dismissed blocked", ActivityActionPost, ActivityStatusDismissed, true, false, "invalid_handoff_transition"},

		// dismiss always ok
		{"dismiss any", ActivityActionDismiss, ActivityStatusReady, false, true, ""},
		{"dismiss posted", ActivityActionDismiss, ActivityStatusPosted, true, true, ""},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := ValidateHandoffTransition(tc.action, tc.current, tc.owner)
			if tc.ok {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
			var ht *HandoffTransitionError
			require.ErrorAs(t, err, &ht)
			require.Equal(t, tc.code, ht.Code)
		})
	}
}

func TestStatusForAction(t *testing.T) {
	t.Parallel()
	require.Equal(t, ActivityStatusReady, StatusForAction(ActivityActionMarkReady))
	require.Equal(t, ActivityStatusApproved, StatusForAction(ActivityActionApprove))
	require.Equal(t, ActivityStatusPosted, StatusForAction(ActivityActionPost))
	require.Equal(t, ActivityStatusDismissed, StatusForAction(ActivityActionDismiss))
}

func TestNoScheduledStatuses(t *testing.T) {
	// Guard: superiority forbids scheduled/queued statuses.
	t.Parallel()
	for _, banned := range []string{"scheduled", "queued", "due", "calendar"} {
		require.NotEqual(t, banned, ActivityStatusReady)
		require.NotEqual(t, banned, ActivityStatusApproved)
		require.NotEqual(t, banned, ActivityStatusPosted)
		require.NotEqual(t, banned, ActivityStatusDismissed)
		require.NotEqual(t, banned, ActivityActionMarkReady)
		require.NotEqual(t, banned, ActivityActionApprove)
	}
}

func TestValidateHandoffTransition_TrimsWhitespace(t *testing.T) {
	t.Parallel()
	require.NoError(t, ValidateHandoffTransition(ActivityActionApprove, "  ready  ", true))
	require.NoError(t, ValidateHandoffTransition(ActivityActionPost, "  approved  ", false))
	err := ValidateHandoffTransition(ActivityActionMarkReady, "  posted  ", true)
	require.Error(t, err)
}

func TestValidateHandoffTransition_RetiredImportLegacyPostIsUnknown(t *testing.T) {
	t.Parallel()
	err := ValidateHandoffTransition("import_legacy_post", ActivityStatusApproved, true)
	require.Error(t, err)
	var ht *HandoffTransitionError
	require.ErrorAs(t, err, &ht)
	require.Equal(t, "invalid_action", ht.Code)
}

func TestValidateHandoffTransition_UnknownAction(t *testing.T) {
	t.Parallel()
	err := ValidateHandoffTransition("schedule_post", "", true)
	require.Error(t, err)
	var ht *HandoffTransitionError
	require.ErrorAs(t, err, &ht)
	require.Equal(t, "invalid_action", ht.Code)
}

func TestStatusForAction_Unknown(t *testing.T) {
	t.Parallel()
	require.Empty(t, StatusForAction("import_legacy_post"))
	require.Empty(t, StatusForAction("schedule_post"))
	require.Empty(t, StatusForAction(""))
}

func TestCanMarkPosted_OwnerRepost(t *testing.T) {
	t.Parallel()
	// Owner may re-record posted (idempotent mark); staff may not from posted.
	require.True(t, CanMarkPosted(ActivityStatusPosted, true))
	require.False(t, CanMarkPosted(ActivityStatusPosted, false))
}

func TestCanDismiss_Always(t *testing.T) {
	t.Parallel()
	for _, cur := range []string{"", ActivityStatusReady, ActivityStatusApproved, ActivityStatusPosted, ActivityStatusDismissed} {
		require.True(t, CanDismiss(cur), cur)
	}
}
