import React from 'react';
import { GuestTableView } from '../../../components/guest/GuestTableView';
import { getTableByCode } from '../../../api/bills';

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
