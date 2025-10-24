'use client';

import React, { useState, useEffect, useCallback, useMemo } from 'react';
import { useParams } from 'next/navigation';
import { Card, CardBody, Spinner, Button, Image } from '@nextui-org/react';
import { GuestTranslationProvider, useGuestTranslation } from '../../../../i18n/GuestTranslationProvider';
import { ArrowLeft, Menu } from 'lucide-react';
import Link from 'next/link';
import dynamic from 'next/dynamic';
import PersistentGuestNav from '../../../../components/navigation/PersistentGuestNav';

// Lazy load heavy components
const GuestBill = dynamic(() => import('../../../../components/guest/GuestBill'), {
  loading: () => <div className="flex justify-center p-8"><Spinner size="lg" /></div>
});
import { 
  getTableByCode, 
  getOpenBillByTableCode,
  BillWithItemsResponse 
} from '../../../../api/bills';
import { Business, MenuCategory, businessApi } from '../../../../api/business';
import { getMenuByTableCode } from '../../../../api/bills';
import { FloatingLanguageSelector } from '@/components/guest/FloatingLanguageSelector';

interface Table {
  id: number;
  business_id: number;
  name: string;
  code: string;
  seats: number;
  status: string;
  qr_code_url: string;
}

interface TableData {
  table: Table;
  business: Business;
  menu: {
    categories: string;
  };
  categories: MenuCategory[];
}

// Internal component that uses the hook
function GuestBillPageContent() {
  const { t, setBusinessId } = useGuestTranslation();
  const params = useParams();
  const tableCode = params.tableCode as string;
  
  const [tableData, setTableData] = useState<TableData | null>(null);
  const [currentBill, setCurrentBill] = useState<BillWithItemsResponse | null>(null);
  const [loading, setLoading] = useState(true);
  const [showThankYou, setShowThankYou] = useState(false);
  const [paidBill, setPaidBill] = useState<BillWithItemsResponse | null>(null);
  const [paymentDetails, setPaymentDetails] = useState<{
    totalPaid: number;
    tipAmount: number;
  } | null>(null);
  const [selectedLanguage, setSelectedLanguage] = useState<string>('');
  const [translatedCategories, setTranslatedCategories] = useState<MenuCategory[]>([]);
  
  // Currency settings
  const [businessCurrencies, setBusinessCurrencies] = useState({
    default_currency: 'USD',
    display_currency: 'USD'
  });

  const loadTableData = useCallback(async () => {
    setLoading(true);
    try {
      // Load table data and bill data in parallel for better performance
      const [tableResponse, billResponse] = await Promise.allSettled([
        getTableByCode(tableCode),
        getOpenBillByTableCode(tableCode)
      ]);

      if (tableResponse.status === 'fulfilled') {
        setTableData(tableResponse.value);
        
        // Set business ID for translation provider
        if (tableResponse.value?.business?.id) {
          setBusinessId(tableResponse.value.business.id);
        }
        
        // Load business currency settings
        try {
          const business = await businessApi.getBusiness(tableResponse.value.business.id);
          setBusinessCurrencies({
            default_currency: business.default_currency || 'USD',
            display_currency: business.display_currency || 'USD'
          });
        } catch (error) {
          console.error('Error loading business currency settings:', error);
        }
      } else {
        console.error('Error loading table data:', tableResponse.reason);
      }

      if (billResponse.status === 'fulfilled') {
        setCurrentBill(billResponse.value);
      } else {
        setCurrentBill(null);
      }
    } catch (error) {
      console.error('Error loading data:', error);
    } finally {
      setLoading(false);
    }
  }, [tableCode, setBusinessId]);

  const loadTranslatedMenu = useCallback(async (languageCode: string) => {
    try {
      const menuData = await getMenuByTableCode(tableCode, languageCode);
      // Use translated categories if available, otherwise fall back to original
      const categories = menuData.parsed_categories || menuData.categories;
      setTranslatedCategories(categories || []);
    } catch (error) {
      console.error('Error loading translated menu:', error);
      // Fall back to original categories if translation fails
      if (tableData?.categories) {
        setTranslatedCategories(tableData.categories);
      }
    }
  }, [tableCode, tableData?.categories]);

  const handleLanguageChange = useCallback((languageCode: string) => {
    console.log('Bill page: Language changed to:', languageCode);
    setSelectedLanguage(languageCode);
    loadTranslatedMenu(languageCode);
  }, [loadTranslatedMenu]);

  // Initialize language and menu data when table data loads
  useEffect(() => {
    if (tableData?.business?.id && tableData?.categories) {
      const savedLanguage = localStorage.getItem(`guest-language-${tableData.business.id}`);
      
      if (savedLanguage && !selectedLanguage) {
        console.log('Bill page: Restoring saved language and loading translated menu:', savedLanguage);
        setSelectedLanguage(savedLanguage);
        // Load translated menu immediately
        loadTranslatedMenu(savedLanguage);
      } else if (!selectedLanguage) {
        console.log('Bill page: No saved language, initializing with original categories');
        setTranslatedCategories(tableData.categories);
      }
    }
  }, [tableData?.business?.id, tableData?.categories, selectedLanguage, loadTranslatedMenu]);

  const handlePaymentComplete = useCallback(async (paymentDetails: { totalPaid: number; tipAmount: number }) => {
    try {
      
      // Store the current bill and payment details for the thank you screen
      if (currentBill) {
        setPaidBill(currentBill);
        setPaymentDetails(paymentDetails);
        setShowThankYou(true);
        setCurrentBill(null);
      }
      
    } catch (error) {
      console.error('Error handling payment completion:', error);
      // Still show thank you screen even if there's an error
      if (currentBill) {
        setPaidBill(currentBill);
        setPaymentDetails(paymentDetails);
        setShowThankYou(true);
        setCurrentBill(null);
      }
    }
  }, [currentBill]);

  useEffect(() => {
    loadTableData();
  }, [loadTableData]);

  // Memoize business data to prevent unnecessary re-renders
  const businessData = useMemo(() => tableData?.business, [tableData?.business]);

  if (loading) {
    return (
      <div className="min-h-screen bg-white">
        <PersistentGuestNav 
          tableCode={tableCode}
          currentBill={null}
        />
        
        <div className="max-w-4xl mx-auto px-6 py-8">
          {/* Loading skeleton */}
          <div className="animate-pulse">
            <div className="h-8 bg-gray-200 rounded w-1/3 mb-6"></div>
            <div className="bg-gray-100 rounded-2xl p-6 mb-6">
              <div className="h-6 bg-gray-200 rounded w-1/4 mb-4"></div>
              <div className="space-y-3">
                <div className="h-4 bg-gray-200 rounded w-full"></div>
                <div className="h-4 bg-gray-200 rounded w-3/4"></div>
                <div className="h-4 bg-gray-200 rounded w-1/2"></div>
              </div>
            </div>
            <div className="h-12 bg-gray-200 rounded-lg w-full"></div>
          </div>
        </div>
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

  const business = businessData;

  // Early return if business data is not available
  if (!business) {
    return (
      <div className="min-h-screen flex items-center justify-center">
        <Card className="max-w-md">
          <CardBody className="text-center py-8">
            <h2 className="text-xl font-semibold mb-2">Business Not Found</h2>
            <p className="text-default-500">
              Unable to load business information for this table.
            </p>
          </CardBody>
        </Card>
      </div>
    );
  }

  // Show thank you screen after payment
  if (showThankYou && paidBill) {
    return (
      <div className="min-h-screen bg-white relative overflow-hidden">
        {/* Subtle animated background elements */}
        <div className="absolute inset-0 overflow-hidden pointer-events-none">
          <div className="absolute -top-40 -right-40 w-80 h-80 bg-gradient-to-br from-green-50 to-blue-50 rounded-full blur-3xl opacity-30 animate-pulse"></div>
          <div className="absolute -bottom-40 -left-40 w-96 h-96 bg-gradient-to-tr from-gray-50 to-green-50 rounded-full blur-3xl opacity-20 animate-pulse" style={{ animationDelay: '2s' }}></div>
        </div>

        {/* Thank You Content */}
        <div className="relative z-10 max-w-4xl mx-auto px-6 py-8">
          {/* Header */}
          <div className="text-center mb-8">
            <div className="w-20 h-20 bg-green-50 rounded-full flex items-center justify-center mx-auto mb-4 border border-green-100">
              <svg className="w-10 h-10 text-green-600" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M5 13l4 4L19 7" />
              </svg>
            </div>
            <h1 className="text-4xl font-light tracking-tight text-gray-900 mb-2">
              Thank You!
            </h1>
            <p className="text-xl text-gray-600 font-light tracking-wide">
              Your payment has been processed successfully
            </p>
          </div>

          {/* Order Summary Card */}
          <Card className="bg-white border border-gray-200 shadow-sm mb-6">
            <CardBody className="p-8">
              <div className="flex items-center justify-between mb-6">
                <div>
                  <h2 className="text-2xl font-medium text-gray-900">Order Summary</h2>
                  <p className="text-gray-600">Bill #{paidBill.bill.bill_number}</p>
                </div>
                <div className="text-right">
                  <p className="text-sm text-gray-500">Table</p>
                  <p className="text-lg font-medium text-gray-900">{tableData.table.name}</p>
                </div>
              </div>

              {/* Items */}
              <div className="space-y-4 mb-6">
                {paidBill.items.map((item, index) => (
                  <div key={index} className="flex justify-between items-center py-3 border-b border-gray-100 last:border-b-0">
                    <div className="flex-1">
                      <h3 className="font-medium text-gray-900">{item.name}</h3>
                      <p className="text-sm text-gray-500">Qty: {item.quantity}</p>
                    </div>
                    <div className="text-right">
                      <p className="font-medium text-gray-900">${item.subtotal.toFixed(2)}</p>
                    </div>
                  </div>
                ))}
              </div>

              {/* Bill Totals */}
              <div className="border-t border-gray-200 pt-4 space-y-2">
                <div className="flex justify-between text-gray-600">
                  <span>Subtotal:</span>
                  <span>${paidBill.bill.subtotal.toFixed(2)}</span>
                </div>
                {paidBill.bill.tax_amount > 0 && (
                  <div className="flex justify-between text-gray-600">
                    <span>Tax:</span>
                    <span>${paidBill.bill.tax_amount.toFixed(2)}</span>
                  </div>
                )}
                {paidBill.bill.service_fee_amount > 0 && (
                  <div className="flex justify-between text-gray-600">
                    <span>Service Fee:</span>
                    <span>${paidBill.bill.service_fee_amount.toFixed(2)}</span>
                  </div>
                )}
                {paymentDetails && paymentDetails.tipAmount > 0 && (
                  <div className="flex justify-between text-green-600">
                    <span>Tip:</span>
                    <span>${paymentDetails.tipAmount.toFixed(2)}</span>
                  </div>
                )}
                <div className="flex justify-between text-lg font-medium text-gray-900 pt-2 border-t border-gray-200">
                  <span>Total Paid:</span>
                  <span>${paymentDetails ? paymentDetails.totalPaid.toFixed(2) : '0.00'}</span>
                </div>
              </div>
            </CardBody>
          </Card>

          {/* Receipt Options */}
          <div className="grid grid-cols-1 sm:grid-cols-2 gap-4 mb-8">
            <Button
              size="lg"
              className="bg-blue-600 text-white hover:bg-blue-700"
              startContent={
                <svg className="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                  <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M3 8l7.89 4.26a2 2 0 002.22 0L21 8M5 19h14a2 2 0 002-2V7a2 2 0 00-2-2H5a2 2 0 00-2 2v10a2 2 0 002 2z" />
                </svg>
              }
              onClick={() => {
                const subject = `Receipt - ${business.name} - Bill #${paidBill.bill.bill_number}`;
                const totalPaid = paymentDetails ? paymentDetails.totalPaid : 0;
                const body = `Thank you for dining with us!\n\nOrder Details:\nBill #${paidBill.bill.bill_number}\nTable: ${tableData.table.name}\nTotal: $${totalPaid.toFixed(2)}\n\nThank you for choosing ${business.name}!`;
                window.location.href = `mailto:?subject=${encodeURIComponent(subject)}&body=${encodeURIComponent(body)}`;
              }}
            >
              Email Receipt
            </Button>
            
            <Button
              size="lg"
              variant="bordered"
              className="border-gray-300 text-gray-700 hover:bg-gray-50"
              startContent={
                <svg className="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                  <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M8.684 13.342C8.886 12.938 9 12.482 9 12c0-.482-.114-.938-.316-1.342m0 2.684a3 3 0 110-2.684m0 2.684l6.632 3.316m-6.632-6l6.632-3.316m0 0a3 3 0 105.367-2.684 3 3 0 00-5.367 2.684zm0 9.316a3 3 0 105.367 2.684 3 3 0 00-5.367-2.684z" />
                </svg>
              }
              onClick={() => {
                const totalPaid = paymentDetails ? paymentDetails.totalPaid : 0;
                if (navigator.share) {
                  navigator.share({
                    title: `Receipt - ${business.name}`,
                    text: `Thank you for dining at ${business.name}! Bill #${paidBill.bill.bill_number} - Total: $${totalPaid.toFixed(2)}`,
                  });
                } else {
                  // Fallback for browsers that don't support Web Share API
                  const text = `Thank you for dining at ${business.name}! Bill #${paidBill.bill.bill_number} - Total: $${totalPaid.toFixed(2)}`;
                  navigator.clipboard.writeText(text);
                  alert('Receipt details copied to clipboard!');
                }
              }}
            >
              Share Receipt
            </Button>
          </div>

          {/* Actions */}
          <div className="text-center space-y-4">
            <Button
              size="lg"
              variant="light"
              className="text-gray-600 hover:text-gray-800"
              startContent={<ArrowLeft className="w-5 h-5" />}
              onClick={() => {
                setShowThankYou(false);
                setPaidBill(null);
              }}
            >
              Back to Table
            </Button>
            
            <p className="text-sm text-gray-500">
              Thank you for choosing <span className="font-medium">{business.name}</span>!<br />
              We hope to see you again soon.
            </p>
          </div>
        </div>
      </div>
    );
  }

  if (!currentBill) {
    return (
      <div className="min-h-screen bg-white relative overflow-hidden">
        {/* Subtle animated background elements */}
        <div className="absolute inset-0 overflow-hidden pointer-events-none">
          <div className="absolute -top-40 -right-40 w-80 h-80 bg-gradient-to-br from-blue-50 to-purple-50 rounded-full blur-3xl opacity-30 animate-pulse"></div>
          <div className="absolute -bottom-40 -left-40 w-96 h-96 bg-gradient-to-tr from-gray-50 to-blue-50 rounded-full blur-3xl opacity-20 animate-pulse" style={{ animationDelay: '2s' }}></div>
        </div>

        {/* Clean Header */}
        <div className="relative z-10 bg-white/80 backdrop-blur-xl border-b border-gray-100">
          <div className="max-w-4xl mx-auto px-6 py-6">
            <div className="flex items-center gap-6">
              <Link 
              href={`/t/${tableCode}`}
              onClick={() => console.log('Bill page: Clicking back to table link:', `/t/${tableCode}`)}
            >
                <Button
                  isIconOnly
                  variant="light"
                  className="text-gray-600 hover:text-gray-900 hover:bg-gray-100 border border-gray-200 rounded-xl"
                  size="lg"
                >
                  <ArrowLeft className="w-6 h-6" />
                </Button>
              </Link>
              
              <div className="flex items-center gap-4 flex-1">
                {business.logo && (
                  <div className="w-12 h-12 rounded-xl overflow-hidden border border-gray-100 shadow-sm">
                    <Image
                      src={business.logo}
                      alt={business.name}
                      className="w-full h-full object-cover"
                    />
                  </div>
                )}
                <div>
                  <h1 className="text-2xl font-light text-gray-900 tracking-wide">
                    {business.name}
                  </h1>
                  <p className="text-gray-500 font-light">Bill</p>
                </div>
              </div>
            </div>
          </div>
        </div>

        {/* No Bill Content */}
        <div className="relative z-10 max-w-2xl mx-auto px-6 py-16 pb-28">
          <div className="bg-white border border-gray-200 rounded-2xl p-12 text-center shadow-lg hover:shadow-xl transition-shadow duration-300">
            <div className="w-20 h-20 bg-gray-50 rounded-2xl flex items-center justify-center mx-auto mb-8 border border-gray-100">
              <div className="text-3xl">🧾</div>
            </div>
            <h2 className="text-3xl font-light text-gray-900 mb-4 tracking-wide">No Active Bill</h2>
            <p className="text-gray-600 text-lg mb-10 leading-relaxed max-w-md mx-auto font-light">
              There&apos;s no active bill for this table yet. Start by browsing our menu and adding items.
            </p>
            <Link 
              href={`/t/${tableCode}/menu`}
              onClick={() => console.log('Bill page: Clicking Add Items link to menu:', `/t/${tableCode}/menu`)}
            >
              <button className="group bg-gray-900 text-white px-8 py-3 text-base font-medium hover:bg-gray-800 transition-all duration-200 relative overflow-hidden tracking-wide rounded-lg">
                <div className="flex items-center gap-3">
                  <Menu className="w-5 h-5 group-hover:scale-110 transition-transform duration-200" />
                  <span>Browse Menu</span>
                </div>
              </button>
            </Link>
          </div>
        </div>

        {/* Persistent Navigation */}
        <PersistentGuestNav 
          tableCode={tableCode}
          currentBill={currentBill}
        />
      </div>
    );
  }

  return (
        <div className="min-h-screen bg-white relative overflow-hidden">
      {/* Subtle animated background elements */}
      <div className="absolute inset-0 overflow-hidden pointer-events-none">
        <div className="absolute -top-40 -right-40 w-80 h-80 bg-gradient-to-br from-blue-50 to-purple-50 rounded-full blur-3xl opacity-30 animate-pulse"></div>
        <div className="absolute -bottom-40 -left-40 w-96 h-96 bg-gradient-to-tr from-gray-50 to-blue-50 rounded-full blur-3xl opacity-20 animate-pulse" style={{ animationDelay: '2s' }}></div>
      </div>

      {/* Clean Header */}
      <div className="relative z-10 bg-white/80 backdrop-blur-xl border-b border-gray-100">
        <div className="max-w-4xl mx-auto px-6 py-6">
          <div className="flex items-center gap-6">
            <Link 
              href={`/t/${tableCode}`}
              onClick={() => console.log('Bill page (no bill): Clicking back to table link:', `/t/${tableCode}`)}
            >
              <Button
                isIconOnly
                variant="light"
                className="text-gray-600 hover:text-gray-900 hover:bg-gray-100 border border-gray-200 rounded-xl"
                size="lg"
              >
                <ArrowLeft className="w-6 h-6" />
              </Button>
            </Link>
            
            <div className="flex items-center gap-4 flex-1">
              {business.logo && (
                <div className="w-12 h-12 rounded-xl overflow-hidden border border-gray-100 shadow-sm">
                  <Image
                    src={business.logo}
                    alt={business.name}
                    className="w-full h-full object-cover"
                  />
                </div>
              )}
              <div>
                <h1 className="text-2xl font-light text-gray-900 tracking-wide">
                  {business.name}
                </h1>
                <p className="text-gray-500 font-light">Bill #{currentBill.bill.bill_number}</p>
              </div>
            </div>

            
            <Link 
              href={`/t/${tableCode}/menu`}
              onClick={() => console.log('Bill page header: Clicking Add Items link to menu:', `/t/${tableCode}/menu`)}
            >
              <button className="group border border-gray-300 text-gray-700 px-6 py-2 text-sm font-medium hover:border-gray-400 hover:text-gray-900 transition-all duration-200 tracking-wide rounded-lg">
                <div className="flex items-center gap-2">
                  <Menu className="w-4 h-4 group-hover:scale-110 transition-transform duration-200" />
                  <span>Add Items</span>
                </div>
              </button>
            </Link>
          </div>
        </div>
      </div>


      {/* Bill Content */}
      <div className="relative z-10 max-w-4xl mx-auto px-6 py-8 pb-28">
        <GuestBill
          bill={currentBill}
          business={business}
          tableCode={tableCode}
          selectedLanguage={selectedLanguage}
          defaultCurrency={businessCurrencies.default_currency}
          displayCurrency={businessCurrencies.display_currency}
          onPaymentComplete={handlePaymentComplete}
        />
      </div>

      {/* Persistent Navigation */}
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
      </div>
  );
}

// Main page component that provides the context
export default function GuestBillPage() {
  return (
    <GuestTranslationProvider businessId={undefined}>
      <GuestBillPageContent />
    </GuestTranslationProvider>
  );
}
