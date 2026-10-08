"use client";

import React from "react";
import { Button } from "@nextui-org/react";
import { Check } from "lucide-react";

export interface SetupStep {
  key: string;
  title: string;
  description: string;
  /** Whether this step is already satisfied (shows a check, muted). */
  done: boolean;
  /** Action for the first not-yet-done step; later steps render no button. */
  actionLabel?: string;
  onAction?: () => void;
}

interface SetupChecklistProps {
  title: string;
  subtitle: string;
  steps: SetupStep[];
  className?: string;
}

// A calm, ordered "what to do next" list for a brand-new business — NOT a
// step-of-N wizard. Every step is visible up front so the operator sees the
// whole path; only the first incomplete step is actionable, so there's exactly
// one obvious next move. Reused on both the Schedule and Team tabs at cold start.
export default function SetupChecklist({
  title,
  subtitle,
  steps,
  className = "",
}: SetupChecklistProps) {
  const currentIndex = steps.findIndex((s) => !s.done);

  return (
    <div
      className={`rounded-3xl border border-warm-200 bg-white p-6 shadow-sm shadow-warm-900/5 sm:p-8 ${className}`}
    >
      <h2 className="font-title text-lg font-semibold text-ink-950">{title}</h2>
      <p className="mt-1 text-sm text-ink-500">{subtitle}</p>

      <ol className="mt-6 space-y-3">
        {steps.map((step, i) => {
          const isCurrent = i === currentIndex;
          return (
            <li
              key={step.key}
              className={`flex items-start gap-4 rounded-2xl border p-4 transition ${
                isCurrent
                  ? "border-brand/25 bg-brand/10 shadow-sm shadow-brand/5"
                  : "border-warm-100 bg-white"
              }`}
            >
              <span
                aria-hidden="true"
                className={`mt-0.5 flex h-7 w-7 shrink-0 items-center justify-center rounded-full text-xs font-semibold ${
                  step.done
                    ? "bg-emerald-600 text-white"
                    : isCurrent
                      ? "bg-brand/15 text-brand-dark"
                      : "bg-warm-100 text-ink-400"
                }`}
              >
                {step.done ? <Check className="h-4 w-4" /> : i + 1}
              </span>

              <div className="min-w-0 flex-1">
                <p
                  className={`text-sm font-medium ${
                    step.done ? "text-ink-400" : "text-ink-950"
                  }`}
                >
                  {step.title}
                </p>
                <p className="mt-0.5 text-sm text-ink-500">
                  {step.description}
                </p>
              </div>

              {isCurrent && step.actionLabel && step.onAction ? (
                <Button
                  size="sm"
                  className="shrink-0 bg-brand font-semibold text-white shadow-sm shadow-brand/20 hover:bg-brand-dark"
                  onPress={step.onAction}
                >
                  {step.actionLabel}
                </Button>
              ) : null}
            </li>
          );
        })}
      </ol>
    </div>
  );
}
