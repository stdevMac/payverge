'use client';

import { useEffect, useRef, useState } from 'react';

interface PaymentNotification {
  type: 'payment_received' | 'bill_update';
  bill_id: number;
  transaction_hash?: string;
  amount?: number;
  tip_amount?: number;
  payer_address?: string;
  business_id?: number;
  status: string;
  paid_amount?: number;
  total?: number;
  remaining?: number;
  timestamp: string;
}

interface UsePaymentWebSocketOptions {
  billId?: number;
  tableCode?: string;
  businessId?: number;
  onPaymentReceived?: (notification: PaymentNotification) => void;
  onBillUpdate?: (notification: PaymentNotification) => void;
}

export function usePaymentWebSocket({
  billId,
  tableCode,
  businessId,
  onPaymentReceived,
  onBillUpdate
}: UsePaymentWebSocketOptions) {
  const [isConnected, setIsConnected] = useState(false);
  const [lastMessage, setLastMessage] = useState<PaymentNotification | null>(null);
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
        console.log('Payment WebSocket connected');
        setIsConnected(true);
        reconnectAttempts.current = 0;

        // Subscribe to relevant rooms
        if (billId) {
          wsRef.current?.send(JSON.stringify({
            type: 'subscribe',
            room: `bill_${billId}`
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
          const notification: PaymentNotification = JSON.parse(event.data);
          setLastMessage(notification);

          // Handle different notification types
          switch (notification.type) {
            case 'payment_received':
              onPaymentReceived?.(notification);
              break;
            case 'bill_update':
              onBillUpdate?.(notification);
              break;
          }
        } catch (error) {
          console.error('Failed to parse WebSocket message:', error);
        }
      };

      wsRef.current.onclose = (event) => {
        console.log('Payment WebSocket disconnected:', event.code, event.reason);
        setIsConnected(false);

        // Attempt to reconnect if not a normal closure
        if (event.code !== 1000 && reconnectAttempts.current < maxReconnectAttempts) {
          const delay = Math.min(1000 * Math.pow(2, reconnectAttempts.current), 30000);
          console.log(`Attempting to reconnect in ${delay}ms...`);
          
          reconnectTimeoutRef.current = setTimeout(() => {
            reconnectAttempts.current++;
            connect();
          }, delay);
        }
      };

      wsRef.current.onerror = (error) => {
        console.error('Payment WebSocket error:', error);
      };

    } catch (error) {
      console.error('Failed to create WebSocket connection:', error);
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
  };

  const sendMessage = (message: any) => {
    if (wsRef.current?.readyState === WebSocket.OPEN) {
      wsRef.current.send(JSON.stringify(message));
    }
  };

  useEffect(() => {
    connect();
    return disconnect;
  }, [billId, tableCode, businessId]);

  return {
    isConnected,
    lastMessage,
    sendMessage,
    connect,
    disconnect
  };
}
