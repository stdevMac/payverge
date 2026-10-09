/** @jest-environment node */
"use strict";

const fs = require("fs");
const path = require("path");
const { collectAdvisories, evaluate } = require("../check-npm-audit");

const BRACES = {
  source: 1240992,
  name: "braces",
  title: "braces stack exhaustion",
  url: "https://github.com/advisories/GHSA-vfj7-8cjw-p6xm",
  severity: "high",
};

function report(...advisories) {
  const vulnerabilities = {
    chokidar: { severity: "high", via: ["braces"] },
  };
  for (const adv of advisories) {
    vulnerabilities[adv.name] = { severity: adv.severity, via: [adv] };
  }
  return { vulnerabilities };
}

const TODAY = new Date("2026-10-05T00:00:00Z");
const allow = (id, reviewBy = "2027-01-31") => ({
  advisories: [{ id, reason: "r", reviewBy }],
});

describe("check-npm-audit", () => {
  it("collects advisories once, ignoring transitive string references", () => {
    const advisories = collectAdvisories(report(BRACES));
    expect(advisories.map((a) => a.id)).toEqual(["GHSA-vfj7-8cjw-p6xm"]);
  });

  it("fails on a high advisory that is not allowlisted", () => {
    const { failures } = evaluate(
      report(BRACES),
      { advisories: [] },
      { today: TODAY },
    );
    expect(failures).toHaveLength(1);
    expect(failures[0]).toContain("GHSA-vfj7-8cjw-p6xm");
  });

  it("accepts an allowlisted advisory before its review date", () => {
    const res = evaluate(report(BRACES), allow("GHSA-vfj7-8cjw-p6xm"), {
      today: TODAY,
    });
    expect(res.failures).toEqual([]);
    expect(res.accepted).toHaveLength(1);
  });

  it("fails an allowlisted advisory once its review date has passed", () => {
    const res = evaluate(
      report(BRACES),
      allow("GHSA-vfj7-8cjw-p6xm", "2026-10-04"),
      {
        today: TODAY,
      },
    );
    expect(res.failures[0]).toContain("expired");
  });

  it("ignores advisories below the threshold and reports stale entries", () => {
    const moderate = {
      ...BRACES,
      severity: "moderate",
      url: "https://x/GHSA-aaaa-bbbb-cccc",
    };
    const res = evaluate(report(moderate), allow("GHSA-vfj7-8cjw-p6xm"), {
      today: TODAY,
    });
    expect(res.failures).toEqual([]);
    expect(res.stale).toEqual(["GHSA-vfj7-8cjw-p6xm"]);
  });

  it("ships a well-formed allowlist", () => {
    const list = JSON.parse(
      fs.readFileSync(
        path.join(__dirname, "..", "npm-audit-allowlist.json"),
        "utf8",
      ),
    );
    for (const entry of list.advisories) {
      expect(entry.id).toMatch(/^GHSA-/);
      expect(entry.reason.length).toBeGreaterThan(20);
      expect(entry.reviewBy).toMatch(/^\d{4}-\d{2}-\d{2}$/);
    }
  });
});
