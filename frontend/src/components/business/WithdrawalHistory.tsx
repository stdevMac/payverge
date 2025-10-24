'use client';

import React, { useState, useEffect } from 'react';
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

  const loadWithdrawals = async (page: number = 1) => {
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
  };

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
  }, [businessId, isAuthenticated, authLoading]);

  if (loading && withdrawals.length === 0) {
    return (
      <Card className="w-full">
        <CardBody className="flex items-center justify-center py-8">
          <Spinner size="lg" />
          <p className="mt-4 text-gray-600">{tString('loading')}</p>
        </CardBody>
      </Card>
    );
  }

  if (error) {
    return (
      <Card className="w-full">
        <CardBody className="text-center py-8">
          <XCircle className="w-12 h-12 text-red-500 mx-auto mb-4" />
          <h3 className="text-lg font-semibold text-gray-900 mb-2">{tString('errorTitle')}</h3>
          <p className="text-gray-600 mb-4">{error}</p>
          <Button color="primary" onPress={() => loadWithdrawals()}>
            {tString('tryAgain')}
          </Button>
        </CardBody>
      </Card>
    );
  }

  return (
    <>
      <Card className="w-full">
        <CardHeader className="flex gap-3">
          <div className="flex items-center justify-center w-12 h-12 rounded-xl bg-blue-50">
            <History className="w-6 h-6 text-blue-600" />
          </div>
          <div>
            <h4 className="text-lg font-medium text-gray-900">{tString('title')}</h4>
            <p className="text-sm text-gray-600">{tString('subtitle')}</p>
          </div>
        </CardHeader>
        <CardBody>
          {withdrawals.length === 0 ? (
            <div className="text-center py-8">
              <Wallet className="w-12 h-12 text-gray-400 mx-auto mb-4" />
              <h3 className="text-lg font-semibold text-gray-900 mb-2">{tString('emptyTitle')}</h3>
              <p className="text-gray-600">{tString('emptyDescription')}</p>
            </div>
          ) : (
            <>
              <Table aria-label={tString('tableAriaLabel')}>
                <TableHeader>
                  <TableColumn>{tString('table.date')}</TableColumn>
                  <TableColumn>{tString('table.amount')}</TableColumn>
                  <TableColumn>{tString('table.status')}</TableColumn>
                  <TableColumn>{tString('table.network')}</TableColumn>
                  <TableColumn>{tString('table.actions')}</TableColumn>
                </TableHeader>
                <TableBody>
                  {withdrawals.map((withdrawal) => (
                    <TableRow key={withdrawal.id}>
                      <TableCell>
                        <div className="flex items-center gap-2">
                          <Calendar className="w-4 h-4 text-gray-400" />
                          <span className="text-sm">{formatDate(withdrawal.created_at)}</span>
                        </div>
                      </TableCell>
                      <TableCell>
                        <div className="flex flex-col">
                          <span className="font-semibold">{formatAmount(withdrawal.total_amount)}</span>
                          <span className="text-xs text-gray-500">
                            {tString('table.payment')}: {formatAmount(withdrawal.payment_amount)} | 
                            {tString('table.tips')}: {formatAmount(withdrawal.tip_amount)}
                          </span>
                        </div>
                      </TableCell>
                      <TableCell>
                        <Chip
                          color={getStatusColor(withdrawal.status)}
                          variant="flat"
                          startContent={getStatusIcon(withdrawal.status)}
                          size="sm"
                        >
                          {withdrawal.status.charAt(0).toUpperCase() + withdrawal.status.slice(1)}
                        </Chip>
                      </TableCell>
                      <TableCell>
                        <span className="text-sm capitalize">{withdrawal.blockchain_network}</span>
                      </TableCell>
                      <TableCell>
                        <div className="flex gap-2">
                          <Button
                            isIconOnly
                            size="sm"
                            variant="light"
                            onPress={() => viewDetails(withdrawal)}
                          >
                            <Eye className="w-4 h-4" />
                          </Button>
                          <Button
                            isIconOnly
                            size="sm"
                            variant="light"
                            onPress={() => openBlockchainExplorer(withdrawal.transaction_hash, withdrawal.blockchain_network)}
                          >
                            <ExternalLink className="w-4 h-4" />
                          </Button>
                        </div>
                      </TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>

              {totalPages > 1 && (
                <div className="flex justify-center mt-6">
                  <Pagination
                    total={totalPages}
                    page={currentPage}
                    onChange={handlePageChange}
                    showControls
                  />
                </div>
              )}
            </>
          )}
        </CardBody>
      </Card>

      {/* Withdrawal Details Modal */}
      <Modal isOpen={isOpen} onClose={onClose} size="2xl">
        <ModalContent>
          <ModalHeader className="flex flex-col gap-1">
            <h3 className="text-xl font-semibold">{tString('modal.title')}</h3>
            <p className="text-sm text-gray-600">{tString('modal.subtitle')}</p>
          </ModalHeader>
          <ModalBody>
            {selectedWithdrawal && (
              <div className="space-y-6">
                {/* Status and Amount */}
                <div className="flex items-center justify-between">
                  <div>
                    <h4 className="text-lg font-semibold">{formatAmount(selectedWithdrawal.total_amount)}</h4>
                    <p className="text-sm text-gray-600">{tString('modal.totalWithdrawn')}</p>
                  </div>
                  <Chip
                    color={getStatusColor(selectedWithdrawal.status)}
                    variant="flat"
                    startContent={getStatusIcon(selectedWithdrawal.status)}
                  >
                    {selectedWithdrawal.status.charAt(0).toUpperCase() + selectedWithdrawal.status.slice(1)}
                  </Chip>
                </div>

                {/* Amount Breakdown */}
                <div className="grid grid-cols-2 gap-4">
                  <Card>
                    <CardBody className="text-center py-4">
                      <DollarSign className="w-8 h-8 text-green-600 mx-auto mb-2" />
                      <p className="text-lg font-semibold">{formatAmount(selectedWithdrawal.payment_amount)}</p>
                      <p className="text-sm text-gray-600">{tString('modal.paymentEarnings')}</p>
                    </CardBody>
                  </Card>
                  <Card>
                    <CardBody className="text-center py-4">
                      <DollarSign className="w-8 h-8 text-orange-600 mx-auto mb-2" />
                      <p className="text-lg font-semibold">{formatAmount(selectedWithdrawal.tip_amount)}</p>
                      <p className="text-sm text-gray-600">{tString('modal.tipEarnings')}</p>
                    </CardBody>
                  </Card>
                </div>

                {/* Transaction Details */}
                <div className="space-y-4">
                  <div>
                    <label className="text-sm font-medium text-gray-700">{tString('modal.transactionHash')}</label>
                    <div className="flex items-center gap-2 mt-1">
                      <code className="flex-1 p-2 bg-gray-100 rounded text-sm font-mono">
                        {selectedWithdrawal.transaction_hash}
                      </code>
                      <Button
                        isIconOnly
                        size="sm"
                        variant="light"
                        onPress={() => openBlockchainExplorer(selectedWithdrawal.transaction_hash, selectedWithdrawal.blockchain_network)}
                      >
                        <ExternalLink className="w-4 h-4" />
                      </Button>
                    </div>
                  </div>

                  <div>
                    <label className="text-sm font-medium text-gray-700">{tString('modal.withdrawalAddress')}</label>
                    <code className="block mt-1 p-2 bg-gray-100 rounded text-sm font-mono">
                      {selectedWithdrawal.withdrawal_address}
                    </code>
                  </div>

                  <div className="grid grid-cols-2 gap-4">
                    <div>
                      <label className="text-sm font-medium text-gray-700">{tString('modal.network')}</label>
                      <p className="mt-1 capitalize">{selectedWithdrawal.blockchain_network}</p>
                    </div>
                    <div>
                      <label className="text-sm font-medium text-gray-700">{tString('modal.created')}</label>
                      <p className="mt-1">{formatDate(selectedWithdrawal.created_at)}</p>
                    </div>
                  </div>

                  {selectedWithdrawal.confirmed_at && (
                    <div>
                      <label className="text-sm font-medium text-gray-700">{tString('modal.confirmed')}</label>
                      <p className="mt-1">{formatDate(selectedWithdrawal.confirmed_at)}</p>
                    </div>
                  )}
                </div>
              </div>
            )}
          </ModalBody>
          <ModalFooter>
            <Button variant="light" onPress={onClose}>
              {tString('modal.close')}
            </Button>
            {selectedWithdrawal && (
              <Button
                color="primary"
                onPress={() => openBlockchainExplorer(selectedWithdrawal.transaction_hash, selectedWithdrawal.blockchain_network)}
                startContent={<ExternalLink className="w-4 h-4" />}
              >
                {tString('modal.viewOnExplorer')}
              </Button>
            )}
          </ModalFooter>
        </ModalContent>
      </Modal>
    </>
  );
}
