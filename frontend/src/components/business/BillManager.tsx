import React, { useState, useEffect, useCallback } from 'react';
import { Plus } from 'lucide-react';
import { getBill, getBusinessBills, BillWithItemsResponse, closeBill, Bill } from '../../api/bills';
import { BillCreator } from './BillCreator';
import { getOrdersByBillId, updateOrderStatus, Order } from '../../api/orders';
import PaymentProcessor from '../payment/PaymentProcessor';
import BillSplittingFlow from '../splitting/BillSplittingFlow';
import { useSimpleLocale, getTranslation } from '@/i18n/SimpleTranslationProvider';

// Import the new smaller components
import { BillsTable } from './BillsTable';
import { BillFilters } from './BillFilters';
import { PendingOrdersSection } from './PendingOrdersSection';
import { BillDetailsModal } from './BillDetailsModal';

interface BillManagerProps {
  businessId: number;
}

export const BillManager: React.FC<BillManagerProps> = ({ businessId }) => {
  const { locale: currentLocale } = useSimpleLocale();

  // Translation helper
  const tString = (key: string): string => {
    const fullKey = `billManager.${key}`;
    const result = getTranslation(fullKey, currentLocale);
    return Array.isArray(result) ? result[0] || key : result as string;
  };

  // State
  const [bills, setBills] = useState<Bill[]>([]);
  const [loading, setLoading] = useState(true);
  const [selectedBill, setSelectedBill] = useState<BillWithItemsResponse | null>(null);
  const [showBillDetails, setShowBillDetails] = useState(false);
  const [showBillCreator, setShowBillCreator] = useState(false);
  const [showPaymentProcessor, setShowPaymentProcessor] = useState(false);
  const [showBillSplitting, setShowBillSplitting] = useState(false);
  const [orders, setOrders] = useState<Record<number, Order[]>>({});
  const [actionLoading, setActionLoading] = useState<number | null>(null);
  const [activeTab, setActiveTab] = useState<string>('active');

  // Filter states
  const [activeSearchQuery, setActiveSearchQuery] = useState('');
  const [activeDateFrom, setActiveDateFrom] = useState<string>('');
  const [activeDateTo, setActiveDateTo] = useState<string>('');
  const [historySearchQuery, setHistorySearchQuery] = useState('');
  const [historyStatusFilter, setHistoryStatusFilter] = useState<'all' | 'paid' | 'closed'>('all');
  const [historyDateFrom, setHistoryDateFrom] = useState<string>('');
  const [historyDateTo, setHistoryDateTo] = useState<string>('');

  // Data loading functions
  const loadOrdersForBills = useCallback(async (billsList: Bill[]) => {
    const ordersMap: Record<number, Order[]> = {};
    const billsToLoad = billsList.slice(0, 50);
    
    for (const bill of billsToLoad) {
      try {
        const orderData = await getOrdersByBillId(businessId, bill.id);
        ordersMap[bill.id] = orderData.orders || [];
      } catch (error) {
        ordersMap[bill.id] = [];
      }
    }
    
    setOrders(ordersMap);
  }, [businessId]);

  const loadBills = useCallback(async () => {
    setLoading(true);
    try {
      const data = await getBusinessBills(businessId);
      const billsList = data.bills || [];
      setBills(billsList);
      await loadOrdersForBills(billsList);
    } catch (error) {
      console.error('Error loading bills:', error);
    } finally {
      setLoading(false);
    }
  }, [businessId, loadOrdersForBills]);

  useEffect(() => {
    if (businessId) {
      loadBills();
    }
  }, [businessId, loadBills]);

  // Event handlers
  const handleViewBill = async (billId: number) => {
    try {
      const billDetails = await getBill(billId);
      setSelectedBill(billDetails);
      setShowBillDetails(true);
    } catch (error) {
      console.error('Error loading bill details:', error);
    }
  };

  const handleCloseBill = async (billId: number) => {
    setActionLoading(billId);
    try {
      await closeBill(billId);
      await loadBills();
    } catch (error) {
      console.error('Error closing bill:', error);
    } finally {
      setActionLoading(null);
    }
  };

  const handleApproveOrder = async (orderId: number) => {
    setActionLoading(orderId);
    try {
      await updateOrderStatus(businessId, orderId, {
        status: 'approved',
        approved_by: 'staff'
      });
      await loadBills();
    } catch (error) {
      console.error('Error approving order:', error);
    } finally {
      setActionLoading(null);
    }
  };

  const handleRejectOrder = async (orderId: number) => {
    setActionLoading(orderId);
    try {
      await updateOrderStatus(businessId, orderId, {
        status: 'cancelled',
        approved_by: 'staff'
      });
      await loadBills();
    } catch (error) {
      console.error('Error rejecting order:', error);
    } finally {
      setActionLoading(null);
    }
  };

  // Filter functions
  const resetActiveFilters = () => {
    setActiveSearchQuery('');
    setActiveDateFrom('');
    setActiveDateTo('');
  };

  const resetHistoryFilters = () => {
    setHistorySearchQuery('');
    setHistoryStatusFilter('all');
    setHistoryDateFrom('');
    setHistoryDateTo('');
  };

  // Filtered lists
  const activeBills = React.useMemo(() => {
    let list = bills.filter((b) => b.status === 'open');

    const q = activeSearchQuery.trim().toLowerCase();
    if (q) {
      list = list.filter((b) => {
        const billNo = b.bill_number?.toLowerCase() || '';
        const tableStr = `table ${b.table_id}`.toLowerCase();
        const totalStr = String(b.total_amount);
        return billNo.includes(q) || tableStr.includes(q) || totalStr.includes(q);
      });
    }

    const from = activeDateFrom ? new Date(activeDateFrom) : null;
    const to = activeDateTo ? new Date(activeDateTo) : null;
    if (from || to) {
      list = list.filter((b) => {
        const created = new Date(b.created_at);
        const afterFrom = from ? created >= new Date(from.setHours(0, 0, 0, 0)) : true;
        const beforeTo = to ? created <= new Date(to.setHours(23, 59, 59, 999)) : true;
        return afterFrom && beforeTo;
      });
    }

    list.sort((a, b) => new Date(b.created_at).getTime() - new Date(a.created_at).getTime());
    return list;
  }, [bills, activeSearchQuery, activeDateFrom, activeDateTo]);

  const historyBills = React.useMemo(() => {
    let list = bills.filter((b) => b.status === 'paid' || b.status === 'closed');

    if (historyStatusFilter !== 'all') {
      list = list.filter((b) => b.status === historyStatusFilter);
    }

    const q = historySearchQuery.trim().toLowerCase();
    if (q) {
      list = list.filter((b) => {
        const billNo = b.bill_number?.toLowerCase() || '';
        const tableStr = `table ${b.table_id}`.toLowerCase();
        const totalStr = String(b.total_amount);
        return billNo.includes(q) || tableStr.includes(q) || totalStr.includes(q);
      });
    }

    const from = historyDateFrom ? new Date(historyDateFrom) : null;
    const to = historyDateTo ? new Date(historyDateTo) : null;
    if (from || to) {
      list = list.filter((b) => {
        const created = new Date(b.created_at);
        const afterFrom = from ? created >= new Date(from.setHours(0, 0, 0, 0)) : true;
        const beforeTo = to ? created <= new Date(to.setHours(23, 59, 59, 999)) : true;
        return afterFrom && beforeTo;
      });
    }

    list.sort((a, b) => new Date(b.created_at).getTime() - new Date(a.created_at).getTime());
    return list;
  }, [bills, historyStatusFilter, historySearchQuery, historyDateFrom, historyDateTo]);

  // Loading state
  if (loading) {
    return (
      <div className="p-4 max-w-6xl mx-auto">
        <div className="flex justify-center items-center py-16">
          <div className="text-center">
            <div className="w-16 h-16 bg-gray-50 rounded-2xl flex items-center justify-center mx-auto mb-6 border border-gray-100">
              <div className="w-8 h-8 border-2 border-gray-300 border-t-gray-900 rounded-full animate-spin"></div>
            </div>
            <p className="text-gray-600 font-light tracking-wide">{tString('loading')}</p>
          </div>
        </div>
      </div>
    );
  }

  return (
    <div className="p-4 max-w-6xl mx-auto">
      {/* Header */}
      <div className="flex justify-between items-center mb-8">
        <div>
          <h1 className="text-2xl font-light text-gray-900 tracking-wide">
            {tString('title')}
          </h1>
          <p className="text-gray-600 font-light text-sm mt-1">
            {tString('subtitle')}
          </p>
        </div>
        <button
          onClick={() => setShowBillCreator(true)}
          className="bg-gray-900 text-white px-6 py-3 rounded-lg font-medium hover:bg-gray-800 transition-colors flex items-center gap-2 shadow-sm"
        >
          <Plus className="w-4 h-4" />
          {tString('buttons.createBill')}
        </button>
      </div>

      {/* Pending Orders Section */}
      <PendingOrdersSection
        orders={orders}
        actionLoading={actionLoading}
        onApproveOrder={handleApproveOrder}
        onRejectOrder={handleRejectOrder}
        tString={tString}
      />

      {/* Tab Navigation */}
      <div className="bg-white border border-gray-200 rounded-lg shadow-sm">
        <div className="flex border-b border-gray-200">
          <button
            onClick={() => setActiveTab('active')}
            className={`px-6 py-4 text-sm font-medium transition-colors ${
              activeTab === 'active'
                ? 'border-b-2 border-gray-900 text-gray-900 bg-gray-50'
                : 'text-gray-600 hover:text-gray-900 hover:bg-gray-50'
            }`}
          >
            {tString('tabs.active')} ({activeBills.length})
          </button>
          <button
            onClick={() => setActiveTab('history')}
            className={`px-6 py-4 text-sm font-medium transition-colors ${
              activeTab === 'history'
                ? 'border-b-2 border-gray-900 text-gray-900 bg-gray-50'
                : 'text-gray-600 hover:text-gray-900 hover:bg-gray-50'
            }`}
          >
            {tString('tabs.history')} ({historyBills.length})
          </button>
        </div>

        {/* Tab Content */}
        <div className="p-6">
          {activeTab === 'active' ? (
            <div className="space-y-6">
              <BillFilters
                searchQuery={activeSearchQuery}
                onSearchChange={setActiveSearchQuery}
                dateFrom={activeDateFrom}
                onDateFromChange={setActiveDateFrom}
                dateTo={activeDateTo}
                onDateToChange={setActiveDateTo}
                onReset={resetActiveFilters}
                tString={tString}
              />
              <BillsTable
                bills={activeBills}
                isActive={true}
                actionLoading={actionLoading}
                onViewBill={handleViewBill}
                onCloseBill={handleCloseBill}
                onCreateBill={() => setShowBillCreator(true)}
                onResetFilters={resetActiveFilters}
                tString={tString}
              />
            </div>
          ) : (
            <div className="space-y-6">
              <BillFilters
                searchQuery={historySearchQuery}
                onSearchChange={setHistorySearchQuery}
                dateFrom={historyDateFrom}
                onDateFromChange={setHistoryDateFrom}
                dateTo={historyDateTo}
                onDateToChange={setHistoryDateTo}
                statusFilter={historyStatusFilter}
                onStatusFilterChange={setHistoryStatusFilter}
                onReset={resetHistoryFilters}
                tString={tString}
                showStatusFilter={true}
              />
              <BillsTable
                bills={historyBills}
                isActive={false}
                actionLoading={actionLoading}
                onViewBill={handleViewBill}
                onCloseBill={handleCloseBill}
                onCreateBill={() => setShowBillCreator(true)}
                onResetFilters={resetHistoryFilters}
                tString={tString}
              />
            </div>
          )}
        </div>
      </div>

      {/* Modals */}
      <BillDetailsModal
        isOpen={showBillDetails}
        onClose={() => setShowBillDetails(false)}
        bill={selectedBill}
        onCloseBill={handleCloseBill}
        onPayWithCrypto={() => setShowPaymentProcessor(true)}
        onSplitBill={() => setShowBillSplitting(true)}
        onPaymentMarked={() => {
          loadBills();
          if (selectedBill) {
            handleViewBill(selectedBill.bill.id);
          }
        }}
        tString={tString}
      />

      <BillCreator
        isOpen={showBillCreator}
        onClose={() => setShowBillCreator(false)}
        businessId={businessId}
        onBillCreated={loadBills}
      />

      {selectedBill && (
        <>
          <PaymentProcessor
            isOpen={showPaymentProcessor}
            onClose={() => setShowPaymentProcessor(false)}
            billId={selectedBill.bill.id}
            amount={selectedBill.bill.total_amount - selectedBill.bill.paid_amount}
            businessName="Business"
            businessAddress={selectedBill.bill.settlement_address}
            tipAddress={selectedBill.bill.tipping_address}
            onPaymentComplete={async () => {
              setShowPaymentProcessor(false);
              await loadBills();
              if (selectedBill) {
                await handleViewBill(selectedBill.bill.id);
              }
            }}
          />

          <BillSplittingFlow
            bill={{
              id: selectedBill.bill.id,
              billNumber: selectedBill.bill.bill_number,
              items: selectedBill.items,
              subtotal: selectedBill.bill.subtotal,
              taxAmount: selectedBill.bill.tax_amount,
              serviceFeeAmount: selectedBill.bill.service_fee_amount,
              totalAmount: selectedBill.bill.total_amount,
            }}
            businessName="Business"
            businessAddress="Business Address"
            tableNumber={selectedBill.bill.table_id?.toString() || 'Counter'}
            isOpen={showBillSplitting}
            onClose={() => setShowBillSplitting(false)}
            onPaymentInitiate={async () => {
              setShowBillSplitting(false);
              await loadBills();
              if (selectedBill) {
                await handleViewBill(selectedBill.bill.id);
              }
            }}
          />
        </>
      )}
    </div>
  );
};
