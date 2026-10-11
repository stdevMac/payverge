"use client";

import React from "react";
import { Button } from "@nextui-org/react";
import { Save, Check } from "lucide-react";

interface SaveBarLabels {
  save: string;
  saving: string;
  unsaved: string;
  /** Honest autosave copy — only rendered when `mode === "auto"`. */
  auto: string;
  /** Idle copy for explicit-save (`mode === "button"`) when the form is clean. */
  clean: string;
}

interface SaveBarProps {
  mode: "button" | "auto";
  isSaving: boolean;
  dirty: boolean;
  onSave: () => void;
  labels: SaveBarLabels;
}

export default function SaveBar({ mode, isSaving, dirty, onSave, labels }: SaveBarProps) {
  return (
    <div className="sticky bottom-0 z-20 mt-8 flex items-center justify-end gap-3 border-t border-warm-200/80 bg-white/85 px-4 py-3 shadow-[0_-4px_16px_rgba(46,42,37,0.06)] backdrop-blur-xl">
      {mode === "auto" ? (
        <span className="inline-flex items-center gap-1.5 rounded-full border border-emerald-200 bg-emerald-50/90 px-3 py-1.5 text-sm font-medium text-emerald-800 shadow-sm shadow-emerald-900/10">
          <Check className="w-4 h-4 text-emerald-600" aria-hidden="true" />
          {labels.auto}
        </span>
      ) : (
        <>
          {dirty ? (
            <span className="rounded-full border border-amber-200 bg-amber-50/90 px-3 py-1.5 text-sm font-medium text-amber-800 shadow-sm shadow-amber-900/10">
              {labels.unsaved}
            </span>
          ) : labels.clean ? (
            <span
              data-testid="save-bar-all-saved"
              className="inline-flex items-center gap-1.5 rounded-full border border-warm-200 bg-warm-50/90 px-3 py-1.5 text-sm font-medium text-ink-600"
            >
              {labels.clean}
            </span>
          ) : null}
          <Button
            radius="full"
            className="bg-brand font-medium text-white hover:bg-brand-dark"
            startContent={<Save className="w-4 h-4" />}
            isLoading={isSaving}
            isDisabled={!dirty || isSaving}
            onPress={onSave}
            aria-disabled={!dirty || isSaving}
            title={!dirty ? labels.clean || undefined : undefined}
          >
            {isSaving ? labels.saving : labels.save}
          </Button>
        </>
      )}
    </div>
  );
}
