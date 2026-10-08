/**
 * Process-local last-known-good cache for storefront SSR.
 *
 * Keyed by slug. Used only on transient 5xx/network failures so a known-live
 * venue does not SSR as "Loading business…" after a later stampede (#685).
 * Confirmed 404 / unpublish must `delete` the slug — never serve a published
 * snapshot after the owner takes the page down.
 *
 * Short TTL is a backstop, not a substitute for the 404 drop.
 */

export type SsrLastKnownGoodOptions = {
  ttlMs?: number;
  now?: () => number;
};

const DEFAULT_TTL_MS = 30_000;

type CacheEntry<T> = {
  value: T;
  expiresAt: number;
};

export class SsrLastKnownGoodCache<T> {
  private readonly entries = new Map<string, CacheEntry<T>>();
  private readonly ttlMs: number;
  private readonly now: () => number;

  constructor(options: SsrLastKnownGoodOptions = {}) {
    this.ttlMs = options.ttlMs ?? DEFAULT_TTL_MS;
    this.now = options.now ?? Date.now;
  }

  get(slug: string): T | undefined {
    const entry = this.entries.get(slug);
    if (!entry) return undefined;
    if (entry.expiresAt <= this.now()) {
      this.entries.delete(slug);
      return undefined;
    }
    return entry.value;
  }

  set(slug: string, value: T): void {
    this.entries.set(slug, {
      value,
      expiresAt: this.now() + this.ttlMs,
    });
  }

  delete(slug: string): void {
    this.entries.delete(slug);
  }

  clear(): void {
    this.entries.clear();
  }
}
