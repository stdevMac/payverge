// Context for the logged-out shots (guest menu + storefront). Leave these null
// to auto-resolve the REAL values for whatever business you're logged into:
// background.js fetches the business's `custom_url` and a live `table_code`
// from the API using your session cookie. Set a string here to force a
// specific value and skip resolution. If a value can't be resolved and isn't
// set, that shot is skipped (never silently shot against a fake "demo" page).
//   apiBase: null -> auto-derive from the page origin (dev :8080 / prod api.*).
//            Override only if your API lives somewhere non-standard.
export const defaults = {
  tableCode: null,
  customUrl: null,
  apiBase: null,
};

// Curated marketing shot list. Add a shot by appending one object.
//   name     -> output file slug (payverge-shots/<date>/<name>.png)
//   url      -> path template; {businessId}/{tableCode}/{customUrl} are filled
//   waitFor  -> optional selector to await before capture
//   settleMs -> optional extra settle after ready (default 800ms in content.js)
//   action   -> optional pre-shot DOM step, e.g. { type: "click", selector: "..." }
// Positive "the dashboard actually finished booting" marker. It renders only
// AFTER the auth/business boot gate clears (DashboardLayout), so waiting for it
// (plus the default no-loading-indicator check) avoids capturing the
// "Loading business dashboard…" screen or a blank pre-render.
const DASH = '[data-testid="dashboard-shell"]';

// Complete operator tab set = PRIMARY_TABS + SECONDARY_TABS from
// frontend/src/components/business/sidebar/sidebarConfig.ts ("fiscal" is a
// TabKey but was merged into the accounting tab, so it gets no shot). Keep
// this list in sync when a sidebar tab is added or removed.
export const shots = [
  // --- Operator dashboard (requires an authenticated session with real data) ---
  { name: "overview", url: "/business/{businessId}/dashboard?tab=overview", waitFor: DASH },
  { name: "bills", url: "/business/{businessId}/dashboard?tab=bills", waitFor: DASH },
  { name: "cash-register", url: "/business/{businessId}/dashboard?tab=cash-register", waitFor: DASH, settleMs: 1000 },
  { name: "kitchen", url: "/business/{businessId}/dashboard?tab=kitchen", waitFor: DASH, settleMs: 1000 },
  { name: "reservations", url: "/business/{businessId}/dashboard?tab=reservations", waitFor: DASH },
  { name: "menu", url: "/business/{businessId}/dashboard?tab=menu", waitFor: DASH },
  { name: "tables", url: "/business/{businessId}/dashboard?tab=tables", waitFor: DASH, settleMs: 1000 },
  { name: "ai-waiter", url: "/business/{businessId}/dashboard?tab=ai-waiter", waitFor: DASH, settleMs: 1000 },
  { name: "director-console", url: "/business/{businessId}/dashboard?tab=director-console", waitFor: DASH, settleMs: 1500 },
  { name: "marketing", url: "/business/{businessId}/dashboard?tab=marketing", waitFor: DASH },
  // NOTE: recharts was removed from the frontend (see noRecharts.test.ts);
  // charts are lazy chart.js canvases that only exist on some sub-tabs, so
  // the shell marker + a longer settle is the reliable wait here.
  { name: "analytics", url: "/business/{businessId}/dashboard?tab=analytics", waitFor: DASH, settleMs: 1500 },
  { name: "crm", url: "/business/{businessId}/dashboard?tab=crm", waitFor: DASH },
  { name: "delivery", url: "/business/{businessId}/dashboard?tab=delivery", waitFor: DASH, settleMs: 1000 },
  { name: "counter", url: "/business/{businessId}/dashboard?tab=counter", waitFor: DASH, settleMs: 1000 },
  { name: "inventory", url: "/business/{businessId}/dashboard?tab=inventory", waitFor: DASH },
  { name: "staff", url: "/business/{businessId}/dashboard?tab=staff", waitFor: DASH, settleMs: 1000 },
  { name: "schedule", url: "/business/{businessId}/dashboard?tab=schedule", waitFor: DASH, settleMs: 1500 },
  { name: "business-page", url: "/business/{businessId}/dashboard?tab=business-page", waitFor: DASH, settleMs: 1000 },
  { name: "accounting", url: "/business/{businessId}/dashboard?tab=accounting", waitFor: DASH, settleMs: 1000 },
  { name: "plugins", url: "/business/{businessId}/dashboard?tab=plugins", waitFor: DASH, settleMs: 1000 },
  { name: "settings", url: "/business/{businessId}/dashboard?tab=settings", waitFor: DASH },

  // --- Guest-facing (no login) ---
  { name: "guest-menu", url: "/t/{tableCode}/menu", settleMs: 1000 },

  // --- Public storefront (no login) ---
  { name: "storefront", url: "/b/{customUrl}", settleMs: 1000 },
];
