'use client';

import React, { useState, useEffect, useCallback } from 'react';
import { useParams } from 'next/navigation';
import { Card, CardBody, Spinner, Button, Image } from '@nextui-org/react';
import { ArrowLeft, Menu } from 'lucide-react';
import Link from 'next/link';
import GuestBill from '../../../../components/guest/GuestBill';
import { PersistentGuestNav } from '../../../../components/navigation/PersistentGuestNav';
import { 
  getTableByCode, 
  getOpenBillByTableCode,
  BillResponse 
} from '../../../../api/bills';
import { Business, MenuCategory } from '../../../../api/business';

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

export default function GuestBillPage() {
  const params = useParams();
  const tableCode = params.tableCode as string;
  
  const [tableData, setTableData] = useState<TableData | null>(null);
  const [currentBill, setCurrentBill] = useState<BillResponse | null>(null);
  const [loading, setLoading] = useState(true);

  const loadTableData = useCallback(async () => {
    setLoading(true);
    try {
      const tableResponse = await getTableByCode(tableCode);
      setTableData(tableResponse);

      try {
        const billResponse = await getOpenBillByTableCode(tableCode);
        setCurrentBill(billResponse);
      } catch {
        setCurrentBill(null);
      }
    } catch (error) {
      console.error('Error loading table data:', error);
    } finally {
      setLoading(false);
    }
  }, [tableCode]);

  const handlePaymentComplete = useCallback(async () => {
    try {
      console.log('Payment completed, reloading table data...');
      // Reload table data directly instead of calling loadTableData
      const tableResponse = await getTableByCode(tableCode);
      setTableData(tableResponse);

      try {
        const billResponse = await getOpenBillByTableCode(tableCode);
        setCurrentBill(billResponse);
      } catch {
        setCurrentBill(null);
      }
    } catch (error) {
      console.error('Error reloading after payment:', error);
    }
  }, [tableCode]);

  useEffect(() => {
    console.log('Bill page mounted, loading data for table:', tableCode);
    loadTableData();
  }, [tableCode]); // Only depend on tableCode, not the callback

  // Add debugging for any state changes
  useEffect(() => {
    console.log('Bill page state changed:', { 
      loading, 
      hasTableData: !!tableData, 
      hasBill: !!currentBill,
      tableCode 
    });
  }, [loading, tableData, currentBill, tableCode]);

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

  const { business } = tableData;

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
          onPaymentComplete={handlePaymentComplete}
        />
      </div>

      {/* Persistent Navigation */}
      <PersistentGuestNav 
        tableCode={tableCode}
        currentBill={currentBill}
      />
    </div>
  );
}
