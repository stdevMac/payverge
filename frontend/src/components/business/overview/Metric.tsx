"use client";

import React from "react";
import { ArrowDownRight, ArrowUpRight } from "lucide-react";
import { AnimatedNumberText, PremiumPanel } from "../premium";

type MetricSize = "hero" | "inline" | "card";
type MetricVariant = "default" | "accent" | "urgent";

interface MetricProps {
  size: MetricSize;
  label: string;
  value: string | React.ReactNode;
  hint?: string;
  delta?: { value: string; positive: boolean };
  icon?: React.ReactNode;
  variant?: MetricVariant;
  loading?: boolean;
  onClick?: () => void;
  href?: string;
}

const VARIANT_CLASSES: Record<MetricVariant, string> = {
  default: "",
  accent: "",
  urgent: "",
};

function renderMetricValue(value: string | React.ReactNode) {
  if (typeof value !== "string") return value;
  const normalized = value.replace(/,/g, "");
  const numeric = Number(normalized);
  if (!Number.isFinite(numeric) || normalized.trim() === "") return value;
  // Preserve the caller's decimal precision: "3.4" animates to "3.4", not "3".
  // Integer inputs keep the original Math.round formatting exactly.
  const decimals = normalized.includes(".")
    ? normalized.split(".")[1]?.length ?? 0
    : 0;
  return (
    <AnimatedNumberText
      value={numeric}
      format={(next) =>
        decimals > 0 ? next.toFixed(decimals) : String(Math.round(next))
      }
    />
  );
}

export default function Metric({
  size,
  label,
  value,
  hint,
  delta,
  icon,
  variant = "default",
  loading,
  onClick,
  href,
}: MetricProps) {
  const interactive = !!onClick || !!href;
  const isHero = size === "hero";
  const isCard = size === "card";
  const isInline = size === "inline";

  const hoverClass = interactive
    ? size === "inline"
      ? "hover:border-warm-300"
      : "hover:shadow-md"
    : "";

  const wrapperClass = [
    "transition-all",
    VARIANT_CLASSES[variant],
    isHero ? "flex flex-col px-8 py-10" : "",
    isCard ? "flex flex-col px-5 py-6" : "",
    isInline ? "flex items-center gap-3 px-4 py-3 rounded-xl" : "",
    hoverClass,
    interactive && !isInline
      ? "focus:outline-none focus-visible:ring-2 focus-visible:ring-brand"
      : "",
  ]
    .filter(Boolean)
    .join(" ");

  const skeletonHeight = isHero ? "h-[4.5rem] w-48" : isCard ? "h-9 w-24" : "h-[1.625rem] w-16";

  const inner = (
    <>
      {isInline && icon ? (
        <span className="flex-shrink-0 w-9 h-9 rounded-lg bg-warm-50 text-ink-600 flex items-center justify-center">
          {icon}
        </span>
      ) : null}

      <div className={isInline ? "flex flex-col min-w-0" : "flex flex-col"}>
        <span
          className={`text-label uppercase text-ink-500 ${
            isInline ? "line-clamp-2 min-h-[2rem]" : ""
          }`}
        >
          {label}
        </span>

        <span
          className={
            isHero
              ? "mt-2 text-display-2xl font-semibold text-ink-950 tabular-nums"
              : isCard
              ? "mt-2 text-display-md font-semibold text-ink-950 tabular-nums"
              : "text-heading-md font-semibold text-ink-900 tabular-nums"
          }
        >
          {loading ? (
            <span className={`inline-block rounded bg-warm-100 animate-pulse ${skeletonHeight}`} />
          ) : (
            renderMetricValue(value)
          )}
        </span>

        {hint && !loading ? <p className="text-body-sm text-ink-500 mt-1">{hint}</p> : null}

        {delta && !loading ? (
          <span
            className={`mt-3 inline-flex items-center gap-1 text-body-sm font-semibold ${
              delta.positive ? "text-brand" : "text-rose-700"
            }`}
          >
            {delta.positive ? (
              <ArrowUpRight className="w-4 h-4" aria-hidden="true" />
            ) : (
              <ArrowDownRight className="w-4 h-4" aria-hidden="true" />
            )}
            {delta.value}
          </span>
        ) : null}
      </div>
    </>
  );

  const interactiveClass = interactive ? `${wrapperClass} text-left w-full` : wrapperClass;

  if (href) {
    return (
      <PremiumPanel
        as="a"
        href={href}
        interactive
        className={interactiveClass}
        tone={variant === "urgent" ? "urgent" : variant === "accent" ? "accent" : "default"}
        withTexture={size !== "inline"}
        data-premium-panel="true"
        data-variant={variant}
        data-size={size}
      >
        {inner}
      </PremiumPanel>
    );
  }
  if (onClick) {
    return (
      <PremiumPanel
        as="button"
        type="button"
        onClick={onClick}
        interactive
        className={interactiveClass}
        tone={variant === "urgent" ? "urgent" : variant === "accent" ? "accent" : "default"}
        withTexture={size !== "inline"}
        data-premium-panel="true"
        data-variant={variant}
        data-size={size}
      >
        {inner}
      </PremiumPanel>
    );
  }
  return (
    <PremiumPanel
      className={wrapperClass}
      tone={variant === "urgent" ? "urgent" : variant === "accent" ? "accent" : "default"}
      withTexture={size !== "inline"}
      data-premium-panel="true"
      data-variant={variant}
      data-size={size}
    >
      {inner}
    </PremiumPanel>
  );
}
