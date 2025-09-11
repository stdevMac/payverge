'use client';

import React from 'react';
import { Card, CardBody, Button, Chip, Progress, Spinner } from '@nextui-org/react';
import { CheckCircle2, AlertCircle, Clock, ExternalLink, RefreshCw } from 'lucide-react';

interface TransactionStatusProps {
  transactionHash?: string;
  status: 'pending' | 'confirming' | 'confirmed' | 'failed';
  confirmations?: number;
  requiredConfirmations?: number;
  blockNumber?: number;
  gasUsed?: number;
  error?: string;
  onRetry?: () => void;
}

export default function TransactionStatus({
  transactionHash,
  status,
  confirmations = 0,
  requiredConfirmations = 12,
  blockNumber,
  gasUsed,
  error,
  onRetry
}: TransactionStatusProps) {
  const getStatusIcon = () => {
    switch (status) {
      case 'pending':
        return <Clock className="text-warning" size={24} />;
      case 'confirming':
        return <Spinner size="sm" color="primary" />;
      case 'confirmed':
        return <CheckCircle2 className="text-success" size={24} />;
      case 'failed':
        return <AlertCircle className="text-danger" size={24} />;
      default:
        return <Clock className="text-gray-400" size={24} />;
    }
  };

  const getStatusColor = () => {
    switch (status) {
      case 'pending':
        return 'warning';
      case 'confirming':
        return 'primary';
      case 'confirmed':
        return 'success';
      case 'failed':
        return 'danger';
      default:
        return 'default';
    }
  };

  const getStatusText = () => {
    switch (status) {
      case 'pending':
        return 'Transaction Pending';
      case 'confirming':
        return 'Confirming Transaction';
      case 'confirmed':
        return 'Transaction Confirmed';
      case 'failed':
        return 'Transaction Failed';
      default:
        return 'Unknown Status';
    }
  };

  const getStatusDescription = () => {
    switch (status) {
      case 'pending':
        return 'Your transaction has been submitted to the network and is waiting to be processed.';
      case 'confirming':
        return `Your transaction is being confirmed. ${confirmations}/${requiredConfirmations} confirmations received.`;
      case 'confirmed':
        return 'Your transaction has been successfully confirmed on the blockchain.';
      case 'failed':
        return error || 'Your transaction failed to process. Please try again.';
      default:
        return 'Checking transaction status...';
    }
  };

  const openEtherscan = () => {
    if (transactionHash) {
      window.open(`https://etherscan.io/tx/${transactionHash}`, '_blank');
    }
  };

  const confirmationProgress = Math.min((confirmations / requiredConfirmations) * 100, 100);

  return (
    <Card className={`border-2 ${
      status === 'confirmed' ? 'border-success-200 bg-success-50' :
      status === 'failed' ? 'border-danger-200 bg-danger-50' :
      status === 'confirming' ? 'border-primary-200 bg-primary-50' :
      'border-warning-200 bg-warning-50'
    }`}>
      <CardBody className="space-y-4">
        {/* Status Header */}
        <div className="flex items-center gap-3">
          {getStatusIcon()}
          <div className="flex-1">
            <div className="flex items-center gap-2">
              <h3 className="font-semibold">{getStatusText()}</h3>
              <Chip
                size="sm"
                color={getStatusColor()}
                variant="flat"
              >
                {status.toUpperCase()}
              </Chip>
            </div>
            <p className="text-sm text-gray-600 mt-1">
              {getStatusDescription()}
            </p>
          </div>
        </div>

        {/* Confirmation Progress */}
        {status === 'confirming' && (
          <div className="space-y-2">
            <div className="flex justify-between text-sm">
              <span>Confirmations</span>
              <span>{confirmations}/{requiredConfirmations}</span>
            </div>
            <Progress
              value={confirmationProgress}
              color="primary"
              size="sm"
              className="w-full"
            />
          </div>
        )}

        {/* Transaction Details */}
        {transactionHash && (
          <div className="space-y-2 text-sm">
            <div className="flex justify-between">
              <span className="text-gray-600">Transaction Hash:</span>
              <div className="flex items-center gap-1">
                <span className="font-mono text-xs">
                  {transactionHash.slice(0, 10)}...{transactionHash.slice(-8)}
                </span>
                <Button
                  isIconOnly
                  size="sm"
                  variant="light"
                  onPress={openEtherscan}
                >
                  <ExternalLink size={12} />
                </Button>
              </div>
            </div>

            {blockNumber && (
              <div className="flex justify-between">
                <span className="text-gray-600">Block Number:</span>
                <span className="font-mono">{blockNumber.toLocaleString()}</span>
              </div>
            )}

            {gasUsed && (
              <div className="flex justify-between">
                <span className="text-gray-600">Gas Used:</span>
                <span className="font-mono">{gasUsed.toLocaleString()}</span>
              </div>
            )}
          </div>
        )}

        {/* Error Details */}
        {status === 'failed' && error && (
          <Card className="bg-danger-100 border-danger-200">
            <CardBody>
              <div className="flex items-start gap-2">
                <AlertCircle className="text-danger-600 mt-0.5" size={16} />
                <div className="text-sm text-danger-700">
                  <p className="font-medium mb-1">Error Details:</p>
                  <p className="text-xs font-mono bg-danger-200 p-2 rounded">
                    {error}
                  </p>
                </div>
              </div>
            </CardBody>
          </Card>
        )}

        {/* Action Buttons */}
        <div className="flex gap-2">
          {transactionHash && (
            <Button
              variant="bordered"
              startContent={<ExternalLink size={16} />}
              onPress={openEtherscan}
              size="sm"
            >
              View on Etherscan
            </Button>
          )}

          {status === 'failed' && onRetry && (
            <Button
              color="primary"
              startContent={<RefreshCw size={16} />}
              onPress={onRetry}
              size="sm"
            >
              Retry Transaction
            </Button>
          )}
        </div>

        {/* Loading Animation for Pending States */}
        {(status === 'pending' || status === 'confirming') && (
          <div className="flex items-center justify-center py-2">
            <div className="flex items-center gap-2 text-sm text-gray-600">
              <Spinner size="sm" />
              <span>
                {status === 'pending' ? 'Waiting for network confirmation...' : 'Confirming transaction...'}
              </span>
            </div>
          </div>
        )}
      </CardBody>
    </Card>
  );
}
