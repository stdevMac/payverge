'use client';

import React, { useState, useEffect } from 'react';
import { useSimpleLocale, getTranslation } from '@/i18n/SimpleTranslationProvider';
import { useRouter, useSearchParams } from 'next/navigation';
import { ToastProvider } from '../../../../../contexts/ToastContext';

// Custom hooks
import { useBusinessDashboard } from '@/hooks/useBusinessDashboard';

// Components
import DashboardLayout from '../../../../../components/business/DashboardLayout';
import MenuBuilder from '../../../../../components/business/MenuBuilder';
import TableManager from '../../../../../components/business/TableManager';
import { BillManager } from '../../../../../components/business/BillManager';
import BusinessSettings from '../../../../../components/business/BusinessSettings';
import StaffManagement from '../../../../../components/business/StaffManagement';
import Kitchen from '../../../../../components/business/Kitchen';
import CounterManager from '../../../../../components/business/CounterManager';
import BusinessOverview from '../../../../../components/business/BusinessOverview';
import Dashboard from '../../../../../components/dashboard/Dashboard';
import BlockchainManager from '../../../../../components/business/BlockchainManager';
import SubscriptionManagement from '../../../../../components/business/SubscriptionManagement';
import { useBusinessSubscriptionData } from '../../../../../contracts/hooks';

// Valid tab names for validation (moved outside component to avoid recreation)
const validTabs = ['overview', 'analytics', 'menu', 'tables', 'bills', 'staff', 'kitchen', 'counter', 'blockchain', 'subscriptions', 'settings'];

interface BusinessDashboardProps {
  params: {
    businessId: string;
  };
}

export default function BusinessDashboardPage({ params }: BusinessDashboardProps) {
  // Translation setup
  const { locale } = useSimpleLocale();
  const [currentLocale, setCurrentLocale] = useState(locale);
  
  useEffect(() => {
    setCurrentLocale(locale);
  }, [locale]);
  
  const tString = (key: string): string => {
    const fullKey = `businessDashboard.${key}`;
    const result = getTranslation(fullKey, currentLocale);
    return Array.isArray(result) ? result[0] || key : result as string;
  };

  // Navigation hooks
  const router = useRouter();
  const searchParams = useSearchParams();

  // State
  const [activeTab, setActiveTab] = useState('overview');
  const [sidebarOpen, setSidebarOpen] = useState(false);

  const businessId = parseInt(params.businessId);

  // Custom setActiveTab that syncs with URL
  const handleSetActiveTab = (tab: string) => {
    if (!validTabs.includes(tab)) return;
    
    setActiveTab(tab);
    
    // Update URL without page reload
    const url = new URL(window.location.href);
    if (tab === 'overview') {
      // Remove query param for default tab
      url.searchParams.delete('tab');
    } else {
      url.searchParams.set('tab', tab);
    }
    
    // Use replace to avoid adding to browser history for every tab change
    router.replace(url.pathname + url.search, { scroll: false });
  };

  // Read tab from URL on page load
  useEffect(() => {
    const tabFromUrl = searchParams.get('tab');
    if (tabFromUrl && validTabs.includes(tabFromUrl)) {
      setActiveTab(tabFromUrl);
    }
  }, [searchParams]);

  // Custom hooks
  const {
    business,
    loading,
    authLoading,
    error,
    showWelcome,
    isFirstVisit,
    setShowWelcome,
    refreshBusiness,
    user,
    isConnected,
    address
  } = useBusinessDashboard(businessId);

  // Fetch subscription data from smart contract
  const { 
    data: subscriptionData, 
    isLoading: subscriptionLoading, 
    refetch: refetchSubscription 
  } = useBusinessSubscriptionData(address as `0x${string}`, businessId);

  // Analytics hook removed since Dashboard component handles its own data fetching

  // Render tab content
  const renderTabContent = () => {
    if (!business) return null;

    switch (activeTab) {
      case 'overview':
        return business ? (
          <BusinessOverview 
            business={business} 
            onNavigateToTab={handleSetActiveTab}
          />
        ) : null;
      
      case 'analytics':
        return (
          <Dashboard
            businessId={businessId.toString()}
          />
        );
      
      case 'menu':
        return <MenuBuilder businessId={businessId} />;
      
      case 'tables':
        return <TableManager businessId={businessId} />;
      
      case 'bills':
        return <BillManager businessId={businessId} />;
      
      case 'staff':
        return <StaffManagement businessId={businessId.toString()} />;
      
      case 'kitchen':
        return <Kitchen businessId={businessId} />;
      
      case 'counter':
        return <CounterManager businessId={businessId} />;
      
      case 'blockchain':
        return (
          <BlockchainManager
            business={business}
            businessId={businessId}
            userAddress={address}
            isConnected={isConnected}
          />
        );
      
      case 'subscriptions':
        return (
          <div className="p-6">
            <div className="max-w-4xl mx-auto">
              <div className="mb-6">
                <h1 className="text-2xl font-bold text-gray-900 mb-2">
                  {tString('subscriptions.title')}
                </h1>
                <p className="text-gray-600">
                  {tString('subscriptions.description')}
                </p>
              </div>
              
              <SubscriptionManagement 
                subscriptionData={subscriptionData}
                onRenewSubscription={refetchSubscription}
                businessId={businessId}
              />
            </div>
          </div>
        );
      
      case 'settings':
        return (
          <BusinessSettings
            businessId={businessId}
          />
        );
      
      default:
        return (
          <div className="p-6">
            <div className="text-center text-gray-500">
              {tString('error.tabNotFound')}
            </div>
          </div>
        );
    }
  };

  return (
    <ToastProvider>
      <DashboardLayout
        business={business}
        loading={loading}
        authLoading={authLoading}
        error={error}
        activeTab={activeTab}
        setActiveTab={handleSetActiveTab}
        sidebarOpen={sidebarOpen}
        setSidebarOpen={setSidebarOpen}
        showWelcome={showWelcome}
        setShowWelcome={setShowWelcome}
        isFirstVisit={isFirstVisit}
        isConnected={isConnected}
      >
        {renderTabContent()}
      </DashboardLayout>
    </ToastProvider>
  );
}
