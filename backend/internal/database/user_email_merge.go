package database

import (
	"strings"
)

// EmailDuplicateGroup is a set of live user accounts that share a normalized
// mailbox. The admin surface must show these as ONE identity with a merge
// affordance rather than two independent rows (Task 22 / Claudia duplicates).
type EmailDuplicateGroup struct {
	NormalizedEmail string `json:"normalized_email"`
	UserIDs         []uint `json:"user_ids"`
	PrimaryUserID   uint   `json:"primary_user_id"`
	CanMerge        bool   `json:"can_merge"`
}

// ListEmailDuplicateGroups returns every lower(email) group that has more than
// one non-deleted user row. PrimaryUserID is the oldest account (lowest id) —
// the natural survivor of a merge.
func ListEmailDuplicateGroups() ([]EmailDuplicateGroup, error) {
	db := GetDB()
	if db == nil {
		return nil, gormErrNilDB()
	}

	type row struct {
		ID    uint
		Email string
	}
	var rows []row
	// Soft-deleted accounts are out of scope for merge; empty emails (wallet
	// placeholders) never form a merge group.
	if err := db.Raw(`
		SELECT id, email FROM users
		WHERE email IS NOT NULL AND TRIM(email) <> ''
		  AND deleted_at IS NULL
		ORDER BY id ASC
	`).Scan(&rows).Error; err != nil {
		return nil, err
	}

	byNorm := make(map[string][]uint)
	for _, r := range rows {
		norm := strings.ToLower(strings.TrimSpace(r.Email))
		if norm == "" {
			continue
		}
		byNorm[norm] = append(byNorm[norm], r.ID)
	}

	out := make([]EmailDuplicateGroup, 0)
	for norm, ids := range byNorm {
		if len(ids) < 2 {
			continue
		}
		out = append(out, EmailDuplicateGroup{
			NormalizedEmail: norm,
			UserIDs:         ids,
			PrimaryUserID:   ids[0], // oldest (ORDER BY id ASC)
			CanMerge:        true,
		})
	}
	return out, nil
}

// gormErrNilDB is a tiny helper so this file doesn't pull fmt solely for one error.
func gormErrNilDB() error {
	return errDBNotInitialized
}

// errDBNotInitialized is returned when GetDB() is nil (test/boot hazard).
var errDBNotInitialized = errString("database: not initialized")

type errString string

func (e errString) Error() string { return string(e) }
