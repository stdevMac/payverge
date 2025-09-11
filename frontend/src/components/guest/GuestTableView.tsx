'use client';

import React, { useState, useEffect, useCallback } from 'react';
import {
  Card,
  CardBody,
  CardHeader,
  Button,
  Chip,
  Spinner,
  Modal,
  ModalContent,
  ModalHeader,
  ModalBody,
  ModalFooter,
  Image,
} from '@nextui-org/react';
import {
  Users,
  QrCode,
  Receipt,
  Menu,
  Eye,
  DollarSign,
  Store,
  MapPin,
  ShoppingCart
} from 'lucide-react';
import {
  getTableByCode,
  getTableStatusByCode,
  getOpenBillByTableCode,
  BillResponse,
} from '../../api/bills';
import { Business, MenuCategory } from '../../api/business';
import GuestMenu from './GuestMenu';
import GuestBill from './GuestBill';
import { useBillWebSocket } from '../../hooks/useBillWebSocket';
import PaymentNotification from '../notifications/PaymentNotification';
import BillUpdateNotification from '../notifications/BillUpdateNotification';

interface Table {
  id: number;
  business_id: number;
  name: string;
  code: string;
  seats: number;
  status: string;
  qr_code_url: string;
}

interface GuestTableViewProps {
  tableCode: string;
}

interface TableData {
  table: Table;
  business: Business;
  menu: {
    categories: string;
  };
  categories: MenuCategory[];
}

export const GuestTableView: React.FC<GuestTableViewProps> = ({ tableCode }) => {
  const [tableData, setTableData] = useState<TableData | null>(null);
  const [currentBill, setCurrentBill] = useState<BillResponse | null>(null);
  const [loading, setLoading] = useState(true);
  const [activeTab, setActiveTab] = useState<'menu' | 'bill'>('menu');
  const [showBillModal, setShowBillModal] = useState(false);
  const [paymentNotification, setPaymentNotification] = useState<any>(null);
  const [billUpdateNotification, setBillUpdateNotification] = useState<any>(null);

  // Derived state for components
  const businessData = tableData?.business || null;
  const menuCategories = tableData?.categories || [];

  const loadTableData = useCallback(async () => {
    setLoading(true);
    try {
      // Load table, business, and menu data
      const tableResponse = await getTableByCode(tableCode);
      setTableData(tableResponse);

      // Check if there's an open bill
      try {
        const billResponse = await getOpenBillByTableCode(tableCode);
        setCurrentBill(billResponse);
      } catch {
        // No open bill, which is fine
        setCurrentBill(null);
      }
    } catch (error) {
      console.error('Error loading table data:', error);
    } finally {
      setLoading(false);
    }
  }, [tableCode]);

  // Handler functions
  const handleAddToBill = useCallback(async (itemName: string, price: number, quantity: number = 1) => {
    if (!currentBill || !businessData) return;
    
    try {
      // Add item to existing bill
      const { addBillItem } = await import('../../api/bills');
      
      const addItemRequest = {
        menu_item_id: itemName, // Using name as ID since MenuItem interface doesn't have id
        name: itemName,
        price: price,
        quantity,
        options: []
      };
      
      await addBillItem(currentBill.bill.id, addItemRequest);
      
      // Reload bill data
      const billResponse = await getOpenBillByTableCode(tableCode);
      setCurrentBill(billResponse);
    } catch (error) {
      console.error('Error adding item to bill:', error);
    }
  }, [currentBill, businessData, tableCode]);

  const handlePaymentComplete = useCallback(async () => {
    try {
      // Reload table data after payment
      await loadTableData();
    } catch (error) {
      console.error('Error reloading after payment:', error);
    }
  }, [loadTableData]);

  // WebSocket integration for real-time updates
  const { isConnected } = useBillWebSocket({
    tableCode,
    billId: currentBill?.bill.id,
    onBillUpdate: (updatedBill) => {
      setCurrentBill(updatedBill);
      setBillUpdateNotification({
        type: 'bill_updated',
        billNumber: updatedBill.bill.bill_number,
        timestamp: new Date().toISOString()
      });
    },
    onBillClosed: (billId) => {
      setCurrentBill(null);
      setBillUpdateNotification({
        type: 'bill_closed',
        billNumber: currentBill?.bill.bill_number || 'Unknown',
        timestamp: new Date().toISOString()
      });
    },
    onPaymentReceived: (payment) => {
      setPaymentNotification({
        amount: payment.amount,
        currency: payment.currency || 'USD',
        billNumber: currentBill?.bill.bill_number || 'Unknown',
        timestamp: new Date().toISOString()
      });
      // Reload data after payment
      loadTableData();
    },
    onItemAdded: (item) => {
      setBillUpdateNotification({
        type: 'item_added',
        itemName: item.name,
        billNumber: currentBill?.bill.bill_number || 'Unknown',
        timestamp: new Date().toISOString()
      });
    },
    onItemRemoved: (itemId) => {
      setBillUpdateNotification({
        type: 'item_removed',
        itemName: 'Item',
        billNumber: currentBill?.bill.bill_number || 'Unknown',
        timestamp: new Date().toISOString()
      });
    }
  });


  useEffect(() => {
    loadTableData();
  }, [loadTableData]);

  const handleViewBill = () => {
    if (currentBill) {
      setShowBillModal(true);
    }
  };

  if (loading) {
    return (
      <div className="min-h-screen flex items-center justify-center">
        <Spinner size="lg" />
      </div>
    );
  }

  if (!tableData) {
    return (
      <div className="min-h-screen flex items-center justify-center">
        <Card className="max-w-md">
          <CardBody className="text-center py-8">
            <h2 className="text-xl font-semibold mb-2">Table Not Found</h2>
            <p className="text-default-500">
              The table code &quot;{tableCode}&quot; could not be found.
            </p>
          </CardBody>
        </Card>
      </div>
    );
  }

  const { table, business, categories } = tableData;

  return (
    <div className="min-h-screen bg-default-50">
      {/* Header */}
      <div className="bg-white shadow-sm border-b">
        <div className="max-w-4xl mx-auto px-4 py-4">
          <div className="flex items-center justify-between">
            <div className="flex items-center gap-4">
              {business.logo && (
                <Image
                  src={business.logo}
                  alt={business.name}
                  className="w-12 h-12 rounded-lg"
                />
              )}
              <div>
                <p className="text-default-600">Welcome to &quot;{business.name}&quot;</p>
                <div className="flex items-center gap-2 text-sm text-default-500">
                  <Store className="w-4 h-4" />
                  <span>Don&apos;t have a bill yet? Ask your server to create one.</span>
                </div>
              </div>
            </div>
            {currentBill && (
              <Button
                color="primary"
                variant="flat"
                startContent={<Receipt className="w-4 h-4" />}
                onPress={handleViewBill}
              >
                View Bill
              </Button>
            )}
          </div>
        </div>
      </div>

      {/* Business Info */}
      <div className="max-w-4xl mx-auto px-4 py-6">
        <Card className="mb-6">
          <CardBody>
            <div className="grid grid-cols-1 md:grid-cols-3 gap-4">
              {business.address && (
                <div className="flex items-start gap-3">
                  <MapPin className="w-5 h-5 text-default-400 mt-0.5" />
                  <div>
                    <p className="font-medium text-sm">Address</p>
                    <p className="text-sm text-default-600">
                      {business.address.street}<br />
                      {business.address.city}, {business.address.state} {business.address.postal_code}
                    </p>
                  </div>
                </div>
              )}
            </div>
          </CardBody>
        </Card>

        {/* Tab Navigation */}
        <div className="flex gap-2 mb-6">
          <Button
            variant={activeTab === 'menu' ? 'solid' : 'light'}
            color={activeTab === 'menu' ? 'primary' : 'default'}
            startContent={<ShoppingCart className="w-4 h-4" />}
            onPress={() => setActiveTab('menu')}
          >
            Menu
          </Button>
          <Button
            variant={activeTab === 'bill' ? 'solid' : 'light'}
            color={activeTab === 'bill' ? 'primary' : 'default'}
            startContent={<Receipt className="w-4 h-4" />}
            onPress={() => setActiveTab('bill')}
            isDisabled={!currentBill}
          >
            Current Bill {currentBill && `($${currentBill.bill.total_amount.toFixed(2)})`}
          </Button>
        </div>

        {/* Content */}
        {activeTab === 'menu' && (
          <GuestMenu
            categories={menuCategories}
            business={business}
            tableCode={tableCode}
            currentBill={currentBill}
            onAddToBill={handleAddToBill}
          />
        )}

        {activeTab === 'bill' && currentBill && (
          <GuestBill
            bill={currentBill}
            business={business!}
            tableCode={tableCode}
            onPaymentComplete={handlePaymentComplete}
          />
        )}

        {activeTab === 'bill' && !currentBill && (
          <Card>
            <CardBody className="text-center py-8">
              <Receipt className="w-12 h-12 mx-auto text-default-300 mb-4" />
              <h3 className="text-lg font-medium text-default-500 mb-2">No Active Bill</h3>
              <p className="text-default-400">
                There&apos;s no active bill for this table yet.
              </p>
            </CardBody>
          </Card>
        )}
      </div>

      {/* Bill Modal */}
      {currentBill && (
        <Modal
          isOpen={showBillModal}
          onClose={() => setShowBillModal(false)}
          size="2xl"
          scrollBehavior="inside"
        >
          <ModalContent>
            <ModalHeader>
              <div className="flex items-center gap-2">
                <Receipt className="w-5 h-5" />
                Bill - {currentBill.bill.bill_number}
              </div>
            </ModalHeader>
            <ModalBody>
              <GuestBill
                bill={currentBill}
                business={business!}
                tableCode={tableCode}
                onPaymentComplete={handlePaymentComplete}
                compact
              />
            </ModalBody>
            <ModalFooter>
              <Button variant="light" onPress={() => setShowBillModal(false)}>
                Close
              </Button>
            </ModalFooter>
          </ModalContent>
        </Modal>
      )}

      {/* Notifications */}
      <PaymentNotification
        payment={paymentNotification}
        onDismiss={() => setPaymentNotification(null)}
      />
      
      <BillUpdateNotification
        update={billUpdateNotification}
        onDismiss={() => setBillUpdateNotification(null)}
      />
    </div>
  );
};
