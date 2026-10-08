import { getPublicConfig } from "@/config/publicConfig";

// Reports translation lookups that fell through to fallback. Each
// (locale, key) tuple fires once per session so a single
// under-translated screen doesn't spam analytics or the console.
//
// Surface: console.warn for dev visibility + an `analytics_session_id`
// scoped beacon to `/analytics/missing_translation` (best-effort,
// silent on failure). We deliberately avoid pulling in the full
// analytics tracker module here — that would import axios, which is
// unsafe under SSR and bloats the guest bundle's critical path.

const REPORTED_THIS_SESSION = new Set<string>();

export interface MissingTranslationReport {
  locale: string;
  key: string;
  fallbackUsed: "english" | "leaf";
}

export function reportMissingTranslation(report: MissingTranslationReport): void {
  if (typeof window === "undefined") return;

  const dedupeKey = `${report.locale}::${report.key}`;
  if (REPORTED_THIS_SESSION.has(dedupeKey)) return;
  REPORTED_THIS_SESSION.add(dedupeKey);

  if (process.env.NODE_ENV !== "production") {
    console.warn(
      `[i18n] Missing translation for "${report.key}" in locale "${report.locale}" (fell back to ${report.fallbackUsed})`,
    );
  }

  // Best-effort beacon — never await, never throw. Backend ingest lives
  // at POST /api/v1/analytics/missing_translation (rate-limited public
  // route) and aggregates by (locale, key, page, fallback) into the
  // missing_translations table. Read side at GET
  // /api/v1/admin/analytics/missing-translations.
  try {
    const payload = JSON.stringify({
      page: window.location?.pathname || "",
      locale: report.locale,
      key: report.key,
      fallback_used: report.fallbackUsed,
      timestamp: new Date().toISOString(),
    });

    // Target the backend API base from the runtime public config. It always
    // carries the `/api/v1` prefix — absolute, or same-origin `/api/v1` behind
    // the built-in proxy — so the path never 404s against the frontend (HI-7).
    const apiBase = getPublicConfig().apiUrl;
    const endpoint = `${apiBase}/analytics/missing_translation`;
    let sent = false;

    if (typeof navigator !== "undefined" && typeof navigator.sendBeacon === "function") {
      const blob = new Blob([payload], { type: "application/json" });
      sent = navigator.sendBeacon(endpoint, blob);
    }

    if (!sent && typeof fetch === "function") {
      void fetch(endpoint, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: payload,
        keepalive: true,
        credentials: "same-origin",
      }).catch(() => {
        /* swallow — telemetry is never load-bearing */
      });
    }
  } catch {
    /* swallow — telemetry is never load-bearing */
  }
}

// Test-only — lets the jest suite reset dedup state between cases so
// each test sees a clean reporter. Not exported through any barrel.
export function __resetReporterForTests(): void {
  REPORTED_THIS_SESSION.clear();
}
