import React from 'react';
import dynamic from 'next/dynamic';
import { Spinner } from '@nextui-org/react';
import { getTableByCode } from '../../../api/bills';

// Lazy load the heavy GuestTableView component
const GuestTableView = dynamic(() => import('../../../components/guest/GuestTableView').then(mod => ({ default: mod.GuestTableView })), {
  loading: () => (
    <div className="min-h-screen bg-white flex items-center justify-center">
      <Spinner size="lg" />
    </div>
  )
});

interface TablePageProps {
  params: {
    tableCode: string;
  };
}

export default function TablePage({ params }: TablePageProps) {
  return (
    <div className="min-h-screen bg-white">
      <GuestTableView tableCode={params.tableCode} />
    </div>
  );
}

export async function generateMetadata({ params }: TablePageProps) {
  try {
    // Fetch table and business data for the title
    const tableData = await getTableByCode(params.tableCode);
    const businessName = tableData?.business?.name;
    
    return {
      title: businessName ? `${businessName} - Payverge` : 'Payverge',
      description: businessName 
        ? `View menu and place orders at ${businessName}` 
        : 'View menu and place orders at your table',
    };
  } catch (error) {
    // Fallback if we can't fetch the business data
    return {
      title: 'Payverge',
      description: 'View menu and place orders at your table',
    };
  }
}
