/**
 * PG-15.1 — strip lightweight markdown markers from model text for plain
 * display. Escapes nothing into HTML; converts **bold** / *italic* / __ / _
 * markers to plain text only. Safe for model output (no raw-HTML path).
 */
export interface StripSimpleMarkdownOptions {
  /**
   * Leave `![alt](url)` tokens intact so a caller that renders images itself
   * (AiWaiter's allowlisted menu-host images) can still find them. Every other
   * marker is still flattened. Default false: image → alt text.
   */
  keepImages?: boolean;
}

const IMAGE_TOKEN = /!\[([^\]]*)\]\([^)]+\)/g;
// Sentinel chosen so none of the emphasis/link/heading passes below can match
// inside it (no `](`, no `*`, no `_`, no leading `#`). Keeps a `*` or `_` in an
// image URL from being eaten as emphasis while the token is parked.
const IMAGE_PLACEHOLDER = /%%PVIMG(\d+)%%/g;

export function stripSimpleMarkdown(
  input: string,
  options?: StripSimpleMarkdownOptions,
): string {
  if (!input) return input;
  let out = input;
  // Fenced code blocks → inner text only
  out = out.replace(/```[\w]*\n?([\s\S]*?)```/g, "$1");
  // Images ![alt](url) → alt, or parked verbatim for callers that render them.
  const parked: string[] = [];
  if (options?.keepImages) {
    out = out.replace(IMAGE_TOKEN, (match) => {
      parked.push(match);
      return `%%PVIMG${parked.length - 1}%%`;
    });
  } else {
    out = out.replace(IMAGE_TOKEN, "$1");
  }
  // Links [label](url) → label
  out = out.replace(/\[([^\]]+)\]\([^)]+\)/g, "$1");
  // Bold/italic markers
  out = out.replace(/\*\*([^*]+)\*\*/g, "$1");
  out = out.replace(/__([^_]+)__/g, "$1");
  out = out.replace(/(?<!\w)\*([^*]+)\*(?!\w)/g, "$1");
  out = out.replace(/(?<!\w)_([^_]+)_(?!\w)/g, "$1");
  // Headings
  out = out.replace(/^#{1,6}\s+/gm, "");
  // Restore parked image tokens verbatim.
  if (parked.length > 0) {
    out = out.replace(IMAGE_PLACEHOLDER, (match, index: string) => {
      const restored = parked[Number(index)];
      return restored === undefined ? match : restored;
    });
  }
  return out;
}
