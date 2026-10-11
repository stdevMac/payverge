package demo

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"

	"github.com/stdevmac/payverge/backend/internal/database"
)

type Summary struct {
	Instance     *database.DemoInstance `json:"instance"`
	Businesses   []database.Business    `json:"businesses"`
	Access       []AccessIdentity       `json:"access"`
	Runs         []database.DemoRun     `json:"runs"`
	Verification VerificationResult     `json:"verification"`
	// Heartbeat surfaces admin_demo_append liveness (Task 17). Nil only when
	// the instance itself is missing; otherwise always populated.
	Heartbeat *Heartbeat `json:"heartbeat,omitempty"`
}

type AccessIdentity struct {
	BusinessID   uint   `json:"business_id"`
	BusinessName string `json:"business_name"`
	Role         string `json:"role"`
	Name         string `json:"name"`
	Email        string `json:"email"`
	LoginPath    string `json:"login_path"`
	PINHint      string `json:"pin_hint,omitempty"`
}

func (s *Service) AppendDueDaysForAdmin(ctx context.Context, adminUserID uint) error {
	if err := s.assertAdmin(ctx, adminUserID); err != nil {
		return err
	}
	var instance database.DemoInstance
	err := s.db.WithContext(ctx).Where("admin_user_id = ?", adminUserID).First(&instance).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		_, err := s.EnsureForAdmin(ctx, adminUserID)
		return err
	}
	if err != nil {
		return err
	}
	return s.appendDueDaysForInstance(ctx, instance.ID)
}

func (s *Service) SummaryForAdmin(ctx context.Context, adminUserID uint, ensure bool) (*Summary, error) {
	var instance *database.DemoInstance
	if ensure {
		ensured, err := s.EnsureForAdmin(ctx, adminUserID)
		if err != nil {
			// Ensure persisted status=failed + last_error on the instance.
			// Degrade to the existing instance instead of an opaque 500 so
			// the admin surface can show WHY the reseed is failing — this
			// exact failure mode shipped a stale showroom for a full deploy
			// cycle while the only evidence was one startup log line.
			var existing database.DemoInstance
			if lookupErr := s.db.WithContext(ctx).Where("admin_user_id = ?", adminUserID).First(&existing).Error; lookupErr != nil {
				return nil, err
			}
			instance = &existing
		} else {
			instance = ensured
		}
	} else {
		if err := s.assertAdmin(ctx, adminUserID); err != nil {
			return nil, err
		}
		var existing database.DemoInstance
		if err := s.db.WithContext(ctx).Where("admin_user_id = ?", adminUserID).First(&existing).Error; err != nil {
			return nil, err
		}
		instance = &existing
	}

	businessIDs := compactBusinessIDs(instance.PrimaryBusinessID, instance.SecondaryBusinessID)
	businesses := make([]database.Business, 0, len(businessIDs))
	if len(businessIDs) > 0 {
		if err := s.db.WithContext(ctx).Where("id IN ?", businessIDs).Order("id ASC").Find(&businesses).Error; err != nil {
			return nil, err
		}
	}
	access, err := s.accessIdentities(ctx, businesses)
	if err != nil {
		return nil, err
	}

	runs := []database.DemoRun{}
	if err := s.db.WithContext(ctx).
		Where("admin_user_id = ?", adminUserID).
		Order("started_at DESC").
		Limit(10).
		Find(&runs).Error; err != nil {
		return nil, err
	}

	verification, err := s.VerifyInstance(ctx, instance.ID)
	if err != nil {
		return nil, err
	}

	heartbeat := s.computeHeartbeat(ctx, instance, runs)

	return &Summary{
		Instance:     instance,
		Businesses:   businesses,
		Access:       access,
		Runs:         runs,
		Verification: verification,
		Heartbeat:    heartbeat,
	}, nil
}

// computeHeartbeat derives append liveness from the latest successful ensure /
// append_day run. Stale when last success is older than 2× DefaultAppendInterval.
func (s *Service) computeHeartbeat(ctx context.Context, instance *database.DemoInstance, runs []database.DemoRun) *Heartbeat {
	interval := DefaultAppendInterval
	staleAfter := 2 * interval
	hb := &Heartbeat{
		Status:             HeartbeatUnknown,
		AppendIntervalSecs: int64(interval.Seconds()),
		StaleAfterSecs:     int64(staleAfter.Seconds()),
	}
	if instance == nil {
		return hb
	}
	// Prefer finished_at of the newest succeeded ensure/append run.
	var last *time.Time
	for i := range runs {
		run := runs[i]
		if run.Status != database.DemoRunStatusSucceeded {
			continue
		}
		if run.RunType != database.DemoRunTypeEnsure && run.RunType != database.DemoRunTypeAppendDay {
			continue
		}
		var at time.Time
		if run.FinishedAt != nil {
			at = run.FinishedAt.UTC()
		} else {
			at = run.StartedAt.UTC()
		}
		if last == nil || at.After(*last) {
			t := at
			last = &t
		}
	}
	// Fallback: instance.UpdatedAt when ready (covers legacy rows without runs).
	if last == nil && instance.Status == database.DemoInstanceStatusReady && !instance.UpdatedAt.IsZero() {
		t := instance.UpdatedAt.UTC()
		last = &t
	}
	hb.LastAppendAt = last
	if last == nil {
		hb.Status = HeartbeatUnknown
		return hb
	}
	age := s.now().UTC().Sub(*last)
	if age > staleAfter {
		hb.Status = HeartbeatStale
	} else {
		hb.Status = HeartbeatFresh
	}
	return hb
}

func (s *Service) accessIdentities(ctx context.Context, businesses []database.Business) ([]AccessIdentity, error) {
	if len(businesses) == 0 {
		return []AccessIdentity{}, nil
	}
	businessIDs := make([]uint, 0, len(businesses))
	businessNames := make(map[uint]string, len(businesses))
	for _, business := range businesses {
		businessIDs = append(businessIDs, business.ID)
		businessNames[business.ID] = business.Name
	}
	var staff []database.Staff
	if err := s.db.WithContext(ctx).
		Where("business_id IN ? AND is_active = ?", businessIDs, true).
		Order("business_id ASC, role ASC, id ASC").
		Find(&staff).Error; err != nil {
		return nil, err
	}
	// Surface the same per-staff PIN the seeder hashed — derived from email so
	// we never store plaintext, and pairwise-distinct within each business.
	emailsByBiz := map[uint][]string{}
	for _, st := range staff {
		emailsByBiz[st.BusinessID] = append(emailsByBiz[st.BusinessID], st.Email)
	}
	pinsByBiz := map[uint]map[string]string{}
	for bizID, emails := range emailsByBiz {
		pinsByBiz[bizID] = assignDemoStaffPINs(emails)
	}
	out := make([]AccessIdentity, 0, len(staff))
	for _, st := range staff {
		identity := AccessIdentity{
			BusinessID:   st.BusinessID,
			BusinessName: businessNames[st.BusinessID],
			Role:         string(st.Role),
			Name:         st.Name,
			Email:        st.Email,
			LoginPath:    "/staff/login",
		}
		if st.PinSetAt != nil {
			pin, ok := pinsByBiz[st.BusinessID][st.Email]
			if !ok {
				pin = demoStaffPIN(st.Email)
			}
			// Only surface a hint that still opens the staff login: the PIN
			// key is per instance (M-pin) and, without PLUGIN_SECRET_KEY, per
			// process, so a hint re-derived after a key change would be wrong.
			if demoPINMatchesHash(st.PinHash, pin) {
				identity.PINHint = pin
			}
		}
		out = append(out, identity)
	}
	return out, nil
}
