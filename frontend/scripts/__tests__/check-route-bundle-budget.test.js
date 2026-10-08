/** @jest-environment node */
"use strict";

const fs = require("fs");
const os = require("os");
const path = require("path");
const {
  DASHBOARD_ROUTE,
  measureRouteFiles,
  measureAllRoutes,
} = require("../check-route-bundle-budget");

function tmpNext(pages, fileSizes) {
  const dir = fs.mkdtempSync(path.join(os.tmpdir(), "route-budget-"));
  const nextDir = path.join(dir, ".next");
  fs.mkdirSync(nextDir, { recursive: true });
  fs.writeFileSync(
    path.join(nextDir, "app-build-manifest.json"),
    JSON.stringify({ pages }, null, 2),
  );
  for (const [rel, size] of Object.entries(fileSizes)) {
    const abs = path.join(nextDir, rel);
    fs.mkdirSync(path.dirname(abs), { recursive: true });
    fs.writeFileSync(abs, Buffer.alloc(size, 0x61));
  }
  return nextDir;
}

describe("measureRouteFiles", () => {
  it("sums statSync sizes and de-dupes repeated chunk paths", () => {
    const nextDir = tmpNext({}, {
      "static/chunks/a.js": 1024,
      "static/chunks/b.js": 2048,
    });
    const m = measureRouteFiles(nextDir, [
      "static/chunks/a.js",
      "static/chunks/b.js",
      "static/chunks/a.js",
    ]);
    expect(m.totalBytes).toBe(3072);
    expect(m.chunkCount).toBe(2);
    expect(m.missing).toBe(0);
  });

  it("counts missing files without throwing", () => {
    const nextDir = tmpNext({}, { "static/chunks/a.js": 100 });
    const m = measureRouteFiles(nextDir, [
      "static/chunks/a.js",
      "static/chunks/gone.js",
    ]);
    expect(m.totalBytes).toBe(100);
    expect(m.missing).toBe(1);
    expect(m.chunkCount).toBe(1);
  });
});

describe("measureAllRoutes", () => {
  it("measures dashboard and median across page routes only", () => {
    const nextDir = tmpNext(
      {
        [DASHBOARD_ROUTE]: ["static/chunks/big.js", "static/chunks/shared.js"],
        "/(shop)/other/page": ["static/chunks/small.js", "static/chunks/shared.js"],
        "/(shop)/layout": ["static/chunks/layout-only.js"], // excluded from median
        "/loading": ["static/chunks/loading.js"],
      },
      {
        "static/chunks/big.js": 5000 * 1024,
        "static/chunks/shared.js": 100 * 1024,
        "static/chunks/small.js": 200 * 1024,
        "static/chunks/layout-only.js": 50 * 1024,
        "static/chunks/loading.js": 10 * 1024,
      },
    );
    const m = measureAllRoutes(nextDir);
    expect(m.pageRouteCount).toBe(2);
    expect(m.dashboard).not.toBeNull();
    expect(m.dashboard.route).toBe(DASHBOARD_ROUTE);
    expect(m.dashboard.chunkCount).toBe(2);
    // 5100 KiB
    expect(m.dashboard.totalKiB).toBe(5100);
    // sizes: 5100 KiB and 300 KiB → median is the higher of the two when n=2 floor(1)=1 → 5100
    // actually floor(2/2)=1 → sorted [300, 5100][1] = 5100
    expect(m.medianPageKiB).toBe(5100);
  });

  it("throws when manifest is missing", () => {
    const dir = fs.mkdtempSync(path.join(os.tmpdir(), "route-budget-empty-"));
    expect(() => measureAllRoutes(path.join(dir, ".next"))).toThrow(
      /app-build-manifest/,
    );
  });
});
