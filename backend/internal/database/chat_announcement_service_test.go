package database

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// TestCreateAndListAnnouncementsAudience proves audience filtering: a server with
// an active BOH position sees "all" + their role channel + their dept channel,
// but NOT another role's or another dept's announcements. Validation (title,
// audience syntax) and the "" -> "all" default are covered too.
func TestCreateAndListAnnouncementsAudience(t *testing.T) {
	d, g := newChatTestDB(t)
	require.NoError(t, g.Create(&Business{ID: 1, BusinessId: "biz-1"}).Error)
	require.NoError(t, g.Create(&Position{ID: 9, BusinessID: 1, Name: "Line Cook", Department: "BOH", IsActive: true}).Error)
	require.NoError(t, g.Create(&StaffPosition{BusinessID: 1, StaffID: 5, PositionID: 9}).Error)

	all, err := d.CreateAnnouncement(1, 1, "All hands", "body", false, "all")
	require.NoError(t, err)
	require.Equal(t, "all", all.AudienceFilter)
	_, err = d.CreateAnnouncement(1, 1, "Servers", "tips", true, "role:server")
	require.NoError(t, err)
	_, err = d.CreateAnnouncement(1, 1, "Hosts", "seating", false, "role:host")
	require.NoError(t, err)
	_, err = d.CreateAnnouncement(1, 1, "Kitchen", "prep", false, "dept:BOH")
	require.NoError(t, err)
	_, err = d.CreateAnnouncement(1, 1, "Front", "x", false, "dept:FOH")
	require.NoError(t, err)

	// Empty audience defaults to "all".
	def, err := d.CreateAnnouncement(1, 1, "Default", "x", false, "")
	require.NoError(t, err)
	require.Equal(t, "all", def.AudienceFilter)

	// Invalid audience and empty title are rejected.
	_, err = d.CreateAnnouncement(1, 1, "Bad", "x", false, "team:lol")
	require.ErrorIs(t, err, ErrInvalidAudienceFilter)
	_, err = d.CreateAnnouncement(1, 1, "   ", "x", false, "all")
	require.ErrorIs(t, err, ErrAnnouncementTitleRequired)

	got, err := d.ListAnnouncements(1, 5, "server", false)
	require.NoError(t, err)
	titles := map[string]bool{}
	for _, a := range got {
		titles[a.Title] = true
	}
	require.True(t, titles["All hands"], "audience=all visible to everyone")
	require.True(t, titles["Default"], "default-all visible")
	require.True(t, titles["Servers"], "own role announcement visible")
	require.True(t, titles["Kitchen"], "dept:BOH visible to staff with an active BOH position")
	require.False(t, titles["Hosts"], "role:host hidden from a server")
	require.False(t, titles["Front"], "dept:FOH hidden from staff without an FOH position")
}

// TestAnnouncementAckIdempotentAndStatus proves double-ack is idempotent and the
// X-of-Y aggregate (acked count + total eligible audience + the unacked roster)
// is correct and audience-scoped (a host is excluded from a role:server roster).
func TestAnnouncementAckIdempotentAndStatus(t *testing.T) {
	d, g := newChatTestDB(t)
	require.NoError(t, g.Create(&Business{ID: 1, BusinessId: "biz-1"}).Error)
	require.NoError(t, g.Create(&Staff{ID: 5, BusinessID: 1, Email: "s5@x.co", Name: "Ana", Role: StaffRoleServer, IsActive: true, InvitedBy: "o"}).Error)
	require.NoError(t, g.Create(&Staff{ID: 6, BusinessID: 1, Email: "s6@x.co", Name: "Bo", Role: StaffRoleServer, IsActive: true, InvitedBy: "o"}).Error)
	require.NoError(t, g.Create(&Staff{ID: 7, BusinessID: 1, Email: "s7@x.co", Name: "Cy", Role: StaffRoleServer, IsActive: true, InvitedBy: "o"}).Error)
	require.NoError(t, g.Create(&Staff{ID: 8, BusinessID: 1, Email: "s8@x.co", Name: "Di", Role: StaffRoleHost, IsActive: true, InvitedBy: "o"}).Error)

	ann, err := d.CreateAnnouncement(1, 5, "Servers only", "read me", true, "role:server")
	require.NoError(t, err)

	_, err = d.AckAnnouncement(1, ann.ID, 5)
	require.NoError(t, err)
	_, err = d.AckAnnouncement(1, ann.ID, 5) // double ack
	require.NoError(t, err)
	var ackRows int64
	require.NoError(t, g.Model(&AnnouncementAck{}).Where("announcement_id = ? AND staff_id = ?", ann.ID, 5).Count(&ackRows).Error)
	require.Equal(t, int64(1), ackRows, "double-ack stays idempotent (one row)")

	// A foreign-tenant announcement id is not-found.
	_, err = d.AckAnnouncement(2, ann.ID, 5)
	require.ErrorIs(t, err, ErrAnnouncementNotFound)

	st, err := d.GetAnnouncementAckStatus(1, ann.ID)
	require.NoError(t, err)
	require.Equal(t, 3, st.TotalEligible, "three active servers eligible (host excluded)")
	require.Equal(t, 1, st.Acked, "one server acked")
	require.Len(t, st.Unacked, 2, "two servers still unacked")
	unacked := map[uint]bool{}
	for _, m := range st.Unacked {
		unacked[m.StaffID] = true
	}
	require.True(t, unacked[6] && unacked[7], "the two non-acking servers are listed")
	require.False(t, unacked[5], "the acked staff is not in the unacked list")
	require.False(t, unacked[8], "the out-of-audience host is not counted")

	// "all" audience counts every active staff member.
	annAll, err := d.CreateAnnouncement(1, 5, "Everyone", "x", true, "all")
	require.NoError(t, err)
	stAll, err := d.GetAnnouncementAckStatus(1, annAll.ID)
	require.NoError(t, err)
	require.Equal(t, 4, stAll.TotalEligible, "all four active staff eligible for audience=all")
	require.Equal(t, 0, stAll.Acked)
}

// TestAnnouncementAckStatusAccessShape locks the "X of Y confirmed" aggregate to a
// bounded, N+1-free shape: resolving the acked count, total eligible audience, and
// the unacked roster must NOT issue a per-staff query. With 12 eligible staff an
// N+1 would surface as ~12 extra SELECTs; we assert a small constant instead.
func TestAnnouncementAckStatusAccessShape(t *testing.T) {
	rec := &laborSQLRecorder{Interface: logger.Default.LogMode(logger.Silent)}
	dsn := fmt.Sprintf("file:%s-%d?mode=memory&cache=shared", t.Name(), chatTestDBSeq.Add(1))
	g, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{Logger: rec})
	require.NoError(t, err)
	sqlDB, err := g.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, g.AutoMigrate(&Business{}, &Staff{}, &Position{}, &StaffPosition{}, &Announcement{}, &AnnouncementAck{}))
	prev := db
	SetTestDB(g)
	t.Cleanup(func() { SetTestDB(prev) })
	d := GetDBWrapper()

	require.NoError(t, g.Create(&Business{ID: 1, BusinessId: "biz-1"}).Error)
	for i := uint(1); i <= 12; i++ {
		require.NoError(t, g.Create(&Staff{ID: i, BusinessID: 1, Email: fmt.Sprintf("s%d@x.co", i),
			Name: fmt.Sprintf("S%d", i), Role: StaffRoleServer, IsActive: true, InvitedBy: "o"}).Error)
	}
	ann, err := d.CreateAnnouncement(1, 1, "Servers", "x", true, "role:server")
	require.NoError(t, err)
	_, err = d.AckAnnouncement(1, ann.ID, 3)
	require.NoError(t, err)

	rec.stmts = nil
	st, err := d.GetAnnouncementAckStatus(1, ann.ID)
	require.NoError(t, err)
	require.Equal(t, 12, st.TotalEligible)
	require.Equal(t, 1, st.Acked)
	require.Len(t, st.Unacked, 11)

	sels := selectStmts(rec)
	require.LessOrEqual(t, len(sels), 3,
		"ack aggregate must be a small constant number of SELECTs (announcement + roster JOIN), never N+1 per staff; got %d", len(sels))
}

// TestGetAnnouncementAckSummaries proves the batch ack-summary read returns a
// correct {acked, total_eligible, first names} per announcement across MIXED
// audiences (all / role / dept) in one call, tenant-scoped, with per-announcement
// eligibility resolved exactly as GetAnnouncementAckStatus resolves it.
func TestGetAnnouncementAckSummaries(t *testing.T) {
	d, g := newChatTestDB(t)
	require.NoError(t, g.Create(&Business{ID: 1, BusinessId: "biz-1"}).Error)
	require.NoError(t, g.Create(&Position{ID: 9, BusinessID: 1, Name: "Line Cook", Department: "BOH", IsActive: true}).Error)
	// 3 servers, 1 host, 1 BOH cook.
	require.NoError(t, g.Create(&Staff{ID: 1, BusinessID: 1, Email: "a@x.co", Name: "Ana", Role: StaffRoleServer, IsActive: true, InvitedBy: "o"}).Error)
	require.NoError(t, g.Create(&Staff{ID: 2, BusinessID: 1, Email: "b@x.co", Name: "Bo", Role: StaffRoleServer, IsActive: true, InvitedBy: "o"}).Error)
	require.NoError(t, g.Create(&Staff{ID: 3, BusinessID: 1, Email: "c@x.co", Name: "Cy", Role: StaffRoleServer, IsActive: true, InvitedBy: "o"}).Error)
	require.NoError(t, g.Create(&Staff{ID: 4, BusinessID: 1, Email: "d@x.co", Name: "Di", Role: StaffRoleHost, IsActive: true, InvitedBy: "o"}).Error)
	require.NoError(t, g.Create(&Staff{ID: 5, BusinessID: 1, Email: "e@x.co", Name: "El", Role: StaffRoleKitchen, IsActive: true, InvitedBy: "o"}).Error)
	require.NoError(t, g.Create(&StaffPosition{BusinessID: 1, StaffID: 5, PositionID: 9}).Error)

	annAll, err := d.CreateAnnouncement(1, 1, "Everyone", "x", true, "all")
	require.NoError(t, err)
	annServer, err := d.CreateAnnouncement(1, 1, "Servers", "x", true, "role:server")
	require.NoError(t, err)
	annBOH, err := d.CreateAnnouncement(1, 1, "Kitchen", "x", true, "dept:BOH")
	require.NoError(t, err)

	// Ana acks all + server; El acks the BOH one.
	_, err = d.AckAnnouncement(1, annAll.ID, 1)
	require.NoError(t, err)
	_, err = d.AckAnnouncement(1, annServer.ID, 1)
	require.NoError(t, err)
	_, err = d.AckAnnouncement(1, annBOH.ID, 5)
	require.NoError(t, err)

	summaries, err := d.GetAnnouncementAckSummaries(1, []uint{annAll.ID, annServer.ID, annBOH.ID}, 3)
	require.NoError(t, err)
	require.Len(t, summaries, 3)

	all := summaries[annAll.ID]
	require.Equal(t, 5, all.TotalEligible, "all five active staff eligible for audience=all")
	require.Equal(t, 1, all.Acked)
	require.Equal(t, []string{"Ana"}, all.FirstAckerNames)

	srv := summaries[annServer.ID]
	require.Equal(t, 3, srv.TotalEligible, "three active servers")
	require.Equal(t, 1, srv.Acked)
	require.Equal(t, []string{"Ana"}, srv.FirstAckerNames)

	boh := summaries[annBOH.ID]
	require.Equal(t, 1, boh.TotalEligible, "one BOH cook")
	require.Equal(t, 1, boh.Acked)
	require.Equal(t, []string{"El"}, boh.FirstAckerNames)

	// Tenant scoping: a foreign business gets nothing.
	foreign, err := d.GetAnnouncementAckSummaries(2, []uint{annAll.ID}, 3)
	require.NoError(t, err)
	require.Empty(t, foreign)

	// Empty id list is a no-op.
	none, err := d.GetAnnouncementAckSummaries(1, nil, 3)
	require.NoError(t, err)
	require.Empty(t, none)
}

// TestGetAnnouncementAckSummariesAccessShape locks the batch summary to a bounded,
// N+1-free shape: resolving acked counts + total-eligible per audience + first
// acker names for MANY announcements must NOT issue a per-announcement query. With
// 30 announcements an N+1 would surface as ~30+ extra SELECTs; we assert a small
// constant independent of announcement count.
func TestGetAnnouncementAckSummariesAccessShape(t *testing.T) {
	rec := &laborSQLRecorder{Interface: logger.Default.LogMode(logger.Silent)}
	dsn := fmt.Sprintf("file:%s-%d?mode=memory&cache=shared", t.Name(), chatTestDBSeq.Add(1))
	g, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{Logger: rec})
	require.NoError(t, err)
	sqlDB, err := g.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, g.AutoMigrate(&Business{}, &Staff{}, &Position{}, &StaffPosition{}, &Announcement{}, &AnnouncementAck{}))
	prev := db
	SetTestDB(g)
	t.Cleanup(func() { SetTestDB(prev) })
	d := GetDBWrapper()

	require.NoError(t, g.Create(&Business{ID: 1, BusinessId: "biz-1"}).Error)
	require.NoError(t, g.Create(&Position{ID: 9, BusinessID: 1, Name: "Cook", Department: "BOH", IsActive: true}).Error)
	for i := uint(1); i <= 12; i++ {
		require.NoError(t, g.Create(&Staff{ID: i, BusinessID: 1, Email: fmt.Sprintf("s%d@x.co", i),
			Name: fmt.Sprintf("S%d", i), Role: StaffRoleServer, IsActive: true, InvitedBy: "o"}).Error)
	}
	// Staff 3 must be eligible for dept:BOH acks used below.
	require.NoError(t, g.Create(&StaffPosition{BusinessID: 1, StaffID: 3, PositionID: 9}).Error)
	ids := make([]uint, 0, 30)
	for i := 0; i < 30; i++ {
		aud := "all"
		if i%3 == 1 {
			aud = "role:server"
		} else if i%3 == 2 {
			aud = "dept:BOH"
		}
		ann, err := d.CreateAnnouncement(1, 1, fmt.Sprintf("N%d", i), "x", true, aud)
		require.NoError(t, err)
		_, err = d.AckAnnouncement(1, ann.ID, 3)
		require.NoError(t, err)
		ids = append(ids, ann.ID)
	}

	rec.stmts = nil
	summaries, err := d.GetAnnouncementAckSummaries(1, ids, 3)
	require.NoError(t, err)
	require.Len(t, summaries, 30)

	sels := selectStmts(rec)
	// Constant fan-out: announcements + active-staff acked counts (all + per
	// role + per dept families) + first names + per-audience totals. Must stay
	// well below announcement count (30 here) — never N+1 per notice.
	require.LessOrEqual(t, len(sels), 12,
		"batch ack summary must be a small constant number of SELECTs, never N+1 per announcement; got %d", len(sels))
}

// TestUpdateAnnouncement proves an in-place edit preserves the id + ack rows,
// bumps the content, re-validates the audience, and is tenant-scoped.
func TestUpdateAnnouncement(t *testing.T) {
	d, g := newChatTestDB(t)
	require.NoError(t, g.Create(&Business{ID: 1, BusinessId: "biz-1"}).Error)
	require.NoError(t, g.Create(&Staff{ID: 5, BusinessID: 1, Email: "s5@x.co", Name: "Ana", Role: StaffRoleServer, IsActive: true, InvitedBy: "o"}).Error)

	ann, err := d.CreateAnnouncement(1, 1, "Old title", "old body", false, "all")
	require.NoError(t, err)
	_, err = d.AckAnnouncement(1, ann.ID, 5)
	require.NoError(t, err)

	updated, err := d.UpdateAnnouncement(1, ann.ID, "New title", "new body", true, "role:server")
	require.NoError(t, err)
	require.Equal(t, ann.ID, updated.ID, "edit preserves the row id")
	require.Equal(t, "New title", updated.Title)
	require.Equal(t, "new body", updated.Content)
	require.True(t, updated.RequireAck)
	require.Equal(t, "role:server", updated.AudienceFilter)

	// Ack row survives the edit (id preserved).
	var ackRows int64
	require.NoError(t, g.Model(&AnnouncementAck{}).Where("announcement_id = ?", ann.ID).Count(&ackRows).Error)
	require.Equal(t, int64(1), ackRows, "existing acks are preserved across an edit")

	// Empty title and malformed audience are rejected.
	_, err = d.UpdateAnnouncement(1, ann.ID, "  ", "x", false, "all")
	require.ErrorIs(t, err, ErrAnnouncementTitleRequired)
	_, err = d.UpdateAnnouncement(1, ann.ID, "T", "x", false, "team:lol")
	require.ErrorIs(t, err, ErrInvalidAudienceFilter)

	// Cross-tenant edit is not-found.
	_, err = d.UpdateAnnouncement(2, ann.ID, "T", "x", false, "all")
	require.ErrorIs(t, err, ErrAnnouncementNotFound)
}

// TestDeleteAnnouncement proves a hard delete removes the announcement AND its
// ack rows (stopping ack tracking), records an RBAC audit entry, and is
// tenant-scoped.
func TestDeleteAnnouncement(t *testing.T) {
	d, g := newChatTestDB(t)
	require.NoError(t, g.AutoMigrate(&RBACAuditLog{}))
	require.NoError(t, g.Create(&Business{ID: 1, BusinessId: "biz-1"}).Error)
	require.NoError(t, g.Create(&Staff{ID: 5, BusinessID: 1, Email: "s5@x.co", Name: "Ana", Role: StaffRoleServer, IsActive: true, InvitedBy: "o"}).Error)

	ann, err := d.CreateAnnouncement(1, 1, "Doomed", "x", true, "all")
	require.NoError(t, err)
	_, err = d.AckAnnouncement(1, ann.ID, 5)
	require.NoError(t, err)

	// Cross-tenant delete is not-found (no rows touched).
	require.ErrorIs(t, d.DeleteAnnouncement(2, ann.ID, 1), ErrAnnouncementNotFound)
	var still int64
	require.NoError(t, g.Model(&Announcement{}).Where("id = ?", ann.ID).Count(&still).Error)
	require.Equal(t, int64(1), still, "a cross-tenant delete must not remove the row")

	require.NoError(t, d.DeleteAnnouncement(1, ann.ID, 5))

	var annRows, ackRows int64
	require.NoError(t, g.Model(&Announcement{}).Where("id = ?", ann.ID).Count(&annRows).Error)
	require.NoError(t, g.Model(&AnnouncementAck{}).Where("announcement_id = ?", ann.ID).Count(&ackRows).Error)
	require.Equal(t, int64(0), annRows, "announcement removed")
	require.Equal(t, int64(0), ackRows, "ack tracking stops (rows cascaded)")

	// A second delete is not-found.
	require.ErrorIs(t, d.DeleteAnnouncement(1, ann.ID, 5), ErrAnnouncementNotFound)

	// An audit trail row was written (the table has no soft-delete column).
	var auditRows int64
	require.NoError(t, g.Model(&RBACAuditLog{}).Where("business_id = ? AND action = ?", 1, RBACActionAnnouncementDeleted).Count(&auditRows).Error)
	require.Equal(t, int64(1), auditRows, "delete records an RBAC audit entry")
}

// BenchmarkGetAnnouncementAckSummaries captures the batch-summary baseline: this
// read replaces the per-announcement AckRoster poll, so it must stay cheap even
// with a full feed of require_ack notices across mixed audiences.
func BenchmarkGetAnnouncementAckSummaries(b *testing.B) {
	dsn := fmt.Sprintf("file:%s-%d?mode=memory&cache=shared", b.Name(), chatTestDBSeq.Add(1))
	g, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(b, err)
	sqlDB, err := g.DB()
	require.NoError(b, err)
	sqlDB.SetMaxOpenConns(1)
	require.NoError(b, g.AutoMigrate(&Business{}, &Staff{}, &Position{}, &StaffPosition{}, &Announcement{}, &AnnouncementAck{}))
	prev := db
	SetTestDB(g)
	b.Cleanup(func() { SetTestDB(prev) })
	d := GetDBWrapper()

	require.NoError(b, g.Create(&Business{ID: 1, BusinessId: "biz-1"}).Error)
	require.NoError(b, g.Create(&Position{ID: 9, BusinessID: 1, Name: "Cook", Department: "BOH", IsActive: true}).Error)
	for i := uint(1); i <= 30; i++ {
		require.NoError(b, g.Create(&Staff{ID: i, BusinessID: 1, Email: fmt.Sprintf("s%d@x.co", i),
			Name: fmt.Sprintf("S%d", i), Role: StaffRoleServer, IsActive: true, InvitedBy: "o"}).Error)
	}
	// AckAnnouncement enforces audience eligibility; staff 3 acks every notice
	// below, so a BOH position keeps it inside the dept:BOH audiences too.
	require.NoError(b, g.Create(&StaffPosition{BusinessID: 1, StaffID: 3, PositionID: 9}).Error)
	ids := make([]uint, 0, 50)
	for i := 0; i < 50; i++ {
		aud := "all"
		if i%3 == 1 {
			aud = "role:server"
		} else if i%3 == 2 {
			aud = "dept:BOH"
		}
		ann, err := d.CreateAnnouncement(1, 1, fmt.Sprintf("N%d", i), "x", true, aud)
		require.NoError(b, err)
		_, err = d.AckAnnouncement(1, ann.ID, 3)
		require.NoError(b, err)
		ids = append(ids, ann.ID)
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := d.GetAnnouncementAckSummaries(1, ids, 3); err != nil {
			b.Fatal(err)
		}
	}
}

func TestListAnnouncementAudienceStaffIDs(t *testing.T) {
	d, g := newChatTestDB(t)
	require.NoError(t, g.Create(&Business{ID: 1, BusinessId: "biz-1"}).Error)
	server1 := Staff{BusinessID: 1, Email: "s1@b.test", Name: "S1", Role: "server", IsActive: true, InvitedBy: "o"}
	host := Staff{BusinessID: 1, Email: "h@b.test", Name: "H", Role: "host", IsActive: true, InvitedBy: "o"}
	gone := Staff{BusinessID: 1, Email: "g@b.test", Name: "G", Role: "server", IsActive: true, InvitedBy: "o"}
	require.NoError(t, g.Create(&server1).Error)
	require.NoError(t, g.Create(&host).Error)
	require.NoError(t, g.Create(&gone).Error)
	require.NoError(t, g.Model(&gone).Update("is_active", false).Error)

	// audience "all" → active staff only (server1 + host), never the inactive one.
	annAll, err := d.CreateAnnouncement(1, server1.ID, "T", "C", false, "all")
	require.NoError(t, err)
	ids, err := d.ListAnnouncementAudienceStaffIDs(1, annAll.ID)
	require.NoError(t, err)
	require.ElementsMatch(t, []uint{server1.ID, host.ID}, ids)

	// audience "role:server" → only servers.
	annRole, err := d.CreateAnnouncement(1, server1.ID, "T", "C", false, "role:server")
	require.NoError(t, err)
	ids, err = d.ListAnnouncementAudienceStaffIDs(1, annRole.ID)
	require.NoError(t, err)
	require.ElementsMatch(t, []uint{server1.ID}, ids)
}

// TestCountUnackedAnnouncements proves the badge count only tallies require_ack
// announcements the caller is targeted by AND has not yet acked. Non-ack notices,
// out-of-audience notices, and already-acked notices must be excluded; an owner
// (staffID 0, who cannot ack) counts nothing.
func TestCountUnackedAnnouncements(t *testing.T) {
	d, g := newChatTestDB(t)
	require.NoError(t, g.Create(&Business{ID: 1, BusinessId: "biz-1"}).Error)
	require.NoError(t, g.Create(&Staff{ID: 5, BusinessID: 1, Email: "s5@x.co", Name: "Sam", Role: StaffRoleServer, IsActive: true, InvitedBy: "o"}).Error)
	require.NoError(t, g.Create(&Position{ID: 9, BusinessID: 1, Name: "Line Cook", Department: "BOH", IsActive: true}).Error)
	require.NoError(t, g.Create(&StaffPosition{BusinessID: 1, StaffID: 5, PositionID: 9}).Error)

	// Three that COUNT for a server holding a BOH position: all / own role / own dept.
	annAll, err := d.CreateAnnouncement(1, 1, "All hands", "read", true, "all")
	require.NoError(t, err)
	_, err = d.CreateAnnouncement(1, 1, "Servers", "read", true, "role:server")
	require.NoError(t, err)
	_, err = d.CreateAnnouncement(1, 1, "Kitchen", "read", true, "dept:BOH")
	require.NoError(t, err)
	// Excluded: no ack required, wrong role, wrong dept.
	_, err = d.CreateAnnouncement(1, 1, "FYI", "fyi", false, "all")
	require.NoError(t, err)
	_, err = d.CreateAnnouncement(1, 1, "Hosts", "read", true, "role:host")
	require.NoError(t, err)
	_, err = d.CreateAnnouncement(1, 1, "Front", "read", true, "dept:FOH")
	require.NoError(t, err)

	n, err := d.CountUnackedAnnouncements(1, 5, "server")
	require.NoError(t, err)
	require.Equal(t, int64(3), n, "all + role:server + dept:BOH require_ack, none acked yet")

	// Acking one require_ack notice drops the count.
	_, err = d.AckAnnouncement(1, annAll.ID, 5)
	require.NoError(t, err)
	n, err = d.CountUnackedAnnouncements(1, 5, "server")
	require.NoError(t, err)
	require.Equal(t, int64(2), n, "the acked notice no longer nags")

	// An owner (no staff row) has nothing to ack.
	n, err = d.CountUnackedAnnouncements(1, 0, "")
	require.NoError(t, err)
	require.Equal(t, int64(0), n)

	// Tenant scoping — a foreign business sees nothing.
	n, err = d.CountUnackedAnnouncements(2, 5, "server")
	require.NoError(t, err)
	require.Equal(t, int64(0), n)
}

// TestCountUnackedAnnouncementsAccessShape locks the badge count to a bounded,
// N+1-free shape: resolving the caller's departments then a single NOT-EXISTS
// count, never a per-announcement query. With many targeted notices an N+1 would
// surface as one SELECT per row; we assert a small constant instead.
func TestCountUnackedAnnouncementsAccessShape(t *testing.T) {
	rec := &laborSQLRecorder{Interface: logger.Default.LogMode(logger.Silent)}
	dsn := fmt.Sprintf("file:%s-%d?mode=memory&cache=shared", t.Name(), chatTestDBSeq.Add(1))
	g, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{Logger: rec})
	require.NoError(t, err)
	sqlDB, err := g.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, g.AutoMigrate(&Business{}, &Staff{}, &Position{}, &StaffPosition{}, &Announcement{}, &AnnouncementAck{}))
	prev := db
	SetTestDB(g)
	t.Cleanup(func() { SetTestDB(prev) })
	d := GetDBWrapper()

	require.NoError(t, g.Create(&Business{ID: 1, BusinessId: "biz-1"}).Error)
	for i := 0; i < 20; i++ {
		_, err := d.CreateAnnouncement(1, 1, fmt.Sprintf("N%d", i), "x", true, "role:server")
		require.NoError(t, err)
	}

	rec.stmts = nil
	n, err := d.CountUnackedAnnouncements(1, 5, "server")
	require.NoError(t, err)
	require.Equal(t, int64(20), n)

	sels := selectStmts(rec)
	require.LessOrEqual(t, len(sels), 2,
		"badge count must be a small constant number of SELECTs (dept resolution + one NOT-EXISTS count), never N+1 per announcement; got %d", len(sels))
}

// BenchmarkCountUnackedAnnouncements captures the badge-count baseline; this read
// backs a polled staff-nav badge, so it stays cheap and allocation-light.
func BenchmarkCountUnackedAnnouncements(b *testing.B) {
	dsn := fmt.Sprintf("file:%s-%d?mode=memory&cache=shared", b.Name(), chatTestDBSeq.Add(1))
	g, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(b, err)
	sqlDB, err := g.DB()
	require.NoError(b, err)
	sqlDB.SetMaxOpenConns(1)
	require.NoError(b, g.AutoMigrate(&Business{}, &Staff{}, &Position{}, &StaffPosition{}, &Announcement{}, &AnnouncementAck{}))
	prev := db
	SetTestDB(g)
	b.Cleanup(func() { SetTestDB(prev) })
	d := GetDBWrapper()

	require.NoError(b, g.Create(&Business{ID: 1, BusinessId: "biz-1"}).Error)
	require.NoError(b, g.Create(&Position{ID: 9, BusinessID: 1, Name: "Line Cook", Department: "BOH", IsActive: true}).Error)
	require.NoError(b, g.Create(&StaffPosition{BusinessID: 1, StaffID: 5, PositionID: 9}).Error)
	for i := 0; i < 50; i++ {
		aud := "all"
		if i%3 == 0 {
			aud = "role:server"
		} else if i%3 == 1 {
			aud = "dept:BOH"
		}
		_, err := d.CreateAnnouncement(1, 1, fmt.Sprintf("N%d", i), "x", true, aud)
		require.NoError(b, err)
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := d.CountUnackedAnnouncements(1, 5, "server"); err != nil {
			b.Fatal(err)
		}
	}
}
