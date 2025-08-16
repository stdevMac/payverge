"use client";

import { useEffect, useState } from "react";
import { useAccount } from "wagmi";
import { Spinner } from "@nextui-org/react";
import { useTranslation } from "@/i18n/useTranslation";

export const Web3Button = () => {
  const { isConnected, status } = useAccount();
  const [isLoading, setIsLoading] = useState(true);
  const [hasInitialized, setHasInitialized] = useState(false);
  const { t, language } = useTranslation();

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
        button.setAttribute('label', t('navigation.connect'));
        
        // For connected state, we can leave it empty to use the default address display
        button.setAttribute('connected-label', '');
      }
    };

    // Run once on mount and whenever language changes
    if (!isLoading && hasInitialized) {
      // Small delay to ensure the button is rendered
      setTimeout(updateAppkitButtonText, 100);
    }
  }, [language, isLoading, hasInitialized, t]);

  if (isLoading || status === 'reconnecting') {
    return (
      <div className="h-[40px] flex items-center justify-center px-4">
        <Spinner size="sm" />
      </div>
    );
  }

  return <appkit-button balance={isConnected ? "show" : "hide"} />;
};
