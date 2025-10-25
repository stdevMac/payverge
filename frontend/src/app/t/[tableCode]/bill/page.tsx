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
import ReceiptGenerator from '../../../../components/receipt/ReceiptGenerator';

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
            <h2 className="text-xl font-semibold mb-2">{t('errors.tableNotFound')}</h2>
            <p className="text-default-500">
              {t('errors.tableNotFoundDescription', { tableCode })}
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
            <h2 className="text-xl font-semibold mb-2">{t('errors.businessNotFound')}</h2>
            <p className="text-default-500">
              {t('errors.businessNotFoundDescription')}
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
              {t('bill.thankYouTitle')}
            </h1>
            <p className="text-xl text-gray-600 font-light tracking-wide">
              {t('bill.paymentProcessedSuccessfully')}
            </p>
          </div>

          {/* Order Summary Card */}
          <Card className="bg-white border border-gray-200 shadow-sm mb-6">
            <CardBody className="p-8">
              <div className="flex items-center justify-between mb-6">
                <div>
                  <h2 className="text-2xl font-medium text-gray-900">{t('bill.orderSummary')}</h2>
                  <p className="text-gray-600">{t('bill.billNumber', { number: paidBill.bill.bill_number })}</p>
                </div>
                <div className="text-right">
                  <p className="text-sm text-gray-500">{t('bill.tableLabel')}</p>
                  <p className="text-lg font-medium text-gray-900">{tableData.table.name}</p>
                </div>
              </div>

              {/* Items */}
              <div className="space-y-4 mb-6">
                {paidBill.items.map((item, index) => (
                  <div key={index} className="flex justify-between items-center py-3 border-b border-gray-100 last:border-b-0">
                    <div className="flex-1">
                      <h3 className="font-medium text-gray-900">{item.name}</h3>
                      <p className="text-sm text-gray-500">{t('bill.qtyLabel', { quantity: item.quantity })}</p>
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
                  <span>{t('bill.subtotal')}:</span>
                  <span>${paidBill.bill.subtotal.toFixed(2)}</span>
                </div>
                {paidBill.bill.tax_amount > 0 && (
                  <div className="flex justify-between text-gray-600">
                    <span>{t('bill.tax')}:</span>
                    <span>${paidBill.bill.tax_amount.toFixed(2)}</span>
                  </div>
                )}
                {paidBill.bill.service_fee_amount > 0 && (
                  <div className="flex justify-between text-gray-600">
                    <span>{t('bill.serviceFee')}:</span>
                    <span>${paidBill.bill.service_fee_amount.toFixed(2)}</span>
                  </div>
                )}
                {paymentDetails && paymentDetails.tipAmount > 0 && (
                  <div className="flex justify-between text-green-600">
                    <span>{t('bill.tip')}:</span>
                    <span>${paymentDetails.tipAmount.toFixed(2)}</span>
                  </div>
                )}
                <div className="flex justify-between text-lg font-medium text-gray-900 pt-2 border-t border-gray-200">
                  <span>{t('bill.totalPaid')}</span>
                  <span>${paymentDetails ? paymentDetails.totalPaid.toFixed(2) : '0.00'}</span>
                </div>
              </div>
            </CardBody>
          </Card>

          {/* Receipt Generator */}
          <div className="mb-8">
            <ReceiptGenerator
              data={{
                business: {
                  name: business.name,
                  logo: business.logo,
                  address: business.address,
                  phone: business.phone,
                },
                bill: {
                  bill_number: paidBill.bill.bill_number,
                  created_at: paidBill.bill.created_at,
                  subtotal: paidBill.bill.subtotal,
                  tax_amount: paidBill.bill.tax_amount,
                  service_fee_amount: paidBill.bill.service_fee_amount,
                  total_amount: paidBill.bill.total_amount,
                },
                table: {
                  name: tableData.table.name,
                },
                items: paidBill.items.map(item => ({
                  name: item.name,
                  quantity: item.quantity,
                  subtotal: item.subtotal,
                })),
                paymentDetails: paymentDetails ? {
                  totalPaid: paymentDetails.totalPaid,
                  tipAmount: paymentDetails.tipAmount || 0,
                  paymentMethod: 'Crypto (USDC)',
                  transactionId: undefined,
                } : undefined,
              }}
              onDownloadReceipt={(pdfBlob) => {
                // Create a download link for the PDF
                const url = URL.createObjectURL(pdfBlob);
                const a = document.createElement('a');
                a.href = url;
                a.download = `receipt-${paidBill.bill.bill_number}.pdf`;
                document.body.appendChild(a);
                a.click();
                document.body.removeChild(a);
                URL.revokeObjectURL(url);
              }}
            />
          </div>

          {/* Google Review Button - Only show if business has Google reviews enabled */}
          {business.google_reviews_enabled && business.google_review_link && (
            <div className="text-center mb-8">
              <Button
                size="lg"
                className="bg-green-600 text-white hover:bg-green-700"
                startContent={
                  <svg className="w-5 h-5" fill="currentColor" viewBox="0 0 24 24">
                    <path d="M22.56 12.25c0-.78-.07-1.53-.2-2.25H12v4.26h5.92c-.26 1.37-1.04 2.53-2.21 3.31v2.77h3.57c2.08-1.92 3.28-4.74 3.28-8.09z"/>
                    <path d="M12 23c2.97 0 5.46-.98 7.28-2.66l-3.57-2.77c-.98.66-2.23 1.06-3.71 1.06-2.86 0-5.29-1.93-6.16-4.53H2.18v2.84C3.99 20.53 7.7 23 12 23z"/>
                    <path d="M5.84 14.09c-.22-.66-.35-1.36-.35-2.09s.13-1.43.35-2.09V7.07H2.18C1.43 8.55 1 10.22 1 12s.43 3.45 1.18 4.93l2.85-2.22.81-.62z"/>
                    <path d="M12 5.38c1.62 0 3.06.56 4.21 1.64l3.15-3.15C17.45 2.09 14.97 1 12 1 7.7 1 3.99 3.47 2.18 7.07l3.66 2.84c.87-2.6 3.3-4.53 6.16-4.53z"/>
                  </svg>
                }
                onClick={() => {
                  window.open(business.google_review_link, '_blank', 'noopener,noreferrer');
                }}
              >
                {t('bill.leaveGoogleReview')}
              </Button>
            </div>
          )}

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
              {t('bill.backToTable')}
            </Button>
            
            <p className="text-sm text-gray-500">
              {t('bill.thankYouMessage', { businessName: business.name })}<br />
              {t('bill.hopeToSeeYouSoon')}
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
                  <p className="text-gray-500 font-light">{t('navigation.bill')}</p>
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
            <h2 className="text-3xl font-light text-gray-900 mb-4 tracking-wide">{t('bill.noActiveBill')}</h2>
            <p className="text-gray-600 text-lg mb-10 leading-relaxed max-w-md mx-auto font-light">
              {t('bill.noActiveBillDescription')}
            </p>
            <Link 
              href={`/t/${tableCode}/menu`}
              onClick={() => console.log('Bill page: Clicking Add Items link to menu:', `/t/${tableCode}/menu`)}
            >
              <button className="group bg-gray-900 text-white px-8 py-3 text-base font-medium hover:bg-gray-800 transition-all duration-200 relative overflow-hidden tracking-wide rounded-lg">
                <div className="flex items-center gap-3">
                  <Menu className="w-5 h-5 group-hover:scale-110 transition-transform duration-200" />
                  <span>{t('bill.browseMenu')}</span>
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
                  <span>{t('bill.addItems')}</span>
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
