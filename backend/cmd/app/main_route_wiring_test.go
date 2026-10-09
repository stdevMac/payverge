package main

import (
	"os"
	"strings"
	"testing"
)

func TestBillPaymentRoutesUseBillBusinessAccessGuard(t *testing.T) {
	source, err := os.ReadFile("main.go")
	if err != nil {
		t.Fatalf("read main.go: %v", err)
	}
	sourceText := string(source)

	assertBillPaymentRouteGuarded(t, sourceText, `protectedRoutes.POST("/bills/:bill_id/alternative-payment"`, "paymentHandler.MarkAlternativePayment")
	assertBillPaymentRouteGuarded(t, sourceText, `protectedRoutes.GET("/bills/:bill_id/pending-alternative-payments"`, "paymentHandler.GetPendingAlternativePayments")
	assertBillPaymentRouteGuarded(t, sourceText, `protectedRoutes.POST("/bills/:bill_id/pending-alternative-payments/:request_id/cancel"`, "paymentHandler.CancelPendingAlternativePayment")
	assertBillPaymentRouteGuarded(t, sourceText, `protectedRoutes.POST("/bills/:bill_id/pending-alternative-payments/:request_id/reject"`, "paymentHandler.RejectPendingAlternativePayment")
	assertBillPaymentRouteGuarded(t, sourceText, `protectedRoutes.GET("/bills/:bill_id/payment-breakdown"`, "paymentHandler.GetBillPaymentBreakdown")
}

func TestGuestAlternativePaymentRequestUsesGuestWriteRateLimiter(t *testing.T) {
	source, err := os.ReadFile("main.go")
	if err != nil {
		t.Fatalf("read main.go: %v", err)
	}
	const want = `publicRoutes.POST("/guest/bill/:bill_token/request-alternative-payment", guestOrderRateLimiter.RateLimit(), paymentHandler.RequestAlternativePayment)`
	if !strings.Contains(string(source), want) {
		t.Fatalf("guest alternative-payment request must use the shared guest write rate limiter")
	}
}

func TestGuestTableLookupUsesReadLimiterNotOrderLimiter(t *testing.T) {
	source, err := os.ReadFile("main.go")
	if err != nil {
		t.Fatalf("read main.go: %v", err)
	}
	src := string(source)
	if !strings.Contains(src, `guestTableReadLimiter := middleware.NewSimpleRateLimiter(context.Background(),
			intEnv("GUEST_TABLE_READ_RATE_LIMIT_REQUESTS_PER_MINUTE", middleware.GuestTableReadRequestsPerMinute))`) {
		t.Fatalf("guest table GET lookup must use the shared read limiter (middleware.GuestTableReadRequestsPerMinute) distinct from guest order writes")
	}
	required := []string{
		`publicRoutes.GET("/guest/table/:code", guestTableReadLimiter.RateLimit(), server.GetTableByCodePublic)`,
		`publicRoutes.GET("/guest/table/:code/bill", guestTableReadLimiter.RateLimit(), server.GetOpenBillByTableCode)`,
	}
	for _, want := range required {
		if !strings.Contains(src, want) {
			t.Fatalf("guest table lookup wiring missing %s", want)
		}
	}
	if strings.Contains(src, `publicRoutes.GET("/guest/table/:code/bill", guestOrderRateLimiter.RateLimit()`) {
		t.Fatalf("GET bill lookup must not share the POST guest-order limiter")
	}
}

func TestCashRegisterRoutesAreWired(t *testing.T) {
	source, err := os.ReadFile("main.go")
	if err != nil {
		t.Fatalf("read main.go: %v", err)
	}
	sourceText := string(source)

	required := []string{
		`cashRegisterRoutes := protectedRoutes.Group("/businesses/:id/cash-register")`,
		`cashRegisterRoutes.GET("/current",`,
		`cashRegisterRoutes.POST("/sessions",`,
		`cashRegisterRoutes.GET("/sessions",`,
		`cashRegisterRoutes.GET("/sessions/:sessionId",`,
		`cashRegisterRoutes.POST("/sessions/:sessionId/movements",`,
		`cashRegisterRoutes.POST("/sessions/:sessionId/close",`,
		`cashRegisterRoutes.GET("/unassigned",`,
		`server.RoleBasedAccessMiddleware(string(server.PermCashRegisterRead))`,
		`server.RoleBasedAccessMiddleware(string(server.PermCashRegisterOperate))`,
		`handlers.NewCashRegisterHandler(database.GetDB())`,
	}
	for _, want := range required {
		if !strings.Contains(sourceText, want) {
			t.Fatalf("cash register route wiring is missing %s", want)
		}
	}
}

func TestBrowserPrintLeaseRoutesRequirePrintPermission(t *testing.T) {
	source, err := os.ReadFile("main.go")
	if err != nil {
		t.Fatalf("read main.go: %v", err)
	}
	sourceText := string(source)

	for _, route := range []struct {
		start   string
		handler string
	}{
		{`protectedRoutes.POST("/businesses/:id/print/jobs/claim"`, "printJobHandlers.ClaimBrowserJob"},
		{`protectedRoutes.POST("/businesses/:id/print/jobs/:jobId/renew"`, "printJobHandlers.RenewBrowserLease"},
	} {
		start := strings.Index(sourceText, route.start)
		if start == -1 {
			t.Fatalf("browser print route is missing: %s", route.start)
		}
		handlerOffset := strings.Index(sourceText[start:], route.handler)
		if handlerOffset == -1 {
			t.Fatalf("browser print route %s does not call %s", route.start, route.handler)
		}
		block := sourceText[start : start+handlerOffset+len(route.handler)]
		if !strings.Contains(block, `server.RoleBasedAnyAccessMiddleware(printAgentPermissions...)`) {
			t.Fatalf("browser print route %s must require print-agent permissions, not read-only printer access", route.start)
		}
		if strings.Contains(block, `server.RoleBasedAccessMiddleware("printers:read")`) {
			t.Fatalf("browser print route %s must not be claimable with read-only printer access", route.start)
		}
	}
}

func TestMarketingRouteWiringUsesMarketingPermissions(t *testing.T) {
	source, err := os.ReadFile("main.go")
	if err != nil {
		t.Fatalf("read main.go: %v", err)
	}
	sourceText := string(source)

	required := []string{
		`aiMenuRoutes.POST("/marketing/image", server.RoleBasedAccessMiddleware("marketing:write"), server.GenerateMarketingImage)`,
		`aiMenuRoutes.POST("/marketing/caption", server.RoleBasedAccessMiddleware("marketing:write"), server.GenerateMarketingCaption)`,
		`aiMenuRoutes.POST("/marketing/activity", server.RoleBasedAccessMiddleware("marketing:write"), server.RecordMarketingActivityHandler)`,
		`aiMenuRoutes.PUT("/marketing/settings", server.RoleBasedAccessMiddleware("marketing:write"), server.PutMarketingSettingsHandler)`,
	}
	for _, want := range required {
		if !strings.Contains(sourceText, want) {
			t.Fatalf("marketing route wiring is missing %s", want)
		}
	}
}

func assertBillPaymentRouteGuarded(t *testing.T, source, routeStart, handler string) {
	t.Helper()

	start := strings.Index(source, routeStart)
	if start == -1 {
		t.Fatalf("protected bill payment route is missing: %s", routeStart)
	}
	handlerOffset := strings.Index(source[start:], handler)
	if handlerOffset == -1 {
		t.Fatalf("protected bill payment route %s does not call %s", routeStart, handler)
	}
	routeBlock := source[start : start+handlerOffset+len(handler)]
	if !strings.Contains(routeBlock, "server.RequireBillBusinessAccess()") {
		t.Fatalf("protected bill payment route is missing RequireBillBusinessAccess guard: %s", routeStart)
	}
}
