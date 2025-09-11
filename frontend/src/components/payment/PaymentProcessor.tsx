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

  const handlePayment = async () => {
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
  };

  // Watch for transaction confirmations
  React.useEffect(() => {
    if (approvalHash && !isApprovalConfirming) {
      handlePayment();
    }
  }, [approvalHash, isApprovalConfirming]);

  React.useEffect(() => {
    if (isPaymentSuccess) {
      setPaymentStep('success');
      // Notify backend about the payment
      notifyBackend();
    }
  }, [isPaymentSuccess]);

  const notifyBackend = async () => {
    try {
      await fetch('/api/v1/payments/webhook', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          transaction_hash: paymentHash,
          bill_id: billId.toString(),
          amount: Math.round(amount * 100), // Convert to cents
          tip_amount: Math.round(tipValue * 100),
          status: 'confirmed'
        })
      });
    } catch (error) {
      console.error('Failed to notify backend:', error);
    }
  };

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
              <div className="text-xs text-default-500">
                <span>≈ {formatUnits(totalAmountUSDC, 6)} USDC</span>
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
          {isConnected ? 'Pay with USDC' : 'Connect Wallet'}
        </Button>
      </ModalFooter>
    </>
  );

  const renderWalletStep = () => (
    <>
      <ModalHeader>Connect Your Wallet</ModalHeader>
      <ModalBody>
        <WalletConnector showBalance={true} showDisconnect={false} />
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
              className="mt-4"
              color="primary"
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
