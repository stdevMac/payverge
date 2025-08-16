'use client';

import { useState, useEffect } from 'react';
import { useParams } from 'next/navigation';
import { useQuery } from '@tanstack/react-query';
import { useAccount, useContractWrite, usePrepareContractWrite, useContractRead } from 'wagmi';
import { ConnectButton } from '@rainbow-me/rainbowkit';
import { toast } from 'react-hot-toast';
import { format } from 'date-fns';
import { 
  CreditCard, 
  Clock, 
  CheckCircle, 
  AlertCircle, 
  ExternalLink,
  Copy,
  QrCode
} from 'lucide-react';
import { invoiceAPI, Invoice, wsManager } from '../../../lib/api';
import { 
  INVOICE_CONTRACT_ABI, 
  USDC_CONTRACT_ABI, 
  getContractAddresses, 
  parseUSDC, 
  formatUSDC 
} from '../../../lib/contract';
import { Header } from '../../../components/Header';

export default function PaymentPage() {
  const params = useParams();
  const invoiceId = parseInt(params.id as string);
  const { address, isConnected } = useAccount();
  const [paymentAmount, setPaymentAmount] = useState('');
  const [isProcessing, setIsProcessing] = useState(false);

  const { data: invoice, isLoading, refetch } = useQuery({
    queryKey: ['invoice', invoiceId],
    queryFn: () => invoiceAPI.getById(invoiceId),
    enabled: !!invoiceId,
  });

  // Listen for real-time payment updates
  useEffect(() => {
    const handlePaymentReceived = (data: any) => {
      if (data.invoice_id === invoiceId) {
        refetch();
        toast.success('Payment received!');
      }
    };

    wsManager.on('payment_received', handlePaymentReceived);
    return () => wsManager.off('payment_received', handlePaymentReceived);
  }, [invoiceId, refetch]);

  // Contract addresses
  const contractAddresses = getContractAddresses(1); // Default to mainnet

  // Check USDC allowance
  const { data: allowance } = useContractRead({
    address: contractAddresses.USDC as `0x${string}`,
    abi: USDC_CONTRACT_ABI,
    functionName: 'allowance',
    args: address && [address, contractAddresses.INVOICE_GENERATOR as `0x${string}`],
    enabled: !!address,
  });

  // Check USDC balance
  const { data: balance } = useContractRead({
    address: contractAddresses.USDC as `0x${string}`,
    abi: USDC_CONTRACT_ABI,
    functionName: 'balanceOf',
    args: address && [address],
    enabled: !!address,
  });

  // Prepare USDC approval
  const { config: approveConfig } = usePrepareContractWrite({
    address: contractAddresses.USDC as `0x${string}`,
    abi: USDC_CONTRACT_ABI,
    functionName: 'approve',
    args: [
      contractAddresses.INVOICE_GENERATOR as `0x${string}`,
      paymentAmount ? parseUSDC(paymentAmount) : BigInt(0)
    ],
    enabled: !!paymentAmount && parseFloat(paymentAmount) > 0,
  });

  const { write: approveUSDC } = useContractWrite(approveConfig);

  // Prepare payment transaction
  const { config: payConfig } = usePrepareContractWrite({
    address: contractAddresses.INVOICE_GENERATOR as `0x${string}`,
    abi: INVOICE_CONTRACT_ABI,
    functionName: 'payInvoice',
    args: [
      BigInt(invoiceId),
      paymentAmount ? parseUSDC(paymentAmount) : BigInt(0)
    ],
    enabled: !!paymentAmount && parseFloat(paymentAmount) > 0,
  });

  const { write: payInvoice } = useContractWrite(payConfig);

  const handlePayment = async () => {
    if (!address || !invoice) {
      toast.error('Please connect your wallet');
      return;
    }

    if (!paymentAmount || parseFloat(paymentAmount) <= 0) {
      toast.error('Please enter a valid payment amount');
      return;
    }

    const amount = parseUSDC(paymentAmount);
    
    // Check if user has enough balance
    if (balance && amount > balance) {
      toast.error('Insufficient USDC balance');
      return;
    }

    setIsProcessing(true);

    try {
      // Check if we need approval
      if (!allowance || amount > allowance) {
        toast.loading('Approving USDC spending...');
        await approveUSDC?.();
        // Wait for approval confirmation
        await new Promise(resolve => setTimeout(resolve, 3000));
      }

      // Make payment
      toast.loading('Processing payment...');
      await payInvoice?.();
      
      toast.success('Payment submitted! Waiting for confirmation...');
      
    } catch (error: any) {
      console.error('Payment error:', error);
      toast.error(error.message || 'Payment failed');
    } finally {
      setIsProcessing(false);
    }
  };

  const copyToClipboard = (text: string) => {
    navigator.clipboard.writeText(text);
    toast.success('Copied to clipboard!');
  };

  if (isLoading) {
    return (
      <div className="min-h-screen bg-gray-50">
        <Header />
        <div className="container mx-auto px-4 py-16">
          <div className="text-center">
            <div className="animate-spin rounded-full h-12 w-12 border-b-2 border-primary-600 mx-auto"></div>
            <p className="mt-4 text-gray-600">Loading invoice...</p>
          </div>
        </div>
      </div>
    );
  }

  if (!invoice) {
    return (
      <div className="min-h-screen bg-gray-50">
        <Header />
        <div className="container mx-auto px-4 py-16">
          <div className="text-center">
            <AlertCircle className="h-12 w-12 text-red-500 mx-auto mb-4" />
            <h1 className="text-2xl font-bold text-gray-900 mb-2">
              Invoice Not Found
            </h1>
            <p className="text-gray-600">
              The invoice you're looking for doesn't exist or has been removed.
            </p>
          </div>
        </div>
      </div>
    );
  }

  const remainingAmount = (invoice.amount - invoice.amount_paid) / 1000000;
  const isFullyPaid = invoice.status === 'paid';

  return (
    <div className="min-h-screen bg-gray-50">
      <Header />
      
      <div className="container mx-auto px-4 py-8">
        <div className="max-w-2xl mx-auto">
          {/* Invoice Header */}
          <div className="card mb-8">
            <div className="text-center mb-6">
              <CreditCard className="h-12 w-12 text-primary-600 mx-auto mb-4" />
              <h1 className="text-3xl font-bold text-gray-900 mb-2">
                Invoice #{invoice.invoice_id}
              </h1>
              {invoice.creator_name && (
                <p className="text-lg text-gray-600">
                  From: {invoice.creator_name}
                </p>
              )}
            </div>

            <div className="border-t border-gray-200 pt-6">
              <div className="grid md:grid-cols-2 gap-6">
                <div>
                  <h3 className="text-lg font-semibold text-gray-900 mb-3">
                    {invoice.title}
                  </h3>
                  {invoice.description && (
                    <p className="text-gray-600 mb-4">{invoice.description}</p>
                  )}
                  
                  <div className="space-y-2 text-sm">
                    <div className="flex justify-between">
                      <span className="text-gray-600">Amount:</span>
                      <span className="font-medium">${invoice.amount_formatted} USDC</span>
                    </div>
                    <div className="flex justify-between">
                      <span className="text-gray-600">Paid:</span>
                      <span className="font-medium">${(invoice.amount_paid / 1000000).toFixed(2)} USDC</span>
                    </div>
                    <div className="flex justify-between border-t pt-2">
                      <span className="text-gray-600">Remaining:</span>
                      <span className="font-semibold text-lg">${remainingAmount.toFixed(2)} USDC</span>
                    </div>
                  </div>
                </div>

                <div className="space-y-4">
                  <div>
                    <span className="text-sm text-gray-600">Status:</span>
                    <div className="mt-1">
                      {isFullyPaid ? (
                        <span className="status-paid">Paid</span>
                      ) : invoice.amount_paid > 0 ? (
                        <span className="status-partially-paid">Partially Paid</span>
                      ) : (
                        <span className="status-pending">Pending</span>
                      )}
                    </div>
                  </div>

                  {invoice.due_date && (
                    <div>
                      <span className="text-sm text-gray-600">Due Date:</span>
                      <div className="mt-1 flex items-center space-x-2">
                        <Clock className="h-4 w-4 text-gray-400" />
                        <span className="text-sm">
                          {format(new Date(invoice.due_date), 'MMMM d, yyyy')}
                        </span>
                      </div>
                    </div>
                  )}

                  <div>
                    <span className="text-sm text-gray-600">Created:</span>
                    <div className="mt-1 text-sm">
                      {format(new Date(invoice.created_at), 'MMMM d, yyyy')}
                    </div>
                  </div>
                </div>
              </div>
            </div>
          </div>

          {/* Payment Section */}
          {!isFullyPaid ? (
            <div className="card">
              <h2 className="text-xl font-bold text-gray-900 mb-6">
                Make Payment
              </h2>

              {!isConnected ? (
                <div className="text-center py-8">
                  <p className="text-gray-600 mb-4">
                    Connect your wallet to pay this invoice
                  </p>
                  <ConnectButton />
                </div>
              ) : (
                <div className="space-y-6">
                  <div>
                    <label className="block text-sm font-medium text-gray-700 mb-2">
                      Payment Amount (USDC)
                    </label>
                    <div className="flex space-x-3">
                      <input
                        type="number"
                        step="0.01"
                        min="0.01"
                        max={remainingAmount}
                        value={paymentAmount}
                        onChange={(e) => setPaymentAmount(e.target.value)}
                        placeholder="0.00"
                        className="input flex-1"
                      />
                      <button
                        onClick={() => setPaymentAmount(remainingAmount.toString())}
                        className="btn-secondary whitespace-nowrap"
                      >
                        Pay Full Amount
                      </button>
                    </div>
                    <p className="mt-1 text-sm text-gray-500">
                      Maximum: ${remainingAmount.toFixed(2)} USDC
                    </p>
                  </div>

                  {balance && (
                    <div className="bg-blue-50 border border-blue-200 rounded-lg p-4">
                      <p className="text-sm text-blue-700">
                        Your USDC Balance: ${formatUSDC(balance)} USDC
                      </p>
                    </div>
                  )}

                  <button
                    onClick={handlePayment}
                    disabled={isProcessing || !paymentAmount || parseFloat(paymentAmount) <= 0}
                    className="btn-primary w-full py-3 text-base font-medium"
                  >
                    {isProcessing ? 'Processing...' : 'Pay with USDC'}
                  </button>

                  <div className="bg-yellow-50 border border-yellow-200 rounded-lg p-4">
                    <div className="flex items-start">
                      <AlertCircle className="h-5 w-5 text-yellow-600 mt-0.5 mr-3" />
                      <div className="text-sm text-yellow-700">
                        <p className="font-medium mb-1">Payment Information:</p>
                        <ul className="space-y-1">
                          <li>• 1% platform fee will be deducted automatically</li>
                          <li>• 99% goes directly to the invoice creator</li>
                          <li>• Payment is processed instantly on the blockchain</li>
                        </ul>
                      </div>
                    </div>
                  </div>
                </div>
              )}
            </div>
          ) : (
            <div className="card text-center">
              <CheckCircle className="h-12 w-12 text-success-600 mx-auto mb-4" />
              <h2 className="text-2xl font-bold text-gray-900 mb-2">
                Invoice Paid
              </h2>
              <p className="text-gray-600">
                This invoice has been fully paid. Thank you!
              </p>
            </div>
          )}

          {/* Share Options */}
          <div className="card mt-8">
            <h3 className="text-lg font-semibold text-gray-900 mb-4">
              Share This Invoice
            </h3>
            <div className="space-y-3">
              <div className="flex items-center space-x-3">
                <input
                  type="text"
                  value={invoice.payment_link}
                  readOnly
                  className="input flex-1 text-sm"
                />
                <button
                  onClick={() => copyToClipboard(invoice.payment_link)}
                  className="btn-secondary"
                >
                  <Copy className="h-4 w-4" />
                </button>
              </div>
              
              {invoice.qr_code_url && (
                <div className="text-center pt-4">
                  <p className="text-sm text-gray-600 mb-3">
                    Or scan this QR code:
                  </p>
                  <img
                    src={invoice.qr_code_url}
                    alt="Payment QR Code"
                    className="mx-auto max-w-[200px] border rounded-lg"
                  />
                </div>
              )}
            </div>
          </div>
        </div>
      </div>
    </div>
  );
}
