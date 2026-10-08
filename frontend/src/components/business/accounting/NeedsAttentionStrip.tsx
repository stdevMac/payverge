"use client";

import React from "react";
import { AlertTriangle } from "lucide-react";
import {
  usePayrollRuns,
  useReceipts,
  useSummary,
} from "@/hooks/accounting/useAccountingQueries";
import { PremiumPanel } from "../premium";

export type NeedsAttentionStripProps = {
  businessId: string | number;
  start: string;
  end: string;
  t: (key: string, params?: Record<string, string | number>) => string;
  tWith: (key: string, replacements: Record<string, string | number>) => string;
  fmtMoney: (value: number, currency: string) => string;
};

type AttentionSignal = {
  id: "draft-runs" | "failed-deliveries" | "collection-gap";
  label: string;
  href: string;
  testId: string;
  reviewTestId: string;
};

/**
 * Amber "needs attention" strip for the Accounting Overview.
 * Surfaces draft payrolls, invoices needing attention, and an open collection
 * gap — each with a Review deep-link into the pre-filtered sub-tab.
 */
export default function NeedsAttentionStrip({
  businessId,
  start,
  end,
  t,
  tWith,
  fmtMoney,
}: NeedsAttentionStripProps) {
  const { data: summary } = useSummary(businessId, { start, end });

  // Lightweight envelope probes (page_size:1) — we only need the totals.
  // Scoped to the page's date range: without start/end these counted ALL-TIME
  // drafts/attention while every other Overview number is period-scoped (live
  // bug: "230 invoices need attention" on a 30-day view).
  const draftProbe = usePayrollRuns(businessId, {
    start,
    end,
    status: "draft",
    page: 1,
    page_size: 1,
  });
  const attentionProbe = useReceipts(businessId, {
    start,
    end,
    needs_attention: true,
    page: 1,
    page_size: 1,
  });

  const draftCount = draftProbe.data?.total ?? 0;
  const attentionCount = attentionProbe.data?.total ?? 0;
  const collectionGap = summary?.collection_gap ?? 0;
  const currency = summary?.currency || "USD";

  const signals: AttentionSignal[] = [];

  if (draftCount > 0) {
    signals.push({
      id: "draft-runs",
      label: tWith("attention.draftRuns", { count: draftCount }),
      href: "?tab=accounting&sub=payroll&status=draft",
      testId: "attention-draft-runs",
      reviewTestId: "attention-draft-runs-review",
    });
  }

  if (attentionCount > 0) {
    signals.push({
      id: "failed-deliveries",
      label: tWith("attention.failedDeliveries", { count: attentionCount }),
      href: "?tab=accounting&sub=invoices&filter=needs_attention",
      testId: "attention-failed-deliveries",
      reviewTestId: "attention-failed-deliveries-review",
    });
  }

  if (collectionGap > 0) {
    signals.push({
      id: "collection-gap",
      label: tWith("attention.collectionGap", {
        amount: fmtMoney(collectionGap, currency),
      }),
      // Best available Invoices seed for open fiscal work (no unpaid-bill
      // filter exists). pending = not-yet-authorized receipts in range.
      href: "?tab=accounting&sub=outstanding",
      testId: "attention-collection-gap",
      reviewTestId: "attention-collection-gap-review",
    });
  }

  if (signals.length === 0) return null;

  return (
    <PremiumPanel
      tone="urgent"
      className="p-4"
      withTexture={false}
      data-testid="needs-attention-strip"
    >
      <div className="flex items-start gap-3">
        <span className="mt-0.5 flex h-8 w-8 shrink-0 items-center justify-center rounded-xl bg-amber-100 text-amber-800">
          <AlertTriangle className="h-4 w-4" aria-hidden />
        </span>
        <div className="min-w-0 flex-1 space-y-2">
          <h3 className="text-sm font-semibold text-amber-950">
            {t("attention.title")}
          </h3>
          <ul className="space-y-1.5" role="list">
            {signals.map((signal) => (
              <li
                key={signal.id}
                data-testid={signal.testId}
                className="flex flex-wrap items-center justify-between gap-2 rounded-xl border border-amber-200/80 bg-white/70 px-3 py-2"
              >
                <span className="text-sm font-medium text-amber-950">
                  {signal.label}
                </span>
                <a
                  href={signal.href}
                  data-testid={signal.reviewTestId}
                  className="rounded text-xs font-semibold text-brand underline decoration-brand/40 underline-offset-2 transition-colors hover:text-brand-dark focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand"
                >
                  {t("attention.action.review")}
                </a>
              </li>
            ))}
          </ul>
        </div>
      </div>
    </PremiumPanel>
  );
}
