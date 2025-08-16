"use client";
import { useState, useEffect } from "react";

export const useOnboarding = () => {
    const [isOnboardingOpen, setIsOnboardingOpen] = useState(false);

    useEffect(() => {
        const hasSeenOnboarding = localStorage.getItem("hasSeenOnboarding");
        if (!hasSeenOnboarding) {
            setIsOnboardingOpen(true);
        }
    }, []);

    const closeOnboarding = () => {
        setIsOnboardingOpen(false);
        try {
            localStorage.setItem("hasSeenOnboarding", "true");
        } catch (error) {
            console.warn('Failed to save onboarding state:', error);
        }
    };

    return {
        isOnboardingOpen,
        closeOnboarding,
    };
};
