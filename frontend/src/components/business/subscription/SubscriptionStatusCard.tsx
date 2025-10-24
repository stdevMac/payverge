'use client';

import React from 'react';
import { Calendar, CreditCard, AlertTriangle, CheckCircle, Clock, DollarSign, TrendingUp } from 'lucide-react';
import { SubscriptionData } from './types';
import { getDaysUntilExpiry } from './utils';

interface SubscriptionStatusCardProps {
  subscriptionData: SubscriptionData;
  tString: (key: string) => string;
  onRenewClick: () => void;
  onToggleAutoRenewal?: () => void;
}

export const SubscriptionStatusCard: React.FC<SubscriptionStatusCardProps> = ({
  subscriptionData,
  tString,
  onRenewClick,
  onToggleAutoRenewal,
}) => {
  const daysUntilExpiry = getDaysUntilExpiry(subscriptionData.timeRemaining);
  const isExpiringSoon = daysUntilExpiry <= 7 && daysUntilExpiry > 0;

  const getStatusIcon = (status: string) => {
    switch (status) {
      case 'active': return <CheckCircle className="w-4 h-4" />;
      case 'expired': return <AlertTriangle className="w-4 h-4" />;
      case 'suspended': return <Clock className="w-4 h-4" />;
      case 'cancelled': return <AlertTriangle className="w-4 h-4" />;
      default: return <Clock className="w-4 h-4" />;
    }
  };

  return (
    <>
      {/* Subscription Status Overview */}
      <div className="bg-white border border-gray-200 rounded-lg shadow-sm">
        <div className="border-b border-gray-200 p-6">
          <div className="flex items-center justify-between w-full">
            <div className="flex items-center gap-3">
              <div className="w-12 h-12 bg-gray-50 rounded-xl flex items-center justify-center border border-gray-100">
                <CreditCard className="w-6 h-6 text-gray-700" />
              </div>
              <div>
                <h2 className="text-xl font-light text-gray-900 tracking-wide">{tString('title')}</h2>
                <p className="text-gray-600 font-light text-sm">{tString('subtitle')}</p>
              </div>
            </div>
            <span className={`inline-flex items-center px-2 py-1 rounded-full text-xs font-medium ${
              subscriptionData.status === 'active' ? 'bg-green-100 text-green-700' :
              subscriptionData.status === 'expired' ? 'bg-red-100 text-red-700' :
              subscriptionData.status === 'suspended' ? 'bg-yellow-100 text-yellow-700' :
              'bg-gray-100 text-gray-700'
            }`}>
              {getStatusIcon(subscriptionData.status)}
              <span className="ml-1 capitalize">{tString(`status.${subscriptionData.status}`)}</span>
            </span>
          </div>
        </div>
        <div className="p-6 space-y-6">
          <div className="grid grid-cols-1 md:grid-cols-3 gap-6">
            {/* Current Plan */}
            <div className="bg-gray-50 border border-gray-200 rounded-lg p-4">
              <div className="flex items-center gap-2 text-xs font-medium text-gray-600 uppercase tracking-wide mb-2">
                <TrendingUp className="w-4 h-4" />
                {tString('subscriptionModel')}
              </div>
              <div className="text-xl font-semibold text-gray-900 capitalize mb-1">
                {tString('payAsYouGo')}
              </div>
              <div className="text-sm text-gray-600">
                {tString('lastPayment')}: ${(parseFloat(subscriptionData.lastPaymentAmount) / 1000000).toFixed(2)} USDC
              </div>
            </div>

            {/* Subscription End */}
            <div className="bg-gray-50 border border-gray-200 rounded-lg p-4">
              <div className="flex items-center gap-2 text-xs font-medium text-gray-600 uppercase tracking-wide mb-2">
                <Calendar className="w-4 h-4" />
                {tString('subscriptionEnds')}
              </div>
              <div className="text-xl font-semibold text-gray-900 mb-1">
                {subscriptionData.subscriptionEndDate ? new Date(subscriptionData.subscriptionEndDate).toLocaleDateString() : tString('notSet')}
              </div>
              <div className="text-sm text-gray-600">
                {daysUntilExpiry > 0 ? `${daysUntilExpiry} ${tString('daysRemaining')}` : tString('expired')}
              </div>
            </div>

            {/* Time Remaining */}
            <div className="bg-gray-50 border border-gray-200 rounded-lg p-4">
              <div className="flex items-center gap-2 text-xs font-medium text-gray-600 uppercase tracking-wide mb-2">
                <Clock className="w-4 h-4" />
                {tString('timeRemaining')}
              </div>
              <div className="text-xl font-semibold text-gray-900 mb-1">
                {Math.floor(subscriptionData.timeRemaining / (24 * 60 * 60))} {tString('days')}
              </div>
              <div className="text-sm text-gray-600">
                {tString('totalPaid')}: ${(parseFloat(subscriptionData.totalPaid) / 1000000).toFixed(2)} USDC
              </div>
            </div>
          </div>

          {/* Expiry Warning */}
          {isExpiringSoon && (
            <div className="bg-amber-50 border border-amber-200 rounded-lg p-4">
              <div className="flex items-center gap-3">
                <AlertTriangle className="text-amber-600 w-5 h-5 flex-shrink-0" />
                <div>
                  <p className="text-sm font-medium text-amber-800">
                    {tString('expiryWarning.title').replace('{days}', daysUntilExpiry.toString()).replace('{plural}', daysUntilExpiry !== 1 ? 's' : '')}
                  </p>
                  <p className="text-xs text-amber-700 mt-1">
                    {tString('expiryWarning.subtitle')}
                  </p>
                </div>
              </div>
            </div>
          )}

          {/* Action Buttons */}
          <div className="flex gap-3">
            <button
              className="flex-1 px-4 py-3 bg-gray-900 text-white rounded-lg hover:bg-gray-800 font-medium transition-colors flex items-center justify-center gap-2"
              onClick={onRenewClick}
            >
              <DollarSign className="w-4 h-4" />
              {tString('renewSubscription')}
            </button>
          </div>
        </div>
      </div>

      {/* Payment History */}
      <div className="bg-white border border-gray-200 rounded-lg shadow-sm">
        <div className="border-b border-gray-200 p-6">
          <div className="flex items-center gap-3">
            <div className="w-12 h-12 bg-gray-50 rounded-xl flex items-center justify-center border border-gray-100">
              <Calendar className="w-6 h-6 text-gray-700" />
            </div>
            <div>
              <h3 className="text-xl font-light text-gray-900 tracking-wide">{tString('paymentHistory.title')}</h3>
              <p className="text-gray-600 font-light text-sm">{tString('paymentHistory.subtitle')}</p>
            </div>
          </div>
        </div>
        <div className="p-6">
          <div className="space-y-4">
            <div className="flex items-center justify-between py-3 border-b border-gray-100">
              <div className="flex items-center gap-3">
                <div className="w-8 h-8 bg-green-100 rounded-full flex items-center justify-center">
                  <CheckCircle className="text-green-600 w-4 h-4" />
                </div>
                <div>
                  <p className="font-medium text-gray-900">
                    {tString('paymentHistory.subscriptionPayment')}
                  </p>
                  <p className="text-sm text-gray-600">
                    {subscriptionData.lastPaymentDate ? new Date(subscriptionData.lastPaymentDate).toLocaleDateString() : tString('paymentHistory.noPaymentDate')}
                  </p>
                </div>
              </div>
              <div className="text-right">
                <p className="font-semibold text-gray-900">${(parseFloat(subscriptionData.lastPaymentAmount) / 1000000).toFixed(2)} USDC</p>
                <p className="text-sm text-green-600">{tString('paymentHistory.paid')}</p>
              </div>
            </div>
            
            <div className="text-center py-4">
              <p className="text-sm text-gray-500">{tString('paymentHistory.noAdditionalHistory')}</p>
            </div>
          </div>
        </div>
      </div>
    </>
  );
};
