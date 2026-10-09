"use client";

/**
 * DirectorMessage — minimal, dependency-free markdown renderer for the
 * Director's Console chat surface.
 *
 * Why hand-rolled? `react-markdown` v10 is ESM-only and would require
 * Jest transformIgnorePatterns gymnastics for the small set of markdown
 * features the Director's Console actually emits (headings, paragraphs,
 * unordered/ordered lists, bold, italic, inline code). The Director
 * backend ships a `structured_response` payload for the rich layout; this
 * component is the *fallback* path that runs when the LLM returns a plain
 * markdown body. The goal is simply to preserve structural elements
 * (headings, lists, bold) so operators don't see literal `##` and `-`
 * characters in the UI.
 *
 * Inline formatting handled:
 *   - bold via double-asterisk
 *   - italic via single-asterisk or underscore
 *   - inline code via backticks
 *
 * Block elements handled:
 *   - heading markers (#, ##, ###)
 *   - bullet items (- or *)
 *   - numbered items (1.)
 *   - blank-line-separated paragraphs
 */

import React from "react";

export interface DirectorMessageProps {
  content: string;
  role: "user" | "assistant" | "system";
}

export default function DirectorMessage({ content, role }: DirectorMessageProps) {
  if (role === "user" || role === "system") {
    // User and system messages are rendered verbatim. Authors typed them
    // exactly as shown; we must not silently transform markdown markers
    // into HTML structure.
    return (
      <p className="text-body text-ink-800 whitespace-pre-wrap">{content}</p>
    );
  }

  return <MarkdownBody source={content} />;
}

function MarkdownBody({ source }: { source: string }) {
  const blocks = React.useMemo(() => parseBlocks(source), [source]);

  return (
    <div className="space-y-2 text-body text-ink-700">
      {blocks.map((block, idx) => renderBlock(block, idx))}
    </div>
  );
}

type Block =
  | { type: "heading"; level: 1 | 2 | 3; text: string }
  | { type: "paragraph"; text: string }
  | { type: "ul"; items: string[] }
  | { type: "ol"; items: string[] };

function parseBlocks(source: string): Block[] {
  const lines = source.replace(/\r\n/g, "\n").split("\n");
  const blocks: Block[] = [];

  let i = 0;
  while (i < lines.length) {
    const line = lines[i];

    if (line.trim() === "") {
      i += 1;
      continue;
    }

    // Heading: one to three leading hash chars followed by a space.
    const headingMatch = /^(#{1,3})\s+(.*)$/.exec(line);
    if (headingMatch) {
      const level = headingMatch[1].length as 1 | 2 | 3;
      blocks.push({ type: "heading", level, text: headingMatch[2].trim() });
      i += 1;
      continue;
    }

    // Unordered list: contiguous lines starting with `- ` or `* `.
    if (/^\s*[-*]\s+/.test(line)) {
      const items: string[] = [];
      while (i < lines.length && /^\s*[-*]\s+/.test(lines[i])) {
        items.push(lines[i].replace(/^\s*[-*]\s+/, "").trim());
        i += 1;
      }
      blocks.push({ type: "ul", items });
      continue;
    }

    // Ordered list: contiguous lines starting with `<digits>. `.
    if (/^\s*\d+\.\s+/.test(line)) {
      const items: string[] = [];
      while (i < lines.length && /^\s*\d+\.\s+/.test(lines[i])) {
        items.push(lines[i].replace(/^\s*\d+\.\s+/, "").trim());
        i += 1;
      }
      blocks.push({ type: "ol", items });
      continue;
    }

    // Paragraph: accumulate non-blank lines until a structural break.
    const paragraphLines: string[] = [];
    while (
      i < lines.length &&
      lines[i].trim() !== "" &&
      !/^(#{1,3})\s+/.test(lines[i]) &&
      !/^\s*[-*]\s+/.test(lines[i]) &&
      !/^\s*\d+\.\s+/.test(lines[i])
    ) {
      paragraphLines.push(lines[i]);
      i += 1;
    }
    if (paragraphLines.length > 0) {
      blocks.push({ type: "paragraph", text: paragraphLines.join(" ") });
    }
  }

  return blocks;
}

function renderBlock(block: Block, key: number): React.ReactNode {
  switch (block.type) {
    case "heading": {
      const text = renderInline(block.text);
      if (block.level === 1) {
        return (
          <h3
            key={key}
            className="font-title text-heading-md text-ink-900 mt-3 mb-1"
          >
            {text}
          </h3>
        );
      }
      if (block.level === 2) {
        return (
          <h4
            key={key}
            className="font-title text-heading-sm text-ink-900 mt-3 mb-1"
          >
            {text}
          </h4>
        );
      }
      return (
        <h5 key={key} className="font-semibold text-ink-900 mt-2 mb-1">
          {text}
        </h5>
      );
    }
    case "paragraph":
      return (
        <p key={key} className="text-body text-ink-700 leading-relaxed">
          {renderInline(block.text)}
        </p>
      );
    case "ul":
      return (
        <ul
          key={key}
          className="list-disc pl-5 text-body text-ink-700 space-y-1"
        >
          {block.items.map((item, idx) => (
            <li key={idx}>{renderInline(item)}</li>
          ))}
        </ul>
      );
    case "ol":
      return (
        <ol
          key={key}
          className="list-decimal pl-5 text-body text-ink-700 space-y-1"
        >
          {block.items.map((item, idx) => (
            <li key={idx}>{renderInline(item)}</li>
          ))}
        </ol>
      );
    default:
      return null;
  }
}

/**
 * renderInline — walk a single line of markdown and emit React nodes
 * for the inline formatting markers we support.
 *
 * The walker moves left-to-right and peels off the next matching
 * delimiter pair (bold, italic, inline code). This is intentionally
 * small — we don't support nested emphasis or links, because the
 * Director backend's structured response carries those affordances
 * natively.
 */
function renderInline(text: string): React.ReactNode {
  const nodes: React.ReactNode[] = [];
  let buffer = "";
  let i = 0;
  let key = 0;

  const flushBuffer = () => {
    if (buffer.length > 0) {
      nodes.push(buffer);
      buffer = "";
    }
  };

  while (i < text.length) {
    // bold via double-asterisk
    if (text[i] === "*" && text[i + 1] === "*") {
      const end = text.indexOf("**", i + 2);
      if (end !== -1) {
        flushBuffer();
        const inner = text.slice(i + 2, end);
        nodes.push(
          <strong key={`b-${key++}`} className="font-semibold text-ink-900">
            {inner}
          </strong>,
        );
        i = end + 2;
        continue;
      }
    }

    // italic via single-asterisk (skip when sitting on a bold marker)
    if (text[i] === "*" && text[i + 1] !== "*") {
      const end = text.indexOf("*", i + 1);
      if (end !== -1) {
        flushBuffer();
        const inner = text.slice(i + 1, end);
        nodes.push(
          <em key={`i-${key++}`} className="italic">
            {inner}
          </em>,
        );
        i = end + 1;
        continue;
      }
    }

    // italic via underscore
    if (text[i] === "_") {
      const end = text.indexOf("_", i + 1);
      if (end !== -1) {
        flushBuffer();
        const inner = text.slice(i + 1, end);
        nodes.push(
          <em key={`u-${key++}`} className="italic">
            {inner}
          </em>,
        );
        i = end + 1;
        continue;
      }
    }

    // inline code via backticks
    if (text[i] === "`") {
      const end = text.indexOf("`", i + 1);
      if (end !== -1) {
        flushBuffer();
        const inner = text.slice(i + 1, end);
        nodes.push(
          <code
            key={`c-${key++}`}
            className="bg-warm-100 px-1.5 py-0.5 rounded text-sm text-ink-900"
          >
            {inner}
          </code>,
        );
        i = end + 1;
        continue;
      }
    }

    buffer += text[i];
    i += 1;
  }

  flushBuffer();
  return nodes;
}
