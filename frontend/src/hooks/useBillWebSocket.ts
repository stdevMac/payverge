import { useCallback } from 'react';
import { useWebSocket, WebSocketMessage } from './useWebSocket';
import { BillResponse } from '../api/bills';

interface UseBillWebSocketOptions {
  tableCode?: string;
  billId?: number;
  onBillUpdate?: (bill: BillResponse) => void;
  onBillClosed?: (billId: number) => void;
  onPaymentReceived?: (payment: any) => void;
  onItemAdded?: (item: any) => void;
  onItemRemoved?: (itemId: string) => void;
}

export const useBillWebSocket = ({
  tableCode,
  billId,
  onBillUpdate,
  onBillClosed,
  onPaymentReceived,
  onItemAdded,
  onItemRemoved
}: UseBillWebSocketOptions) => {
  const wsUrl = process.env.NEXT_PUBLIC_WS_URL || 'ws://localhost:8080/ws';

  const handleMessage = useCallback((message: WebSocketMessage) => {
    switch (message.type) {
      case 'bill_updated':
        onBillUpdate?.(message.data);
        break;
      case 'bill_closed':
        onBillClosed?.(message.data.billId);
        break;
      case 'payment_received':
        onPaymentReceived?.(message.data);
        break;
      case 'item_added':
        onItemAdded?.(message.data);
        break;
      case 'item_removed':
        onItemRemoved?.(message.data.itemId);
        break;
      default:
        console.log('Unknown WebSocket message type:', message.type);
    }
  }, [onBillUpdate, onBillClosed, onPaymentReceived, onItemAdded, onItemRemoved]);

  const { isConnected, sendMessage, disconnect, reconnect } = useWebSocket({
    url: wsUrl,
    tableCode,
    billId,
    onMessage: handleMessage,
    onConnect: () => {
      console.log('Bill WebSocket connected for table:', tableCode);
    },
    onDisconnect: () => {
      console.log('Bill WebSocket disconnected for table:', tableCode);
    },
    onError: (error) => {
      console.error('Bill WebSocket error:', error);
    }
  });

  const subscribeToTable = useCallback((code: string) => {
    sendMessage({
      type: 'subscribe_table',
      tableCode: code
    });
  }, [sendMessage]);

  const subscribeToBill = useCallback((id: number) => {
    sendMessage({
      type: 'subscribe_bill',
      billId: id
    });
  }, [sendMessage]);

  const unsubscribeFromTable = useCallback((code: string) => {
    sendMessage({
      type: 'unsubscribe_table',
      tableCode: code
    });
  }, [sendMessage]);

  const unsubscribeFromBill = useCallback((id: number) => {
    sendMessage({
      type: 'unsubscribe_bill',
      billId: id
    });
  }, [sendMessage]);

  return {
    isConnected,
    subscribeToTable,
    subscribeToBill,
    unsubscribeFromTable,
    unsubscribeFromBill,
    disconnect,
    reconnect
  };
};
