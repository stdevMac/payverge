'use client';

import React, { useEffect, useState, useCallback } from 'react';
import { Card, CardBody, CardHeader, Button, Tabs, Tab, Spinner } from '@nextui-org/react';
import { Building2, Menu, Users, Receipt, Settings, BarChart3, AlertCircle } from 'lucide-react';
import { useParams, useRouter } from 'next/navigation';
import { useAppKitAccount } from '@reown/appkit/react';
import { Business, getBusiness } from '../../../../api/business';
import MenuBuilder from '../../../../components/business/MenuBuilder';
import TableManager from '../../../../components/business/TableManager';
import { BillManager } from '../../../../components/business/BillManager';
import BusinessSettings from '../../../../components/business/BusinessSettings';

interface BusinessDashboardProps {
  params: {
    businessId: string;
  };
}

export default function BusinessDashboardPage({ params }: BusinessDashboardProps) {
  const [business, setBusiness] = useState<Business | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [activeTab, setActiveTab] = useState('overview');

  const router = useRouter();
  const { address, isConnected } = useAppKitAccount();
  const businessId = parseInt(params.businessId);

  // All hooks must be called before any early returns
  const loadBusiness = useCallback(async () => {
    try {
      setLoading(true);
      setError(null);
      
      // Check if user is connected before making API call
      if (!isConnected || !address) {
        throw new Error('Please connect your wallet to access business dashboard');
      }
      
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
          errorMessage = 'Authentication required. Please connect your wallet and sign in.';
        } else if (status === 403) {
          errorMessage = 'Access denied. You may not own this business.';
        } else if (status === 404) {
          errorMessage = 'Business not found.';
        }
      }
      
      setError(errorMessage);
      setLoading(false);
    }
  }, [businessId, isConnected, address]);

  useEffect(() => {
    // Only load business if we have a valid businessId
    if (!isNaN(businessId)) {
      loadBusiness();
    }
    
    // Set a timeout to prevent infinite loading
    const timeout = setTimeout(() => {
      if (loading && !business && !error) {
        setError('Loading timeout. Please check your connection and try again.');
        setLoading(false);
      }
    }, 15000); // 15 second timeout
    
    return () => clearTimeout(timeout);
  }, [loadBusiness, businessId, loading, business, error]);

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

  if (loading) {
    return (
      <div className="min-h-screen bg-gray-50 flex items-center justify-center">
        <div className="text-center">
          <Spinner size="lg" />
          <p className="text-gray-600 mt-4">Loading business dashboard...</p>
        </div>
      </div>
    );
  }

  if (error || (!loading && !business)) {
    return (
      <div className="min-h-screen bg-gray-50 flex items-center justify-center">
        <Card className="max-w-md">
          <CardBody className="text-center">
            <AlertCircle className="mx-auto mb-4 text-danger-600" size={48} />
            <p className="text-danger-600 mb-4">{error || 'Business not found'}</p>
            <div className="flex gap-2 justify-center">
              <Button color="primary" variant="flat" onPress={loadBusiness}>
                Try Again
              </Button>
              <Button color="primary" onPress={() => router.push('/dashboard')}>
                Back to Dashboard
              </Button>
            </div>
          </CardBody>
        </Card>
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
          <div className="space-y-6">
            {/* Business Overview Cards */}
            <div className="grid grid-cols-1 md:grid-cols-3 gap-6">
              <Card>
                <CardBody className="text-center">
                  <Receipt className="mx-auto mb-2 text-primary-600" size={32} />
                  <h3 className="text-lg font-semibold">Active Bills</h3>
                  <p className="text-2xl font-bold text-primary-600">0</p>
                </CardBody>
              </Card>
              
              <Card>
                <CardBody className="text-center">
                  <Users className="mx-auto mb-2 text-success-600" size={32} />
                  <h3 className="text-lg font-semibold">Tables</h3>
                  <p className="text-2xl font-bold text-success-600">0</p>
                </CardBody>
              </Card>
              
              <Card>
                <CardBody className="text-center">
                  <BarChart3 className="mx-auto mb-2 text-warning-600" size={32} />
                  <h3 className="text-lg font-semibold">Today&apos;s Revenue</h3>
                  <p className="text-2xl font-bold text-warning-600">$0.00</p>
                </CardBody>
              </Card>
            </div>

            {/* Quick Actions */}
            <Card>
              <CardHeader>
                <h3 className="text-lg font-semibold">Quick Actions</h3>
              </CardHeader>
              <CardBody>
                <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-4 gap-4">
                  <Button
                    color="primary"
                    variant="flat"
                    startContent={<Receipt size={16} />}
                    onPress={() => setActiveTab('bills')}
                  >
                    Create Bill
                  </Button>
                  <Button
                    color="secondary"
                    variant="flat"
                    startContent={<Menu size={16} />}
                    onPress={() => setActiveTab('menu')}
                  >
                    Edit Menu
                  </Button>
                  <Button
                    color="success"
                    variant="flat"
                    startContent={<Users size={16} />}
                    onPress={() => setActiveTab('tables')}
                  >
                    Manage Tables
                  </Button>
                  <Button
                    color="warning"
                    variant="flat"
                    startContent={<Settings size={16} />}
                    onPress={() => setActiveTab('settings')}
                  >
                    Settings
                  </Button>
                </div>
              </CardBody>
            </Card>

            {/* Business Info */}
            <Card>
              <CardHeader>
                <h3 className="text-lg font-semibold">Business Information</h3>
              </CardHeader>
              <CardBody>
                <div className="grid grid-cols-1 md:grid-cols-2 gap-6">
                  <div>
                    <h4 className="font-medium text-gray-900 mb-2">Settlement Address</h4>
                    <p className="font-mono text-sm text-gray-600 break-all">
                      {business?.settlement_address || 'Not set'}
                    </p>
                  </div>
                  <div>
                    <h4 className="font-medium text-gray-900 mb-2">Tipping Address</h4>
                    <p className="font-mono text-sm text-gray-600 break-all">
                      {business?.tipping_address || 'Not set'}
                    </p>
                  </div>
                  <div>
                    <h4 className="font-medium text-gray-900 mb-2">Tax Rate</h4>
                    <p className="text-gray-600">{business?.tax_rate || 0}%</p>
                  </div>
                  <div>
                    <h4 className="font-medium text-gray-900 mb-2">Service Fee</h4>
                    <p className="text-gray-600">{business?.service_fee_rate || 0}%</p>
                  </div>
                </div>
              </CardBody>
            </Card>
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
    <div className="min-h-screen bg-gray-50">
      <div className="max-w-7xl mx-auto px-4 sm:px-6 lg:px-8 py-8">
        {/* Header */}
        <div className="mb-8">
          <div className="flex items-center gap-3 mb-2">
            <Building2 className="text-primary-600" size={32} />
            <h1 className="text-3xl font-bold text-gray-900">{business.name}</h1>
            <div className={`px-3 py-1 rounded-full text-sm font-medium ${
              business.is_active 
                ? 'bg-success-100 text-success-800' 
                : 'bg-gray-100 text-gray-800'
            }`}>
              {business.is_active ? 'Active' : 'Inactive'}
            </div>
          </div>
          <p className="text-gray-600">Manage your business operations and settings</p>
        </div>

        {/* Navigation Tabs */}
        <div className="mb-8">
          <Tabs
            selectedKey={activeTab}
            onSelectionChange={(key) => setActiveTab(key as string)}
            variant="underlined"
            classNames={{
              tabList: "gap-6 w-full relative rounded-none p-0 border-b border-divider",
              cursor: "w-full bg-primary-500",
              tab: "max-w-fit px-0 h-12",
              tabContent: "group-data-[selected=true]:text-primary-500"
            }}
          >
            <Tab
              key="overview"
              title={
                <div className="flex items-center space-x-2">
                  <BarChart3 size={16} />
                  <span>Overview</span>
                </div>
              }
            />
            <Tab
              key="menu"
              title={
                <div className="flex items-center space-x-2">
                  <Menu size={16} />
                  <span>Menu</span>
                </div>
              }
            />
            <Tab
              key="tables"
              title={
                <div className="flex items-center space-x-2">
                  <Users size={16} />
                  <span>Tables</span>
                </div>
              }
            />
            <Tab
              key="bills"
              title={
                <div className="flex items-center space-x-2">
                  <Receipt size={16} />
                  <span>Bills</span>
                </div>
              }
            />
            <Tab
              key="settings"
              title={
                <div className="flex items-center space-x-2">
                  <Settings size={16} />
                  <span>Settings</span>
                </div>
              }
            />
          </Tabs>
        </div>

        {/* Tab Content */}
        <div className="min-h-[600px]">
          {renderTabContent()}
        </div>
      </div>
    </div>
  );
}
