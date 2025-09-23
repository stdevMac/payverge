'use client';

import React from 'react';
import { Card, CardBody, Chip } from '@nextui-org/react';
import { Menu, Receipt, ArrowLeft, Home } from 'lucide-react';
import Link from 'next/link';
import { useRouter, usePathname } from 'next/navigation';

interface PersistentGuestNavProps {
  tableCode: string;
  currentBill?: {
    bill: {
      total_amount: number;
    };
    items: any[];
  } | null;
  showBackToTable?: boolean;
}

export const PersistentGuestNav: React.FC<PersistentGuestNavProps> = ({
  tableCode,
  currentBill,
  showBackToTable = false,
}) => {
  const router = useRouter();
  const pathname = usePathname();

  const isMenuPage = pathname?.includes('/menu');
  const isBillPage = pathname?.includes('/bill');
  const isTablePage = !isMenuPage && !isBillPage;

  return (
    <div className="fixed bottom-0 left-0 right-0 z-50 bg-white/80 backdrop-blur-xl border-t border-gray-100 shadow-lg">
      <div className="max-w-4xl mx-auto px-6 py-4">
        <div className="flex items-center justify-center gap-8">
          {/* Back to Table */}
          <Link 
            href={`/t/${tableCode}`}
            onClick={() => console.log('Navigation: Clicking Table link to:', `/t/${tableCode}`)}
          >
            <div className={`group flex flex-col items-center gap-2 px-6 py-3 rounded-xl transition-all duration-300 ${
              isTablePage 
                ? 'bg-gray-100 text-gray-900 shadow-md border border-gray-200' 
                : 'text-gray-500 hover:text-gray-700 hover:bg-gray-50'
            }`}>
              <div className="relative">
                <Home className="w-6 h-6 group-hover:scale-110 transition-transform duration-300" />
                {isTablePage && (
                  <div className="absolute -top-1 -right-1 w-2 h-2 bg-blue-500 rounded-full animate-pulse"></div>
                )}
              </div>
              <span className="text-sm font-medium tracking-wide">Table</span>
            </div>
          </Link>

          {/* Menu */}
          <Link 
            href={`/t/${tableCode}/menu`}
            onClick={() => console.log('Navigation: Clicking Menu link to:', `/t/${tableCode}/menu`)}
          >
            <div className={`group flex flex-col items-center gap-2 px-6 py-3 rounded-xl transition-all duration-300 ${
              isMenuPage 
                ? 'bg-gray-100 text-gray-900 shadow-md border border-gray-200' 
                : 'text-gray-500 hover:text-gray-700 hover:bg-gray-50'
            }`}>
              <div className="relative">
                <Menu className="w-6 h-6 group-hover:scale-110 transition-transform duration-300" />
                {isMenuPage && (
                  <div className="absolute -top-1 -right-1 w-2 h-2 bg-blue-500 rounded-full animate-pulse"></div>
                )}
              </div>
              <span className="text-sm font-medium tracking-wide">Menu</span>
            </div>
          </Link>

          {/* Bill */}
          <Link href={`/t/${tableCode}/bill`}>
            <div className={`group flex flex-col items-center gap-2 px-6 py-3 rounded-xl transition-all duration-300 relative ${
              isBillPage 
                ? 'bg-gray-100 text-gray-900 shadow-md border border-gray-200' 
                : currentBill 
                  ? 'text-green-600 hover:text-green-700 hover:bg-green-50' 
                  : 'text-gray-300 cursor-not-allowed'
            }`}>
              <div className="relative">
                <Receipt className="w-6 h-6 group-hover:scale-110 transition-transform duration-300" />
                {currentBill && (
                  <div className="absolute -top-2 -right-2 w-5 h-5 bg-green-500 rounded-full flex items-center justify-center shadow-lg">
                    <span className="text-xs text-white font-bold">
                      {currentBill.items.length}
                    </span>
                  </div>
                )}
                {isBillPage && (
                  <div className="absolute -top-1 -right-1 w-2 h-2 bg-green-500 rounded-full animate-pulse"></div>
                )}
              </div>
              <span className="text-sm font-medium tracking-wide">
                {currentBill ? `$${currentBill.bill.total_amount.toFixed(0)}` : 'Bill'}
              </span>
            </div>
          </Link>
        </div>
      </div>
    </div>
  );
};

export default PersistentGuestNav;
