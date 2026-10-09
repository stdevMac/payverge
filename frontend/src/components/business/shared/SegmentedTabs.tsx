"use client";

import React from "react";
import type { LucideIcon } from "lucide-react";
import { PremiumSegmentedTabs } from "../premium";

export interface SegmentedTab {
  key: string;
  label: string;
  icon?: LucideIcon;
  /** Optional count badge (e.g. pending approvals). 0/undefined hides it. */
  badge?: number;
  /**
   * Optional badge cap override. Pass `null` for unlimited (exact total).
   * Omit for AnimatedBadge default (9+). Used by BillManager history/active.
   */
  badgeCap?: number | null;
}

interface SegmentedTabsProps {
  tabs: SegmentedTab[];
  activeKey: string;
  onChange: (key: string) => void;
  size?: "sm" | "md";
  className?: string;
  ariaLabel?: string;
  /** Wires tabs to their tabpanels (`${idPrefix}-tab/-panel-${key}`). */
  idPrefix?: string;
}

// Compatibility wrapper: business tabs keep importing SegmentedTabs while the
// rendered control gets the premium animated active pill and badge treatment.
export default function SegmentedTabs(props: SegmentedTabsProps) {
  return <PremiumSegmentedTabs {...props} />;
}
