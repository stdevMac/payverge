'use client';

import React, { useState, useEffect, useCallback } from 'react';
import { useSimpleLocale, getTranslation } from '@/i18n/SimpleTranslationProvider';
import { useDisclosure } from '@nextui-org/react';
import { useRegistrationFee } from '@/contracts/hooks';
import { SubscriptionStatusCard } from './subscription/SubscriptionStatusCard';
import { RenewalModal } from './subscription/RenewalModal';

import { SubscriptionData, SubscriptionManagementProps } from './subscription/types';

export const SubscriptionManagement: React.FC<SubscriptionManagementProps> = ({
  subscriptionData,
  onRenewSubscription,
  onToggleAutoRenewal,
  businessId,
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
    <div className="p-4 max-w-6xl mx-auto">
      {/* Compact Header */}
      <div className="flex justify-between items-center mb-6">
        <div>
          <h1 className="text-2xl font-light text-gray-900 tracking-wide">{tString('title')}</h1>
          <p className="text-gray-600 font-light text-sm mt-1">{tString('subtitle')}</p>
        </div>
      </div>

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
          businessId={businessId}
        />
      </div>
    </div>
  );
};

export default SubscriptionManagement;
