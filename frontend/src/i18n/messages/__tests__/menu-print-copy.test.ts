import fs from "node:fs";
import path from "node:path";

import {
  MENU_DESIGN_FAMILY_IDS,
  MENU_OUTPUT_FORMATS,
  MENU_TREATMENTS,
  PAPER_DIMENSIONS_MM,
  PRINT_CONTACT_PLACEMENTS,
  PRINT_TYPOGRAPHY_PERSONALITIES,
} from "../../../lib/menuPrint/types";
import en from "../en/businessDashboard.json";
import esAr from "../es-ar/businessDashboard.json";
import es from "../es/businessDashboard.json";

type MessageTree = Record<string, unknown>;

const printTree = (root: MessageTree): MessageTree =>
  ((root.dashboard as MessageTree).menuBuilder as MessageTree)
    .print as MessageTree;

const readKey = (root: unknown, key: string): unknown =>
  key
    .split(".")
    .reduce<unknown>(
      (value, segment) =>
        value && typeof value === "object"
          ? (value as MessageTree)[segment]
          : undefined,
      root,
    );

const flattenStrings = (
  root: MessageTree,
  prefix = "",
): Record<string, string> =>
  Object.fromEntries(
    Object.entries(root).flatMap(([key, value]) => {
      const next = prefix ? `${prefix}.${key}` : key;
      return typeof value === "string"
        ? [[next, value]]
        : value && typeof value === "object" && !Array.isArray(value)
          ? Object.entries(flattenStrings(value as MessageTree, next))
          : [];
    }),
  );

const FAMILY_KEY_PARTS: Record<
  (typeof MENU_DESIGN_FAMILY_IDS)[number],
  string
> = {
  atelier: "atelier",
  maison: "maison",
  osteria: "osteria",
  "night-house": "nightHouse",
  counter: "counter",
  street: "street",
  field: "field",
  gallery: "gallery",
};

const TREATMENT_KEY_PARTS: Record<(typeof MENU_TREATMENTS)[number], string> = {
  "type-led": "typeLed",
  balanced: "balanced",
  "photo-led": "photoLed",
  compact: "compact",
};

const FORMAT_KEY_PARTS: Record<(typeof MENU_OUTPUT_FORMATS)[number], string> = {
  "single-sheet": "singleSheet",
  "two-page-spread": "twoPageSpread",
  "folded-booklet": "foldedBooklet",
  "takeaway-trifold": "takeawayTrifold",
  "drinks-card": "drinksCard",
  "counter-menu": "counterMenu",
};

const PAPER_KEY_PARTS = {
  letter: "letter",
  a4: "a4",
  "half-letter": "halfLetter",
  a5: "a5",
} as const;

const IMAGE_ROLE_KEY_PARTS = [
  "cover",
  "fullWidth",
  "halfPage",
  "editorialCrop",
  "categoryOpener",
  "paired",
  "compactTile",
] as const;

const FOCAL_KEY_PARTS = [
  "topLeft",
  "top",
  "topRight",
  "left",
  "center",
  "right",
  "bottomLeft",
  "bottom",
  "bottomRight",
] as const;

const RECOMMENDATION_KEYS = [
  "versatileRestaurant",
  "editorialAlternative",
  "warmAlternative",
  "cafeCounter",
  "cleanAlternative",
  "fineDiningMaison",
  "barNightHouse",
  "casualAlternative",
  "quickServiceStreet",
  "counterAlternative",
  "bakeryCounter",
  "foodTruckStreet",
  "balanced",
] as const;

const requiredDynamicKeys = [
  ...["look", "personalize"].map((key) => `stage.${key}`),
  ...MENU_DESIGN_FAMILY_IDS.flatMap((id) => [
    `family.${FAMILY_KEY_PARTS[id]}`,
    `familyDesc.${FAMILY_KEY_PARTS[id]}`,
  ]),
  ...MENU_TREATMENTS.flatMap((id) => [
    `treatment.${TREATMENT_KEY_PARTS[id]}`,
    `treatmentDesc.${TREATMENT_KEY_PARTS[id]}`,
    `photos.${TREATMENT_KEY_PARTS[id]}NoSelection`,
  ]),
  ...MENU_OUTPUT_FORMATS.flatMap((id) => [`format.${FORMAT_KEY_PARTS[id]}`]),
  ...Object.keys(PAPER_DIMENSIONS_MM).map(
    (id) => `paper.${PAPER_KEY_PARTS[id as keyof typeof PAPER_KEY_PARTS]}`,
  ),
  ...["en", "es", "es-ar"].map((key) => `language.${key}`),
  ...IMAGE_ROLE_KEY_PARTS.map((key) => `imageRole.${key}`),
  ...FOCAL_KEY_PARTS.map((key) => `focal.${key}`),
  ...PRINT_TYPOGRAPHY_PERSONALITIES.map((key) => `typography.${key}`),
  ...PRINT_CONTACT_PLACEMENTS.map((key) => `contact.${key}`),
  ...RECOMMENDATION_KEYS.map((key) => `recommendation.${key}`),
  ...["complete", "blocked", "timeout", "superseded", "error"].map(
    (key) => `printStatus.${key}`,
  ),
  "pageEstimate.label",
  "pageEstimate.pending",
  "logo.wordmark",
  "logo.contained",
  "logo.markOnly",
  "logo.hidden",
  "palette.ground",
  "palette.ink",
  "palette.accent",
  "palette.muted",
  "ornament.none",
  "ornament.restrained",
  "ornament.expressive",
  "cover.none",
  "cover.typographic",
  "cover.photographic",
  "compatibility.streetTypeLedUnavailable",
  "compatibility.galleryTypeLedUnavailable",
  "compatibility.galleryCompactUnavailable",
  "overlay.safeArea",
  "overlay.trimGuide",
  "overlay.folds",
  "printBlocked.title",
  "printBlocked.message",
  "printBlocked.retry",
  "diagnostics.lowResolutionImage",
] as const;

function sourceFiles(root: string): string[] {
  return fs.readdirSync(root, { withFileTypes: true }).flatMap((entry) => {
    const absolute = path.join(root, entry.name);
    if (entry.isDirectory()) {
      if (entry.name === "__tests__" || entry.name === "templates") return [];
      return sourceFiles(absolute);
    }
    return /\.(?:ts|tsx)$/.test(entry.name) && !entry.name.includes(".test.")
      ? [absolute]
      : [];
  });
}

function staticallyReferencedKeys(): string[] {
  const roots = [
    path.resolve(__dirname, "../../../components/business/MenuBuilder/print"),
    path.resolve(__dirname, "../../../lib/menuPrint"),
  ];
  const exactReference = /["'`](print\.[A-Za-z0-9.-]+)["'`]/g;
  const keys = new Set<string>();
  for (const file of roots.flatMap(sourceFiles)) {
    for (const match of fs
      .readFileSync(file, "utf8")
      .matchAll(exactReference)) {
      keys.add(match[1].slice("print.".length));
    }
  }
  return [...keys].sort();
}

describe("restaurant Print Menu operator copy", () => {
  const english = printTree(en as MessageTree);
  const spanish = printTree(es as MessageTree);
  const argentineOverrides = printTree(esAr as MessageTree);

  it("localizes every static reference and dynamic design-system choice", () => {
    const required = new Set([
      ...staticallyReferencedKeys(),
      ...requiredDynamicKeys,
    ]);

    for (const key of required) {
      expect(readKey(english, key)).toEqual(expect.any(String));
      expect(readKey(spanish, key)).toEqual(expect.any(String));
      expect((readKey(english, key) as string).trim()).not.toBe("");
      expect((readKey(spanish, key) as string).trim()).not.toBe("");
    }
  });

  it("keeps the complete English and Spanish print trees in structural parity", () => {
    expect(Object.keys(flattenStrings(spanish)).sort()).toEqual(
      Object.keys(flattenStrings(english)).sort(),
    );
  });

  it("removes visible legacy template and studio language", () => {
    expect(Object.keys(flattenStrings(english))).not.toEqual(
      expect.arrayContaining([
        expect.stringMatching(/(?:^|\.)(?:template|templatesLabel)(?:\.|$)/),
      ]),
    );
    expect(JSON.stringify(english)).not.toMatch(/\b(?:studio|template)\b/i);
    expect(JSON.stringify(spanish)).not.toMatch(/\b(?:estudio|plantilla)\b/i);
    expect(readKey(english, "buttonAria")).toBe("Design and print this menu");
  });

  it("keeps es-AR as a small, genuinely different voseo override layer", () => {
    const overrides = flattenStrings(argentineOverrides);
    expect(Object.keys(overrides).sort()).toEqual([
      "back",
      "diagnostics.imageEffectiveDpi",
      "diagnostics.imageFailed",
      "diagnostics.imageUnplaced",
      "diagnostics.photographicCoverNeedsImage",
      "emptyMenu",
      "format.help",
      "guidance.review",
      "guidance.savePdf",
      "language.retry",
      "look.help",
      "modalSubtitle",
      "modalTitle",
      "print",
      "printBlocked.message",
      "printBlocked.retry",
      "printStatus.blocked",
      "qr.caption",
      "stage.look",
      "stage.personalize",
      "treatment.help",
    ]);
    for (const [key, value] of Object.entries(overrides)) {
      expect(value).not.toBe(readKey(spanish, key));
    }
    expect(Object.values(overrides).join(" ")).toMatch(/Elegí/);
    expect(Object.values(overrides).join(" ")).toMatch(/Revisá/);
    expect(Object.values(overrides).join(" ")).toMatch(/Imprimí/);
    expect(Object.values(overrides).join(" ")).toMatch(/Volvé/);
    expect(Object.values(overrides).join(" ")).toMatch(/Probá de nuevo/);
  });
});
