'use client';

import React, { useState } from 'react';
import { Card, CardBody, CardHeader, Button, Input, Spinner } from '@nextui-org/react';
import { Wallet, DollarSign, Edit3, ExternalLink, Check, X, AlertCircle } from 'lucide-react';
import { useSimpleLocale, getTranslation } from '@/i18n/SimpleTranslationProvider';
import { Business } from '@/api/business';
import { 
  useBusinessInfo, 
  useGetClaimableAmounts, 
  useUpdateBusinessPaymentAddress, 
  useUpdateBusinessTippingAddress, 
  useClaimEarnings, 
  formatUsdcAmount 
} from '@/contracts/hooks';
import { Address } from 'viem';
import { createWithdrawal } from '@/api/withdrawals';
import WithdrawalHistory from './WithdrawalHistory';

interface BlockchainManagerProps {
  business: Business;
  businessId: number;
  userAddress?: string;
  isConnected?: boolean;
}

export default function BlockchainManager({ business, businessId, userAddress, isConnected = false }: BlockchainManagerProps) {
  // Translation setup
  const { locale } = useSimpleLocale();
  const [currentLocale, setCurrentLocale] = useState(locale);
  
  React.useEffect(() => {
    setCurrentLocale(locale);
  }, [locale]);
  
  const tString = (key: string): string => {
    const fullKey = `businessDashboard.${key}`;
    const result = getTranslation(fullKey, currentLocale);
    return Array.isArray(result) ? result[0] || key : result as string;
  };

  // State for editing addresses
  const [editingPaymentAddress, setEditingPaymentAddress] = useState(false);
  const [editingTippingAddress, setEditingTippingAddress] = useState(false);
  const [newPaymentAddress, setNewPaymentAddress] = useState('');
  const [newTippingAddress, setNewTippingAddress] = useState('');
  const [claimingEarnings, setClaimingEarnings] = useState(false);
  const [claimingStep, setClaimingStep] = useState<string>('');
  const [transactionHash, setTransactionHash] = useState<string>('');

  // Smart contract hooks
  const { data: businessInfo, refetch: refetchBusinessInfo } = useBusinessInfo(business?.settlement_address as Address);
  
  const paymentAddress = (businessInfo as any)?.paymentAddress || business?.settlement_address;
  const tippingAddress = (businessInfo as any)?.tippingAddress || business?.tipping_address;
  
  const { data: claimableBalance, refetch: refetchClaimableBalance } = useGetClaimableAmounts(
    paymentAddress as Address, 
    tippingAddress as Address
  );
  
  const { updatePaymentAddress } = useUpdateBusinessPaymentAddress();
  const { updateTippingAddress } = useUpdateBusinessTippingAddress();
  const { claimEarnings } = useClaimEarnings();

  // Handlers
  const handleUpdatePaymentAddress = async () => {
    if (!newPaymentAddress) return;
    
    try {
      await updatePaymentAddress(newPaymentAddress as Address);
      setEditingPaymentAddress(false);
      setNewPaymentAddress('');
      refetchBusinessInfo();
    } catch (error) {
      console.error('Error updating payment address:', error);
    }
  };

  const handleUpdateTippingAddress = async () => {
    if (!newTippingAddress) return;
    
    try {
      await updateTippingAddress(newTippingAddress as Address);
      setEditingTippingAddress(false);
      setNewTippingAddress('');
      refetchBusinessInfo();
    } catch (error) {
      console.error('Error updating tipping address:', error);
    }
  };

  const handleClaimEarnings = async () => {
    if (!claimableBalance || !Array.isArray(claimableBalance) || 
        (claimableBalance[0] === BigInt(0) && claimableBalance[1] === BigInt(0))) {
      return;
    }

    setClaimingEarnings(true);
    setClaimingStep(tString('blockchain.claiming.preparing'));
    
    try {
      setClaimingStep(tString('blockchain.claiming.confirmWallet'));
      const hash = await claimEarnings();
      setTransactionHash(hash);
      
      setClaimingStep(tString('blockchain.claiming.submitted'));
      
      try {
        const paymentAmount = parseFloat(formatUsdcAmount(claimableBalance[0] || BigInt(0)));
        const tipAmount = parseFloat(formatUsdcAmount(claimableBalance[1] || BigInt(0)));
        const totalAmount = paymentAmount + tipAmount;

        await createWithdrawal(businessId, {
          transaction_hash: hash,
          payment_amount: paymentAmount,
          tip_amount: tipAmount,
          total_amount: totalAmount,
          withdrawal_address: userAddress || '',
          blockchain_network: 'base-sepolia'
        });
        
        setClaimingStep(tString('blockchain.claiming.recording'));
      } catch (backendError) {
        console.error('Backend withdrawal record error:', backendError);
      }
      
      setClaimingStep(tString('blockchain.claiming.complete'));
      
      setTimeout(() => {
        setClaimingEarnings(false);
        setClaimingStep('');
        setTransactionHash('');
        refetchClaimableBalance();
      }, 3000);
      
    } catch (error) {
      console.error('Error claiming earnings:', error);
      setClaimingStep(tString('blockchain.claiming.error'));
      setTimeout(() => {
        setClaimingEarnings(false);
        setClaimingStep('');
      }, 3000);
    }
  };

  const formatBalance = (balance: bigint) => {
    return parseFloat(formatUsdcAmount(balance)).toFixed(2);
  };

  const getTotalClaimable = () => {
    if (!claimableBalance || !Array.isArray(claimableBalance)) return '0.00';
    const total = claimableBalance[0] + claimableBalance[1];
    return formatBalance(total);
  };

  return (
    <div className="p-4 max-w-6xl mx-auto">
      {/* Compact Header */}
      <div className="flex justify-between items-center mb-6">
        <div>
          <h1 className="text-2xl font-light text-gray-900 tracking-wide">{tString('blockchain.title')}</h1>
          <p className="text-gray-600 font-light text-sm mt-1">{tString('blockchain.subtitle')}</p>
        </div>
      </div>

      <div className="space-y-6">
        {/* Status Section */}
        <div className="bg-white border border-gray-200 rounded-lg shadow-sm">
          <div className="border-b border-gray-200 p-4">
            <div className="flex items-center gap-3">
              <div className="w-10 h-10 bg-gray-50 rounded-xl flex items-center justify-center border border-gray-100">
                <Wallet className="w-5 h-5 text-gray-700" />
              </div>
              <div>
                <h2 className="text-lg font-light text-gray-900 tracking-wide">{tString('blockchain.subtitle')}</h2>
              </div>
            </div>
          </div>
          <div className="p-4">

            <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
              <div className="flex items-center gap-3 p-3 bg-gray-50 rounded-lg">
                <div className={`w-3 h-3 rounded-full ${
                  isConnected ? 'bg-green-500 animate-pulse' : 'bg-red-500'
                }`}></div>
                <span className="text-sm font-medium text-gray-700">
                  {isConnected ? tString('blockchain.status.walletConnected') : tString('blockchain.status.walletNotConnected')}
                </span>
                {userAddress && (
                  <span className="text-xs font-mono text-gray-500 bg-white px-2 py-1 rounded border ml-auto">
                    {userAddress.slice(0, 6)}...{userAddress.slice(-4)}
                  </span>
                )}
              </div>
              
              <div className="flex items-center gap-3 p-3 bg-gray-50 rounded-lg">
                <div className={`w-3 h-3 rounded-full ${
                  businessInfo ? 'bg-green-500' : 'bg-yellow-500'
                }`}></div>
                <span className="text-sm font-medium text-gray-700">
                  {businessInfo ? tString('blockchain.status.registered') : tString('blockchain.status.notRegistered')}
                </span>
                {business?.settlement_address && (
                  <span className="text-xs font-mono text-gray-500 bg-white px-2 py-1 rounded border ml-auto">
                    {business.settlement_address.slice(0, 6)}...{business.settlement_address.slice(-4)}
                  </span>
                )}
              </div>
            </div>
          </div>
        </div>

        {/* Claimable Balances */}
        <div className="bg-white border border-gray-200 rounded-lg shadow-sm">
          <div className="border-b border-gray-200 p-4">
            <div className="flex items-center gap-3">
              <div className="w-10 h-10 bg-gray-50 rounded-xl flex items-center justify-center border border-gray-100">
                <DollarSign className="w-5 h-5 text-gray-700" />
              </div>
              <div>
                <h3 className="text-lg font-light text-gray-900 tracking-wide">{tString('blockchain.claimableEarnings.title')}</h3>
                <p className="text-sm text-gray-600">{tString('blockchain.claimableEarnings.subtitle')}</p>
              </div>
            </div>
          </div>
          <div className="p-4">
            <div className="space-y-4">
              {/* Payment and Tip Amounts */}
              <div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
                <div className="bg-gray-50 border border-gray-200 rounded-lg p-4">
                  <div className="flex items-center gap-2 mb-2">
                    <span className="text-xs font-medium text-gray-600 uppercase tracking-wide">{tString('blockchain.claimableEarnings.paymentEarnings')}</span>
                  </div>
                  <div className="text-2xl font-semibold text-green-600">
                    ${claimableBalance && Array.isArray(claimableBalance) ? formatBalance(claimableBalance[0]) : '0.00'}
                  </div>
                </div>
                
                <div className="bg-gray-50 border border-gray-200 rounded-lg p-4">
                  <div className="flex items-center gap-2 mb-2">
                    <span className="text-xs font-medium text-gray-600 uppercase tracking-wide">{tString('blockchain.claimableEarnings.tipEarnings')}</span>
                  </div>
                  <div className="text-2xl font-semibold text-yellow-600">
                    ${claimableBalance && Array.isArray(claimableBalance) ? formatBalance(claimableBalance[1]) : '0.00'}
                  </div>
                </div>
              </div>

              {/* Total and Claim Button */}
              <div className="border-t border-gray-200 pt-4">
                <div className="flex items-center justify-between mb-4">
                  <span className="text-lg font-light text-gray-900">{tString('blockchain.claimableEarnings.totalAvailable')}</span>
                  <span className="text-2xl font-semibold text-blue-600">
                    ${getTotalClaimable()}
                  </span>
                </div>
                
                <button
                  className={`w-full px-4 py-3 rounded-lg font-medium transition-colors ${
                    getTotalClaimable() !== '0.00'
                      ? 'bg-gray-900 text-white hover:bg-gray-800'
                      : 'bg-gray-100 text-gray-500 cursor-not-allowed'
                  }`}
                  onClick={handleClaimEarnings}
                  disabled={claimingEarnings || getTotalClaimable() === '0.00'}
                >
                  <div className="flex items-center justify-center gap-2">
                    {claimingEarnings ? (
                      <div className="w-4 h-4 border-2 border-white border-t-transparent rounded-full animate-spin"></div>
                    ) : (
                      <Wallet className="w-5 h-5" />
                    )}
                    {claimingEarnings ? claimingStep : tString('buttons.claimEarnings')}
                  </div>
                </button>

                {transactionHash && (
                  <div className="mt-4 text-center">
                    <a
                      href={`https://sepolia.basescan.org/tx/${transactionHash}`}
                      target="_blank"
                      rel="noopener noreferrer"
                      className="text-sm text-blue-600 hover:text-blue-800 inline-flex items-center gap-1 font-medium"
                    >
                      {tString('blockchain.viewTransaction')} <ExternalLink className="w-4 h-4" />
                    </a>
                  </div>
                )}
              </div>
            </div>
          </div>
        </div>

        {/* Transaction Processing Overlay */}
        {claimingEarnings && (
          <div className="fixed inset-0 bg-black/50 backdrop-blur-sm z-50 flex items-center justify-center">
            <div className="w-full max-w-md mx-4 bg-white border border-gray-200 rounded-lg shadow-sm">
              <div className="text-center py-8 p-6">
                <div className="mb-6">
                  <div className="w-16 h-16 bg-gray-50 rounded-full flex items-center justify-center mx-auto mb-4 border border-gray-100">
                    <div className="w-8 h-8 border-2 border-gray-300 border-t-gray-900 rounded-full animate-spin"></div>
                  </div>
                  <h3 className="text-xl font-light text-gray-900 mb-2 tracking-wide">
                    {tString('blockchain.claiming.processingTitle')}
                  </h3>
                  <p className="text-gray-600 mb-4 font-light">
                    {claimingStep}
                  </p>
                  {transactionHash && (
                    <div className="bg-gray-50 rounded-lg p-3 mb-4 border border-gray-200">
                      <p className="text-xs text-gray-500 mb-1">{tString('blockchain.claimableEarnings.transactionHash')}</p>
                      <p className="text-xs font-mono text-gray-700 break-all">
                        {transactionHash}
                      </p>
                    </div>
                  )}
                </div>
                
                <div className="bg-yellow-50 border border-yellow-200 rounded-lg p-4">
                  <div className="flex items-center gap-2 text-yellow-800">
                    <AlertCircle className="w-5 h-5 flex-shrink-0" />
                    <div className="text-sm">
                      <p className="font-medium">{tString('blockchain.claiming.dontClose')}</p>
                      <p>{tString('blockchain.claiming.blockchainProcessing')}</p>
                    </div>
                  </div>
                </div>
              </div>
            </div>
          </div>
        )}

        {/* Business Addresses */}
        <div className="bg-white border border-gray-200 rounded-lg shadow-sm">
          <div className="border-b border-gray-200 p-4">
            <div className="flex items-center gap-3">
              <div className="w-10 h-10 bg-gray-50 rounded-xl flex items-center justify-center border border-gray-100">
                <Wallet className="w-5 h-5 text-gray-700" />
              </div>
              <div>
                <h3 className="text-lg font-light text-gray-900 tracking-wide">{tString('blockchain.addressManagement.title')}</h3>
              </div>
            </div>
          </div>
          <div className="p-4">
            <div className="space-y-6">
              {/* Payment Address */}
              <div>
                <div className="flex items-center justify-between mb-3">
                  <h4 className="font-medium text-gray-900">{tString('blockchain.addressManagement.paymentAddress')}</h4>
                  <button
                    className="px-3 py-2 text-sm font-medium text-gray-700 bg-white border border-gray-200 rounded-lg hover:bg-gray-50 flex items-center gap-2"
                    onClick={() => {
                      setEditingPaymentAddress(!editingPaymentAddress);
                      setNewPaymentAddress(paymentAddress || '');
                    }}
                  >
                    <Edit3 className="w-4 h-4" />
                    {tString('blockchain.addressManagement.edit')}
                  </button>
                </div>
                
                {editingPaymentAddress ? (
                  <div className="space-y-3">
                    <input
                      type="text"
                      value={newPaymentAddress}
                      onChange={(e) => setNewPaymentAddress(e.target.value)}
                      placeholder="0x..."
                      className="w-full px-3 py-2 border border-gray-200 rounded-lg font-mono text-sm focus:outline-none focus:ring-2 focus:ring-blue-500"
                    />
                    <div className="flex gap-2">
                      <button
                        className="px-3 py-2 text-sm font-medium text-white bg-gray-900 rounded-lg hover:bg-gray-800"
                        onClick={handleUpdatePaymentAddress}
                      >
                        {tString('blockchain.addressManagement.update')}
                      </button>
                      <button
                        className="px-3 py-2 text-sm font-medium text-gray-700 bg-white border border-gray-200 rounded-lg hover:bg-gray-50"
                        onClick={() => {
                          setEditingPaymentAddress(false);
                          setNewPaymentAddress('');
                        }}
                      >
                        {tString('buttons.cancel')}
                      </button>
                    </div>
                  </div>
                ) : (
                  <div className="font-mono text-sm text-gray-600 bg-gray-50 p-3 rounded-lg border border-gray-200 break-all">
                    {paymentAddress || 'Not set'}
                  </div>
                )}
              </div>

              {/* Tipping Address */}
              <div>
                <div className="flex items-center justify-between mb-3">
                  <h4 className="font-medium text-gray-900">{tString('blockchain.addressManagement.tippingAddress')}</h4>
                  <button
                    className="px-3 py-2 text-sm font-medium text-gray-700 bg-white border border-gray-200 rounded-lg hover:bg-gray-50 flex items-center gap-2"
                    onClick={() => {
                      setEditingTippingAddress(!editingTippingAddress);
                      setNewTippingAddress(tippingAddress || '');
                    }}
                  >
                    <Edit3 className="w-4 h-4" />
                    {tString('blockchain.addressManagement.edit')}
                  </button>
                </div>
                
                {editingTippingAddress ? (
                  <div className="space-y-3">
                    <input
                      type="text"
                      value={newTippingAddress}
                      onChange={(e) => setNewTippingAddress(e.target.value)}
                      placeholder="0x..."
                      className="w-full px-3 py-2 border border-gray-200 rounded-lg font-mono text-sm focus:outline-none focus:ring-2 focus:ring-blue-500"
                    />
                    <div className="flex gap-2">
                      <button
                        className="px-3 py-2 text-sm font-medium text-white bg-gray-900 rounded-lg hover:bg-gray-800"
                        onClick={handleUpdateTippingAddress}
                      >
                        {tString('blockchain.addressManagement.update')}
                      </button>
                      <button
                        className="px-3 py-2 text-sm font-medium text-gray-700 bg-white border border-gray-200 rounded-lg hover:bg-gray-50"
                        onClick={() => {
                          setEditingTippingAddress(false);
                          setNewTippingAddress('');
                        }}
                      >
                        {tString('buttons.cancel')}
                      </button>
                    </div>
                  </div>
                ) : (
                  <div className="font-mono text-sm text-gray-600 bg-gray-50 p-3 rounded-lg border border-gray-200 break-all">
                    {tippingAddress || 'Not set'}
                  </div>
                )}
              </div>
            </div>
          </div>
        </div>

        {/* Withdrawal History */}
        <WithdrawalHistory businessId={businessId} isAuthenticated={() => true} authLoading={false} />
      </div>
    </div>
  );
}
