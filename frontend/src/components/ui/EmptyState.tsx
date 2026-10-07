import React from "react";
import type { LucideIcon } from "lucide-react";
import IconTile from "@/components/ui/IconTile";
import { btnPrimary } from "@/components/ui/buttonStyles";

interface EmptyStateProps {
  icon: LucideIcon;
  title: string;
  /** Canonical description line. */
  subtitle: string;
  /** Simple built-in CTA: a link (with actionHref) or a button (with onAction). */
  actionLabel?: string;
  onAction?: () => void;
  actionHref?: string;
  /**
   * Caller-owned action node — an alternative to actionLabel/onAction/actionHref
   * for when the CTA is a custom element (NextUI Button, front-door widget, a
   * cluster of buttons). When provided it takes precedence over the built-in
   * button. Use a buttonStyles recipe (btnPrimary / btnSecondary) inside.
   */
  action?: React.ReactNode;
  /** Quiet footnote rendered under the action. */
  hint?: string;
  /**
   * Render the white-panel treatment (tinted icon tile inside a bordered white
   * card) instead of the bare centered stack. This is the former TabEmptyState
   * look — use it for in-tab empties (empty tables/filtered lists) that should
   * read as a panel rather than owning a full screen.
   */
  panel?: boolean;
  className?: string;
  "data-testid"?: string;
  /**
   * Tighter spacing + smaller icon for empties that sit inside a panel or an
   * inbox tab rather than owning a full screen. Avoids the full-height "giant
   * empty card" stack when several sections are nothing-to-show at once.
   */
  compact?: boolean;
}

// Inlined from PremiumPanel (as="section", tone="default", withTexture={false})
// so the panel look matches the dashboard's other panels exactly without the
// ui/ tier reaching back into the business/premium tier.
const PANEL_CLASSES =
  "relative overflow-hidden rounded-2xl border before:pointer-events-none before:absolute before:inset-x-0 before:top-0 before:h-px before:bg-white/80 border-warm-200/90 bg-white shadow-card";

export function EmptyState({
  icon,
  title,
  subtitle,
  actionLabel,
  onAction,
  actionHref,
  action,
  hint,
  panel = false,
  className = "",
  "data-testid": dataTestId,
  compact = false,
}: EmptyStateProps) {
  const iconTile = <IconTile icon={icon} size={compact ? "md" : "lg"} />;
  const heading = (
    <h3 className="mt-4 text-base font-semibold text-ink-900">{title}</h3>
  );

  const actionArea = action ? (
    <div className="mt-5">{action}</div>
  ) : actionLabel && actionHref ? (
    <a href={actionHref} className={`mt-5 ${btnPrimary}`}>
      {actionLabel}
    </a>
  ) : actionLabel && onAction ? (
    <button type="button" onClick={onAction} className={`mt-5 ${btnPrimary}`}>
      {actionLabel}
    </button>
  ) : null;

  const hintNode = hint ? (
    <p className="mt-4 text-xs text-ink-500">{hint}</p>
  ) : null;

  if (panel) {
    return (
      <section
        data-testid={dataTestId}
        className={`${PANEL_CLASSES} flex flex-col items-center px-6 text-center ${
          compact ? "py-8" : "py-14 sm:py-16"
        } ${className}`.trim()}
      >
        {iconTile}
        {heading}
        <p className="mt-1 max-w-sm text-sm leading-6 text-ink-500">
          {subtitle}
        </p>
        {actionArea}
        {hintNode}
      </section>
    );
  }

  return (
    <div
      data-testid={dataTestId}
      className={`flex flex-col items-center justify-center text-center ${
        compact ? "py-8" : "py-16"
      } ${className}`}
    >
      {iconTile}
      {heading}
      <p className="mt-1 max-w-sm text-sm text-ink-500">{subtitle}</p>
      {actionArea}
      {hintNode}
    </div>
  );
}
