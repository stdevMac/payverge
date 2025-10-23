'use client';

import React from 'react';
import { Button, Spinner } from '@nextui-org/react';
import { Menu, AlertCircle } from 'lucide-react';
import { useSimpleLocale, getTranslation } from '@/i18n/SimpleTranslationProvider';
import { Business } from '@/api/business';
import DashboardSidebar from './DashboardSidebar';
import BusinessRegistrationSuccess from './BusinessRegistrationSuccess';

interface DashboardLayoutProps {
  business: Business | null;
  loading: boolean;
  authLoading: boolean;
  error: string | null;
  activeTab: string;
  setActiveTab: (tab: string) => void;
  sidebarOpen: boolean;
  setSidebarOpen: (open: boolean) => void;
  showWelcome: boolean;
  setShowWelcome: (show: boolean) => void;
  isFirstVisit: boolean;
  children: React.ReactNode;
  isConnected: boolean;
}

export default function DashboardLayout({
  business,
  loading,
  authLoading,
  error,
  activeTab,
  setActiveTab,
  sidebarOpen,
  setSidebarOpen,
  showWelcome,
  setShowWelcome,
  isFirstVisit,
  children,
  isConnected
}: DashboardLayoutProps) {
  // Translation setup
  const { locale } = useSimpleLocale();
  const [currentLocale, setCurrentLocale] = React.useState(locale);
  
  React.useEffect(() => {
    setCurrentLocale(locale);
  }, [locale]);
  
  const tString = (key: string): string => {
    const fullKey = `businessDashboard.${key}`;
    const result = getTranslation(fullKey, currentLocale);
    return Array.isArray(result) ? result[0] || key : result as string;
  };

  // Show loading state
  if (authLoading || loading) {
    return (
      <div className="min-h-screen bg-gray-50 flex items-center justify-center">
        <div className="text-center">
          <Spinner size="lg" className="mb-4" />
          <p className="text-gray-600">
            {authLoading ? tString('loading.authenticating') : tString('loading.business')}
          </p>
        </div>
      </div>
    );
  }

  // Show authentication error
  if (error && (error.indexOf("sign in") !== -1 || error.indexOf("connect your wallet") !== -1)) {
    return (
      <div className="min-h-screen bg-white relative overflow-hidden">
        <div className="absolute inset-0 overflow-hidden pointer-events-none">
          <div className="absolute -top-40 -right-40 w-80 h-80 bg-gradient-to-br from-blue-50 to-purple-50 rounded-full blur-3xl opacity-30 animate-pulse"></div>
          <div className="absolute -bottom-40 -left-40 w-96 h-96 bg-gradient-to-tr from-gray-50 to-blue-50 rounded-full blur-3xl opacity-20 animate-pulse" style={{ animationDelay: '2s' }}></div>
        </div>
        
        <div className="relative z-10 flex items-center justify-center min-h-screen">
          <div className="text-center max-w-md mx-auto">
            <div className="w-16 h-16 bg-red-50 rounded-2xl flex items-center justify-center mx-auto mb-6 border border-red-100">
              <svg className="w-8 h-8 text-red-500" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M12 15v2m-6 4h12a2 2 0 002-2v-6a2 2 0 00-2-2H6a2 2 0 00-2 2v6a2 2 0 002 2zm10-10V7a4 4 0 00-8 0v4h8z" />
              </svg>
            </div>
            <h2 className="text-2xl font-light text-gray-900 mb-4 tracking-wide">
              {tString('authentication.title')}
            </h2>
            <p className="text-gray-600 font-light tracking-wide mb-8">{error}</p>
            
            {!isConnected ? (
              <div className="space-y-4">
                <p className="text-sm text-gray-500">{tString('authentication.connectMessage')}</p>
                <div className="flex justify-center">
                  <w3m-button />
                </div>
              </div>
            ) : (
              <div className="space-y-4">
                <p className="text-sm text-gray-500">{tString('authentication.signMessage')}</p>
                <button
                  onClick={() => window.location.reload()}
                  className="px-6 py-3 text-sm font-medium text-white bg-gray-900 border border-transparent rounded-md hover:bg-gray-800 transition-colors duration-200"
                >
                  {tString('authentication.tryAgain')}
                </button>
              </div>
            )}
            
            <div className="mt-8">
              <a 
                href="/"
                className="text-sm text-gray-500 hover:text-gray-700 transition-colors duration-200"
              >
                ← {tString('authentication.backToHome')}
              </a>
            </div>
          </div>
        </div>
      </div>
    );
  }

  // Show other errors
  if (error) {
    return (
      <div className="min-h-screen bg-gray-50 flex items-center justify-center">
        <div className="text-center max-w-md mx-auto">
          <div className="w-16 h-16 bg-red-50 rounded-2xl flex items-center justify-center mx-auto mb-6 border border-red-100">
            <AlertCircle className="w-8 h-8 text-red-500" />
          </div>
          <h2 className="text-xl font-medium text-gray-900 mb-4">
            {tString('error.title')}
          </h2>
          <p className="text-gray-600 mb-6">{error}</p>
          <Button
            color="primary"
            onPress={() => window.location.reload()}
          >
            {tString('error.retry')}
          </Button>
        </div>
      </div>
    );
  }

  // Show business not found
  if (!business) {
    return (
      <div className="min-h-screen bg-gray-50 flex items-center justify-center">
        <div className="text-center">
          <p className="text-gray-600">{tString('error.businessNotFound')}</p>
        </div>
      </div>
    );
  }

  return (
    <div className="min-h-screen bg-gray-50">
      <div className="flex h-screen">
        {/* Sidebar */}
        <DashboardSidebar
          business={business}
          activeTab={activeTab}
          setActiveTab={setActiveTab}
          sidebarOpen={sidebarOpen}
          setSidebarOpen={setSidebarOpen}
        />

        {/* Main Content */}
        <div className="flex-1 flex flex-col overflow-hidden">
          {/* Mobile Header */}
          <div className="lg:hidden bg-white border-b border-gray-200 px-4 py-3">
            <div className="flex items-center justify-between">
              <Button
                isIconOnly
                variant="light"
                size="sm"
                onPress={() => setSidebarOpen(true)}
              >
                <Menu className="w-5 h-5" />
              </Button>
              <h1 className="font-semibold text-gray-900 truncate">
                {business.name}
              </h1>
              <div className="w-8" /> {/* Spacer for centering */}
            </div>
          </div>

          {/* Content Area */}
          <main className="flex-1 overflow-y-auto">
            {children}
          </main>
        </div>
      </div>

      {/* Welcome Screen */}
      {business && showWelcome && (
        <div className="fixed inset-0 z-50 bg-white">
          <BusinessRegistrationSuccess
            businessName={business.name}
            businessId={business.id.toString()}
            variant="dashboard"
            onClose={() => setShowWelcome(false)}
          />
        </div>
      )}
    </div>
  );
}
