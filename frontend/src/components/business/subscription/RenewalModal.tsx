'use client';

import React, { useState, useEffect } from 'react';
// Removed NextUI imports - using clean HTML/CSS design
import { Calendar, CreditCard, AlertTriangle, CheckCircle, Clock, DollarSign, TrendingUp, Settings, Hash } from 'lucide-react';
import { useAccount } from 'wagmi';
import { 
  useUsdcBalance,
  useUsdcAllowance,
  useApproveUsdc,
  useRenewSubscription,
  useRenewSubscriptionWithCoupon,
  useValidateCoupon,
  useCalculateSubscriptionTime,
  parseUsdcAmount,
  formatUsdcAmount
} from '@/contracts/hooks';
import { RenewalOption, RenewalState } from './types';
import { getRenewalOptions, formatSubscriptionTime } from './utils';

interface RenewalModalProps {
  isOpen: boolean;
  onClose: () => void;
  onRenewalSuccess?: () => void;
  registrationFee?: unknown;
  tString: (key: string) => string;
}

export const RenewalModal: React.FC<RenewalModalProps> = ({
  isOpen,
  onClose,
  onRenewalSuccess,
  registrationFee,
  tString,
}) => {
  // Wallet and Smart Contract Hooks
  const { address } = useAccount();
  const { data: usdcBalance } = useUsdcBalance();
  const { data: usdcAllowance } = useUsdcAllowance();
  const { approveUsdc } = useApproveUsdc();
  const { renewSubscription } = useRenewSubscription();
  const { renewSubscriptionWithCoupon } = useRenewSubscriptionWithCoupon();

  // Dynamic renewal options based on registration fee
  const renewalOptions = getRenewalOptions(registrationFee, tString);

  // Renewal state
  const [renewalState, setRenewalState] = useState<RenewalState>({
    selectedOption: renewalOptions[2], // Default to 6 months
    customAmount: '',
    useCustomAmount: false,
    couponCode: '',
    useCoupon: false,
    isProcessing: false,
    processingStep: 'approval',
  });

  // Update selected option when registration fee loads
  useEffect(() => {
    if (registrationFee) {
      const updatedOptions = getRenewalOptions(registrationFee, tString);
      setRenewalState(prev => ({
        ...prev,
        selectedOption: updatedOptions[2], // Keep 6 months selected but with correct amount
      }));
    }
  }, [registrationFee, tString]);

  // Validate coupon with smart contract
  const { 
    isValid: isCouponValid, 
    discountAmount: couponDiscount, 
    isLoading: isCouponLoading 
  } = useValidateCoupon(renewalState.useCoupon && renewalState.couponCode ? renewalState.couponCode : '');

  // Calculate subscription time based on payment amount
  const paymentAmountWei = renewalState.useCustomAmount && renewalState.customAmount 
    ? parseUsdcAmount(renewalState.customAmount) 
    : parseUsdcAmount(renewalState.selectedOption.suggestedAmount);
  const { data: calculatedSubscriptionTime } = useCalculateSubscriptionTime(paymentAmountWei);

  // Calculate final payment amount (with coupon discount if applicable)
  const finalPaymentAmount = renewalState.useCoupon && isCouponValid && couponDiscount
    ? paymentAmountWei - couponDiscount
    : paymentAmountWei;

  // Check if user has sufficient balance
  const hasInsufficientBalance = usdcBalance && finalPaymentAmount > usdcBalance;

  // Handle renewal processing
  const handleRenewalPayment = async () => {
    if (!address) {
      alert(tString('alerts.connectWallet'));
      return;
    }

    try {
      setRenewalState(prev => ({ ...prev, isProcessing: true, processingStep: 'approval' }));

      // Check if approval is needed
      const currentAllowance = usdcAllowance || BigInt(0);
      if (currentAllowance < finalPaymentAmount) {
        // Approve USDC spending
        await approveUsdc(finalPaymentAmount);
      }

      setRenewalState(prev => ({ ...prev, processingStep: 'renewal' }));

      // Process renewal payment (with or without coupon)
      let txHash;
      if (renewalState.useCoupon && renewalState.couponCode && isCouponValid) {
        txHash = await renewSubscriptionWithCoupon(paymentAmountWei, renewalState.couponCode);
      } else {
        txHash = await renewSubscription(finalPaymentAmount);
      }

      setRenewalState(prev => ({ ...prev, processingStep: 'complete' }));

      // Call the parent callback if provided
      onRenewalSuccess?.();

      // Close modal after successful renewal
      setTimeout(() => {
        setRenewalState(prev => ({ ...prev, isProcessing: false }));
        onClose();
      }, 2000);

    } catch (error) {
      console.error('Renewal failed:', error);
      alert(tString('alerts.renewalFailed'));
      setRenewalState(prev => ({ ...prev, isProcessing: false }));
    }
  };

  const updateRenewalState = (updates: Partial<RenewalState>) => {
    setRenewalState(prev => ({ ...prev, ...updates }));
  };

  if (!isOpen) return null;

  return (
    <>
      {/* Renew Subscription Modal */}
      <div className="fixed inset-0 bg-black/50 backdrop-blur-sm z-50 flex items-center justify-center p-4">
        <div className="w-full max-w-2xl bg-white border border-gray-200 rounded-lg shadow-sm max-h-[90vh] overflow-y-auto">
          <div className="border-b border-gray-200 p-4">
            <div className="flex items-center justify-between">
              <div>
                <h2 className="text-xl font-light text-gray-900 tracking-wide">{tString('renewalModal.title')}</h2>
                <p className="text-sm text-gray-600 mt-1">{tString('renewalModal.description')}</p>
              </div>
              <button
                onClick={onClose}
                className="text-gray-400 hover:text-gray-600 transition-colors p-1"
                disabled={renewalState.isProcessing}
              >
                <svg className="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                  <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M6 18L18 6M6 6l12 12" />
                </svg>
              </button>
            </div>
          </div>
          <div className="p-4">
            <div className="space-y-4">
              {/* Wallet Status */}
              {!address && (
                <div className="bg-amber-50 border border-amber-200 rounded-lg p-6">
                  <div className="flex items-start gap-3">
                    <AlertTriangle className="w-6 h-6 text-amber-600 flex-shrink-0 mt-0.5" />
                    <div>
                      <h3 className="font-medium text-amber-900 mb-2">
                        {tString('renewalModal.walletNotConnected.title')}
                      </h3>
                      <div className="text-sm text-amber-800">
                        <p>{tString('renewalModal.walletNotConnected.subtitle')}</p>
                      </div>
                    </div>
                  </div>
                </div>
              )}

              {address && usdcBalance !== undefined && (
                <div className="bg-blue-50 border border-blue-200 rounded-lg p-6">
                  <div className="flex items-start gap-3">
                    <CheckCircle className="w-6 h-6 text-blue-600 flex-shrink-0 mt-0.5" />
                    <div>
                      <h3 className="font-medium text-blue-900 mb-2">
                        {tString('renewalModal.walletConnected.title')}
                      </h3>
                      <div className="text-sm text-blue-800">
                        <p>{tString('renewalModal.walletConnected.balance')}: ${formatUsdcAmount(usdcBalance)}</p>
                      </div>
                    </div>
                  </div>
                </div>
              )}

              {/* Payment Options */}
              <div className="space-y-4">
                <h4 className="font-semibold text-gray-900">{tString('renewalModal.suggestedOptions')}</h4>
                <div className="grid grid-cols-2 gap-3">
                  {renewalOptions.map((option) => (
                    <div
                      key={option.months}
                      className={`relative cursor-pointer transition-all duration-200 bg-white border rounded-lg p-3 text-center ${
                        !renewalState.useCustomAmount && renewalState.selectedOption.months === option.months 
                          ? 'ring-2 ring-blue-500 shadow-lg border-blue-500' 
                          : 'border-gray-200 hover:shadow-md hover:border-gray-300'
                      } ${option.popular ? 'border-2 border-blue-500' : ''}`}
                      onClick={() => {
                        setRenewalState(prev => ({
                          ...prev,
                          selectedOption: option,
                          useCustomAmount: false,
                          customAmount: ''
                        }));
                      }}
                    >
                      {option.popular && (
                        <div className="absolute -top-2 left-1/2 transform -translate-x-1/2">
                          <span className="bg-blue-600 text-white px-2 py-1 rounded-full text-xs font-medium">
                            {tString('renewalModal.popular')}
                          </span>
                        </div>
                      )}
                      <div className="text-lg font-bold text-gray-900 mb-1">
                        ${option.suggestedAmount}
                      </div>
                      <div className="text-xs text-gray-600 mb-1">
                        {option.description}
                      </div>
                      <div className="text-xs text-blue-600">
                        {!renewalState.useCustomAmount && renewalState.selectedOption.months === option.months && calculatedSubscriptionTime 
                          ? formatSubscriptionTime(Number(calculatedSubscriptionTime.toString()), tString)
                          : `~${option.months} month${option.months > 1 ? 's' : ''}`
                        }
                      </div>
                    </div>
                  ))}
                </div>
                
                {/* Custom Amount Option */}
                <div className="bg-gray-50 border border-gray-200 rounded-lg p-4">
                  <div className="space-y-3">
                    <label className="flex items-center gap-3 cursor-pointer">
                      <input
                        type="checkbox"
                        checked={renewalState.useCustomAmount}
                        onChange={(e) => {
                          setRenewalState(prev => ({
                            ...prev,
                            useCustomAmount: e.target.checked,
                            customAmount: e.target.checked ? prev.customAmount : ''
                          }));
                        }}
                        className="w-4 h-4 text-blue-600 border-gray-300 rounded focus:ring-blue-500 focus:ring-offset-0"
                      />
                      <div>
                        <span className="text-sm font-medium text-gray-900">{tString('renewalModal.customAmount.title')}</span>
                        <p className="text-xs text-gray-600 mt-0.5">Enter a custom payment amount</p>
                      </div>
                    </label>
                    {renewalState.useCustomAmount && (
                      <div className="ml-7">
                        <div className="relative">
                          <DollarSign className="absolute left-3 top-1/2 transform -translate-y-1/2 w-4 h-4 text-gray-400" />
                          <input
                            type="text"
                            placeholder={tString('renewalModal.customAmount.placeholder')}
                            value={renewalState.customAmount}
                            onChange={(e) => {
                              const value = e.target.value;
                              setRenewalState(prev => ({
                                ...prev,
                                customAmount: value
                              }));
                            }}
                            className="w-full h-10 pl-8 pr-3 py-2 bg-white border border-gray-300 rounded-lg text-sm placeholder-gray-500 focus:outline-none focus:ring-2 focus:ring-blue-500 focus:border-blue-500"
                          />
                        </div>
                      </div>
                    )}
                  </div>
                    {renewalState.useCustomAmount && (
                      <div className="mt-2 space-y-1">
                        {(() => {
                          const yearlyFee = registrationFee && typeof registrationFee === 'bigint' 
                            ? Number(formatUsdcAmount(registrationFee)) 
                            : 120;
                          const customAmountNum = parseFloat(renewalState.customAmount || '0');
                          const isOverYearlyFee = customAmountNum > yearlyFee;
                          
                          return (
                            <>
                              <p className="text-sm text-gray-600">
                                {tString('renewalModal.customAmount.description').replace('${fee}', yearlyFee.toString())}
                              </p>
                              {isOverYearlyFee && (
                                <p className="text-sm font-medium text-amber-600 bg-amber-50 px-2 py-1 rounded">
                                  ⚠️ {tString('renewalModal.customAmount.warning').replace('${fee}', yearlyFee.toString())}
                                </p>
                              )}
                              {renewalState.customAmount && calculatedSubscriptionTime ? (
                                <p className="text-sm font-medium text-blue-600">
                                  {tString('renewalModal.customAmount.estimatedTime')}: {formatSubscriptionTime(Number(calculatedSubscriptionTime.toString()), tString)}
                                </p>
                              ) : null}
                            </>
                          );
                        })()}
                      </div>
                    )}
                </div>

                {/* Coupon Code Option */}
                <div className="bg-gray-50 border border-gray-200 rounded-lg p-4">
                  <div className="space-y-3">
                    <label className="flex items-center gap-3 cursor-pointer">
                      <input
                        type="checkbox"
                        checked={renewalState.useCoupon}
                        onChange={(e) => {
                          setRenewalState(prev => ({
                            ...prev,
                            useCoupon: e.target.checked,
                            couponCode: e.target.checked ? prev.couponCode : ''
                          }));
                        }}
                        className="w-4 h-4 text-green-600 border-gray-300 rounded focus:ring-green-500 focus:ring-offset-0"
                      />
                      <div>
                        <span className="text-sm font-medium text-gray-900">{tString('renewalModal.coupon.title')}</span>
                        <p className="text-xs text-gray-600 mt-0.5">Apply a discount coupon code</p>
                      </div>
                    </label>
                    {renewalState.useCoupon && (
                      <div className="ml-7">
                        <div className="relative">
                          <Hash className="absolute left-3 top-1/2 transform -translate-y-1/2 w-4 h-4 text-gray-400" />
                          <input
                            type="text"
                            placeholder={tString('renewalModal.coupon.placeholder')}
                            value={renewalState.couponCode}
                            onChange={(e) => {
                              setRenewalState(prev => ({
                                ...prev,
                                couponCode: e.target.value.toUpperCase()
                              }));
                            }}
                            className={`w-full h-10 pl-8 pr-3 py-2 bg-white border rounded-lg text-sm placeholder-gray-500 focus:outline-none focus:ring-2 ${
                              renewalState.couponCode ? (
                                isCouponLoading ? 'border-gray-300 focus:ring-gray-500 focus:border-gray-500' :
                                isCouponValid ? 'border-green-300 focus:ring-green-500 focus:border-green-500' : 
                                'border-red-300 focus:ring-red-500 focus:border-red-500'
                              ) : 'border-gray-300 focus:ring-blue-500 focus:border-blue-500'
                            }`}
                          />
                        </div>
                      </div>
                    )}
                  </div>
                  {renewalState.useCoupon && renewalState.couponCode && !isCouponLoading && (
                    <div className="mt-2">
                      {isCouponValid ? (
                        <p className="text-sm font-medium text-green-600 bg-green-50 px-2 py-1 rounded">
                          ✅ {tString('renewalModal.coupon.validCoupon')}: ${formatUsdcAmount(couponDiscount)}
                        </p>
                      ) : (
                        <p className="text-sm font-medium text-red-600 bg-red-50 px-2 py-1 rounded">
                          ❌ {tString('renewalModal.coupon.invalidCoupon')}
                        </p>
                      )}
                    </div>
                  )}
                </div>
              </div>

              {/* Payment Summary */}
              <div className="bg-gray-50 border border-gray-200 rounded-lg p-4">
                <h4 className="font-semibold text-gray-900 mb-3">{tString('renewalModal.paymentSummary.title')}</h4>
                <div className="space-y-2">
                  <div className="flex justify-between">
                    <span className="text-gray-600">{tString('renewalModal.paymentSummary.paymentAmount')}</span>
                    <span className="font-medium">
                      ${renewalState.useCustomAmount ? renewalState.customAmount || '0.00' : renewalState.selectedOption.suggestedAmount}
                    </span>
                  </div>
                  {renewalState.useCoupon && isCouponValid && couponDiscount && (
                    <div className="flex justify-between">
                      <span className="text-gray-600">{tString('renewalModal.paymentSummary.couponDiscount')}</span>
                      <span className="font-medium text-green-600">
                        -${formatUsdcAmount(couponDiscount)}
                      </span>
                    </div>
                  )}
                  <div className="border-t border-gray-200 pt-2"></div>
                  <div className="flex justify-between text-lg font-semibold">
                    <span>{tString('renewalModal.paymentSummary.total')}</span>
                    <span>
                      ${(() => {
                        const originalAmount = parseFloat(renewalState.useCustomAmount ? renewalState.customAmount || '0' : renewalState.selectedOption.suggestedAmount);
                        const discount = renewalState.useCoupon && isCouponValid && couponDiscount ? Number(formatUsdcAmount(couponDiscount)) : 0;
                        const finalAmount = Math.max(0, originalAmount - discount);
                        return finalAmount.toFixed(2);
                      })()}
                    </span>
                  </div>
                  {calculatedSubscriptionTime ? (
                    <div className="text-sm text-gray-600 mt-2">
                      {tString('renewalModal.paymentSummary.additionalTime')}: {formatSubscriptionTime(Number(calculatedSubscriptionTime.toString()), tString)}
                    </div>
                  ) : null}
                </div>
              </div>
            </div>
          </div>
          <div className="border-t border-gray-200 p-4 flex justify-end gap-2">
            <button
              className="px-4 py-2 text-sm font-medium text-gray-700 bg-white border border-gray-300 rounded-lg hover:bg-gray-50 focus:outline-none focus:ring-2 focus:ring-gray-500 focus:ring-offset-2 disabled:opacity-50 disabled:cursor-not-allowed transition-colors"
              onClick={onClose}
              disabled={renewalState.isProcessing}
            >
              {tString('renewalModal.buttons.cancel')}
            </button>
            <button
              className={`px-4 py-2 text-sm font-medium text-white rounded-lg transition-colors flex items-center gap-2 focus:outline-none focus:ring-2 focus:ring-offset-2 disabled:opacity-50 disabled:cursor-not-allowed ${
                renewalState.isProcessing 
                  ? 'bg-gray-600 focus:ring-gray-500' 
                  : 'bg-gray-900 hover:bg-gray-800 focus:ring-gray-500'
              }`}
              onClick={handleRenewalPayment}
              disabled={
                renewalState.isProcessing || 
                (renewalState.useCustomAmount ? !renewalState.customAmount || parseFloat(renewalState.customAmount) <= 0 : false) ||
                !address
              }
            >
              {renewalState.isProcessing && (
                <div className="w-4 h-4 border-2 border-white border-t-transparent rounded-full animate-spin"></div>
              )}
              {renewalState.isProcessing ? (
                renewalState.processingStep === 'approval' ? tString('renewalModal.buttons.approvingUsdc') :
                renewalState.processingStep === 'renewal' ? tString('renewalModal.buttons.processingRenewal') :
                tString('renewalModal.buttons.complete')
              ) : (
                tString('renewalModal.buttons.confirmPayment')
              )}
            </button>
          </div>
        </div>
      </div>
    </>
  );
};
