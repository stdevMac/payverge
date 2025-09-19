import React from 'react';
import { GuestTableView } from '../../../components/guest/GuestTableView';

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
  return {
    title: `Table ${params.tableCode} - Payverge`,
    description: 'View menu and place orders at your table',
  };
}
