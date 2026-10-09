package database

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestResolveStaffNotifyTargets(t *testing.T) {
	db, cleanup := newStaffNotificationTestDB(t) // reuses Business+Staff+StaffNotification migrate; add User below
	defer cleanup()
	require.NoError(t, db.GetGorm().AutoMigrate(&User{}))
	require.NoError(t, db.GetGorm().Create(&Business{ID: 1, BusinessId: "biz-1"}).Error)

	// Staff with a matching User (push-capable, has a language).
	withUser := Staff{BusinessID: 1, Email: "a@b1.test", Name: "A", Role: "server", IsActive: true}
	require.NoError(t, db.GetGorm().Create(&withUser).Error)
	require.NoError(t, db.GetGorm().Create(&User{Email: "a@b1.test", LanguageSelected: "es-AR"}).Error)

	// Staff with no User account (PIN-only): inbox + SSE, no push.
	noUser := Staff{BusinessID: 1, Email: "c@b1.test", Name: "C", Role: "host", IsActive: true}
	require.NoError(t, db.GetGorm().Create(&noUser).Error)

	// Inactive staff must be filtered out entirely. GORM omits the zero-value
	// IsActive:false (default:true tag fills it), so force the column off.
	gone := Staff{BusinessID: 1, Email: "d@b1.test", Name: "D", Role: "server"}
	require.NoError(t, db.GetGorm().Create(&gone).Error)
	require.NoError(t, db.GetGorm().Model(&Staff{}).Where("id = ?", gone.ID).Update("is_active", false).Error)

	out, err := db.ResolveStaffNotifyTargets(1, []uint{withUser.ID, noUser.ID, gone.ID})
	require.NoError(t, err)
	byStaff := map[uint]StaffNotifyTarget{}
	for _, r := range out {
		byStaff[r.StaffID] = r
	}
	require.Len(t, out, 2, "inactive staff excluded")
	require.NotZero(t, byStaff[withUser.ID].UserID)
	require.Equal(t, "es-AR", byStaff[withUser.ID].Language)
	require.Zero(t, byStaff[noUser.ID].UserID, "PIN-only staff resolve to user_id 0")
	require.Empty(t, byStaff[noUser.ID].Language)
}

// Staff emails are lowercased at write time, but user emails are stored
// verbatim (email/password registration path). The staff→user email join must
// be case-insensitive or a mixed-case user silently loses push notifications.
func TestResolveStaffNotifyTargets_MixedCaseUserEmail(t *testing.T) {
	db, cleanup := newStaffNotificationTestDB(t)
	defer cleanup()
	require.NoError(t, db.GetGorm().AutoMigrate(&User{}))
	require.NoError(t, db.GetGorm().Create(&Business{ID: 1, BusinessId: "biz-mixed"}).Error)

	staff := Staff{BusinessID: 1, Email: "mixed@case.test", Name: "M", Role: "server", IsActive: true}
	require.NoError(t, db.GetGorm().Create(&staff).Error)
	// User registered with mixed-case email — stored verbatim.
	require.NoError(t, db.GetGorm().Create(&User{Email: "Mixed@Case.Test", LanguageSelected: "es"}).Error)

	out, err := db.ResolveStaffNotifyTargets(1, []uint{staff.ID})
	require.NoError(t, err)
	require.Len(t, out, 1)
	require.NotZero(t, out[0].UserID, "mixed-case user email must still resolve a user_id")
	require.Equal(t, "es", out[0].Language)
}
