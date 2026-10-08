package server

import "github.com/gin-gonic/gin"

// callerHasPermission checks a single permission using ResolveContextPermissions.
func callerHasPermission(c *gin.Context, perm string) bool {
	perms, all := ResolveContextPermissions(c)
	if all {
		return true
	}
	for _, p := range perms {
		if p == perm {
			return true
		}
	}
	return false
}
