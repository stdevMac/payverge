import fs from "fs";
import path from "path";
import roleDefaults from "./__fixtures__/role-default-permissions.json";
import { ALL_PERMISSIONS, PERMISSION_CATEGORIES } from "./permissions";

function bePermissionLiterals(): string[] {
  // frontend/src/constants → repo root is ../../../
  const rbacPath = path.join(
    __dirname,
    "../../../backend/internal/server/rbac.go",
  );
  const src = fs.readFileSync(rbacPath, "utf8");
  const re = /Permission\s*=\s*"([^"]+)"/g;
  const out: string[] = [];
  let m: RegExpExecArray | null;
  while ((m = re.exec(src)) !== null) out.push(m[1]);
  return out;
}

describe("ALL_PERMISSIONS ↔ rbac.go", () => {
  it("matches backend Permission string literals exactly", () => {
    const be = new Set(bePermissionLiterals());
    const fe = new Set<string>(ALL_PERMISSIONS as readonly string[]);
    const missingOnFe = [...be].filter((p) => !fe.has(p)).sort();
    const extraOnFe = [...fe].filter((p) => !be.has(p)).sort();
    expect({ missingOnFe, extraOnFe }).toEqual({
      missingOnFe: [],
      extraOnFe: [],
    });
  });

  it("places every ALL_PERMISSIONS entry in exactly one PERMISSION_CATEGORIES group", () => {
    const seen = new Map<string, string[]>();
    for (const [category, perms] of Object.entries(PERMISSION_CATEGORIES)) {
      for (const p of perms) {
        const owners = seen.get(p) ?? [];
        owners.push(category);
        seen.set(p, owners);
      }
    }

    const missingFromCategories = ALL_PERMISSIONS.filter((p) => !seen.has(p));
    const duplicates = [...seen.entries()]
      .filter(([, cats]) => cats.length > 1)
      .map(([perm, cats]) => ({ perm, cats }))
      .sort((a, b) => a.perm.localeCompare(b.perm));
    const extraInCategories = [...seen.keys()]
      .filter((p) => !(ALL_PERMISSIONS as readonly string[]).includes(p))
      .sort();

    expect({ missingFromCategories, duplicates, extraInCategories }).toEqual({
      missingFromCategories: [],
      duplicates: [],
      extraInCategories: [],
    });
  });

  it("catalogs file mutations with settings and grants them only to managers by default", () => {
    expect(PERMISSION_CATEGORIES.settings).toEqual(
      expect.arrayContaining(["files:upload", "files:delete"]),
    );
    expect(roleDefaults.manager).toEqual(
      expect.arrayContaining(["files:upload", "files:delete"]),
    );

    for (const role of ["host", "server", "kitchen"] as const) {
      expect(roleDefaults[role]).not.toEqual(
        expect.arrayContaining(["files:upload", "files:delete"]),
      );
    }
  });
});
