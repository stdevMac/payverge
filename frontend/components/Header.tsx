'use client';

import { ConnectButton } from '@rainbow-me/rainbowkit';
import { CreditCard } from 'lucide-react';

export function Header() {
  return (
    <header className="bg-white border-b border-gray-200">
      <div className="container mx-auto px-4 py-4">
        <div className="flex items-center justify-between">
          <div className="flex items-center space-x-2">
            <CreditCard className="h-8 w-8 text-primary-600" />
            <h1 className="text-xl font-bold text-gray-900">
              Invoice Generator
            </h1>
          </div>
          
          <div className="flex items-center space-x-4">
            <ConnectButton />
          </div>
        </div>
      </div>
    </header>
  );
}
