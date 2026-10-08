"use client";

import React from "react";
import { motion } from "framer-motion";
import { dashboardSpring } from "./motion";
import { useReducedDashboardMotion } from "./useReducedDashboardMotion";

interface AnimatedBadgeProps {
  count?: number | null;
  label: string;
  /**
   * Visible cap for the badge text (default 9 → "9+").
   * Pass `null` or `Infinity` for no cap so the badge shows the exact total
   * (e.g. BillManager history matches the header `billTotal` source of truth).
   */
  cap?: number | null;
  className?: string;
  testId?: string;
  ariaHidden?: boolean;
}

function formatBadgeCount(count: number, cap: number | null | undefined): string {
  // null / non-finite → unlimited exact total (BillManager history vs header) [L2-16].
  // undefined keeps the default 9 (rail / unread-style badges stay compact).
  const effectiveCap = cap === undefined ? 9 : cap;
  if (
    effectiveCap == null ||
    !Number.isFinite(effectiveCap) ||
    count <= effectiveCap
  ) {
    return String(count);
  }
  return `${effectiveCap}+`;
}

export function AnimatedBadge({
  count,
  label,
  cap,
  className = "",
  testId,
  ariaHidden = false,
}: AnimatedBadgeProps) {
  const reduceMotion = useReducedDashboardMotion();

  if (!count || count <= 0) return null;

  const text = formatBadgeCount(count, cap);

  return (
    <motion.span
      key={count}
      data-testid={testId}
      aria-hidden={ariaHidden || undefined}
      aria-label={ariaHidden ? undefined : `${count} ${label}`}
      initial={reduceMotion ? false : { scale: 0.72, opacity: 0 }}
      animate={{ scale: 1, opacity: 1 }}
      transition={dashboardSpring}
      className={[
        "inline-flex h-[18px] min-w-[18px] items-center justify-center rounded-full bg-rose-600 px-1.5 text-[10px] font-bold leading-none text-white shadow-sm shadow-rose-900/15 tabular-nums",
        className,
      ]
        .filter(Boolean)
        .join(" ")}
    >
      {text}
    </motion.span>
  );
}
