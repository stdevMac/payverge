package services

// onboarding_stamp.go — server-side onboarding completion anchor.
//
// Before this, the ONLY writer of onboarding_completed_at was the
// OnboardingHub client auto-POST — never fired for staff-only dashboards or
// operators who don't revisit the Overview tab. The two mutations that can
// flip RequiredDone (first table create, first non-empty menu write) also stamp
// it server-side through the same one-shot NULL→now conditional update used by
// the CompleteOnboarding handler.

import (
	"time"

	"github.com/stdevmac/payverge/backend/internal/database"
)

// StampOnboardingCompletedIfReady stamps onboarding_completed_at when the
// required setup steps are done and nothing has stamped it yet. It returns
// whether this call won the stamp.
// Cheap-path first: one narrow SELECT bails before the 6-query setup-status
// computation whenever the stamp already exists (i.e. every call after the
// first). Callers on request paths should invoke it via logger.SafeGo.
func StampOnboardingCompletedIfReady(businessID uint) (bool, error) {
	db := database.GetDB()

	var row struct{ OnboardingCompletedAt *time.Time }
	if err := db.Model(&database.Business{}).
		Select("onboarding_completed_at").
		Where("id = ?", businessID).
		First(&row).Error; err != nil {
		return false, err
	}
	if row.OnboardingCompletedAt != nil {
		return false, nil
	}

	status, err := ComputeSetupStatus(businessID)
	if err != nil {
		return false, err
	}
	if !status.RequiredDone {
		return false, nil
	}

	res := db.Model(&database.Business{}).
		Where("id = ? AND onboarding_completed_at IS NULL", businessID).
		Update("onboarding_completed_at", time.Now().UTC())
	if res.Error != nil {
		return false, res.Error
	}
	if res.RowsAffected == 0 {
		return false, nil // lost the race to another writer — its event fired
	}

	return true, nil
}
