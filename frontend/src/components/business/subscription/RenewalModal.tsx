'use client';

import React, { useState, useEffect } from 'react';
import { Modal, ModalContent, ModalHeader, ModalBody, ModalFooter, Button, Card, CardBody, Chip, Input, Checkbox, Divider } from '@nextui-org/react';
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

  return (
    <>
      {/* Renew Subscription Modal */}
      <Modal isOpen={isOpen} onClose={onClose} size="2xl">
        <ModalContent>
          <ModalHeader>{tString('renewalModal.title')}</ModalHeader>
          <ModalBody>
            <div className="space-y-6">
              <p className="text-gray-600">
                {tString('renewalModal.description')}
              </p>

              {/* Wallet Status */}
              {!address && (
                <Card className="bg-amber-50 border border-amber-200">
                  <CardBody className="p-4">
                    <div className="flex items-center gap-3">
                      <AlertTriangle className="text-amber-600" size={20} />
                      <div>
                        <p className="text-sm font-medium text-amber-800">
                          {tString('renewalModal.walletNotConnected.title')}
                        </p>
                        <p className="text-xs text-amber-700">
                          {tString('renewalModal.walletNotConnected.subtitle')}
                        </p>
                      </div>
                    </div>
                  </CardBody>
                </Card>
              )}

              {address && usdcBalance !== undefined && (
                <Card className="bg-blue-50 border border-blue-200">
                  <CardBody className="p-4">
                    <div className="flex items-center justify-between">
                      <div className="flex items-center gap-3">
                        <CheckCircle className="text-blue-600" size={20} />
                        <div>
                          <p className="text-sm font-medium text-blue-800">
                            {tString('renewalModal.walletConnected.title')}
                          </p>
                          <p className="text-xs text-blue-700">
                            {tString('renewalModal.walletConnected.balance')}: ${formatUsdcAmount(usdcBalance)}
                          </p>
                        </div>
                      </div>
                    </div>
                  </CardBody>
                </Card>
              )}

              {/* Payment Options */}
              <div className="space-y-4">
                <h4 className="font-semibold text-gray-900">{tString('renewalModal.suggestedOptions')}</h4>
                <div className="grid grid-cols-2 gap-3">
                  {renewalOptions.map((option) => (
                    <Card
                      key={option.months}
                      isPressable
                      isHoverable
                      onPress={() => {
                        setRenewalState(prev => ({
                          ...prev,
                          selectedOption: option,
                          useCustomAmount: false,
                          customAmount: ''
                        }));
                      }}
                      className={`relative transition-all duration-200 ${
                        !renewalState.useCustomAmount && renewalState.selectedOption.months === option.months 
                          ? 'ring-2 ring-blue-500 shadow-lg' 
                          : 'hover:shadow-md'
                      } ${option.popular ? 'border-2 border-blue-500' : ''}`}
                    >
                      {option.popular && (
                        <div className="absolute -top-2 left-1/2 transform -translate-x-1/2">
                          <span className="bg-gradient-to-r from-blue-500 to-purple-600 text-white px-2 py-1 rounded-full text-xs font-medium">
                            {tString('renewalModal.popular')}
                          </span>
                        </div>
                      )}
                      <CardBody className="p-3 text-center">
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
                      </CardBody>
                    </Card>
                  ))}
                </div>
                
                {/* Custom Amount Option */}
                <Card 
                  className={`transition-all duration-200 ${
                    renewalState.useCustomAmount ? 'ring-2 ring-blue-500 shadow-lg' : 'hover:shadow-md'
                  }`}
                >
                  <CardBody className="p-4">
                    <div className="flex items-center gap-4">
                      <Checkbox
                        isSelected={renewalState.useCustomAmount}
                        onValueChange={(checked) => {
                          setRenewalState(prev => ({
                            ...prev,
                            useCustomAmount: checked,
                            customAmount: checked ? prev.customAmount : ''
                          }));
                        }}
                        color="primary"
                      >
                        <span className="font-medium">{tString('renewalModal.customAmount.title')}</span>
                      </Checkbox>
                      {renewalState.useCustomAmount && (
                        <Input
                          placeholder={tString('renewalModal.customAmount.placeholder')}
                          value={renewalState.customAmount}
                          onChange={(e) => {
                            const value = e.target.value;
                            setRenewalState(prev => ({
                              ...prev,
                              customAmount: value
                            }));
                          }}
                          variant="bordered"
                          size="sm"
                          startContent={<DollarSign size={16} className="text-gray-400" />}
                          classNames={{
                            inputWrapper: "h-10 border-2",
                          }}
                          className="w-32"
                        />
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
                  </CardBody>
                </Card>

                {/* Coupon Code Option */}
                <Card 
                  className={`transition-all duration-200 ${
                    renewalState.useCoupon ? 'ring-2 ring-green-500 shadow-lg' : 'hover:shadow-md'
                  }`}
                >
                  <CardBody className="p-4">
                    <div className="flex items-center gap-4">
                      <Checkbox
                        isSelected={renewalState.useCoupon}
                        onValueChange={(checked) => {
                          setRenewalState(prev => ({
                            ...prev,
                            useCoupon: checked,
                            couponCode: checked ? prev.couponCode : ''
                          }));
                        }}
                        color="success"
                      >
                        <span className="font-medium">{tString('renewalModal.coupon.title')}</span>
                      </Checkbox>
                      {renewalState.useCoupon && (
                        <Input
                          placeholder={tString('renewalModal.coupon.placeholder')}
                          value={renewalState.couponCode}
                          onChange={(e) => {
                            setRenewalState(prev => ({
                              ...prev,
                              couponCode: e.target.value.toUpperCase()
                            }));
                          }}
                          variant="bordered"
                          size="sm"
                          startContent={<Hash size={16} className="text-gray-400" />}
                          classNames={{
                            inputWrapper: "h-10 border-2",
                          }}
                          className="w-40"
                          color={
                            renewalState.couponCode ? (
                              isCouponLoading ? "default" :
                              isCouponValid ? "success" : "danger"
                            ) : "default"
                          }
                        />
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
                  </CardBody>
                </Card>
              </div>

              {/* Payment Summary */}
              <Card className="bg-gradient-to-br from-gray-50 to-blue-50/30">
                <CardBody className="p-4">
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
                    <Divider />
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
                </CardBody>
              </Card>
            </div>
          </ModalBody>
          <ModalFooter>
            <Button 
              variant="light" 
              onPress={onClose}
              isDisabled={renewalState.isProcessing}
            >
              {tString('renewalModal.buttons.cancel')}
            </Button>
            <Button 
              color="primary" 
              onPress={handleRenewalPayment}
              isDisabled={
                renewalState.isProcessing || 
                (renewalState.useCustomAmount ? !renewalState.customAmount || parseFloat(renewalState.customAmount) <= 0 : false) ||
                !address
              }
              isLoading={renewalState.isProcessing}
            >
              {renewalState.isProcessing ? (
                renewalState.processingStep === 'approval' ? tString('renewalModal.buttons.approvingUsdc') :
                renewalState.processingStep === 'renewal' ? tString('renewalModal.buttons.processingRenewal') :
                tString('renewalModal.buttons.complete')
              ) : (
                tString('renewalModal.buttons.confirmPayment')
              )}
            </Button>
          </ModalFooter>
        </ModalContent>
      </Modal>
    </>
  );
};
