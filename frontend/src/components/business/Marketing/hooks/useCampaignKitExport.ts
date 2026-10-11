"use client";

import { useCallback, useState } from "react";
import { attributionFileText, type AttributionTag } from "../attribution";
import { kitRenderPlan, type KitRenderEntry } from "../formats/campaignKit";
import type { FormatId } from "../formats/formats";
import {
  DEFAULT_NARRATIVE_ROLES,
  NARRATIVE_README_DEFAULT,
  buildNarrativeManifest,
  narrativeRenderPlan,
  type NarrativeCaptionAngle,
  type NarrativeRoleId,
  type NarrativeRenderEntry,
} from "../formats/narrativeRoles";
import {
  assertPrintPresetValid,
  formatForPrintPreset,
  isPrintPresetId,
  printPresetFor,
  printPresetRasterWidth,
  type PrintPresetId,
} from "../formats/printPresets";
import { renderScenePrintHtml } from "../formats/printWalker";
import { buildZip, type ZipEntry } from "../formats/zip";
import { guestUrlToQrDataUrl, isPublicGuestUrl } from "../guestLinks";
import { buildPostFilename, downloadBlob } from "../postContent";
import { renderPostScene, renderPostToBlob } from "../templates/renderPost";
import type { RenderPostInput } from "../templates/renderPost";

/**
 * Render a campaign kit and hand the operator one file.
 *
 * `"use client"` because it creates DOM nodes and triggers a download. The
 * modules it orchestrates — `campaignKit`, `narrativeRoles`, `printWalker`,
 * `printPresets`, `zip` — are all pure, which is the same split `useQrSheetPrint`
 * uses over `qrSheetHtml`: a pure builder plus a thin client hook that owns the
 * transport.
 *
 * SEQUENTIAL, deliberately. Six formats at full resolution is up to about 40 MB
 * of live canvas backing store — Safari refuses large canvas allocations well
 * before that, and `renderPostToBlob` only releases a canvas after `toBlob`
 * resolves. Rendering one at a time bounds peak memory to a single canvas, and
 * the shared image cache in renderPost.ts (IMAGE_CACHE_LIMIT 8) means the photo
 * and the logo are decoded once and reused across the whole kit either way, so
 * parallelism would buy latency and cost memory.
 *
 * LAZY, equally deliberately. Nothing here runs on feed render or on drawer
 * open; it runs on an explicit export. `templates/renderPost.perf.test.ts` holds
 * a 6ms per-CARD budget for the thumbnail grid, and a six-format fan-out is
 * roughly six times one card's work — fine for a button press, ruinous for a
 * scroll.
 *
 * Narrative pack (S2-C) is the same render path with role-named members, a
 * README that says post when ready, and a schedule-free manifest. No second
 * renderer.
 *
 * S3-Reach: optional AttributionTag stamps promo codes into QR payloads and
 * pack text files. Print-shop presets export HTML@300dpi or high-dpi PNG packs
 * with print-instructions.txt. Native social publish is intentionally absent.
 */

export interface KitExportRequest {
  base: RenderPostInput;
  formats: readonly FormatId[];
  caption: string;
  /** Used for the archive name and every member's filename stem. */
  targetName: string;
  /** Absolute origin for @font-face URLs in the print member. */
  origin: string;
  /** BCP 47 tag for the print member's `lang`. */
  lang: string;
  /**
   * Optional public guest deep link for print QR (S2-D). Prefer an
   * already-attributed URL (pv_ref / pv_mkt) from attachAttributionQuery.
   */
  guestUrl?: string;
  /** Optional promo / correlation tag for pack files + print badge (S3-Reach). */
  attribution?: AttributionTag;
}

/**
 * Narrative campaign pack: one concept → role assets (teaser/hero/story/tent/
 * email strip) in a single zip, immediately. Caption angles are text-only.
 * Never includes schedule metadata.
 */
export interface NarrativeKitExportRequest {
  base: RenderPostInput;
  caption: string;
  /** Optional extra caption angles (urgency, social, …). Caption-only. */
  captionAngles?: readonly NarrativeCaptionAngle[];
  /** Defaults to the full five-role pack. */
  roles?: readonly NarrativeRoleId[];
  targetName: string;
  origin: string;
  lang: string;
  /**
   * Translated README body. Defaults to English `NARRATIVE_README_DEFAULT`
   * when omitted — UI should pass `t("narrativeKit.readme")`.
   */
  readmeText?: string;
  /**
   * Translated one-line note for manifest.json (`narrativeKit.manifestNote`).
   */
  manifestNote?: string;
  /** Optional public guest deep link → guest-link.txt + qr.png when valid. */
  guestUrl?: string;
  attribution?: AttributionTag;
}

/**
 * Print-shop pack (S3-E): one preset → HTML or high-dpi PNG + instructions +
 * optional attribution. Never a thermal receipt. Never a schedule.
 */
export interface PrintShopExportRequest {
  base: RenderPostInput;
  presetId: PrintPresetId;
  caption: string;
  targetName: string;
  origin: string;
  lang: string;
  guestUrl?: string;
  attribution?: AttributionTag;
  /**
   * Optional translated print-instructions body. Defaults to the preset's
   * English defaultInstructions when omitted.
   */
  instructionsText?: string;
}

export interface KitExportProgress {
  done: number;
  total: number;
  current: FormatId;
  /** Set when exporting a narrative pack so the UI can name the role. */
  roleId?: NarrativeRoleId;
}

export type KitExportError =
  | "kit_export_failed"
  | "no_formats"
  | "invalid_print_preset";

function archiveName(
  targetName: string,
  kind: "campaign" | "narrative" | "print-shop",
  presetId?: string,
): string {
  const stem = (targetName || "post").replace(/\s+/g, "-").toLowerCase();
  if (kind === "narrative") return `${stem}-narrative-kit.zip`;
  if (kind === "print-shop") {
    const preset = (presetId || "print").replace(/_/g, "-");
    return `${stem}-${preset}-print-shop.zip`;
  }
  return `${stem}-campaign-kit.zip`;
}

/** Blob → bytes. Prefer arrayBuffer; fall back to FileReader for older runtimes. */
async function blobToBytes(blob: Blob): Promise<Uint8Array> {
  if (typeof blob.arrayBuffer === "function") {
    return new Uint8Array(await blob.arrayBuffer());
  }
  return new Promise((resolve, reject) => {
    const reader = new FileReader();
    reader.onload = () => {
      const result = reader.result;
      if (result instanceof ArrayBuffer) resolve(new Uint8Array(result));
      else reject(new Error("blob_read_failed"));
    };
    reader.onerror = () =>
      reject(reader.error ?? new Error("blob_read_failed"));
    reader.readAsArrayBuffer(blob);
  });
}

function downloadArchive(bytes: Uint8Array, filename: string): void {
  // Copied into a fresh ArrayBuffer so the Blob does not retain the writer's
  // buffer, which is up to the size of the whole kit. Lifetime of the object
  // URL is owned by downloadBlob — do not revoke here.
  downloadBlob(
    new Blob([bytes.slice()], { type: "application/zip" }),
    filename,
  );
}

type PlanEntry = KitRenderEntry | NarrativeRenderEntry;

function roleOf(entry: PlanEntry): NarrativeRoleId | undefined {
  return "role" in entry ? entry.role.id : undefined;
}

async function renderPlanToMembers(args: {
  plan: readonly PlanEntry[];
  request: {
    origin: string;
    targetName: string;
    lang: string;
    guestUrl?: string;
    qrDataUrl?: string | null;
    promoCode?: string;
  };
  setProgress: (p: KitExportProgress | null) => void;
}): Promise<{ members: ZipEntry[]; failures: FormatId[] }> {
  const failures: FormatId[] = [];
  const members: ZipEntry[] = [];
  const encoder = new TextEncoder();
  const guestUrl = isPublicGuestUrl(args.request.guestUrl)
    ? args.request.guestUrl!.trim()
    : undefined;
  const qrDataUrl = guestUrl
    ? (args.request.qrDataUrl ?? undefined)
    : undefined;
  const promoCode = (args.request.promoCode ?? "").trim() || undefined;

  for (let index = 0; index < args.plan.length; index += 1) {
    const entry = args.plan[index];
    args.setProgress({
      done: index,
      total: args.plan.length,
      current: entry.format.id,
      roleId: roleOf(entry),
    });
    try {
      if (entry.format.medium === "print") {
        const scene = await renderPostScene(entry.input);
        members.push({
          name: entry.filename,
          data: encoder.encode(
            renderScenePrintHtml(scene, entry.format, {
              origin: args.request.origin,
              title: args.request.targetName,
              lang: args.request.lang,
              guestUrl,
              qrDataUrl: qrDataUrl || undefined,
              promoCode,
            }),
          ),
        });
      } else {
        const blob = await renderPostToBlob(entry.input);
        members.push({
          name: entry.filename,
          data: await blobToBytes(blob),
        });
      }
    } catch (cause) {
      // One format failing — a CORS miss, a canvas allocation refusal — must
      // not cost the operator the other five. The failures are surfaced rather
      // than swallowed so the UI can name them.
      if (cause instanceof DOMException && cause.name === "AbortError")
        throw cause;
      failures.push(entry.format.id);
    }
  }

  return { members, failures };
}

/** Append guest-link.txt + qr.png when a public guest URL is known (S2-D). */
async function appendGuestLinkMembers(
  members: ZipEntry[],
  encoder: TextEncoder,
  guestUrl?: string,
): Promise<void> {
  if (!isPublicGuestUrl(guestUrl)) return;
  const url = guestUrl!.trim();
  members.push({ name: "guest-link.txt", data: encoder.encode(url + "\n") });
  const qrDataUrl = await guestUrlToQrDataUrl(url, 512);
  if (!qrDataUrl) return;
  const comma = qrDataUrl.indexOf(",");
  if (comma < 0) return;
  const b64 = qrDataUrl.slice(comma + 1);
  try {
    const binary = atob(b64);
    const bytes = new Uint8Array(binary.length);
    for (let i = 0; i < binary.length; i += 1) bytes[i] = binary.charCodeAt(i);
    members.push({ name: "qr.png", data: bytes });
  } catch {
    // QR optional — link text still present.
  }
}

/** Append attribution.txt when a promo code is known (S3-Reach). */
function appendAttributionMember(
  members: ZipEntry[],
  encoder: TextEncoder,
  attribution?: AttributionTag,
  guestUrl?: string,
): void {
  if (!attribution?.promoCode?.trim()) return;
  const text = attributionFileText({
    tag: attribution,
    attributedUrl: isPublicGuestUrl(guestUrl) ? guestUrl!.trim() : null,
  });
  members.push({ name: "attribution.txt", data: encoder.encode(text) });
}

function appendCaptionMembers(
  members: ZipEntry[],
  encoder: TextEncoder,
  caption: string,
  captionAngles?: readonly NarrativeCaptionAngle[],
): void {
  const primary = caption.trim();
  if (primary) {
    members.push({ name: "caption.txt", data: encoder.encode(primary) });
  }
  for (const angle of captionAngles ?? []) {
    const id =
      angle.id.replace(/[^a-zA-Z0-9_-]+/g, "-").toLowerCase() || "angle";
    const text = angle.text.trim();
    if (!text) continue;
    if (id === "primary" && primary) continue;
    members.push({
      name: `caption-${id}.txt`,
      data: encoder.encode(text),
    });
  }
}

export function useCampaignKitExport() {
  const [progress, setProgress] = useState<KitExportProgress | null>(null);
  const [failedFormats, setFailedFormats] = useState<FormatId[]>([]);
  const [error, setError] = useState<KitExportError | null>(null);

  const exportKit = useCallback(async (request: KitExportRequest) => {
    const plan = kitRenderPlan(
      request.base,
      request.formats,
      request.targetName,
    );
    if (!plan.length) {
      setError("no_formats");
      return false;
    }

    setError(null);
    setFailedFormats([]);
    try {
      const qrDataUrl = isPublicGuestUrl(request.guestUrl)
        ? await guestUrlToQrDataUrl(request.guestUrl)
        : null;
      const { members, failures } = await renderPlanToMembers({
        plan,
        request: {
          ...request,
          qrDataUrl,
          promoCode: request.attribution?.promoCode,
        },
        setProgress,
      });

      setProgress(null);
      setFailedFormats(failures);

      if (!members.length) {
        setError("kit_export_failed");
        return false;
      }

      const encoder = new TextEncoder();
      appendCaptionMembers(members, encoder, request.caption);
      await appendGuestLinkMembers(members, encoder, request.guestUrl);
      appendAttributionMember(
        members,
        encoder,
        request.attribution,
        request.guestUrl,
      );

      downloadArchive(
        buildZip(members),
        archiveName(request.targetName, "campaign"),
      );
      return true;
    } catch (cause) {
      setProgress(null);
      if (cause instanceof DOMException && cause.name === "AbortError") {
        throw cause;
      }
      setError("kit_export_failed");
      return false;
    }
  }, []);

  /**
   * Export the narrative role pack (S2-C): teaser / hero / story / tent /
   * email strip in one zip, with README + schedule-free manifest.
   */
  const exportNarrativeKit = useCallback(
    async (request: NarrativeKitExportRequest) => {
      const roles = request.roles?.length
        ? request.roles
        : DEFAULT_NARRATIVE_ROLES;
      const plan = narrativeRenderPlan(request.base, roles, request.targetName);
      if (!plan.length) {
        setError("no_formats");
        return false;
      }

      setError(null);
      setFailedFormats([]);
      try {
        const qrDataUrl = isPublicGuestUrl(request.guestUrl)
          ? await guestUrlToQrDataUrl(request.guestUrl)
          : null;
        const { members, failures } = await renderPlanToMembers({
          plan,
          request: {
            ...request,
            qrDataUrl,
            promoCode: request.attribution?.promoCode,
          },
          setProgress,
        });

        setProgress(null);
        setFailedFormats(failures);

        if (!members.length) {
          setError("kit_export_failed");
          return false;
        }

        const encoder = new TextEncoder();
        appendCaptionMembers(
          members,
          encoder,
          request.caption,
          request.captionAngles,
        );
        await appendGuestLinkMembers(members, encoder, request.guestUrl);
        appendAttributionMember(
          members,
          encoder,
          request.attribution,
          request.guestUrl,
        );

        const readme = (request.readmeText ?? NARRATIVE_README_DEFAULT).trim();
        members.push({
          name: "README.txt",
          data: encoder.encode(readme + "\n"),
        });

        const note = (
          request.manifestNote ?? "Assets ready now — you post when you want."
        ).trim();
        const manifest = buildNarrativeManifest({
          targetName: request.targetName,
          roles,
          caption: request.caption,
          captionAngles: request.captionAngles,
          note,
        });
        // Only successful visual members should be reflected? Manifest lists the
        // intended pack (roles requested); operators still get README even if a
        // single format failed. Filename list on disk is the membership source.
        members.push({
          name: "manifest.json",
          data: encoder.encode(`${JSON.stringify(manifest, null, 2)}\n`),
        });

        downloadArchive(
          buildZip(members),
          archiveName(request.targetName, "narrative"),
        );
        return true;
      } catch (cause) {
        setProgress(null);
        if (cause instanceof DOMException && cause.name === "AbortError") {
          throw cause;
        }
        setError("kit_export_failed");
        return false;
      }
    },
    [],
  );

  /**
   * Print-shop pack (S3-E / S3-Reach): table tent HTML@300dpi, window cling
   * high-dpi PNG, or flyer high-dpi PNG + print-instructions + attribution.
   * Thermal receipt branding is out of scope.
   */
  const exportPrintShopPack = useCallback(
    async (request: PrintShopExportRequest) => {
      if (!isPrintPresetId(request.presetId)) {
        setError("invalid_print_preset");
        return false;
      }
      const preset = printPresetFor(request.presetId);
      if (!preset || assertPrintPresetValid(preset) !== null) {
        setError("invalid_print_preset");
        return false;
      }

      setError(null);
      setFailedFormats([]);
      setProgress({
        done: 0,
        total: 1,
        current: preset.formatId,
      });

      const encoder = new TextEncoder();
      const members: ZipEntry[] = [];
      const format = formatForPrintPreset(preset);
      const guestUrl = isPublicGuestUrl(request.guestUrl)
        ? request.guestUrl!.trim()
        : undefined;
      const promoCode = request.attribution?.promoCode?.trim() || undefined;

      try {
        const qrDataUrl = guestUrl ? await guestUrlToQrDataUrl(guestUrl) : null;
        if (preset.exportKind === "print_html") {
          const scene = await renderPostScene({
            ...request.base,
            aspect: preset.formatId,
          });
          const filename = buildPostFilename(
            request.targetName,
            preset.formatId,
            "html",
          );
          members.push({
            name: filename,
            data: encoder.encode(
              renderScenePrintHtml(scene, format, {
                origin: request.origin,
                title: request.targetName,
                lang: request.lang,
                guestUrl,
                qrDataUrl: qrDataUrl || undefined,
                promoCode,
              }),
            ),
          });
        } else {
          const targetWidth = printPresetRasterWidth(preset);
          const blob = await renderPostToBlob(
            { ...request.base, aspect: preset.formatId },
            targetWidth != null ? { targetWidth } : undefined,
          );
          const filename = buildPostFilename(
            request.targetName,
            preset.formatId,
            "png",
          );
          // High-dpi marker in the archive so shops can tell 2× from social.
          const hiName = filename.replace(/\.png$/i, "-print-hi.png");
          members.push({
            name: hiName,
            data: await blobToBytes(blob),
          });
        }

        setProgress(null);

        if (!members.length) {
          setError("kit_export_failed");
          return false;
        }

        const instructions = (
          request.instructionsText ?? preset.defaultInstructions
        ).trim();
        members.push({
          name: "print-instructions.txt",
          data: encoder.encode(instructions + "\n"),
        });

        appendCaptionMembers(members, encoder, request.caption);
        await appendGuestLinkMembers(members, encoder, guestUrl);
        appendAttributionMember(
          members,
          encoder,
          request.attribution,
          guestUrl,
        );

        downloadArchive(
          buildZip(members),
          archiveName(request.targetName, "print-shop", preset.id),
        );
        return true;
      } catch (cause) {
        setProgress(null);
        if (cause instanceof DOMException && cause.name === "AbortError") {
          throw cause;
        }
        setFailedFormats([preset.formatId]);
        setError("kit_export_failed");
        return false;
      }
    },
    [],
  );

  return {
    exportKit,
    exportNarrativeKit,
    exportPrintShopPack,
    progress,
    failedFormats,
    error,
  };
}
