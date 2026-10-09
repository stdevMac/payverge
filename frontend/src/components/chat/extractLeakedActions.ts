import type { ChatAction } from "@/components/chat/types";

const LEAKED_JSON_OBJ_RE = /\{[^{}]*"href"[^{}]*\}/g;
const LEAKED_KV_BLOCK_RE =
  /href:\s*(\S+)(?:\s+kind:\s*(\S+))?(?:\s+disabled:\s*(true|false))?(?:\s+disabled_reason:\s*(null|"[^"]*"|[^\n]+))?/g;

type Kind = ChatAction["kind"];

/**
 * isSafeHref reports whether an action href is safe to render or navigate to:
 * an internal absolute path ("/..." but not protocol-relative "//host"), or an
 * http(s) URL. Everything else — javascript:, data:, mailto:, etc. — is rejected
 * so semi-trusted model output cannot produce an executable or off-origin link.
 * Mirrors the Go isSafeHref in ops_leaked_actions.go byte-for-byte.
 */
export function isSafeHref(href: string): boolean {
  const h = (href ?? "").trim();
  if (!h || h.startsWith("//")) return false;
  if (h.startsWith("/")) return true;
  const lower = h.toLowerCase();
  return lower.startsWith("http://") || lower.startsWith("https://");
}

function normalizeKind(kind: string | undefined, href: string): Kind {
  const k = (kind ?? "").trim().toLowerCase();
  if (k === "navigate" || k === "external" || k === "handoff") return k;
  const h = href.trim();
  if (h.startsWith("http://") || h.startsWith("https://")) return "external";
  return "navigate";
}

function lastPathSegment(href: string): string {
  const h = href.trim();
  if (!h) return "";
  const qIdx = h.indexOf("?");
  const path = qIdx >= 0 ? h.slice(0, qIdx) : h;
  const query = qIdx >= 0 ? h.slice(qIdx + 1) : "";
  if (query) {
    for (const part of query.split("&")) {
      if (part.startsWith("tab=")) return part.slice("tab=".length);
    }
  }
  const segs = path.split("/").filter(Boolean);
  return segs.length ? segs[segs.length - 1] : "";
}

function labelFromContext(preceding: string, href: string): string {
  const trimmed = preceding.replace(/^[.:;,\-—\s]+|[.:;,\-—\s]+$/gu, "");
  if (trimmed) {
    const lines = trimmed.split("\n");
    const lastLine = lines[lines.length - 1].trim();
    if (lastLine && lastLine.length <= 60) return lastLine;
  }
  const seg = lastPathSegment(href);
  return seg || "Open";
}

function normalizeReason(tok: string | undefined): string | undefined {
  const t = (tok ?? "").trim();
  if (!t || t.toLowerCase() === "null") return undefined;
  if (t.length >= 2 && t.startsWith('"') && t.endsWith('"')) return t.slice(1, -1);
  return t;
}

/**
 * extractLeakedActions is a defense-in-depth mirror of the backend extractor.
 * It pulls action metadata the model leaked into assistant prose — key-value
 * runs (href:/kind:/disabled:/disabled_reason:) and embedded JSON action
 * objects — into ChatAction entries and returns the content with those blocks
 * stripped. Empty-href candidates are dropped. Dedupe is left to the caller,
 * which merges these with the message's own actions.
 */
export function extractLeakedActions(content: string): {
  content: string;
  actions: ChatAction[];
} {
  const actions: ChatAction[] = [];
  let clean = content;

  // 1) Embedded JSON action objects (parse structurally, then strip).
  const jsonMatches = clean.match(LEAKED_JSON_OBJ_RE) ?? [];
  for (const raw of jsonMatches) {
    try {
      const obj = JSON.parse(raw) as {
        label?: string;
        href?: string;
        kind?: string;
        disabled?: boolean;
        disabled_reason?: string | null;
      };
      const href = (obj.href ?? "").trim().replace(/[.,;:!?]+$/u, "");
      if (!href || !isSafeHref(href)) continue;
      const label = (obj.label ?? "").trim() || labelFromContext("", href);
      actions.push({
        label,
        href,
        kind: normalizeKind(obj.kind, href),
        disabled: Boolean(obj.disabled),
        disabled_reason:
          typeof obj.disabled_reason === "string" && obj.disabled_reason
            ? obj.disabled_reason
            : undefined,
      });
    } catch {
      // Not valid JSON — leave it in place (KV pass may still catch an href).
    }
  }
  clean = clean.replace(LEAKED_JSON_OBJ_RE, "");

  // 2) Key-value runs. Walk matches with matchAll (indices enabled) so the
  //    preceding text is available for label fallback while stripping.
  let out = "";
  let last = 0;
  for (const m of clean.matchAll(LEAKED_KV_BLOCK_RE)) {
    const start = m.index ?? 0;
    const preceding = clean.slice(last, start);
    out += preceding;
    const end = start + m[0].length;
    last = end;
    const href = (m[1] ?? "").trim().replace(/[.,;:!?]+$/u, "");
    if (!href || !isSafeHref(href)) {
      // Not a safe/real action link (e.g. javascript:/data: schemes, or prose
      // that merely mentions "href:"). Keep the matched text in the clean
      // output verbatim instead of stripping it, and emit nothing.
      out += clean.slice(start, end);
      continue;
    }
    actions.push({
      label: labelFromContext(preceding, href),
      href,
      kind: normalizeKind(m[2], href),
      disabled: (m[3] ?? "").toLowerCase() === "true",
      disabled_reason: normalizeReason(m[4]),
    });
  }
  out += clean.slice(last);
  clean = out;

  // Collapse whitespace left by stripped blocks; trim trailing blank lines.
  clean = clean
    .split("\n")
    .map((ln) => ln.replace(/[ \t]{2,}/g, " ").replace(/[ \t]+$/g, ""))
    .join("\n")
    .replace(/\n+$/g, "")
    .trim();

  return { content: clean, actions };
}
