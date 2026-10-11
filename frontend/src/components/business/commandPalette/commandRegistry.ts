/**
 * Pure (React-free) command model + fuzzy search for the ⌘K palette.
 *
 * Kept free of React and i18n so it can be exhaustively unit-tested: the UI
 * passes already-localized labels in, and maps `iconKey` strings to lucide
 * components on its side.
 */
import { getTabLockMeta, type AccessState } from "./tabAccess";

type CommandGroup = "recent" | "navigate" | "actions" | "records";

export interface Command {
  /** Stable unique id, e.g. "nav:bills" or "action:storefront". */
  id: string;
  group: CommandGroup;
  kind: "nav" | "action" | "record";
  label: string;
  description?: string;
  /** Extra search terms (aliases, English fallbacks) that never render. */
  keywords: string[];
  /** String key the UI maps to a lucide icon — keeps this module React-free. */
  iconKey: string;
  /** Locked by a server administrator: reachable, but shows the lock notice. */
  locked: boolean;
  /** Present for kind === "nav" (and records that jump to a tab). */
  tabKey?: string;
  /** Present for kind === "action". */
  actionKey?: string;
  /**
   * Composite tab navigation spec for record jumps, e.g.
   * `bills?billId=12` or `menu?menuSearch=Burger`. When set, runCommand should
   * pass this to setActiveTab instead of bare tabKey.
   */
  navSpec?: string;
}

export interface NavTabDef {
  key: string;
  label: string;
  description?: string;
  iconKey: string;
  keywords?: string[];
}

export interface BuildNavOptions {
  /** Staff restriction. Empty = no restriction (owner sees everything). */
  allowedTabs: string[];
  isStaffUser: boolean;
  access: AccessState;
}

/**
 * Turn the dashboard's tab registry into navigate-commands, applying the exact
 * same access rules as the sidebar:
 *   - staff-hidden (locked + staff) tabs are dropped,
 *   - `allowedTabs` (when non-empty) restricts staff to their permitted tabs,
 *   - locked tabs survive but carry `locked` for the chip.
 */
export function buildNavCommands(
  tabs: NavTabDef[],
  { allowedTabs, isStaffUser, access }: BuildNavOptions,
): Command[] {
  const restrict = allowedTabs.length > 0;
  const out: Command[] = [];

  for (const tab of tabs) {
    if (restrict && !allowedTabs.includes(tab.key)) continue;

    const meta = getTabLockMeta(tab.key, access, isStaffUser);
    if (meta.hidden) continue;

    out.push({
      id: `nav:${tab.key}`,
      group: "navigate",
      kind: "nav",
      label: tab.label,
      description: tab.description,
      keywords: tab.keywords ?? [],
      iconKey: tab.iconKey,
      locked: meta.locked,
      tabKey: tab.key,
    });
  }

  return out;
}

/** A single indexable business record for the ⌘K palette (Task 35). */
export interface RecordDef {
  /** Stable unique id, e.g. "bill:12". */
  id: string;
  /** Destination tab (must already be permitted). */
  tabKey: string;
  /** Composite setActiveTab spec with deep-link params. */
  navSpec: string;
  label: string;
  description?: string;
  keywords?: string[];
  iconKey: string;
}

export interface BuildRecordOptions {
  allowedTabs: string[];
  isStaffUser: boolean;
  access: AccessState;
  /** Cap per domain so the palette stays snappy. */
  limitPerDomain?: number;
}

/**
 * Turn live business records into searchable commands. Records whose tab is
 * staff-hidden or not in allowedTabs are dropped; locked tabs still
 * surface (with locked chip).
 */
export function buildRecordCommands(
  records: RecordDef[],
  { allowedTabs, isStaffUser, access, limitPerDomain = 40 }: BuildRecordOptions,
): Command[] {
  const restrict = allowedTabs.length > 0;
  const perDomain = new Map<string, number>();
  const out: Command[] = [];

  for (const rec of records) {
    if (restrict && !allowedTabs.includes(rec.tabKey)) continue;
    const meta = getTabLockMeta(rec.tabKey, access, isStaffUser);
    if (meta.hidden) continue;

    const count = perDomain.get(rec.tabKey) ?? 0;
    if (count >= limitPerDomain) continue;
    perDomain.set(rec.tabKey, count + 1);

    out.push({
      id: `record:${rec.id}`,
      group: "records",
      kind: "record",
      label: rec.label,
      description: rec.description,
      keywords: rec.keywords ?? [],
      iconKey: rec.iconKey,
      locked: meta.locked,
      tabKey: rec.tabKey,
      navSpec: rec.navSpec,
    });
  }

  return out;
}

// --- Fuzzy scoring -----------------------------------------------------------

const BOUNDARY = /[\s\-_/]/;

/**
 * Score one query against one text. Returns null when not even a subsequence
 * match. Higher is better. Prefix > word-boundary substring > mid substring >
 * contiguous subsequence > scattered subsequence.
 */
function fuzzyScore(query: string, text: string): number | null {
  if (!query) return 0;
  const q = query.toLowerCase();
  const t = text.toLowerCase();

  const idx = t.indexOf(q);
  if (idx === 0) return 1000 - t.length * 0.1; // prefix — strongest signal
  if (idx > 0) {
    const atBoundary = BOUNDARY.test(t[idx - 1]);
    return (atBoundary ? 800 : 550) - idx - t.length * 0.1;
  }

  // Subsequence walk.
  let ti = 0;
  let qi = 0;
  let score = 0;
  let prev = -2;
  let run = 0;
  for (; ti < t.length && qi < q.length; ti++) {
    if (t[ti] === q[qi]) {
      if (ti === prev + 1) {
        run += 1;
        score += 5 + run; // reward contiguous runs
      } else {
        run = 0;
        score += 1;
      }
      if (ti === 0 || BOUNDARY.test(t[ti - 1])) score += 4; // word-start bonus
      prev = ti;
      qi += 1;
    }
  }
  if (qi < q.length) return null; // didn't consume the whole query
  return score - t.length * 0.05;
}

/**
 * Best fuzzy score of a query against a command, weighting label highest, then
 * keyword aliases, then the description. Null when nothing matches.
 */
export function scoreCommand(query: string, command: Command): number | null {
  if (!query.trim()) return 0;

  let best: number | null = null;
  const consider = (raw: number | null, weight: number) => {
    if (raw === null) return;
    const weighted = raw * weight;
    if (best === null || weighted > best) best = weighted;
  };

  consider(fuzzyScore(query, command.label), 1);
  for (const kw of command.keywords) consider(fuzzyScore(query, kw), 0.9);
  if (command.description) consider(fuzzyScore(query, command.description), 0.55);

  return best;
}

/**
 * Filter + sort commands for a query. Empty/whitespace query returns the input
 * untouched (caller decides grouping). Otherwise best match first, with the
 * original order as a stable tiebreaker.
 */
export function rankCommands(query: string, commands: Command[]): Command[] {
  if (!query.trim()) return commands;

  const scored = commands
    .map((command, index) => ({ command, index, score: scoreCommand(query, command) }))
    .filter((s): s is { command: Command; index: number; score: number } => s.score !== null);

  scored.sort((a, b) => (b.score - a.score) || (a.index - b.index));
  return scored.map((s) => s.command);
}
