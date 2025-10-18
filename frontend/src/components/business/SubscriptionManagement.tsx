'use client';

import React, { useState, useEffect } from 'react';
import { Card, CardBody, CardHeader, Button, Chip, Progress, Divider, Modal, ModalContent, ModalHeader, ModalBody, ModalFooter, useDisclosure, Input, Checkbox } from '@nextui-org/react';
import { Calendar, CreditCard, AlertTriangle, CheckCircle, Clock, DollarSign, TrendingUp, Settings, Hash } from 'lucide-react';
import { 
  useRegistrationFee, 
  useCalculateSubscriptionTime, 
  formatUsdcAmount, 
  parseUsdcAmount,
  useUsdcBalance,
  useUsdcAllowance,
  useApproveUsdc,
  useRenewSubscription,
  useRenewSubscriptionWithCoupon,
  useValidateCoupon
} from '@/contracts/hooks';
import { useAccount } from 'wagmi';

interface SubscriptionData {
  status: 'active' | 'expired' | 'suspended' | 'cancelled';
  lastPaymentDate: string;
  subscriptionEndDate: string;
  lastPaymentAmount: string;
  totalPaid: string;
  yearlyFee: string; // The full yearly fee amount
  timeRemaining: number; // seconds remaining
  remindersSent: number;
}

interface SubscriptionManagementProps {
  subscriptionData: SubscriptionData;
  onRenewSubscription?: () => void;
  onToggleAutoRenewal?: () => void;
}

// Subscription renewal options
interface RenewalOption {
  months: number;
  suggestedAmount: string;
  description: string;
  popular?: boolean;
}

// Dynamic renewal options based on registration fee
const getRenewalOptions = (registrationFee?: unknown): RenewalOption[] => {
  const yearlyFee = registrationFee && typeof registrationFee === 'bigint' 
    ? Number(formatUsdcAmount(registrationFee)) 
    : 100; // More reasonable default closer to actual registration fee
  
  return [
    {
      months: 1,
      suggestedAmount: (yearlyFee / 12).toFixed(2),
      description: 'Extend for 1 month',
    },
    {
      months: 3,
      suggestedAmount: (yearlyFee / 4).toFixed(2),
      description: 'Extend for 3 months',
    },
    {
      months: 6,
      suggestedAmount: (yearlyFee / 2).toFixed(2),
      description: 'Extend for 6 months',
      popular: true,
    },
    {
      months: 12,
      suggestedAmount: yearlyFee.toFixed(2),
      description: 'Extend for full year',
    },
  ];
};

export const SubscriptionManagement: React.FC<SubscriptionManagementProps> = ({
  subscriptionData,
  onRenewSubscription,
  onToggleAutoRenewal,
}) => {
  const { isOpen: isRenewOpen, onOpen: onRenewOpen, onClose: onRenewClose } = useDisclosure();

  // Wallet and Smart Contract Hooks
  const { address } = useAccount();
  const { data: registrationFee } = useRegistrationFee();
  const { data: usdcBalance } = useUsdcBalance();
  const { data: usdcAllowance } = useUsdcAllowance();
  const { approveUsdc } = useApproveUsdc();
  const { renewSubscription } = useRenewSubscription();
  const { renewSubscriptionWithCoupon } = useRenewSubscriptionWithCoupon();
  
  // Dynamic renewal options based on registration fee
  const renewalOptions = getRenewalOptions(registrationFee);
  
  // Renewal state
  const [selectedOption, setSelectedOption] = useState<RenewalOption>(renewalOptions[2]); // Default to 6 months
  
  // Update selected option when registration fee loads
  useEffect(() => {
    if (registrationFee) {
      const updatedOptions = getRenewalOptions(registrationFee);
      setSelectedOption(updatedOptions[2]); // Keep 6 months selected but with correct amount
    }
  }, [registrationFee]);
  
  const [customAmount, setCustomAmount] = useState('');
  const [useCustomAmount, setUseCustomAmount] = useState(false);
  const [isProcessing, setIsProcessing] = useState(false);
  const [processingStep, setProcessingStep] = useState<'approval' | 'renewal' | 'complete'>('approval');
  
  // Coupon state
  const [couponCode, setCouponCode] = useState('');
  const [useCoupon, setUseCoupon] = useState(false);
  
  // Validate coupon with smart contract
  const { 
    isValid: isCouponValid, 
    discountAmount: couponDiscount, 
    isLoading: isCouponLoading 
  } = useValidateCoupon(useCoupon && couponCode ? couponCode : '');
  
  // Calculate subscription time based on payment amount
  const paymentAmountWei = useCustomAmount && customAmount 
    ? parseUsdcAmount(customAmount) 
    : parseUsdcAmount(selectedOption.suggestedAmount);
  const { data: calculatedSubscriptionTime } = useCalculateSubscriptionTime(paymentAmountWei);

  // Helper function to format subscription time
  const formatSubscriptionTime = (seconds: number) => {
    const days = Math.floor(seconds / (24 * 60 * 60));
    const months = Math.floor(days / 30);
    const years = Math.floor(days / 365);
    
    if (years >= 1) {
      return `${years} year${years > 1 ? 's' : ''}`;
    } else if (months >= 1) {
      return `${months} month${months > 1 ? 's' : ''}`;
    } else {
      return `${days} day${days > 1 ? 's' : ''}`;
    }
  };

  // Handle renewal processing
  const handleRenewalPayment = async () => {
    if (!address) {
      alert('Please connect your wallet');
      return;
    }

    try {
      setIsProcessing(true);
      setProcessingStep('approval');

      const paymentAmount = useCustomAmount && customAmount 
        ? parseUsdcAmount(customAmount) 
        : parseUsdcAmount(selectedOption.suggestedAmount);

      // Check if approval is needed
      const currentAllowance = usdcAllowance || BigInt(0);
      if (currentAllowance < paymentAmount) {
        // Approve USDC spending
        await approveUsdc(paymentAmount);
      }

      setProcessingStep('renewal');

      // Process renewal payment (with or without coupon)
      let txHash;
      if (useCoupon && couponCode && isCouponValid) {
        txHash = await renewSubscriptionWithCoupon(paymentAmount, couponCode);
      } else {
        txHash = await renewSubscription(paymentAmount);
      }

      setProcessingStep('complete');

      // Call the parent callback if provided
      onRenewSubscription?.();

      // Close modal after successful renewal
      setTimeout(() => {
        setIsProcessing(false);
        onRenewClose();
      }, 2000);

    } catch (error) {
      console.error('Renewal failed:', error);
      alert('Renewal failed. Please try again.');
      setIsProcessing(false);
    }
  };

  const getStatusColor = (status: string) => {
    switch (status) {
      case 'active': return 'success';
      case 'expired': return 'danger';
      case 'suspended': return 'warning';
      case 'cancelled': return 'default';
      default: return 'default';
    }
  };

  const getStatusIcon = (status: string) => {
    switch (status) {
      case 'active': return <CheckCircle size={16} />;
      case 'expired': return <AlertTriangle size={16} />;
      case 'suspended': return <Clock size={16} />;
      case 'cancelled': return <AlertTriangle size={16} />;
      default: return <Clock size={16} />;
    }
  };

  const getDaysUntilExpiry = () => {
    return Math.ceil(subscriptionData.timeRemaining / (24 * 60 * 60)); // Convert seconds to days
  };

  const getMonthsFromSeconds = (seconds: number) => {
    return Math.round(seconds / (30 * 24 * 60 * 60)); // Approximate months
  };

  const daysUntilExpiry = getDaysUntilExpiry();
  const isExpiringSoon = daysUntilExpiry <= 7 && daysUntilExpiry > 0;

  return (
    <div className="space-y-6">
      {/* Subscription Status Overview */}
      <Card className="shadow-sm">
        <CardHeader className="pb-3">
          <div className="flex items-center justify-between w-full">
            <div className="flex items-center gap-3">
              <div className="w-10 h-10 bg-gradient-to-br from-blue-500 to-purple-600 rounded-lg flex items-center justify-center">
                <CreditCard className="text-white" size={20} />
              </div>
              <div>
                <h3 className="text-lg font-semibold text-gray-900">Subscription Status</h3>
                <p className="text-sm text-gray-600">Manage your business subscription</p>
              </div>
            </div>
            <Chip
              color={getStatusColor(subscriptionData.status)}
              variant="flat"
              startContent={getStatusIcon(subscriptionData.status)}
              className="capitalize"
            >
              {subscriptionData.status}
            </Chip>
          </div>
        </CardHeader>
        <CardBody className="pt-0">
          <div className="grid grid-cols-1 md:grid-cols-3 gap-6">
            {/* Current Plan */}
            <div className="space-y-2">
              <div className="flex items-center gap-2 text-sm text-gray-600">
                <TrendingUp size={16} />
                Subscription Model
              </div>
              <div className="text-xl font-semibold text-gray-900 capitalize">
                Pay-as-you-go
              </div>
              <div className="text-sm text-gray-600">
                Last payment: ${(parseFloat(subscriptionData.lastPaymentAmount) / 1000000).toFixed(2)} USDC
              </div>
            </div>

            {/* Subscription End */}
            <div className="space-y-2">
              <div className="flex items-center gap-2 text-sm text-gray-600">
                <Calendar size={16} />
                Subscription Ends
              </div>
              <div className="text-xl font-semibold text-gray-900">
                {subscriptionData.subscriptionEndDate ? new Date(subscriptionData.subscriptionEndDate).toLocaleDateString() : 'Not set'}
              </div>
              <div className="text-sm text-gray-600">
                {daysUntilExpiry > 0 ? `${daysUntilExpiry} days remaining` : 'Expired'}
              </div>
            </div>

            {/* Time Remaining */}
            <div className="space-y-2">
              <div className="flex items-center gap-2 text-sm text-gray-600">
                <Clock size={16} />
                Time Remaining
              </div>
              <div className="text-xl font-semibold text-gray-900">
                {Math.floor(subscriptionData.timeRemaining / (24 * 60 * 60))} days
              </div>
              <div className="text-sm text-gray-600">
                Total paid: ${(parseFloat(subscriptionData.totalPaid) / 1000000).toFixed(2)} USDC
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
                    Your subscription expires in {daysUntilExpiry} day{daysUntilExpiry !== 1 ? 's' : ''}
                  </p>
                  <p className="text-xs text-amber-700 mt-1">
                    Renew now to avoid service interruption
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
              onPress={onRenewOpen}
              startContent={<DollarSign size={16} />}
              className="flex-1"
            >
              Renew Subscription
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
              <h3 className="text-lg font-semibold text-gray-900">Payment History</h3>
              <p className="text-sm text-gray-600">Recent subscription payments</p>
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
                    Subscription Payment
                  </p>
                  <p className="text-sm text-gray-600">
                    {subscriptionData.lastPaymentDate ? new Date(subscriptionData.lastPaymentDate).toLocaleDateString() : 'No payment date'}
                  </p>
                </div>
              </div>
              <div className="text-right">
                <p className="font-semibold text-gray-900">${(parseFloat(subscriptionData.lastPaymentAmount) / 1000000).toFixed(2)} USDC</p>
                <p className="text-sm text-green-600">Paid</p>
              </div>
            </div>
            
            <div className="text-center py-4">
              <p className="text-sm text-gray-500">No additional payment history</p>
            </div>
          </div>
        </CardBody>
      </Card>

      {/* Renew Subscription Modal */}
      <Modal isOpen={isRenewOpen} onClose={onRenewClose} size="2xl">
        <ModalContent>
          <ModalHeader>Renew Subscription</ModalHeader>
          <ModalBody>
            <div className="space-y-6">
              <p className="text-gray-600">
                Choose how much you want to pay to extend your subscription. You&apos;ll get time proportional to the yearly fee.
              </p>

              {/* Wallet Status */}
              {!address && (
                <Card className="bg-amber-50 border border-amber-200">
                  <CardBody className="p-4">
                    <div className="flex items-center gap-3">
                      <AlertTriangle className="text-amber-600" size={20} />
                      <div>
                        <p className="text-sm font-medium text-amber-800">
                          Wallet Not Connected
                        </p>
                        <p className="text-xs text-amber-700">
                          Please connect your wallet to renew your subscription
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
                            Wallet Connected
                          </p>
                          <p className="text-xs text-blue-700">
                            USDC Balance: ${formatUsdcAmount(usdcBalance)}
                          </p>
                        </div>
                      </div>
                    </div>
                  </CardBody>
                </Card>
              )}

              {/* Payment Options */}
              <div className="space-y-4">
                <h4 className="font-semibold text-gray-900">Suggested Payment Options</h4>
                <div className="grid grid-cols-2 gap-3">
                  {renewalOptions.map((option) => (
                    <Card
                      key={option.months}
                      isPressable
                      isHoverable
                      onPress={() => {
                        setSelectedOption(option);
                        setUseCustomAmount(false);
                        setCustomAmount('');
                      }}
                      className={`relative transition-all duration-200 ${
                        !useCustomAmount && selectedOption.months === option.months 
                          ? 'ring-2 ring-blue-500 shadow-lg' 
                          : 'hover:shadow-md'
                      } ${option.popular ? 'border-2 border-blue-500' : ''}`}
                    >
                      {option.popular && (
                        <div className="absolute -top-2 left-1/2 transform -translate-x-1/2">
                          <span className="bg-gradient-to-r from-blue-500 to-purple-600 text-white px-2 py-1 rounded-full text-xs font-medium">
                            Popular
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
                          {!useCustomAmount && selectedOption.months === option.months && calculatedSubscriptionTime 
                            ? formatSubscriptionTime(Number(calculatedSubscriptionTime.toString()))
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
                    useCustomAmount ? 'ring-2 ring-blue-500 shadow-lg' : 'hover:shadow-md'
                  }`}
                >
                  <CardBody className="p-4">
                    <div className="flex items-center gap-4">
                      <Checkbox
                        isSelected={useCustomAmount}
                        onValueChange={(checked) => {
                          setUseCustomAmount(checked);
                          if (!checked) {
                            setCustomAmount('');
                          }
                        }}
                        color="primary"
                      >
                        <span className="font-medium">Pay custom amount</span>
                      </Checkbox>
                      {useCustomAmount && (
                        <Input
                          placeholder="Enter amount"
                          value={customAmount}
                          onChange={(e) => {
                            const value = e.target.value;
                            const yearlyFee = registrationFee && typeof registrationFee === 'bigint' 
                              ? Number(formatUsdcAmount(registrationFee)) 
                              : 120;
                            
                            setCustomAmount(value);
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
                    {useCustomAmount && (
                      <div className="mt-2 space-y-1">
                        {(() => {
                          const yearlyFee = registrationFee && typeof registrationFee === 'bigint' 
                            ? Number(formatUsdcAmount(registrationFee)) 
                            : 120;
                          const customAmountNum = parseFloat(customAmount || '0');
                          const isOverYearlyFee = customAmountNum > yearlyFee;
                          
                          return (
                            <>
                              <p className="text-sm text-gray-600">
                                You&apos;ll get subscription time proportional to the yearly fee (${yearlyFee} = 1 year)
                              </p>
                              {isOverYearlyFee && (
                                <p className="text-sm font-medium text-amber-600 bg-amber-50 px-2 py-1 rounded">
                                  ⚠️ Amount above ${yearlyFee} will still only give 1 year maximum
                                </p>
                              )}
                              {customAmount && calculatedSubscriptionTime ? (
                                <p className="text-sm font-medium text-blue-600">
                                  Estimated time: {formatSubscriptionTime(Number(calculatedSubscriptionTime.toString()))}
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
                    useCoupon ? 'ring-2 ring-green-500 shadow-lg' : 'hover:shadow-md'
                  }`}
                >
                  <CardBody className="p-4">
                    <div className="flex items-center gap-4">
                      <Checkbox
                        isSelected={useCoupon}
                        onValueChange={(checked) => {
                          setUseCoupon(checked);
                          if (!checked) {
                            setCouponCode('');
                          }
                        }}
                        color="success"
                      >
                        <span className="font-medium">Apply coupon code</span>
                      </Checkbox>
                      {useCoupon && (
                        <Input
                          placeholder="Enter coupon code"
                          value={couponCode}
                          onChange={(e) => setCouponCode(e.target.value.toUpperCase())}
                          variant="bordered"
                          size="sm"
                          startContent={<Hash size={16} className="text-gray-400" />}
                          classNames={{
                            inputWrapper: "h-10 border-2",
                          }}
                          className="w-40"
                          color={
                            couponCode ? (
                              isCouponLoading ? "default" :
                              isCouponValid ? "success" : "danger"
                            ) : "default"
                          }
                        />
                      )}
                    </div>
                    {useCoupon && couponCode && !isCouponLoading && (
                      <div className="mt-2">
                        {isCouponValid ? (
                          <p className="text-sm font-medium text-green-600 bg-green-50 px-2 py-1 rounded">
                            ✅ Valid coupon! Discount: ${formatUsdcAmount(couponDiscount)}
                          </p>
                        ) : (
                          <p className="text-sm font-medium text-red-600 bg-red-50 px-2 py-1 rounded">
                            ❌ Invalid or expired coupon code
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
                  <h4 className="font-semibold text-gray-900 mb-3">Payment Summary</h4>
                  <div className="space-y-2">
                    <div className="flex justify-between">
                      <span className="text-gray-600">Payment Amount</span>
                      <span className="font-medium">
                        ${useCustomAmount ? customAmount || '0.00' : selectedOption.suggestedAmount}
                      </span>
                    </div>
                    {useCoupon && isCouponValid && (
                      <div className="flex justify-between">
                        <span className="text-gray-600">Coupon Discount</span>
                        <span className="font-medium text-green-600">
                          -${formatUsdcAmount(couponDiscount)}
                        </span>
                      </div>
                    )}
                    <Divider />
                    <div className="flex justify-between text-lg font-semibold">
                      <span>Total</span>
                      <span>
                        ${(() => {
                          const originalAmount = parseFloat(useCustomAmount ? customAmount || '0' : selectedOption.suggestedAmount);
                          const discount = useCoupon && isCouponValid ? Number(formatUsdcAmount(couponDiscount)) : 0;
                          const finalAmount = Math.max(0, originalAmount - discount);
                          return finalAmount.toFixed(2);
                        })()}
                      </span>
                    </div>
                    {calculatedSubscriptionTime ? (
                      <div className="text-sm text-gray-600 mt-2">
                        Additional time: {formatSubscriptionTime(Number(calculatedSubscriptionTime.toString()))}
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
              onPress={onRenewClose}
              isDisabled={isProcessing}
            >
              Cancel
            </Button>
            <Button 
              color="primary" 
              onPress={handleRenewalPayment}
              isDisabled={
                isProcessing || 
                (useCustomAmount ? !customAmount || parseFloat(customAmount) <= 0 : false) ||
                !address
              }
              isLoading={isProcessing}
            >
              {isProcessing ? (
                processingStep === 'approval' ? 'Approving USDC...' :
                processingStep === 'renewal' ? 'Processing Renewal...' :
                'Complete!'
              ) : (
                'Confirm Payment'
              )}
            </Button>
          </ModalFooter>
        </ModalContent>
      </Modal>

    </div>
  );
};

export default SubscriptionManagement;
