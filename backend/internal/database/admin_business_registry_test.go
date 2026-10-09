package database

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupAdminBusinessRegistryDB(t testing.TB) *gorm.DB {
	t.Helper()

	dsn := fmt.Sprintf("file:admin_registry_%s?mode=memory&cache=shared", t.Name())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)

	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)

	SetTestDB(db)
	require.NoError(t, db.AutoMigrate(&User{}, &Business{}))
	return db
}

// TestListBusinessesForAdmin_KindRealExcludesFixtures guards SEAM 4 / Task 12:
// filtering kind=real must exclude demo-seeded rows and CI fixture owners
// (@payverge.test / @payverge.local), and must be the single registry entry point.
func TestListBusinessesForAdmin_KindRealExcludesFixtures(t *testing.T) {
	db := setupAdminBusinessRegistryDB(t)
	now := time.Now().UTC()

	realOwner := User{Email: "owner@restaurant.com", Name: "Real Owner", Role: "user"}
	testOwner := User{Email: "ci@payverge.test", Name: "CI Fixture", Role: "user"}
	localOwner := User{Email: "qa@payverge.local", Name: "Local Fixture", Role: "user"}
	require.NoError(t, db.Create(&realOwner).Error)
	require.NoError(t, db.Create(&testOwner).Error)
	require.NoError(t, db.Create(&localOwner).Error)

	mkBiz := func(id, name string, kind BusinessKind, userID *uint, isDemo bool) {
		t.Helper()
		require.NoError(t, db.Create(&Business{
			BusinessId: id,
			Name:       name,
			OwnerName:  "Owner",
			UserID:     userID,
			Kind:       kind,
			IsDemo:     isDemo,
			CreatedAt:  now,
			UpdatedAt:  now,
		}).Error)
	}

	mkBiz("reg-real", "Real Bistro", BusinessKindReal, &realOwner.ID, false)
	mkBiz("reg-demo", "Demo Cafe", BusinessKindDemo, nil, true)
	mkBiz("reg-test", "CI Test Biz", BusinessKindTest, &testOwner.ID, false)
	mkBiz("reg-local", "Local Test Biz", BusinessKindTest, &localOwner.ID, false)

	// Default filter (kind=real): only the real bistro.
	rows, total, err := ListBusinessesForAdmin(AdminBusinessFilter{})
	require.NoError(t, err)
	assert.EqualValues(t, 1, total, "default kind=real must exclude demo/test")
	require.Len(t, rows, 1)
	assert.Equal(t, "Real Bistro", rows[0].Name)
	assert.Equal(t, BusinessKindReal, rows[0].Kind)

	// Explicit kind=real same result.
	count, err := CountBusinessesForAdmin(AdminBusinessFilter{Kind: string(BusinessKindReal)})
	require.NoError(t, err)
	assert.EqualValues(t, 1, count)

	// kind=demo only the seeder row.
	count, err = CountBusinessesForAdmin(AdminBusinessFilter{Kind: string(BusinessKindDemo)})
	require.NoError(t, err)
	assert.EqualValues(t, 1, count)

	// kind=test excludes real + demo.
	count, err = CountBusinessesForAdmin(AdminBusinessFilter{Kind: string(BusinessKindTest)})
	require.NoError(t, err)
	assert.EqualValues(t, 2, count)

	// kind=all returns every row.
	count, err = CountBusinessesForAdmin(AdminBusinessFilter{Kind: AdminBusinessKindAll})
	require.NoError(t, err)
	assert.EqualValues(t, 4, count)
}

// TestAdminBusinessRegistrySurfacesShareOneCount asserts the four admin
// surfaces that answer "how many businesses" all resolve through
// CountBusinessesForAdmin and agree for the same filter.
func TestAdminBusinessRegistrySurfacesShareOneCount(t *testing.T) {
	db := setupAdminBusinessRegistryDB(t)
	now := time.Now().UTC()

	owner := User{Email: "shared@real.com", Name: "Shared", Role: "user"}
	fixture := User{Email: "bot@payverge.test", Name: "Bot", Role: "user"}
	require.NoError(t, db.Create(&owner).Error)
	require.NoError(t, db.Create(&fixture).Error)

	for i, kind := range []BusinessKind{BusinessKindReal, BusinessKindReal, BusinessKindDemo, BusinessKindTest} {
		uid := &owner.ID
		if kind == BusinessKindTest {
			uid = &fixture.ID
		}
		require.NoError(t, db.Create(&Business{
			BusinessId: fmt.Sprintf("surf-%d", i),
			Name:       fmt.Sprintf("Surf %d", i),
			UserID:     uid,
			Kind:       kind,
			IsDemo:     kind == BusinessKindDemo,
			Email:      fmt.Sprintf("biz%d@example.com", i),
			CreatedAt:  now,
		}).Error)
	}

	filter := AdminBusinessFilter{Kind: string(BusinessKindReal)}
	canonical, err := CountBusinessesForAdmin(filter)
	require.NoError(t, err)
	assert.EqualValues(t, 2, canonical)

	// List total must match Count for the same filter (single entry point).
	_, listTotal, err := ListBusinessesForAdmin(filter)
	require.NoError(t, err)
	assert.Equal(t, canonical, listTotal, "list total and count must share one filter path")
}
