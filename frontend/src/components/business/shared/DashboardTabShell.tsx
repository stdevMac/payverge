"use client";

import React from "react";
import type { LucideIcon } from "lucide-react";
import PageHeader, {
  type PageHeaderStat,
  type PageHeaderStatus,
} from "./PageHeader";
import ActivationPanel, { type ActivationFeature } from "./ActivationPanel";
import SegmentedTabs, { type SegmentedTab } from "./SegmentedTabs";

interface TabShellHeader {
  title: string;
  /** Quiet uppercase label above the serif title. */
  eyebrow?: string;
  subtitle?: string;
  status?: PageHeaderStatus;
  stats?: PageHeaderStat[];
  actions?: React.ReactNode;
  /** Opt-in compact single-row header (Marketing only); forwarded to PageHeader. */
  dense?: boolean;
}

interface TabShellActivation {
  icon: LucideIcon;
  title: string;
  description: string;
  features?: ActivationFeature[];
  action: React.ReactNode;
  footnote?: string;
}

interface TabShellTabs {
  items: SegmentedTab[];
  activeKey: string;
  onChange: (key: string) => void;
  ariaLabel?: string;
}

interface DashboardTabShellProps {
  header: TabShellHeader;
  /**
   * Rendered access-gate view. Accepts `condition && <View />` so callers can
   * inline the ladder without ternaries. Wins over every other state.
   */
  locked?: React.ReactNode;
  /** Rendered skeleton while loading. Wins over activation and content. */
  loading?: React.ReactNode;
  /**
   * Feature-off state: the shell renders the header followed by the shared
   * ActivationPanel instead of tabs/content.
   */
  activation?: TabShellActivation | false | null;
  /**
   * Optional board-level chrome rendered between the page header and the
   * sub-tab rail (outside every tabpanel). Use for strips that must stay
   * scoped to the whole tab — e.g. Kitchen "Needs approval" — so they don't
   * look like they belong to Ready / All Orders empty states.
   */
  banner?: React.ReactNode;
  /** Optional sub-view strip rendered un-boxed between header and content. */
  tabs?: TabShellTabs;
  /** Page width. Grid-heavy tabs (Schedule) use "wide". */
  width?: "default" | "wide";
  /**
   * Fill the dashboard pane (`h-full min-h-0` flex column) so a child
   * scrollport — Reservations Tablero — can occupy remaining height
   * inside the `h-[100dvh] overflow-hidden` shell.
   */
  fill?: boolean;
  className?: string;
  children?: React.ReactNode;
}

const widthClass: Record<
  NonNullable<DashboardTabShellProps["width"]>,
  string
> = {
  default: "max-w-6xl",
  wide: "max-w-7xl",
};

/**
 * The one dashboard-tab layout. Every tab declares its anatomy — header,
 * access gate, loading skeleton, activation state, sub-tabs, content — and the
 * shell resolves the state ladder (locked → loading → activation → content)
 * and renders the identical structure everywhere. Changing tab anatomy means
 * changing this file, not 23 components.
 *
 * Tab contract — the rules every tab inherits by rendering through this shell:
 *
 *   1. The page header never disappears. Only `locked` (whole-tab access gate)
 *      bypasses the shell. Loading renders *inside* the content slot so the
 *      header and sub-tab rail stay mounted (S-9). EMPTY and DISABLED are NOT
 *      early returns: "disabled" is the `activation` prop (header +
 *      ActivationPanel), and "empty" is a content-level child (EmptyState with
 *      `panel`). A tab must never `return` its own empty/disabled view ahead of
 *      the shell and drop the header.
 *
 *   2. Serif budget. The PageHeader title and the ActivationPanel title are the
 *      ONLY serif (font-title) text on a tab. Section headings, empty states,
 *      and card titles below are all sans — don't spend the serif budget twice.
 *
 *   3. Counts live in ONE home. A number is either a header stat, a sub-tab
 *      badge, or a content KPI — never duplicated across two of them. Pick the
 *      home closest to where the operator acts on it.
 */
export default function DashboardTabShell({
  header,
  locked,
  loading,
  activation,
  banner,
  tabs,
  width = "default",
  fill = false,
  className = "",
  children,
}: DashboardTabShellProps) {
  const uid = React.useId();
  const container = [
    "mx-auto min-w-0 w-full",
    widthClass[width],
    // Fill must not use space-y-*: margins inflate min-content and starve a
    // flex-1 Tablero inside the overflow-hidden dashboard shell (#657).
    fill ? "flex h-full min-h-0 flex-col gap-3 p-4" : "space-y-5 p-4 sm:p-6",
    className,
  ]
    .filter(Boolean)
    .join(" ");
  const tabPanelClass = fill
    ? "flex min-h-0 min-w-0 flex-1 flex-col"
    : "min-w-0";

  if (locked) {
    return <>{locked}</>;
  }

  // Loading wins over activation but keeps header (+ sub-tabs) mounted (S-9).
  // Skeleton occupies the content slot only — never blanks the chrome.
  if (loading) {
    return (
      <div className={container}>
        <div className={fill ? "shrink-0" : undefined}>
          <PageHeader
            title={header.title}
            eyebrow={header.eyebrow}
            subtitle={header.subtitle}
            status={header.status}
            stats={header.stats}
            actions={header.actions}
            dense={header.dense}
          />
        </div>
        {banner ? (
          <div className={fill ? "shrink-0" : undefined}>{banner}</div>
        ) : null}
        {tabs ? (
          <div className={fill ? "shrink-0" : undefined}>
            <SegmentedTabs
              tabs={tabs.items}
              activeKey={tabs.activeKey}
              onChange={tabs.onChange}
              ariaLabel={tabs.ariaLabel ?? header.title}
              idPrefix={uid}
            />
          </div>
        ) : null}
        {tabs ? (
          <div
            role="tabpanel"
            id={`${uid}-panel-${tabs.activeKey}`}
            aria-labelledby={`${uid}-tab-${tabs.activeKey}`}
            className={tabPanelClass}
          >
            {loading}
          </div>
        ) : (
          loading
        )}
      </div>
    );
  }

  if (activation) {
    return (
      <div className={container}>
        <PageHeader
          title={header.title}
          eyebrow={header.eyebrow}
          subtitle={header.subtitle}
          status={header.status}
          actions={header.actions}
        />
        <ActivationPanel
          icon={activation.icon}
          title={activation.title}
          description={activation.description}
          features={activation.features}
          action={activation.action}
          footnote={activation.footnote}
        />
      </div>
    );
  }

  return (
    <div className={container}>
      <div className={fill ? "shrink-0" : undefined}>
        <PageHeader
          title={header.title}
          eyebrow={header.eyebrow}
          subtitle={header.subtitle}
          status={header.status}
          stats={header.stats}
          actions={header.actions}
          dense={header.dense}
        />
      </div>
      {banner ? (
        <div className={fill ? "shrink-0" : undefined}>{banner}</div>
      ) : null}
      {tabs ? (
        <div className={fill ? "shrink-0" : undefined}>
          <SegmentedTabs
            tabs={tabs.items}
            activeKey={tabs.activeKey}
            onChange={tabs.onChange}
            ariaLabel={tabs.ariaLabel ?? header.title}
            idPrefix={uid}
          />
        </div>
      ) : null}
      {tabs ? (
        <div
          role="tabpanel"
          id={`${uid}-panel-${tabs.activeKey}`}
          aria-labelledby={`${uid}-tab-${tabs.activeKey}`}
          className={tabPanelClass}
        >
          {children}
        </div>
      ) : (
        children
      )}
    </div>
  );
}
