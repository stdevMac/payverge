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
  Chip,
  Modal,
  ModalContent,
  ModalHeader,
  ModalBody,
  ModalFooter,
  useDisclosure,
  Divider,
  Progress,
  Tooltip
} from '@nextui-org/react';
import { 
  PaymentMethod, 
  PAYMENT_METHOD_OPTIONS, 
  formatUSDCAmount,
  parseUSDCAmount,
  getPaymentMethodIcon,
  getPaymentMethodLabel
} from '@/types/alternativePayments';
import { useBusinessAlternativePayments } from '@/hooks/useAlternativePayments';
import { toast } from 'react-hot-toast';
import { useSimpleLocale, getTranslation } from '@/i18n/SimpleTranslationProvider';

interface AlternativePaymentManagerProps {
  billId: string;
  billTotal: number;
  onPaymentMarked?: () => void;
}

export default function AlternativePaymentManager({
  billId,
  billTotal,
  onPaymentMarked
}: AlternativePaymentManagerProps) {
  const { locale: currentLocale } = useSimpleLocale();
  
  // Translation helper
  const tString = (key: string): string => {
    const fullKey = `alternativePaymentManager.${key}`;
    const result = getTranslation(fullKey, currentLocale);
    return Array.isArray(result) ? result[0] || key : result as string;
  };
  
  // Use the custom hook for real-time data
  const {
    paymentBreakdown,
    pendingPayments,
    loading,
    error,
    markPayment
  } = useBusinessAlternativePayments(billId);
  
  const [markingPayment, setMarkingPayment] = useState(false);
  
  // Manual payment form state
  const [participantAddress, setParticipantAddress] = useState('');
  const [amount, setAmount] = useState('');
  const [selectedMethod, setSelectedMethod] = useState<PaymentMethod>(PaymentMethod.CASH);
  
  const { isOpen, onOpen, onClose } = useDisclosure();

  const handleMarkPayment = async (
    address: string,
    paymentAmount: string,
    method: PaymentMethod
  ) => {
    setMarkingPayment(true);
    try {
      const response = await markPayment(
        address,
        parseUSDCAmount(paymentAmount),
        method
      );

      if (response.success) {
        toast.success('Alternative payment marked successfully!');
        onPaymentMarked?.();
        
        // Clear form if it was manual entry
        if (address === participantAddress) {
          setParticipantAddress('');
          setAmount('');
          onClose();
        }
      } else {
        toast.error(response.message);
      }
    } catch (error) {
      console.error('Error marking payment:', error);
      toast.error('Failed to mark payment');
    } finally {
      setMarkingPayment(false);
    }
  };

  const handleManualPaymentSubmit = () => {
    if (!participantAddress || !amount) {
      toast.error('Please fill in all fields');
      return;
    }

    const numAmount = parseFloat(amount);
    if (numAmount <= 0) {
      toast.error('Amount must be greater than 0');
      return;
    }

    handleMarkPayment(participantAddress, amount, selectedMethod);
  };

  const getPaymentProgress = () => {
    if (!paymentBreakdown) return 0;
    
    const total = parseFloat(paymentBreakdown.totalAmount) / 1_000_000;
    const paid = (parseFloat(paymentBreakdown.cryptoPaid) + parseFloat(paymentBreakdown.alternativePaid)) / 1_000_000;
    
    return total > 0 ? (paid / total) * 100 : 0;
  };

  if (loading) {
    return (
      <Card>
        <CardBody className="flex items-center justify-center p-8">
          <div className="text-center">
            <div className="animate-spin rounded-full h-8 w-8 border-b-2 border-primary mx-auto mb-4"></div>
            <p>Loading payment data...</p>
          </div>
        </CardBody>
      </Card>
    );
  }

  return (
    <div className="space-y-6">
      {/* Payment Overview */}
      <Card>
        <CardHeader>
          <div className="flex justify-between items-center w-full">
            <h3 className="text-lg font-semibold">Payment Status</h3>
            {paymentBreakdown?.isComplete && (
              <Chip color="success" variant="flat">Bill Complete</Chip>
            )}
          </div>
        </CardHeader>
        <CardBody>
          {paymentBreakdown && (
            <div className="space-y-4">
              <Progress 
                value={getPaymentProgress()} 
                color={paymentBreakdown.isComplete ? "success" : "primary"}
                className="w-full"
              />
              
              <div className="grid grid-cols-2 md:grid-cols-4 gap-4 text-sm">
                <div>
                  <p className="text-gray-500">Total Bill</p>
                  <p className="font-semibold">${formatUSDCAmount(paymentBreakdown.totalAmount)}</p>
                </div>
                <div>
                  <p className="text-gray-500">Crypto Paid</p>
                  <p className="font-semibold text-green-600">${formatUSDCAmount(paymentBreakdown.cryptoPaid)}</p>
                </div>
                <div>
                  <p className="text-gray-500">Alternative Paid</p>
                  <p className="font-semibold text-blue-600">${formatUSDCAmount(paymentBreakdown.alternativePaid)}</p>
                </div>
                <div>
                  <p className="text-gray-500">Remaining</p>
                  <p className="font-semibold text-orange-600">${formatUSDCAmount(paymentBreakdown.remaining)}</p>
                </div>
              </div>
            </div>
          )}
        </CardBody>
      </Card>

      {/* Pending Payments */}
      {pendingPayments.length > 0 && (
        <Card>
          <CardHeader>
            <h3 className="text-lg font-semibold">Pending Alternative Payments</h3>
          </CardHeader>
          <CardBody>
            <div className="space-y-3">
              {pendingPayments.map((payment) => (
                <div key={payment.id} className="flex items-center justify-between p-3 border rounded-lg">
                  <div className="flex items-center space-x-3">
                    <span className="text-2xl">{getPaymentMethodIcon(payment.paymentMethod)}</span>
                    <div>
                      <p className="font-medium">
                        {payment.participantName || `${payment.participantAddress.slice(0, 6)}...${payment.participantAddress.slice(-4)}`}
                      </p>
                      <p className="text-sm text-gray-500">
                        ${formatUSDCAmount(payment.amount)} via {getPaymentMethodLabel(payment.paymentMethod)}
                      </p>
                    </div>
                  </div>
                  <Button
                    color="primary"
                    size="sm"
                    isLoading={markingPayment}
                    onPress={() => handleMarkPayment(
                      payment.participantAddress,
                      formatUSDCAmount(payment.amount),
                      payment.paymentMethod
                    )}
                  >
                    Confirm Payment
                  </Button>
                </div>
              ))}
            </div>
          </CardBody>
        </Card>
      )}

      {/* Manual Payment Entry */}
      <Card>
        <CardHeader>
          <div className="flex justify-between items-center w-full">
            <h3 className="text-lg font-semibold">Mark Alternative Payment</h3>
            <Button color="primary" onPress={onOpen}>
              Add Payment
            </Button>
          </div>
        </CardHeader>
        <CardBody>
          <p className="text-gray-600">
            Manually mark payments made through cash, card, or other methods.
          </p>
        </CardBody>
      </Card>

      {/* Manual Payment Modal */}
      <Modal isOpen={isOpen} onClose={onClose} size="lg">
        <ModalContent>
          <ModalHeader>Mark Alternative Payment</ModalHeader>
          <ModalBody>
            <div className="space-y-4">
              <Input
                label="Customer Address"
                placeholder="0x... or customer identifier"
                value={participantAddress}
                onValueChange={setParticipantAddress}
                description="Ethereum address or customer identifier"
              />
              
              <Input
                label="Amount (USD)"
                placeholder="0.00"
                type="number"
                step="0.01"
                min="0"
                value={amount}
                onValueChange={setAmount}
                startContent={<span className="text-gray-500">$</span>}
              />
              
              <Select
                label="Payment Method"
                selectedKeys={[selectedMethod.toString()]}
                onSelectionChange={(keys) => {
                  const key = Array.from(keys)[0] as string;
                  setSelectedMethod(parseInt(key) as PaymentMethod);
                }}
              >
                {PAYMENT_METHOD_OPTIONS.filter(option => option.value !== PaymentMethod.CRYPTO).map((option) => (
                  <SelectItem key={option.value.toString()} value={option.value.toString()}>
                    <div className="flex items-center space-x-2">
                      <span>{option.icon}</span>
                      <span>{option.label}</span>
                    </div>
                  </SelectItem>
                ))}
              </Select>
              
              <div className="bg-blue-50 p-3 rounded-lg">
                <p className="text-sm text-blue-800">
                  <strong>Note:</strong> Only mark payments as received after you have physically collected the payment.
                  This action cannot be undone.
                </p>
              </div>
            </div>
          </ModalBody>
          <ModalFooter>
            <Button variant="light" onPress={onClose}>
              Cancel
            </Button>
            <Button
              color="primary"
              isLoading={markingPayment}
              onPress={handleManualPaymentSubmit}
            >
              Mark Payment Received
            </Button>
          </ModalFooter>
        </ModalContent>
      </Modal>
    </div>
  );
}
