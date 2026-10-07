/**
 * Narrative campaign roles (Season 2 — Intelligence / S2-C).
 *
 * A narrative kit is ONE approved creative concept rendered into a fixed set of
 * story roles — teaser, hero, story, tent, email strip — and downloaded as one
 * zip **now**. It is not a calendar, drip, or "post Tue/Thu" plan.
 *
 * Roles map onto the Wave 4 FormatDef registry; they do not invent a second
 * canvas. `kitRenderPlan` still produces one Scene per format via the existing
 * walkers (canvas / print). This module only names the story function of each
 * member and the archive membership contract tests pin.
 *
 * Pure data + pure helpers. No React, no canvas, no schedule fields.
 */

import type { RenderPostInput } from "../templates/renderPost";
import { COMPOSITIONS, type CompositionId } from "../composition/compositions";
import { isCompositionId } from "../composition/chooser";
import { FORMATS, isFormatId, type FormatDef, type FormatId } from "./formats";
import type { KitRenderEntry } from "./campaignKit";

/** Story function of one asset in a narrative campaign pack. */
export type NarrativeRoleId =
  | "teaser"
  | "hero"
  | "story"
  | "tent"
  | "email_strip";

export interface NarrativeRoleDef {
  id: NarrativeRoleId;
  /** Format this role renders at. One role → one FormatId; no freeform canvas. */
  formatId: FormatId;
  /** `narrativeKit.roles.<id>.name` */
  labelKey: string;
  /** `narrativeKit.roles.<id>.use` */
  useKey: string;
  /**
   * Folder-safe filename stem for the role (not the format id). Combined with
   * the format token so operators can tell *why* a file exists, not only size.
   */
  fileStem: string;
}

/**
 * Closed narrative role registry.
 *
 * Mapping rationale (destination craft, not new art direction):
 * - teaser     → 1:1 square (profile / WhatsApp / Google / teaser cut)
 * - hero       → 4:5 feed hero (default approved canvas)
 * - story      → 9:16 Stories / Reels still
 * - tent       → 5:7 table tent print (print walker)
 * - email_strip→ strip email / web footer band
 *
 * `wide` (storefront 16:9) stays available on the craft format checklist but is
 * intentionally not a narrative role — the five roles cover "post now" social
 * + table + email without bloating the default pack.
 */
export const NARRATIVE_ROLES: Record<NarrativeRoleId, NarrativeRoleDef> = {
  teaser: {
    id: "teaser",
    formatId: "1:1",
    labelKey: "narrativeKit.roles.teaser.name",
    useKey: "narrativeKit.roles.teaser.use",
    fileStem: "teaser",
  },
  hero: {
    id: "hero",
    formatId: "4:5",
    labelKey: "narrativeKit.roles.hero.name",
    useKey: "narrativeKit.roles.hero.use",
    fileStem: "hero",
  },
  story: {
    id: "story",
    formatId: "9:16",
    labelKey: "narrativeKit.roles.story.name",
    useKey: "narrativeKit.roles.story.use",
    fileStem: "story",
  },
  tent: {
    id: "tent",
    formatId: "5:7",
    labelKey: "narrativeKit.roles.tent.name",
    useKey: "narrativeKit.roles.tent.use",
    fileStem: "tent",
  },
  email_strip: {
    id: "email_strip",
    formatId: "strip",
    labelKey: "narrativeKit.roles.email_strip.name",
    useKey: "narrativeKit.roles.email_strip.use",
    fileStem: "email-strip",
  },
};

/** Display + export order. Hero first so the approved canvas leads the pack. */
export const NARRATIVE_ROLE_ORDER: readonly NarrativeRoleId[] = [
  "hero",
  "teaser",
  "story",
  "tent",
  "email_strip",
];

/**
 * Default pack: every narrative role. A separate name from the display order
 * so the default selection can narrow without reordering exports.
 * @alias
 */
export const DEFAULT_NARRATIVE_ROLES: readonly NarrativeRoleId[] =
  NARRATIVE_ROLE_ORDER;

export function isNarrativeRoleId(value: unknown): value is NarrativeRoleId {
  return (
    typeof value === "string" &&
    (NARRATIVE_ROLE_ORDER as readonly string[]).includes(value)
  );
}

/**
 * Resolve the role list for a pack. Unknown ids are dropped (forward-compat);
 * empty/absent → full default set. Order is always NARRATIVE_ROLE_ORDER for
 * the members present — operators should not invent a custom schedule order.
 */
export function resolveNarrativeRoles(
  stored: readonly string[] | undefined,
): NarrativeRoleId[] {
  if (!stored?.length) {
    return [...DEFAULT_NARRATIVE_ROLES];
  }
  const wanted = new Set<NarrativeRoleId>();
  for (const entry of stored) {
    if (isNarrativeRoleId(entry)) wanted.add(entry);
  }
  if (wanted.size === 0) return [...DEFAULT_NARRATIVE_ROLES];
  return NARRATIVE_ROLE_ORDER.filter((id) => wanted.has(id));
}

/** Formats implied by a role list (deduped, role order). */
export function formatsForNarrativeRoles(
  roles: readonly NarrativeRoleId[],
): FormatId[] {
  const out: FormatId[] = [];
  const seen = new Set<FormatId>();
  for (const roleId of roles) {
    const def = NARRATIVE_ROLES[roleId];
    if (!def || !isFormatId(def.formatId) || seen.has(def.formatId)) continue;
    seen.add(def.formatId);
    out.push(def.formatId);
  }
  return out;
}

export interface NarrativeRenderEntry extends KitRenderEntry {
  role: NarrativeRoleDef;
}

function compositionAccepts(
  composition: CompositionId | undefined,
  format: FormatDef,
): boolean {
  if (!composition || !isCompositionId(composition)) return true;
  return COMPOSITIONS[composition].supportsFormat(format);
}

/**
 * Archive member filename: `{target}-{role}-{formatToken}.{ext}`.
 * Role stem makes membership readable without opening the file.
 */
export function narrativeMemberFilename(
  targetName: string | undefined,
  role: NarrativeRoleDef,
  extension?: string,
): string {
  const format = FORMATS[role.formatId];
  const ext = extension ?? (format.medium === "print" ? "html" : "png");
  const stem = (targetName || "post").replace(/\s+/g, "-").toLowerCase();
  const formatToken = role.formatId.replace(":", "x");
  return `${stem}-${role.fileStem}-${formatToken}.${ext}`;
}

/**
 * One render entry per narrative role, varying only the format (and filename).
 * Reuses the Wave 4 kit plan contract: same kit/composition/photo/slots.
 */
export function narrativeRenderPlan(
  base: RenderPostInput,
  roles: readonly NarrativeRoleId[] = DEFAULT_NARRATIVE_ROLES,
  targetName?: string,
): NarrativeRenderEntry[] {
  const resolved = resolveNarrativeRoles(roles);
  return resolved
    .filter((roleId) => {
      const role = NARRATIVE_ROLES[roleId];
      return compositionAccepts(base.composition, FORMATS[role.formatId]);
    })
    .map((roleId) => {
      const role = NARRATIVE_ROLES[roleId];
      const format = FORMATS[role.formatId];
      return {
        role,
        format,
        input: { ...base, aspect: role.formatId },
        filename: narrativeMemberFilename(
          targetName ?? base.slots.dishName,
          role,
        ),
      };
    });
}

/** Caption angle file entry for the zip (text-only; no image generation). */
export interface NarrativeCaptionAngle {
  /** Stable id for the filename stem, e.g. "primary", "urgency". */
  id: string;
  text: string;
}

/**
 * Expected zip member names for a narrative pack — pure description for tests
 * and UI. Explicitly does NOT include schedule / due / queue names.
 */
export function narrativeZipMembership(args: {
  targetName: string;
  roles?: readonly NarrativeRoleId[];
  caption?: string;
  captionAngles?: readonly NarrativeCaptionAngle[];
  includeReadme?: boolean;
  includeManifest?: boolean;
}): string[] {
  const roles = resolveNarrativeRoles(args.roles);
  const names: string[] = [];
  for (const roleId of roles) {
    const role = NARRATIVE_ROLES[roleId];
    names.push(narrativeMemberFilename(args.targetName, role));
  }
  const caption = (args.caption ?? "").trim();
  if (caption) {
    names.push("caption.txt");
  }
  const angles = args.captionAngles ?? [];
  for (const angle of angles) {
    const id =
      angle.id.replace(/[^a-zA-Z0-9_-]+/g, "-").toLowerCase() || "angle";
    const text = angle.text.trim();
    if (!text) continue;
    // Primary caption already lives in caption.txt — skip duplicate angle id.
    if (id === "primary" && caption) continue;
    names.push(`caption-${id}.txt`);
  }
  if (args.includeReadme !== false) {
    names.push("README.txt");
  }
  if (args.includeManifest !== false) {
    names.push("manifest.json");
  }
  return names;
}

/**
 * Manifest written into the zip. Operators (and tests) can verify what shipped.
 * FORBIDDEN: any schedule / due / queue / best_time field.
 */
export interface NarrativeKitManifest {
  kind: "narrative_campaign_kit";
  /** Always immediate; never a calendar. */
  delivery: "immediate";
  /** Human reminder — not a schedule. */
  note: string;
  target_name: string;
  roles: Array<{
    id: NarrativeRoleId;
    format: FormatId;
    filename: string;
  }>;
  captions: string[];
}

/** Keys that must never appear on a narrative manifest (schedule guard). */
export const FORBIDDEN_NARRATIVE_MANIFEST_KEYS = [
  "scheduled_at",
  "scheduled_for",
  "schedule",
  "due_at",
  "dueDate",
  "due_date",
  "cadence",
  "queue",
  "best_time",
  "publish_at",
  "day_index",
  "week",
] as const;

export function buildNarrativeManifest(args: {
  targetName: string;
  roles?: readonly NarrativeRoleId[];
  caption?: string;
  captionAngles?: readonly NarrativeCaptionAngle[];
  /** Operator-facing note (already translated). */
  note: string;
}): NarrativeKitManifest {
  const roles = resolveNarrativeRoles(args.roles);
  const plan = roles.map((roleId) => {
    const role = NARRATIVE_ROLES[roleId];
    return {
      id: roleId,
      format: role.formatId,
      filename: narrativeMemberFilename(args.targetName, role),
    };
  });
  const captions: string[] = [];
  if ((args.caption ?? "").trim()) captions.push("caption.txt");
  for (const angle of args.captionAngles ?? []) {
    const id =
      angle.id.replace(/[^a-zA-Z0-9_-]+/g, "-").toLowerCase() || "angle";
    if (!angle.text.trim()) continue;
    if (id === "primary" && (args.caption ?? "").trim()) continue;
    captions.push(`caption-${id}.txt`);
  }
  return {
    kind: "narrative_campaign_kit",
    delivery: "immediate",
    note: args.note,
    target_name: args.targetName,
    roles: plan,
    captions,
  };
}

/**
 * Default README body (English fallback). UI should pass translated text via
 * `readmeText` on the export request; this is the pure-module default.
 */
export const NARRATIVE_README_DEFAULT =
  "Assets ready now — you post when you want.\n" +
  "This zip is not a schedule. There is no due date and no post queue.\n" +
  "Use each file on the channel that matches its role (hero, story, tent, …).\n";

/**
 * Snapshot may store which roles were last exported (additive). Absent =
 * legacy / format-only campaign kit, not a narrative pack default rewrite.
 */
export function narrativeRolesFromSnapshot(snapshot: {
  narrative_roles?: readonly string[];
}): NarrativeRoleId[] | null {
  if (!snapshot.narrative_roles?.length) return null;
  const resolved = resolveNarrativeRoles(snapshot.narrative_roles);
  // resolveNarrativeRoles falls back to full default on empty unknown — only
  // return a list if at least one stored id was recognised.
  const anyKnown = snapshot.narrative_roles.some((r) => isNarrativeRoleId(r));
  return anyKnown ? resolved : null;
}
