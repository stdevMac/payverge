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
  Progress,
} from '@nextui-org/react';
import { Wallet, AlertCircle, CheckCircle2, ExternalLink } from 'lucide-react';

interface PaymentProcessorProps {
  isOpen: boolean;
  onClose: () => void;
  billId: number;
  amount: number;
  businessName: string;
  businessAddress: string;
  tipAddress: string;
  onPaymentComplete?: () => void;
}

type PaymentStep = 'amount' | 'wallet' | 'approval' | 'payment' | 'confirming' | 'success' | 'error';

export default function PaymentProcessor({
  isOpen,
  onClose,
  billId,
  amount,
  businessName,
  businessAddress,
  tipAddress,
  onPaymentComplete
}: PaymentProcessorProps) {
  console.log('PaymentProcessor: Rendering with props:', { isOpen, billId, amount });

  const [paymentStep, setPaymentStep] = useState<PaymentStep>('amount');
  const [tipAmount, setTipAmount] = useState<string>('0');
  const [error, setError] = useState<string>('');
  const [transactionHash, setTransactionHash] = useState<string>('');

  // Mock wagmi hooks for now
  const isConnected = false;
  const address = null;

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
        handleApproval();
      }
    } else if (paymentStep === 'wallet' && isConnected) {
      handleApproval();
    }
  };

  const handleApproval = async () => {
    setPaymentStep('approval');
    setError('');

    try {
      // TODO: Implement USDC approval when wagmi is set up
      console.log('Mock: USDC approval for amount:', totalAmount);
      setTimeout(() => {
        setPaymentStep('payment');
      }, 1000);
    } catch (err) {
      setError('Failed to approve USDC spending');
      setPaymentStep('error');
    }
  };

  const handlePayment = async () => {
    setPaymentStep('payment');
    setError('');

    try {
      // TODO: Implement blockchain payment when wagmi is set up
      console.log('Mock: Processing payment for bill:', billId, 'amount:', totalAmount);
      
      // Simulate payment processing
      setTimeout(() => {
        setPaymentStep('success');
        onPaymentComplete?.();
      }, 2000);
    } catch (err) {
      setError('Failed to process payment');
      setPaymentStep('error');
    }
  };

  const notifyBackend = async () => {
    try {
      // TODO: Implement backend notification when API is ready
      console.log('Mock: Notifying backend of payment completion');
    } catch (err) {
      console.error('Failed to notify backend:', err);
    }
  };

  // TODO: Re-enable when wagmi is set up
  // React.useEffect(() => {
  //   if (transactionHash && paymentStep !== 'success') {
  //     handlePayment()
  //   }
  // }, [transactionHash, paymentStep]);

  // React.useEffect(() => {
  //   if (isPaymentSuccess) {
  //     setPaymentStep('success');
  //     notifyBackend();
  //   }
  // }, [isPaymentSuccess]);


  const handleClose = () => {
    setPaymentStep('amount');
    setTipAmount('0');
    setError('');
    setTransactionHash('');
    onClose();
  };

  const handleSuccess = () => {
    onPaymentComplete?.();
    handleClose();
  };

  const renderAmountStep = () => (
    <>
      <ModalHeader className="flex flex-col gap-1 pb-2">
        <div className="flex items-center gap-3">
          <div className="w-10 h-10 bg-primary-100 rounded-full flex items-center justify-center">
            <Wallet className="w-5 h-5 text-primary-600" />
          </div>
          <div>
            <h2 className="text-xl font-semibold text-gray-900">Payment Details</h2>
            <p className="text-sm text-gray-500">{businessName}</p>
          </div>
        </div>
      </ModalHeader>
      <ModalBody className="py-6">
        <div className="space-y-6">
          {/* Bill Summary Card */}
          <Card className="bg-gradient-to-br from-gray-50 to-gray-100 border-0 shadow-sm">
            <CardBody className="p-6">
              <div className="space-y-4">
                <div className="flex justify-between items-center">
                  <span className="text-gray-600 font-medium">Bill Amount</span>
                  <span className="text-xl font-bold text-gray-900">{formatCurrency(amount)}</span>
                </div>

                <>
                  <Divider className="bg-gray-200" />
                  <div className="flex justify-between items-center">
                    <span className="text-gray-600 font-medium">Tip</span>
                    <span className="text-lg font-semibold text-green-600">{formatCurrency(tipValue)}</span>
                  </div>
                </>

                <Divider className="bg-gray-300" />

                <div className="flex justify-between items-center p-3 bg-white rounded-lg shadow-sm">
                  <span className="text-lg font-bold text-gray-900">Total</span>
                  <span className="text-2xl font-bold text-primary-600">{formatCurrency(totalAmount)}</span>
                </div>
              </div>
            </CardBody>
          </Card>

          {/* Tip Selection */}
          <div className="space-y-4">
            <div className="flex items-center gap-2">
              <h4 className="text-lg font-semibold text-gray-900">Add Tip</h4>
              <Chip size="sm" variant="flat" color="success" className="text-xs">
                Optional
              </Chip>
            </div>

            <div className="grid grid-cols-3 gap-3">
              {tipPresets.map((preset) => (
                <Button
                  key={preset}
                  size="lg"
                  variant={tipValue === amount * preset ? "solid" : "bordered"}
                  color={tipValue === amount * preset ? "primary" : "default"}
                  onPress={() => handleTipPreset(preset)}
                  className={`h-14 font-semibold transition-all duration-200 ${tipValue === amount * preset
                      ? 'shadow-lg scale-105'
                      : 'hover:scale-102 hover:shadow-md'
                    }`}
                >
                  <div className="text-center">
                    <div className="text-sm font-bold">
                      {preset === 0 ? 'No Tip' : `${(preset * 100).toFixed(0)}%`}
                    </div>
                    {preset > 0 && (
                      <div className="text-xs opacity-80">
                        {formatCurrency(amount * preset)}
                      </div>
                    )}
                  </div>
                </Button>
              ))}
            </div>

            {/* Custom Tip Section */}
            <div className="mt-6 pt-4 border-t border-gray-200">
              <h5 className="text-sm font-medium text-gray-700 mb-3">Or enter a custom amount</h5>
              <Input
                type="number"
                label="Custom Tip Amount"
                placeholder="0.00"
                value={tipAmount}
                onChange={(e) => setTipAmount(e.target.value)}
                startContent={
                  <div className="pointer-events-none flex items-center">
                    <span className="text-default-400 text-base font-medium">$</span>
                  </div>
                }
                min="0"
                step="0.01"
                size="lg"
                classNames={{
                  input: "text-xl font-semibold",
                  inputWrapper: "h-16 bg-gray-50 border-gray-200 hover:bg-gray-100 focus-within:bg-white transition-colors",
                  label: "text-gray-600 font-medium"
                }}
              />
            </div>
          </div>
        </div>
      </ModalBody>
      <ModalFooter className="pt-6 pb-4">
        <Button
          variant="light"
          onPress={onClose}
          size="lg"
          className="font-semibold"
        >
          Cancel
        </Button>
        <Button
          color="primary"
          onPress={handleNext}
          size="lg"
          className="font-semibold px-8 shadow-lg"
          endContent={<ExternalLink className="w-4 h-4" />}
        >
          Continue to Payment
        </Button>
      </ModalFooter>
    </>
  );

  const renderWalletStep = () => (
    <>
      <ModalHeader className="flex flex-col gap-1 pb-2">
        <div className="flex items-center gap-3">
          <div className="w-10 h-10 bg-blue-100 rounded-full flex items-center justify-center">
            <Wallet className="w-5 h-5 text-blue-600" />
          </div>
          <div>
            <h2 className="text-xl font-semibold text-gray-900">Connect Your Wallet</h2>
            <p className="text-sm text-gray-500">Choose a wallet to pay with USDC</p>
          </div>
        </div>
      </ModalHeader>
      <ModalBody className="py-6">
        <div className="space-y-6">
          <Card className="bg-gradient-to-br from-blue-50 to-indigo-50 border-0 shadow-sm">
            <CardBody className="p-6 text-center">
              <div className="w-16 h-16 bg-blue-100 rounded-full flex items-center justify-center mx-auto mb-4">
                <Wallet className="w-8 h-8 text-blue-600" />
              </div>
              <h3 className="text-lg font-semibold text-gray-900 mb-2">Secure Payment</h3>
              <p className="text-gray-600 text-sm leading-relaxed">
                Your payment will be processed securely using USDC on the blockchain.
                Connect your wallet to continue.
              </p>
            </CardBody>
          </Card>

          {/* TODO: Re-enable when WalletConnector is set up */}
          {/* <WalletConnector showBalance={true} showDisconnect={false} /> */}
          <div className="text-center text-gray-600">
            <p>🔗 Wallet connection coming soon</p>
          </div>
        </div>
      </ModalBody>
    </>
  );

  const renderProcessingStep = (title: string, description: string) => (
    <>
      <ModalHeader>{title}</ModalHeader>
      <ModalBody>
        <div className="text-center space-y-4">
          <div className="animate-spin mx-auto w-8 h-8 border-4 border-primary border-t-transparent rounded-full" />
          <div>
            <h3 className="font-semibold mb-2">{description}</h3>
            <p className="text-sm text-gray-600">
              Total amount: {formatCurrency(totalAmount)}
            </p>
            <Progress
              size="sm"
              isIndeterminate
              color="primary"
              className="mt-4"
            />
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
                {transactionHash && (
                  <div className="flex justify-between items-center">
                    <span>Transaction:</span>
                    <Button
                      size="sm"
                      variant="light"
                      endContent={<ExternalLink size={12} />}
                      onPress={() => window.open(`https://etherscan.io/tx/${transactionHash}`, '_blank')}
                    >
                      View
                    </Button>
                  </div>
                )}
              </div>
            </CardBody>
          </Card>
        </div>
      </ModalBody>
      <ModalFooter>
        <Button color="primary" onPress={handleSuccess}>
          Done
        </Button>
      </ModalFooter>
    </>
  );

  const renderErrorStep = () => (
    <>
      <ModalHeader>
        <div className="flex items-center gap-2 text-danger">
          <AlertCircle size={20} />
          <span>Payment Failed</span>
        </div>
      </ModalHeader>
      <ModalBody>
        <div className="text-center space-y-4">
          <AlertCircle size={48} className="mx-auto text-danger" />
          <div>
            <h3 className="font-semibold mb-2">Payment Error</h3>
            <p className="text-sm text-gray-600">{error}</p>
          </div>
        </div>
      </ModalBody>
      <ModalFooter>
        <Button variant="light" onPress={handleClose}>
          Cancel
        </Button>
        <Button color="primary" onPress={() => setPaymentStep('amount')}>
          Try Again
        </Button>
      </ModalFooter>
    </>
  );

  return (
    <Modal
      isOpen={isOpen}
      onClose={handleClose}
      size="md"
      closeButton={!['approval', 'payment', 'confirming'].includes(paymentStep)}
    >
      <ModalContent>
        {paymentStep === 'amount' && renderAmountStep()}
        {paymentStep === 'wallet' && renderWalletStep()}
        {paymentStep === 'approval' && renderProcessingStep('Approving USDC', 'Please approve USDC spending in your wallet...')}
        {paymentStep === 'payment' && renderProcessingStep('Processing Payment', 'Please confirm the payment transaction in your wallet...')}
        {paymentStep === 'confirming' && renderProcessingStep('Confirming Payment', 'Waiting for blockchain confirmation...')}
        {paymentStep === 'success' && renderSuccessStep()}
        {paymentStep === 'error' && renderErrorStep()}
      </ModalContent>
    </Modal>
  );
}
