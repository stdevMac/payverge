"use client";

import Link from "next/link";
import React, { useId, useState } from "react";
import dashboardTabsJson from "@/config/dashboard-tabs.json";
import type { AssistantSource } from "@/types/assistant";
import { classifyAssistantHref } from "./assistantLinks";
import {
  assistantEventProperties,
  type AssistantAnalyticsContext,
  trackAssistantEvent,
} from "./assistantAnalytics";

export interface AssistantSourcesLabels {
  usedSources: (count: number) => string;
  sourcesRegion: (count: number) => string;
  sourceOrigin: (origin: string) => string;
  externalSource: (hostname: string) => string;
}

export interface AssistantSourcesProps {
  sources: readonly AssistantSource[];
  labels: AssistantSourcesLabels;
  analytics?: AssistantAnalyticsContext;
}

type SafeSourceDestination =
  | { kind: "internal"; href: string }
  | { kind: "external"; href: string; hostname: string }
  | { kind: "blocked" };

const ASCII_CONTROL = /[\u0000-\u001f\u007f]/;
const ENCODED_PATH_DELIMITER = /%2f/i;
const BUSINESS_DASHBOARD_PATH = /^\/business\/[A-Za-z0-9_-]+\/dashboard$/;
const DASHBOARD_ROUTE_IDENTITY_PARAMS = [
  "businessId",
  "business_id",
  "businessSlug",
  "business_slug",
] as const;
const REGISTERED_DASHBOARD_TABS = new Set(
  dashboardTabsJson.flatMap((row) =>
    row.route_kind === "dashboard_tab" && typeof row.key === "string"
      ? [row.key]
      : [],
  ),
);

function isRegisteredSourceApplicationPath(
  href: string,
  pathname: string,
): boolean {
  if (pathname !== "/dashboard" && !BUSINESS_DASHBOARD_PATH.test(pathname)) {
    return false;
  }

  let parsed: URL;
  try {
    parsed = new URL(href, "https://assistant.invalid");
  } catch {
    return false;
  }
  if (
    DASHBOARD_ROUTE_IDENTITY_PARAMS.some((param) =>
      parsed.searchParams.has(param),
    )
  ) {
    return false;
  }
  const tabs = parsed.searchParams.getAll("tab");
  return (
    tabs.length === 0 ||
    (tabs.length === 1 && REGISTERED_DASHBOARD_TABS.has(tabs[0]))
  );
}

function safeSourceDestination(href: string | null): SafeSourceDestination {
  if (
    !href ||
    href !== href.trim() ||
    href.includes("\\") ||
    ASCII_CONTROL.test(href) ||
    href.startsWith("//")
  ) {
    return { kind: "blocked" };
  }

  let decoded: string;
  try {
    decoded = decodeURIComponent(href);
  } catch {
    return { kind: "blocked" };
  }
  if (decoded.includes("\\") || ASCII_CONTROL.test(decoded)) {
    return { kind: "blocked" };
  }

  if (href.startsWith("/")) {
    const suffixIndex = href.search(/[?#]/);
    const rawPathname = suffixIndex === -1 ? href : href.slice(0, suffixIndex);
    if (ENCODED_PATH_DELIMITER.test(rawPathname)) {
      return { kind: "blocked" };
    }
    let pathname: string;
    try {
      pathname = decodeURIComponent(rawPathname);
    } catch {
      return { kind: "blocked" };
    }
    if (
      pathname.startsWith("//") ||
      pathname.split("/").some((segment) => segment === "." || segment === "..")
    ) {
      return { kind: "blocked" };
    }
    if (
      classifyAssistantHref(href) === "internal" ||
      isRegisteredSourceApplicationPath(href, pathname)
    ) {
      return { kind: "internal", href };
    }
    return { kind: "blocked" };
  }

  if (classifyAssistantHref(href) !== "external") {
    return { kind: "blocked" };
  }
  try {
    const url = new URL(href);
    if (
      (url.protocol !== "http:" && url.protocol !== "https:") ||
      !url.host ||
      url.username ||
      url.password
    ) {
      return { kind: "blocked" };
    }
    return { kind: "external", href, hostname: url.hostname };
  } catch {
    return { kind: "blocked" };
  }
}

function SourceTitle({
  source,
  analytics,
}: {
  source: AssistantSource;
  analytics?: AssistantAnalyticsContext;
}) {
  const destination = safeSourceDestination(source.href);
  const onClick = () => {
    if (!analytics) return;
    trackAssistantEvent(
      "assistant_source_clicked",
      assistantEventProperties(analytics, { source_kind: source.type }),
    );
  };

  if (destination.kind === "internal") {
    return (
      <Link
        href={destination.href}
        onClick={onClick}
        className="font-semibold text-brand underline decoration-brand/30 underline-offset-2 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand"
      >
        {source.title}
      </Link>
    );
  }

  if (destination.kind === "external") {
    return (
      <a
        href={destination.href}
        onClick={onClick}
        target="_blank"
        rel="noopener noreferrer"
        className="font-semibold text-brand underline decoration-brand/30 underline-offset-2 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand"
      >
        {source.title}
        <span aria-hidden="true" className="ms-1 text-ink-500">
          ↗
        </span>
      </a>
    );
  }

  return <span className="font-semibold text-ink-900">{source.title}</span>;
}

export function AssistantSources({
  sources,
  labels,
  analytics,
}: AssistantSourcesProps) {
  const [expanded, setExpanded] = useState(false);
  const disclosureID = useId();
  const regionID = useId();

  if (sources.length === 0) return null;

  return (
    <div className="space-y-2">
      <button
        id={disclosureID}
        type="button"
        aria-expanded={expanded}
        aria-controls={regionID}
        onClick={() => setExpanded((current) => !current)}
        className="rounded-lg px-2 py-1 text-label font-medium text-ink-600 hover:bg-warm-100 hover:text-ink-900 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand"
      >
        {labels.usedSources(sources.length)}
      </button>

      {expanded ? (
        <div
          id={regionID}
          role="region"
          aria-label={labels.sourcesRegion(sources.length)}
          className="space-y-2"
        >
          {sources.map((source) => {
            const destination = safeSourceDestination(source.href);
            return (
              <article
                key={source.id}
                className="rounded-xl border border-warm-200 bg-warm-50 p-3"
              >
                <SourceTitle source={source} analytics={analytics} />
                <p className="mt-1 text-label text-ink-600">
                  {labels.sourceOrigin(source.origin)}
                </p>
                {destination.kind === "external" ? (
                  <p className="mt-1 text-label text-ink-500">
                    {labels.externalSource(destination.hostname)}
                  </p>
                ) : null}
              </article>
            );
          })}
        </div>
      ) : null}
    </div>
  );
}
