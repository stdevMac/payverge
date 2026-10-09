package database

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// TestIsBusinessOperational pins the only remaining business lock: an admin
// suspension (is_active=false) or closure (closed_at set). Demo showrooms
// never lock so a long-running walkthrough cannot go dark.
func TestIsBusinessOperational(t *testing.T) {
	closed := time.Now().Add(-time.Hour)

	assert.False(t, IsBusinessOperational(nil))
	assert.True(t, IsBusinessOperational(&Business{IsActive: true}))
	assert.False(t, IsBusinessOperational(&Business{IsActive: false}), "admin-suspended business must lock")
	assert.False(t, IsBusinessOperational(&Business{IsActive: true, ClosedAt: &closed}), "closed business must lock")
	assert.True(t, IsBusinessOperational(&Business{IsActive: false, ClosedAt: &closed, IsDemo: true}),
		"demo showroom must stay operational regardless of admin lifecycle fields")
}

func TestAdminBusinessStatus(t *testing.T) {
	closed := time.Now()
	assert.Equal(t, BusinessStatusActive, AdminBusinessStatus(&Business{IsActive: true}))
	assert.Equal(t, BusinessStatusSuspended, AdminBusinessStatus(&Business{IsActive: false}))
	assert.Equal(t, BusinessStatusClosed, AdminBusinessStatus(&Business{IsActive: false, ClosedAt: &closed}))
	assert.Equal(t, BusinessStatusClosed, AdminBusinessStatus(&Business{IsActive: true, ClosedAt: &closed}))
}
