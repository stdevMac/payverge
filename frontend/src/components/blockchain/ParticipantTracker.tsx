'use client';

import React, { useState, useEffect, useCallback } from 'react';
import {
  Card,
  CardBody,
  CardHeader,
  Chip,
  Avatar,
  Progress,
  Button,
  Spinner,
} from '@nextui-org/react';
import { Users, Wallet, RefreshCw, Eye } from 'lucide-react';
import { useBlockchainAPI, Participant, formatAddress, formatUSDCAmount } from '../../api/blockchain';

interface ParticipantTrackerProps {
  billId: number;
  totalAmount: number;
  onParticipantClick?: (address: string) => void;
  refreshInterval?: number; // in milliseconds
  className?: string;
}

export const ParticipantTracker: React.FC<ParticipantTrackerProps> = ({
  billId,
  totalAmount,
  onParticipantClick,
  refreshInterval = 10000, // 10 seconds default
  className = '',
}) => {
  const [participants, setParticipants] = useState<string[]>([]);
  const [participantDetails, setParticipantDetails] = useState<Record<string, Participant>>({});
  const [loading, setLoading] = useState(true);
  const [refreshing, setRefreshing] = useState(false);
  const [totalPaid, setTotalPaid] = useState(0);
  const [participantCount, setParticipantCount] = useState(0);

  const blockchainAPI = useBlockchainAPI();

  const loadParticipants = useCallback(async (showRefreshing = false) => {
    if (showRefreshing) setRefreshing(true);
    try {
      // Get participants list
      const participantsResponse = await blockchainAPI.getBillParticipants(billId);
      if (participantsResponse.success) {
        setParticipants(participantsResponse.participants);
        setParticipantCount(participantsResponse.count);

        // Get details for each participant
        const details: Record<string, Participant> = {};
        let totalPaidAmount = 0;

        for (const address of participantsResponse.participants) {
          try {
            const participantResponse = await blockchainAPI.getParticipantInfo(billId, address);
            if (participantResponse.success) {
              details[address] = participantResponse.participant;
              totalPaidAmount += participantResponse.participant.paid_amount;
            }
          } catch (error) {
            console.error(`Error loading participant ${address}:`, error);
          }
        }

        setParticipantDetails(details);
        setTotalPaid(totalPaidAmount);
      }
    } catch (error) {
      console.error('Error loading participants:', error);
    } finally {
      setLoading(false);
      if (showRefreshing) setRefreshing(false);
    }
  }, [billId, blockchainAPI]);

  const handleRefresh = () => {
    loadParticipants(true);
  };

  useEffect(() => {
    loadParticipants();

    // Set up auto-refresh
    if (refreshInterval > 0) {
      const interval = setInterval(() => {
        loadParticipants();
      }, refreshInterval);

      return () => clearInterval(interval);
    }
  }, [billId, refreshInterval, loadParticipants]);

  const progressPercentage = totalAmount > 0 ? (totalPaid / totalAmount) * 100 : 0;
  const remainingAmount = totalAmount - totalPaid;
  const isFullyPaid = progressPercentage >= 100;

  // Don't show the component if:
  // 1. Still loading, OR
  // 2. Less than 2 participants, OR  
  // 3. Bill is fully paid
  if (loading) {
    return (
      <Card className={`${className}`}>
        <CardBody className="flex items-center justify-center py-8">
          <Spinner size="lg" />
          <p className="mt-4 text-gray-600">Loading participants...</p>
        </CardBody>
      </Card>
    );
  }

  // Hide component if conditions aren't met
  if (participantCount < 2 || isFullyPaid) {
    return null;
  }

  return (
    <Card className={`${className}`}>
      <CardHeader className="flex justify-between items-center">
        <div className="flex items-center gap-3">
          <div className="w-10 h-10 bg-blue-100 rounded-xl flex items-center justify-center">
            <Users className="w-5 h-5 text-blue-600" />
          </div>
          <div>
            <h3 className="text-lg font-semibold">Payment Participants</h3>
            <p className="text-sm text-gray-600">
              {participantCount} {participantCount === 1 ? 'person has' : 'people have'} contributed
            </p>
          </div>
        </div>
        <Button
          isIconOnly
          variant="light"
          size="sm"
          onClick={handleRefresh}
          isLoading={refreshing}
          className="text-gray-600 hover:text-gray-900"
        >
          <RefreshCw className="w-4 h-4" />
        </Button>
      </CardHeader>

      <CardBody className="space-y-6">
        {/* Payment Progress */}
        <div className="space-y-3">
          <div className="flex justify-between items-center">
            <span className="text-sm font-medium text-gray-700">Payment Progress</span>
            <span className="text-sm text-gray-600">
              ${formatUSDCAmount(totalPaid)} / ${formatUSDCAmount(totalAmount)}
            </span>
          </div>
          <Progress
            value={progressPercentage}
            className="w-full"
            color={progressPercentage >= 100 ? "success" : "primary"}
            size="md"
          />
          <div className="flex justify-between items-center text-xs text-gray-500">
            <span>{progressPercentage.toFixed(1)}% complete</span>
            {remainingAmount > 0 && (
              <span>Remaining: ${formatUSDCAmount(remainingAmount)}</span>
            )}
          </div>
        </div>

        {/* Participants List */}
        {participants.length > 0 ? (
          <div className="space-y-3">
            <h4 className="text-sm font-medium text-gray-700">Contributors</h4>
            <div className="space-y-2">
              {participants.map((address) => {
                const participant = participantDetails[address];
                return (
                  <div
                    key={address}
                    className="flex items-center justify-between p-3 bg-gray-50 rounded-xl hover:bg-gray-100 transition-colors cursor-pointer"
                    onClick={() => onParticipantClick?.(address)}
                  >
                    <div className="flex items-center gap-3">
                      <Avatar
                        size="sm"
                        name={formatAddress(address)}
                        className="bg-gradient-to-br from-blue-400 to-purple-500 text-white"
                      />
                      <div>
                        <p className="text-sm font-medium text-gray-900">
                          {formatAddress(address)}
                        </p>
                        {participant && (
                          <p className="text-xs text-gray-500">
                            {participant.payment_count} payment{participant.payment_count !== 1 ? 's' : ''}
                          </p>
                        )}
                      </div>
                    </div>
                    <div className="text-right">
                      {participant ? (
                        <>
                          <p className="text-sm font-semibold text-gray-900">
                            ${formatUSDCAmount(participant.paid_amount)}
                          </p>
                          <Chip size="sm" color="success" variant="flat">
                            Paid
                          </Chip>
                        </>
                      ) : (
                        <Spinner size="sm" />
                      )}
                    </div>
                  </div>
                );
              })}
            </div>
          </div>
        ) : (
          <div className="text-center py-8">
            <div className="w-16 h-16 bg-gray-100 rounded-2xl flex items-center justify-center mx-auto mb-4">
              <Wallet className="w-8 h-8 text-gray-400" />
            </div>
            <h4 className="text-lg font-medium text-gray-900 mb-2">No Payments Yet</h4>
            <p className="text-gray-600">
              Participants will appear here as they make payments
            </p>
          </div>
        )}

        {/* Bill Status */}
        {progressPercentage >= 100 && (
          <div className="bg-green-50 border border-green-200 rounded-xl p-4">
            <div className="flex items-center gap-3">
              <div className="w-8 h-8 bg-green-100 rounded-full flex items-center justify-center">
                <Users className="w-4 h-4 text-green-600" />
              </div>
              <div>
                <p className="text-sm font-semibold text-green-900">Bill Fully Paid!</p>
                <p className="text-xs text-green-700">
                  All payments have been received from {participantCount} contributor{participantCount !== 1 ? 's' : ''}
                </p>
              </div>
            </div>
          </div>
        )}
      </CardBody>
    </Card>
  );
};

export default ParticipantTracker;
