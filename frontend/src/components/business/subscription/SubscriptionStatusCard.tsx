'use client';

import React from 'react';
import { Card, CardBody, CardHeader, Button, Chip } from '@nextui-org/react';
import { Calendar, CreditCard, AlertTriangle, CheckCircle, Clock, DollarSign, TrendingUp } from 'lucide-react';
import { SubscriptionData } from './types';
import { getStatusColor, getStatusIcon, getDaysUntilExpiry } from './utils';

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

  return (
    <>
      {/* Subscription Status Overview */}
      <Card className="shadow-sm">
        <CardHeader className="pb-3">
          <div className="flex items-center justify-between w-full">
            <div className="flex items-center gap-3">
              <div className="w-10 h-10 bg-gradient-to-br from-blue-500 to-purple-600 rounded-lg flex items-center justify-center">
                <CreditCard className="text-white" size={20} />
              </div>
              <div>
                <h3 className="text-lg font-semibold text-gray-900">{tString('title')}</h3>
                <p className="text-sm text-gray-600">{tString('subtitle')}</p>
              </div>
            </div>
            <Chip
              color={getStatusColor(subscriptionData.status)}
              variant="flat"
              startContent={getStatusIcon(subscriptionData.status)}
              className="capitalize"
            >
              {tString(`status.${subscriptionData.status}`)}
            </Chip>
          </div>
        </CardHeader>
        <CardBody className="pt-0">
          <div className="grid grid-cols-1 md:grid-cols-3 gap-6">
            {/* Current Plan */}
            <div className="space-y-2">
              <div className="flex items-center gap-2 text-sm text-gray-600">
                <TrendingUp size={16} />
                {tString('subscriptionModel')}
              </div>
              <div className="text-xl font-semibold text-gray-900 capitalize">
                {tString('payAsYouGo')}
              </div>
              <div className="text-sm text-gray-600">
                {tString('lastPayment')}: ${(parseFloat(subscriptionData.lastPaymentAmount) / 1000000).toFixed(2)} USDC
              </div>
            </div>

            {/* Subscription End */}
            <div className="space-y-2">
              <div className="flex items-center gap-2 text-sm text-gray-600">
                <Calendar size={16} />
                {tString('subscriptionEnds')}
              </div>
              <div className="text-xl font-semibold text-gray-900">
                {subscriptionData.subscriptionEndDate ? new Date(subscriptionData.subscriptionEndDate).toLocaleDateString() : tString('notSet')}
              </div>
              <div className="text-sm text-gray-600">
                {daysUntilExpiry > 0 ? `${daysUntilExpiry} ${tString('daysRemaining')}` : tString('expired')}
              </div>
            </div>

            {/* Time Remaining */}
            <div className="space-y-2">
              <div className="flex items-center gap-2 text-sm text-gray-600">
                <Clock size={16} />
                {tString('timeRemaining')}
              </div>
              <div className="text-xl font-semibold text-gray-900">
                {Math.floor(subscriptionData.timeRemaining / (24 * 60 * 60))} {tString('days')}
              </div>
              <div className="text-sm text-gray-600">
                {tString('totalPaid')}: ${(parseFloat(subscriptionData.totalPaid) / 1000000).toFixed(2)} USDC
              </div>
            </div>
          </div>

          {/* Expiry Warning */}
          {isExpiringSoon && (
            <div className="mt-6 p-4 bg-amber-50 border border-amber-200 rounded-lg">
              <div className="flex items-center gap-3">
                <AlertTriangle className="text-amber-600" size={20} />
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
          <div className="flex gap-3 mt-6">
            <Button
              color="primary"
              variant="solid"
              onPress={onRenewClick}
              startContent={<DollarSign size={16} />}
              className="flex-1"
            >
              {tString('renewSubscription')}
            </Button>
          </div>
        </CardBody>
      </Card>

      {/* Payment History */}
      <Card className="shadow-sm">
        <CardHeader>
          <div className="flex items-center gap-3">
            <div className="w-8 h-8 bg-gray-100 rounded-lg flex items-center justify-center">
              <Calendar className="text-gray-600" size={16} />
            </div>
            <div>
              <h3 className="text-lg font-semibold text-gray-900">{tString('paymentHistory.title')}</h3>
              <p className="text-sm text-gray-600">{tString('paymentHistory.subtitle')}</p>
            </div>
          </div>
        </CardHeader>
        <CardBody>
          <div className="space-y-4">
            <div className="flex items-center justify-between py-3 border-b border-gray-100">
              <div className="flex items-center gap-3">
                <div className="w-8 h-8 bg-green-100 rounded-full flex items-center justify-center">
                  <CheckCircle className="text-green-600" size={16} />
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
        </CardBody>
      </Card>
    </>
  );
};
