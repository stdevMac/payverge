'use client';

import React, { useState } from 'react';
import {
  Modal,
  ModalContent,
  ModalHeader,
  ModalBody,
  ModalFooter,
  Button,
  Card,
  CardBody,
  Divider,
  Input,
  Chip,
} from '@nextui-org/react';
import { Wallet, AlertCircle, CheckCircle2 } from 'lucide-react';
import { useAccount } from 'wagmi';
import WalletConnector from './WalletConnector';

interface PaymentModalProps {
  isOpen: boolean;
  onClose: () => void;
  billId: number;
  amount: number;
  businessName: string;
  onPaymentComplete?: () => void;
}

export default function PaymentModal({ 
  isOpen, 
  onClose, 
  billId, 
  amount, 
  businessName, 
  onPaymentComplete 
}: PaymentModalProps) {
  const [paymentStep, setPaymentStep] = useState<'amount' | 'wallet' | 'processing' | 'success'>('amount');
  const [tipAmount, setTipAmount] = useState<string>('0');
  const [isProcessing, setIsProcessing] = useState(false);
  
  const { isConnected } = useAccount();

  const formatCurrency = (value: number) => `$${value.toFixed(2)}`;
  
  const tipPresets = [0, 0.15, 0.18, 0.20, 0.25];
  const tipValue = parseFloat(tipAmount) || 0;
  const totalAmount = amount + tipValue;

  const handleTipPreset = (percentage: number) => {
    const tip = amount * percentage;
    setTipAmount(tip.toFixed(2));
  };

  const handleNext = () => {
    if (paymentStep === 'amount') {
      if (!isConnected) {
        setPaymentStep('wallet');
      } else {
        handlePayment();
      }
    } else if (paymentStep === 'wallet' && isConnected) {
      handlePayment();
    }
  };

  const handlePayment = async () => {
    setIsProcessing(true);
    setPaymentStep('processing');
    
    // Simulate payment processing
    try {
      await new Promise(resolve => setTimeout(resolve, 3000));
      setPaymentStep('success');
      setTimeout(() => {
        onPaymentComplete?.();
        handleClose();
      }, 2000);
    } catch (error) {
      setIsProcessing(false);
      setPaymentStep('amount');
    }
  };

  const handleClose = () => {
    setPaymentStep('amount');
    setTipAmount('0');
    setIsProcessing(false);
    onClose();
  };

  const renderAmountStep = () => (
    <>
      <ModalHeader>
        <div className="flex items-center gap-2">
          <Wallet size={20} />
          <span>Payment for {businessName}</span>
        </div>
      </ModalHeader>
      <ModalBody>
        <Card>
          <CardBody className="space-y-4">
            <div className="text-center">
              <h3 className="text-lg font-semibold mb-2">Bill Amount</h3>
              <p className="text-3xl font-bold text-primary">{formatCurrency(amount)}</p>
            </div>

            <Divider />

            <div>
              <h4 className="font-medium mb-3">Add Tip (Optional)</h4>
              <div className="grid grid-cols-5 gap-2 mb-3">
                {tipPresets.map((preset) => (
                  <Button
                    key={preset}
                    size="sm"
                    variant={tipValue === amount * preset ? "solid" : "bordered"}
                    color={tipValue === amount * preset ? "primary" : "default"}
                    onPress={() => handleTipPreset(preset)}
                  >
                    {preset === 0 ? 'No Tip' : `${(preset * 100).toFixed(0)}%`}
                  </Button>
                ))}
              </div>
              <Input
                type="number"
                label="Custom Tip Amount"
                placeholder="0.00"
                value={tipAmount}
                onValueChange={setTipAmount}
                startContent="$"
                min="0"
                step="0.01"
              />
            </div>

            <Divider />

            <div className="space-y-2">
              <div className="flex justify-between">
                <span>Bill Amount:</span>
                <span>{formatCurrency(amount)}</span>
              </div>
              <div className="flex justify-between">
                <span>Tip:</span>
                <span>{formatCurrency(tipValue)}</span>
              </div>
              <div className="flex justify-between font-semibold text-lg">
                <span>Total:</span>
                <span>{formatCurrency(totalAmount)}</span>
              </div>
            </div>
          </CardBody>
        </Card>
      </ModalBody>
      <ModalFooter>
        <Button variant="light" onPress={handleClose}>
          Cancel
        </Button>
        <Button color="primary" onPress={handleNext}>
          {isConnected ? 'Pay Now' : 'Connect Wallet'}
        </Button>
      </ModalFooter>
    </>
  );

  const renderWalletStep = () => (
    <>
      <ModalHeader>Connect Your Wallet</ModalHeader>
      <ModalBody>
        <WalletConnector showBalance={false} showDisconnect={false} />
      </ModalBody>
      <ModalFooter>
        <Button variant="light" onPress={handleClose}>
          Cancel
        </Button>
        {isConnected && (
          <Button color="primary" onPress={handleNext}>
            Continue to Payment
          </Button>
        )}
      </ModalFooter>
    </>
  );

  const renderProcessingStep = () => (
    <>
      <ModalHeader>Processing Payment</ModalHeader>
      <ModalBody>
        <div className="text-center space-y-4">
          <div className="animate-spin mx-auto w-8 h-8 border-4 border-primary border-t-transparent rounded-full" />
          <div>
            <h3 className="font-semibold mb-2">Processing your payment...</h3>
            <p className="text-sm text-gray-600">
              Please wait while we process your payment of {formatCurrency(totalAmount)}
            </p>
          </div>
        </div>
      </ModalBody>
    </>
  );

  const renderSuccessStep = () => (
    <>
      <ModalHeader>
        <div className="flex items-center gap-2 text-success">
          <CheckCircle2 size={20} />
          <span>Payment Successful</span>
        </div>
      </ModalHeader>
      <ModalBody>
        <div className="text-center space-y-4">
          <CheckCircle2 size={48} className="mx-auto text-success" />
          <div>
            <h3 className="font-semibold mb-2">Payment Complete!</h3>
            <p className="text-sm text-gray-600">
              Your payment of {formatCurrency(totalAmount)} has been processed successfully.
            </p>
          </div>
          <Card className="bg-success-50">
            <CardBody>
              <div className="space-y-2 text-sm">
                <div className="flex justify-between">
                  <span>Bill ID:</span>
                  <span>#{billId}</span>
                </div>
                <div className="flex justify-between">
                  <span>Amount Paid:</span>
                  <span>{formatCurrency(totalAmount)}</span>
                </div>
                <div className="flex justify-between">
                  <span>Status:</span>
                  <Chip size="sm" color="success">Confirmed</Chip>
                </div>
              </div>
            </CardBody>
          </Card>
        </div>
      </ModalBody>
    </>
  );

  return (
    <Modal 
      isOpen={isOpen} 
      onClose={handleClose}
      size="md"
      closeButton={paymentStep !== 'processing'}
    >
      <ModalContent>
        {paymentStep === 'amount' && renderAmountStep()}
        {paymentStep === 'wallet' && renderWalletStep()}
        {paymentStep === 'processing' && renderProcessingStep()}
        {paymentStep === 'success' && renderSuccessStep()}
      </ModalContent>
    </Modal>
  );
}
