'use client';

import React, { useState } from 'react';
import {
  Card,
  CardBody,
  CardHeader,
  Button,
  Input,
  Select,
  SelectItem,
  Modal,
  ModalContent,
  ModalHeader,
  ModalBody,
  ModalFooter,
  useDisclosure,
  Chip,
  Progress,
  Divider,
  RadioGroup,
  Radio
} from '@nextui-org/react';
import { 
  PaymentMethod, 
  PAYMENT_METHOD_OPTIONS, 
  formatUSDCAmount,
  parseUSDCAmount,
  getPaymentMethodIcon,
  getPaymentMethodLabel
} from '@/types/alternativePayments';
import { useGuestAlternativePayments } from '@/hooks/useAlternativePayments';
import { toast } from 'react-hot-toast';
import { Wallet, CreditCard, DollarSign, Smartphone, MoreHorizontal } from 'lucide-react';

interface PaymentMethodSelectorProps {
  billId: string;
  billTotal: number;
  businessName: string;
  onPaymentComplete?: () => void;
}

export default function PaymentMethodSelector({
  billId,
  billTotal,
  businessName,
  onPaymentComplete
}: PaymentMethodSelectorProps) {
  // Use the guest hook for real-time data
  const {
    paymentBreakdown,
    loading,
    error,
    requestPayment
  } = useGuestAlternativePayments(billId);

  const [selectedMethod, setSelectedMethod] = useState<PaymentMethod>(PaymentMethod.CRYPTO);
  const [customAmount, setCustomAmount] = useState('');
  const [participantName, setParticipantName] = useState('');
  const [requesting, setRequesting] = useState(false);
  
  const { isOpen: isCryptoModalOpen, onOpen: onCryptoModalOpen, onClose: onCryptoModalClose } = useDisclosure();
  const { isOpen: isAlternativeModalOpen, onOpen: onAlternativeModalOpen, onClose: onAlternativeModalClose } = useDisclosure();

  const handleCryptoPayment = () => {
    onCryptoModalOpen();
  };

  const handleAlternativePayment = (method: PaymentMethod) => {
    setSelectedMethod(method);
    onAlternativeModalOpen();
  };

  const handleAlternativePaymentRequest = async () => {
    if (!customAmount || parseFloat(customAmount) <= 0) {
      toast.error('Please enter a valid amount');
      return;
    }

    setRequesting(true);
    try {
      const response = await requestPayment(
        parseUSDCAmount(customAmount),
        selectedMethod,
        participantName || undefined
      );

      if (response.success) {
        toast.success('Payment request sent to business owner!');
        onAlternativeModalClose();
        setCustomAmount('');
        setParticipantName('');
      } else {
        toast.error(response.message);
      }
    } catch (error) {
      console.error('Error requesting alternative payment:', error);
      toast.error('Failed to send payment request');
    } finally {
      setRequesting(false);
    }
  };

  const getPaymentProgress = () => {
    if (!paymentBreakdown) return 0;
    
    const total = parseFloat(paymentBreakdown.totalAmount) / 1_000_000;
    const paid = (parseFloat(paymentBreakdown.cryptoPaid) + parseFloat(paymentBreakdown.alternativePaid)) / 1_000_000;
    
    return total > 0 ? (paid / total) * 100 : 0;
  };

  const getRemainingAmount = () => {
    if (!paymentBreakdown) return billTotal;
    return parseFloat(formatUSDCAmount(paymentBreakdown.remaining));
  };

  const getMethodIcon = (method: PaymentMethod) => {
    switch (method) {
      case PaymentMethod.CRYPTO:
        return <Wallet className="w-5 h-5" />;
      case PaymentMethod.CASH:
        return <DollarSign className="w-5 h-5" />;
      case PaymentMethod.CARD:
        return <CreditCard className="w-5 h-5" />;
      case PaymentMethod.VENMO:
        return <Smartphone className="w-5 h-5" />;
      default:
        return <MoreHorizontal className="w-5 h-5" />;
    }
  };

  if (paymentBreakdown?.isComplete) {
    return (
      <Card className="border-green-200 bg-green-50">
        <CardBody className="text-center p-6">
          <div className="text-green-600 mb-2">
            <Wallet className="w-12 h-12 mx-auto mb-2" />
          </div>
          <h3 className="text-lg font-semibold text-green-800 mb-2">Bill Paid in Full</h3>
          <p className="text-green-600">
            Thank you! This bill has been completely paid.
          </p>
        </CardBody>
      </Card>
    );
  }

  return (
    <div className="space-y-6">
      {/* Payment Progress */}
      {paymentBreakdown && (
        <Card>
          <CardHeader>
            <h3 className="text-lg font-semibold">Payment Progress</h3>
          </CardHeader>
          <CardBody>
            <div className="space-y-4">
              <Progress 
                value={getPaymentProgress()} 
                color="primary"
                className="w-full"
              />
              
              <div className="grid grid-cols-3 gap-4 text-sm">
                <div className="text-center">
                  <p className="text-gray-500">Total</p>
                  <p className="font-semibold">${formatUSDCAmount(paymentBreakdown.totalAmount)}</p>
                </div>
                <div className="text-center">
                  <p className="text-gray-500">Paid</p>
                  <p className="font-semibold text-green-600">
                    ${formatUSDCAmount((parseFloat(paymentBreakdown.cryptoPaid) + parseFloat(paymentBreakdown.alternativePaid)).toString())}
                  </p>
                </div>
                <div className="text-center">
                  <p className="text-gray-500">Remaining</p>
                  <p className="font-semibold text-orange-600">${formatUSDCAmount(paymentBreakdown.remaining)}</p>
                </div>
              </div>
            </div>
          </CardBody>
        </Card>
      )}

      {/* Payment Method Selection */}
      <Card>
        <CardHeader>
          <h3 className="text-lg font-semibold">Choose Payment Method</h3>
        </CardHeader>
        <CardBody>
          <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
            {/* Crypto Payment */}
            <Card 
              isPressable 
              onPress={handleCryptoPayment}
              className="border-2 border-transparent hover:border-primary transition-colors"
            >
              <CardBody className="text-center p-6">
                <div className="text-primary mb-3">
                  {getMethodIcon(PaymentMethod.CRYPTO)}
                </div>
                <h4 className="font-semibold mb-2">Pay with Crypto</h4>
                <p className="text-sm text-gray-600 mb-3">
                  Instant payment with USDC
                </p>
                <Chip color="primary" variant="flat" size="sm">
                  Recommended
                </Chip>
              </CardBody>
            </Card>

            {/* Alternative Payment Methods */}
            {PAYMENT_METHOD_OPTIONS.filter(option => option.value !== PaymentMethod.CRYPTO).map((option) => (
              <Card 
                key={option.value}
                isPressable 
                onPress={() => handleAlternativePayment(option.value)}
                className="border-2 border-transparent hover:border-secondary transition-colors"
              >
                <CardBody className="text-center p-6">
                  <div className="text-secondary mb-3">
                    {getMethodIcon(option.value)}
                  </div>
                  <h4 className="font-semibold mb-2">{option.label}</h4>
                  <p className="text-sm text-gray-600">
                    {option.description}
                  </p>
                </CardBody>
              </Card>
            ))}
          </div>
        </CardBody>
      </Card>

      {/* Crypto Payment Modal */}
      <Modal isOpen={isCryptoModalOpen} onClose={onCryptoModalClose} size="lg">
        <ModalContent>
          <ModalHeader>Pay with Crypto (USDC)</ModalHeader>
          <ModalBody>
            <div className="text-center p-6">
              <p className="text-gray-600 mb-4">
                You&apos;ll be redirected to connect your wallet and complete the payment.
              </p>
              <Button
                color="primary"
                size="lg"
                onPress={() => {
                  // This would integrate with the existing PaymentProcessor modal
                  // For now, we'll show a placeholder
                  toast.success('Crypto payment integration coming soon!');
                  onCryptoModalClose();
                }}
              >
                Connect Wallet & Pay
              </Button>
            </div>
          </ModalBody>
        </ModalContent>
      </Modal>

      {/* Alternative Payment Modal */}
      <Modal isOpen={isAlternativeModalOpen} onClose={onAlternativeModalClose} size="lg">
        <ModalContent>
          <ModalHeader>
            Pay with {getPaymentMethodLabel(selectedMethod)}
          </ModalHeader>
          <ModalBody>
            <div className="space-y-4">
              <div className="bg-blue-50 p-4 rounded-lg">
                <div className="flex items-center space-x-3 mb-2">
                  <span className="text-2xl">{getPaymentMethodIcon(selectedMethod)}</span>
                  <div>
                    <h4 className="font-semibold">{getPaymentMethodLabel(selectedMethod)} Payment</h4>
                    <p className="text-sm text-gray-600">
                      Request to pay with {getPaymentMethodLabel(selectedMethod).toLowerCase()}
                    </p>
                  </div>
                </div>
              </div>

              <Input
                label="Your Name (Optional)"
                placeholder="Enter your name"
                value={participantName}
                onValueChange={setParticipantName}
                description="Help the business identify your payment"
              />
              
              <Input
                label="Amount (USD)"
                placeholder="0.00"
                type="number"
                step="0.01"
                min="0"
                max={getRemainingAmount()}
                value={customAmount}
                onValueChange={setCustomAmount}
                startContent={<span className="text-gray-500">$</span>}
                description={`Remaining: $${getRemainingAmount().toFixed(2)}`}
              />
              
              <div className="bg-orange-50 p-3 rounded-lg">
                <p className="text-sm text-orange-800">
                  <strong>Note:</strong> This will send a payment request to {businessName}. 
                  They will confirm once they receive your {getPaymentMethodLabel(selectedMethod).toLowerCase()} payment.
                </p>
              </div>
            </div>
          </ModalBody>
          <ModalFooter>
            <Button variant="light" onPress={onAlternativeModalClose}>
              Cancel
            </Button>
            <Button
              color="primary"
              isLoading={requesting}
              onPress={handleAlternativePaymentRequest}
            >
              Send Payment Request
            </Button>
          </ModalFooter>
        </ModalContent>
      </Modal>
    </div>
  );
}
