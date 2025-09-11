'use client';

import React from 'react';
import { Button, Card, CardBody } from '@nextui-org/react';
import { QrCode, Menu, Receipt, ArrowLeft } from 'lucide-react';
import Link from 'next/link';
import { useRouter } from 'next/navigation';

interface GuestNavigationProps {
  tableCode: string;
  currentPage?: 'table' | 'menu' | 'bill' | 'scan';
  showBackButton?: boolean;
}

export const GuestNavigation: React.FC<GuestNavigationProps> = ({
  tableCode,
  currentPage = 'table',
  showBackButton = false,
}) => {
  const router = useRouter();

  const navigationItems = [
    {
      key: 'table',
      label: 'Table View',
      icon: <QrCode size={20} />,
      href: `/t/${tableCode}`,
      description: 'View table info and current bill'
    },
    {
      key: 'menu',
      label: 'Menu',
      icon: <Menu size={20} />,
      href: `/t/${tableCode}/menu`,
      description: 'Browse menu and add items'
    },
    {
      key: 'bill',
      label: 'Current Bill',
      icon: <Receipt size={20} />,
      href: `/t/${tableCode}/bill`,
      description: 'View and pay your bill'
    }
  ];

  return (
    <div className="w-full">
      {/* Back Button */}
      {showBackButton && (
        <div className="mb-4">
          <Button
            variant="light"
            startContent={<ArrowLeft size={16} />}
            onPress={() => router.back()}
            className="text-gray-600"
          >
            Back
          </Button>
        </div>
      )}

      {/* Navigation Cards */}
      <div className="grid grid-cols-1 sm:grid-cols-3 gap-4 mb-6">
        {navigationItems.map((item) => (
          <Link key={item.key} href={item.href}>
            <Card 
              isPressable
              className={`h-full transition-all duration-200 ${
                currentPage === item.key 
                  ? 'bg-primary-50 border-primary-200 shadow-md' 
                  : 'hover:bg-gray-50 hover:shadow-md'
              }`}
            >
              <CardBody className="p-4 text-center">
                <div className={`flex justify-center mb-2 ${
                  currentPage === item.key ? 'text-primary-600' : 'text-gray-600'
                }`}>
                  {item.icon}
                </div>
                <h3 className={`font-medium text-sm mb-1 ${
                  currentPage === item.key ? 'text-primary-700' : 'text-gray-900'
                }`}>
                  {item.label}
                </h3>
                <p className="text-xs text-gray-500">
                  {item.description}
                </p>
              </CardBody>
            </Card>
          </Link>
        ))}
      </div>

      {/* QR Scanner Link */}
      <div className="text-center">
        <Link href="/scan">
          <Button
            variant="bordered"
            startContent={<QrCode size={16} />}
            className="text-gray-600 border-gray-300"
          >
            Scan New QR Code
          </Button>
        </Link>
      </div>
    </div>
  );
};

export default GuestNavigation;
