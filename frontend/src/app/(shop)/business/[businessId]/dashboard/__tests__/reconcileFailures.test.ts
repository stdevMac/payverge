import {
  applyReconcileCycle,
  failedReconcileLeg,
  isRateLimitedReconcileError,
  isReconcileClean,
  nextReconcileFailureCount,
  RECONCILE_LEG_OK,
  reconcileLegAccess,
  shouldShowStaleBanner,
  skippedReconcileLeg,
  STALE_BANNER_THRESHOLD,
} from "../reconcileFailures";

/**
 * One reconcile cycle exactly as the dashboard runs it: each leg's raw
 * rejection reason is classified by the real `isRateLimitedReconcileError`
 * and the counter by the real `applyReconcileCycle`. Nothing here is
 * mocked — a `null` reason means that leg returned 200.
 */
function runReconcileCycle(
  prev: number,
  legs: {
    bills: unknown;
    orders: unknown;
    reservations: unknown;
    crm?: unknown;
    crmProbed?: boolean;
  },
): number {
  const leg = (reason: unknown) =>
    reason === null ? RECONCILE_LEG_OK : failedReconcileLeg(reason);
  const crmProbed = legs.crmProbed !== false;
  return applyReconcileCycle(prev, {
    bills: leg(legs.bills),
    orders: leg(legs.orders),
    reservations: leg(legs.reservations),
    // `null` is a 200. Do not `??` it away — that used to turn a CRM
    // 200 into a hard miss.
    crm: crmProbed
      ? leg(legs.crm === undefined ? {} : legs.crm)
      : skippedReconcileLeg(),
  }).nextCount;
}

const EDGE_502 = { response: { status: 502 } };
const RATE_LIMIT_429 = { response: { status: 429 } };

describe("dashboard live-reconcile failure banner", () => {
  it("heals to zero after every probed source 200s", () => {
    let count = 0;
    count = nextReconcileFailureCount(count, false);
    count = nextReconcileFailureCount(count, false);
    count = nextReconcileFailureCount(count, false);
    expect(count).toBeGreaterThanOrEqual(STALE_BANNER_THRESHOLD);
    expect(shouldShowStaleBanner(count, false)).toBe(true);

    count = nextReconcileFailureCount(count, true);
    expect(count).toBe(0);
    expect(shouldShowStaleBanner(count, false)).toBe(false);
  });

  it("stays hidden while dismissed even if failures continue", () => {
    expect(shouldShowStaleBanner(5, true)).toBe(false);
  });

  it("does not show on a single miss", () => {
    expect(shouldShowStaleBanner(1, false)).toBe(false);
  });

  it("does not treat a mixed 502+200 cycle as clean (#623)", () => {
    expect(
      isReconcileClean({
        billsOk: true,
        ordersOk: false,
        reservationsOk: false,
      }),
    ).toBe(false);
    expect(
      isReconcileClean({
        billsOk: false,
        ordersOk: true,
        reservationsOk: true,
        crmOk: true,
      }),
    ).toBe(false);
    expect(
      isReconcileClean({
        billsOk: true,
        ordersOk: true,
        reservationsOk: true,
      }),
    ).toBe(true);
    expect(
      isReconcileClean({
        billsOk: true,
        ordersOk: true,
        reservationsOk: true,
        crmOk: false,
      }),
    ).toBe(false);
  });

  it("keeps the banner up when bills 502 and reservations 200 (#623)", () => {
    let count = STALE_BANNER_THRESHOLD;
    expect(shouldShowStaleBanner(count, false)).toBe(true);
    count = runReconcileCycle(count, {
      bills: EDGE_502,
      orders: null,
      reservations: null,
      crm: null,
    });
    expect(count).toBeGreaterThan(STALE_BANNER_THRESHOLD);
    expect(shouldShowStaleBanner(count, false)).toBe(true);
  });

  it("treats axios and typed 429s as rate-limited, not message text (#623)", () => {
    expect(isRateLimitedReconcileError({ response: { status: 429 } })).toBe(
      true,
    );
    expect(isRateLimitedReconcileError({ status: 429 })).toBe(true);
    expect(isRateLimitedReconcileError({ status: "429" })).toBe(true);
    expect(
      isRateLimitedReconcileError({
        name: "RateLimitCooldownError",
        message: "Request is rate limited",
      }),
    ).toBe(true);
    expect(isRateLimitedReconcileError({ response: { status: 500 } })).toBe(
      false,
    );
    expect(isRateLimitedReconcileError(new Error("network"))).toBe(false);
    expect(
      isRateLimitedReconcileError(
        new Error("Request failed with status code 429"),
      ),
    ).toBe(false);
    expect(
      isRateLimitedReconcileError(new Error("upstream rate limited")),
    ).toBe(false);
  });

  it("does not raise the banner on a 429-only storm from clean (#623)", () => {
    let count = 0;
    count = runReconcileCycle(count, {
      bills: RATE_LIMIT_429,
      orders: RATE_LIMIT_429,
      reservations: RATE_LIMIT_429,
      crm: RATE_LIMIT_429,
    });
    count = runReconcileCycle(count, {
      bills: RATE_LIMIT_429,
      orders: RATE_LIMIT_429,
      reservations: RATE_LIMIT_429,
      crm: RATE_LIMIT_429,
    });
    expect(count).toBe(0);
    expect(shouldShowStaleBanner(count, false)).toBe(false);
  });

  it("holds a pinned banner through dinner-rush 429s until every source 200s (#623)", () => {
    let count = 0;
    count = runReconcileCycle(count, {
      bills: EDGE_502,
      orders: EDGE_502,
      reservations: EDGE_502,
      crm: EDGE_502,
    });
    count = runReconcileCycle(count, {
      bills: EDGE_502,
      orders: EDGE_502,
      reservations: EDGE_502,
      crm: EDGE_502,
    });
    expect(shouldShowStaleBanner(count, false)).toBe(true);

    // One 429-only cycle must not walk the strip off (threshold 2).
    count = runReconcileCycle(count, {
      bills: RATE_LIMIT_429,
      orders: RATE_LIMIT_429,
      reservations: RATE_LIMIT_429,
      crm: RATE_LIMIT_429,
    });
    expect(count).toBe(STALE_BANNER_THRESHOLD);
    expect(shouldShowStaleBanner(count, false)).toBe(true);

    count = runReconcileCycle(count, {
      bills: RATE_LIMIT_429,
      orders: RATE_LIMIT_429,
      reservations: RATE_LIMIT_429,
      crm: RATE_LIMIT_429,
    });
    expect(shouldShowStaleBanner(count, false)).toBe(true);

    count = runReconcileCycle(count, {
      bills: null,
      orders: null,
      reservations: null,
      crm: null,
    });
    expect(count).toBe(0);
    expect(shouldShowStaleBanner(count, false)).toBe(false);
  });

  it("still pins the banner for a non-429 outage (#623 guard)", () => {
    let count = 0;
    for (let cycle = 0; cycle < 4; cycle += 1) {
      count = runReconcileCycle(count, {
        bills: EDGE_502,
        orders: EDGE_502,
        reservations: EDGE_502,
        crm: EDGE_502,
      });
    }
    expect(count).toBeGreaterThanOrEqual(STALE_BANNER_THRESHOLD);
    expect(shouldShowStaleBanner(count, false)).toBe(true);
  });

  it("does not hold-or-decay when bills 502s while CRM only 429s (#623)", () => {
    let count = STALE_BANNER_THRESHOLD;
    expect(shouldShowStaleBanner(count, false)).toBe(true);

    count = runReconcileCycle(count, {
      bills: EDGE_502,
      orders: RATE_LIMIT_429,
      reservations: RATE_LIMIT_429,
      crm: RATE_LIMIT_429,
    });
    expect(count).toBeGreaterThan(STALE_BANNER_THRESHOLD);
    expect(shouldShowStaleBanner(count, false)).toBe(true);

    count = runReconcileCycle(count, {
      bills: EDGE_502,
      orders: RATE_LIMIT_429,
      reservations: RATE_LIMIT_429,
      crm: RATE_LIMIT_429,
    });
    expect(shouldShowStaleBanner(count, false)).toBe(true);
  });

  it("raises the counter from clean when a bills 502 rides with sibling 429s (#623)", () => {
    let count = 0;
    count = runReconcileCycle(count, {
      bills: EDGE_502,
      orders: RATE_LIMIT_429,
      reservations: RATE_LIMIT_429,
      crm: RATE_LIMIT_429,
    });
    count = runReconcileCycle(count, {
      bills: EDGE_502,
      orders: RATE_LIMIT_429,
      reservations: RATE_LIMIT_429,
      crm: RATE_LIMIT_429,
    });
    expect(count).toBe(2);
    expect(shouldShowStaleBanner(count, false)).toBe(true);
  });

  it("holds through an all-legs-429 storm after a pin; unprobed CRM stays silent (#623)", () => {
    let count = STALE_BANNER_THRESHOLD;
    count = runReconcileCycle(count, {
      bills: RATE_LIMIT_429,
      orders: RATE_LIMIT_429,
      reservations: RATE_LIMIT_429,
      crmProbed: false,
    });
    expect(count).toBe(STALE_BANNER_THRESHOLD);
    expect(shouldShowStaleBanner(count, false)).toBe(true);
  });

  it("does not clear when only bills 200 mid-storm (#623)", () => {
    const count = runReconcileCycle(STALE_BANNER_THRESHOLD, {
      bills: null,
      orders: RATE_LIMIT_429,
      reservations: RATE_LIMIT_429,
      crm: RATE_LIMIT_429,
    });
    expect(count).toBe(STALE_BANNER_THRESHOLD);
    expect(shouldShowStaleBanner(count, false)).toBe(true);
  });

  it("does not clear when only reservations or only CRM 200 (#623)", () => {
    expect(
      runReconcileCycle(STALE_BANNER_THRESHOLD, {
        bills: RATE_LIMIT_429,
        orders: RATE_LIMIT_429,
        reservations: null,
        crm: RATE_LIMIT_429,
      }),
    ).toBe(STALE_BANNER_THRESHOLD);
    expect(
      runReconcileCycle(STALE_BANNER_THRESHOLD, {
        bills: RATE_LIMIT_429,
        orders: RATE_LIMIT_429,
        reservations: RATE_LIMIT_429,
        crm: null,
      }),
    ).toBe(STALE_BANNER_THRESHOLD);
    expect(
      shouldShowStaleBanner(STALE_BANNER_THRESHOLD, false),
    ).toBe(true);
  });

  it("clears only after every failed live-refresh source 200s (#623)", () => {
    let count = STALE_BANNER_THRESHOLD;
    expect(shouldShowStaleBanner(count, false)).toBe(true);

    count = runReconcileCycle(count, {
      bills: RATE_LIMIT_429,
      orders: RATE_LIMIT_429,
      reservations: RATE_LIMIT_429,
      crm: RATE_LIMIT_429,
    });
    expect(shouldShowStaleBanner(count, false)).toBe(true);

    count = runReconcileCycle(count, {
      bills: null,
      orders: null,
      reservations: null,
      crm: null,
    });
    expect(count).toBe(0);
    expect(shouldShowStaleBanner(count, false)).toBe(false);
  });
});

/** The kitchen role's default grants (backend/internal/server/rbac.go). */
const KITCHEN_PERMISSIONS = [
  "business:read",
  "menu:read",
  "orders:read",
  "orders:write",
  "orders:status",
  "orders:kitchen",
  "bills:read",
  "staff:read",
  "delivery:dispatch:read",
];

const FORBIDDEN_403 = { response: { status: 403 } };

describe("dashboard live-reconcile legs a staff role cannot read", () => {
  it("skips reservations, CRM and floor occupancy for the kitchen role", () => {
    expect(
      reconcileLegAccess({
        isStaffUser: true,
        permissionsKnown: true,
        permissions: KITCHEN_PERMISSIONS,
      }),
    ).toEqual({ reservations: false, crm: false, floorOccupancy: false });
  });

  it("probes every leg for owners and while staff permissions are unknown", () => {
    const all = { reservations: true, crm: true, floorOccupancy: true };
    expect(
      reconcileLegAccess({
        isStaffUser: false,
        permissionsKnown: false,
        permissions: [],
      }),
    ).toEqual(all);
    expect(
      reconcileLegAccess({
        isStaffUser: true,
        permissionsKnown: false,
        permissions: [],
      }),
    ).toEqual(all);
  });

  it("probes the legs a staff role does hold", () => {
    expect(
      reconcileLegAccess({
        isStaffUser: true,
        permissionsKnown: true,
        permissions: [...KITCHEN_PERMISSIONS, "reservations:read", "tables:read"],
      }),
    ).toEqual({ reservations: true, crm: false, floorOccupancy: true });
  });

  it("never raises the stale banner for a kitchen shift with bills and orders 200", () => {
    let count = 0;
    for (let cycle = 0; cycle < 5; cycle++) {
      count = applyReconcileCycle(count, {
        bills: RECONCILE_LEG_OK,
        orders: RECONCILE_LEG_OK,
        reservations: skippedReconcileLeg(),
        crm: skippedReconcileLeg(),
      }).nextCount;
    }
    expect(count).toBe(0);
    expect(shouldShowStaleBanner(count, false)).toBe(false);
  });

  it("heals a counter left by a 403 before permissions loaded", () => {
    let count = runReconcileCycle(0, {
      bills: null,
      orders: null,
      reservations: FORBIDDEN_403,
      crmProbed: false,
    });
    expect(count).toBe(1);
    count = applyReconcileCycle(count, {
      bills: RECONCILE_LEG_OK,
      orders: RECONCILE_LEG_OK,
      reservations: skippedReconcileLeg(),
      crm: skippedReconcileLeg(),
    }).nextCount;
    expect(count).toBe(0);
  });

  it("still counts a 403 on a leg the role holds (permission revoked mid-shift)", () => {
    let count = 0;
    count = runReconcileCycle(count, {
      bills: null,
      orders: null,
      reservations: FORBIDDEN_403,
      crmProbed: false,
    });
    count = runReconcileCycle(count, {
      bills: null,
      orders: null,
      reservations: FORBIDDEN_403,
      crmProbed: false,
    });
    expect(shouldShowStaleBanner(count, false)).toBe(true);
  });

  it("a skipped reservations leg cannot mask a bills outage", () => {
    const { allOk } = applyReconcileCycle(0, {
      bills: failedReconcileLeg(EDGE_502),
      orders: RECONCILE_LEG_OK,
      reservations: skippedReconcileLeg(),
      crm: skippedReconcileLeg(),
    });
    expect(allOk).toBe(false);
    expect(isReconcileClean({ billsOk: true, ordersOk: true })).toBe(true);
  });
});
