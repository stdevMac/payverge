import en from "@/i18n/messages/en/marketingDashboard.json";
import es from "@/i18n/messages/es/marketingDashboard.json";
import { FORMATS, FORMAT_ORDER } from "./formats";
import {
  NARRATIVE_ROLE_ORDER,
  NARRATIVE_ROLES,
} from "./narrativeRoles";

type Tree = Record<string, unknown>;

function lookup(tree: Tree, dotted: string): unknown {
  return dotted
    .split(".")
    .reduce<unknown>(
      (node, key) =>
        node && typeof node === "object"
          ? (node as Tree)[key]
          : undefined,
      tree,
    );
}

describe("format i18n", () => {
  it("gives every format a name and a use case in en and es", () => {
    FORMAT_ORDER.forEach((id) => {
      const format = FORMATS[id];
      [format.labelKey, format.useKey].forEach((key) => {
        expect(typeof lookup(en as Tree, key)).toBe("string");
        expect(typeof lookup(es as Tree, key)).toBe("string");
        expect(String(lookup(en as Tree, key)).length).toBeGreaterThan(0);
        expect(String(lookup(es as Tree, key)).length).toBeGreaterThan(0);
      });
    });
  });

  // check-operator-locales.js enforces this repo-wide at commit time; asserting
  // it here means a missing es key fails in the same run as the code that needs
  // it, rather than at the hook.
  it("keeps the campaignKit namespace in en/es parity", () => {
    const enKit = (en as Tree).campaignKit as Tree;
    const esKit = (es as Tree).campaignKit as Tree;
    expect(Object.keys(enKit).sort()).toEqual(Object.keys(esKit).sort());
  });

  it("keeps the narrativeKit namespace in en/es parity with every role", () => {
    const enKit = (en as Tree).narrativeKit as Tree;
    const esKit = (es as Tree).narrativeKit as Tree;
    expect(Object.keys(enKit).sort()).toEqual(Object.keys(esKit).sort());
    NARRATIVE_ROLE_ORDER.forEach((id) => {
      const role = NARRATIVE_ROLES[id];
      [role.labelKey, role.useKey].forEach((key) => {
        expect(typeof lookup(en as Tree, key)).toBe("string");
        expect(typeof lookup(es as Tree, key)).toBe("string");
        expect(String(lookup(en as Tree, key)).length).toBeGreaterThan(0);
      });
    });
    // Anti-calendar wording must exist (S2-C.7).
    expect(String(lookup(en as Tree, "narrativeKit.readyNow"))).toMatch(
      /not a schedule/i,
    );
    expect(String(lookup(es as Tree, "narrativeKit.readyNow")).length).toBeGreaterThan(
      0,
    );
  });

  // The two meanings of "kit" have to stay in separate namespaces or a
  // translator has no way to tell which one they are looking at.
  it("does not collide with the art-direction kit namespace", () => {
    expect(Object.keys((en as Tree).kits as Tree)).toEqual([
      "editorial",
      "bold",
      "minimal",
      "chalkboard",
      "linen",
      "ticket",
    ]);
  });
});
