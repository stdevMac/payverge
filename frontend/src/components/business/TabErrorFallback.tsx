"use client";

import React from "react";
import type { FallbackProps } from "react-error-boundary";

interface TabErrorFallbackProps extends FallbackProps {
  // tString comes from the dashboard page's operator i18n provider, which
  // survives a tab-only crash (the ErrorBoundary sits INSIDE the provider).
  tString: (key: string, params?: Record<string, string | number>) => string;
}

/** True for webpack/Next chunk load failures that cannot be fixed by re-mount. */
export function isChunkLoadError(error: unknown): boolean {
  if (!error || typeof error !== "object") return false;
  const err = error as { name?: string; message?: string };
  if (err.name === "ChunkLoadError") return true;
  const msg = typeof err.message === "string" ? err.message : "";
  return /Loading chunk .+ failed/i.test(msg);
}

const CHUNK_RELOAD_KEY_PREFIX = "chunk-reload:";

export function chunkReloadStorageKey(tabName = "default"): string {
  return `${CHUNK_RELOAD_KEY_PREFIX}${tabName || "default"}`;
}

/**
 * Decide whether to hard-reload for a chunk error (once per tab). Returns true
 * if the caller should call location.reload(); false means soft-reset instead.
 * sessionStorage guard prevents a broken deploy from reload-looping.
 */
export function shouldHardReloadForChunkError(
  error: unknown,
  storage: Pick<Storage, "getItem" | "setItem"> | null | undefined,
  tabName = "default",
): boolean {
  if (!isChunkLoadError(error) || !storage) return false;
  const key = chunkReloadStorageKey(tabName);
  try {
    if (storage.getItem(key)) return false;
    storage.setItem(key, "1");
    return true;
  } catch {
    return false;
  }
}

/**
 * In-page fallback for a single dashboard tab. A throw in one god-component
 * (Accounting / Reservations / Fiscal / Inventory) is contained here instead
 * of unwinding to the route boundary and blanking the entire console mid-service.
 *
 * ChunkLoadError is special: webpack caches the rejected chunk promise, so
 * resetErrorBoundary() re-throws the same error. Full page reload recovers;
 * sessionStorage once-per-tab guards against a broken-deploy reload loop.
 */
export default function TabErrorFallback({
  error,
  resetErrorBoundary,
  tString,
}: TabErrorFallbackProps) {
  const handleRetry = () => {
    const tabName =
      typeof window !== "undefined" ? window.name || "default" : "default";
    const storage =
      typeof window !== "undefined" ? window.sessionStorage : null;
    if (shouldHardReloadForChunkError(error, storage, tabName)) {
      window.location.reload();
      return;
    }
    resetErrorBoundary();
  };

  return (
    <div className="m-6 rounded-2xl border border-warm-200 bg-white p-8 text-center shadow-sm shadow-warm-950/[0.04]">
      <h2 className="font-title text-2xl text-ink-950">
        {tString("error.title")}
      </h2>
      <p className="mt-3 text-body text-ink-600">
        {tString("error.tabCrashed")}
      </p>
      <button
        type="button"
        onClick={handleRetry}
        className="mt-6 inline-flex items-center justify-center rounded-full bg-brand px-6 py-3 text-sm font-semibold text-white transition-colors hover:bg-brand-dark focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-dark focus-visible:ring-offset-2 focus-visible:ring-offset-white"
      >
        {tString("error.retry")}
      </button>
    </div>
  );
}
