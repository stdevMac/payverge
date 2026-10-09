"use client";

import React, { useRef } from "react";
import { Button } from "@nextui-org/react";
import { X, ArrowLeftRight, UserMinus } from "lucide-react";
import { useDialogBehavior } from "@/hooks/useDialogBehavior";

export interface OfferCoverSheetLabels {
  title: string;
  subtitle: string;
  offerCover: string;
  offerCoverHint: string;
  giveUp: string;
  giveUpHint: string;
  cancel: string;
  close: string;
}

export interface OfferCoverSheetProps {
  /** Formatted date+time range of the shift, for context only — never money. */
  shiftLabel: string;
  onOffer: () => void;
  onGiveUp: () => void;
  onClose: () => void;
  /** True while a request mutation is in flight — freezes both choices. */
  busy: boolean;
  labels: OfferCoverSheetLabels;
}

/**
 * OfferCoverSheet is the staff-side coverage INITIATION surface (Phase 4c). From a
 * shift they can't work, a staffer picks one of two hand-off paths:
 *  • Offer for cover  → a swap broadcast to eligible teammates (someone accepts,
 *    the manager approves, the shift transfers to them).
 *  • Give up the shift → straight to the manager, who opens it up on approval.
 * Purely presentational + money-free; the container (MyScheduleView) owns the
 * mutation and the eligibility/future-shift gating.
 */
export default function OfferCoverSheet({
  shiftLabel,
  onOffer,
  onGiveUp,
  onClose,
  busy,
  labels,
}: OfferCoverSheetProps) {
  const containerRef = useRef<HTMLDivElement>(null);
  const offerButtonRef = useRef<HTMLButtonElement>(null);

  // The sheet is mounted only while open. Escape is a no-op while a request
  // is in flight so a dismissal can't race the mutation.
  useDialogBehavior({
    isOpen: true,
    onClose: () => {
      if (busy) return;
      onClose();
    },
    containerRef,
    initialFocusRef: offerButtonRef,
  });

  return (
    <div
      ref={containerRef}
      tabIndex={-1}
      className="fixed inset-0 z-50 flex items-end justify-center bg-black/40 p-0 sm:items-center sm:p-4"
      role="dialog"
      aria-modal="true"
      aria-label={labels.title}
    >
      <div className="w-full max-w-md rounded-t-2xl border border-gray-200 bg-white p-5 shadow-xl sm:rounded-2xl">
        <div className="mb-4 flex items-start justify-between">
          <div>
            <h3 className="font-title text-base text-gray-900">{labels.title}</h3>
            <p className="mt-0.5 text-xs text-gray-500">{shiftLabel}</p>
          </div>
          <button
            type="button"
            onClick={onClose}
            aria-label={labels.close}
            className="rounded-lg p-1 text-gray-400 transition hover:bg-gray-100 hover:text-gray-700"
          >
            <X className="h-4 w-4" />
          </button>
        </div>

        <p className="mb-3 text-sm text-gray-600">{labels.subtitle}</p>

        <div className="space-y-3">
          <button
            ref={offerButtonRef}
            type="button"
            onClick={onOffer}
            disabled={busy}
            className="flex w-full items-start gap-3 rounded-xl border border-gray-200 p-4 text-left transition hover:border-brand-400 hover:bg-brand-50/40 disabled:cursor-not-allowed disabled:opacity-60"
          >
            <ArrowLeftRight
              className="mt-0.5 h-5 w-5 shrink-0 text-brand"
              aria-hidden="true"
            />
            <span>
              <span className="block text-sm font-semibold text-gray-900">
                {labels.offerCover}
              </span>
              <span className="mt-0.5 block text-xs text-gray-500">
                {labels.offerCoverHint}
              </span>
            </span>
          </button>

          <button
            type="button"
            onClick={onGiveUp}
            disabled={busy}
            className="flex w-full items-start gap-3 rounded-xl border border-gray-200 p-4 text-left transition hover:border-brand-400 hover:bg-brand-50/40 disabled:cursor-not-allowed disabled:opacity-60"
          >
            <UserMinus
              className="mt-0.5 h-5 w-5 shrink-0 text-brand"
              aria-hidden="true"
            />
            <span>
              <span className="block text-sm font-semibold text-gray-900">
                {labels.giveUp}
              </span>
              <span className="mt-0.5 block text-xs text-gray-500">
                {labels.giveUpHint}
              </span>
            </span>
          </button>
        </div>

        <div className="mt-4 flex justify-end">
          <Button variant="light" onPress={onClose} isDisabled={busy}>
            {labels.cancel}
          </Button>
        </div>
      </div>
    </div>
  );
}
