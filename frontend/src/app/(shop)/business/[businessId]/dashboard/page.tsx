'use client';

import React, { useEffect, useState, useCallback } from 'react';
import { Card, CardBody, CardHeader, Button, Tabs, Tab, Spinner } from '@nextui-org/react';
import { Building2, Menu, Users, Receipt, Settings, BarChart3, AlertCircle } from 'lucide-react';
import { useParams, useRouter } from 'next/navigation';
import { useAppKitAccount } from '@reown/appkit/react';
import { useUserStore } from '@/store/useUserStore';
import { useAuth } from '@/hooks/useAuth';
import { getCookie } from '@/config/aws-s3/cookie-management/store.helpers';
import { isTokenValid, decodeJwt } from '@/utils/jwt';
import { getUserProfile } from '@/api/users/profile';
import { Business, getBusiness } from '../../../../../api/business';
import { UserInterface as User } from '@/interface/users/users-interface';
import MenuBuilder from '../../../../../components/business/MenuBuilder';
import TableManager from '../../../../../components/business/TableManager';
import { BillManager } from '../../../../../components/business/BillManager';
import BusinessSettings from '../../../../../components/business/BusinessSettings';

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

  const router = useRouter();
  const { address, isConnected } = useAppKitAccount();
  const { user } = useUserStore();
  const { isAuthenticated } = useAuth();
  const businessId = parseInt(params.businessId);

  // Authentication check
  useEffect(() => {
    const checkAuth = async () => {
      try {
        const token = getCookie("session_token");
        
        if (!token || !isTokenValid(token)) {
          // Wait a moment to avoid flashing error states
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
          // Try to fetch user data if not in store
          try {
            const userData = await getUserProfile(address);
            if (userData) {
              userData.role = tokenData.role || 'user';
              useUserStore.getState().setUser(userData);
            } else {
              // Create temporary user from token
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
            // If API fails, still create temp user from token
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
          setError("Authentication failed. Please try signing in again.");
          setAuthLoading(false);
        }, 1000);
      }
    };

    checkAuth();
  }, [user]);

  // All hooks must be called before any early returns
  const loadBusiness = useCallback(async () => {
    try {
      setLoading(true);
      setError(null);
      
      const businessData = await getBusiness(businessId);
      setBusiness(businessData);
      setLoading(false);
    } catch (error) {
      console.error('Error loading business:', error);
      let errorMessage = 'Failed to load business';
      
      if (error instanceof Error) {
        errorMessage = error.message;
      } else if (typeof error === 'object' && error !== null && 'status' in error) {
        const status = (error as any).status;
        if (status === 401) {
          errorMessage = 'Authentication required. Please sign in again.';
        } else if (status === 403) {
          errorMessage = 'Access denied. You may not own this business.';
        } else if (status === 404) {
          errorMessage = 'Business not found.';
        }
      }
      
      setError(errorMessage);
      setLoading(false);
    }
  }, [businessId]);

  useEffect(() => {
    // Only load business if we have a valid businessId AND authentication is complete
    if (!isNaN(businessId) && !authLoading) {
      loadBusiness();
    }
    
    // Set a timeout to prevent infinite loading
    const timeout = setTimeout(() => {
      if (loading && !business && !error && !authLoading) {
        setError('Loading timeout. Please check your connection and try again.');
        setLoading(false);
      }
    }, 15000); // 15 second timeout
    
    return () => clearTimeout(timeout);
  }, [loadBusiness, businessId, loading, business, error, authLoading]);


  // Show authentication error
  if (error && (error.includes("sign in") || error.includes("Authentication failed"))) {
    return (
      <div className="min-h-screen bg-white relative overflow-hidden">
        {/* Subtle animated background elements */}
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
            <h2 className="text-2xl font-light text-gray-900 mb-4 tracking-wide">Authentication Required</h2>
            <p className="text-gray-600 font-light tracking-wide mb-8">{error}</p>
            
            <div className="space-y-4">
              <p className="text-sm text-gray-500">Please authenticate to access your business dashboard.</p>
              <button
                onClick={() => window.location.reload()}
                className="px-6 py-3 text-sm font-medium text-white bg-gray-900 border border-transparent rounded-md hover:bg-gray-800 transition-colors duration-200"
              >
                Try Again
              </button>
            </div>
            
            <div className="mt-8">
              <button 
                onClick={() => router.push('/dashboard')}
                className="text-sm text-gray-500 hover:text-gray-700 transition-colors duration-200"
              >
                ← Back to Dashboard
              </button>
            </div>
          </div>
        </div>
      </div>
    );
  }

  // Check if businessId is valid - moved after all hooks
  if (isNaN(businessId)) {
    return (
      <div className="min-h-screen bg-gray-50 flex items-center justify-center">
        <Card className="max-w-md">
          <CardBody className="text-center">
            <AlertCircle className="mx-auto mb-4 text-danger-600" size={48} />
            <p className="text-danger-600 mb-4">Invalid business ID</p>
            <Button color="primary" onPress={() => router.push('/dashboard')}>
              Back to Dashboard
            </Button>
          </CardBody>
        </Card>
      </div>
    );
  }

  // Show loading state if either auth is loading OR business data is loading OR we don't have business data yet
  if (loading || authLoading || (!business && !error)) {
    return (
      <div className="min-h-screen bg-white relative overflow-hidden">
        {/* Subtle animated background elements */}
        <div className="absolute inset-0 overflow-hidden pointer-events-none">
          <div className="absolute -top-40 -right-40 w-80 h-80 bg-gradient-to-br from-blue-50 to-purple-50 rounded-full blur-3xl opacity-30 animate-pulse"></div>
          <div className="absolute -bottom-40 -left-40 w-96 h-96 bg-gradient-to-tr from-gray-50 to-blue-50 rounded-full blur-3xl opacity-20 animate-pulse" style={{ animationDelay: '2s' }}></div>
        </div>
        
        <div className="relative z-10 flex items-center justify-center min-h-screen">
          <div className="text-center">
            <div className="w-16 h-16 bg-gray-50 rounded-2xl flex items-center justify-center mx-auto mb-6 border border-gray-100">
              <div className="w-8 h-8 border-2 border-gray-300 border-t-gray-900 rounded-full animate-spin"></div>
            </div>
            <p className="text-gray-600 font-light tracking-wide">
              {authLoading ? "Authenticating..." : "Loading business dashboard..."}
            </p>
          </div>
        </div>
      </div>
    );
  }

  // Only show error if we have an actual error AND we're not loading
  if (error && !loading) {
    return (
      <div className="min-h-screen bg-white relative overflow-hidden">
        {/* Subtle animated background elements */}
        <div className="absolute inset-0 overflow-hidden pointer-events-none">
          <div className="absolute -top-40 -right-40 w-80 h-80 bg-gradient-to-br from-blue-50 to-purple-50 rounded-full blur-3xl opacity-30 animate-pulse"></div>
          <div className="absolute -bottom-40 -left-40 w-96 h-96 bg-gradient-to-tr from-gray-50 to-blue-50 rounded-full blur-3xl opacity-20 animate-pulse" style={{ animationDelay: '2s' }}></div>
        </div>
        
        <div className="relative z-10 flex items-center justify-center min-h-screen">
          <div className="max-w-md mx-auto px-6">
            <div className="bg-white border border-gray-200 rounded-2xl p-8 text-center shadow-lg">
              <div className="w-16 h-16 bg-red-50 rounded-2xl flex items-center justify-center mx-auto mb-6 border border-red-100">
                <AlertCircle className="w-8 h-8 text-red-600" />
              </div>
              <h3 className="text-xl font-light text-gray-900 tracking-wide mb-4">Something went wrong</h3>
              <p className="text-gray-600 font-light mb-8">{error || 'Business not found'}</p>
              <button
                onClick={() => router.push('/dashboard')}
                className="bg-gray-900 text-white px-8 py-3 text-base font-medium hover:bg-gray-800 transition-all duration-200 tracking-wide rounded-lg"
              >
                Back to Dashboard
              </button>
            </div>
          </div>
        </div>
      </div>
    );
  }

  // Type guard: At this point, we know business is not null
  if (!business) {
    return (
      <div className="min-h-screen bg-gray-50 flex items-center justify-center">
        <div className="text-center">
          <Spinner size="lg" />
          <p className="text-gray-600 mt-4">Loading business dashboard...</p>
        </div>
      </div>
    );
  }

  const renderTabContent = () => {
    switch (activeTab) {
      case 'overview':
        return (
          <div className="space-y-8">
            {/* Business Overview Cards */}
            <div className="grid grid-cols-1 md:grid-cols-3 gap-6">
              <div className="bg-white border border-gray-200 rounded-2xl p-8 text-center shadow-sm hover:shadow-lg transition-shadow duration-300">
                <div className="w-16 h-16 bg-blue-50 rounded-2xl flex items-center justify-center mx-auto mb-6 border border-blue-100">
                  <Receipt className="w-8 h-8 text-blue-600" />
                </div>
                <h3 className="text-lg font-light text-gray-900 tracking-wide mb-2">Active Bills</h3>
                <p className="text-3xl font-light text-blue-600 tracking-wide">0</p>
              </div>
              
              <div className="bg-white border border-gray-200 rounded-2xl p-8 text-center shadow-sm hover:shadow-lg transition-shadow duration-300">
                <div className="w-16 h-16 bg-green-50 rounded-2xl flex items-center justify-center mx-auto mb-6 border border-green-100">
                  <Users className="w-8 h-8 text-green-600" />
                </div>
                <h3 className="text-lg font-light text-gray-900 tracking-wide mb-2">Tables</h3>
                <p className="text-3xl font-light text-green-600 tracking-wide">0</p>
              </div>
              
              <div className="bg-white border border-gray-200 rounded-2xl p-8 text-center shadow-sm hover:shadow-lg transition-shadow duration-300">
                <div className="w-16 h-16 bg-orange-50 rounded-2xl flex items-center justify-center mx-auto mb-6 border border-orange-100">
                  <BarChart3 className="w-8 h-8 text-orange-600" />
                </div>
                <h3 className="text-lg font-light text-gray-900 tracking-wide mb-2">Today&apos;s Revenue</h3>
                <p className="text-3xl font-light text-orange-600 tracking-wide">$0.00</p>
              </div>
            </div>

            {/* Quick Actions */}
            <div className="bg-white border border-gray-200 rounded-2xl p-8 shadow-sm">
              <h3 className="text-xl font-light text-gray-900 tracking-wide mb-8">Quick Actions</h3>
              <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-4 gap-4">
                <button
                  onClick={() => setActiveTab('bills')}
                  className="group flex items-center gap-3 p-4 bg-gray-50 rounded-xl hover:bg-gray-100 transition-all duration-200 hover:-translate-y-1"
                >
                  <Receipt className="w-5 h-5 text-gray-600 group-hover:scale-110 transition-transform duration-200" />
                  <span className="font-medium text-gray-900 tracking-wide">Create Bill</span>
                </button>
                <button
                  onClick={() => setActiveTab('menu')}
                  className="group flex items-center gap-3 p-4 bg-gray-50 rounded-xl hover:bg-gray-100 transition-all duration-200 hover:-translate-y-1"
                >
                  <Menu className="w-5 h-5 text-gray-600 group-hover:scale-110 transition-transform duration-200" />
                  <span className="font-medium text-gray-900 tracking-wide">Edit Menu</span>
                </button>
                <button
                  onClick={() => setActiveTab('tables')}
                  className="group flex items-center gap-3 p-4 bg-gray-50 rounded-xl hover:bg-gray-100 transition-all duration-200 hover:-translate-y-1"
                >
                  <Users className="w-5 h-5 text-gray-600 group-hover:scale-110 transition-transform duration-200" />
                  <span className="font-medium text-gray-900 tracking-wide">Manage Tables</span>
                </button>
                <button
                  onClick={() => setActiveTab('settings')}
                  className="group flex items-center gap-3 p-4 bg-gray-50 rounded-xl hover:bg-gray-100 transition-all duration-200 hover:-translate-y-1"
                >
                  <Settings className="w-5 h-5 text-gray-600 group-hover:scale-110 transition-transform duration-200" />
                  <span className="font-medium text-gray-900 tracking-wide">Settings</span>
                </button>
              </div>
            </div>

            {/* Business Info */}
            <div className="bg-white border border-gray-200 rounded-2xl p-8 shadow-sm">
              <h3 className="text-xl font-light text-gray-900 tracking-wide mb-8">Business Information</h3>
              <div className="grid grid-cols-1 md:grid-cols-2 gap-8">
                <div>
                  <h4 className="font-medium text-gray-900 mb-3 tracking-wide">Settlement Address</h4>
                  <p className="font-mono text-sm text-gray-600 break-all bg-gray-50 p-4 rounded-xl border border-gray-100">
                    {business?.settlement_address || 'Not set'}
                  </p>
                </div>
                <div>
                  <h4 className="font-medium text-gray-900 mb-3 tracking-wide">Tipping Address</h4>
                  <p className="font-mono text-sm text-gray-600 break-all bg-gray-50 p-4 rounded-xl border border-gray-100">
                    {business?.tipping_address || 'Not set'}
                  </p>
                </div>
                <div>
                  <h4 className="font-medium text-gray-900 mb-3 tracking-wide">Tax Rate</h4>
                  <p className="text-gray-600 text-lg font-light">{business?.tax_rate || 0}%</p>
                </div>
                <div>
                  <h4 className="font-medium text-gray-900 mb-3 tracking-wide">Service Fee</h4>
                  <p className="text-gray-600 text-lg font-light">{business?.service_fee_rate || 0}%</p>
                </div>
              </div>
            </div>
          </div>
        );

      case 'menu':
        return <MenuBuilder businessId={businessId} />;

      case 'tables':
        return <TableManager businessId={businessId} />;

      case 'bills':
        return <BillManager businessId={businessId} />;

      case 'settings':
        return <BusinessSettings businessId={businessId} />;

      default:
        return null;
    }
  };

  return (
    <div className="min-h-screen bg-white relative overflow-hidden">
      {/* Subtle animated background elements */}
      <div className="absolute inset-0 overflow-hidden pointer-events-none">
        <div className="absolute -top-40 -right-40 w-80 h-80 bg-gradient-to-br from-blue-50 to-purple-50 rounded-full blur-3xl opacity-30 animate-pulse"></div>
        <div className="absolute -bottom-40 -left-40 w-96 h-96 bg-gradient-to-tr from-gray-50 to-blue-50 rounded-full blur-3xl opacity-20 animate-pulse" style={{ animationDelay: '2s' }}></div>
      </div>

      <div className="relative z-10 max-w-6xl mx-auto px-6 py-16">
        {/* Header */}
        <section className="mb-16">
          <div className="text-center mb-8">
            <div className="inline-flex items-center gap-2 bg-gray-50 border border-gray-200 px-4 py-2 rounded-full text-sm text-gray-600 mb-6 hover:bg-gray-100 transition-colors duration-200">
              <div className={`w-2 h-2 rounded-full animate-pulse ${
                business.is_active ? 'bg-green-400' : 'bg-gray-400'
              }`}></div>
              {business.is_active ? 'Business Active' : 'Business Inactive'}
            </div>
            
            <h1 className="text-4xl lg:text-6xl font-light text-gray-900 mb-6 tracking-wide leading-tight">
              {business.name}
            </h1>
            <p className="text-lg text-gray-600 font-light leading-relaxed max-w-2xl mx-auto">
              Manage your business operations and settings
            </p>
          </div>
        </section>

        {/* Navigation Tabs */}
        <section className="mb-12">
          <div className="bg-white border border-gray-200 rounded-2xl p-6 shadow-sm">
            <div className="flex gap-3 overflow-x-auto pb-2">
              {[
                { key: 'overview', icon: BarChart3, label: 'Overview' },
                { key: 'menu', icon: Menu, label: 'Menu' },
                { key: 'tables', icon: Users, label: 'Tables' },
                { key: 'bills', icon: Receipt, label: 'Bills' },
                { key: 'settings', icon: Settings, label: 'Settings' }
              ].map(({ key, icon: Icon, label }) => (
                <button
                  key={key}
                  onClick={() => setActiveTab(key)}
                  className={`flex-shrink-0 flex items-center gap-3 px-6 py-3 rounded-xl text-sm font-medium tracking-wide transition-all duration-200 ${
                    activeTab === key
                      ? 'bg-gray-900 text-white shadow-md'
                      : 'bg-gray-50 text-gray-600 hover:bg-gray-100 hover:text-gray-900'
                  }`}
                >
                  <Icon className="w-5 h-5" />
                  <span>{label}</span>
                </button>
              ))}
            </div>
          </div>
        </section>

        {/* Tab Content */}
        <section className="min-h-[600px]">
          {renderTabContent()}
        </section>
      </div>
    </div>
  );
}
