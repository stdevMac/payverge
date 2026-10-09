// Pick the tab the run should drive. The old behavior ("whatever tab is
// active in the current window") broke whenever the run was triggered from
// anywhere but the popup-over-the-dashboard — e.g. the popup opened as a tab.
// Preference order:
//   1. an ACTIVE tab already on a business dashboard,
//   2. the most recently accessed business-dashboard tab anywhere,
//   3. null (caller reports "open the dashboard first").
const DASHBOARD_URL = /\/business\/[^/]+\/dashboard/;

export function chooseTargetTab(tabs) {
  const candidates = tabs.filter((t) => DASHBOARD_URL.test(t.url || ""));
  if (candidates.length === 0) return null;
  const active = candidates.find((t) => t.active);
  if (active) return active;
  return candidates.reduce((best, t) =>
    (t.lastAccessed ?? 0) > (best.lastAccessed ?? 0) ? t : best,
  );
}
