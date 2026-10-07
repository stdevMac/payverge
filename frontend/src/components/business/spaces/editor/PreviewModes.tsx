"use client";

import React from "react";
import type { PreviewMode } from "./types";

interface PreviewModesProps {
  mode: PreviewMode;
  onChange: (mode: PreviewMode) => void;
  t: (key: string) => string;
}

const MODES: PreviewMode[] = ["edit", "guest", "staff", "live"];

export function PreviewModes({ mode, onChange, t }: PreviewModesProps) {
  return (
    <div
      className="inline-flex rounded-full border border-warm-200 bg-white p-0.5 shadow-sm"
      role="tablist"
      aria-label={t("editor.previewModesAria")}
      data-testid="preview-modes"
    >
      {MODES.map((m) => {
        const active = mode === m;
        return (
          <button
            key={m}
            type="button"
            role="tab"
            aria-selected={active}
            data-testid={`preview-mode-${m}`}
            onClick={() => onChange(m)}
            className={`rounded-full px-2 py-1 text-xs font-medium transition-colors ${
              active
                ? "bg-brand text-white shadow-sm"
                : "text-ink-600 hover:bg-warm-50 hover:text-ink-900"
            }`}
          >
            {t(`editor.preview.${m}`)}
          </button>
        );
      })}
    </div>
  );
}
