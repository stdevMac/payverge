import en from "@/i18n/messages/en/marketingDashboard.json";
import es from "@/i18n/messages/es/marketingDashboard.json";
import { DESTINATION_ORDER, DESTINATIONS } from "./destinations";

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

describe("destination i18n", () => {
  it("gives every destination name, hint, and export CTA in en and es", () => {
    DESTINATION_ORDER.forEach((id) => {
      const dest = DESTINATIONS[id];
      [dest.labelKey, dest.hintKey, dest.exportKey].forEach((key) => {
        expect(typeof lookup(en as Tree, key)).toBe("string");
        expect(typeof lookup(es as Tree, key)).toBe("string");
        expect(String(lookup(en as Tree, key)).length).toBeGreaterThan(0);
        expect(String(lookup(es as Tree, key)).length).toBeGreaterThan(0);
      });
    });
  });

  it("keeps the destinations namespace in en/es parity", () => {
    const enDest = (en as Tree).destinations as Tree;
    const esDest = (es as Tree).destinations as Tree;
    expect(Object.keys(enDest).sort()).toEqual(Object.keys(esDest).sort());
    DESTINATION_ORDER.forEach((id) => {
      const enEntry = enDest[id] as Tree;
      const esEntry = esDest[id] as Tree;
      expect(Object.keys(enEntry).sort()).toEqual(Object.keys(esEntry).sort());
    });
    expect(Object.keys(enDest.checklist as Tree).sort()).toEqual(
      Object.keys(esDest.checklist as Tree).sort(),
    );
  });

  it("covers every checklist key used by destinations", () => {
    const keys = new Set<string>();
    DESTINATION_ORDER.forEach((id) => {
      DESTINATIONS[id].checklistKeys.forEach((k) => keys.add(k));
    });
    keys.forEach((key) => {
      const path = `destinations.checklist.${key}`;
      expect(typeof lookup(en as Tree, path)).toBe("string");
      expect(typeof lookup(es as Tree, path)).toBe("string");
    });
  });

  it("outcome destination copy never says scheduled", () => {
    const enOutcome = (en as Tree).outcome as Tree;
    const esOutcome = (es as Tree).outcome as Tree;
    ["downloadedWithDestination", "sharedWithDestination", "copiedWithDestination"].forEach(
      (key) => {
        const enText = String(enOutcome[key] ?? "").toLowerCase();
        const esText = String(esOutcome[key] ?? "").toLowerCase();
        expect(enText.length).toBeGreaterThan(0);
        expect(esText.length).toBeGreaterThan(0);
        expect(enText).not.toMatch(/schedul/);
        expect(esText).not.toMatch(/programad|agendad/);
      },
    );
  });
});
