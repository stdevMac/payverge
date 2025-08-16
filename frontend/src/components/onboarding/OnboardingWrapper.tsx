"use client";

import React from "react";
import { OnboardingModal } from "./OnboardingModal";
import { useOnboarding } from "@/hooks/useOnboarding";

export const OnboardingWrapper = () => {
  const { isOnboardingOpen, closeOnboarding } = useOnboarding();

  return (
    <OnboardingModal isOpen={isOnboardingOpen} onClose={closeOnboarding} />
  );
};
