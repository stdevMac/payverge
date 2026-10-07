package database

import (
	"fmt"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupStaffMembershipTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared&_busy_timeout=5000", t.Name())
	g, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := g.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(8)
	require.NoError(t, g.AutoMigrate(
		&Business{}, &Staff{}, &StaffIdentity{}, &StaffMembership{},
		&StaffInvitation{}, &RBACAuditLog{},
	))
	require.NoError(t, g.Exec(`CREATE UNIQUE INDEX IF NOT EXISTS idx_staff_business_email_lower
		ON staff (business_id, LOWER(TRIM(email)))`).Error)
	require.NoError(t, g.Exec(`CREATE UNIQUE INDEX IF NOT EXISTS idx_staff_invitations_pending_email
		ON staff_invitations (business_id, LOWER(TRIM(email))) WHERE status = 'pending'`).Error)
	SetTestDB(g)
	return g
}

func createMembershipBusiness(t *testing.T, g *gorm.DB, key string) *Business {
	t.Helper()
	b := &Business{
		BusinessId: key, Name: key, OwnerAddress: "owner-" + key,
		SettlementAddr: "0x1111111111111111111111111111111111111111",
		TippingAddr:    "0x2222222222222222222222222222222222222222",
	}
	require.NoError(t, g.Create(b).Error)
	return b
}

func TestStaffService_DualWritesOneIdentityWithBusinessScopedMemberships(t *testing.T) {
	g := setupStaffMembershipTestDB(t)
	b1 := createMembershipBusiness(t, g, "membership-a")
	b2 := createMembershipBusiness(t, g, "membership-b")
	svc := GetDBWrapper().StaffService

	one := &Staff{BusinessID: b1.ID, Email: " Worker@Example.com ", Name: "Worker", Role: StaffRoleServer, IsActive: true, InvitedBy: "owner-a"}
	two := &Staff{BusinessID: b2.ID, Email: "worker@example.com", Name: "Worker", Role: StaffRoleManager, IsActive: true, InvitedBy: "owner-b"}
	require.NoError(t, svc.Create(one))
	require.NoError(t, svc.Create(two))

	var identities []StaffIdentity
	require.NoError(t, g.Find(&identities).Error)
	require.Len(t, identities, 1)
	assert.Equal(t, "worker@example.com", identities[0].NormalizedEmail)

	var memberships []StaffMembership
	require.NoError(t, g.Order("business_id").Find(&memberships).Error)
	require.Len(t, memberships, 2)
	assert.Equal(t, one.ID, memberships[0].LegacyStaffID)
	assert.Equal(t, StaffRoleServer, memberships[0].Role)
	assert.Equal(t, two.ID, memberships[1].LegacyStaffID)
	assert.Equal(t, StaffRoleManager, memberships[1].Role)

	require.NoError(t, svc.SoftDelete(one.ID))
	var firstMembership StaffMembership
	require.NoError(t, g.Where("legacy_staff_id = ?", one.ID).First(&firstMembership).Error)
	assert.False(t, firstMembership.IsActive)
	var secondMembership StaffMembership
	require.NoError(t, g.Where("legacy_staff_id = ?", two.ID).First(&secondMembership).Error)
	assert.True(t, secondMembership.IsActive, "removing one business membership must not disable another")
}

func TestStaffService_BusinessRosterQueriesPreserveActiveOnlyDefault(t *testing.T) {
	g := setupStaffMembershipTestDB(t)
	business := createMembershipBusiness(t, g, "roster-lifecycle")
	svc := GetDBWrapper().StaffService

	active := &Staff{BusinessID: business.ID, Email: "active@example.com", Name: "Active", Role: StaffRoleServer, IsActive: true, InvitedBy: "owner"}
	inactive := &Staff{BusinessID: business.ID, Email: "inactive@example.com", Name: "Inactive", Role: StaffRoleHost, IsActive: true, InvitedBy: "owner"}
	require.NoError(t, svc.Create(active))
	require.NoError(t, svc.Create(inactive))
	require.NoError(t, svc.SoftDelete(inactive.ID))

	operational, err := svc.GetByBusinessID(business.ID)
	require.NoError(t, err)
	require.Len(t, operational, 1)
	assert.Equal(t, active.ID, operational[0].ID)

	lifecycle, err := svc.GetAllByBusinessID(business.ID)
	require.NoError(t, err)
	require.Len(t, lifecycle, 2)
}

func TestStaffInvitationConstraint_ConcurrentCaseInsensitiveDuplicate(t *testing.T) {
	g := setupStaffMembershipTestDB(t)
	b := createMembershipBusiness(t, g, "invite-race")

	start := make(chan struct{})
	errs := make(chan error, 2)
	var wg sync.WaitGroup
	for i, email := range []string{"Race@Example.com", " race@example.com "} {
		wg.Add(1)
		go func(i int, email string) {
			defer wg.Done()
			<-start
			errs <- g.Create(&StaffInvitation{
				BusinessID: b.ID, Email: email, Name: "Race", Role: StaffRoleServer,
				Token: fmt.Sprintf("race-token-%d", i), Status: InvitationStatusPending,
				InvitedBy: "owner",
			}).Error
		}(i, email)
	}
	close(start)
	wg.Wait()
	close(errs)

	var successes, failures int
	for err := range errs {
		if err == nil {
			successes++
		} else {
			failures++
		}
	}
	assert.Equal(t, 1, successes)
	assert.Equal(t, 1, failures)
}
