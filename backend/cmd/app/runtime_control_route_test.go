package main

import (
	"os"
	"strings"
	"testing"
)

func TestRuntimeControlRoutesStayBehindAdminRecoveryAuthentication(t *testing.T) {
	raw, err := os.ReadFile("main.go")
	if err != nil {
		t.Fatal(err)
	}
	source := string(raw)
	admin := strings.Index(source, `adminRoutes := r.Group("/api/v1/admin")`)
	auth := strings.Index(source[admin:], `server.AuthenticationAdminMiddleware()`)
	routes := strings.Index(source[admin:], `adminRoutes.GET("/runtime-controls"`)
	if admin < 0 || auth < 0 || routes < 0 || auth > routes {
		t.Fatal("runtime-control routes must remain inside the authenticated admin group")
	}
	for _, want := range []string{
		`r.Use(runtimecontrol.Middleware(runtimeControlService))`,
		`authHandler.SetRegistrationAdmission(runtimeControlService)`,
		`server.SetLaunchIdentityAdmission(func(`,
		`adminRoutes.PUT("/runtime-controls/:key"`,
		`adminRoutes.POST("/runtime-controls/invite-batches"`,
		`adminRoutes.GET("/runtime-controls/audit"`,
	} {
		if !strings.Contains(source, want) {
			t.Fatalf("missing runtime-control route policy: %s", want)
		}
	}
}
