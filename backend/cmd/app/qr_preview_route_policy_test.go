package main

import (
	"os"
	"regexp"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestQRPreviewMilestoneRouteIsPermissionGuarded(t *testing.T) {
	source, err := os.ReadFile("main.go")
	require.NoError(t, err)

	route := regexp.MustCompile(`protectedRoutes\.POST\("/businesses/:id/onboarding/qr-preview"[^\n]+`).
		Find(source)
	require.NotEmpty(t, route, "QR-preview milestone route must be mounted")
	require.Contains(t, string(route), `RoleBasedAccessMiddleware("tables:read")`)
	require.Contains(t, string(route), "server.MarkQRPreviewed")
}
