package database

// StaffNamesByIDs resolves a set of staff ids to their display names in ONE
// bounded, tenant-scoped query (id, name WHERE business_id AND id IN). The
// manager live-floor board uses it to label rows without a per-row lookup (no
// N+1). Ids that don't resolve (cross-tenant / deleted) are simply absent from
// the returned map. No money fields.
func (d *DB) StaffNamesByIDs(businessID uint, staffIDs []uint) (map[uint]string, error) {
	out := map[uint]string{}
	if len(staffIDs) == 0 {
		return out, nil
	}
	var rows []struct {
		ID   uint
		Name string
	}
	if err := d.GetGorm().
		Table("staff").
		Select("id", "name").
		Where("business_id = ? AND id IN ?", businessID, staffIDs).
		Scan(&rows).Error; err != nil {
		return nil, err
	}
	for _, r := range rows {
		out[r.ID] = r.Name
	}
	return out, nil
}
