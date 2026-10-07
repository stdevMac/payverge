package database

import (
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// seedDirectChannel inserts a manual direct channel plus a membership row for
// each staff id — the T1/T2 tests build DMs this way so they stay independent of
// GetOrCreateDM (a T3 method).
func seedDirectChannel(t *testing.T, g *gorm.DB, businessID uint, refKey string, members ...uint) ChatChannel {
	t.Helper()
	ch := ChatChannel{BusinessID: businessID, Type: ChatChannelTypeDirect, RefKey: refKey}
	require.NoError(t, g.Create(&ch).Error)
	for _, sid := range members {
		require.NoError(t, g.Create(&ChatChannelMember{ChannelID: ch.ID, StaffID: sid, BusinessID: businessID, Role: ChatMemberRoleMember}).Error)
	}
	return ch
}

// TestGetOrCreateVirtualChannelIdempotent proves the resolver converges on a
// single backing row: two get-or-create calls for the same ref_key return the
// SAME channel id (no duplicate row, no partial-unique violation surfaced).
func TestGetOrCreateVirtualChannelIdempotent(t *testing.T) {
	d, g := newChatTestDB(t)
	require.NoError(t, g.Create(&Business{ID: 1, BusinessId: "biz-1"}).Error)

	a, err := d.GetOrCreateVirtualChannel(1, "role:server", "Server")
	require.NoError(t, err)
	require.NotZero(t, a.ID)
	require.Equal(t, ChatChannelTypeRole, a.Type)

	b, err := d.GetOrCreateVirtualChannel(1, "role:server", "Server")
	require.NoError(t, err)
	require.Equal(t, a.ID, b.ID, "get-or-create must be idempotent on the ref_key")

	var count int64
	require.NoError(t, g.Model(&ChatChannel{}).Where("business_id = ? AND ref_key = ?", 1, "role:server").Count(&count).Error)
	require.Equal(t, int64(1), count, "exactly one backing row for the virtual channel")

	// Dept channels reuse the role type with a dept: prefix.
	dc, err := d.GetOrCreateVirtualChannel(1, "dept:BOH", "BOH")
	require.NoError(t, err)
	require.Equal(t, ChatChannelTypeRole, dc.Type)
	require.NotEqual(t, a.ID, dc.ID)
}

// TestListChannelsForStaff asserts a staff member sees their role channel, each
// dept channel for departments they hold an active position in, and their own
// DMs — but NOT another staff member's DMs.
func TestListChannelsForStaff(t *testing.T) {
	d, g := newChatTestDB(t)
	require.NoError(t, g.Create(&Business{ID: 1, BusinessId: "biz-1"}).Error)
	// Two positions in the BOH department (should collapse to one dept channel),
	// plus an inactive position whose dept must NOT yield a channel.
	require.NoError(t, g.Create(&Position{ID: 9, BusinessID: 1, Name: "Line Cook", Department: "BOH", IsActive: true}).Error)
	require.NoError(t, g.Create(&Position{ID: 10, BusinessID: 1, Name: "Dish", Department: "BOH", IsActive: true}).Error)
	require.NoError(t, g.Create(&Position{ID: 11, BusinessID: 1, Name: "Retired", Department: "FOH"}).Error)
	// GORM omits the zero-value IsActive:false (default:true tag fills it), so force the column off.
	require.NoError(t, g.Model(&Position{}).Where("id = ?", 11).Update("is_active", false).Error)
	require.NoError(t, g.Create(&StaffPosition{BusinessID: 1, StaffID: 5, PositionID: 9}).Error)
	require.NoError(t, g.Create(&StaffPosition{BusinessID: 1, StaffID: 5, PositionID: 10}).Error)
	require.NoError(t, g.Create(&StaffPosition{BusinessID: 1, StaffID: 5, PositionID: 11}).Error)

	dm5 := seedDirectChannel(t, g, 1, "dm:5:6", 5, 6)
	dmOthers := seedDirectChannel(t, g, 1, "dm:6:7", 6, 7) // staff 5 not a member

	channels, err := d.ListChannelsForStaff(1, 5, "server")
	require.NoError(t, err)

	refKeys := map[string]bool{}
	ids := map[uint]bool{}
	for _, ch := range channels {
		refKeys[ch.RefKey] = true
		ids[ch.ID] = true
	}
	require.True(t, refKeys["role:server"], "own role channel present")
	require.True(t, refKeys["dept:BOH"], "dept channel for an active position present")
	require.False(t, refKeys["dept:FOH"], "no channel for an inactive position's dept")
	require.True(t, ids[dm5.ID], "own DM present")
	require.False(t, ids[dmOthers.ID], "another staff's DM must NOT appear")
}

// TestListChannelsSteadyStateIssuesNoInserts is the perf-gate access-shape guard
// for the dashboard-polling chat list path. The old implementation called
// GetOrCreateVirtualChannel in a loop, so EVERY read issued an INSERT..ON CONFLICT
// per role/dept channel (burning the id sequence). After warming the channels
// once, a steady-state list of either surface must issue ZERO INSERTs.
func TestListChannelsSteadyStateIssuesNoInserts(t *testing.T) {
	d, g := newChatTestDB(t)
	require.NoError(t, g.Create(&Business{ID: 1, BusinessId: "biz-1"}).Error)
	require.NoError(t, g.Create(&Staff{ID: 5, BusinessID: 1, Role: "server", IsActive: true}).Error)
	require.NoError(t, g.Create(&Position{ID: 9, BusinessID: 1, Name: "Line Cook", Department: "BOH", IsActive: true}).Error)
	require.NoError(t, g.Create(&StaffPosition{BusinessID: 1, StaffID: 5, PositionID: 9}).Error)

	// Warm-up pass materializes the virtual channels.
	_, err := d.ListChannelsForStaff(1, 5, "server")
	require.NoError(t, err)
	_, err = d.ListChannelsForOperator(1, 0)
	require.NoError(t, err)

	// Count INSERTs (gorm:create) issued from here on.
	var inserts int64
	require.NoError(t, g.Callback().Create().After("gorm:create").
		Register("count_inserts_steady", func(db *gorm.DB) { atomic.AddInt64(&inserts, 1) }))
	t.Cleanup(func() { _ = g.Callback().Create().Remove("count_inserts_steady") })

	// Steady-state reads: channels already exist → no INSERTs.
	staffCh, err := d.ListChannelsForStaff(1, 5, "server")
	require.NoError(t, err)
	opCh, err := d.ListChannelsForOperator(1, 0)
	require.NoError(t, err)

	require.Zero(t, atomic.LoadInt64(&inserts), "steady-state channel list must not INSERT")
	// And the channel set is still correct (role + dept present).
	staffRefs := map[string]bool{}
	for _, c := range staffCh {
		staffRefs[c.RefKey] = true
	}
	require.True(t, staffRefs["role:server"] && staffRefs["dept:BOH"], "staff virtual channels intact")
	opRefs := map[string]bool{}
	for _, c := range opCh {
		opRefs[c.RefKey] = true
	}
	require.True(t, opRefs["role:server"] && opRefs["dept:BOH"], "operator virtual channels intact")
}

// TestCanReadChannelBranches exercises EVERY privacy branch incl. the negatives.
func TestCanReadChannelBranches(t *testing.T) {
	d, g := newChatTestDB(t)
	require.NoError(t, g.Create(&Business{ID: 1, BusinessId: "biz-1"}).Error)
	require.NoError(t, g.Create(&Position{ID: 9, BusinessID: 1, Name: "Line Cook", Department: "BOH", IsActive: true}).Error)
	require.NoError(t, g.Create(&StaffPosition{BusinessID: 1, StaffID: 5, PositionID: 9}).Error)

	// --- direct/group: member vs non-member ---
	dm := seedDirectChannel(t, g, 1, "dm:5:6", 5, 6)
	ok, err := d.CanReadChannel(1, &dm, 5, "server")
	require.NoError(t, err)
	require.True(t, ok, "DM member can read")
	ok, err = d.CanReadChannel(1, &dm, 99, "manager")
	require.NoError(t, err)
	require.False(t, ok, "non-member of a DM is denied even as manager")

	group := ChatChannel{BusinessID: 1, Type: ChatChannelTypeGroup, Name: "Shift A"}
	require.NoError(t, g.Create(&group).Error)
	require.NoError(t, g.Create(&ChatChannelMember{ChannelID: group.ID, StaffID: 6, BusinessID: 1, Role: ChatMemberRoleMember}).Error)
	ok, _ = d.CanReadChannel(1, &group, 6, "server")
	require.True(t, ok, "group member can read")
	ok, _ = d.CanReadChannel(1, &group, 5, "server")
	require.False(t, ok, "group non-member is denied")

	// --- role channel: exact role match only ---
	roleCh, err := d.GetOrCreateVirtualChannel(1, "role:host", "Host")
	require.NoError(t, err)
	ok, _ = d.CanReadChannel(1, roleCh, 5, "host")
	require.True(t, ok, "matching role can read role channel")
	ok, _ = d.CanReadChannel(1, roleCh, 5, "server")
	require.False(t, ok, "a server CANNOT read a role:host channel")

	// --- dept channel: active StaffPosition in the dept ---
	deptCh, err := d.GetOrCreateVirtualChannel(1, "dept:BOH", "BOH")
	require.NoError(t, err)
	ok, err = d.CanReadChannel(1, deptCh, 5, "server")
	require.NoError(t, err)
	require.True(t, ok, "staff with an active BOH position can read dept:BOH")
	ok, _ = d.CanReadChannel(1, deptCh, 99, "server")
	require.False(t, ok, "non-operator staff without a BOH position is denied dept:BOH")

	// --- announcement deferred (closed) + cross-tenant denial ---
	ann := ChatChannel{BusinessID: 1, Type: ChatChannelTypeAnnouncement, Name: "Notices", RefKey: "ann:all"}
	require.NoError(t, g.Create(&ann).Error)
	ok, _ = d.CanReadChannel(1, &ann, 5, "server")
	require.False(t, ok, "announcement channels are gated in Stage 3, closed here")
	ok, _ = d.CanReadChannel(2, &dm, 5, "server")
	require.False(t, ok, "cross-tenant channel access is denied")
}

// TestListChannelsForOperator asserts an owner/manager caller sees one role
// channel per DISTINCT ACTIVE staff role and one dept channel per distinct
// active-position department — plus their own manual DMs, never anyone else's.
func TestListChannelsForOperator(t *testing.T) {
	d, g := newChatTestDB(t)
	require.NoError(t, g.Create(&Business{ID: 1, BusinessId: "biz-1"}).Error)
	// Two active roles (server twice → one channel) + an inactive staffer whose
	// role must NOT produce a channel.
	require.NoError(t, g.Create(&Staff{ID: 5, BusinessID: 1, Email: "a@x.com", Name: "A", Role: "server", InvitedBy: "w"}).Error)
	require.NoError(t, g.Create(&Staff{ID: 6, BusinessID: 1, Email: "b@x.com", Name: "B", Role: "server", InvitedBy: "w"}).Error)
	require.NoError(t, g.Create(&Staff{ID: 7, BusinessID: 1, Email: "c@x.com", Name: "C", Role: "kitchen", InvitedBy: "w"}).Error)
	require.NoError(t, g.Create(&Staff{ID: 8, BusinessID: 1, Email: "d@x.com", Name: "D", Role: "host", InvitedBy: "w"}).Error)
	require.NoError(t, g.Model(&Staff{}).Where("id = ?", 8).Update("is_active", false).Error)
	// One active-position dept + one dept reachable only via an inactive position.
	require.NoError(t, g.Create(&Position{ID: 9, BusinessID: 1, Name: "Line Cook", Department: "BOH", IsActive: true}).Error)
	require.NoError(t, g.Create(&Position{ID: 11, BusinessID: 1, Name: "Retired", Department: "FOH"}).Error)
	require.NoError(t, g.Model(&Position{}).Where("id = ?", 11).Update("is_active", false).Error)

	dmOthers := seedDirectChannel(t, g, 1, "dm:5:6", 5, 6)

	// Owner caller (staffID 0): all role/dept channels, no DMs.
	channels, err := d.ListChannelsForOperator(1, 0)
	require.NoError(t, err)
	refKeys := map[string]bool{}
	ids := map[uint]bool{}
	for _, ch := range channels {
		refKeys[ch.RefKey] = true
		ids[ch.ID] = true
	}
	require.True(t, refKeys["role:server"], "role channel per active role (server)")
	require.True(t, refKeys["role:kitchen"], "role channel per active role (kitchen)")
	require.False(t, refKeys["role:host"], "inactive staff's role yields no channel")
	require.True(t, refKeys["dept:BOH"], "dept channel per active-position dept")
	require.False(t, refKeys["dept:FOH"], "inactive position's dept yields no channel")
	require.False(t, ids[dmOthers.ID], "operator listing never exposes others' DMs")

	// No duplicate channel for the doubled server role.
	serverCount := 0
	for _, ch := range channels {
		if ch.RefKey == "role:server" {
			serverCount++
		}
	}
	require.Equal(t, 1, serverCount)

	// Manager caller (staff 5): same virtual fan-out PLUS their own DMs.
	channels, err = d.ListChannelsForOperator(1, 5)
	require.NoError(t, err)
	ids = map[uint]bool{}
	for _, ch := range channels {
		ids[ch.ID] = true
	}
	require.True(t, ids[dmOthers.ID], "a manager keeps their OWN dm in the list")
}

// TestCanReadChannelOperator asserts the operator gate: owners (staffID 0) and
// managers may read/post any role or dept channel — but never someone else's DM.
func TestCanReadChannelOperator(t *testing.T) {
	d, g := newChatTestDB(t)
	require.NoError(t, g.Create(&Business{ID: 1, BusinessId: "biz-1"}).Error)

	roleCh, err := d.GetOrCreateVirtualChannel(1, "role:server", "Server")
	require.NoError(t, err)
	deptCh, err := d.GetOrCreateVirtualChannel(1, "dept:BOH", "BOH")
	require.NoError(t, err)
	dm := seedDirectChannel(t, g, 1, "dm:5:6", 5, 6)

	// Owner (staffID 0, no staff role).
	ok, err := d.CanReadChannel(1, roleCh, 0, "")
	require.NoError(t, err)
	require.True(t, ok, "owner can read any role channel")
	ok, _ = d.CanReadChannel(1, deptCh, 0, "")
	require.True(t, ok, "owner can read any dept channel")
	ok, _ = d.CanReadChannel(1, &dm, 0, "")
	require.False(t, ok, "owner cannot read staff DMs")

	// Manager (staff 9, role manager) — no position rows needed.
	ok, _ = d.CanReadChannel(1, roleCh, 9, "manager")
	require.True(t, ok, "manager can read any role channel")
	ok, _ = d.CanReadChannel(1, deptCh, 9, "manager")
	require.True(t, ok, "manager can read any dept channel")
	ok, _ = d.CanReadChannel(1, &dm, 9, "manager")
	require.False(t, ok, "manager cannot read others' DMs")
}
