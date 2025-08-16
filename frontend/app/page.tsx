'use client';

import { useState } from 'react';
import { useAccount } from 'wagmi';
import { ConnectButton } from '@rainbow-me/rainbowkit';
import { CreateInvoiceForm } from '../components/CreateInvoiceForm';
import { InvoiceDashboard } from '../components/InvoiceDashboard';
import { Header } from '../components/Header';
import { Hero } from '../components/Hero';

export default function HomePage() {
  const { address, isConnected } = useAccount();
  const [activeTab, setActiveTab] = useState<'create' | 'dashboard'>('create');

  if (!isConnected) {
    return (
      <div className="min-h-screen bg-gradient-to-br from-primary-50 to-blue-100">
        <Header />
        <Hero />
        <div className="container mx-auto px-4 py-16">
          <div className="max-w-md mx-auto text-center">
            <div className="card">
              <h2 className="text-2xl font-bold text-gray-900 mb-4">
                Connect Your Wallet
              </h2>
              <p className="text-gray-600 mb-6">
                Connect your wallet to start creating invoices and getting paid in USDC.
              </p>
              <ConnectButton />
            </div>
          </div>
        </div>
      </div>
    );
  }

  return (
    <div className="min-h-screen bg-gray-50">
      <Header />
      
      <div className="container mx-auto px-4 py-8">
        <div className="mb-8">
          <h1 className="text-3xl font-bold text-gray-900 mb-2">
            Invoice Generator
          </h1>
          <p className="text-gray-600">
            Create professional invoices and get paid instantly in USDC
          </p>
        </div>

        {/* Tab Navigation */}
        <div className="mb-8">
          <nav className="flex space-x-8">
            <button
              onClick={() => setActiveTab('create')}
              className={`py-2 px-1 border-b-2 font-medium text-sm ${
                activeTab === 'create'
                  ? 'border-primary-500 text-primary-600'
                  : 'border-transparent text-gray-500 hover:text-gray-700 hover:border-gray-300'
              }`}
            >
              Create Invoice
            </button>
            <button
              onClick={() => setActiveTab('dashboard')}
              className={`py-2 px-1 border-b-2 font-medium text-sm ${
                activeTab === 'dashboard'
                  ? 'border-primary-500 text-primary-600'
                  : 'border-transparent text-gray-500 hover:text-gray-700 hover:border-gray-300'
              }`}
            >
              My Invoices
            </button>
          </nav>
        </div>

        {/* Tab Content */}
        {activeTab === 'create' && (
          <div className="max-w-2xl mx-auto">
            <CreateInvoiceForm />
          </div>
        )}

        {activeTab === 'dashboard' && address && (
          <InvoiceDashboard creatorAddress={address} />
        )}
      </div>
    </div>
  );
}
