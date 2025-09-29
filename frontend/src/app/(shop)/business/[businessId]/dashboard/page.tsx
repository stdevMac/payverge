'use client';

import React, { useEffect, useState, useCallback } from 'react';
import { Card, CardBody, CardHeader, Button, Tabs, Tab, Spinner } from '@nextui-org/react';
import { Building2, Menu, Users, Receipt, Settings, BarChart3, AlertCircle, X, UserCheck, ChefHat, Coffee } from 'lucide-react';
import { useParams, useRouter } from 'next/navigation';
import { useAppKitAccount } from '@reown/appkit/react';
import { useUserStore } from '@/store/useUserStore';
import { useAuth } from '@/hooks/useAuth';
import { getCookie } from '@/config/aws-s3/cookie-management/store.helpers';
import { isTokenValid, decodeJwt } from '@/utils/jwt';
import { getUserProfile } from '@/api/users/profile';
import { Business, getBusiness, getBusinessTables, Table } from '../../../../../api/business';
import { UserInterface as User } from '@/interface/users/users-interface';
import MenuBuilder from '../../../../../components/business/MenuBuilder';
import TableManager from '../../../../../components/business/TableManager';
import { BillManager } from '../../../../../components/business/BillManager';
import BusinessSettings from '../../../../../components/business/BusinessSettings';
import StaffManagement from '../../../../../components/business/StaffManagement';
import Kitchen from '../../../../../components/business/Kitchen';
import CounterManager from '../../../../../components/business/CounterManager';
import Dashboard from '../../../../../components/dashboard/Dashboard';
import { ToastProvider } from '../../../../../contexts/ToastContext';
import analyticsApi, { SalesData } from '../../../../../api/analytics';

interface BusinessDashboardProps {
  params: {
    businessId: string;
  };
}

export default function BusinessDashboardPage({ params }: BusinessDashboardProps) {
  const [business, setBusiness] = useState<Business | null>(null);
  const [loading, setLoading] = useState(true);
  const [authLoading, setAuthLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [activeTab, setActiveTab] = useState('overview');
  const [sidebarOpen, setSidebarOpen] = useState(false);
  const [salesData, setSalesData] = useState<SalesData | null>(null);
  const [analyticsLoading, setAnalyticsLoading] = useState(false);
  const [tables, setTables] = useState<Table[]>([]);

  const router = useRouter();
  const { address, isConnected } = useAppKitAccount();
  const { user } = useUserStore();
  const { isAuthenticated } = useAuth();
  const businessId = parseInt(params.businessId);

  // Fetch analytics data
  const fetchAnalytics = useCallback(async () => {
    if (!businessId) return;
    
    try {
      setAnalyticsLoading(true);
      const [salesResponse, tablesResponse] = await Promise.all([
        analyticsApi.getSalesAnalytics(businessId.toString(), 'today'),
        getBusinessTables(businessId)
      ]);
      setSalesData(salesResponse);
      setTables(tablesResponse.tables || []);
    } catch (error) {
      console.error('Error fetching analytics:', error);
    } finally {
      setAnalyticsLoading(false);
    }
  }, [businessId]);

  // Authentication check
  useEffect(() => {
    const checkAuth = async () => {
      try {
        const token = getCookie("session_token");
        
        if (!token || !isTokenValid(token)) {
          setTimeout(() => {
            setError("Please sign in to continue.");
            setAuthLoading(false);
          }, 1000);
          return;
        }

        const tokenData = decodeJwt(token);
        
        if (!tokenData.address) {
          setTimeout(() => {
            setError("Invalid session token.");
            setAuthLoading(false);
          }, 1000);
          return;
        }

        const address = tokenData.address.toLowerCase();
        
        if (!user) {
          try {
            const userData = await getUserProfile(address);
            if (userData) {
              userData.role = tokenData.role || 'user';
              useUserStore.getState().setUser(userData);
            } else {
              const tempUser: User = {
                username: "",
                email: "",
                address: address,
                role: tokenData.role || 'user',
                joined_at: new Date().toISOString(),
                language_selected: "en",
                referral_code: "",
                referrer: "",
                referees: {},
                notifications: [],
              };
              useUserStore.getState().setUser(tempUser);
            }
          } catch (apiError) {
            const tempUser: User = {
              username: "",
              email: "",
              address: address,
              role: tokenData.role || 'user',
              joined_at: new Date().toISOString(),
              language_selected: "en",
              referral_code: "",
              referrer: "",
              referees: {},
              notifications: [],
            };
            useUserStore.getState().setUser(tempUser);
          }
        }
        
        setAuthLoading(false);
      } catch (error) {
        setTimeout(() => {
          setError("Authentication failed.");
          setAuthLoading(false);
        }, 1000);
      }
    };

    checkAuth();
  }, [user]);

  // Fetch business data
  useEffect(() => {
    const fetchBusiness = async () => {
      if (authLoading) return;
      
      try {
        setLoading(true);
        const businessData = await getBusiness(businessId);
        setBusiness(businessData);
        // Fetch analytics data after business is loaded
        await fetchAnalytics();
      } catch (error) {
        console.error('Error fetching business:', error);
        setError('Failed to load business data');
      } finally {
        setLoading(false);
      }
    };

    if (!authLoading && !error) {
      fetchBusiness();
    }
  }, [businessId, authLoading, error]);

  // Error state
  if (error) {
    return (
      <div className="min-h-screen bg-gray-50 flex items-center justify-center">
        <div className="text-center">
          <AlertCircle className="w-16 h-16 text-red-500 mx-auto mb-4" />
          <h2 className="text-xl font-semibold text-gray-900 mb-2">Access Denied</h2>
          <p className="text-gray-600 mb-6">{error}</p>
          <Button color="primary" onClick={() => router.push('/auth/signin')}>
            Sign In
          </Button>
        </div>
      </div>
    );
  }

  // Loading state
  if (loading || authLoading) {
    return (
      <div className="min-h-screen bg-gray-50 flex items-center justify-center">
        <div className="text-center">
          <Spinner size="lg" />
          <p className="text-gray-600 mt-4">Loading business dashboard...</p>
        </div>
      </div>
    );
  }

  function renderTabContent() {
    switch (activeTab) {
      case 'overview':
        return (
          <div className="space-y-6 lg:space-y-8">
            {/* Business Overview Cards */}
            <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-4 lg:gap-6">
              <div className="bg-white border border-gray-200 rounded-2xl p-6 lg:p-8 text-center shadow-sm hover:shadow-lg transition-shadow duration-300">
                <div className="w-12 h-12 lg:w-16 lg:h-16 bg-blue-50 rounded-2xl flex items-center justify-center mx-auto mb-4 lg:mb-6 border border-blue-100">
                  <Receipt className="w-6 h-6 lg:w-8 lg:h-8 text-blue-600" />
                </div>
                <h3 className="text-base lg:text-lg font-light text-gray-900 tracking-wide mb-2">Active Bills</h3>
                <p className="text-2xl lg:text-3xl font-light text-blue-600 tracking-wide">
                  {analyticsLoading ? '...' : (salesData?.bill_count || 0)}
                </p>
              </div>
              
              <div className="bg-white border border-gray-200 rounded-2xl p-6 lg:p-8 text-center shadow-sm hover:shadow-lg transition-shadow duration-300">
                <div className="w-12 h-12 lg:w-16 lg:h-16 bg-green-50 rounded-2xl flex items-center justify-center mx-auto mb-4 lg:mb-6 border border-green-100">
                  <Users className="w-6 h-6 lg:w-8 lg:h-8 text-green-600" />
                </div>
                <h3 className="text-base lg:text-lg font-light text-gray-900 tracking-wide mb-2">Tables</h3>
                <p className="text-2xl lg:text-3xl font-light text-green-600 tracking-wide">
                  {analyticsLoading ? '...' : tables.length}
                </p>
              </div>
              
              <div className="bg-white border border-gray-200 rounded-2xl p-6 lg:p-8 text-center shadow-sm hover:shadow-lg transition-shadow duration-300 sm:col-span-2 lg:col-span-1">
                <div className="w-12 h-12 lg:w-16 lg:h-16 bg-orange-50 rounded-2xl flex items-center justify-center mx-auto mb-4 lg:mb-6 border border-orange-100">
                  <BarChart3 className="w-6 h-6 lg:w-8 lg:h-8 text-orange-600" />
                </div>
                <h3 className="text-base lg:text-lg font-light text-gray-900 tracking-wide mb-2">Today&apos;s Revenue</h3>
                <p className="text-2xl lg:text-3xl font-light text-orange-600 tracking-wide">
                  {analyticsLoading ? '...' : `$${(salesData?.total_revenue || 0).toFixed(2)}`}
                </p>
              </div>
            </div>

            {/* Business Info */}
            <div className="bg-white border border-gray-200 rounded-2xl p-6 lg:p-8 shadow-sm">
              <h3 className="text-lg lg:text-xl font-light text-gray-900 tracking-wide mb-6 lg:mb-8">Business Information</h3>
              <div className="grid grid-cols-1 sm:grid-cols-2 gap-6 lg:gap-8">
                <div>
                  <h4 className="font-medium text-gray-900 mb-3 tracking-wide text-sm lg:text-base">Settlement Address</h4>
                  <p className="font-mono text-xs lg:text-sm text-gray-600 break-all bg-gray-50 p-3 lg:p-4 rounded-xl border border-gray-100">
                    {business?.settlement_address || 'Not set'}
                  </p>
                </div>
                <div>
                  <h4 className="font-medium text-gray-900 mb-3 tracking-wide text-sm lg:text-base">Tipping Address</h4>
                  <p className="font-mono text-xs lg:text-sm text-gray-600 break-all bg-gray-50 p-3 lg:p-4 rounded-xl border border-gray-100">
                    {business?.tipping_address || 'Not set'}
                  </p>
                </div>
                <div>
                  <h4 className="font-medium text-gray-900 mb-3 tracking-wide text-sm lg:text-base">Tax Rate</h4>
                  <p className="text-gray-600 text-base lg:text-lg font-light">{business?.tax_rate || 0}%</p>
                </div>
                <div>
                  <h4 className="font-medium text-gray-900 mb-3 tracking-wide text-sm lg:text-base">Service Fee</h4>
                  <p className="text-gray-600 text-base lg:text-lg font-light">{business?.service_fee_rate || 0}%</p>
                </div>
              </div>
            </div>
          </div>
        );

      case 'analytics':
        return <Dashboard businessId={businessId.toString()} />;
      case 'menu':
        return <MenuBuilder businessId={businessId} />;

      case 'tables':
        return <TableManager businessId={businessId} />;

      case 'counters':
        return <CounterManager businessId={businessId} />;

      case 'bills':
        return <BillManager businessId={businessId} />;

      case 'kitchen':
        return <Kitchen businessId={businessId} />;

      case 'staff':
        return <StaffManagement businessId={businessId.toString()} />;

      case 'settings':
        return <BusinessSettings businessId={businessId} />;

      default:
        return null;
    }
  }

  return (
    <ToastProvider>
      <div className="min-h-screen bg-white">
      {/* Desktop Layout */}
      <div className="lg:flex">
        {/* Desktop Sidebar */}
        <div className="hidden lg:flex lg:flex-col lg:w-64 lg:fixed lg:inset-y-0 lg:bg-gray-50 lg:border-r lg:border-gray-200 lg:top-16 lg:h-[calc(100vh-4rem)] z-30">
          {/* Sidebar Header */}
          <div className="flex items-center gap-3 px-6 py-8 border-b border-gray-200">
            <Building2 className="w-8 h-8 text-gray-700" />
            <div className="min-w-0 flex-1">
              <h2 className="text-lg font-medium text-gray-900 tracking-wide truncate">
                {business?.name || 'Business'}
              </h2>
              <div className="flex items-center gap-2 mt-1">
                <div className={`w-2 h-2 rounded-full ${
                  business?.is_active ? 'bg-green-400' : 'bg-gray-400'
                }`}></div>
                <span className="text-xs text-gray-500">
                  {business?.is_active ? 'Active' : 'Inactive'}
                </span>
              </div>
            </div>
          </div>
          {/* Sidebar Navigation */}
          <nav className="flex-1 px-6 py-6 space-y-2">
            {[
              { key: 'overview', icon: BarChart3, label: 'Overview' },
              { key: 'analytics', icon: BarChart3, label: 'Analytics' },
              { key: 'menu', icon: Menu, label: 'Menu' },
              { key: 'tables', icon: Users, label: 'Tables' },
              { key: 'counters', icon: Coffee, label: 'Counters' },
              { key: 'bills', icon: Receipt, label: 'Bills' },
              { key: 'kitchen', icon: ChefHat, label: 'Kitchen' },
              { key: 'staff', icon: UserCheck, label: 'Staff' },
              { key: 'settings', icon: Settings, label: 'Settings' },
            ].map(({ key, icon: Icon, label }) => (
              <button
                key={key}
                onClick={() => setActiveTab(key)}
                className={`w-full flex items-center gap-3 px-4 py-3 rounded-xl text-sm font-medium tracking-wide transition-all duration-200 ${
                  activeTab === key
                    ? 'bg-gray-900 text-white shadow-md'
                    : 'text-gray-600 hover:bg-white hover:text-gray-900 hover:shadow-sm'
                }`}
              >
                <Icon className="w-5 h-5" />
                <span>{label}</span>
              </button>
            ))}
          </nav>
        </div>

        {/* Main Content */}
        <div className="lg:pl-64 flex-1 min-h-screen pt-16">
          {/* Subtle animated background elements */}
          <div className="absolute inset-0 overflow-hidden pointer-events-none">
            <div className="absolute -top-40 -right-40 w-80 h-80 bg-gradient-to-br from-blue-50 to-purple-50 rounded-full blur-3xl opacity-30 animate-pulse"></div>
            <div className="absolute -bottom-40 -left-40 w-96 h-96 bg-gradient-to-tr from-gray-50 to-blue-50 rounded-full blur-3xl opacity-20 animate-pulse" style={{ animationDelay: '2s' }}></div>
          </div>

          <div className="relative z-10 max-w-7xl mx-auto px-4 sm:px-6 lg:px-8 py-8 lg:py-16">
            {/* Mobile Header with Hamburger */}
            <div className="lg:hidden mb-6">
              <div className="flex items-center justify-between bg-white border border-gray-200 rounded-2xl p-4 shadow-sm">
                <div className="flex items-center gap-3">
                  <Building2 className="w-6 h-6 text-gray-700" />
                  <div>
                    <h2 className="text-base font-medium text-gray-900 tracking-wide truncate">
                      {business?.name || 'Dashboard'}
                    </h2>
                    <div className="flex items-center gap-2">
                      <div className={`w-1.5 h-1.5 rounded-full ${
                        business?.is_active ? 'bg-green-400' : 'bg-gray-400'
                      }`}></div>
                      <span className="text-xs text-gray-500">
                        {business?.is_active ? 'Active' : 'Inactive'}
                      </span>
                    </div>
                  </div>
                </div>
                <button
                  onClick={() => setSidebarOpen(true)}
                  className="p-2 text-gray-600 hover:text-gray-900 hover:bg-gray-100 rounded-lg transition-colors"
                >
                  <Menu className="w-6 h-6" />
                </button>
              </div>
            </div>

            {/* Mobile Sidebar Overlay */}
            {sidebarOpen && (
              <div className="lg:hidden fixed inset-0 z-[60] flex">
                <div className="fixed inset-0 bg-black bg-opacity-50" onClick={() => setSidebarOpen(false)} />
                <div className="relative flex flex-col w-64 bg-white shadow-xl">
                  <div className="flex items-center justify-between p-6 border-b border-gray-200">
                    <h2 className="text-lg font-medium text-gray-900 tracking-wide">Navigation</h2>
                    <button
                      onClick={() => setSidebarOpen(false)}
                      className="p-2 text-gray-600 hover:text-gray-900 hover:bg-gray-100 rounded-lg transition-colors"
                    >
                      <X className="w-5 h-5" />
                    </button>
                  </div>
                  <nav className="flex-1 p-6 space-y-2">
                    {[
                      { key: 'overview', icon: BarChart3, label: 'Overview' },
                      { key: 'analytics', icon: BarChart3, label: 'Analytics' },
                      { key: 'menu', icon: Menu, label: 'Menu' },
                      { key: 'tables', icon: Users, label: 'Tables' },
                      { key: 'counters', icon: Coffee, label: 'Counters' },
                      { key: 'bills', icon: Receipt, label: 'Bills' },
                      { key: 'staff', icon: UserCheck, label: 'Staff' },
                      { key: 'settings', icon: Settings, label: 'Settings' },
                    ].map(({ key, icon: Icon, label }) => (
                      <button
                        key={key}
                        onClick={() => {
                          setActiveTab(key);
                          setSidebarOpen(false);
                        }}
                        className={`w-full flex items-center gap-3 px-4 py-3 rounded-xl text-sm font-medium tracking-wide transition-all duration-200 ${
                          activeTab === key
                            ? 'bg-gray-900 text-white shadow-md'
                            : 'text-gray-600 hover:bg-gray-100 hover:text-gray-900'
                        }`}
                      >
                        <Icon className="w-5 h-5" />
                        <span>{label}</span>
                      </button>
                    ))}
                  </nav>
                </div>
              </div>
            )}

            {/* Desktop Header */}
            <section className="hidden lg:block mb-12">
              <div className="mb-8">
                <h1 className="text-4xl lg:text-5xl font-light text-gray-900 mb-4 tracking-wide leading-tight">
                  Dashboard
                </h1>
                <p className="text-lg text-gray-600 font-light leading-relaxed max-w-2xl">
                  Manage your business operations and settings
                </p>
              </div>
            </section>

            {/* Tab Content */}
            <section className="min-h-[400px]">
              {renderTabContent()}
            </section>
          </div>
        </div>
      </div>
    </div>
    </ToastProvider>
  );
}
