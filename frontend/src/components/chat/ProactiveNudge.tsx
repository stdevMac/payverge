"use client";

import React from "react";
import { motion, useReducedMotion } from "framer-motion";
import { Sparkles, X } from "lucide-react";

/**
 * ProactiveNudge
 *
 * A small, presentational, closeable teaser card that chat widgets show near
 * their floating action button to invite a visitor to talk to the AI. This
 * component only renders the animated card — it owns no timing, persistence,
 * or dismissal state. Parents decide when to show/hide it (e.g. idle-time
 * heuristics, localStorage/cookie dismissal memory) and are expected to
 * mount/unmount this component inside an `AnimatePresence` so the exit
 * animation runs.
 */
export interface ProactiveNudgeProps {
  message: string;
  /** Localized aria-label for the ✕ close button. */
  dismissLabel: string;
  /** Fired when the visitor clicks the card body (parent opens the chat). */
  onEngage: () => void;
  /** Fired when the visitor clicks ✕ (parent hides + persists dismissal). */
  onDismiss: () => void;
  /** Positioning classes from the parent (e.g. fixed/absolute + offsets). */
  className?: string;
}

export default function ProactiveNudge({
  message,
  dismissLabel,
  onEngage,
  onDismiss,
  className,
}: ProactiveNudgeProps) {
  const reduceMotion = useReducedMotion();

  const motionProps = reduceMotion
    ? {
        initial: { opacity: 0 },
        animate: { opacity: 1 },
        exit: { opacity: 0 },
      }
    : {
        initial: { opacity: 0, y: 16, scale: 0.95 },
        animate: { opacity: 1, y: 0, scale: 1 },
        exit: { opacity: 0, y: 8, scale: 0.97 },
        transition: { type: "spring", stiffness: 380, damping: 26 },
      };

  return (
    <motion.div
      role="status"
      className={`w-[min(280px,calc(100vw-3rem))] ${className ?? ""}`}
      {...motionProps}
    >
      <div className="relative rounded-2xl border border-warm-200 bg-white shadow-xl shadow-ink-900/10">
        <button
          type="button"
          onClick={onDismiss}
          aria-label={dismissLabel}
          title={dismissLabel}
          className="absolute -top-2 -end-2 flex h-6 w-6 items-center justify-center rounded-full border border-warm-200 bg-white text-ink-400 shadow-sm transition-colors hover:text-ink-700"
        >
          <X className="h-3.5 w-3.5" />
        </button>
        <button
          type="button"
          onClick={onEngage}
          className="block w-full rounded-2xl px-4 py-3 text-start transition-colors hover:bg-warm-50/70"
        >
          <span className="flex items-start gap-2.5">
            <span className="mt-0.5 flex h-7 w-7 shrink-0 items-center justify-center rounded-full bg-brand/10 text-brand">
              {/* one-time attention wiggle after the entrance settles */}
              {reduceMotion ? (
                <Sparkles className="h-3.5 w-3.5" />
              ) : (
                <motion.span
                  animate={{ rotate: [0, -10, 10, -4, 0] }}
                  transition={{ delay: 0.7, duration: 0.9, ease: "easeInOut" }}
                  className="flex"
                >
                  <Sparkles className="h-3.5 w-3.5" />
                </motion.span>
              )}
            </span>
            <span className="text-sm leading-snug text-ink-800">{message}</span>
          </span>
        </button>
      </div>
    </motion.div>
  );
}
