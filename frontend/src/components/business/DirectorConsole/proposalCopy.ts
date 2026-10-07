/**
 * L4-18 — Prefer operator-locale copy over English Go string builders.
 * When the operator locale is Spanish (or other non-en), rebuild a short title
 * from the already-localized kind label + affected count rather than pasting
 * server English prose into the card.
 *
 * Preview summaries and warnings are also rebuilt from structured fields
 * (affected_count / kind) so Spanish operators never see raw Go fmt.Sprintf
 * prose like "3 item(s) → unavailable." or "No items matched scope …".
 */

import type { DirectorProposedAction } from "@/api/directorConsole";
import { countKey } from "@/i18n/countForm";

export function isEnglishLocale(locale: string): boolean {
  const base = locale.toLowerCase().split("-")[0];
  return base === "en";
}

export function localizedProposalTitle(
  proposal: Pick<DirectorProposedAction, "kind" | "title" | "preview">,
  locale: string,
  t: (key: string, params?: Record<string, string | number>) => string,
): string {
  if (isEnglishLocale(locale)) return proposal.title;
  const kindKey = `proposal.kinds.${proposal.kind.replace(/\./g, "_")}`;
  const kindLabel = t(kindKey);
  const count = proposal.preview?.affected_count ?? 0;
  if (count > 0) {
    return t(countKey("proposal.titleWithCount", count), {
      kind: kindLabel,
      count,
    });
  }
  return kindLabel !== kindKey ? kindLabel : proposal.title;
}

export function localizedProposalDescription(
  proposal: Pick<DirectorProposedAction, "description" | "preview">,
  _locale: string,
  t: (key: string, params?: Record<string, string | number>) => string,
): string {
  const count = proposal.preview?.affected_count ?? 0;
  if (count === 0) return t("proposal.descNoneAffected");
  return t(countKey("proposal.descAffected", count), { count });
}

/**
 * Localize preview.summary from affected_count. Exact count 1 uses `_one`
 * so operators never see "1 item(s)" / "1 elemento(s)".
 */
export function localizedProposalSummary(
  proposal: Pick<DirectorProposedAction, "preview" | "kind">,
  _locale: string,
  t: (key: string, params?: Record<string, string | number>) => string,
): string {
  const count = proposal.preview?.affected_count ?? 0;
  if (count === 0) return t("proposal.summaryNone");
  return t(countKey("proposal.summaryAffected", count), { count });
}

/**
 * Localize a single warning string. Known Go templates map to i18n keys;
 * unknown warnings are suppressed for non-English locales (better empty than
 * English leak) and shown raw for English.
 */
export function localizedProposalWarning(
  warning: string,
  locale: string,
  t: (key: string, params?: Record<string, string | number>) => string,
): string {
  if (isEnglishLocale(locale)) return warning;
  const w = warning.trim();
  // "No items matched scope "…"" / "No items matched target "…""
  if (/^No items matched (scope|target)/i.test(w)) {
    return t("proposal.warnings.noMatch");
  }
  // "Large swing …" style — generic large-change caution
  if (/large swing/i.test(w) || /large change/i.test(w)) {
    return t("proposal.warnings.largeChange");
  }
  // Fallback: do not paint raw English into the Spanish UI.
  return t("proposal.warnings.generic");
}
