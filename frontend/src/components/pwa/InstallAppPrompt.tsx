"use client";

import { Download, X } from "lucide-react";

interface Props {
  title: string;
  body: string;
  installLabel: string;
  dismissLabel: string;
  staffBottomOffset: boolean;
  isPending: boolean;
  onInstall(): void;
  onDismiss(): void;
}

export default function InstallAppPrompt({
  title,
  body,
  installLabel,
  dismissLabel,
  staffBottomOffset,
  isPending,
  onInstall,
  onDismiss,
}: Props) {
  const bottomOffset = staffBottomOffset
    ? "bottom-[calc(4.75rem+env(safe-area-inset-bottom))] xl:bottom-[calc(1rem+env(safe-area-inset-bottom))]"
    : "bottom-[calc(1rem+env(safe-area-inset-bottom))]";

  return (
    <section
      role="region"
      aria-label={title}
      aria-busy={isPending || undefined}
      className={`fixed inset-x-3 ${bottomOffset} z-50 mx-auto max-w-sm rounded-2xl border border-warm-200 bg-white p-3 shadow-panel xl:left-auto xl:right-4 xl:mx-0 xl:w-96 xl:max-w-[calc(100vw-2rem)]`}
    >
      <div className="flex items-start gap-3">
        <span
          aria-hidden="true"
          className="grid h-11 w-11 shrink-0 place-items-center rounded-xl bg-brand text-white"
        >
          <Download className="h-5 w-5" />
        </span>

        <div className="min-w-0 flex-1 py-0.5">
          <h2 className="text-sm font-semibold text-ink-950">{title}</h2>
          <p className="mt-0.5 text-xs leading-5 text-ink-500">{body}</p>
        </div>

        <button
          type="button"
          onClick={onDismiss}
          disabled={isPending}
          aria-label={dismissLabel}
          className="grid h-11 w-11 shrink-0 place-items-center rounded-xl text-ink-500 transition-colors hover:bg-warm-100 hover:text-ink-800 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand focus-visible:ring-offset-2 disabled:cursor-wait disabled:opacity-60"
        >
          <X className="h-5 w-5" aria-hidden="true" />
        </button>
      </div>

      <button
        type="button"
        onClick={onInstall}
        disabled={isPending}
        aria-busy={isPending || undefined}
        className="mt-3 flex min-h-11 w-full items-center justify-center rounded-xl bg-brand px-4 text-sm font-semibold text-white transition-colors hover:bg-brand-dark focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand focus-visible:ring-offset-2 disabled:cursor-wait disabled:opacity-70"
      >
        {installLabel}
      </button>
    </section>
  );
}
