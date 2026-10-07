package services

import (
	"testing"
	"time"

	"github.com/stdevmac/payverge/backend/internal/database"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func setupReservationClaimTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := "file:res-claim-" + t.Name() + "?mode=memory&cache=shared"
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = sqlDB.Close() })
	require.NoError(t, db.AutoMigrate(
		&database.Business{},
		&database.TableReservation{},
		&database.RBACAuditLog{},
	))
	return db
}

func seedClaimReservation(t *testing.T, db *gorm.DB) (businessID, reservationID uint) {
	t.Helper()
	biz := &database.Business{
		BusinessId: "res-claim-biz-" + t.Name(), Name: "Res Claim Biz",
		OwnerAddress: "0xowner", SettlementAddr: "0x1", TippingAddr: "0x2", IsActive: true,
	}
	require.NoError(t, db.Create(biz).Error)
	res := &database.TableReservation{
		BusinessID: biz.ID, CustomerName: "Guest", PartySize: 2,
		ReservationTime: time.Now().Add(2 * time.Hour), Duration: 90, Status: "pending",
		Source: "customer", Language: "en",
	}
	require.NoError(t, db.Create(res).Error)
	return biz.ID, res.ID
}

func TestClaimReservation_AtomicSingleWinner(t *testing.T) {
	db := setupReservationClaimTestDB(t)
	bizID, resID := seedClaimReservation(t, db)
	svc := NewReservationService(db)
	aID, bID := uint(1), uint(2)

	out, prev, err := svc.ClaimReservation(bizID, resID, ClaimActor{
		StaffID: &aID, Name: "Ana", Role: "host",
	}, false)
	require.NoError(t, err)
	require.NotNil(t, out.ClaimedByStaffID)
	assert.EqualValues(t, aID, *out.ClaimedByStaffID)
	assert.Nil(t, prev)

	// Second operator without steal is blocked.
	_, _, err = svc.ClaimReservation(bizID, resID, ClaimActor{
		StaffID: &bID, Name: "Bob", Role: "host",
	}, false)
	var held *ReservationClaimHeldError
	require.ErrorAs(t, err, &held)
	assert.Equal(t, "Ana", held.ClaimedByName)
}

func TestClaimReservation_StealAudited(t *testing.T) {
	db := setupReservationClaimTestDB(t)
	bizID, resID := seedClaimReservation(t, db)
	svc := NewReservationService(db)
	aID, mgrID := uint(1), uint(9)

	_, _, err := svc.ClaimReservation(bizID, resID, ClaimActor{
		StaffID: &aID, Name: "Ana", Role: "host",
	}, false)
	require.NoError(t, err)

	// Manager without steal flag → steal required.
	_, _, err = svc.ClaimReservation(bizID, resID, ClaimActor{
		StaffID: &mgrID, Name: "Mgr", Role: "manager", CanSteal: true,
	}, false)
	require.ErrorIs(t, err, ErrReservationClaimStealRequired)

	// Front-line with steal flag → forbidden.
	_, _, err = svc.ClaimReservation(bizID, resID, ClaimActor{
		StaffID: &aID, Name: "Other", Role: "server", CanSteal: false,
	}, true)
	require.NoError(t, err)
	// aID already holds — self re-claim succeeds; use bID for forbidden
	bID := uint(2)
	_, _, err = svc.ClaimReservation(bizID, resID, ClaimActor{
		StaffID: &bID, Name: "Bob", Role: "server", CanSteal: false,
	}, true)
	require.ErrorIs(t, err, ErrReservationClaimForbidden)

	// Manager with steal → succeeds + audit.
	out, prev, err := svc.ClaimReservation(bizID, resID, ClaimActor{
		StaffID: &mgrID, Name: "Mgr", Role: "manager", CanSteal: true,
	}, true)
	require.NoError(t, err)
	require.NotNil(t, out.ClaimedByStaffID)
	assert.EqualValues(t, mgrID, *out.ClaimedByStaffID)
	require.NotNil(t, prev)
	assert.EqualValues(t, aID, *prev)

	var audits int64
	require.NoError(t, db.Model(&database.RBACAuditLog{}).
		Where("business_id = ? AND action = ?", bizID, database.RBACActionClaimStolen).
		Count(&audits).Error)
	assert.EqualValues(t, 1, audits)
}

func TestAssertReservationClaimHeld_BlocksUnclaimedAndOther(t *testing.T) {
	db := setupReservationClaimTestDB(t)
	bizID, resID := seedClaimReservation(t, db)
	svc := NewReservationService(db)
	aID, bID := uint(1), uint(2)

	// Unclaimed → must claim first.
	err := svc.AssertReservationClaimHeld(bizID, resID, ClaimActor{StaffID: &aID, Name: "Ana", Role: "host"})
	var held *ReservationClaimHeldError
	require.ErrorAs(t, err, &held)

	_, _, err = svc.ClaimReservation(bizID, resID, ClaimActor{
		StaffID: &aID, Name: "Ana", Role: "host",
	}, false)
	require.NoError(t, err)

	err = svc.AssertReservationClaimHeld(bizID, resID, ClaimActor{StaffID: &bID, Name: "Bob", Role: "host"})
	require.ErrorAs(t, err, &held)
	assert.Equal(t, "Ana", held.ClaimedByName)

	require.NoError(t, svc.AssertReservationClaimHeld(bizID, resID, ClaimActor{StaffID: &aID, Name: "Ana", Role: "host"}))
}

// L4-8: manager force-release of another operator's claim must write audit.
func TestReleaseReservation_ForceReleaseIsAudited(t *testing.T) {
	db := setupReservationClaimTestDB(t)
	bizID, resID := seedClaimReservation(t, db)
	svc := NewReservationService(db)
	aID, mgrID := uint(1), uint(9)

	_, _, err := svc.ClaimReservation(bizID, resID, ClaimActor{
		StaffID: &aID, Name: "Ana", Role: "host",
	}, false)
	require.NoError(t, err)

	require.NoError(t, svc.ReleaseReservation(bizID, resID, ClaimActor{
		StaffID: &mgrID, Name: "Mgr", Role: "manager", CanSteal: true,
	}, false))

	var audits int64
	require.NoError(t, db.Model(&database.RBACAuditLog{}).
		Where("business_id = ? AND action = ?", bizID, database.RBACActionClaimForceReleased).
		Count(&audits).Error)
	assert.EqualValues(t, 1, audits, "force-release must leave claim_force_released audit")

	// Self-release must not audit as force-release.
	_, _, err = svc.ClaimReservation(bizID, resID, ClaimActor{
		StaffID: &aID, Name: "Ana", Role: "host",
	}, false)
	require.NoError(t, err)
	require.NoError(t, svc.ReleaseReservation(bizID, resID, ClaimActor{
		StaffID: &aID, Name: "Ana", Role: "host",
	}, false))
	require.NoError(t, db.Model(&database.RBACAuditLog{}).
		Where("business_id = ? AND action = ?", bizID, database.RBACActionClaimForceReleased).
		Count(&audits).Error)
	assert.EqualValues(t, 1, audits)
}

// H2: owner principal (StaffID nil) force-release must not write staff_id=0.
func TestReleaseReservation_OwnerForceReleaseDoesNotUseStaffZero(t *testing.T) {
	db := setupReservationClaimTestDB(t)
	bizID, resID := seedClaimReservation(t, db)
	svc := NewReservationService(db)
	aID := uint(3)

	_, _, err := svc.ClaimReservation(bizID, resID, ClaimActor{
		StaffID: &aID, Name: "Ana", Role: "host",
	}, false)
	require.NoError(t, err)

	// Owner: StaffID nil, CanSteal true — previously wrote staff_id=0 (FK fail on PG).
	require.NoError(t, svc.ReleaseReservation(bizID, resID, ClaimActor{
		StaffID: nil, Name: "Owner", Role: "owner", CanSteal: true,
	}, false))

	var audits []database.RBACAuditLog
	require.NoError(t, db.Where("business_id = ? AND action = ?", bizID, database.RBACActionClaimForceReleased).
		Find(&audits).Error)
	require.Len(t, audits, 1)
	assert.Equal(t, aID, audits[0].StaffID, "audit staff_id must be previous holder, never 0")
	assert.NotEqual(t, uint(0), audits[0].StaffID)
	assert.Equal(t, "Owner", audits[0].ChangedBy)
}

// Auto post-mutate release (selfOnly) must not clear a claim stolen mid-flight.
func TestReleaseReservation_SelfOnlyDoesNotForceClearStolenClaim(t *testing.T) {
	db := setupReservationClaimTestDB(t)
	bizID, resID := seedClaimReservation(t, db)
	svc := NewReservationService(db)
	mgrID, ownerStaff := uint(9), uint(0)
	_ = ownerStaff

	// Manager claims.
	_, _, err := svc.ClaimReservation(bizID, resID, ClaimActor{
		StaffID: &mgrID, Name: "Mgr", Role: "manager", CanSteal: true,
	}, false)
	require.NoError(t, err)

	// Owner steals.
	_, _, err = svc.ClaimReservation(bizID, resID, ClaimActor{
		StaffID: nil, Name: "Owner", Role: "owner", CanSteal: true,
	}, true)
	require.NoError(t, err)

	// Manager's automatic finally-release with selfOnly must be a no-op.
	require.NoError(t, svc.ReleaseReservation(bizID, resID, ClaimActor{
		StaffID: &mgrID, Name: "Mgr", Role: "manager", CanSteal: true,
	}, true))

	var row database.TableReservation
	require.NoError(t, db.Where("id = ?", resID).First(&row).Error)
	require.NotNil(t, row.ClaimedAt, "owner claim must still be held")
	assert.Equal(t, "Owner", row.ClaimedByName)
	assert.Equal(t, "owner", row.ClaimedByRole)

	var forceAudits int64
	require.NoError(t, db.Model(&database.RBACAuditLog{}).
		Where("business_id = ? AND action = ? AND changed_by = ?",
			bizID, database.RBACActionClaimForceReleased, "Mgr").
		Count(&forceAudits).Error)
	assert.EqualValues(t, 0, forceAudits, "self_only must not write force-release audit for Mgr")
}

// Source-shape gate: claim columns must exist on the model (Rule 1 enforcement).
func TestTableReservation_ClaimFieldsPresent(t *testing.T) {
	r := database.TableReservation{}
	// Compile-time: fields exist. Runtime: zero values mean unclaimed.
	assert.Nil(t, r.ClaimedByStaffID)
	assert.Equal(t, "", r.ClaimedByName)
	assert.Nil(t, r.ClaimedAt)
}
