package database

import (
	"context"
	"strings"
)

// StoreErrorLog stores an error log in the database.
//
// Defends against the PostgreSQL jsonb column rejecting empty strings:
// the Metadata column is declared `type:jsonb` so an unset field would
// otherwise hit `pq: invalid input syntax for type json` and fail the
// insert. We coerce empty Metadata to "{}" so the row always inserts.
func StoreErrorLog(ctx context.Context, log *ErrorLog) error {
	if log != nil && log.Metadata == "" {
		log.Metadata = "{}"
	}
	result := db.WithContext(ctx).Create(log)
	return result.Error
}

// GetErrorLogsPaginated retrieves error logs with pagination and optional filters.
// excludeSource drops rows with that source. When the caller filters for the
// excluded source itself, the result is an empty list. Rows with a NULL or
// blank source count as the excluded source (default untrusted).
func GetErrorLogsPaginated(ctx context.Context, limit, offset int, source, component, excludeSource string) ([]*ErrorLog, int64, error) {
	if excludeSource != "" && strings.EqualFold(strings.TrimSpace(source), strings.TrimSpace(excludeSource)) {
		return []*ErrorLog{}, 0, nil
	}

	query := db.WithContext(ctx).Model(&ErrorLog{}).Order("created_at DESC")

	if source != "" {
		query = query.Where("source = ?", source)
	}
	if excludeSource != "" {
		// A row with no source (NULL/blank, e.g. written before the source
		// column was stamped) is treated as the excluded, untrusted source.
		// Case and padding variants of the excluded source match too.
		query = query.Where("LOWER(TRIM(COALESCE(NULLIF(TRIM(source), ''), ?))) <> ?",
			excludeSource, strings.ToLower(strings.TrimSpace(excludeSource)))
	}
	if component != "" {
		query = query.Where("component = ?", component)
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	var logs []*ErrorLog
	if err := query.Limit(limit).Offset(offset).Find(&logs).Error; err != nil {
		return nil, 0, err
	}

	return logs, total, nil
}
