package structs

// Role defines user access levels in the system
type Role string

const (
	RoleAdmin Role = "admin" // Administrator role with full access
	RoleUser  Role = "user"  // Standard user role
)
