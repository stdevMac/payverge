'use client';

import React, { useState, useEffect } from 'react';
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
  Spinner,
} from '@nextui-org/react';
import { Wallet, CreditCard, AlertCircle, CheckCircle2, ExternalLink } from 'lucide-react';
import { useAccount, useConnect, useDisconnect, useWriteContract, useWaitForTransactionReceipt } from 'wagmi';
import { parseUnits } from 'viem';

interface PaymentModalProps {
  isOpen: boolean;
  onClose: () => void;
  billId: number;
  amount: number;
  businessName: string;
  onPaymentComplete?: () => void;
  onPaymentSuccess?: (hash: string) => void;
}

interface PaymentBreakdown {
  subtotal: number;
  tip: number;
  total: number;
}

const USDC_CONTRACT_ADDRESS = '0xA0b86a33E6441b8435b662f0E2d0E2E8b0E8E8E8'; // Replace with actual USDC address
const PAYVERGE_CONTRACT_ADDRESS = '0xB0b86a33E6441b8435b662f0E2d0E2E8b0E8E8E8'; // Replace with deployed contract address

const PAYVERGE_ABI = [
  {
    inputs: [
      { name: 'billId', type: 'bytes32' },
      { name: 'amount', type: 'uint256' },
      { name: 'tipAmount', type: 'uint256' },
      { name: 'businessAddress', type: 'address' },
      { name: 'tipAddress', type: 'address' },
    ],
    name: 'payBill',
    outputs: [],
    stateMutability: 'nonpayable',
    type: 'function',
  },
] as const;

const USDC_ABI = [
  {
    inputs: [
      { name: 'spender', type: 'address' },
      { name: 'amount', type: 'uint256' },
    ],
    name: 'approve',
    outputs: [{ name: '', type: 'bool' }],
    stateMutability: 'nonpayable',
    type: 'function',
  },
  {
    inputs: [
      { name: 'owner', type: 'address' },
      { name: 'spender', type: 'address' },
    ],
    name: 'allowance',
    outputs: [{ name: '', type: 'uint256' }],
    stateMutability: 'view',
    type: 'function',
  },
] as const;

export default function PaymentModal({ isOpen, onClose, billId, amount, businessName, onPaymentComplete, onPaymentSuccess }: PaymentModalProps) {
  const [paymentStep, setPaymentStep] = useState<'amount' | 'wallet' | 'approve' | 'payment' | 'success'>('amount');
  const [paymentAmount, setPaymentAmount] = useState<string>('');
  const [tipAmount, setTipAmount] = useState<string>('0');
  const [paymentBreakdown, setPaymentBreakdown] = useState<PaymentBreakdown>({ subtotal: 0, tip: 0, total: 0 });
  const [error, setError] = useState<string | null>(null);
  const [isProcessing, setIsProcessing] = useState(false);

  const { address, isConnected } = useAccount();
  const { connect, connectors } = useConnect();
  const { disconnect } = useDisconnect();

  const { writeContract: approveUSDC, data: approveHash, isPending: isApproving } = useWriteContract();
  const { writeContract: payBill, data: paymentHash, isPending: isPaymentPending } = useWriteContract();

  const { isLoading: isApproveConfirming, isSuccess: isApproveSuccess } = useWaitForTransactionReceipt({
    hash: approveHash,
  });

  const { isLoading: isPaymentConfirming, isSuccess: isPaymentSuccess } = useWaitForTransactionReceipt({
    hash: paymentHash,
  });

  const remainingAmount = amount;

  useEffect(() => {
    if (isPaymentSuccess && paymentHash) {
      setPaymentStep('success');
      onPaymentSuccess?.(paymentHash);
    }
  }, [isPaymentSuccess, paymentHash, onPaymentSuccess]);

  useEffect(() => {
    if (isApproveSuccess) {
      setPaymentStep('payment');
    }
  }, [isApproveSuccess]);

  useEffect(() => {
    const amount = parseFloat(paymentAmount) || 0;
    const tip = parseFloat(tipAmount) || 0;
    setPaymentBreakdown({
      subtotal: amount,
      tip: tip,
      total: amount + tip,
    });
  }, [paymentAmount, tipAmount]);

  const handleAmountNext = () => {
    const amount = parseFloat(paymentAmount);
    if (!amount || amount <= 0) {
      setError('Please enter a valid payment amount');
      return;
    }
    if (amount > remainingAmount / 100) {
      setError('Payment amount cannot exceed remaining balance');
      return;
    }
    setError(null);
    
    if (isConnected) {
      setPaymentStep('approve');
    } else {
      setPaymentStep('wallet');
    }
  };

  const handleWalletConnect = async (connector: any) => {
    try {
      setError(null);
      await connect({ connector });
      setPaymentStep('approve');
    } catch (err: any) {
      setError(err.message || 'Failed to connect wallet');
    }
  };

  const handleApproveUSDC = async () => {
    if (!address) return;

    try {
      setError(null);
      setIsProcessing(true);

      const totalAmountWei = parseUnits(paymentBreakdown.total.toString(), 6);

      await approveUSDC({
        address: USDC_CONTRACT_ADDRESS,
        abi: USDC_ABI,
        functionName: 'approve',
        args: [PAYVERGE_CONTRACT_ADDRESS, totalAmountWei],
      });
    } catch (err: any) {
      setError(err.message || 'Failed to approve USDC');
      setIsProcessing(false);
    }
  };

  const handlePayment = async () => {
    if (!address) return;

    try {
      setError(null);
      setIsProcessing(true);

      const billIdBytes = `0x${billId.toString(16).padStart(64, '0')}`;
      const amountWei = parseUnits(paymentBreakdown.subtotal.toString(), 6);
      const tipAmountWei = parseUnits(paymentBreakdown.tip.toString(), 6);

      await payBill({
        address: PAYVERGE_CONTRACT_ADDRESS,
        abi: PAYVERGE_ABI,
        functionName: 'payBill',
        args: [
          billIdBytes as `0x${string}`,
          amountWei,
          tipAmountWei,
          '0x0000000000000000000000000000000000000000' as `0x${string}`, // businessAddress - replace with actual
          '0x0000000000000000000000000000000000000000' as `0x${string}`, // tippingAddress - replace with actual
        ],
      });
    } catch (err: any) {
      setError(err.message || 'Payment failed');
      setIsProcessing(false);
    }
  };

  const resetModal = () => {
    setPaymentStep('amount');
    setPaymentAmount('');
    setTipAmount('0');
    setError(null);
    setIsProcessing(false);
  };

  const handleClose = () => {
    resetModal();
    onClose();
  };

  const renderTipPresets = () => {
    const subtotal = parseFloat(paymentAmount) || 0;
    const presets = [15, 18, 20, 25];
    
    return (
      <div className="flex gap-2 flex-wrap">
        {presets.map((percentage) => {
          const tipValue = (subtotal * percentage / 100).toFixed(2);
          return (
            <Button
              key={percentage}
              size="sm"
              variant="bordered"
              onPress={() => setTipAmount(tipValue)}
            >
              {percentage}% (${tipValue})
            </Button>
          );
        })}
        <Button
          size="sm"
          variant="bordered"
          onPress={() => setTipAmount('0')}
        >
          No Tip
        </Button>
      </div>
    );
  };

  const renderAmountStep = () => (
    <div className="space-y-6">
      <Card>
        <CardBody>
          <div className="space-y-4">
            <div className="flex justify-between">
              <span className="text-gray-600">Bill Total:</span>
              <span className="font-semibold">${(amount / 100).toFixed(2)}</span>
            </div>
            <div className="flex justify-between">
              <span className="text-gray-600">Paid:</span>
              <span className="text-success">$0.00</span>
            </div>
            <Divider />
            <div className="flex justify-between">
              <span className="font-semibold">Remaining:</span>
              <span className="font-semibold text-primary">${(remainingAmount / 100).toFixed(2)}</span>
            </div>
          </div>
        </CardBody>
      </Card>

      <div className="space-y-4">
        <Input
          label="Payment Amount"
          placeholder="0.00"
          value={paymentAmount}
          onChange={(e) => setPaymentAmount(e.target.value)}
          startContent={<span className="text-gray-500">$</span>}
          type="number"
          step="0.01"
          min="0"
          max={(remainingAmount / 100).toString()}
        />

        <div className="space-y-2">
          <label className="text-sm font-medium">Tip Amount</label>
          {renderTipPresets()}
          <Input
            placeholder="0.00"
            value={tipAmount}
            onChange={(e) => setTipAmount(e.target.value)}
            startContent={<span className="text-gray-500">$</span>}
            type="number"
            step="0.01"
            min="0"
          />
        </div>

        {paymentBreakdown.total > 0 && (
          <Card className="bg-gray-50">
            <CardBody>
              <div className="space-y-2">
                <div className="flex justify-between">
                  <span>Payment:</span>
                  <span>${paymentBreakdown.subtotal.toFixed(2)}</span>
                </div>
                <div className="flex justify-between">
                  <span>Tip:</span>
                  <span>${paymentBreakdown.tip.toFixed(2)}</span>
                </div>
                <Divider />
                <div className="flex justify-between font-semibold">
                  <span>Total:</span>
                  <span>${paymentBreakdown.total.toFixed(2)}</span>
                </div>
              </div>
            </CardBody>
          </Card>
        )}
      </div>
    </div>
  );

  const renderWalletStep = () => (
    <div className="space-y-4">
      <div className="text-center">
        <Wallet className="mx-auto mb-4 text-primary" size={48} />
        <h3 className="text-lg font-semibold mb-2">Connect Your Wallet</h3>
        <p className="text-gray-600">Choose a wallet to connect and pay with USDC</p>
      </div>

      <div className="space-y-3">
        {connectors.map((connector) => (
          <Button
            key={connector.id}
            variant="bordered"
            className="w-full justify-start"
            onPress={() => handleWalletConnect(connector)}
          >
            <Wallet size={20} />
            {connector.name}
          </Button>
        ))}
      </div>
    </div>
  );

  const renderApproveStep = () => (
    <div className="space-y-4">
      <div className="text-center">
        <CreditCard className="mx-auto mb-4 text-primary" size={48} />
        <h3 className="text-lg font-semibold mb-2">Approve USDC</h3>
        <p className="text-gray-600">
          Approve the Payverge contract to spend ${paymentBreakdown.total.toFixed(2)} USDC
        </p>
      </div>

      <Card className="bg-blue-50">
        <CardBody>
          <div className="space-y-2">
            <div className="flex justify-between">
              <span>Payment:</span>
              <span>${paymentBreakdown.subtotal.toFixed(2)} USDC</span>
            </div>
            <div className="flex justify-between">
              <span>Tip:</span>
              <span>${paymentBreakdown.tip.toFixed(2)} USDC</span>
            </div>
            <Divider />
            <div className="flex justify-between font-semibold">
              <span>Total to Approve:</span>
              <span>${paymentBreakdown.total.toFixed(2)} USDC</span>
            </div>
          </div>
        </CardBody>
      </Card>

      <Button
        color="primary"
        className="w-full"
        onPress={handleApproveUSDC}
        isLoading={isApproving || isApproveConfirming}
        isDisabled={isProcessing}
      >
        {isApproving || isApproveConfirming ? 'Approving...' : 'Approve USDC'}
      </Button>
    </div>
  );

  const renderPaymentStep = () => (
    <div className="space-y-4">
      <div className="text-center">
        <CheckCircle2 className="mx-auto mb-4 text-success" size={48} />
        <h3 className="text-lg font-semibold mb-2">Ready to Pay</h3>
        <p className="text-gray-600">USDC approved. Click to complete your payment.</p>
      </div>

      <Card className="bg-green-50">
        <CardBody>
          <div className="space-y-2">
            <div className="flex justify-between">
              <span>Payment:</span>
              <span>${paymentBreakdown.subtotal.toFixed(2)} USDC</span>
            </div>
            <div className="flex justify-between">
              <span>Tip:</span>
              <span>${paymentBreakdown.tip.toFixed(2)} USDC</span>
            </div>
            <Divider />
            <div className="flex justify-between font-semibold">
              <span>Total:</span>
              <span>${paymentBreakdown.total.toFixed(2)} USDC</span>
            </div>
          </div>
        </CardBody>
      </Card>

      <Button
        color="success"
        className="w-full"
        onPress={handlePayment}
        isLoading={isPaymentPending || isPaymentConfirming}
        isDisabled={isProcessing}
      >
        {isPaymentPending || isPaymentConfirming ? 'Processing Payment...' : 'Pay Now'}
      </Button>
    </div>
  );

  const renderSuccessStep = () => (
    <div className="space-y-4 text-center">
      <CheckCircle2 className="mx-auto text-success" size={64} />
      <h3 className="text-xl font-semibold text-success">Payment Successful!</h3>
      <p className="text-gray-600">Your payment has been processed successfully.</p>

      <Card className="bg-success-50">
        <CardBody>
          <div className="space-y-2">
            <div className="flex justify-between">
              <span>Amount Paid:</span>
              <span className="font-semibold">${paymentBreakdown.subtotal.toFixed(2)}</span>
            </div>
            <div className="flex justify-between">
              <span>Tip:</span>
              <span className="font-semibold">${paymentBreakdown.tip.toFixed(2)}</span>
            </div>
            <Divider />
            <div className="flex justify-between font-bold">
              <span>Total:</span>
              <span>${paymentBreakdown.total.toFixed(2)}</span>
            </div>
          </div>
        </CardBody>
      </Card>

      {paymentHash && (
        <Button
          variant="bordered"
          startContent={<ExternalLink size={16} />}
          onPress={() => window.open(`https://etherscan.io/tx/${paymentHash}`, '_blank')}
        >
          View Transaction
        </Button>
      )}
    </div>
  );

  const getStepTitle = () => {
    switch (paymentStep) {
      case 'amount': return 'Payment Amount';
      case 'wallet': return 'Connect Wallet';
      case 'approve': return 'Approve USDC';
      case 'payment': return 'Complete Payment';
      case 'success': return 'Payment Complete';
      default: return 'Payment';
    }
  };

  const canGoBack = paymentStep !== 'amount' && paymentStep !== 'success';
  const canProceed = paymentStep === 'amount' && paymentAmount && parseFloat(paymentAmount) > 0;

  return (
    <Modal
      isOpen={isOpen}
      onClose={handleClose}
      size="md"
      scrollBehavior="inside"
      isDismissable={paymentStep !== 'approve' && paymentStep !== 'payment'}
      hideCloseButton={paymentStep === 'approve' || paymentStep === 'payment'}
    >
      <ModalContent>
        <ModalHeader className="flex flex-col gap-1">
          <h2 className="text-xl font-semibold">{getStepTitle()}</h2>
          {paymentStep !== 'success' && (
            <div className="flex gap-2">
              {['amount', 'wallet', 'approve', 'payment'].map((step, index) => (
                <div
                  key={step}
                  className={`h-2 flex-1 rounded-full ${
                    ['amount', 'wallet', 'approve', 'payment'].indexOf(paymentStep) >= index
                      ? 'bg-primary'
                      : 'bg-gray-200'
                  }`}
                />
              ))}
            </div>
          )}
        </ModalHeader>

        <ModalBody>
          {error && (
            <Card className="border-danger-200 bg-danger-50">
              <CardBody>
                <div className="flex items-center gap-2 text-danger-700">
                  <AlertCircle size={16} />
                  <span className="text-sm">{error}</span>
                </div>
              </CardBody>
            </Card>
          )}

          {paymentStep === 'amount' && renderAmountStep()}
          {paymentStep === 'wallet' && renderWalletStep()}
          {paymentStep === 'approve' && renderApproveStep()}
          {paymentStep === 'payment' && renderPaymentStep()}
          {paymentStep === 'success' && renderSuccessStep()}
        </ModalBody>

        <ModalFooter>
          {canGoBack && (
            <Button
              variant="bordered"
              onPress={() => {
                if (paymentStep === 'wallet') setPaymentStep('amount');
                else if (paymentStep === 'approve') setPaymentStep(isConnected ? 'amount' : 'wallet');
                else if (paymentStep === 'payment') setPaymentStep('approve');
              }}
              isDisabled={isProcessing}
            >
              Back
            </Button>
          )}

          {paymentStep === 'amount' && (
            <Button
              color="primary"
              onPress={handleAmountNext}
              isDisabled={!canProceed}
            >
              Continue
            </Button>
          )}

          {paymentStep === 'success' && (
            <Button color="primary" onPress={handleClose}>
              Close
            </Button>
          )}

          {paymentStep !== 'amount' && paymentStep !== 'success' && paymentStep !== 'approve' && paymentStep !== 'payment' && (
            <Button variant="bordered" onPress={handleClose}>
              Cancel
            </Button>
          )}
        </ModalFooter>
      </ModalContent>
    </Modal>
  );
}
