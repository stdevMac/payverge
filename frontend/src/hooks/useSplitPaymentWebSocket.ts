'use client';

import { useEffect, useRef, useState } from 'react';

interface SplitPaymentNotification {
  type: 'split_payment_received' | 'split_payment_update' | 'bill_update';
  bill_id: number;
  split_id?: string;
  person_id?: string;
  person_name?: string;
  transaction_hash?: string;
  amount?: number;
  tip_amount?: number;
  total_amount?: number;
  payer_address?: string;
  business_id?: number;
  status: 'pending' | 'processing' | 'completed' | 'failed';
  paid_amount?: number;
  remaining_amount?: number;
  split_progress?: {
    total_people: number;
    completed_payments: number;
    total_paid: number;
    total_remaining: number;
  };
  timestamp: string;
}

interface UseSplitPaymentWebSocketOptions {
  billId?: number;
  splitId?: string;
  tableCode?: string;
  businessId?: number;
  onSplitPaymentReceived?: (notification: SplitPaymentNotification) => void;
  onSplitPaymentUpdate?: (notification: SplitPaymentNotification) => void;
  onBillUpdate?: (notification: SplitPaymentNotification) => void;
}

export function useSplitPaymentWebSocket({
  billId,
  splitId,
  tableCode,
  businessId,
  onSplitPaymentReceived,
  onSplitPaymentUpdate,
  onBillUpdate
}: UseSplitPaymentWebSocketOptions) {
  const [isConnected, setIsConnected] = useState(false);
  const [lastMessage, setLastMessage] = useState<SplitPaymentNotification | null>(null);
  const [splitProgress, setSplitProgress] = useState<SplitPaymentNotification['split_progress'] | null>(null);
  const wsRef = useRef<WebSocket | null>(null);
  const reconnectTimeoutRef = useRef<NodeJS.Timeout>();
  const reconnectAttempts = useRef(0);
  const maxReconnectAttempts = 5;

  const connect = () => {
    if (wsRef.current?.readyState === WebSocket.OPEN) {
      return;
    }

    try {
      const wsUrl = process.env.NEXT_PUBLIC_WS_URL || 'ws://localhost:8080/api/v1/ws';
      wsRef.current = new WebSocket(wsUrl);

      wsRef.current.onopen = () => {
        setIsConnected(true);
        reconnectAttempts.current = 0;

        // Subscribe to relevant rooms for split payments
        if (billId) {
          wsRef.current?.send(JSON.stringify({
            type: 'subscribe',
            room: `bill_${billId}`
          }));
          
          // Subscribe to split-specific room
          wsRef.current?.send(JSON.stringify({
            type: 'subscribe',
            room: `bill_${billId}_split`
          }));
        }
        
        if (splitId) {
          wsRef.current?.send(JSON.stringify({
            type: 'subscribe',
            room: `split_${splitId}`
          }));
        }
        
        if (tableCode) {
          wsRef.current?.send(JSON.stringify({
            type: 'subscribe',
            room: `table_${tableCode}`
          }));
        }
        
        if (businessId) {
          wsRef.current?.send(JSON.stringify({
            type: 'subscribe',
            room: `business_${businessId}`
          }));
        }
      };

      wsRef.current.onmessage = (event) => {
        try {
          const notification: SplitPaymentNotification = JSON.parse(event.data);
          setLastMessage(notification);

          // Update split progress if available
          if (notification.split_progress) {
            setSplitProgress(notification.split_progress);
          }

          // Handle different notification types
          switch (notification.type) {
            case 'split_payment_received':
              onSplitPaymentReceived?.(notification);
              break;
            case 'split_payment_update':
              onSplitPaymentUpdate?.(notification);
              break;
            case 'bill_update':
              onBillUpdate?.(notification);
              break;
          }
        } catch (error) {
          // Silently handle parse errors
        }
      };

      wsRef.current.onclose = (event) => {
        setIsConnected(false);

        // Attempt to reconnect if not a normal closure
        if (event.code !== 1000 && reconnectAttempts.current < maxReconnectAttempts) {
          const delay = Math.min(1000 * Math.pow(2, reconnectAttempts.current), 30000);
          
          reconnectTimeoutRef.current = setTimeout(() => {
            reconnectAttempts.current++;
            connect();
          }, delay);
        }
      };

      wsRef.current.onerror = (error) => {
        // Handle WebSocket errors silently
      };

    } catch (error) {
      // Handle connection errors silently
    }
  };

  const disconnect = () => {
    if (reconnectTimeoutRef.current) {
      clearTimeout(reconnectTimeoutRef.current);
    }
    
    if (wsRef.current) {
      wsRef.current.close(1000, 'Component unmounted');
      wsRef.current = null;
    }
    
    setIsConnected(false);
    setSplitProgress(null);
  };

  const sendMessage = (message: any) => {
    if (wsRef.current?.readyState === WebSocket.OPEN) {
      wsRef.current.send(JSON.stringify(message));
    }
  };

  const notifyPaymentStarted = (personId: string, personName: string, amount: number, tipAmount: number) => {
    sendMessage({
      type: 'split_payment_started',
      bill_id: billId,
      split_id: splitId,
      person_id: personId,
      person_name: personName,
      amount,
      tip_amount: tipAmount,
      total_amount: amount + tipAmount,
      timestamp: new Date().toISOString()
    });
  };

  const notifyPaymentCompleted = (personId: string, transactionHash: string, amount: number, tipAmount: number) => {
    sendMessage({
      type: 'split_payment_completed',
      bill_id: billId,
      split_id: splitId,
      person_id: personId,
      transaction_hash: transactionHash,
      amount,
      tip_amount: tipAmount,
      total_amount: amount + tipAmount,
      timestamp: new Date().toISOString()
    });
  };

  const notifyPaymentFailed = (personId: string, error: string) => {
    sendMessage({
      type: 'split_payment_failed',
      bill_id: billId,
      split_id: splitId,
      person_id: personId,
      error,
      timestamp: new Date().toISOString()
    });
  };

  useEffect(() => {
    connect();
    return disconnect;
  }, [billId, splitId, tableCode, businessId]);

  return {
    isConnected,
    lastMessage,
    splitProgress,
    sendMessage,
    notifyPaymentStarted,
    notifyPaymentCompleted,
    notifyPaymentFailed,
    connect,
    disconnect
  };
}
