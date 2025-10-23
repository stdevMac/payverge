"use client";

import { useEffect, useState } from "react";
import { useAccount } from "wagmi";
import { Spinner } from "@nextui-org/react";
import { useSimpleLocale, getTranslation } from "@/i18n/SimpleTranslationProvider";

export const Web3Button = () => {
  const { isConnected, status } = useAccount();
  const [isLoading, setIsLoading] = useState(true);
  const [hasInitialized, setHasInitialized] = useState(false);
  const { locale } = useSimpleLocale();
  const [currentLocale, setCurrentLocale] = useState(locale);

  // Update translations when locale changes
  useEffect(() => {
    setCurrentLocale(locale);
  }, [locale]);

  // Translation helper
  const tString = (key: string): string => {
    const fullKey = `navigation.${key}`;
    const result = getTranslation(fullKey, currentLocale);
    return Array.isArray(result) ? result[0] || key : result as string;
  };

  useEffect(() => {
    // Only set loading to false when we have a definitive wallet state
    if (status !== 'reconnecting' && !hasInitialized) {
      const timer = setTimeout(() => {
        setIsLoading(false);
        setHasInitialized(true);
      }, 500);
      return () => clearTimeout(timer);
    }
  }, [status, hasInitialized]);

  useEffect(() => {
    // Update the appkit-button text based on the current language
    const updateAppkitButtonText = () => {
      const button = document.querySelector('appkit-button');
      if (button) {
        // Always set the label based on the current language
        button.setAttribute('label', tString('connect'));
        
        // For connected state, we can leave it empty to use the default address display
        button.setAttribute('connected-label', '');
      }
    };

    // Run once on mount and whenever language changes
    if (!isLoading && hasInitialized) {
      // Small delay to ensure the button is rendered
      setTimeout(updateAppkitButtonText, 100);
    }
  }, [currentLocale, isLoading, hasInitialized, tString]);

  if (isLoading || status === 'reconnecting') {
    return (
      <div className="h-[40px] flex items-center justify-center px-4">
        <Spinner size="sm" />
      </div>
    );
  }

  return <appkit-button balance={isConnected ? "show" : "hide"} />;
};
