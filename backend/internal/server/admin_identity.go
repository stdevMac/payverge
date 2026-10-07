package server

import (
	"fmt"
	"strconv"
	"strings"
)

// ExtractAdminUserID reads the authenticated admin user id from Gin context.
func ExtractAdminUserID(c interface {
	Get(string) (interface{}, bool)
}) (uint, error) {
	id, exists := c.Get("user_id")
	if !exists {
		return 0, fmt.Errorf("user_id missing from context")
	}
	switch v := id.(type) {
	case float64:
		return uint(v), nil
	case uint:
		return v, nil
	case int:
		if v < 0 {
			return 0, fmt.Errorf("user_id is negative: %d", v)
		}
		return uint(v), nil
	case uint64:
		return uint(v), nil
	case int64:
		if v < 0 {
			return 0, fmt.Errorf("user_id is negative: %d", v)
		}
		return uint(v), nil
	case string:
		trimmed := strings.TrimSpace(v)
		if trimmed == "" {
			return 0, fmt.Errorf("user_id is empty string")
		}
		parsed, err := strconv.ParseUint(trimmed, 10, 64)
		if err != nil {
			return 0, fmt.Errorf("user_id string not numeric: %q", v)
		}
		return uint(parsed), nil
	default:
		return 0, fmt.Errorf("unexpected user_id type %T", id)
	}
}
