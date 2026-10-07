"use client";

import React from "react";
import { motion } from "framer-motion";
import { useReducedDashboardMotion } from "./useReducedDashboardMotion";

type TransitionDirection = -1 | 0 | 1;

interface DashboardTabTransitionProps {
  tabKey: string;
  direction?: TransitionDirection;
  children: React.ReactNode;
}

// Deliberately NOT AnimatePresence + mode="wait": exit-gating the next tab on
// the previous tab's exit animation wedged permanently when the outgoing tree
// contained its own framer-motion/NextUI presence children (the dashboard went
// blank until a hard reload). The old tab unmounts instantly; the incoming tab
// plays a short, cheap enter tween. No `filter: blur()` here either — blurring
// the entire tab tree forced full-tree re-rasterization on every frame.
export function DashboardTabTransition({
  tabKey,
  direction = 1,
  children,
}: DashboardTabTransitionProps) {
  const reduceMotion = useReducedDashboardMotion();

  if (reduceMotion) {
    return <div className="flex h-full min-h-0 min-w-0 flex-col">{children}</div>;
  }

  return (
    <motion.div
      key={tabKey}
      initial={{ opacity: 0, x: direction === 0 ? 0 : direction * 14, y: 4 }}
      animate={{ opacity: 1, x: 0, y: 0 }}
      transition={{ duration: 0.22, ease: [0.16, 1, 0.3, 1] }}
      className="flex h-full min-h-0 min-w-0 flex-col"
    >
      {children}
    </motion.div>
  );
}
