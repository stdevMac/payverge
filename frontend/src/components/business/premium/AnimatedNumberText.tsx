"use client";

import React, { useEffect, useLayoutEffect, useRef, useState } from "react";
import { useSpring } from "framer-motion";
import { useReducedDashboardMotion } from "./useReducedDashboardMotion";

interface AnimatedNumberTextProps {
  value: number;
  format?: (value: number) => string;
  className?: string;
}

/**
 * Animated numeric display for dashboard KPIs.
 *
 * First paint with real data must show the fully formatted final value
 * immediately (no spring-from-zero). Live QA mistook mid-spring intermediates
 * for currency bugs. Animation is only used on subsequent value changes
 * (date-range switch, refetch) after a non-zero baseline is already showing.
 */
export function AnimatedNumberText({
  value,
  format = (next) => String(Math.round(next)),
  className = "",
}: AnimatedNumberTextProps) {
  const reduceMotion = useReducedDashboardMotion();
  const formatRef = useRef(format);
  formatRef.current = format;

  // Standalone spring (number source) so we fully control jump vs. set.
  const spring = useSpring(value, { stiffness: 120, damping: 24, mass: 0.8 });
  // State-backed text guarantees the final format is in the DOM on first paint
  // (MotionValue children can lag a frame under spring attach).
  const [text, setText] = useState(() => format(value));
  const prevValueRef = useRef<number | null>(null);

  useLayoutEffect(() => {
    const prev = prevValueRef.current;
    // Jump (no tween) when:
    // - first commit / mount
    // - reduced motion preferred
    // - leaving a zero baseline (query load → first real data) so intermediates
    //   never look like a uniformly scaled false currency amount
    const shouldJump =
      reduceMotion ||
      prev === null ||
      (prev === 0 && value !== 0);

    prevValueRef.current = value;

    if (shouldJump) {
      spring.jump(value);
      setText(formatRef.current(value));
      return;
    }

    spring.set(value);
  }, [spring, value, reduceMotion]);

  useEffect(() => {
    return spring.on("change", (latest) => {
      setText(formatRef.current(latest));
    });
  }, [spring]);

  return <span className={className}>{text}</span>;
}
