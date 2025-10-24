import React from 'react';
import { RenewalOption } from './types';
import { formatUsdcAmount } from '@/contracts/hooks';
import { CheckCircle, AlertTriangle, Clock } from 'lucide-react';

// Dynamic renewal options based on registration fee
export const getRenewalOptions = (registrationFee?: unknown, tString?: (key: string) => string): RenewalOption[] => {
  const yearlyFee = registrationFee && typeof registrationFee === 'bigint' 
    ? Number(formatUsdcAmount(registrationFee)) 
    : 100; // More reasonable default closer to actual registration fee
  
  return [
    {
      months: 1,
      suggestedAmount: (yearlyFee / 12).toFixed(2),
      description: tString ? tString('renewalModal.renewalOptions.extend1Month') : 'Extend for 1 month',
    },
    {
      months: 3,
      suggestedAmount: (yearlyFee / 4).toFixed(2),
      description: tString ? tString('renewalModal.renewalOptions.extend3Months') : 'Extend for 3 months',
    },
    {
      months: 6,
      suggestedAmount: (yearlyFee / 2).toFixed(2),
      description: tString ? tString('renewalModal.renewalOptions.extend6Months') : 'Extend for 6 months',
      popular: true,
    },
    {
      months: 12,
      suggestedAmount: yearlyFee.toFixed(2),
      description: tString ? tString('renewalModal.renewalOptions.extendFullYear') : 'Extend for full year',
    },
  ];
};

// Helper function to format subscription time
export const formatSubscriptionTime = (seconds: number, tString: (key: string) => string) => {
  const days = Math.floor(seconds / (24 * 60 * 60));
  const months = Math.floor(days / 30);
  const years = Math.floor(days / 365);
  
  if (years >= 1) {
    const unit = years > 1 ? tString('timeUnits.years') : tString('timeUnits.year');
    return `${years} ${unit}`;
  } else if (months >= 1) {
    const unit = months > 1 ? tString('timeUnits.months') : tString('timeUnits.month');
    return `${months} ${unit}`;
  } else {
    const unit = days > 1 ? tString('timeUnits.days') : tString('timeUnits.day');
    return `${days} ${unit}`;
  }
};

export const getStatusColor = (status: string) => {
  switch (status) {
    case 'active': return 'success';
    case 'expired': return 'danger';
    case 'suspended': return 'warning';
    case 'cancelled': return 'default';
    default: return 'default';
  }
};

export const getStatusIcon = (status: string) => {
  switch (status) {
    case 'active': return React.createElement(CheckCircle, { size: 16 });
    case 'expired': return React.createElement(AlertTriangle, { size: 16 });
    case 'suspended': return React.createElement(Clock, { size: 16 });
    case 'cancelled': return React.createElement(AlertTriangle, { size: 16 });
    default: return React.createElement(Clock, { size: 16 });
  }
};

export const getDaysUntilExpiry = (timeRemaining: number) => {
  return Math.ceil(timeRemaining / (24 * 60 * 60)); // Convert seconds to days
};

export const getMonthsFromSeconds = (seconds: number) => {
  return Math.floor(seconds / (30 * 24 * 60 * 60)); // Approximate months
};
