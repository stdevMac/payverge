'use client';

import React, { useState, useEffect, useCallback } from 'react';
import { Card, CardBody, Button, Spinner, Image } from '@nextui-org/react';
import { ArrowLeft, MapPin } from 'lucide-react';
import { useParams } from 'next/navigation';
import Link from 'next/link';
import GuestMenu from '../../../../components/guest/GuestMenu';
import { PersistentGuestNav } from '../../../../components/navigation/PersistentGuestNav';
import { BillResponse, getTableByCode, getOpenBillByTableCode } from '../../../../api/bills';
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

export default function GuestMenuPage() {
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

  const handleAddToBill = useCallback(async (itemName: string, price: number, quantity: number = 1) => {
    if (!currentBill || !tableData?.business) return;
    
    try {
      const { addBillItem } = await import('../../../../api/bills');
      
      const addItemRequest = {
        menu_item_id: itemName,
        name: itemName,
        price: price,
        quantity,
        options: []
      };
      
      await addBillItem(currentBill.bill.id, addItemRequest);
      
      const billResponse = await getOpenBillByTableCode(tableCode);
      setCurrentBill(billResponse);
    } catch (error) {
      console.error('Error adding item to bill:', error);
    }
  }, [currentBill, tableData?.business, tableCode]);

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
            <h2 className="text-xl font-semibold mb-2">Table Not Found</h2>
            <p className="text-default-500">
              The table code &quot;{tableCode}&quot; could not be found.
            </p>
          </CardBody>
        </Card>
      </div>
    );
  }

  const { business, categories } = tableData;

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
            <Link href={`/t/${tableCode}`}>
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
                <p className="text-gray-500 font-light">Menu</p>
              </div>
            </div>
          </div>
        </div>
      </div>

      {/* Menu Content */}
      <div className="relative z-10 max-w-4xl mx-auto px-6 py-8 pb-28">
        <GuestMenu
          categories={categories}
          business={business}
          tableCode={tableCode}
          currentBill={currentBill}
          onAddToBill={handleAddToBill}
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
