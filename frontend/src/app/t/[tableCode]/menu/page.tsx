'use client';

import React, { useState, useEffect, useCallback } from 'react';
import { Card, CardBody, CardHeader, Button, Spinner, Chip } from '@nextui-org/react';
import { ArrowLeft, Plus, ShoppingCart } from 'lucide-react';
import { useParams, useRouter } from 'next/navigation';
import Link from 'next/link';
import GuestMenu from '../../../../components/guest/GuestMenu';
import GuestNavigation from '../../../../components/navigation/GuestNavigation';
import { axiosInstance } from '../../../../api/tools/instance';
import { addBillItem, BillResponse, getTableByCode, getOpenBillByTableCode } from '../../../../api/bills';
import { Business, MenuCategory } from '../../../../api/business';
import { getTableInfoEndpoint, getCurrentBillEndpoint } from '../../../../api/standardizedEndpoints';

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
    <div className="min-h-screen bg-default-50">
      {/* Header */}
      <div className="bg-white shadow-sm border-b">
      </div>

      {/* Menu Content */}
      <div className="max-w-4xl mx-auto px-4 py-6">
        <GuestMenu
          categories={categories}
          business={business}
          tableCode={tableCode}
          currentBill={currentBill}
          onAddToBill={handleAddToBill}
        />
      </div>
    </div>
  );
}
