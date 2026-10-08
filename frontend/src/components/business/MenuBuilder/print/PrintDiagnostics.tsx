"use client";

import React from "react";

import type {
  MenuPrintDiagnostic,
  PlannedMenuDocument,
} from "@/lib/menuPrint/types";

export interface PrintDiagnosticsProps {
  document: PlannedMenuDocument;
  tString: (key: string) => string;
  onNavigate: (target: MenuPrintDiagnostic["target"]) => void;
}

function interpolate(
  source: string,
  values: MenuPrintDiagnostic["values"],
): string {
  return Object.entries(values ?? {}).reduce(
    (message, [key, value]) => message.replaceAll(`{${key}}`, String(value)),
    source,
  );
}

function DiagnosticGroup({
  label,
  tone,
  diagnostics,
  tString,
  onNavigate,
}: {
  label: string;
  tone: "blocked" | "warning";
  diagnostics: MenuPrintDiagnostic[];
  tString: (key: string) => string;
  onNavigate: PrintDiagnosticsProps["onNavigate"];
}) {
  if (diagnostics.length === 0) return null;
  return (
    <section className="space-y-2">
      <h4 className="text-xs font-semibold uppercase tracking-wide text-ink-600">
        {label}
      </h4>
      <ul className="space-y-2">
        {diagnostics.map((diagnostic) => (
          <li key={diagnostic.id}>
            <button
              type="button"
              onClick={() => onNavigate(diagnostic.target)}
              className={[
                "w-full rounded-lg border border-warm-200 border-s-2 bg-white px-3 py-2 text-left text-sm leading-relaxed text-ink-800",
                "transition-colors hover:border-warm-300 hover:bg-warm-50 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand",
                tone === "blocked" ? "border-s-rose-400" : "border-s-amber-400",
              ].join(" ")}
            >
              {interpolate(tString(diagnostic.messageKey), diagnostic.values)}
            </button>
          </li>
        ))}
      </ul>
    </section>
  );
}

/**
 * Plain-language notes about the menu in front of the operator. No readiness
 * jargon, no scores, no raw measurements — just what to fix and what to look at.
 */
export function PrintDiagnostics({
  document,
  tString,
  onNavigate,
}: PrintDiagnosticsProps) {
  const warnings = document.diagnostics.filter(
    ({ severity }) => severity === "warning",
  );
  const blockers = document.diagnostics.filter(
    ({ severity }) => severity === "blocked",
  );

  return (
    <aside
      aria-label={tString("print.notes.label")}
      className="space-y-4 rounded-xl border border-warm-200 bg-warm-50 p-4"
    >
      <h3 className="text-sm font-semibold text-ink-900">
        {tString("print.notes.label")}
      </h3>
      {document.diagnostics.length === 0 ? (
        <p className="text-sm leading-relaxed text-ink-600">
          {tString("print.diagnostics.ready")}
        </p>
      ) : (
        <div className="space-y-4">
          <DiagnosticGroup
            label={tString("print.diagnostics.blockers")}
            tone="blocked"
            diagnostics={blockers}
            tString={tString}
            onNavigate={onNavigate}
          />
          <DiagnosticGroup
            label={tString("print.diagnostics.warnings")}
            tone="warning"
            diagnostics={warnings}
            tString={tString}
            onNavigate={onNavigate}
          />
        </div>
      )}
    </aside>
  );
}
