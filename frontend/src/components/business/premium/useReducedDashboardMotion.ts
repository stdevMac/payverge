"use client";

import { useReducedMotion } from "framer-motion";

export function useReducedDashboardMotion() {
  return useReducedMotion() ?? false;
}
