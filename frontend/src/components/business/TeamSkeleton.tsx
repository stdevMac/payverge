"use client";

import React from "react";
import { SkeletonList } from "@/components/ui/skeletons";
import { useSimpleLocale, getTranslation } from "@/i18n/SimpleTranslationProvider";

// The Team surfaces are card lists (staff rows, invitations), not a 6-column
// table — SkeletonList mirrors that shape so the load reads as a structured
// placeholder rather than dimmed/stale content. It renders into
// DashboardTabShell's loading slot, which already supplies the centered
// canvas and padding, so this owns no page geometry of its own. SkeletonList
// owns role="status" and the aria-label, matching sibling skeletons like
// CashRegisterSkeleton.
export function TeamSkeleton() {
  const { locale } = useSimpleLocale();
  return (
    <SkeletonList
      rows={4}
      ariaLabel={getTranslation("common.loading", locale) as string}
    />
  );
}
