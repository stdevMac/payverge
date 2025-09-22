'use client';

import React, { useState, useEffect, useCallback } from 'react';
import { notifyPaymentWebhook } from '@/api/payments';
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
import { useAccount, useWriteContract, useWaitForTransactionReceipt } from 'wagmi';
import { parseUnits, formatUnits } from 'viem';
import WalletConnector from './WalletConnector';

// PayvergePaymentsV2 ABI (key functions)
const PAYVERGE_ABI = [
  {
    inputs: [
      { name: "billId", type: "bytes32" },
      { name: "amount", type: "uint256" },
      { name: "tipAmount", type: "uint256" },
      { name: "businessAddress", type: "address" },
      { name: "tipAddress", type: "address" }
    ],
    name: "payBill",
    outputs: [],
    stateMutability: "nonpayable",
    type: "function"
  }
] as const;

// USDC ABI (for approval)
const USDC_ABI = [
  {
    inputs: [
      { name: "spender", type: "address" },
      { name: "amount", type: "uint256" }
    ],
    name: "approve",
    outputs: [{ name: "", type: "bool" }],
    stateMutability: "nonpayable",
    type: "function"
  },
  {
    inputs: [
      { name: "owner", type: "address" },
      { name: "spender", type: "address" }
    ],
    name: "allowance",
    outputs: [{ name: "", type: "uint256" }],
    stateMutability: "view",
    type: "function"
  }
] as const;

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
  const [paymentStep, setPaymentStep] = useState<PaymentStep>('amount');
  const [tipAmount, setTipAmount] = useState<string>('0');
  const [error, setError] = useState<string>('');
  const [transactionHash, setTransactionHash] = useState<string>('');

  const { isConnected, address } = useAccount();

  // Contract addresses (these should come from environment variables)
  const USDC_ADDRESS = process.env.NEXT_PUBLIC_USDC_ADDRESS as `0x${string}`;
  const PAYVERGE_ADDRESS = process.env.NEXT_PUBLIC_PAYVERGE_ADDRESS as `0x${string}`;

  const { writeContract: writeUSDC, data: approvalHash } = useWriteContract();
  const { writeContract: writePayverge, data: paymentHash } = useWriteContract();

  const { isLoading: isApprovalConfirming } = useWaitForTransactionReceipt({
    hash: approvalHash,
  });

  const { isLoading: isPaymentConfirming, isSuccess: isPaymentSuccess } = useWaitForTransactionReceipt({
    hash: paymentHash,
  });

  const formatCurrency = (value: number) => `$${value.toFixed(2)}`;

  const tipPresets = [0, 0.15, 0.18, 0.20, 0.25];
  const tipValue = parseFloat(tipAmount) || 0;
  const totalAmount = amount + tipValue;

  // Convert USD to USDC (6 decimals)
  const amountUSDC = parseUnits(amount.toString(), 6);
  const tipAmountUSDC = parseUnits(tipValue.toString(), 6);
  const totalAmountUSDC = amountUSDC + tipAmountUSDC;

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
      // Approve USDC spending
      await writeUSDC({
        address: USDC_ADDRESS,
        abi: USDC_ABI,
        functionName: 'approve',
        args: [PAYVERGE_ADDRESS, totalAmountUSDC],
      });
    } catch (err) {
      setError('Failed to approve USDC spending');
      setPaymentStep('error');
    }
  };

  const handlePayment = React.useCallback(async () => {
    setPaymentStep('payment');
    setError('');

    try {
      // Convert bill ID to bytes32
      const billIdBytes32 = `0x${billId.toString(16).padStart(64, '0')}` as `0x${string}`;

      await writePayverge({
        address: PAYVERGE_ADDRESS,
        abi: PAYVERGE_ABI,
        functionName: 'payBill',
        args: [
          billIdBytes32,
          amountUSDC,
          tipAmountUSDC,
          businessAddress as `0x${string}`,
          tipAddress as `0x${string}`
        ],
      });

      setTransactionHash(paymentHash || '');
      setPaymentStep('confirming');
    } catch (err) {
      setError('Payment transaction failed');
      setPaymentStep('error');
    }
  }, [writePayverge, billId, businessAddress, tipAddress, PAYVERGE_ADDRESS, amountUSDC, tipAmountUSDC, paymentHash]);

  const notifyBackend = React.useCallback(async () => {
    try {
      await notifyPaymentWebhook({
        billId: parseInt(billId.toString()),
        transactionHash: paymentHash || '',
        amount: Math.round(Number(amount) * 100), // Convert to cents
        tipAmount: Math.round(Number(tipValue) * 100), // Convert to cents
        payerAddress: address || '',
        timestamp: new Date().toISOString(),
      });
    } catch (err) {
      console.error('Failed to notify backend:', err);
    }
  }, [paymentHash, billId, amount, tipValue, address]);

  // Watch for transaction confirmations
  React.useEffect(() => {
    if (transactionHash && paymentStep !== 'success') {
      handlePayment()
    }
  }, [transactionHash, paymentStep, handlePayment]);

  React.useEffect(() => {
    if (isPaymentSuccess) {
      setPaymentStep('success');
      // Notify backend about the payment
      notifyBackend();
    }
  }, [isPaymentSuccess, notifyBackend]);


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

          <WalletConnector showBalance={true} showDisconnect={false} />
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
