'use client';

import React, { useState, useEffect, useCallback } from 'react';
import { useParams } from 'next/navigation';
import { Card, CardBody, Spinner, Button } from '@nextui-org/react';
import { ArrowLeft, Menu } from 'lucide-react';
import Link from 'next/link';
import GuestBill from '../../../../components/guest/GuestBill';
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
      await loadTableData();
    } catch (error) {
      console.error('Error reloading after payment:', error);
    }
  }, [loadTableData]);

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

  const { business } = tableData;

  if (!currentBill) {
    return (
      <div className="min-h-screen bg-default-50">
        {/* Header */}
        <div className="bg-white shadow-sm border-b">
          <div className="max-w-4xl mx-auto px-4 py-4">
            <div className="flex items-center justify-between">
              <div className="flex items-center gap-3">
                <Link href={`/t/${tableCode}`}>
                  <Button
                    isIconOnly
                    variant="light"
                    size="sm"
                  >
                    <ArrowLeft className="w-4 h-4" />
                  </Button>
                </Link>
                <div>
                  <h1 className="text-xl font-bold">{business.name}</h1>
                  <p className="text-sm text-default-500">Bill</p>
                </div>
              </div>
            </div>
          </div>
        </div>

        {/* No Bill Content */}
        <div className="max-w-4xl mx-auto px-4 py-12">
          <Card>
            <CardBody className="text-center py-12">
              <div className="text-6xl mb-4">🧾</div>
              <h2 className="text-2xl font-bold mb-2">No Active Bill</h2>
              <p className="text-default-500 mb-6">
                There&apos;s no active bill for this table yet. Start by ordering from the menu.
              </p>
              <Link href={`/t/${tableCode}/menu`}>
                <Button
                  color="primary"
                  size="lg"
                  startContent={<Menu className="w-5 h-5" />}
                >
                  View Menu
                </Button>
              </Link>
            </CardBody>
          </Card>
        </div>
      </div>
    );
  }

  return (
    <div className="min-h-screen bg-default-50">
      {/* Header */}
      <div className="bg-white shadow-sm border-b">
        <div className="max-w-4xl mx-auto px-4 py-4">
          <div className="flex items-center justify-between">
            <div className="flex items-center gap-3">
              <Link href={`/t/${tableCode}`}>
                <Button
                  isIconOnly
                  variant="light"
                  size="sm"
                >
                  <ArrowLeft className="w-4 h-4" />
                </Button>
              </Link>
              <div>
                <h1 className="text-xl font-bold">{business.name}</h1>
                <p className="text-sm text-default-500">Bill #{currentBill.bill.bill_number}</p>
              </div>
            </div>
            
            <Link href={`/t/${tableCode}/menu`}>
              <Button
                color="primary"
                variant="flat"
                startContent={<Menu className="w-4 h-4" />}
              >
                Add Items
              </Button>
            </Link>
          </div>
        </div>
      </div>

      {/* Bill Content */}
      <div className="max-w-4xl mx-auto px-4 py-6">
        <GuestBill
          bill={currentBill}
          business={business}
          tableCode={tableCode}
          onPaymentComplete={handlePaymentComplete}
        />
      </div>
    </div>
  );
}
