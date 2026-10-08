package middleware

// Guest-facing public route budgets, per client IP per rolling minute.
//
// These are the single source of truth for the two shared guest limiters wired
// in cmd/app/main.go (guestTableReadLimiter / guestOrderRateLimiter). They are
// named constants (not inline literals) so the behavioral regression tests in
// cmd/app can replay a realistic dining session against the exact production
// budget and fail loudly when a rebalance breaks either direction:
// legitimate dining must pass, abuse must still be limited.
const (
	// GuestTableReadRequestsPerMinute is the shared per-IP budget for ALL
	// public guest read routes: /table/:code, /guest/table/:code{,/bill,
	// /business,/menu,/status,/events,/service-call,/loyalty-rate},
	// /currencies, /languages and the /business/:customUrl storefront reads.
	//
	// Issue #814: one SimpleRateLimiter instance serves every route above, so
	// the budget must cover a whole table's worth of guests behind a venue
	// NAT (restaurant Wi-Fi presents one public IP for every phone at the
	// table). A single guest session's worst minute is ~55 reads (page
	// navigations at ~7 reads each, menu/status refreshes, SSE reconnects,
	// bill recovery polls); a party of four sharing the NAT quadruples that.
	// The previous 60/min budget rate-limited a lone diner mid-browse and
	// blocked GET /guest/table/:code/bill — payment of an open check — with a
	// 429 + 59s retry. 240/min fixes that reported flow.
	//
	// Residual ceiling: the untouched GLOBAL production limiter (300/min per
	// IP, main.go GLOBAL_RATE_LIMIT_REQUESTS_PER_MINUTE) binds just above the
	// combined 240 read + 120 write guest budget, so a very busy multi-diner
	// table behind one NAT IP can still see 429s — from the global layer, not
	// this one. Raising the global limiter is a security-scoped decision that
	// is deliberately out of this rebalance's scope; see the batch27
	// adversarial review notes for issue #814.
	GuestTableReadRequestsPerMinute = 240

	// GuestOrderWriteRequestsPerMinute is the shared per-IP budget for
	// unauthenticated guest create/pay routes (order, bill create, service
	// call, split holds/execute, crypto quote/payment, email receipt, fiscal
	// customer/receipt). Writes are user-initiated taps, not background
	// polling, so this stays far tighter than the read budget.
	GuestOrderWriteRequestsPerMinute = 120
)
