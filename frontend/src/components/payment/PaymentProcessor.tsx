'use client';

import React, { useState, useEffect, useCallback } from 'react';
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
  Spinner,
} from '@nextui-org/react';
import { Wallet, AlertCircle, CheckCircle2, ExternalLink, Clock } from 'lucide-react';
import { useAccount, useWaitForTransactionReceipt, usePublicClient } from 'wagmi';
import { parseUnits, formatUnits, Hash, pad, toHex } from 'viem';
import { 
  useProcessPayment, 
  useUsdcBalance, 
  useUsdcAllowance, 
  useApproveUsdc,
  useContractConfig 
} from '../../contracts/hooks';
import { updateBillPayment, createOnChainBill } from '../../api/bills';
import CurrencyConverter from '../common/CurrencyConverter';
import { useSimpleLocale, getTranslation } from '@/i18n/SimpleTranslationProvider';

interface PaymentProcessorProps {
  isOpen: boolean;
  onClose: () => void;
  billId: number;
  amount: number;
  businessName: string;
  businessAddress: string;
  tipAddress: string;
  defaultCurrency?: string;
  displayCurrency?: string;
  onPaymentComplete?: (paymentDetails: { totalPaid: number; tipAmount: number }) => void;
}

type PaymentStep = 
  | 'amount' 
  | 'wallet' 
  | 'approval' 
  | 'waiting-approval' 
  | 'creating-bill'
  | 'payment' 
  | 'waiting-payment'
  | 'confirming' 
  | 'success' 
  | 'error';

interface TransactionState {
  hash?: Hash;
  isConfirmed: boolean;
  error?: string;
}

export default function PaymentProcessor({
  isOpen,
  onClose,
  billId,
  amount,
  businessName,
  businessAddress,
  tipAddress,
  defaultCurrency = 'USD',
  displayCurrency = 'USD',
  onPaymentComplete
}: PaymentProcessorProps) {
  const { locale: currentLocale } = useSimpleLocale();
  
  // Translation helper
  const tString = (key: string): string => {
    const fullKey = `paymentProcessor.${key}`;
    const result = getTranslation(fullKey, currentLocale);
    return Array.isArray(result) ? result[0] || key : result as string;
  };
  
  const [paymentStep, setPaymentStep] = useState<PaymentStep>('amount');
  const [tipAmount, setTipAmount] = useState<string>('0');
  const [error, setError] = useState<string>('');
  const [approvalTx, setApprovalTx] = useState<TransactionState>({ isConfirmed: false });
  const [paymentTx, setPaymentTx] = useState<TransactionState>({ isConfirmed: false });
  const [billCreated, setBillCreated] = useState<boolean>(false);

  // Wagmi hooks for wallet connection and balances
  const { address, isConnected } = useAccount();
  const publicClient = usePublicClient();
  const contractConfig = useContractConfig();
  const { data: usdcBalance, refetch: refetchBalance } = useUsdcBalance(address);
  const { data: usdcAllowance, refetch: refetchAllowance } = useUsdcAllowance(address);
  
  // Contract hooks for payment processing
  const { processPayment } = useProcessPayment();
  const { approveUsdc } = useApproveUsdc();

  // Wait for transaction confirmations
  const { data: approvalReceipt, isSuccess: approvalSuccess, error: approvalError } = 
    useWaitForTransactionReceipt({
      hash: approvalTx.hash,
      query: {
        enabled: !!approvalTx.hash && !approvalTx.isConfirmed,
      },
    });

  const { data: paymentReceipt, isSuccess: paymentSuccess, error: paymentError, isLoading: paymentLoading } = 
    useWaitForTransactionReceipt({
      hash: paymentTx.hash,
      query: {
        enabled: !!paymentTx.hash && !paymentTx.isConfirmed,
      },
    });


  // Calculate amounts in Wei (USDC has 6 decimals)
  const amountInWei = parseUnits(amount.toString(), 6);
  const tipAmountInWei = parseUnits(tipAmount || '0', 6);
  const totalAmountInWei = amountInWei + tipAmountInWei;
  
  // Check if user has sufficient balance and allowance
  const hasSufficientBalance = usdcBalance ? usdcBalance >= totalAmountInWei : false;
  const hasSufficientAllowance = usdcAllowance ? usdcAllowance >= totalAmountInWei : false;

  const formatCurrency = (value: number) => `$${value.toFixed(2)}`;

  const tipPresets = [0, 0.15, 0.18, 0.20, 0.25];
  const tipValue = parseFloat(tipAmount) || 0;
  const totalAmount = amount + tipValue;

  const handleTipPreset = (percentage: number) => {
    const tip = amount * percentage;
    setTipAmount(tip.toFixed(2));
  };

  // Helper function to check if a preset is selected
  const isPresetSelected = (preset: number) => {
    const expectedTip = amount * preset;
    const currentTip = parseFloat(tipAmount) || 0;
    return Math.abs(currentTip - expectedTip) < 0.01; // Allow for small floating point differences
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
      if (!hasSufficientBalance) {
        throw new Error('Insufficient USDC balance');
      }

      if (!hasSufficientAllowance) {
        const txHash = await approveUsdc(totalAmountInWei);
        setApprovalTx({ hash: txHash, isConfirmed: false });
        setPaymentStep('waiting-approval');
      } else {
        // Already has sufficient allowance, proceed to bill creation
        await handleBillCreation();
      }
    } catch (err: any) {
      console.error('Approval failed:', err);
      setError(err.message || 'Failed to approve USDC spending');
      setPaymentStep('error');
    }
  };

  const handleBillCreation = async () => {
    setPaymentStep('creating-bill');
    setError('');

    try {
      const result = await createOnChainBill(billId, {
        business_address: businessAddress,
        total_amount: totalAmount
      });
      
      setBillCreated(true);
      await handlePayment();
    } catch (err: any) {
      // Continue anyway - bill might already exist
      await handlePayment();
    }
  };

  const handlePayment = async () => {
    setPaymentStep('payment');
    setError('');

    try {
      // Validate contract configuration
      if (!contractConfig.payments || contractConfig.payments === '0x0000000000000000000000000000000000000000') {
        throw new Error('Contract address not configured. Please check environment variables.');
      }

      // Generate proper bytes32 bill ID to match backend format
      // Backend uses: common.BytesToHash(big.NewInt(int64(billIDInt)).Bytes())
      // This is equivalent to padding the bill ID as a number to 32 bytes
      const onChainBillId = pad(toHex(BigInt(billId)), { size: 32 });
      const amountInWei = parseUnits(amount.toString(), 6);
      const tipAmountInWei = parseUnits(tipAmount || '0', 6);

      const txHash = await processPayment({
        billId: onChainBillId,
        amount: amountInWei,
        tipAmount: tipAmountInWei,
      });
      
      if (!txHash || txHash.length !== 66) {
        throw new Error(`Invalid transaction hash received: ${txHash}`);
      }
      
      setPaymentTx({ hash: txHash, isConfirmed: false });
      setPaymentStep('waiting-payment');
    } catch (err: any) {
      console.error('Payment failed:', err);
      setError(err.message || 'Payment transaction failed');
      setPaymentStep('error');
    }
  };
  const notifyBackend = async (txHash: Hash) => {
    try {
      const result = await updateBillPayment(billId, {
        transaction_hash: txHash,
        amount_paid: totalAmount,
        tip_amount: parseFloat(tipAmount || '0'),
        payment_method: 'crypto',
        blockchain_network: 'base-sepolia' // or get from chain config
      });
      return result;
    } catch (error) {
      console.error('Failed to notify backend:', error);
      throw error;
    }
  };

  // Effect to handle approval confirmation
  useEffect(() => {
    if (approvalSuccess && approvalReceipt && !approvalTx.isConfirmed) {
      setApprovalTx(prev => ({ ...prev, isConfirmed: true }));
      
      // Refetch allowance to get updated value
      refetchAllowance();
      
      // Proceed to bill creation
      handleBillCreation();
    }
  }, [approvalSuccess, approvalReceipt, approvalTx.isConfirmed]);

  // Effect to handle approval errors
  useEffect(() => {
    if (approvalError && approvalTx.hash) {
      console.error('Approval transaction failed:', approvalError);
      setError('Approval transaction failed');
      setPaymentStep('error');
    }
  }, [approvalError, approvalTx.hash]);

  // Effect to handle payment confirmation
  useEffect(() => {
    if (paymentSuccess && paymentReceipt && !paymentTx.isConfirmed) {
      setPaymentTx(prev => ({ ...prev, isConfirmed: true }));
      setPaymentStep('confirming');
      
      // Notify backend and complete payment
      notifyBackend(paymentReceipt.transactionHash)
        .then(() => {
          setPaymentStep('success');
          if (onPaymentComplete) {
            onPaymentComplete({
              totalPaid: totalAmount,
              tipAmount: parseFloat(tipAmount || '0')
            });
          }
        })
        .catch((error) => {
          console.error('Backend notification failed:', error);
          // Still show success since blockchain transaction succeeded
          setPaymentStep('success');
          if (onPaymentComplete) {
            onPaymentComplete({
              totalPaid: totalAmount,
              tipAmount: parseFloat(tipAmount || '0')
            });
          }
        });
    }
  }, [paymentSuccess, paymentReceipt, paymentTx.isConfirmed]);

  // Effect to handle payment errors
  useEffect(() => {
    if (paymentError && paymentTx.hash) {
      console.error('Payment transaction failed:', paymentError);
      setError(`Payment transaction failed: ${paymentError.message || 'Unknown error'}`);
      setPaymentStep('error');
    }
  }, [paymentError, paymentTx.hash]);

  const handleClose = useCallback(() => {
    if (['approval', 'waiting-approval', 'creating-bill', 'payment', 'waiting-payment', 'confirming'].includes(paymentStep)) {
      return; // Prevent closing during critical steps
    }
    
    // Reset all state
    setPaymentStep('amount');
    setTipAmount('0');
    setError('');
    setApprovalTx({ isConfirmed: false });
    setPaymentTx({ isConfirmed: false });
    setBillCreated(false);
    onClose();
  }, [paymentStep, onClose]);

  const handleRetry = useCallback(() => {
    setError('');
    setPaymentStep('amount');
    setApprovalTx({ isConfirmed: false });
    setPaymentTx({ isConfirmed: false });
    setBillCreated(false);
  }, []);

  const handleSuccess = () => {
    onPaymentComplete?.({
      totalPaid: totalAmount,
      tipAmount: parseFloat(tipAmount || '0')
    });
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
            <h2 className="text-xl font-semibold text-gray-900">{tString('paymentDetails')}</h2>
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
                  <span className="text-gray-600 font-medium">{tString('billAmount')}</span>
                  <span className="text-xl font-bold text-gray-900">{formatCurrency(amount)}</span>
                </div>

                <>
                  <Divider className="bg-gray-200" />
                  <div className="flex justify-between items-center">
                    <span className="text-gray-600 font-medium">{tString('tip')}</span>
                    <span className="text-lg font-semibold text-green-600">{formatCurrency(tipValue)}</span>
                  </div>
                </>

                <Divider className="bg-gray-300" />

                <div className="flex justify-between items-center p-3 bg-white rounded-lg shadow-sm">
                  <span className="text-lg font-bold text-gray-900">{tString('total')}</span>
                  <span className="text-2xl font-bold text-primary-600">{formatCurrency(totalAmount)}</span>
                </div>
              </div>
            </CardBody>
          </Card>

          {/* Tip Selection */}
          <div className="space-y-4">
            <div className="flex items-center gap-2">
              <h4 className="text-lg font-semibold text-gray-900">{tString('addTip')}</h4>
              <Chip size="sm" variant="flat" color="success" className="text-xs">
                {tString('optional')}
              </Chip>
            </div>

            <div className="grid grid-cols-3 gap-3">
              {tipPresets.map((preset) => (
                <Button
                  key={preset}
                  size="lg"
                  variant={isPresetSelected(preset) ? "solid" : "bordered"}
                  color={isPresetSelected(preset) ? "primary" : "default"}
                  onPress={() => handleTipPreset(preset)}
                  className={`h-14 font-semibold transition-all duration-200 ${isPresetSelected(preset)
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
              <h5 className="text-sm font-medium text-gray-700 mb-3">{tString('customTipPrompt')}</h5>
              <Input
                type="number"
                label={tString('customTipAmount')}
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
{tString('continueToPayment')}
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
            <h2 className="text-xl font-semibold text-gray-900">{tString('connectWallet')}</h2>
            <p className="text-sm text-gray-500">{tString('walletInstructions')}</p>
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
              <h3 className="text-lg font-semibold text-gray-900 mb-2">{tString('securePayment')}</h3>
              <p className="text-gray-600 text-sm leading-relaxed">
                {tString('securePaymentDescription')}
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
          <span>{tString('paymentSuccessful')}</span>
        </div>
      </ModalHeader>
      <ModalBody>
        <div className="text-center space-y-4">
          <CheckCircle2 size={48} className="mx-auto text-success" />
          <div>
            <h3 className="font-semibold mb-2">{tString('paymentComplete')}</h3>
            <p className="text-sm text-gray-600">
{tString('paymentSuccessMessage').replace('{{amount}}', formatCurrency(totalAmount))}
            </p>
          </div>
          <Card className="bg-success-50">
            <CardBody>
              <div className="space-y-2 text-sm">
                <div className="flex justify-between">
                  <span>{tString('billId')}:</span>
                  <span>#{billId}</span>
                </div>
                <div className="flex justify-between">
                  <span>{tString('amountPaid')}:</span>
                  <span>{formatCurrency(totalAmount)}</span>
                </div>
                <div className="flex justify-between">
                  <span>{tString('status')}:</span>
                  <Chip size="sm" color="success">{tString('confirmed')}</Chip>
                </div>
                {paymentTx.hash && (
                  <div className="flex justify-between items-center">
                    <span>{tString('transaction')}:</span>
                    <Button
                      size="sm"
                      variant="light"
                      endContent={<ExternalLink size={12} />}
                      onPress={() => window.open(`https://basescan.org/tx/${paymentTx.hash}`, '_blank')}
                    >
{tString('view')}
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
          <span>{tString('paymentFailed')}</span>
        </div>
      </ModalHeader>
      <ModalBody>
        <div className="text-center space-y-4">
          <AlertCircle size={48} className="mx-auto text-danger" />
          <div>
            <h3 className="font-semibold mb-2">{tString('paymentError')}</h3>
            <p className="text-sm text-gray-600">{error}</p>
          </div>
        </div>
      </ModalBody>
      <ModalFooter>
        <Button variant="light" onPress={handleClose}>
          {tString('cancel')}
        </Button>
        <Button color="primary" onPress={() => setPaymentStep('amount')}>
          {tString('tryAgain')}
        </Button>
      </ModalFooter>
    </>
  );

  return (
    <Modal
      isOpen={isOpen}
      onClose={handleClose}
      size="md"
      closeButton={!['approval', 'waiting-approval', 'creating-bill', 'payment', 'waiting-payment', 'confirming'].includes(paymentStep)}
      isDismissable={!['approval', 'waiting-approval', 'creating-bill', 'payment', 'waiting-payment', 'confirming'].includes(paymentStep)}
    >
      <ModalContent>
        {paymentStep === 'amount' && renderAmountStep()}
        {paymentStep === 'wallet' && renderWalletStep()}
        {paymentStep === 'approval' && renderProcessingStep(tString('steps.approval.title'), tString('steps.approval.description'))}
        {paymentStep === 'waiting-approval' && renderProcessingStep(tString('steps.waitingApproval.title'), tString('steps.waitingApproval.description'))}
        {paymentStep === 'creating-bill' && renderProcessingStep(tString('steps.creatingBill.title'), tString('steps.creatingBill.description'))}
        {paymentStep === 'payment' && renderProcessingStep(tString('steps.payment.title'), tString('steps.payment.description'))}
        {paymentStep === 'waiting-payment' && renderProcessingStep(tString('steps.waitingPayment.title'), tString('steps.waitingPayment.description'))}
        {paymentStep === 'confirming' && renderProcessingStep(tString('steps.finalizing.title'), tString('steps.finalizing.description'))}
        {paymentStep === 'success' && renderSuccessStep()}
        {paymentStep === 'error' && renderErrorStep()}
      </ModalContent>
    </Modal>
  );
}
