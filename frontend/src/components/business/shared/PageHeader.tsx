"use client";

import React from "react";

type PageHeaderStatusTone = "positive" | "neutral" | "attention";

export interface PageHeaderStatus {
  label: string;
  tone?: PageHeaderStatusTone;
}

export interface PageHeaderStat {
  label: string;
  value: React.ReactNode;
  /** Optional native tooltip clarifying what the stat counts. */
  title?: string;
  /**
   * When set, the stat renders as a quiet text button (e.g. scroll to the
   * approval inbox). Keep the visual weight identical to a plain stat.
   */
  onClick?: () => void;
}

interface PageHeaderProps {
  title: string;
  /** Quiet uppercase label above the serif title (e.g. parent rail name). */
  eyebrow?: string;
  subtitle?: string;
  /** Single quiet dot + word (Published / Draft / Paused). Never a chip row. */
  status?: PageHeaderStatus;
  /**
   * Quiet inline meta line under the title. Pass only stats that carry real
   * information — zeros, UI state, and anything repeated elsewhere on the
   * page should be omitted by the caller.
   */
  stats?: PageHeaderStat[];
  /** Right-aligned actions: one primary button plus at most a couple of icon buttons. */
  actions?: React.ReactNode;
  /** Opt-in compact header: title, stat chips, and actions share one wrapping
   *  row. Only Marketing opts in; every other tab keeps the default layout. */
  dense?: boolean;
  className?: string;
}

const statusDotClass: Record<PageHeaderStatusTone, string> = {
  positive: "bg-brand",
  neutral: "bg-warm-400",
  attention: "bg-amber-500",
};

/**
 * Un-boxed page header shared by every dashboard tab: serif title directly on
 * the canvas, one-line subtitle, right-aligned actions, and an optional
 * dot-separated text stat line. Intentionally NOT wrapped in a panel — the
 * header breathes while content below lives in at most one level of panels.
 *
 * This title is one of the two serif slots the tab contract allows (see
 * DashboardTabShell); the other belongs to ActivationPanel.
 */
export default function PageHeader({
  title,
  eyebrow,
  subtitle,
  status,
  stats,
  actions,
  dense = false,
  className = "",
}: PageHeaderProps) {
  const visibleStats = stats?.filter(Boolean) ?? [];

  if (dense) {
    return (
      <header
        data-dense="true"
        className={["flex flex-wrap items-center gap-x-4 gap-y-2", className]
          .filter(Boolean)
          .join(" ")}
      >
        <div className="flex min-w-0 flex-wrap items-center gap-x-3 gap-y-1">
          {eyebrow ? (
            <p className="w-full text-label uppercase tracking-wide text-ink-500">
              {eyebrow}
            </p>
          ) : null}
          <h1 className="font-title text-xl tracking-tight text-ink-950 sm:text-2xl">
            {title}
          </h1>
          {status ? (
            <span className="inline-flex items-center gap-1.5 text-xs font-medium text-ink-700">
              <span
                aria-hidden="true"
                className={[
                  "h-1.5 w-1.5 rounded-full",
                  statusDotClass[status.tone ?? "neutral"],
                ].join(" ")}
              />
              {status.label}
            </span>
          ) : null}
          {visibleStats.length > 0 ? (
            <span className="flex flex-wrap items-center gap-x-2.5 gap-y-1 text-sm text-ink-600">
              {visibleStats.map((stat, index) => (
                <React.Fragment key={stat.label}>
                  {index > 0 || status ? (
                    <span aria-hidden="true" className="text-warm-600">
                      ·
                    </span>
                  ) : null}
                  <span
                    className="inline-flex items-baseline gap-1.5 whitespace-nowrap"
                    title={stat.title}
                  >
                    <span className="font-semibold tabular-nums text-ink-800">{stat.value}</span>
                    <span>{stat.label}</span>
                  </span>
                </React.Fragment>
              ))}
            </span>
          ) : null}
        </div>
        {actions ? (
          <div
            data-page-header-actions=""
            className="ml-auto flex min-w-0 max-w-full flex-wrap items-center justify-end gap-2"
          >
            {actions}
          </div>
        ) : null}
      </header>
    );
  }

  return (
    <header className={["flex flex-col gap-3", className].filter(Boolean).join(" ")}>
      <div className="flex min-w-0 w-full flex-wrap items-start justify-between gap-x-6 gap-y-3">
        <div className="min-w-0 max-w-full">
          {eyebrow ? (
            <p className="mb-1 text-label uppercase tracking-wide text-ink-500">
              {eyebrow}
            </p>
          ) : null}
          <div className="flex flex-wrap items-center gap-x-3 gap-y-1">
            <h1 className="font-title text-2xl tracking-tight text-ink-950 sm:text-3xl">
              {title}
            </h1>
            {status ? (
              <span className="inline-flex items-center gap-1.5 text-xs font-medium text-ink-700">
                <span
                  aria-hidden="true"
                  className={[
                    "h-1.5 w-1.5 rounded-full",
                    statusDotClass[status.tone ?? "neutral"],
                  ].join(" ")}
                />
                {status.label}
              </span>
            ) : null}
          </div>
          {subtitle ? (
            <p className="mt-1 max-w-2xl text-sm leading-6 text-ink-700">{subtitle}</p>
          ) : null}
        </div>
        {actions ? (
          <div
            data-page-header-actions=""
            className="flex min-w-0 max-w-full flex-wrap items-center justify-end gap-2"
          >
            {actions}
          </div>
        ) : null}
      </div>
      {visibleStats.length > 0 ? (
        <p className="flex flex-wrap items-center gap-x-2.5 gap-y-1 text-sm text-ink-600">
          {visibleStats.map((stat, index) => (
            <React.Fragment key={stat.label}>
              {index > 0 ? (
                <span aria-hidden="true" className="text-warm-600">
                  ·
                </span>
              ) : null}
              {stat.onClick ? (
                <button
                  type="button"
                  onClick={stat.onClick}
                  title={stat.title}
                  className="inline-flex items-baseline gap-1.5 whitespace-nowrap rounded-sm text-left underline-offset-2 hover:underline focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand"
                >
                  <span className="font-semibold tabular-nums text-ink-800">
                    {stat.value}
                  </span>
                  <span>{stat.label}</span>
                </button>
              ) : (
                <span
                  className="inline-flex items-baseline gap-1.5 whitespace-nowrap"
                  title={stat.title}
                >
                  <span className="font-semibold tabular-nums text-ink-800">
                    {stat.value}
                  </span>
                  <span>{stat.label}</span>
                </span>
              )}
            </React.Fragment>
          ))}
        </p>
      ) : null}
    </header>
  );
}
