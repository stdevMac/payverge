"use client";

import React from "react";
import { Button } from "@nextui-org/react";
import { getMenu, type Business } from "@/api/business";
import { btnSecondaryNextUI } from "@/components/ui/buttonStyles";
import { PremiumPanel } from "../premium";
import { photoAnalysisFor } from "./photo/cache";
import { readinessFor, type PhotoReadiness } from "./photo/readiness";
import { loadOptionalImage } from "./templates/renderPost";

/**
 * How many photos are scored.
 *
 * Each one costs a decode plus a 64×64 analysis, both memoized per URL, and the
 * panel is a diagnostic rather than an inventory: an operator with 200 dishes
 * needs to know the shape of the problem, not to page through it. The cap is
 * applied after ordering by menu position, so it is the top of the menu — the
 * dishes most likely to be promoted — that gets checked.
 */
const MAX_PHOTOS = 24;

interface Row {
  name: string;
  url: string;
  readiness: PhotoReadiness;
}

interface Props {
  business: Business;
  t: (key: string, params?: Record<string, string | number>) => string;
  /** Deep-link into Menu Builder focused on a dish that needs a better photo. */
  onOpenMenuItem?: (itemName: string) => void;
}

const VERDICT_CLASS: Record<PhotoReadiness["verdict"], string> = {
  ready: "bg-brand/10 text-brand",
  weak: "bg-warm-200 text-ink-700",
  reshoot: "bg-rose-50 text-rose-700",
};

/** Menu items carry `image` (legacy) and `images` (current); take the first. */
function firstImage(item: { image?: string; images?: string[] }): string {
  const fromList = item.images?.find((url) => url?.trim());
  return (fromList ?? item.image ?? "").trim();
}

export function PhotoReadinessPanel({ business, t, onOpenMenuItem }: Props) {
  const [status, setStatus] = React.useState<"loading" | "ready" | "failed">(
    "loading",
  );
  const [rows, setRows] = React.useState<Row[]>([]);
  const [attempt, setAttempt] = React.useState(0);

  React.useEffect(() => {
    const controller = new AbortController();
    let cancelled = false;
    setStatus("loading");

    void (async () => {
      let candidates: Array<{ name: string; url: string }>;
      try {
        const menu = await getMenu(business.id);
        // `categories` can be a JSON string on some responses; prefer the
        // already-parsed path when the backend supplies it.
        const categories = Array.isArray(menu.parsed_categories)
          ? menu.parsed_categories
          : Array.isArray(menu.categories)
            ? menu.categories
            : [];
        candidates = categories
          .flatMap((category) => category.items ?? [])
          .map((item) => ({ name: item.name, url: firstImage(item) }))
          .filter((candidate) => candidate.url.length > 0)
          .slice(0, MAX_PHOTOS);
      } catch {
        if (!cancelled) setStatus("failed");
        return;
      }
      if (cancelled) return;

      // Sequential, deliberately. Each decode is memoized by URL in the shared
      // image cache, so this competes with the Library's own thumbnail renders
      // for the same entries; firing 24 loads at once would evict them out from
      // under the grid that is painting.
      const scored: Row[] = [];
      for (const candidate of candidates) {
        if (cancelled) return;
        let image: HTMLImageElement | null = null;
        try {
          image = await loadOptionalImage(
            candidate.url,
            "photo",
            controller.signal,
          );
        } catch {
          return; // AbortError: the panel unmounted.
        }
        scored.push({
          ...candidate,
          // A photo that would not load or would not read back scores
          // "unreadable" rather than being skipped — a library the app cannot
          // see is the finding, not an absence of findings.
          readiness: readinessFor(photoAnalysisFor(candidate.url, image)),
        });
      }
      if (cancelled) return;
      setRows(scored);
      setStatus("ready");
    })();

    return () => {
      cancelled = true;
      controller.abort();
    };
  }, [business.id, attempt]);

  const readyCount = rows.filter(
    (row) => row.readiness.verdict === "ready",
  ).length;

  return (
    <PremiumPanel tone="default" className="p-6">
      <h3 className="text-base font-semibold text-ink-900">
        {t("photoReadiness.heading")}
      </h3>
      <p className="mt-1 text-sm text-ink-500">
        {t("photoReadiness.subheading")}
      </p>

      {status === "loading" ? (
        <p className="mt-4 text-sm text-ink-500">{t("photoReadiness.loading")}</p>
      ) : null}

      {status === "failed" ? (
        <div className="mt-4 flex items-center gap-3">
          <p className="text-sm text-ink-600">{t("photoReadiness.failed")}</p>
          <Button
            className={btnSecondaryNextUI}
            radius="full"
            size="sm"
            onPress={() => setAttempt((value) => value + 1)}
          >
            {t("photoReadiness.retry")}
          </Button>
        </div>
      ) : null}

      {status === "ready" && rows.length === 0 ? (
        <p className="mt-4 text-sm text-ink-500">{t("photoReadiness.empty")}</p>
      ) : null}

      {status === "ready" && rows.length > 0 ? (
        <>
          <p className="mt-4 text-sm font-medium text-ink-700">
            {t("photoReadiness.summary", {
              ready: readyCount,
              total: rows.length,
            })}
          </p>
          <ul className="mt-3 divide-y divide-warm-200">
            {rows.map((row) => (
              <li
                key={`${row.name}:${row.url}`}
                className="flex flex-wrap items-center gap-2 py-2"
              >
                <span className="min-w-0 flex-1 truncate text-sm text-ink-800">
                  {row.name}
                </span>
                <span
                  className={`rounded-full px-2 py-0.5 text-xs font-semibold ${VERDICT_CLASS[row.readiness.verdict]}`}
                >
                  {t(`photoReadiness.verdicts.${row.readiness.verdict}`)}
                </span>
                {row.readiness.reasons.map((reason) => (
                  <span
                    key={reason}
                    className="rounded-full bg-warm-100 px-2 py-0.5 text-xs text-ink-600"
                  >
                    {t(`photoReadiness.reasons.${reason}`)}
                  </span>
                ))}
                <p
                  className="basis-full text-xs leading-5 text-ink-500"
                  data-testid="photo-readiness-explain"
                >
                  {t(`photoReadiness.explain.${row.readiness.verdict}`)}
                  {row.readiness.reasons.includes("underexposed")
                    ? ` ${t("photoReadiness.explain.underexposed")}`
                    : row.readiness.reasons.includes("crushedShadows")
                      ? ` ${t("photoReadiness.explain.crushedShadows")}`
                      : ""}
                </p>
                {row.readiness.verdict !== "ready" && onOpenMenuItem ? (
                  <Button
                    className={btnSecondaryNextUI}
                    radius="full"
                    size="sm"
                    onPress={() => onOpenMenuItem(row.name)}
                  >
                    {row.readiness.reasons.includes("underexposed") ||
                    row.readiness.reasons.includes("crushedShadows")
                      ? t("photoReadiness.replacePhoto")
                      : t("photoReadiness.openInMenu")}
                  </Button>
                ) : null}
              </li>
            ))}
          </ul>
        </>
      ) : null}
    </PremiumPanel>
  );
}
