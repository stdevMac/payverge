import {
  ALL_PERMISSIONS,
  permissionLabel,
  permissionDescription,
} from "./permissions";
import enRbac from "../i18n/messages/en/rbacPermissions.json";
import esRbac from "../i18n/messages/es/rbacPermissions.json";

type RbacEntry = { label: string; description: string };
const en = enRbac as Record<string, RbacEntry>;
const es = esRbac as Record<string, RbacEntry>;

describe("permissionLabel (L5-23)", () => {
  it("returns the translated label when present", () => {
    const t = (key: string) =>
      key === "rbacPermissions.menu:write.label" ? "Edit menu" : key;
    expect(permissionLabel("menu:write", t)).toBe("Edit menu");
  });

  it("falls back to the raw slug when the key is missing", () => {
    const t = (key: string) => key;
    expect(permissionLabel("brand_new:perm", t)).toBe("brand_new:perm");
  });

  it("falls back when translation echoes the full namespaced key", () => {
    const t = (key: string) => `businessDashboard.${key}`;
    expect(permissionLabel("staff:read", t)).toBe("staff:read");
  });

  it("falls back when getTranslation returns sentence-case leaf Label", () => {
    const t = () => "Label";
    expect(permissionLabel("orders:read", t)).toBe("orders:read");
  });
});

describe("permissionDescription", () => {
  it("returns null when description is missing", () => {
    const t = (key: string) => key;
    expect(permissionDescription("menu:write", t)).toBeNull();
  });
});

describe("rbacPermissions content quality (L5-23)", () => {
  it("covers every catalog slug with a non-empty label and description in en and es", () => {
    for (const perm of ALL_PERMISSIONS) {
      expect(en[perm]?.label?.trim()).toBeTruthy();
      expect(en[perm]?.description?.trim()).toBeTruthy();
      expect(es[perm]?.label?.trim()).toBeTruthy();
      expect(es[perm]?.description?.trim()).toBeTruthy();
    }
  });

  it("en labels are human copy, not mechanical slug transforms", () => {
    // The original generator emitted "Ai Waiter: read" — a prettified slug.
    const mechanicalLabel = /^[A-Z][a-z]+ [A-Za-z ]*: (read|write)$/;
    const offenders = ALL_PERMISSIONS.filter((perm) =>
      mechanicalLabel.test(en[perm]?.label ?? ""),
    );
    expect(offenders).toEqual([]);
  });

  it("en descriptions are not the mechanical 'Allows <slug>.' transform", () => {
    const mechanicalDescription = /^Allows [a-z_ ]+: [a-z_ ]+\.$/;
    const offenders = ALL_PERMISSIONS.filter((perm) =>
      mechanicalDescription.test(en[perm]?.description ?? ""),
    );
    expect(offenders).toEqual([]);
  });

  it("es is a real translation, not a byte-copy of en", () => {
    // Sample across domains: every one of these must differ in BOTH fields.
    const sample = [
      "ai_waiter:read",
      "print:bill",
      "fiscal:credit",
      "payroll:write",
      "bills:refund",
      "schedule:write",
      "menu:items",
      "crm:export",
      "printers:read",
      "cash_register:operate",
    ];
    for (const perm of sample) {
      expect(es[perm].label).not.toEqual(en[perm].label);
      expect(es[perm].description).not.toEqual(en[perm].description);
    }
    // Byte-copy tripwire for the whole file: identical leaf values must stay
    // rare (brand terms like "Marketing" may coincide; 272/272 must never).
    let identical = 0;
    for (const perm of ALL_PERMISSIONS) {
      if (es[perm].label === en[perm].label) identical += 1;
      if (es[perm].description === en[perm].description) identical += 1;
    }
    expect(identical).toBeLessThanOrEqual(10);
  });
});

describe("permissionDescription is consumable (L5-23)", () => {
  it("returns the translated description when present", () => {
    const t = (key: string) =>
      key === "rbacPermissions.menu:write.description"
        ? "Edit dishes and categories"
        : key;
    expect(permissionDescription("menu:write", t)).toBe(
      "Edit dishes and categories",
    );
  });

  it("is rendered by StaffRoleManagement (not a dead helper)", () => {
    const fs = require("fs") as typeof import("fs");
    const path = require("path") as typeof import("path");
    const src = fs.readFileSync(
      path.join(
        process.cwd(),
        "src/components/staff/StaffRoleManagement.tsx",
      ),
      "utf8",
    );
    expect(src).toMatch(/permissionDescription/);
    expect(src).toMatch(/formatPermissionDesc\(perm\)/);
    // Description text is mounted in the DOM, not only computed.
    expect(src).toMatch(
      /formatPermissionDesc\(perm\) \? \([\s\S]*?<p[\s\S]*?\{formatPermissionDesc\(perm\)\}/,
    );
  });
});
