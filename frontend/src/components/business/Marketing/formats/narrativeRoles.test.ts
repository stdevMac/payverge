import type { RenderPostInput } from "../templates/renderPost";
import {
  DEFAULT_NARRATIVE_ROLES,
  FORBIDDEN_NARRATIVE_MANIFEST_KEYS,
  NARRATIVE_ROLE_ORDER,
  NARRATIVE_ROLES,
  buildNarrativeManifest,
  formatsForNarrativeRoles,
  isNarrativeRoleId,
  narrativeMemberFilename,
  narrativeRenderPlan,
  narrativeRolesFromSnapshot,
  narrativeZipMembership,
  resolveNarrativeRoles,
} from "./narrativeRoles";
import { FORMATS } from "./formats";
import { buildZip } from "./zip";

const BASE: RenderPostInput = {
  kit: "editorial",
  composition: "photoBottomStack",
  aspect: "4:5",
  photoUrl: "https://cdn.example.com/dish.jpg",
  logoUrl: "https://cdn.example.com/logo.png",
  slots: {
    badge: "CHEF'S PICK",
    dishName: "Milanesa napolitana",
    price: "$12.00",
    cta: "Order tonight",
    handle: "@casasur",
  },
  palette: { primary: "#1a6b6a", secondary: "#0f3d3c" },
  crop: { x: 0.5, y: 0.4, zoom: 1 },
};

describe("narrative role registry", () => {
  it("lists the five story roles in a stable order with hero first", () => {
    expect(NARRATIVE_ROLE_ORDER).toEqual([
      "hero",
      "teaser",
      "story",
      "tent",
      "email_strip",
    ]);
    expect(DEFAULT_NARRATIVE_ROLES).toEqual(NARRATIVE_ROLE_ORDER);
  });

  it("maps each role to exactly one known format", () => {
    const expected: Record<string, string> = {
      teaser: "1:1",
      hero: "4:5",
      story: "9:16",
      tent: "5:7",
      email_strip: "strip",
    };
    for (const id of NARRATIVE_ROLE_ORDER) {
      expect(NARRATIVE_ROLES[id].formatId).toBe(expected[id]);
      expect(FORMATS[NARRATIVE_ROLES[id].formatId]).toBeDefined();
    }
  });

  it("does not invent a freeform canvas — only FormatIds", () => {
    for (const id of NARRATIVE_ROLE_ORDER) {
      expect(Object.keys(FORMATS)).toContain(NARRATIVE_ROLES[id].formatId);
    }
  });

  it("narrows unknown ids", () => {
    expect(isNarrativeRoleId("hero")).toBe(true);
    expect(isNarrativeRoleId("scheduled")).toBe(false);
    expect(isNarrativeRoleId("wide")).toBe(false);
  });
});

describe("resolveNarrativeRoles", () => {
  it("defaults to the full set when absent or empty", () => {
    expect(resolveNarrativeRoles(undefined)).toEqual([...DEFAULT_NARRATIVE_ROLES]);
    expect(resolveNarrativeRoles([])).toEqual([...DEFAULT_NARRATIVE_ROLES]);
  });

  it("filters to known roles and reorders to registry order", () => {
    expect(resolveNarrativeRoles(["story", "hero", "story", "nope"])).toEqual([
      "hero",
      "story",
    ]);
  });

  it("falls back to default when every id is unknown", () => {
    expect(resolveNarrativeRoles(["calendar", "queue"])).toEqual([
      ...DEFAULT_NARRATIVE_ROLES,
    ]);
  });
});

describe("formatsForNarrativeRoles", () => {
  it("returns FormatIds in role order without inventing extras", () => {
    expect(formatsForNarrativeRoles(DEFAULT_NARRATIVE_ROLES)).toEqual([
      "4:5",
      "1:1",
      "9:16",
      "5:7",
      "strip",
    ]);
  });
});

describe("narrativeRenderPlan", () => {
  it("produces one entry per role, varying only the format", () => {
    const plan = narrativeRenderPlan(BASE, DEFAULT_NARRATIVE_ROLES, "Milanesa");
    expect(plan).toHaveLength(5);
    plan.forEach((entry) => {
      expect(entry.input.aspect).toBe(entry.role.formatId);
      expect(entry.input.kit).toBe(BASE.kit);
      expect(entry.input.composition).toBe(BASE.composition);
      expect(entry.input.photoUrl).toBe(BASE.photoUrl);
      expect(entry.input.slots).toEqual(BASE.slots);
      expect(entry.format.id).toBe(entry.role.formatId);
    });
  });

  it("names members with role + format token", () => {
    const plan = narrativeRenderPlan(BASE, ["hero", "tent"], "Milanesa Napolitana");
    expect(plan[0].filename).toBe("milanesa-napolitana-hero-4x5.png");
    expect(plan[1].filename).toBe("milanesa-napolitana-tent-5x7.html");
  });

  it("marks the print tent for the print walker", () => {
    const plan = narrativeRenderPlan(BASE, ["tent", "hero"]);
    const tent = plan.find((e) => e.role.id === "tent");
    expect(tent?.format.medium).toBe("print");
  });

  it("drops a role whose format the composition refuses", () => {
    const plan = narrativeRenderPlan(
      { ...BASE, composition: "legacyBold", kit: "bold" },
      DEFAULT_NARRATIVE_ROLES,
    );
    // legacyBold only supports legacy aspects (1:1 / 4:5 / 9:16).
    const formats = plan.map((e) => e.format.id);
    expect(formats).toEqual(
      expect.arrayContaining(["4:5", "1:1", "9:16"]),
    );
    expect(formats).not.toContain("5:7");
    expect(formats).not.toContain("strip");
  });
});

describe("narrativeZipMembership", () => {
  it("lists every role file + caption + readme + manifest by default", () => {
    const names = narrativeZipMembership({
      targetName: "Milanesa",
      caption: "Tonight only",
    });
    expect(names).toEqual([
      "milanesa-hero-4x5.png",
      "milanesa-teaser-1x1.png",
      "milanesa-story-9x16.png",
      "milanesa-tent-5x7.html",
      "milanesa-email-strip-strip.png",
      "caption.txt",
      "README.txt",
      "manifest.json",
    ]);
  });

  it("includes caption angle files without schedule names", () => {
    const names = narrativeZipMembership({
      targetName: "Dish",
      roles: ["hero"],
      caption: "Primary",
      captionAngles: [
        { id: "primary", text: "Primary" },
        { id: "urgency", text: "Ends tonight" },
        { id: "social", text: "Tag a friend" },
      ],
    });
    expect(names).toContain("caption.txt");
    expect(names).toContain("caption-urgency.txt");
    expect(names).toContain("caption-social.txt");
    // primary angle skipped when caption.txt already holds it
    expect(names).not.toContain("caption-primary.txt");
    for (const forbidden of [
      "schedule",
      "scheduled_for",
      "due_at",
      "queue",
      "day_index",
    ]) {
      expect(names.some((n) => n.includes(forbidden))).toBe(false);
    }
  });

  it("can omit readme/manifest when asked", () => {
    const names = narrativeZipMembership({
      targetName: "X",
      roles: ["hero"],
      includeReadme: false,
      includeManifest: false,
    });
    expect(names).toEqual(["x-hero-4x5.png"]);
  });
});

describe("buildNarrativeManifest", () => {
  it("declares immediate delivery and lists role membership", () => {
    const manifest = buildNarrativeManifest({
      targetName: "Milanesa",
      roles: ["hero", "story"],
      caption: "Hello",
      note: "Assets ready now — you post when you want.",
    });
    expect(manifest.kind).toBe("narrative_campaign_kit");
    expect(manifest.delivery).toBe("immediate");
    expect(manifest.roles).toEqual([
      {
        id: "hero",
        format: "4:5",
        filename: "milanesa-hero-4x5.png",
      },
      {
        id: "story",
        format: "9:16",
        filename: "milanesa-story-9x16.png",
      },
    ]);
    expect(manifest.captions).toEqual(["caption.txt"]);
  });

  it("never carries schedule keys on the manifest object", () => {
    const manifest = buildNarrativeManifest({
      targetName: "X",
      note: "ready",
    });
    const keys = JSON.stringify(manifest);
    for (const forbidden of FORBIDDEN_NARRATIVE_MANIFEST_KEYS) {
      expect(keys).not.toContain(forbidden);
    }
    // Structural: only the documented shape
    expect(Object.keys(manifest).sort()).toEqual(
      [
        "captions",
        "delivery",
        "kind",
        "note",
        "roles",
        "target_name",
      ].sort(),
    );
  });
});

describe("narrative zip membership round-trip with buildZip", () => {
  it("archive member names match the membership helper", () => {
    const membership = narrativeZipMembership({
      targetName: "Milanesa",
      roles: ["hero", "teaser"],
      caption: "Copy",
    });
    const encoder = new TextEncoder();
    const entries = membership.map((name) => ({
      name,
      data: encoder.encode(name === "manifest.json" ? "{}" : "x"),
    }));
    const bytes = buildZip(entries);
    const text = new TextDecoder().decode(bytes);
    for (const name of membership) {
      expect(text).toContain(name);
    }
  });
});

describe("narrativeRolesFromSnapshot", () => {
  it("returns null for legacy / absent", () => {
    expect(narrativeRolesFromSnapshot({})).toBeNull();
    expect(narrativeRolesFromSnapshot({ narrative_roles: [] })).toBeNull();
  });

  it("returns resolved list when any known role is stored", () => {
    expect(
      narrativeRolesFromSnapshot({ narrative_roles: ["story", "tent"] }),
    ).toEqual(["story", "tent"]);
  });
});

describe("narrativeMemberFilename", () => {
  it("uses role fileStem not only format", () => {
    expect(
      narrativeMemberFilename("Soup", NARRATIVE_ROLES.email_strip),
    ).toBe("soup-email-strip-strip.png");
  });
});
