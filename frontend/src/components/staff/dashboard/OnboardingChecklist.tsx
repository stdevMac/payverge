"use client";

import React from "react";
import ChecklistRunnerContainer from "./ChecklistRunnerContainer";
import type { ChecklistRunnerLabels } from "./ChecklistRunner";

// The staff "More → Onboarding" surface. Reuses ChecklistRunnerContainer but
// narrows to the new-hire (non-shift) runs assigned to the caller and frames
// them with onboarding-specific copy. NO money on this wire.

export interface OnboardingChecklistProps {
  businessId: string;
  locale: string;
  labels: ChecklistRunnerLabels;
}

export default function OnboardingChecklist({ businessId, locale, labels }: OnboardingChecklistProps) {
  return (
    <ChecklistRunnerContainer businessId={businessId} locale={locale} labels={labels} onboardingOnly />
  );
}
