interface CacheEntry<T> {
  data: T;
  timestamp: number;
  ttl: number;
}

export class ApiCache {
  private cache = new Map<string, CacheEntry<unknown>>();
  private defaultTTL = 5 * 60 * 1000; // 5 minutes
  private fingerprint: string | null = null;
  private sessionListeners = new Set<() => void>();

  /**
   * Subscribe to session ends: logout, or one signed-in principal replaced
   * by another. Other client caches (React Query) clear on this so they
   * can't serve the previous user's data. Returns an unsubscribe function.
   */
  onSessionChange(listener: () => void): () => void {
    this.sessionListeners.add(listener);
    return () => {
      this.sessionListeners.delete(listener);
    };
  }

  private notifySessionChange(): void {
    for (const listener of [...this.sessionListeners]) {
      try {
        listener();
      } catch {
        // One listener failing must not keep the others from clearing.
      }
    }
  }

  /**
   * Logout: drop the fingerprint and every entry, and notify session
   * listeners even when no fingerprint was ever set.
   */
  endSession(): void {
    this.fingerprint = null;
    this.cache.clear();
    this.notifySessionChange();
  }

  /**
   * Scope the cache to a session fingerprint. Must be called every time the
   * authenticated principal changes (login, logout, staff switch). Changing
   * the fingerprint evicts all existing entries so a shared device can't
   * leak responses across users.
   *
   * Coerces undefined to null so callers passing weakly-typed values don't
   * trigger a spurious full cache clear on every session-info resolution.
   */
  setFingerprint(fp: string | null | undefined): void {
    const normalized: string | null = fp ?? null;
    if (normalized === this.fingerprint) return;
    const previous = this.fingerprint;
    this.fingerprint = normalized;
    // Session changed: evict all entries to prevent cross-user data leaks.
    // Do NOT "optimise" this away by scoping keys by fingerprint prefix —
    // the clear is the security guarantee.
    this.cache.clear();
    // Anonymous -> signed in is a first login, not a switch: nothing of a
    // previous principal can be cached yet.
    if (previous !== null) this.notifySessionChange();
  }

  private getSessionScope(): string {
    return this.fingerprint ?? 'anonymous';
  }

  private generateKey(url: string, params?: unknown): string {
    const paramString = params ? JSON.stringify(params) : '';
    return `${this.getSessionScope()}::${url}${paramString}`;
  }

  set<T>(url: string, data: T, params?: unknown, ttl?: number): void {
    const key = this.generateKey(url, params);
    const entry: CacheEntry<T> = {
      data,
      timestamp: Date.now(),
      ttl: ttl || this.defaultTTL
    };
    this.cache.set(key, entry);
  }

  get<T>(url: string, params?: unknown): T | null {
    const key = this.generateKey(url, params);
    const entry = this.cache.get(key) as CacheEntry<T> | undefined;

    if (!entry) return null;

    // Check if entry has expired
    if (Date.now() - entry.timestamp > entry.ttl) {
      this.cache.delete(key);
      return null;
    }

    return entry.data;
  }

  delete(url: string, params?: unknown): void {
    const key = this.generateKey(url, params);
    this.cache.delete(key);
  }

  clear(): void {
    this.cache.clear();
  }

  clearByPrefixes(prefixes: string[]): void {
    this.cache.forEach((_entry, key) => {
      if (prefixes.some(prefix => key.includes(prefix))) {
        this.cache.delete(key);
      }
    });
  }

  // Clean up expired entries
  cleanup(): void {
    const now = Date.now();
    const keysToDelete: string[] = [];

    this.cache.forEach((entry, key) => {
      if (now - entry.timestamp > entry.ttl) {
        keysToDelete.push(key);
      }
    });

    keysToDelete.forEach(key => this.cache.delete(key));
  }
}

export const apiCache = new ApiCache();

// Auto-cleanup every 10 minutes (browser only — avoid SSR timer leak).
// Held in a module-level handle so it can be torn down (tests / hot-reload)
// and so re-evaluation of this module can't register a second timer.
let cleanupTimer: ReturnType<typeof setInterval> | null = null;

if (typeof window !== 'undefined' && cleanupTimer === null) {
  cleanupTimer = setInterval(() => {
    apiCache.cleanup();
  }, 10 * 60 * 1000);
}

// Stop the auto-cleanup timer. Exposed for test teardown and hot-reload;
// a no-op if the timer is not currently running (e.g. under SSR).
export function stopCacheCleanup(): void {
  if (cleanupTimer !== null) {
    clearInterval(cleanupTimer);
    cleanupTimer = null;
  }
}
