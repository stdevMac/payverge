'use client';

import React, { useState, useEffect, useCallback } from 'react';
import { useSimpleLocale, getTranslation } from '@/i18n/SimpleTranslationProvider';
import { useDisclosure } from '@nextui-org/react';
import { useRegistrationFee } from '@/contracts/hooks';
import { SubscriptionData, SubscriptionManagementProps } from './types';
import { SubscriptionStatusCard } from './SubscriptionStatusCard';
import { RenewalModal } from './RenewalModal';

export const SubscriptionManagement: React.FC<SubscriptionManagementProps> = ({
  subscriptionData,
  onRenewSubscription,
  onToggleAutoRenewal,
}) => {
  // Translation setup
  const { locale } = useSimpleLocale();
  const [currentLocale, setCurrentLocale] = useState(locale);
  
  // Update translations when locale changes
  useEffect(() => {
    setCurrentLocale(locale);
  }, [locale]);
  
  // Translation helper
  const tString = useCallback((key: string): string => {
    const fullKey = `businessDashboard.dashboard.subscriptionManagement.${key}`;
    const result = getTranslation(fullKey, currentLocale);
    return Array.isArray(result) ? result[0] || key : result as string;
  }, [currentLocale]);

  // Modal controls
  const { isOpen: isRenewOpen, onOpen: onRenewOpen, onClose: onRenewClose } = useDisclosure();

  // Smart Contract Hooks
  const { data: registrationFee } = useRegistrationFee();

  // Handle successful renewal
  const handleRenewalSuccess = () => {
    onRenewSubscription?.();
  };

  return (
    <div className="space-y-6">
      {/* Subscription Status Card */}
      <SubscriptionStatusCard
        subscriptionData={subscriptionData}
        tString={tString}
        onRenewClick={onRenewOpen}
        onToggleAutoRenewal={onToggleAutoRenewal}
      />

      {/* Renewal Modal */}
      <RenewalModal
        isOpen={isRenewOpen}
        onClose={onRenewClose}
        onRenewalSuccess={handleRenewalSuccess}
        registrationFee={registrationFee}
        tString={tString}
      />
    </div>
  );
};

// Export types for external use
export type { SubscriptionData, SubscriptionManagementProps } from './types';
