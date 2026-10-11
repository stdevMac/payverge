package utils

import "github.com/gin-gonic/gin"

// BusinessIdentifierFromParam reads a route parameter that may carry either a
// numeric business database ID or the public business_id slug.
func BusinessIdentifierFromParam(c *gin.Context, paramName string) string {
	return c.Param(paramName)
}
