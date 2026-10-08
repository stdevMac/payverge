"use client";

import React from "react";
import { Loader2, CheckCircle2, AlertCircle, Search } from "lucide-react";
import type { ToolCallEvent } from "@/hooks/useDirectorStream";
import {
  getTranslation,
  useSimpleLocale,
} from "@/i18n/SimpleTranslationProvider";
export interface ToolTraceProps {
  toolCalls: ToolCallEvent[];
  defaultCollapsed?: boolean;
}

// The trace narrates what Sage is checking, in the owner's language — not a
// debugger. We deliberately do NOT surface millisecond latencies or raw JSON
// argument dumps here: an owner wants reassurance ("checking your numbers"),
// not engineering telemetry. The pills are read-only status chips, not an
// inspectable drawer.
export default function ToolTrace({
  toolCalls,
  defaultCollapsed = false,
}: ToolTraceProps) {
  const { locale } = useSimpleLocale();
  const t = React.useCallback(
    (key: string, params?: Record<string, string | number>): string => {
      const value = getTranslation(
        `directorConsole.${key}`,
        locale,
        params,
      );
      return Array.isArray(value) ? value[0] || key : (value as string);
    },
    [locale],
  );
  if (toolCalls.length === 0) return null;

  // Pluralization is resolved in code (the i18n layer interpolates {count} but
  // does not pick singular/plural), so we select the matching key.
  const collapsedSummary =
    toolCalls.length === 1
      ? t("toolTrace.sourcesSingular", { count: 1 })
      : t("toolTrace.sourcesPlural", { count: toolCalls.length });

  return (
    <details
      open={!defaultCollapsed}
      className="rounded-lg border border-warm-200 bg-warm-50/60 px-3 py-2 text-body-sm"
    >
      <summary className="cursor-pointer text-label uppercase tracking-wide text-ink-500">
        {defaultCollapsed ? collapsedSummary : t("toolTrace.lookingUp")}
      </summary>
      <div className="mt-2 flex flex-wrap gap-2">
        {toolCalls.map((call) => (
          <ToolPill key={call.id} call={call} />
        ))}
      </div>
    </details>
  );
}

function ToolPill({ call }: { call: ToolCallEvent }) {
  const icon =
    call.state === "running" ? (
      <Loader2 className="w-3 h-3 animate-spin text-ink-400" />
    ) : call.state === "done" ? (
      <CheckCircle2 className="w-3 h-3 text-emerald-600" />
    ) : (
      <AlertCircle className="w-3 h-3 text-rose-600" />
    );
  return (
    <span className="inline-flex items-center gap-1.5 rounded-full border border-warm-200 bg-white px-2.5 py-1 text-xs text-ink-700">
      <Search className="w-3 h-3 text-ink-400" />
      <span className="font-medium">{call.human_label}</span>
      {call.summary ? (
        <span className="text-ink-500">· {call.summary}</span>
      ) : null}
      <span className="ml-0.5">{icon}</span>
    </span>
  );
}
