import React from "react";

/**
 * Shared enable-vs-save footer for plugin config panels (L6-31).
 * TelegramConfig's branch (is_enabled → Save, else → Enable) is the norm.
 */
export type PluginConfigFooterProps = {
  onSave: () => void;
  onCancel: () => void;
  /** When true, primary CTA is Save; when false, Enable. */
  isEnabled: boolean;
  disabled?: boolean;
  cancelLabel: string;
  saveLabel: string;
  enableLabel: string;
  className?: string;
  buttonClassName?: string;
  cancelClassName?: string;
};

export function PluginConfigFooter({
  onSave,
  onCancel,
  isEnabled,
  disabled = false,
  cancelLabel,
  saveLabel,
  enableLabel,
  className = "flex gap-3 justify-end",
  buttonClassName = "rounded-xl bg-brand px-4 py-2 text-sm font-semibold text-white shadow-sm shadow-brand/20 transition-colors hover:bg-brand-dark focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-dark focus-visible:ring-offset-2 disabled:cursor-not-allowed disabled:opacity-50",
  cancelClassName = "rounded-xl px-4 py-2 text-sm font-semibold text-ink-600 transition-colors hover:bg-warm-100 hover:text-ink-950",
}: PluginConfigFooterProps) {
  return (
    <div className={className}>
      <button type="button" onClick={onCancel} className={cancelClassName}>
        {cancelLabel}
      </button>
      <button
        type="button"
        onClick={onSave}
        disabled={disabled}
        className={buttonClassName}
      >
        {isEnabled ? saveLabel : enableLabel}
      </button>
    </div>
  );
}
