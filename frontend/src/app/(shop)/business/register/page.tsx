'use client';

import React, { useState, useEffect } from 'react';
import { Card, CardBody, CardHeader, Button, Input, Checkbox, Divider, Spinner } from '@nextui-org/react';
import { ArrowLeft, Building2, Wallet, Settings } from 'lucide-react';
import Link from 'next/link';
import { useRouter } from 'next/navigation';
import { useAccount } from 'wagmi';
import { useUserStore } from '@/store/useUserStore';
import { useAuth } from '@/hooks/useAuth';
import { getCookie } from '@/config/aws-s3/cookie-management/store.helpers';
import { isTokenValid, decodeJwt } from '@/utils/jwt';
import { getUserProfile } from '@/api/users/profile';
import { createBusiness, CreateBusinessRequest } from '../../../../api/business';
import BusinessTutorialModal from '../../../../components/business/BusinessTutorialModal';
import { UserInterface as User } from '@/interface/users/users-interface';


export default function BusinessRegisterPage() {
  const router = useRouter();
  const { isConnected } = useAccount();
  const { user } = useUserStore();
  const { isAuthenticated } = useAuth();
  
  const [loading, setLoading] = useState(false);
  const [authLoading, setAuthLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [showTutorial, setShowTutorial] = useState(true);
  const [connectedWallet, setConnectedWallet] = useState<string | null>(null);
  const [formData, setFormData] = useState<CreateBusinessRequest>({
    name: "",
    logo: "",
    address: {
      street: "",
      city: "",
      state: "",
      postal_code: "",
      country: "",
    },
    settlement_address: "",
    tipping_address: "",
    tax_rate: 0,
    service_fee_rate: 0,
    tax_inclusive: false,
    service_inclusive: false,
  });

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

  // Update settlement address when wallet is connected
  useEffect(() => {
    if (connectedWallet) {
      setFormData(prev => ({
        ...prev,
        settlement_address: connectedWallet,
        tipping_address: connectedWallet, // Default to same address, user can change
      }));
    }
  }, [connectedWallet]);

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    setLoading(true);
    setError(null);

    try {
      const business = await createBusiness(formData);
      router.push(`/business/${business.id}/dashboard`);
    } catch (err: any) {
      setError(err.message || 'Failed to create business');
    } finally {
      setLoading(false);
    }
  };

  const updateFormData = (field: string, value: any) => {
    setFormData(prev => ({
      ...prev,
      [field]: value
    }));
  };

  const updateAddress = (field: string, value: string) => {
    setFormData(prev => ({
      ...prev,
      address: {
        ...prev.address,
        [field]: value
      }
    }));
  };

  const handleTutorialComplete = (walletAddress: string) => {
    setConnectedWallet(walletAddress);
    setShowTutorial(false);
  };

  const handleTutorialClose = () => {
    setShowTutorial(false);
  };

  // Show loading screen while checking authentication
  if (authLoading) {
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
            <p className="text-gray-600 font-light tracking-wide">Loading...</p>
          </div>
        </div>
      </div>
    );
  }

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
              <p className="text-sm text-gray-500">Please authenticate to register a business.</p>
              <button
                onClick={() => window.location.reload()}
                className="px-6 py-3 text-sm font-medium text-white bg-gray-900 border border-transparent rounded-md hover:bg-gray-800 transition-colors duration-200"
              >
                Try Again
              </button>
            </div>
            
            <div className="mt-8">
              <Link 
                href="/dashboard"
                className="text-sm text-gray-500 hover:text-gray-700 transition-colors duration-200"
              >
                ← Back to Dashboard
              </Link>
            </div>
          </div>
        </div>
      </div>
    );
  }

  return (
    <>
      {/* Tutorial Modal */}
      <BusinessTutorialModal
        isOpen={showTutorial}
        onClose={handleTutorialClose}
        onComplete={handleTutorialComplete}
      />

      <div className="min-h-screen bg-gray-50 py-8">
        <div className="max-w-2xl mx-auto px-4">
        {/* Header */}
        <div className="mb-8">
          <Link href="/dashboard">
            <Button
              variant="light"
              startContent={<ArrowLeft size={16} />}
              className="text-gray-600 mb-4"
            >
              Back to Dashboard
            </Button>
          </Link>
          <h1 className="text-3xl font-bold text-gray-900">Register Your Business</h1>
          <p className="text-gray-600 mt-2">
            Set up your business to start accepting crypto payments with Payverge
          </p>
        </div>

        {/* Error Message */}
        {error && (
          <Card className="mb-6 border-danger-200 bg-danger-50">
            <CardBody>
              <p className="text-danger-700">{error}</p>
            </CardBody>
          </Card>
        )}

        <form onSubmit={handleSubmit} className="space-y-8">
          {/* Basic Information */}
          <Card>
            <CardHeader className="flex gap-3">
              <Building2 className="text-primary-600" size={24} />
              <div>
                <h3 className="text-lg font-semibold">Basic Information</h3>
                <p className="text-sm text-gray-500">Tell us about your business</p>
              </div>
            </CardHeader>
            <CardBody className="space-y-4">
              <Input
                label="Business Name"
                placeholder="Enter your business name"
                value={formData.name}
                onChange={(e) => updateFormData('name', e.target.value)}
                isRequired
                variant="bordered"
              />
              
              <Input
                label="Logo URL (Optional)"
                placeholder="https://example.com/logo.png"
                value={formData.logo}
                onChange={(e) => updateFormData('logo', e.target.value)}
                variant="bordered"
              />

              <Divider />

              <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
                <Input
                  label="Street Address"
                  placeholder="123 Main St"
                  value={formData.address.street}
                  onChange={(e) => updateAddress('street', e.target.value)}
                  variant="bordered"
                />
                <Input
                  label="City"
                  placeholder="San Francisco"
                  value={formData.address.city}
                  onChange={(e) => updateAddress('city', e.target.value)}
                  variant="bordered"
                />
              </div>

              <div className="grid grid-cols-1 md:grid-cols-3 gap-4">
                <Input
                  label="State"
                  placeholder="CA"
                  value={formData.address.state}
                  onChange={(e) => updateAddress('state', e.target.value)}
                  variant="bordered"
                />
                <Input
                  label="Postal Code"
                  placeholder="94105"
                  value={formData.address.postal_code}
                  onChange={(e) => updateAddress('postal_code', e.target.value)}
                  variant="bordered"
                />
                <Input
                  label="Country"
                  placeholder="United States"
                  value={formData.address.country}
                  onChange={(e) => updateAddress('country', e.target.value)}
                  variant="bordered"
                />
              </div>
            </CardBody>
          </Card>

          {/* Wallet Configuration */}
          <Card>
            <CardHeader className="flex gap-3">
              <Wallet className="text-primary-600" size={24} />
              <div>
                <h3 className="text-lg font-semibold">Wallet Configuration</h3>
                <p className="text-sm text-gray-500">Configure where payments will be sent</p>
              </div>
            </CardHeader>
            <CardBody className="space-y-4">
              <Input
                label="Settlement Address"
                placeholder="0x..."
                value={formData.settlement_address}
                onChange={(e) => updateFormData('settlement_address', e.target.value)}
                description="Where business payments will be sent"
                isRequired
                variant="bordered"
                classNames={{
                  input: "font-mono text-sm"
                }}
              />
              
              <Input
                label="Tipping Address"
                placeholder="0x..."
                value={formData.tipping_address}
                onChange={(e) => updateFormData('tipping_address', e.target.value)}
                description="Where tips will be sent"
                isRequired
                variant="bordered"
                classNames={{
                  input: "font-mono text-sm"
                }}
              />
            </CardBody>
          </Card>

          {/* Tax & Service Fees */}
          <Card>
            <CardHeader className="flex gap-3">
              <Settings className="text-primary-600" size={24} />
              <div>
                <h3 className="text-lg font-semibold">Tax & Service Fees</h3>
                <p className="text-sm text-gray-500">Configure your pricing structure</p>
              </div>
            </CardHeader>
            <CardBody className="space-y-4">
              <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
                <Input
                  type="number"
                  label="Tax Rate (%)"
                  placeholder="8.5"
                  value={formData.tax_rate.toString()}
                  onChange={(e) => updateFormData('tax_rate', parseFloat(e.target.value) || 0)}
                  min="0"
                  max="100"
                  step="0.01"
                  variant="bordered"
                />
                
                <Input
                  type="number"
                  label="Service Fee (%)"
                  placeholder="18.0"
                  value={formData.service_fee_rate.toString()}
                  onChange={(e) => updateFormData('service_fee_rate', parseFloat(e.target.value) || 0)}
                  min="0"
                  max="100"
                  step="0.01"
                  variant="bordered"
                />
              </div>

              <div className="space-y-2">
                <Checkbox
                  isSelected={formData.tax_inclusive}
                  onValueChange={(checked) => updateFormData('tax_inclusive', checked)}
                >
                  Tax inclusive pricing
                </Checkbox>
                <Checkbox
                  isSelected={formData.service_inclusive}
                  onValueChange={(checked) => updateFormData('service_inclusive', checked)}
                >
                  Service fee inclusive pricing
                </Checkbox>
              </div>
            </CardBody>
          </Card>

          {/* Submit Button */}
          <div className="flex justify-end space-x-4">
            <Button
              variant="bordered"
              onPress={() => router.back()}
            >
              Cancel
            </Button>
            <Button
              type="submit"
              color="primary"
              isLoading={loading}
              isDisabled={!formData.name || !formData.settlement_address || !formData.tipping_address}
            >
              {loading ? 'Creating Business...' : 'Create Business'}
            </Button>
          </div>
        </form>
        </div>
      </div>
    </>
  );
}
