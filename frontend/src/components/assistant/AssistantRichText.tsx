import React, { type ComponentPropsWithoutRef, type ReactNode } from "react";
import ReactMarkdown, { type Components } from "react-markdown";
import remarkGfm from "remark-gfm";
import { assistantUrlTransform, classifyAssistantHref } from "./assistantLinks";
import {
  assistantEventProperties,
  type AssistantAnalyticsContext,
  trackAssistantEvent,
} from "./assistantAnalytics";
import { stripSimpleMarkdown } from "@/utils/stripSimpleMarkdown";

export interface AssistantRichTextProps {
  content: string;
  format?: "markdown" | "plain_text";
  className?: string;
  analytics?: AssistantAnalyticsContext;
}

type MarkdownAnchorProps = ComponentPropsWithoutRef<"a"> & {
  children?: ReactNode;
  node?: unknown;
};

function SafeAssistantLink({
  children,
  href,
  node: _node,
  target: _target,
  rel: _rel,
  analytics,
  ...props
}: MarkdownAnchorProps & { analytics?: AssistantAnalyticsContext }) {
  const kind = classifyAssistantHref(href);

  if (kind === "blocked" || !href) return <>{children}</>;

  if (kind === "external") {
    return (
      <a
        {...props}
        href={href}
        target="_blank"
        rel="noopener noreferrer"
        className="font-medium text-brand underline decoration-brand/30 underline-offset-2 hover:decoration-brand focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand"
        onClick={() => {
          if (analytics) {
            trackAssistantEvent(
              "assistant_link_clicked",
              assistantEventProperties(analytics, {
                destination_kind: "external",
              }),
            );
          }
        }}
      >
        {children}
        <span aria-hidden="true" className="ms-1 inline-block text-ink-500">
          ↗
        </span>
      </a>
    );
  }

  return (
    <a
      {...props}
      href={href}
      className="font-medium text-brand underline decoration-brand/30 underline-offset-2 hover:decoration-brand focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand"
      onClick={() => {
        if (analytics) {
          trackAssistantEvent(
            "assistant_link_clicked",
            assistantEventProperties(analytics, {
              destination_kind: "internal",
            }),
          );
        }
      }}
    >
      {children}
    </a>
  );
}

const markdownComponents = {
  a: SafeAssistantLink,
  h1: ({ children }) => (
    <h3 className="mt-5 text-heading-md font-semibold text-ink-950 first:mt-0">
      {children}
    </h3>
  ),
  h2: ({ children }) => (
    <h4 className="mt-4 text-heading-sm font-semibold text-ink-950 first:mt-0">
      {children}
    </h4>
  ),
  h3: ({ children }) => (
    <h5 className="mt-4 text-body-lg font-semibold text-ink-900 first:mt-0">
      {children}
    </h5>
  ),
  h4: ({ children }) => (
    <h6 className="mt-3 text-body-md font-semibold text-ink-900 first:mt-0">
      {children}
    </h6>
  ),
  h5: ({ children }) => (
    <h6 className="mt-3 text-body-md font-semibold text-ink-900 first:mt-0">
      {children}
    </h6>
  ),
  h6: ({ children }) => (
    <h6 className="mt-3 text-body-md font-semibold text-ink-900 first:mt-0">
      {children}
    </h6>
  ),
  p: ({ children }) => (
    <p className="my-2 leading-relaxed text-ink-800 first:mt-0 last:mb-0">
      {children}
    </p>
  ),
  ol: ({ children }) => (
    <ol className="my-3 list-decimal space-y-1 ps-6 text-ink-800">
      {children}
    </ol>
  ),
  ul: ({ children }) => (
    <ul className="my-3 list-disc space-y-1 ps-6 text-ink-800">{children}</ul>
  ),
  li: ({ children }) => <li className="ps-1 leading-relaxed">{children}</li>,
  strong: ({ children }) => (
    <strong className="font-semibold text-ink-950">{children}</strong>
  ),
  em: ({ children }) => <em className="text-ink-900">{children}</em>,
  code: ({ children, className }) => (
    <code
      className={`rounded bg-warm-100 px-1.5 py-0.5 font-mono text-sm text-ink-900 ${className ?? ""}`}
    >
      {children}
    </code>
  ),
  pre: ({ children }) => (
    <pre className="my-3 overflow-x-auto rounded-xl border border-warm-200 bg-warm-50 p-4 text-sm text-ink-900">
      {children}
    </pre>
  ),
  blockquote: ({ children }) => (
    <blockquote className="my-3 border-s-4 border-brand/30 ps-4 text-ink-700">
      {children}
    </blockquote>
  ),
  table: ({ children }) => (
    <div className="my-3 overflow-x-auto">
      <table className="w-full border-collapse text-start text-sm">
        {children}
      </table>
    </div>
  ),
  th: ({ children }) => (
    <th className="border-b border-warm-200 px-3 py-2 font-semibold text-ink-950">
      {children}
    </th>
  ),
  td: ({ children }) => (
    <td className="border-b border-warm-100 px-3 py-2 text-ink-800">
      {children}
    </td>
  ),
  img: ({ alt }) => (alt ? <span className="text-ink-700">{alt}</span> : null),
} satisfies Components;

export function AssistantRichText({
  content,
  format = "markdown",
  className,
  analytics,
}: AssistantRichTextProps) {
  const rootClassName = ["text-body-md text-ink-800", className]
    .filter(Boolean)
    .join(" ");

  // plain_text: keep whitespace, but strip lightweight markdown markers so
  // **bold**/lists never leak as raw asterisks when a producer mis-tags format.
  if (format === "plain_text") {
    return (
      <div className={`${rootClassName} whitespace-pre-wrap`}>
        {stripSimpleMarkdown(content)}
      </div>
    );
  }

  return (
    <div className={rootClassName}>
      <ReactMarkdown
        remarkPlugins={[remarkGfm]}
        skipHtml
        urlTransform={assistantUrlTransform}
        components={{
          ...markdownComponents,
          a: (props) => <SafeAssistantLink {...props} analytics={analytics} />,
        }}
      >
        {content}
      </ReactMarkdown>
    </div>
  );
}
