import React, { useState, useEffect } from 'react';
import { Card, CardBody, CardHeader, Button, Spinner, Chip } from '@nextui-org/react';
import { QrCode, Users, Clock, DollarSign } from 'lucide-react';
import { useParams } from 'next/navigation';
import { GuestTableView } from '../../../components/guest/GuestTableView';
import { GuestNavigation } from '../../../components/navigation/GuestNavigation';

interface TablePageProps {
  params: {
    tableCode: string;
  };
}

export default function TablePage({ params }: TablePageProps) {
  return (
    <div className="min-h-screen bg-gray-50 p-4">
      <div className="max-w-4xl mx-auto">
        <GuestNavigation 
          tableCode={params.tableCode} 
          currentPage="table"
        />
        <GuestTableView tableCode={params.tableCode} />
      </div>
    </div>
  );
}

export async function generateMetadata({ params }: TablePageProps) {
  return {
    title: `Table ${params.tableCode} - Payverge`,
    description: 'View menu and place orders at your table',
  };
}
