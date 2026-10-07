package database

import (
	"errors"
	"strings"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Virtual-channel ref-key prefixes. Role and dept channels both use the `role`
// channel type (no new enum value); the prefix on RefKey is what distinguishes a
// per-role channel ("role:server") from a per-department channel ("dept:BOH").
const (
	chatRefKeyRolePrefix = "role:"
	chatRefKeyDeptPrefix = "dept:"

	// chatChannelListMax bounds the per-staff channel fan-out (perf gate).
	chatChannelListMax = 200
)

// ErrChatChannelNotFound is returned when a channel does not exist within the
// caller's business (tenant-scoped lookups).
var ErrChatChannelNotFound = errors.New("chat channel not found")

// GetOrCreateVirtualChannel idempotently resolves the backing chat_channels row
// for a virtual (role/dept) channel keyed by its partial-unique ref_key. Concurrent
// callers converge on a single row: the INSERT rides ON CONFLICT DO NOTHING against
// idx_chat_channels_refkey, then — when the insert was a no-op (the row already
// existed) — we re-select by (business_id, ref_key). Dept channels reuse the `role`
// channel type; only the ref_key prefix differs. Returns the canonical row so two
// calls for the same ref_key always yield the same channel id.
func (d *DB) GetOrCreateVirtualChannel(businessID uint, refKey, name string) (*ChatChannel, error) {
	if businessID == 0 || refKey == "" {
		return nil, ErrChatChannelNotFound
	}
	g := d.GetGorm()
	ch := ChatChannel{BusinessID: businessID, Type: ChatChannelTypeRole, Name: name, RefKey: refKey}
	res := g.Clauses(clause.OnConflict{DoNothing: true}).Create(&ch)
	if res.Error != nil {
		return nil, res.Error
	}
	if res.RowsAffected == 1 && ch.ID != 0 {
		return &ch, nil // fresh insert won the race
	}
	// Conflict (row pre-existed): re-select the canonical row by its unique ref_key.
	var out ChatChannel
	if err := g.Where("business_id = ? AND ref_key = ?", businessID, refKey).First(&out).Error; err != nil {
		return nil, err
	}
	return &out, nil
}

// virtualChannelSpec is one desired virtual (role/dept) channel: its unique
// ref_key plus the display name to use when the row must be created.
type virtualChannelSpec struct {
	refKey string
	name   string
}

// resolveVirtualChannels batch-resolves the given virtual channels, creating any
// that don't yet exist, and returns the canonical rows in the SAME order as
// specs. In the steady state (all channels already materialized) it issues a
// single SELECT ... WHERE ref_key IN (...) and ZERO INSERTs — replacing the old
// per-channel INSERT..ON CONFLICT + SELECT loop that ran on every dashboard read
// and burned the id sequence. specs are expected to be distinct by ref_key.
func (d *DB) resolveVirtualChannels(businessID uint, specs []virtualChannelSpec) ([]ChatChannel, error) {
	if len(specs) == 0 {
		return nil, nil
	}
	g := d.GetGorm()

	refKeys := make([]string, len(specs))
	for i, s := range specs {
		refKeys[i] = s.refKey
	}

	// One read to find everything that already exists.
	var existing []ChatChannel
	if err := g.Where("business_id = ? AND ref_key IN ?", businessID, refKeys).Find(&existing).Error; err != nil {
		return nil, err
	}
	byRef := make(map[string]ChatChannel, len(specs))
	for _, ch := range existing {
		byRef[ch.RefKey] = ch
	}

	// Insert only the missing rows, batched. ON CONFLICT DO NOTHING keeps this
	// safe against a concurrent creator racing the same ref_key.
	var toCreate []ChatChannel
	for _, s := range specs {
		if _, ok := byRef[s.refKey]; !ok {
			toCreate = append(toCreate, ChatChannel{
				BusinessID: businessID, Type: ChatChannelTypeRole, Name: s.name, RefKey: s.refKey,
			})
		}
	}
	if len(toCreate) > 0 {
		if err := g.Clauses(clause.OnConflict{DoNothing: true}).Create(&toCreate).Error; err != nil {
			return nil, err
		}
		// Bulk ON CONFLICT DO NOTHING does not reliably backfill ids for rows that
		// conflicted, so re-select the just-created (or concurrently-created) rows.
		missing := make([]string, len(toCreate))
		for i, ch := range toCreate {
			missing[i] = ch.RefKey
		}
		var created []ChatChannel
		if err := g.Where("business_id = ? AND ref_key IN ?", businessID, missing).Find(&created).Error; err != nil {
			return nil, err
		}
		for _, ch := range created {
			byRef[ch.RefKey] = ch
		}
	}

	// Assemble in the requested order.
	out := make([]ChatChannel, 0, len(specs))
	seen := make(map[string]bool, len(specs))
	for _, s := range specs {
		if seen[s.refKey] {
			continue
		}
		if ch, ok := byRef[s.refKey]; ok {
			out = append(out, ch)
			seen[s.refKey] = true
		}
	}
	return out, nil
}

// GetChannel loads one channel tenant-scoped, or ErrChatChannelNotFound.
func (d *DB) GetChannel(businessID, channelID uint) (*ChatChannel, error) {
	var ch ChatChannel
	if err := d.GetGorm().
		Where("id = ? AND business_id = ?", channelID, businessID).
		First(&ch).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrChatChannelNotFound
		}
		return nil, err
	}
	return &ch, nil
}

// ListChannelsForStaff returns the channels a staff member belongs to, lazily
// materializing the virtual ones: their own role channel ("role:<role>"), one
// dept channel per department in which they hold an active StaffPosition→Position
// ("dept:<dept>"), PLUS their manual DM/group channels (resolved via explicit
// ChatChannelMember rows). It never returns another staff member's DMs — manual
// membership is the only path to a direct/group channel.
func (d *DB) ListChannelsForStaff(businessID, staffID uint, role string) ([]ChatChannel, error) {
	g := d.GetGorm()
	out := make([]ChatChannel, 0, 8)

	// 1+2. The staff member's own role channel plus one dept channel per department
	// they hold an active position in — resolved in a single batched read (order:
	// role first, then departments ascending).
	specs := make([]virtualChannelSpec, 0, 8)
	if role != "" {
		specs = append(specs, virtualChannelSpec{chatRefKeyRolePrefix + role, roleDisplayName(StaffRole(role))})
	}
	var depts []string
	if err := g.Table("staff_positions AS sp").
		Joins("JOIN positions ON positions.id = sp.position_id AND positions.business_id = sp.business_id").
		Where("sp.business_id = ? AND sp.staff_id = ? AND positions.is_active = ? AND positions.department <> ''", businessID, staffID, true).
		Distinct().
		Order("positions.department ASC").
		Limit(chatChannelListMax).
		Pluck("positions.department", &depts).Error; err != nil {
		return nil, err
	}
	for _, dept := range depts {
		specs = append(specs, virtualChannelSpec{chatRefKeyDeptPrefix + dept, dept})
	}
	virtual, err := d.resolveVirtualChannels(businessID, specs)
	if err != nil {
		return nil, err
	}
	out = append(out, virtual...)

	// 3. Manual DM/group channels via explicit membership rows (narrow projection).
	var manual []ChatChannel
	if err := g.Table("chat_channels AS c").
		Select("c.id, c.business_id, c.type, c.name, c.ref_key, c.is_archived, c.created_by_staff_id, c.created_at, c.updated_at").
		Joins("JOIN chat_channel_members m ON m.channel_id = c.id AND m.business_id = c.business_id").
		Where("c.business_id = ? AND m.staff_id = ? AND c.is_archived = ?", businessID, staffID, false).
		Order("c.updated_at DESC").
		Limit(chatChannelListMax).
		Scan(&manual).Error; err != nil {
		return nil, err
	}
	out = append(out, manual...)
	return out, nil
}

// ListChannelsForOperator returns the communication surface for an owner
// (staffID 0) or manager: one role channel per DISTINCT ACTIVE staff role, one
// dept channel per distinct active-position department (both lazily
// materialized), plus the caller's OWN manual DM/group channels. It never
// returns anyone else's DMs — explicit membership stays the only path there.
func (d *DB) ListChannelsForOperator(businessID, staffID uint) ([]ChatChannel, error) {
	g := d.GetGorm()
	out := make([]ChatChannel, 0, 8)

	// 1+2. One role channel per distinct active staff role, then one dept channel
	// per distinct active-position department — resolved in a single batched read
	// (order: roles ascending, then departments ascending) to avoid a per-channel
	// INSERT..ON CONFLICT on every operator dashboard poll.
	var roles []string
	if err := g.Model(&Staff{}).
		Where("business_id = ? AND is_active = ? AND role <> ''", businessID, true).
		Distinct().
		Order("role ASC").
		Limit(chatChannelListMax).
		Pluck("role", &roles).Error; err != nil {
		return nil, err
	}
	var depts []string
	if err := g.Model(&Position{}).
		Where("business_id = ? AND is_active = ? AND department <> ''", businessID, true).
		Distinct().
		Order("department ASC").
		Limit(chatChannelListMax).
		Pluck("department", &depts).Error; err != nil {
		return nil, err
	}
	specs := make([]virtualChannelSpec, 0, len(roles)+len(depts))
	for _, role := range roles {
		specs = append(specs, virtualChannelSpec{chatRefKeyRolePrefix + role, roleDisplayName(StaffRole(role))})
	}
	for _, dept := range depts {
		specs = append(specs, virtualChannelSpec{chatRefKeyDeptPrefix + dept, dept})
	}
	virtual, err := d.resolveVirtualChannels(businessID, specs)
	if err != nil {
		return nil, err
	}
	out = append(out, virtual...)

	// 3. The caller's own manual DM/group channels (owners, staffID 0, have none).
	if staffID != 0 {
		var manual []ChatChannel
		if err := g.Table("chat_channels AS c").
			Select("c.id, c.business_id, c.type, c.name, c.ref_key, c.is_archived, c.created_by_staff_id, c.created_at, c.updated_at").
			Joins("JOIN chat_channel_members m ON m.channel_id = c.id AND m.business_id = c.business_id").
			Where("c.business_id = ? AND m.staff_id = ? AND c.is_archived = ?", businessID, staffID, false).
			Order("c.updated_at DESC").
			Limit(chatChannelListMax).
			Scan(&manual).Error; err != nil {
			return nil, err
		}
		out = append(out, manual...)
	}
	return out, nil
}

// CanReadChannel is the central privacy gate. It answers "may this staff member
// read this channel?" by channel type:
//
//   - direct/group → an explicit ChatChannelMember row must link (channel, staff);
//     a non-member is always denied.
//   - role        → the channel's ref_key must equal "role:<staffRole>".
//   - dept        → (role type, "dept:" prefix) the staff must hold an active
//     StaffPosition whose Position.Department matches the ref_key's department.
//   - announcement → audience matching is owned by Stage 3; default-closed here.
//
// A nil channel or a cross-tenant channel is denied. Errors are DB errors only;
// a clean "not allowed" is (false, nil).
func (d *DB) CanReadChannel(businessID uint, channel *ChatChannel, staffID uint, staffRole string) (bool, error) {
	if channel == nil || channel.BusinessID != businessID {
		return false, nil
	}
	switch channel.Type {
	case ChatChannelTypeDirect, ChatChannelTypeGroup:
		var n int64
		err := d.GetGorm().Model(&ChatChannelMember{}).
			Where("channel_id = ? AND staff_id = ? AND business_id = ?", channel.ID, staffID, businessID).
			Limit(1).Count(&n).Error
		return n > 0, err
	case ChatChannelTypeRole:
		// Operator gate: owners (staffID 0 — no staff principal) and managers may
		// read/post in EVERY role and dept channel; the per-role/per-dept match
		// below only constrains regular staff. DMs above are deliberately exempt.
		if staffID == 0 || staffRole == string(StaffRoleManager) {
			return true, nil
		}
		// Role + dept channels share the `role` type; the ref_key prefix decides.
		if strings.HasPrefix(channel.RefKey, chatRefKeyDeptPrefix) {
			dept := strings.TrimPrefix(channel.RefKey, chatRefKeyDeptPrefix)
			if dept == "" {
				return false, nil
			}
			var n int64
			err := d.GetGorm().Table("staff_positions AS sp").
				Joins("JOIN positions ON positions.id = sp.position_id AND positions.business_id = sp.business_id").
				Where("sp.business_id = ? AND sp.staff_id = ? AND positions.is_active = ? AND positions.department = ?",
					businessID, staffID, true, dept).
				Limit(1).Count(&n).Error
			return n > 0, err
		}
		return staffRole != "" && channel.RefKey == chatRefKeyRolePrefix+staffRole, nil
	case ChatChannelTypeAnnouncement:
		// Announcement audience matching is implemented in Stage 3; closed here.
		return false, nil
	default:
		return false, nil
	}
}
