/**
 * Structural + pure-helper tests for the Session P reference leaves
 * (L5-11 CRM, L6-41 settings, L3-43 delivery).
 */
import fs from "node:fs";
import path from "node:path";
import { buildSearchWithParams, normalizeUrlParam } from "@/hooks/urlState";
import { SETTINGS_SECTIONS } from "@/utils/businessUrl";
import { CRM_FOCUS_VALUES, crmCustomersFocusPatches } from "../CRMManager";
import { DELIVERY_SUB_VALUES } from "../delivery/DeliveryAdmin";

const crmSource = fs.readFileSync(
  path.resolve(__dirname, "../CRMManager.tsx"),
  "utf-8",
);
const settingsSource = fs.readFileSync(
  path.resolve(__dirname, "../BusinessSettings.tsx"),
  "utf-8",
);
const deliverySource = fs.readFileSync(
  path.resolve(__dirname, "../delivery/DeliveryAdmin.tsx"),
  "utf-8",
);

describe("L5-11 CRM focus write-back (reference leaf)", () => {
  it("uses useOptionalUrlState for ?focus=", () => {
    expect(crmSource).toMatch(/useOptionalUrlState/);
    expect(crmSource).toMatch(/key:\s*["']focus["']/);
    expect(crmSource).toMatch(/CRM_FOCUS_VALUES/);
  });

  it("openSegment writes tab+sub+focus atomically (no second replace)", () => {
    expect(crmSource).toMatch(/crmCustomersFocusPatches/);
    expect(crmSource).toMatch(/useSetUrlParams/);
    expect(crmSource).not.toMatch(/setCustomersSegment\(segment/);
    // Old read-once window.location only pattern is gone
    expect(crmSource).not.toMatch(
      /new URLSearchParams\(window\.location\.search\)\.get\(["']focus["']\)/,
    );
  });

  it("validates the focus enum", () => {
    expect([...CRM_FOCUS_VALUES]).toEqual(["lapsed", "vip", "new", "at-risk"]);
  });

  it("crmCustomersFocusPatches keep sub=customers and focus=at-risk together", () => {
    const out = buildSearchWithParams(
      "tab=crm&sub=segments",
      crmCustomersFocusPatches("at-risk"),
    );
    const params = new URLSearchParams(out);
    expect(params.get("tab")).toBe("crm");
    expect(params.get("sub")).toBe("customers");
    expect(params.get("focus")).toBe("at-risk");
  });
});

describe("L6-41 settings section write-back (Root D)", () => {
  it("uses useUrlState for ?section= and deleted the read-once rationale", () => {
    expect(settingsSource).toMatch(/useUrlState/);
    expect(settingsSource).toMatch(/key:\s*["']section["']/);
    expect(settingsSource).toMatch(/SETTINGS_SECTIONS/);
    expect(settingsSource).toMatch(/activeWhen:\s*\{\s*key:\s*["']tab["']/);
    // Root D comment must be gone
    expect(settingsSource).not.toMatch(
      /Read once on mount so later in-page tab clicks aren't fought by the URL/,
    );
  });

  it("normalizes invalid section to profile", () => {
    expect(normalizeUrlParam("garbage", SETTINGS_SECTIONS, "profile")).toBe(
      "profile",
    );
    expect(
      normalizeUrlParam("notifications", SETTINGS_SECTIONS, "profile"),
    ).toBe("notifications");
  });
});

describe("L3-43 delivery sub write-back", () => {
  it("uses useUrlState for ?sub= with delivery sub values", () => {
    expect(deliverySource).toMatch(/useUrlState/);
    expect(deliverySource).toMatch(/key:\s*["']sub["']/);
    expect(deliverySource).toMatch(/DELIVERY_SUB_VALUES/);
    // No bare useState("configuration") for the active tab
    expect(deliverySource).not.toMatch(
      /useState\(\s*["']configuration["']\s*\)/,
    );
  });

  it("declares the five delivery sub-tabs", () => {
    expect([...DELIVERY_SUB_VALUES]).toEqual([
      "configuration",
      "dispatch",
      "history",
      "drivers",
      "performance",
    ]);
  });

  it("normalizes invalid delivery sub to configuration", () => {
    expect(
      normalizeUrlParam("nope", DELIVERY_SUB_VALUES, "configuration"),
    ).toBe("configuration");
    expect(
      normalizeUrlParam("drivers", DELIVERY_SUB_VALUES, "configuration"),
    ).toBe("drivers");
  });
});
