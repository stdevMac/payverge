"use client";

import React, { useRef } from "react";
import type { MenuSanitizeDecision, MenuSanitizeReport } from "@/api/business";
import { useDialogKeyboard } from "@/components/business/schedule/useDialogKeyboard";

interface MenuSanitizationReviewProps {
  report: MenuSanitizeReport;
  translate: (key: string) => string;
  onConfirm: () => void;
  onBack: () => void;
  isConfirming: boolean;
}

function decisionValue(decision: MenuSanitizeDecision): string {
  if (decision.field === "price") {
    return decision.item_name;
  }
  return decision.value;
}

export default function MenuSanitizationReview({
  report,
  translate,
  onConfirm,
  onBack,
  isConfirming,
}: MenuSanitizationReviewProps) {
  // L3-3 / Root C: Escape must dismiss the review (not a modal underneath).
  // R2-6: …but only while Back is actually available. Both footer buttons are
  // disabled during the confirm request, so Escape must be too — otherwise the
  // keyboard path unwinds the review while the write is still in flight.
  const dialogRef = useRef<HTMLElement | null>(null);
  useDialogKeyboard(dialogRef, onBack, !isConfirming);

  return (
    <section
      ref={dialogRef}
      role="alertdialog"
      aria-modal="true"
      aria-labelledby="menu-sanitization-title"
      tabIndex={-1}
      className="rounded-2xl border border-amber-300 bg-amber-50 p-5 shadow-sm outline-none"
    >
      <h3 id="menu-sanitization-title" className="text-lg font-bold text-ink-950">
        {translate("sanitization.title")}
      </h3>
      <p className="mt-1 text-sm text-ink-700">
        {translate("sanitization.description")}
      </p>

      <div className="mt-4 grid gap-4 md:grid-cols-2">
        <div>
          <h4 className="text-sm font-bold text-rose-700">
            {translate("sanitization.dropped")}
          </h4>
          <ul className="mt-2 space-y-2 text-sm text-ink-800">
            {report.dropped.map((decision, index) => (
              <li key={`${decision.field}-${decision.item_index}-${decision.value}-${index}`}>
                <span className="font-semibold">
                  {translate(`sanitization.fields.${decision.field}`)}:
                </span>{" "}
                <span>{decisionValue(decision)}</span>{" "}
                <span className="text-ink-600">
                  — {translate(`sanitization.reasons.${decision.reason}`)}
                </span>
              </li>
            ))}
          </ul>
        </div>

        <div>
          <h4 className="text-sm font-bold text-brand">
            {translate("sanitization.normalized")}
          </h4>
          <ul className="mt-2 space-y-2 text-sm text-ink-800">
            {report.retained
              .filter((decision) => decision.reason === "alias_normalized")
              .map((decision, index) => (
                <li key={`${decision.field}-${decision.item_index}-${decision.value}-${index}`}>
                  <span className="font-semibold">
                    {translate(`sanitization.fields.${decision.field}`)}:
                  </span>{" "}
                  <span>
                    {decision.value} → {decision.canonical_value}
                  </span>{" "}
                  <span className="text-ink-600">
                    — {translate(`sanitization.reasons.${decision.reason}`)}
                  </span>
                </li>
              ))}
          </ul>
        </div>
      </div>

      <div className="mt-5 flex flex-col-reverse gap-3 sm:flex-row sm:justify-end">
        <button
          type="button"
          onClick={onBack}
          disabled={isConfirming}
          className="h-11 rounded-xl border border-warm-300 bg-white px-5 font-semibold text-ink-800 disabled:opacity-50"
        >
          {translate("sanitization.back")}
        </button>
        <button
          type="button"
          onClick={onConfirm}
          disabled={isConfirming}
          className="h-11 rounded-xl bg-brand px-5 font-bold text-white disabled:opacity-50"
        >
          {translate(isConfirming ? "sanitization.confirming" : "sanitization.confirm")}
        </button>
      </div>
    </section>
  );
}
