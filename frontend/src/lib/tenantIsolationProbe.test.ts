/** @jest-environment node */
import {
  buildCrossTenantMatrix,
  classifyIsolationStatus,
} from "./tenantIsolationProbe";

describe("GAP-2 tenant isolation probe helpers", () => {
  it("builds a cross-tenant read/mutate matrix for the other business id", () => {
    const rows = buildCrossTenantMatrix({
      otherBusinessId: 99,
      otherBillId: 7,
    });
    expect(rows.some((r) => r.path.includes("/businesses/99"))).toBe(true);
    expect(rows.some((r) => r.method === "PUT")).toBe(true);
    expect(rows.some((r) => r.path.endsWith("/bills/7"))).toBe(true);
    // Must not include self-business paths only — always foreign id 99.
    expect(rows.every((r) => r.path.includes("/99"))).toBe(true);
  });

  it("classifies 401/403/404 as isolated and 2xx as leaked", () => {
    for (const status of [401, 403, 404]) {
      expect(classifyIsolationStatus(status)).toEqual({
        isolated: true,
        leaked: false,
        failed: false,
      });
    }
    for (const status of [200, 204]) {
      expect(classifyIsolationStatus(status)).toEqual({
        isolated: false,
        leaked: true,
        failed: false,
      });
    }
  });

  it("classifies 5xx and other statuses as failed, never isolated", () => {
    for (const status of [500, 502, 503, 400, 302, 409, 422]) {
      expect(classifyIsolationStatus(status)).toEqual({
        isolated: false,
        leaked: false,
        failed: true,
      });
    }
  });
});
