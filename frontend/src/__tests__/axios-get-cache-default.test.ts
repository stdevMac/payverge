/**
 * Regression guard for C1 (Wave 0) — the axios GET cache is opt-in, not opt-out.
 *
 * The interceptor in `src/api/tools/instance.ts` used to cache EVERY GET for
 * 5 minutes (`config._useCache = config._useCache !== false`). That was a
 * second cache layer React Query could not see; it silently served stale data
 * underneath SSE events, RQ invalidations, and every poll — re-opening the
 * guest double-payment window and freezing "live" operator boards.
 *
 * C1 inverted the default: GETs are UNCACHED unless a call site explicitly
 * passes `_useCache: true`, reserved for genuinely static reference reads. This
 * guard asserts the inverted default holds and that realtime / polling API
 * modules never opt back into the cache.
 */

import fs from "fs";
import path from "path";

const FRONTEND_ROOT = path.resolve(__dirname, "..", "..");

function read(relative: string): string {
  return fs.readFileSync(path.join(FRONTEND_ROOT, relative), "utf8");
}

describe("axios GET cache default is opt-in (C1)", () => {
  it("the request interceptor defaults GET _useCache to false (opt-in only)", () => {
    const src = read("src/api/tools/instance.ts");
    // The inverted, opt-in form must be present…
    expect(src).toMatch(/_useCache\s*=\s*config\._useCache\s*===\s*true/);
    // …and the old opt-out default must be gone.
    expect(src).not.toMatch(/_useCache\s*=\s*config\._useCache\s*!==\s*false/);
  });

  // Realtime, polling, and per-session reads must never opt into the 5-min
  // apiCache — freshness is the whole point of C1. Only static reference reads
  // (currency.ts) are allowed to pass `_useCache: true`.
  const REALTIME_API_MODULES = [
    "src/api/bills.ts",
    "src/api/orders.ts",
    "src/api/alternativePayments.ts",
    "src/api/analytics.ts",
    "src/api/reservations.ts",
    "src/api/delivery.ts",
    "src/api/cashRegister.ts",
    "src/api/kitchenOrders.ts",
    "src/api/counters.ts",
  ] as const;

  for (const file of REALTIME_API_MODULES) {
    it(`${file} does not opt into the GET cache`, () => {
      const src = read(file);
      expect(src).not.toMatch(/_useCache\s*:\s*true/);
    });
  }
});
