"use client";

import DOMPurify from "dompurify";
import { useCallback, useEffect, useRef } from "react";

const MESSAGE_SOURCE = "payverge-menu-print";
const WINDOW_NAME = "payverge-menu-print";
const WINDOW_FEATURES = "popup,width=1200,height=900";
const DEFAULT_COMPLETION_TIMEOUT_MS = 60_000;
const DEFAULT_READY_TIMEOUT_MS = 10_000;
const DEFAULT_CLOSED_POLL_MS = 250;
const READY_WATCHDOG_GRACE_MS = 250;
const DEFERRED_CLOSE_MS = 500;
const MAX_DURATION_MS = 2_147_483_647 - READY_WATCHDOG_GRACE_MS;
let jobSequence = 0;

const PRINT_ALLOWED_TAGS = [
  "html",
  "head",
  "meta",
  "title",
  "style",
  "body",
  "section",
  "div",
  "img",
  "h1",
  "h2",
  "h3",
  "h4",
  "h5",
  "h6",
  "p",
  "span",
  "article",
  "figure",
  "figcaption",
] as const;

const PRINT_ALLOWED_ATTRIBUTES = [
  "charset",
  "name",
  "content",
  "http-equiv",
  "class",
  "src",
  "alt",
  "lang",
  "dir",
  "style",
  "role",
  "title",
  "width",
  "height",
  "aria-label",
] as const;

const PRINT_FORBIDDEN_TAGS = [
  "script",
  "iframe",
  "object",
  "embed",
  "form",
  "button",
  "input",
  "select",
  "option",
  "textarea",
  "svg",
  "math",
  "base",
  "link",
  "audio",
  "video",
  "source",
  "canvas",
] as const;

const PRINT_FORBIDDEN_ATTRIBUTES = [
  "href",
  "xlink:href",
  "srcdoc",
  "formaction",
  "action",
  "data",
  "poster",
  "background",
  "manifest",
  "onerror",
  "onload",
  "onclick",
  "onfocus",
  "onanimationstart",
  "onbegin",
] as const;

const PRINT_ALLOWED_URI =
  /^(?:(?:https?):|data:image\/(?:png|jpe?g|gif|webp|avif|bmp|x-icon);base64,|(?:\.{0,2}\/|\/(?!\/)|[a-z0-9_.~-]+(?:[/?#]|$)))/i;
const CSP_NONCE_PATTERN = /^[A-Za-z0-9+/_-]+={0,2}$/;

export type MenuPrintWindowStatus =
  | "complete"
  | "blocked"
  | "timeout"
  | "superseded"
  | "error";

export interface MenuPrintWindowResult {
  status: MenuPrintWindowStatus;
}

export interface UseMenuPrintWindowOptions {
  completionTimeoutMs?: number;
  readyTimeoutMs?: number;
  closedPollMs?: number;
}

interface ActivePrintJob {
  child: Window;
  jobId: string;
  messageListener: (event: MessageEvent) => void;
  readinessTimer: ReturnType<typeof setTimeout> | null;
  completionTimer: ReturnType<typeof setTimeout> | null;
  closedTimer: ReturnType<typeof setInterval> | null;
  printed: boolean;
  settled: boolean;
  resolve: (result: MenuPrintWindowResult) => void;
}

function scriptValue(value: string): string {
  return JSON.stringify(value)
    .replaceAll("<", "\\u003c")
    .replaceAll(">", "\\u003e")
    .replaceAll("&", "\\u0026")
    .replaceAll("\u2028", "\\u2028")
    .replaceAll("\u2029", "\\u2029");
}

function createLifecycleScriptText(
  jobId: string,
  expectedOrigin: string,
  readyTimeoutMs: number,
): string {
  return `(() => {
  "use strict";
  const JOB_ID = ${scriptValue(jobId)};
  const EXPECTED_ORIGIN = ${scriptValue(expectedOrigin)};
  const READY_TIMEOUT_MS = ${readyTimeoutMs};
  const payload = (type, detail) => window.opener?.postMessage(
    { source: "${MESSAGE_SOURCE}", jobId: JOB_ID, type, detail },
    EXPECTED_ORIGIN,
  );
  const assetsReady = Promise.allSettled([
    document.fonts ? document.fonts.ready : Promise.resolve(),
    ...Array.from(document.images).map((img) => img.complete
      ? Promise.resolve()
      : new Promise((resolve) => {
          img.addEventListener("load", resolve, { once: true });
          img.addEventListener("error", resolve, { once: true });
        })),
  ]);
  Promise.race([
    assetsReady,
    new Promise((resolve) => setTimeout(resolve, READY_TIMEOUT_MS)),
  ]).then(() => payload("ready"));
  addEventListener("afterprint", () => payload("complete"), { once: true });
  addEventListener("pagehide", () => payload("complete"), { once: true });
})();`;
}

function readActiveCspNonce(parentDocument: Document): string | undefined {
  for (const element of parentDocument.querySelectorAll<
    HTMLScriptElement | HTMLStyleElement
  >("script, style")) {
    const candidate = element.nonce;
    if (
      typeof candidate === "string" &&
      candidate.length >= 8 &&
      candidate.length <= 256 &&
      CSP_NONCE_PATTERN.test(candidate)
    ) {
      return candidate;
    }
  }
  return undefined;
}

function compactAsciiWhitespace(value: string): string {
  return value.replace(/[\u0000-\u0020\u007f-\u009f]+/g, "");
}

function isSafeImageSource(value: string, expectedOrigin: string): boolean {
  const compact = compactAsciiWhitespace(value).toLowerCase();
  if (
    /^data:image\/(?:png|jpe?g|gif|webp|avif|bmp|x-icon);base64,/i.test(compact)
  ) {
    return true;
  }
  if (compact.startsWith("data:")) return false;

  try {
    const resolved = new URL(value, `${expectedOrigin}/`);
    return resolved.protocol === "http:" || resolved.protocol === "https:";
  } catch {
    return false;
  }
}

function decodeCssEscapes(value: string): string {
  return value.replace(
    /\\(?:([0-9a-f]{1,6})(?:\r\n|[ \t\r\n\f])?|(\r\n|[\n\r\f])|([\s\S]))/gi,
    (
      _match,
      hex: string | undefined,
      continuation: string | undefined,
      escaped: string | undefined,
    ) => {
      if (hex) {
        const codePoint = Number.parseInt(hex, 16);
        return codePoint === 0 ||
          codePoint > 0x10ffff ||
          (codePoint >= 0xd800 && codePoint <= 0xdfff)
          ? "\ufffd"
          : String.fromCodePoint(codePoint);
      }
      return continuation ? "" : (escaped ?? "");
    },
  );
}

function canonicalizeCss(value: string): string {
  let canonical = value;
  while (true) {
    const next = decodeCssEscapes(
      canonical.replace(/\/\*[\s\S]*?(?:\*\/|$)/g, ""),
    );
    if (next === canonical) return compactAsciiWhitespace(next);
    canonical = next;
  }
}

function isSafeCssFontSource(value: string, expectedOrigin: string): boolean {
  if (!value || value.startsWith("//")) return false;
  const isRootRelative = value.startsWith("/");
  if (!isRootRelative && !/^https?:\/\//i.test(value)) return false;

  try {
    const expected = new URL(expectedOrigin);
    const resolved = new URL(value, `${expected.origin}/`);
    return (
      (resolved.protocol === "http:" || resolved.protocol === "https:") &&
      resolved.origin === expected.origin &&
      !resolved.username &&
      !resolved.password &&
      /^\/fonts\/[a-z0-9/_-]+\.(?:woff2?|ttf|otf|eot)$/i.test(resolved.pathname)
    );
  } catch {
    return false;
  }
}

function hasUnsafeCss(value: string, expectedOrigin: string): boolean {
  const canonical = canonicalizeCss(value);
  const lower = canonical.replace(/[A-Z]/g, (character) =>
    character.toLowerCase(),
  );
  if (
    lower.includes("javascript:") ||
    lower.includes("expression(") ||
    lower.includes("@import") ||
    lower.includes("-moz-binding") ||
    lower.includes("behavior:") ||
    lower.includes("data:text/html") ||
    lower.includes("data:text-html")
  ) {
    return true;
  }

  let cursor = 0;
  while (true) {
    const start = lower.indexOf("url(", cursor);
    if (start === -1) return false;
    const valueStart = start + 4;
    const quote = canonical[valueStart];
    let source: string;
    if (quote === '"' || quote === "'") {
      const valueEnd = canonical.indexOf(quote, valueStart + 1);
      if (valueEnd === -1 || canonical[valueEnd + 1] !== ")") return true;
      source = canonical.slice(valueStart + 1, valueEnd);
      cursor = valueEnd + 2;
    } else {
      const valueEnd = canonical.indexOf(")", valueStart);
      if (valueEnd === -1) return true;
      source = canonical.slice(valueStart, valueEnd);
      if (source.includes('"') || source.includes("'")) return true;
      cursor = valueEnd + 1;
    }
    if (!isSafeCssFontSource(source, expectedOrigin)) return true;
  }
}

function sanitizePrintDocument(
  sourceHtml: string,
  expectedOrigin: string,
): Document {
  const inertSource = new DOMParser().parseFromString(sourceHtml, "text/html");
  inertSource.querySelectorAll("meta").forEach((meta) => {
    const directive = compactAsciiWhitespace(
      meta.getAttribute("http-equiv") ?? "",
    ).toLowerCase();
    if (directive === "refresh") meta.remove();
  });

  const sanitizedHtml = DOMPurify.sanitize(
    `<!doctype html>${inertSource.documentElement.outerHTML}`,
    {
      WHOLE_DOCUMENT: true,
      ALLOWED_TAGS: [...PRINT_ALLOWED_TAGS],
      ALLOWED_ATTR: [...PRINT_ALLOWED_ATTRIBUTES],
      FORBID_TAGS: [...PRINT_FORBIDDEN_TAGS],
      FORBID_ATTR: [...PRINT_FORBIDDEN_ATTRIBUTES],
      ALLOWED_NAMESPACES: ["http://www.w3.org/1999/xhtml"],
      ALLOWED_URI_REGEXP: PRINT_ALLOWED_URI,
      ALLOW_UNKNOWN_PROTOCOLS: false,
      ALLOW_ARIA_ATTR: true,
      ALLOW_DATA_ATTR: true,
    },
  );
  const parsed = new DOMParser().parseFromString(sanitizedHtml, "text/html");

  parsed.querySelectorAll("meta").forEach((meta) => {
    const name = meta.getAttribute("name")?.trim().toLowerCase();
    if (name === "payverge-print-job" || meta.hasAttribute("http-equiv")) {
      meta.remove();
    }
  });
  parsed.querySelectorAll("img").forEach((image) => {
    const source = image.getAttribute("src");
    if (!source || !isSafeImageSource(source, expectedOrigin)) image.remove();
  });
  parsed.querySelectorAll("style").forEach((style) => {
    if (hasUnsafeCss(style.textContent ?? "", expectedOrigin)) style.remove();
  });
  parsed.querySelectorAll<HTMLElement>("[style]").forEach((element) => {
    if (hasUnsafeCss(element.getAttribute("style") ?? "", expectedOrigin)) {
      element.removeAttribute("style");
    }
  });
  parsed.querySelectorAll("*").forEach((element) => {
    for (const attribute of Array.from(element.attributes)) {
      if (attribute.name.toLowerCase().startsWith("on")) {
        element.removeAttribute(attribute.name);
      }
    }
  });

  return parsed;
}

function createPrintDocument(
  sourceHtml: string,
  jobId: string,
  expectedOrigin: string,
): string {
  const parsed = sanitizePrintDocument(sourceHtml, expectedOrigin);

  const meta = parsed.createElement("meta");
  meta.setAttribute("name", "payverge-print-job");
  meta.setAttribute("content", jobId);
  parsed.head.appendChild(meta);

  return `<!doctype html>${parsed.documentElement.outerHTML}`;
}

function appendLifecycleScript(
  childDocument: Document,
  jobId: string,
  expectedOrigin: string,
  readyTimeoutMs: number,
  cspNonce: string | undefined,
): void {
  if (!childDocument.body) throw new Error("Print document body unavailable");
  const lifecycle = childDocument.createElement("script");
  lifecycle.id = "payverge-print-lifecycle";
  if (cspNonce) lifecycle.nonce = cspNonce;
  lifecycle.textContent = createLifecycleScriptText(
    jobId,
    expectedOrigin,
    readyTimeoutMs,
  );
  childDocument.body.appendChild(lifecycle);
}

function positiveDuration(value: number | undefined, fallback: number): number {
  return Number.isFinite(value) && (value ?? 0) > 0
    ? Math.min(MAX_DURATION_MS, Math.max(1, Math.floor(value as number)))
    : fallback;
}

export function useMenuPrintWindow(options: UseMenuPrintWindowOptions = {}) {
  const activeJobRef = useRef<ActivePrintJob | null>(null);
  const sequenceRef = useRef(0);
  const deferredCloseTimersRef = useRef(
    new Map<Window, ReturnType<typeof setTimeout>>(),
  );
  const completionTimeoutMs = positiveDuration(
    options.completionTimeoutMs,
    DEFAULT_COMPLETION_TIMEOUT_MS,
  );
  const readyTimeoutMs = positiveDuration(
    options.readyTimeoutMs,
    DEFAULT_READY_TIMEOUT_MS,
  );
  const closedPollMs = positiveDuration(
    options.closedPollMs,
    DEFAULT_CLOSED_POLL_MS,
  );

  const finish = useCallback(
    (
      job: ActivePrintJob,
      status: MenuPrintWindowStatus,
      { deferClose = true }: { deferClose?: boolean } = {},
    ) => {
      if (job.settled) return;
      job.settled = true;
      window.removeEventListener("message", job.messageListener);
      if (job.readinessTimer) clearTimeout(job.readinessTimer);
      if (job.completionTimer) clearTimeout(job.completionTimer);
      if (job.closedTimer) clearInterval(job.closedTimer);
      if (activeJobRef.current === job) activeJobRef.current = null;

      try {
        window.focus();
      } catch {
        // The result must still settle if a browser refuses programmatic focus.
      }
      job.resolve({ status });

      if (!deferClose) {
        try {
          if (!job.child.closed) job.child.close();
        } catch {
          // The child may already have navigated or closed.
        }
        return;
      }

      const priorClose = deferredCloseTimersRef.current.get(job.child);
      if (priorClose) clearTimeout(priorClose);
      const closeTimer = setTimeout(() => {
        deferredCloseTimersRef.current.delete(job.child);
        if (activeJobRef.current?.child === job.child) return;
        try {
          if (!job.child.closed) job.child.close();
        } catch {
          // The child may already have navigated or closed.
        }
      }, DEFERRED_CLOSE_MS);
      deferredCloseTimersRef.current.set(job.child, closeTimer);
    },
    [],
  );

  const print = useCallback(
    (html: string): Promise<MenuPrintWindowResult> => {
      let child: Window | null = null;
      try {
        child = window.open("", WINDOW_NAME, WINDOW_FEATURES);
      } catch {
        return Promise.resolve({ status: "blocked" });
      }
      if (!child) return Promise.resolve({ status: "blocked" });

      const priorActive = activeJobRef.current;
      if (priorActive) finish(priorActive, "superseded");

      const pendingClose = deferredCloseTimersRef.current.get(child);
      if (pendingClose) {
        clearTimeout(pendingClose);
        deferredCloseTimersRef.current.delete(child);
      }

      // The module counter prevents job-id reuse across hook remounts; the ref
      // records this hook instance's latest monotonically assigned identifier.
      sequenceRef.current = ++jobSequence;
      const jobId = String(sequenceRef.current);
      const cspNonce = readActiveCspNonce(document);
      return new Promise<MenuPrintWindowResult>((resolve) => {
        const job: ActivePrintJob = {
          child,
          jobId,
          messageListener: () => {},
          readinessTimer: null,
          completionTimer: null,
          closedTimer: null,
          printed: false,
          settled: false,
          resolve,
        };

        job.messageListener = (event: MessageEvent) => {
          if (
            event.origin !== window.location.origin ||
            event.source !== child ||
            !event.data ||
            typeof event.data !== "object" ||
            event.data.source !== MESSAGE_SOURCE ||
            event.data.jobId !== jobId ||
            (event.data.type !== "ready" && event.data.type !== "complete")
          ) {
            return;
          }

          if (event.data.type === "complete") {
            finish(job, "complete");
            return;
          }
          if (job.printed || job.settled) return;
          job.printed = true;
          if (job.readinessTimer) {
            clearTimeout(job.readinessTimer);
            job.readinessTimer = null;
          }
          job.completionTimer = setTimeout(
            () => finish(job, "timeout"),
            completionTimeoutMs,
          );
          try {
            child.focus();
            child.print();
          } catch {
            finish(job, "error");
          }
        };
        job.readinessTimer = setTimeout(
          () => finish(job, "timeout"),
          readyTimeoutMs + READY_WATCHDOG_GRACE_MS,
        );
        job.closedTimer = setInterval(() => {
          try {
            if (child.closed) finish(job, "complete");
          } catch {
            finish(job, "complete");
          }
        }, closedPollMs);

        activeJobRef.current = job;
        window.addEventListener("message", job.messageListener);

        try {
          child.document.open();
          child.document.write(
            createPrintDocument(html, jobId, window.location.origin),
          );
          child.document.close();
          appendLifecycleScript(
            child.document,
            jobId,
            window.location.origin,
            readyTimeoutMs,
            cspNonce,
          );
        } catch {
          finish(job, "error", { deferClose: false });
        }
      });
    },
    [closedPollMs, completionTimeoutMs, finish, readyTimeoutMs],
  );

  const cancel = useCallback(() => {
    const active = activeJobRef.current;
    if (active) finish(active, "superseded", { deferClose: false });
  }, [finish]);

  useEffect(() => {
    const deferredCloseTimers = deferredCloseTimersRef.current;
    return () => {
      const active = activeJobRef.current;
      if (active) finish(active, "complete", { deferClose: false });
      for (const [child, timer] of deferredCloseTimers) {
        clearTimeout(timer);
        try {
          if (!child.closed) child.close();
        } catch {
          // The child may already have navigated or closed.
        }
      }
      deferredCloseTimers.clear();
    };
  }, [finish]);

  return { print, cancel };
}
