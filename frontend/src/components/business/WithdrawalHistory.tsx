'use client';

import React, { useState, useEffect, useCallback } from 'react';
import { useSimpleLocale, getTranslation } from "@/i18n/SimpleTranslationProvider";
import {
  Card,
  CardBody,
  CardHeader,
  Table,
  TableHeader,
  TableColumn,
  TableBody,
  TableRow,
  TableCell,
  Chip,
  Button,
  Pagination,
  Spinner,
  Modal,
  ModalContent,
  ModalHeader,
  ModalBody,
  ModalFooter,
  useDisclosure
} from '@nextui-org/react';
import { 
  History, 
  ExternalLink, 
  Calendar, 
  DollarSign, 
  Wallet,
  CheckCircle,
  Clock,
  XCircle,
  Eye
} from 'lucide-react';
import { getWithdrawalHistory, WithdrawalHistory, WithdrawalHistoryResponse } from '../../api/withdrawals';

interface WithdrawalHistoryProps {
  businessId: number;
  isAuthenticated?: () => boolean;
  authLoading?: boolean;
}

export default function WithdrawalHistoryComponent({ businessId, isAuthenticated, authLoading }: WithdrawalHistoryProps) {
  const { locale } = useSimpleLocale();
  const [currentLocale, setCurrentLocale] = useState(locale);
  const [withdrawals, setWithdrawals] = useState<WithdrawalHistory[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [currentPage, setCurrentPage] = useState(1);
  const [totalPages, setTotalPages] = useState(1);
  const [selectedWithdrawal, setSelectedWithdrawal] = useState<WithdrawalHistory | null>(null);
  
  // Update translations when locale changes
  useEffect(() => {
    setCurrentLocale(locale);
  }, [locale]);
  
  // Translation helper
  const tString = (key: string): string => {
    const fullKey = `withdrawalHistory.${key}`;
    const result = getTranslation(fullKey, currentLocale);
    return Array.isArray(result) ? result[0] || key : result as string;
  };
  const { isOpen, onOpen, onClose } = useDisclosure();

  const loadWithdrawals = useCallback(async (page: number = 1) => {
    try {
      setLoading(true);
      setError(null);
      
      const response: WithdrawalHistoryResponse = await getWithdrawalHistory(businessId, page, 10);
      setWithdrawals(response.withdrawals);
      setTotalPages(response.pagination.pages);
      setCurrentPage(page);
    } catch (err: any) {
      console.error('Error loading withdrawal history:', err);
      
      // Don't show error for authentication issues (401) or empty results (404)
      if (err.response?.status === 401) {
        // Authentication error - silently fail and show empty state
        setWithdrawals([]);
        setTotalPages(1);
        setCurrentPage(1);
      } else if (err.response?.status !== 404) {
        setError(err.response?.data?.error || err.message || 'Failed to load withdrawal history');
      } else {
        // 404 means no withdrawals yet, which is normal
        setWithdrawals([]);
        setTotalPages(1);
        setCurrentPage(1);
      }
    } finally {
      setLoading(false);
    }
  }, [businessId]);

  const handlePageChange = (page: number) => {
    loadWithdrawals(page);
  };

  const getStatusColor = (status: string) => {
    switch (status) {
      case 'confirmed':
        return 'success';
      case 'pending':
        return 'warning';
      case 'failed':
        return 'danger';
      default:
        return 'default';
    }
  };

  const getStatusIcon = (status: string) => {
    switch (status) {
      case 'confirmed':
        return <CheckCircle className="w-4 h-4" />;
      case 'pending':
        return <Clock className="w-4 h-4" />;
      case 'failed':
        return <XCircle className="w-4 h-4" />;
      default:
        return <Clock className="w-4 h-4" />;
    }
  };

  const formatDate = (dateString: string) => {
    return new Date(dateString).toLocaleString();
  };

  const formatAmount = (amount: number) => {
    return `$${amount.toFixed(2)}`;
  };

  const truncateHash = (hash: string) => {
    return `${hash.slice(0, 8)}...${hash.slice(-8)}`;
  };

  const openBlockchainExplorer = (hash: string, network: string) => {
    let explorerUrl = '';
    switch (network) {
      case 'ethereum':
        explorerUrl = `https://etherscan.io/tx/${hash}`;
        break;
      case 'base-sepolia':
        explorerUrl = `https://sepolia.basescan.org/tx/${hash}`;
        break;
      case 'base':
        explorerUrl = `https://basescan.org/tx/${hash}`;
        break;
      default:
        explorerUrl = `https://etherscan.io/tx/${hash}`;
    }
    window.open(explorerUrl, '_blank');
  };

  const viewDetails = (withdrawal: WithdrawalHistory) => {
    setSelectedWithdrawal(withdrawal);
    onOpen();
  };

  useEffect(() => {
    // Don't load if still checking authentication
    if (authLoading) {
      return;
    }
    
    // Only load if authenticated
    if (isAuthenticated && isAuthenticated()) {
      loadWithdrawals();
    } else {
      setLoading(false);
      setWithdrawals([]);
    }
  }, [businessId, isAuthenticated, authLoading, loadWithdrawals]);

  if (loading && withdrawals.length === 0) {
    return (
      <div className="flex items-center justify-center py-16">
        <div className="text-center">
          <div className="w-16 h-16 bg-gray-50 rounded-2xl flex items-center justify-center mx-auto mb-6 border border-gray-100">
            <div className="w-8 h-8 border-2 border-gray-300 border-t-gray-900 rounded-full animate-spin"></div>
          </div>
          <p className="text-gray-600 font-light tracking-wide">{tString('loading')}</p>
        </div>
      </div>
    );
  }

  if (error) {
    return (
      <div className="bg-red-50 border border-red-200 rounded-lg p-4">
        <div className="flex items-center gap-3">
          <XCircle className="w-5 h-5 text-red-600 flex-shrink-0" />
          <div>
            <h3 className="text-lg font-medium text-red-800">{tString('errorTitle')}</h3>
            <p className="text-red-700 text-sm">{error}</p>
            <button
              className="mt-2 px-3 py-2 text-sm font-medium text-white bg-red-600 rounded-lg hover:bg-red-700"
              onClick={() => loadWithdrawals()}
            >
              {tString('tryAgain')}
            </button>
          </div>
        </div>
      </div>
    );
  }

  return (
    <>
      <div className="bg-white border border-gray-200 rounded-lg shadow-sm">
        <div className="border-b border-gray-200 p-4">
          <div className="flex items-center gap-3">
            <div className="w-10 h-10 bg-gray-50 rounded-xl flex items-center justify-center border border-gray-100">
              <History className="w-5 h-5 text-gray-700" />
            </div>
            <div>
              <h3 className="text-lg font-light text-gray-900 tracking-wide">{tString('title')}</h3>
              <p className="text-sm text-gray-600">{tString('subtitle')}</p>
            </div>
          </div>
        </div>
        <div className="p-4">
          {withdrawals.length === 0 ? (
            <div className="text-center py-12">
              <div className="w-16 h-16 bg-gray-50 rounded-2xl flex items-center justify-center mx-auto mb-6 border border-gray-100">
                <Wallet className="w-8 h-8 text-gray-400" />
              </div>
              <h3 className="text-lg font-light text-gray-900 tracking-wide mb-2">{tString('emptyTitle')}</h3>
              <p className="text-gray-600 font-light text-sm">{tString('emptyDescription')}</p>
            </div>
          ) : (
            <>
              <div className="overflow-x-auto">
                <table className="w-full">
                  <thead className="bg-gray-50">
                    <tr>
                      <th className="px-4 py-3 text-left text-xs font-medium text-gray-500 uppercase tracking-wider">{tString('table.date')}</th>
                      <th className="px-4 py-3 text-left text-xs font-medium text-gray-500 uppercase tracking-wider">{tString('table.amount')}</th>
                      <th className="px-4 py-3 text-left text-xs font-medium text-gray-500 uppercase tracking-wider">{tString('table.status')}</th>
                      <th className="px-4 py-3 text-left text-xs font-medium text-gray-500 uppercase tracking-wider">{tString('table.network')}</th>
                      <th className="px-4 py-3 text-left text-xs font-medium text-gray-500 uppercase tracking-wider">{tString('table.actions')}</th>
                    </tr>
                  </thead>
                  <tbody className="bg-white divide-y divide-gray-200">
                    {withdrawals.map((withdrawal) => (
                      <tr key={withdrawal.id} className="hover:bg-gray-50">
                        <td className="px-4 py-3">
                          <div className="flex items-center gap-2">
                            <Calendar className="w-4 h-4 text-gray-400" />
                            <span className="text-sm text-gray-900">{formatDate(withdrawal.created_at)}</span>
                          </div>
                        </td>
                        <td className="px-4 py-3">
                          <div className="flex flex-col">
                            <span className="font-semibold text-gray-900">{formatAmount(withdrawal.total_amount)}</span>
                            <span className="text-xs text-gray-500">
                              {tString('table.payment')}: {formatAmount(withdrawal.payment_amount)} | 
                              {tString('table.tips')}: {formatAmount(withdrawal.tip_amount)}
                            </span>
                          </div>
                        </td>
                        <td className="px-4 py-3">
                          <span className={`inline-flex items-center px-2 py-1 rounded-full text-xs font-medium ${
                            withdrawal.status === 'confirmed' ? 'bg-green-100 text-green-700' :
                            withdrawal.status === 'pending' ? 'bg-yellow-100 text-yellow-700' :
                            'bg-red-100 text-red-700'
                          }`}>
                            {getStatusIcon(withdrawal.status)}
                            <span className="ml-1">{withdrawal.status.charAt(0).toUpperCase() + withdrawal.status.slice(1)}</span>
                          </span>
                        </td>
                        <td className="px-4 py-3">
                          <span className="text-sm capitalize text-gray-900">{withdrawal.blockchain_network}</span>
                        </td>
                        <td className="px-4 py-3">
                          <div className="flex gap-2">
                            <button
                              className="p-1 text-gray-400 hover:text-gray-600"
                              onClick={() => viewDetails(withdrawal)}
                            >
                              <Eye className="w-4 h-4" />
                            </button>
                            <button
                              className="p-1 text-gray-400 hover:text-gray-600"
                              onClick={() => openBlockchainExplorer(withdrawal.transaction_hash, withdrawal.blockchain_network)}
                            >
                              <ExternalLink className="w-4 h-4" />
                            </button>
                          </div>
                        </td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>

              {totalPages > 1 && (
                <div className="flex justify-center mt-6">
                  <div className="flex items-center space-x-2">
                    <button
                      className="px-3 py-2 text-sm font-medium text-gray-500 bg-white border border-gray-200 rounded-lg hover:bg-gray-50 disabled:opacity-50 disabled:cursor-not-allowed"
                      onClick={() => handlePageChange(currentPage - 1)}
                      disabled={currentPage === 1}
                    >
                      Previous
                    </button>
                    <span className="px-3 py-2 text-sm text-gray-700">
                      Page {currentPage} of {totalPages}
                    </span>
                    <button
                      className="px-3 py-2 text-sm font-medium text-gray-500 bg-white border border-gray-200 rounded-lg hover:bg-gray-50 disabled:opacity-50 disabled:cursor-not-allowed"
                      onClick={() => handlePageChange(currentPage + 1)}
                      disabled={currentPage === totalPages}
                    >
                      Next
                    </button>
                  </div>
                </div>
              )}
            </>
          )}
        </div>
      </div>

      {/* Withdrawal Details Modal */}
      {isOpen && selectedWithdrawal && (
        <div className="fixed inset-0 bg-black/50 backdrop-blur-sm z-50 flex items-center justify-center p-4">
          <div className="w-full max-w-2xl bg-white border border-gray-200 rounded-lg shadow-sm">
            <div className="border-b border-gray-200 p-4">
              <h3 className="text-xl font-light text-gray-900 tracking-wide">{tString('modal.title')}</h3>
              <p className="text-sm text-gray-600">{tString('modal.subtitle')}</p>
            </div>
            <div className="p-6">
              <div className="space-y-6">
                {/* Status and Amount */}
                <div className="flex items-center justify-between">
                  <div>
                    <h4 className="text-lg font-semibold text-gray-900">{formatAmount(selectedWithdrawal.total_amount)}</h4>
                    <p className="text-sm text-gray-600">{tString('modal.totalWithdrawn')}</p>
                  </div>
                  <span className={`inline-flex items-center px-2 py-1 rounded-full text-xs font-medium ${
                    selectedWithdrawal.status === 'confirmed' ? 'bg-green-100 text-green-700' :
                    selectedWithdrawal.status === 'pending' ? 'bg-yellow-100 text-yellow-700' :
                    'bg-red-100 text-red-700'
                  }`}>
                    {getStatusIcon(selectedWithdrawal.status)}
                    <span className="ml-1">{selectedWithdrawal.status.charAt(0).toUpperCase() + selectedWithdrawal.status.slice(1)}</span>
                  </span>
                </div>

                {/* Amount Breakdown */}
                <div className="grid grid-cols-2 gap-4">
                  <div className="text-center py-4 bg-gray-50 border border-gray-200 rounded-lg">
                    <DollarSign className="w-8 h-8 text-green-600 mx-auto mb-2" />
                    <p className="text-lg font-semibold text-gray-900">{formatAmount(selectedWithdrawal.payment_amount)}</p>
                    <p className="text-sm text-gray-600">{tString('modal.paymentEarnings')}</p>
                  </div>
                  <div className="text-center py-4 bg-gray-50 border border-gray-200 rounded-lg">
                    <DollarSign className="w-8 h-8 text-yellow-600 mx-auto mb-2" />
                    <p className="text-lg font-semibold text-gray-900">{formatAmount(selectedWithdrawal.tip_amount)}</p>
                    <p className="text-sm text-gray-600">{tString('modal.tipEarnings')}</p>
                  </div>
                </div>

                {/* Transaction Details */}
                <div className="space-y-4">
                  <div>
                    <label className="text-sm font-medium text-gray-700">{tString('modal.transactionHash')}</label>
                    <div className="flex items-center gap-2 mt-1">
                      <code className="flex-1 p-2 bg-gray-50 border border-gray-200 rounded text-sm font-mono">
                        {selectedWithdrawal.transaction_hash}
                      </code>
                      <button
                        className="p-1 text-gray-400 hover:text-gray-600"
                        onClick={() => openBlockchainExplorer(selectedWithdrawal.transaction_hash, selectedWithdrawal.blockchain_network)}
                      >
                        <ExternalLink className="w-4 h-4" />
                      </button>
                    </div>
                  </div>

                  <div>
                    <label className="text-sm font-medium text-gray-700">{tString('modal.withdrawalAddress')}</label>
                    <code className="block mt-1 p-2 bg-gray-50 border border-gray-200 rounded text-sm font-mono">
                      {selectedWithdrawal.withdrawal_address}
                    </code>
                  </div>

                  <div className="grid grid-cols-2 gap-4">
                    <div>
                      <label className="text-sm font-medium text-gray-700">{tString('modal.network')}</label>
                      <p className="mt-1 capitalize text-gray-900">{selectedWithdrawal.blockchain_network}</p>
                    </div>
                    <div>
                      <label className="text-sm font-medium text-gray-700">{tString('modal.created')}</label>
                      <p className="mt-1 text-gray-900">{formatDate(selectedWithdrawal.created_at)}</p>
                    </div>
                  </div>

                  {selectedWithdrawal.confirmed_at && (
                    <div>
                      <label className="text-sm font-medium text-gray-700">{tString('modal.confirmed')}</label>
                      <p className="mt-1 text-gray-900">{formatDate(selectedWithdrawal.confirmed_at)}</p>
                    </div>
                  )}
                </div>
              </div>
            </div>
            <div className="border-t border-gray-200 p-4 flex justify-end gap-3">
              <button
                className="px-3 py-2 text-sm font-medium text-gray-700 bg-white border border-gray-200 rounded-lg hover:bg-gray-50"
                onClick={onClose}
              >
                {tString('modal.close')}
              </button>
              <button
                className="px-3 py-2 text-sm font-medium text-white bg-gray-900 rounded-lg hover:bg-gray-800 flex items-center gap-2"
                onClick={() => openBlockchainExplorer(selectedWithdrawal.transaction_hash, selectedWithdrawal.blockchain_network)}
              >
                <ExternalLink className="w-4 h-4" />
                {tString('modal.viewOnExplorer')}
              </button>
            </div>
          </div>
        </div>
      )}
    </>
  );
}
