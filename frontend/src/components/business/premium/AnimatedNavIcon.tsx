"use client";

import React from "react";
import { motion } from "framer-motion";
import type { LucideIcon } from "lucide-react";
import { dashboardSpring } from "./motion";
import { useReducedDashboardMotion } from "./useReducedDashboardMotion";

interface AnimatedNavIconProps {
  icon: LucideIcon;
  active: boolean;
  label: string;
  className?: string;
  "data-testid"?: string;
}

/**
 * Shared sidebar icon treatment: a small, purposeful lift when a destination
 * becomes active. The icon remains decorative because its button owns the
 * accessible name; reduced-motion users receive the same color/state cue
 * without a transform animation.
 */
export function AnimatedNavIcon({
  icon: Icon,
  active,
  label,
  className = "",
  "data-testid": dataTestId,
}: AnimatedNavIconProps) {
  const reduceMotion = useReducedDashboardMotion();

  return (
    <motion.span
      data-testid={dataTestId}
      data-nav-icon="true"
      aria-hidden="true"
      title={undefined}
      initial={false}
      animate={
        reduceMotion
          ? { scale: 1, rotate: 0, y: 0 }
          : { scale: active ? 1.05 : 1, rotate: active ? -2 : 0, y: active ? -0.5 : 0 }
      }
      transition={reduceMotion ? { duration: 0 } : dashboardSpring}
      className={`inline-flex h-4 w-4 shrink-0 items-center justify-center transition-colors duration-200 ${active ? "text-brand" : "text-ink-400 group-hover:text-ink-600"} ${className}`}
      data-label={label}
    >
      <Icon className="h-4 w-4" strokeWidth={2} aria-hidden="true" />
    </motion.span>
  );
}
