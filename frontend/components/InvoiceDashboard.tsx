'use client';

import { useState, useEffect } from 'react';
import { useQuery } from '@tanstack/react-query';
import { format } from 'date-fns';
import { 
  ExternalLink, 
  Copy, 
  QrCode, 
  Mail, 
  Calendar, 
  DollarSign,
  Eye,
  X
} from 'lucide-react';
import { toast } from 'react-hot-toast';
import { invoiceAPI, Invoice, wsManager } from '../lib/api';

interface InvoiceDashboardProps {
  creatorAddress: string;
}

export function InvoiceDashboard({ creatorAddress }: InvoiceDashboardProps) {
  const [selectedInvoice, setSelectedInvoice] = useState<Invoice | null>(null);

  const { data: invoices, isLoading, refetch } = useQuery({
    queryKey: ['invoices', creatorAddress],
    queryFn: () => invoiceAPI.getByCreator(creatorAddress),
    enabled: !!creatorAddress,
  });

  // Listen for real-time updates
  useEffect(() => {
    const handleInvoiceUpdate = (data: any) => {
      // Refetch invoices when we get updates
      refetch();
      toast.success(`Invoice #${data.invoice_id} updated: ${data.status}`);
    };

    const handlePaymentReceived = (data: any) => {
      refetch();
      toast.success(`Payment received for Invoice #${data.invoice_id}!`);
    };

    wsManager.on('invoice_update', handleInvoiceUpdate);
    wsManager.on('payment_received', handlePaymentReceived);

    return () => {
      wsManager.off('invoice_update', handleInvoiceUpdate);
      wsManager.off('payment_received', handlePaymentReceived);
    };
  }, [refetch]);

  const copyToClipboard = (text: string, label: string) => {
    navigator.clipboard.writeText(text);
    toast.success(`${label} copied to clipboard!`);
  };

  const getStatusBadge = (status: string) => {
    switch (status) {
      case 'pending':
        return <span className="status-pending">Pending</span>;
      case 'partially_paid':
        return <span className="status-partially-paid">Partially Paid</span>;
      case 'paid':
        return <span className="status-paid">Paid</span>;
      case 'cancelled':
        return <span className="status-cancelled">Cancelled</span>;
      default:
        return <span className="status-pending">{status}</span>;
    }
  };

  if (isLoading) {
    return (
      <div className="text-center py-12">
        <div className="animate-spin rounded-full h-12 w-12 border-b-2 border-primary-600 mx-auto"></div>
        <p className="mt-4 text-gray-600">Loading your invoices...</p>
      </div>
    );
  }

  if (!invoices || invoices.length === 0) {
    return (
      <div className="text-center py-12">
        <DollarSign className="h-12 w-12 text-gray-400 mx-auto mb-4" />
        <h3 className="text-lg font-medium text-gray-900 mb-2">
          No invoices yet
        </h3>
        <p className="text-gray-600">
          Create your first invoice to start getting paid in USDC
        </p>
      </div>
    );
  }

  return (
    <div>
      <div className="mb-6">
        <h2 className="text-2xl font-bold text-gray-900 mb-2">
          Your Invoices
        </h2>
        <p className="text-gray-600">
          Manage and track all your USDC invoices
        </p>
      </div>

      <div className="grid gap-6">
        {invoices.map((invoice) => (
          <div key={invoice.id} className="card hover:shadow-md transition-shadow">
            <div className="flex items-start justify-between">
              <div className="flex-1">
                <div className="flex items-center space-x-3 mb-2">
                  <h3 className="text-lg font-semibold text-gray-900">
                    {invoice.title}
                  </h3>
                  {getStatusBadge(invoice.status)}
                </div>
                
                <div className="grid md:grid-cols-2 gap-4 text-sm text-gray-600">
                  <div>
                    <p><strong>Invoice ID:</strong> #{invoice.invoice_id}</p>
                    <p><strong>Amount:</strong> ${invoice.amount_formatted} USDC</p>
                    {invoice.amount_paid > 0 && (
                      <p><strong>Paid:</strong> ${(invoice.amount_paid / 1000000).toFixed(2)} USDC</p>
                    )}
                  </div>
                  <div>
                    <p><strong>Created:</strong> {format(new Date(invoice.created_at), 'MMM d, yyyy')}</p>
                    {invoice.due_date && (
                      <p><strong>Due:</strong> {format(new Date(invoice.due_date), 'MMM d, yyyy')}</p>
                    )}
                    {invoice.payer_name && (
                      <p><strong>Client:</strong> {invoice.payer_name}</p>
                    )}
                  </div>
                </div>

                {invoice.description && (
                  <p className="mt-2 text-sm text-gray-600 line-clamp-2">
                    {invoice.description}
                  </p>
                )}
              </div>

              <div className="flex items-center space-x-2 ml-4">
                <button
                  onClick={() => setSelectedInvoice(invoice)}
                  className="btn-secondary text-xs"
                  title="View Details"
                >
                  <Eye className="h-4 w-4" />
                </button>
              </div>
            </div>

            <div className="mt-4 pt-4 border-t border-gray-200">
              <div className="flex items-center justify-between">
                <div className="flex items-center space-x-4">
                  <button
                    onClick={() => copyToClipboard(invoice.payment_link, 'Payment link')}
                    className="flex items-center space-x-1 text-sm text-primary-600 hover:text-primary-700"
                  >
                    <Copy className="h-4 w-4" />
                    <span>Copy Link</span>
                  </button>
                  
                  <a
                    href={invoice.payment_link}
                    target="_blank"
                    rel="noopener noreferrer"
                    className="flex items-center space-x-1 text-sm text-primary-600 hover:text-primary-700"
                  >
                    <ExternalLink className="h-4 w-4" />
                    <span>Open</span>
                  </a>

                  {invoice.qr_code_url && (
                    <a
                      href={invoice.qr_code_url}
                      target="_blank"
                      rel="noopener noreferrer"
                      className="flex items-center space-x-1 text-sm text-primary-600 hover:text-primary-700"
                    >
                      <QrCode className="h-4 w-4" />
                      <span>QR Code</span>
                    </a>
                  )}
                </div>

                {invoice.tx_hash && (
                  <a
                    href={`https://etherscan.io/tx/${invoice.tx_hash}`}
                    target="_blank"
                    rel="noopener noreferrer"
                    className="text-xs text-gray-500 hover:text-gray-700"
                  >
                    View on Etherscan
                  </a>
                )}
              </div>
            </div>
          </div>
        ))}
      </div>

      {/* Invoice Detail Modal */}
      {selectedInvoice && (
        <InvoiceDetailModal
          invoice={selectedInvoice}
          onClose={() => setSelectedInvoice(null)}
        />
      )}
    </div>
  );
}

function InvoiceDetailModal({ invoice, onClose }: { invoice: Invoice; onClose: () => void }) {
  const { data: payments } = useQuery({
    queryKey: ['payments', invoice.invoice_id],
    queryFn: () => invoiceAPI.getPayments(invoice.invoice_id),
  });

  return (
    <div className="fixed inset-0 bg-black bg-opacity-50 flex items-center justify-center p-4 z-50">
      <div className="bg-white rounded-lg max-w-2xl w-full max-h-[90vh] overflow-y-auto">
        <div className="p-6">
          <div className="flex items-center justify-between mb-6">
            <h2 className="text-2xl font-bold text-gray-900">
              Invoice #{invoice.invoice_id}
            </h2>
            <button
              onClick={onClose}
              className="text-gray-400 hover:text-gray-600"
            >
              <X className="h-6 w-6" />
            </button>
          </div>

          <div className="space-y-6">
            {/* Invoice Details */}
            <div>
              <h3 className="text-lg font-semibold text-gray-900 mb-3">Details</h3>
              <div className="grid md:grid-cols-2 gap-4">
                <div>
                  <p className="text-sm text-gray-600 mb-1">Title</p>
                  <p className="font-medium">{invoice.title}</p>
                </div>
                <div>
                  <p className="text-sm text-gray-600 mb-1">Status</p>
                  <div>{getStatusBadge(invoice.status)}</div>
                </div>
                <div>
                  <p className="text-sm text-gray-600 mb-1">Amount</p>
                  <p className="font-medium">${invoice.amount_formatted} USDC</p>
                </div>
                <div>
                  <p className="text-sm text-gray-600 mb-1">Amount Paid</p>
                  <p className="font-medium">${(invoice.amount_paid / 1000000).toFixed(2)} USDC</p>
                </div>
              </div>
              
              {invoice.description && (
                <div className="mt-4">
                  <p className="text-sm text-gray-600 mb-1">Description</p>
                  <p className="text-gray-900">{invoice.description}</p>
                </div>
              )}
            </div>

            {/* Payment History */}
            {payments && payments.length > 0 && (
              <div>
                <h3 className="text-lg font-semibold text-gray-900 mb-3">Payment History</h3>
                <div className="space-y-3">
                  {payments.map((payment) => (
                    <div key={payment.id} className="bg-gray-50 rounded-lg p-4">
                      <div className="flex items-center justify-between">
                        <div>
                          <p className="font-medium">${(payment.amount / 1000000).toFixed(2)} USDC</p>
                          <p className="text-sm text-gray-600">
                            From: {payment.payer.slice(0, 6)}...{payment.payer.slice(-4)}
                          </p>
                          <p className="text-sm text-gray-600">
                            {format(new Date(payment.created_at), 'MMM d, yyyy HH:mm')}
                          </p>
                        </div>
                        <a
                          href={`https://etherscan.io/tx/${payment.tx_hash}`}
                          target="_blank"
                          rel="noopener noreferrer"
                          className="text-sm text-primary-600 hover:text-primary-700"
                        >
                          View Transaction
                        </a>
                      </div>
                    </div>
                  ))}
                </div>
              </div>
            )}

            {/* Sharing Options */}
            <div>
              <h3 className="text-lg font-semibold text-gray-900 mb-3">Share Invoice</h3>
              <div className="space-y-3">
                <div className="flex items-center space-x-3">
                  <input
                    type="text"
                    value={invoice.payment_link}
                    readOnly
                    className="input flex-1 text-sm"
                  />
                  <button
                    onClick={() => navigator.clipboard.writeText(invoice.payment_link)}
                    className="btn-secondary"
                  >
                    <Copy className="h-4 w-4" />
                  </button>
                </div>
                
                {invoice.qr_code_url && (
                  <div className="text-center">
                    <img
                      src={invoice.qr_code_url}
                      alt="Payment QR Code"
                      className="mx-auto max-w-[200px]"
                    />
                    <p className="text-sm text-gray-600 mt-2">
                      QR Code for mobile payments
                    </p>
                  </div>
                )}
              </div>
            </div>
          </div>
        </div>
      </div>
    </div>
  );

  function getStatusBadge(status: string) {
    switch (status) {
      case 'pending':
        return <span className="status-pending">Pending</span>;
      case 'partially_paid':
        return <span className="status-partially-paid">Partially Paid</span>;
      case 'paid':
        return <span className="status-paid">Paid</span>;
      case 'cancelled':
        return <span className="status-cancelled">Cancelled</span>;
      default:
        return <span className="status-pending">{status}</span>;
    }
  }
}
