import {
  PackageX,
  PackageMinus,
  Receipt,
  MessageSquare,
  TrendingUp,
  Trash2,
  Users,
  Megaphone,
  type LucideIcon,
} from "lucide-react";

import type { ProactiveInsightDTO } from "@/api/directorConsole";

export type PreShiftTone = "urgent" | "watch" | "info";

export interface PreShiftCardModel {
  id: string;
  icon: LucideIcon;
  tone: PreShiftTone;
  /** i18n key suffix under `directorConsole.preShift.cards.<copyKey>` */
  copyKey: string;
  params: Record<string, string | number>;
  /** destination dashboard tab the owner taps into to act */
  tab: string;
}

const NAME_LIMIT = 3;

function joinNameList(value: unknown): string {
  const raw = Array.isArray(value) ? (value as unknown[]) : [];
  return raw
    .filter((n): n is string => typeof n === "string")
    .slice(0, NAME_LIMIT)
    .join(", ");
}

function joinNames(params: Record<string, unknown>): string {
  return joinNameList(params.item_names);
}

function num(value: unknown): number {
  return typeof value === "number" && Number.isFinite(value) ? value : 0;
}

function str(value: unknown): string {
  return typeof value === "string" ? value : "";
}

export type InsightTranslator = (
  key: string,
  params?: Record<string, string | number>,
) => string;

const DEFAULT_STALE_MINUTES = 120;

/**
 * Parse backend (or already-localized) duration prose into minutes so Spanish
 * cards never interpolate English units like "40 days".
 */
function parseInsightDurationMinutes(raw: unknown): number {
  if (typeof raw !== "string") return 0;
  const match = raw
    .trim()
    .toLowerCase()
    .match(/^(\d+)\s+(minutes?|minutos?|hours?|horas?|days?|d[ií]as?)$/u);
  if (!match) return 0;
  const n = Number(match[1]);
  if (!Number.isFinite(n) || n < 0) return 0;
  const unit = match[2];
  if (unit.startsWith("min")) return Math.floor(n);
  if (unit.startsWith("hour") || unit.startsWith("hora")) return Math.floor(n) * 60;
  return Math.floor(n) * 1440;
}

/** Prefer structured minutes; coerce numeric strings; else parse duration prose. */
export function insightDurationMinutes(params: Record<string, unknown>): number {
  const raw = params.oldest_minutes;
  const fromOldest =
    typeof raw === "number"
      ? raw
      : typeof raw === "string" && raw.trim() !== ""
        ? Number(raw)
        : NaN;
  if (Number.isFinite(fromOldest) && fromOldest > 0) {
    return Math.floor(fromOldest);
  }
  return parseInsightDurationMinutes(params.duration);
}

/**
 * L1-23 — build a duration string from structured minutes so Spanish copy
 * never interpolates English backend prose ("40 days" / "3 hours").
 */
export function formatInsightDurationMinutes(
  minutes: number,
  t: InsightTranslator,
): string {
  const m = Math.max(0, Math.floor(minutes));
  if (m < 60) {
    return t("duration.minutes", { count: m === 0 ? 1 : m });
  }
  const hours = Math.floor(m / 60);
  if (hours < 24) {
    return t("duration.hours", { count: hours });
  }
  const days = Math.floor(hours / 24);
  return t("duration.days", { count: days });
}

function englishDurationFromMinutes(minutes: number): string {
  const m = Math.max(0, Math.floor(minutes));
  if (m < 60) return m === 1 ? "1 minute" : `${m === 0 ? 1 : m} minutes`;
  const hours = Math.floor(m / 60);
  if (hours < 24) return hours === 1 ? "1 hour" : `${hours} hours`;
  const days = Math.floor(hours / 24);
  return days === 1 ? "1 day" : `${days} days`;
}

function resolveStaleDuration(
  p: Record<string, unknown>,
  t?: InsightTranslator,
): string {
  const minutes = insightDurationMinutes(p);
  if (t) {
    return formatInsightDurationMinutes(
      minutes > 0 ? minutes : DEFAULT_STALE_MINUTES,
      t,
    );
  }
  if (minutes > 0) return englishDurationFromMinutes(minutes);
  return str(p.duration) || "2 hours";
}

/** Replace interpolatable duration with a locale-formatted unit string. */
export function localizeInsightCardParams(
  params: Record<string, string | number>,
  t: InsightTranslator,
): Record<string, string | number> {
  const minutes = insightDurationMinutes(params);
  if (!("duration" in params) && minutes <= 0) {
    return params;
  }
  return {
    ...params,
    duration: formatInsightDurationMinutes(
      minutes > 0 ? minutes : DEFAULT_STALE_MINUTES,
      t,
    ),
  };
}

export function formatInsightCardLine(
  model: Pick<PreShiftCardModel, "copyKey" | "params">,
  t: InsightTranslator,
): string {
  return t(
    `preShift.cards.${model.copyKey}`,
    localizeInsightCardParams(model.params, t),
  );
}

// The operator interpolation engine does plain {var} substitution with no ICU
// plural support, so count-bearing cards select a `.one`/`.other` suffix copy
// key here. This keeps the common count===1 trigger from rendering "1 open
// bill(s) have sat".
function pluralKey(base: string, count: number): string {
  return `${base}.${count === 1 ? "one" : "other"}`;
}

/**
 * Map a single proactive insight to a GM-voice card model.
 * Returns null for unknown types — we never invent generic boilerplate.
 */
export function toPreShiftCard(
  insight: ProactiveInsightDTO,
  t?: InsightTranslator,
): PreShiftCardModel | null {
  const tab = insight.cta?.tab || "overview";
  const p = insight.params || {};

  switch (insight.type) {
    case "inventory_out_of_stock": {
      // L1-22: negative qty is oversold ("Sobrevendido"), not "at zero".
      const hasOversold =
        p.has_oversold === true ||
        p.has_oversold === "true" ||
        num(p.oversold_count) > 0;
      const count = num(p.count);
      // R2-7: `has_oversold` is a boolean OR across the set, so a mixed set
      // (some negative, some exactly zero) claimed every listed item was
      // oversold. Split the sentence when the backend carries both groups;
      // without the split lists (older backend) the single-group copy stands.
      const oversoldCount = Math.min(
        hasOversold ? num(p.oversold_count) || count : 0,
        count,
      );
      const zeroCount = Math.max(0, count - oversoldCount);
      const oversoldNames = joinNameList(p.oversold_names);
      const zeroNames = joinNameList(p.zero_names);
      if (oversoldCount > 0 && zeroCount > 0 && oversoldNames && zeroNames) {
        return {
          id: insight.id,
          icon: PackageX,
          tone: "urgent",
          // No plural branch: a mixed set is always 2+ items, and each group's
          // count renders as a parenthetical number rather than inflected prose.
          copyKey: "outOfStockMixed",
          params: { count, oversoldCount, oversoldNames, zeroCount, zeroNames },
          tab,
        };
      }
      const base = hasOversold ? "oversold" : "outOfStock";
      return {
        id: insight.id,
        icon: PackageX,
        tone: "urgent",
        copyKey: pluralKey(base, count),
        params: { count, names: joinNames(p) },
        tab,
      };
    }
    case "inventory_low_stock":
      // Not count-branched: the backend only emits low_stock at count>=3
      // (director_console_handler.go len(lowStock)>=3 gate), so the plural
      // wording is always grammatically safe.
      return { id: insight.id, icon: PackageMinus, tone: "watch", copyKey: "lowStock", params: { count: num(p.count), names: joinNames(p) }, tab };
    case "stale_open_bills": {
      // #817: `count` is the bills as old as the rendered duration bucket;
      // `stale_count` is every bill past the 2h surfacing threshold. On the
      // demo venue those are 1 and 2 — bill 761 at ~7.4d and bill 1143 at
      // ~1.2d. "{count} bills open longer than {duration}" can carry only one
      // age, so it either smears 7 days over both checks or drops 1143 from
      // the briefing. When the two counts disagree, switch to a line that
      // reports the real total and pins the age to the oldest bill alone.
      const bucketCount = num(p.count);
      const staleTotal = num(p.stale_count);
      const mixedAges = staleTotal > bucketCount;
      return {
        id: insight.id,
        icon: Receipt,
        tone: "watch",
        copyKey: mixedAges
          ? "staleBillsOldest"
          : pluralKey("staleBills", bucketCount),
        // L1-23: prefer oldest_minutes → localized duration; never inject English.
        params: {
          count: mixedAges ? staleTotal : bucketCount,
          duration: resolveStaleDuration(p, t),
          // Structured minutes for the card renderer to re-localize with t().
          oldest_minutes: insightDurationMinutes(p) || 0,
        },
        tab,
      };
    }
    case "ai_conversations_pending":
      return { id: insight.id, icon: MessageSquare, tone: "info", copyKey: pluralKey("aiPending", num(p.count)), params: { count: num(p.count) }, tab };
    case "food_cost_high":
      return { id: insight.id, icon: TrendingUp, tone: "watch", copyKey: pluralKey("foodCostHigh", num(p.count)), params: { count: num(p.count), names: joinNames(p) }, tab };
    case "waste_high":
      return { id: insight.id, icon: Trash2, tone: "watch", copyKey: "wasteHigh", params: { amount: Math.round(num(p.amount)), ingredient: str(p.ingredient) }, tab };
    case "labor_high":
      return { id: insight.id, icon: Users, tone: "watch", copyKey: "laborHigh", params: { pct: Math.round(num(p.pct) * 100), amount: Math.round(num(p.amount)) }, tab };
    case "marketing_posts": {
      // S3-Loop: mark_posted count this period. Info tone — not ops-urgent.
      // Optional freeform channels join for copy; empty channels omit the
      // channel clause via a dedicated key branch.
      const count = num(p.count);
      const channels = Array.isArray(p.channels)
        ? (p.channels as unknown[])
            .filter((c): c is string => typeof c === "string" && c.trim().length > 0)
            .slice(0, 3)
            .join(", ")
        : "";
      const titles = Array.isArray(p.recent_titles)
        ? (p.recent_titles as unknown[])
            .filter((n): n is string => typeof n === "string")
            .slice(0, 3)
            .join(", ")
        : "";
      const base = channels ? "marketingPostsWithChannel" : "marketingPosts";
      return {
        id: insight.id,
        icon: Megaphone,
        tone: "info",
        copyKey: pluralKey(base, count),
        params: {
          count,
          days: num(p.period_days) || 7,
          channels,
          titles,
        },
        tab: tab || "marketing",
      };
    }
    default:
      return null;
  }
}

export function toPreShiftCards(
  insights: ProactiveInsightDTO[] | null | undefined,
  t?: InsightTranslator,
): PreShiftCardModel[] {
  // A fresh business's briefing serializes an empty insight set as JSON `null`
  // (Go nil slice), so tolerate null/undefined rather than crash the tab. The
  // drawer renders the FULL list — the strip caps to 5 via toBriefingChips — so
  // this no longer truncates.
  return (insights ?? [])
    .map((insight) => toPreShiftCard(insight, t))
    .filter((c): c is PreShiftCardModel => c !== null);
}

// Severity ordering for the briefing strip + drawer grouping. Lower rank sorts
// first, so urgent leads. Anything unexpected falls to the back.
const SEVERITY_RANK: Record<PreShiftTone, number> = {
  urgent: 0,
  watch: 1,
  info: 2,
};

/**
 * Stable sort of insight cards by tone (urgent → watch → info). Returns a new
 * array; never mutates the input. Ties keep their original relative order.
 */
export function sortInsightCardsBySeverity(
  cards: PreShiftCardModel[],
): PreShiftCardModel[] {
  return cards
    .map((card, index) => ({ card, index }))
    .sort((a, b) => {
      const byTone = SEVERITY_RANK[a.card.tone] - SEVERITY_RANK[b.card.tone];
      return byTone !== 0 ? byTone : a.index - b.index;
    })
    .map((entry) => entry.card);
}

/**
 * The briefing strip's chip set: the full known-insight list, severity-sorted,
 * capped to `cap` (default 5 — the approved fixed strip cap).
 */
export function toBriefingChips(
  insights: ProactiveInsightDTO[] | null | undefined,
  cap = 5,
  t?: InsightTranslator,
): PreShiftCardModel[] {
  return sortInsightCardsBySeverity(toPreShiftCards(insights, t)).slice(0, cap);
}

export function greetingKey(hour: number): "morning" | "afternoon" | "evening" {
  if (hour < 12) return "morning";
  // Evening from 17:00 venue-local — dinner service is not "afternoon".
  if (hour < 17) return "afternoon";
  return "evening";
}

// ---------------------------------------------------------------------------
// Sage Briefing — GM-voice copy selectors
//
// Pure functions that pick the right i18n template key for each briefing
// section. They encode the render contract (which is the part worth locking in
// tests); the component does the locale-aware number formatting and weaves the
// figures into the prose as emphasized inline spans. Keys live under
// `directorConsole.preShift.briefing.*` so they share the existing namespace.
// ---------------------------------------------------------------------------

/**
 * Which read template to render.
 *
 * - `learning` state → the warm "still learning your numbers" line (no
 *   pace/play/win), regardless of pulse.
 * - `pacePct == null` → the no-comparison read (today's actuals only). This is
 *   the misleading-morning-pace guard: we NEVER imply a pace without a baseline.
 * - `pacePct >= 0` → the "ahead of a typical day" read.
 * - `pacePct < 0`  → the "behind a typical day" read.
 */
export function briefingReadKey(
  state: string,
  pacePct: number | null,
): string {
  if (state === "learning") return "preShift.briefing.learning";
  if (pacePct == null) return "preShift.briefing.read.noPace";
  if (pacePct < 0) return "preShift.briefing.read.withPaceBehind";
  return "preShift.briefing.read.withPace";
}

/**
 * Which kitchen-health clause (if any) to append to the read. Each percentage
 * is an independently-nullable 0..1 fraction, so all four combinations are
 * possible; `null` means there's nothing to say (omit the clause entirely
 * rather than render an empty one).
 */
export function briefingHealthKey(
  foodPct: number | null,
  laborPct: number | null,
): string | null {
  const hasFood = foodPct != null;
  const hasLabor = laborPct != null;
  if (hasFood && hasLabor) return "preShift.briefing.health.both";
  if (hasFood) return "preShift.briefing.health.food";
  if (hasLabor) return "preShift.briefing.health.labor";
  return null;
}

/** The play sentence template, keyed by the move kind. */
export function briefingPlayKey(kind: string): string {
  return `preShift.briefing.play.${kind}`;
}

/** The CTA label template, keyed by the move kind (imperative — voseo in es-AR). */
export function briefingPlayCtaKey(kind: string): string {
  return `preShift.briefing.play.cta.${kind}`;
}

/** The win sentence template, keyed by the win kind. */
export function briefingWinKey(kind: string): string {
  return `preShift.briefing.win.${kind}`;
}

/**
 * First-name token for the serif greeting H1. Callers must pass the
 * **session** display name (staff name or user.username) — not
 * `business.owner_name`, which is fixture/stale on demo rows ("Alex Demo").
 *
 * Returns `null` when there is no clean human name (missing, wallet address
 * stand-in) so the component falls back to the nameless greeting rather than
 * inventing a field. The address guard keeps "Good evening, 0x1f3b…" out of
 * the front door.
 */
export function ownerFirstName(ownerName?: string | null): string | null {
  const trimmed = (ownerName || "").trim();
  if (!trimmed) return null;
  const first = trimmed.split(/\s+/)[0];
  if (!first) return null;
  // Reject wallet-address-shaped tokens (0x…) and anything with no letters.
  if (/^0x/i.test(first)) return null;
  if (!/\p{L}/u.test(first)) return null;
  // Reject QA / test account handles so "Good afternoon, QA." never greets a venue (#196).
  if (/^(qa|test|demo|admin|user|null|undefined)$/i.test(first)) return null;
  if (first.length <= 3 && first === first.toUpperCase()) return null;
  return first;
}
