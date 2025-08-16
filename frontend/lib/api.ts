import axios from 'axios';

const API_BASE_URL = process.env.NEXT_PUBLIC_API_URL || 'http://localhost:8080';

export const api = axios.create({
  baseURL: `${API_BASE_URL}/api/v1`,
  headers: {
    'Content-Type': 'application/json',
  },
});

export interface CreateInvoiceRequest {
  creator: string;
  creator_name?: string;
  title: string;
  description?: string;
  amount: number; // Amount in USDC (will be converted to wei)
  payer_email?: string;
  payer_name?: string;
  due_date?: string;
}

export interface Invoice {
  id: number;
  invoice_id: number;
  creator: string;
  creator_name?: string;
  title: string;
  description?: string;
  amount: number;
  amount_paid: number;
  payer_email?: string;
  payer_name?: string;
  due_date?: string;
  status: 'pending' | 'partially_paid' | 'paid' | 'cancelled';
  metadata_uri: string;
  payment_link: string;
  qr_code_url: string;
  tx_hash?: string;
  is_active: boolean;
  created_at: string;
  updated_at: string;
  amount_formatted: string;
  payment_status: string;
}

export interface Payment {
  id: number;
  invoice_id: number;
  payer: string;
  amount: number;
  fee: number;
  tx_hash: string;
  block_number: number;
  created_at: string;
  updated_at: string;
}

// API functions
export const invoiceAPI = {
  create: (data: CreateInvoiceRequest): Promise<Invoice> =>
    api.post('/invoices', {
      ...data,
      amount: Math.floor(data.amount * 1000000), // Convert to USDC wei (6 decimals)
    }).then(res => res.data),

  getById: (id: number): Promise<Invoice> =>
    api.get(`/invoices/${id}`).then(res => res.data),

  getByCreator: (creator: string): Promise<Invoice[]> =>
    api.get(`/invoices?creator=${creator}`).then(res => res.data),

  getPayments: (id: number): Promise<Payment[]> =>
    api.get(`/invoices/${id}/payments`).then(res => res.data),

  cancel: (id: number, creator: string): Promise<void> =>
    api.delete(`/invoices/${id}?creator=${creator}`).then(res => res.data),

  getMetadata: (id: number): Promise<any> =>
    api.get(`/invoices/${id}/metadata`).then(res => res.data),
};

// WebSocket connection
export class WebSocketManager {
  private ws: WebSocket | null = null;
  private listeners: Map<string, Function[]> = new Map();

  connect() {
    const wsUrl = process.env.NEXT_PUBLIC_WS_URL || 'ws://localhost:8080/ws';
    this.ws = new WebSocket(wsUrl);

    this.ws.onopen = () => {
      console.log('WebSocket connected');
    };

    this.ws.onmessage = (event) => {
      try {
        const message = JSON.parse(event.data);
        this.emit(message.type, message.data);
      } catch (error) {
        console.error('Failed to parse WebSocket message:', error);
      }
    };

    this.ws.onclose = () => {
      console.log('WebSocket disconnected');
      // Attempt to reconnect after 3 seconds
      setTimeout(() => this.connect(), 3000);
    };

    this.ws.onerror = (error) => {
      console.error('WebSocket error:', error);
    };
  }

  disconnect() {
    if (this.ws) {
      this.ws.close();
      this.ws = null;
    }
  }

  on(event: string, callback: Function) {
    if (!this.listeners.has(event)) {
      this.listeners.set(event, []);
    }
    this.listeners.get(event)!.push(callback);
  }

  off(event: string, callback: Function) {
    const listeners = this.listeners.get(event);
    if (listeners) {
      const index = listeners.indexOf(callback);
      if (index > -1) {
        listeners.splice(index, 1);
      }
    }
  }

  private emit(event: string, data: any) {
    const listeners = this.listeners.get(event);
    if (listeners) {
      listeners.forEach(callback => callback(data));
    }
  }
}

export const wsManager = new WebSocketManager();
