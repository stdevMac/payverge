'use client';

import React, { useState, useEffect, useCallback } from 'react';
import {
  Card,
  CardBody,
  Button,
  Chip,
  Spinner,
  Image,
  Divider,
} from '@nextui-org/react';
import {
  Menu,
  Receipt,
  QrCode,
  MapPin,
  Clock,
  Users,
  ChevronRight,
} from 'lucide-react';
import {
  getTableByCode,
  getOpenBillByTableCode,
  BillWithItemsResponse,
} from '../../api/bills';
import { Business, MenuCategory } from '../../api/business';
import { formatCurrency } from '../../api/currency';
import { useBillWebSocket } from '../../hooks/useBillWebSocket';
import PaymentNotification from '../notifications/PaymentNotification';
import BillUpdateNotification from '../notifications/BillUpdateNotification';
import PersistentGuestNav from '../navigation/PersistentGuestNav';
import { useGuestTranslation } from '../../i18n/GuestTranslationProvider';
import { FloatingLanguageSelector } from './FloatingLanguageSelector';
import Link from 'next/link';

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
  const { t, setBusinessId } = useGuestTranslation();
  const [tableData, setTableData] = useState<TableData | null>(null);
  const [currentBill, setCurrentBill] = useState<BillWithItemsResponse | null>(null);
  const [loading, setLoading] = useState(true);
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
      
      // Set business ID for translation provider
      if (tableResponse?.business?.id) {
        setBusinessId(tableResponse.business.id);
      }

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
      // For now, cast to the expected type since WebSocket might not include items
      setCurrentBill(updatedBill as BillWithItemsResponse);
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
        currency: payment.currency || businessData?.display_currency || businessData?.default_currency || 'USD',
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
            <h2 className="text-xl font-semibold mb-2">{t('errors.tableNotFound')}</h2>
            <p className="text-default-500">
              {t('errors.tableNotFoundDescription')}
            </p>
          </CardBody>
        </Card>
      </div>
    );
  }

  const { table, business, categories } = tableData;

  return (
    <div className="min-h-screen bg-white relative overflow-hidden">
      {/* Subtle animated background elements */}
      <div className="absolute inset-0 overflow-hidden pointer-events-none">
        <div className="absolute -top-40 -right-40 w-80 h-80 bg-gradient-to-br from-blue-50 to-purple-50 rounded-full blur-3xl opacity-30 animate-pulse"></div>
        <div className="absolute -bottom-40 -left-40 w-96 h-96 bg-gradient-to-tr from-gray-50 to-blue-50 rounded-full blur-3xl opacity-20 animate-pulse" style={{ animationDelay: '2s' }}></div>
        <div className="absolute top-1/2 left-1/2 transform -translate-x-1/2 -translate-y-1/2 w-80 h-80 bg-gradient-to-r from-purple-50 to-pink-50 rounded-full blur-3xl opacity-15 animate-pulse" style={{ animationDelay: '1s' }}></div>
      </div>

      {/* Clean Header */}
      <section className="relative py-16 lg:py-24">
        <div className="container mx-auto px-6 relative">
          <div className="max-w-4xl mx-auto text-center">
            {/* Floating status badge */}
            <div className="inline-flex items-center gap-2 bg-gray-50 border border-gray-200 px-4 py-2 rounded-full text-sm text-gray-600 mb-8 hover:bg-gray-100 transition-colors duration-200">
              <div className="w-2 h-2 bg-green-400 rounded-full animate-pulse"></div>
              {t('table.title', { tableNumber: table.name })} 
            </div>

            {/* Business Logo */}
            {business.logo && (
              <div className="w-24 h-24 mx-auto mb-6 rounded-2xl overflow-hidden shadow-lg border border-gray-100 hover:shadow-xl transition-shadow duration-300">
                <Image
                  src={business.logo}
                  alt={business.name}
                  className="w-full h-full object-cover"
                />
              </div>
            )}
            
            {/* Business Name */}
            <h1 className="text-4xl lg:text-6xl font-light text-gray-900 mb-6 tracking-wide leading-tight">
              {business.name}
            </h1>
            
            {/* Address */}
            {business.address && (
              <div className="inline-flex items-center gap-3 text-gray-600 mb-12">
                <MapPin className="w-5 h-5" />
                <span className="text-lg font-light">
                  {business.address.street}, {business.address.city}
                </span>
              </div>
            )}
          </div>
        </div>
      </section>

      {/* Main Actions */}
      <section className="relative pb-24">
        <div className="container mx-auto px-6">
          <div className="max-w-4xl mx-auto">
            <div className="grid md:grid-cols-2 gap-8">
              {/* Menu Card */}
              <Link href={`/t/${tableCode}/menu`}>
                <div className="group bg-white border border-gray-200 rounded-2xl p-8 hover:shadow-xl hover:border-gray-300 transition-all duration-300 hover:-translate-y-1 relative overflow-hidden h-full min-h-[280px] flex flex-col">
                  {/* Subtle gradient overlay on hover */}
                  <div className="absolute inset-0 bg-gradient-to-br from-blue-50/50 to-purple-50/50 opacity-0 group-hover:opacity-100 transition-opacity duration-300 rounded-2xl"></div>
                  
                  <div className="relative z-10 text-center space-y-6 flex-1 flex flex-col justify-between">
                    <div className="space-y-6">
                      {/* Icon */}
                      <div className="w-16 h-16 bg-gray-50 rounded-2xl flex items-center justify-center mx-auto group-hover:bg-blue-50 transition-colors duration-300">
                        <Menu className="w-8 h-8 text-gray-600 group-hover:text-blue-600 group-hover:scale-110 transition-all duration-300" />
                      </div>
                      
                      <div className="space-y-3">
                        <h3 className="text-2xl font-light text-gray-900 tracking-wide">
                          {t('table.browseMenu')}
                        </h3>
                        <p className="text-gray-600 font-light leading-relaxed">
                          {t('table.browseMenuDescription')}
                        </p>
                      </div>
                    </div>
                    
                    {/* Arrow */}
                    <div className="flex items-center justify-center gap-2 text-gray-400 group-hover:text-gray-600 group-hover:translate-x-1 transition-all duration-300">
                      <span className="text-sm font-medium tracking-wide">{t('navigation.goToMenu')}</span>
                      <ChevronRight className="w-4 h-4" />
                    </div>
                  </div>
                </div>
              </Link>

              {/* Current Bill Card */}
              {currentBill ? (
                <Link href={`/t/${tableCode}/bill`}>
                  <div className="group bg-white border border-green-200 rounded-2xl p-8 hover:shadow-xl hover:border-green-300 transition-all duration-300 hover:-translate-y-1 relative overflow-hidden h-full min-h-[280px] flex flex-col">
                    {/* Subtle gradient overlay on hover */}
                    <div className="absolute inset-0 bg-gradient-to-br from-green-50/50 to-emerald-50/50 opacity-0 group-hover:opacity-100 transition-opacity duration-300 rounded-2xl"></div>
                    
                    <div className="relative z-10 text-center space-y-6 flex-1 flex flex-col justify-between">
                      <div className="space-y-6">
                        {/* Icon */}
                        <div className="w-16 h-16 bg-green-50 rounded-2xl flex items-center justify-center mx-auto group-hover:bg-green-100 transition-colors duration-300">
                          <Receipt className="w-8 h-8 text-green-600 group-hover:scale-110 transition-transform duration-300" />
                        </div>
                        
                        <div className="space-y-3">
                          <div className="flex items-center justify-center gap-3">
                            <h3 className="text-2xl font-light text-gray-900 tracking-wide">
                              {t('table.currentBill')}
                            </h3>
                            <Chip 
                              size="sm" 
                              className="bg-green-100 text-green-700 border-green-200"
                              variant="bordered"
                            >
                              {t('bill.billStatus.open')}
                            </Chip>
                          </div>
                          <p className="text-green-600 font-medium text-lg">
                            {formatCurrency(
                              currentBill.bill.total_amount, 
                              business.display_currency || business.default_currency || 'USD'
                            )} • {currentBill.items.length} {t('menu.items')}
                          </p>
                        </div>
                      </div>
                      
                      {/* Arrow */}
                      <div className="flex items-center justify-center gap-2 text-gray-400 group-hover:text-gray-600 group-hover:translate-x-1 transition-all duration-300">
                        <span className="text-sm font-medium tracking-wide">{t('navigation.goToBill')}</span>
                        <ChevronRight className="w-4 h-4" />
                      </div>
                    </div>
                  </div>
                </Link>
              ) : (
                <div className="bg-gray-50 border-2 border-dashed border-gray-200 rounded-2xl p-8 text-center hover:border-gray-300 transition-colors duration-300">
                  <div className="space-y-6">
                    <div className="w-16 h-16 bg-gray-100 rounded-2xl flex items-center justify-center mx-auto">
                      <Receipt className="w-8 h-8 text-gray-400" />
                    </div>
                    <div className="space-y-3">
                      <h3 className="text-2xl font-light text-gray-700 tracking-wide">
                        {t('table.noBill')}
                      </h3>
                      <p className="text-gray-500 font-light leading-relaxed">
                        {t('table.noBillDescription')}
                      </p>
                    </div>
                  </div>
                </div>
              )}
            </div>

            {/* Bottom Actions */}
            <div className="mt-16 pt-12 border-t border-gray-100 text-center space-y-6">
              <Link href="/scan">
                <button className="group border border-gray-300 text-gray-700 px-8 py-3 text-base font-medium hover:border-gray-400 hover:text-gray-900 transition-all duration-200 tracking-wide rounded-lg">
                  <div className="flex items-center gap-3">
                    <QrCode className="w-5 h-5 group-hover:scale-110 transition-transform duration-200" />
                    <span>{t('table.scanQR')}</span>
                  </div>
                </button>
              </Link>
              <p className="text-sm text-gray-400 font-light tracking-wide">
                Powered by Payverge • Secure USDC Payments
              </p>
            </div>
          </div>
        </div>
      </section>

      {/* Enhanced Persistent Navigation */}
      <PersistentGuestNav 
        tableCode={tableCode}
        currentBill={currentBill}
      />

      {/* Floating Language Selector */}
      {business?.id && (
        <FloatingLanguageSelector
          tableCode={tableCode}
        />  
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
