'use client';

import React, { useEffect, useState, useCallback } from 'react';
import { useSimpleLocale, getTranslation } from '@/i18n/SimpleTranslationProvider';
import { Card, CardBody, CardHeader, Button, Tabs, Tab, Spinner } from '@nextui-org/react';
import { Building2, Menu, Users, Receipt, Settings, BarChart3, AlertCircle, X, UserCheck, ChefHat, Coffee, Check, Wallet, DollarSign, Edit3, ExternalLink, CreditCard } from 'lucide-react';
import { useParams, useRouter } from 'next/navigation';
import Link from 'next/link';
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
import { 
  useBusinessInfo, 
  useGetClaimableAmounts, 
  useUpdateBusinessPaymentAddress, 
  useUpdateBusinessTippingAddress, 
  useClaimEarnings, 
  formatUsdcAmount 
} from '../../../../../contracts/hooks';
import { Address } from 'viem';
import WithdrawalHistory from '../../../../../components/business/WithdrawalHistory';
import { createWithdrawal } from '../../../../../api/withdrawals';
import SubscriptionManagement from '../../../../../components/business/SubscriptionManagement';
import BusinessRegistrationSuccess from '../../../../../components/business/BusinessRegistrationSuccess';

interface BusinessDashboardProps {
  params: {
    businessId: string;
  };
}

export default function BusinessDashboardPage({ params }: BusinessDashboardProps) {
  // Translation setup
  const { locale } = useSimpleLocale();
  const [currentLocale, setCurrentLocale] = useState(locale);
  
  // Update translations when locale changes
  useEffect(() => {
    setCurrentLocale(locale);
  }, [locale]);
  
  // Translation helper
  const tString = (key: string): string => {
    const fullKey = `businessDashboard.${key}`;
    const result = getTranslation(fullKey, currentLocale);
    return Array.isArray(result) ? result[0] || key : result as string;
  };

  const [business, setBusiness] = useState<Business | null>(null);
  const [loading, setLoading] = useState(true);
  const [authLoading, setAuthLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [activeTab, setActiveTab] = useState('overview');
  const [sidebarOpen, setSidebarOpen] = useState(false);
  const [salesData, setSalesData] = useState<SalesData | null>(null);
  const [analyticsLoading, setAnalyticsLoading] = useState(false);
  const [tables, setTables] = useState<Table[]>([]);
  const [showWelcome, setShowWelcome] = useState(false);
  const [isFirstVisit, setIsFirstVisit] = useState(false);
  
  // On-chain interaction state
  const [editingPaymentAddress, setEditingPaymentAddress] = useState(false);
  const [editingTippingAddress, setEditingTippingAddress] = useState(false);
  const [newPaymentAddress, setNewPaymentAddress] = useState('');
  const [newTippingAddress, setNewTippingAddress] = useState('');
  const [claimingEarnings, setClaimingEarnings] = useState(false);
  const [claimingStep, setClaimingStep] = useState<string>('');
  const [transactionHash, setTransactionHash] = useState<string>('');

  const router = useRouter();
  const { address, isConnected } = useAppKitAccount();
  const { user } = useUserStore();
  const { isAuthenticated } = useAuth();
  const businessId = parseInt(params.businessId);

  // Smart contract hooks - use business settlement address instead of user wallet address
  const { data: businessInfo, refetch: refetchBusinessInfo } = useBusinessInfo(business?.settlement_address as Address);
  
  // Get payment and tipping addresses from businessInfo or fallback to settlement address
  const paymentAddress = (businessInfo as any)?.paymentAddress || business?.settlement_address;
  const tippingAddress = (businessInfo as any)?.tippingAddress || business?.tipping_address;
  
  const { data: claimableBalance, refetch: refetchClaimableBalance } = useGetClaimableAmounts(
    paymentAddress as Address, 
    tippingAddress as Address
  );
  
  // Debug logging
  console.log('Business settlement address:', business?.settlement_address);
  console.log('Smart contract businessInfo:', businessInfo);
  console.log('Smart contract claimableBalance:', claimableBalance);
  const { updatePaymentAddress } = useUpdateBusinessPaymentAddress();
  const { updateTippingAddress } = useUpdateBusinessTippingAddress();
  const { claimEarnings } = useClaimEarnings();

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

  // On-chain interaction handlers
  const handleUpdatePaymentAddress = async () => {
    if (!newPaymentAddress) return;
    
    try {
      await updatePaymentAddress(newPaymentAddress as Address);
      setEditingPaymentAddress(false);
      setNewPaymentAddress('');
      refetchBusinessInfo();
    } catch (error) {
      console.error('Error updating payment address:', error);
    }
  };

  const handleUpdateTippingAddress = async () => {
    if (!newTippingAddress) return;
    
    try {
      await updateTippingAddress(newTippingAddress as Address);
      setEditingTippingAddress(false);
      setNewTippingAddress('');
      refetchBusinessInfo();
    } catch (error) {
      console.error('Error updating tipping address:', error);
    }
  };

  const handleClaimEarnings = async () => {
    if (!claimableBalance || !Array.isArray(claimableBalance) || 
        (claimableBalance[0] === BigInt(0) && claimableBalance[1] === BigInt(0))) {
      return;
    }

    setClaimingEarnings(true);
    setClaimingStep(tString('blockchain.claiming.preparing'));
    
    try {
      
      // Step 1: Initiate transaction
      setClaimingStep(tString('blockchain.claiming.confirmWallet'));
      const hash = await claimEarnings();
      setTransactionHash(hash);
      
      // Step 2: Transaction submitted
      setClaimingStep(tString('blockchain.claiming.submitted'));
      
      // Step 3: Create withdrawal record in backend
      try {
        const paymentAmount = parseFloat(formatUsdcAmount(claimableBalance[0] || BigInt(0)));
        const tipAmount = parseFloat(formatUsdcAmount(claimableBalance[1] || BigInt(0)));
        const totalAmount = paymentAmount + tipAmount;

        await createWithdrawal(businessId, {
          transaction_hash: hash,
          payment_amount: paymentAmount,
          tip_amount: tipAmount,
          total_amount: totalAmount,
          withdrawal_address: address || '',
          blockchain_network: 'base-sepolia' // or get from chain config
        });
        
        setClaimingStep(tString('blockchain.claiming.recording'));
      } catch (backendError) {
        console.error('Failed to record withdrawal in backend:', backendError);
        // Continue anyway - blockchain transaction is what matters
      }
      
      // Wait for a reasonable time for the transaction to be mined
      // In a real app, you'd use proper transaction receipt waiting
      await new Promise(resolve => setTimeout(resolve, 5000));
      
      // Step 4: Transaction confirmed (in a real app, you'd wait for actual confirmation)
      setClaimingStep(tString('blockchain.claiming.confirmed'));
      
      // Refetch the claimable balance
      await refetchClaimableBalance();
      await refetchBusinessInfo();
      
      setClaimingStep(tString('blockchain.claiming.success'));
      
      // Show success for 2 seconds then close
      setTimeout(() => {
        setClaimingEarnings(false);
        setClaimingStep('');
        setTransactionHash('');
      }, 2000);
      
    } catch (error: any) {
      console.error('Error claiming earnings:', error);
      setClaimingStep(`Error: ${error.message || 'Transaction failed'}`);
      
      // Show error for 5 seconds then close
      setTimeout(() => {
        setClaimingEarnings(false);
        setClaimingStep('');
        setTransactionHash('');
      }, 5000);
    }
  };

  // Authentication check
  useEffect(() => {
    const checkAuth = async () => {
      try {
        const token = getCookie("session_token");
        
        if (!token || !isTokenValid(token)) {
          setTimeout(() => {
            setError(tString('errors.signIn'));
            setAuthLoading(false);
          }, 1000);
          return;
        }

        const tokenData = decodeJwt(token);
        
        if (!tokenData.address) {
          setTimeout(() => {
            setError(tString('errors.invalidToken'));
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
          setError(tString('errors.authFailed'));
          setAuthLoading(false);
        }, 1000);
      }
    };

    checkAuth();
  }, [user]);

  // Check for welcome flow
  useEffect(() => {
    const urlParams = new URLSearchParams(window.location.search);
    const welcome = urlParams.get('welcome');
    const newBusiness = urlParams.get('new');
    
    if (welcome === 'true' || newBusiness === 'true') {
      setShowWelcome(true);
      setIsFirstVisit(true);
      // Clean URL
      window.history.replaceState({}, '', window.location.pathname);
    }
  }, []);

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
  }, [businessId, authLoading, error, fetchAnalytics]);

  // Error state
  if (error) {
    return (
      <div className="min-h-screen bg-gray-50 flex items-center justify-center">
        <div className="text-center">
          <AlertCircle className="w-16 h-16 text-red-500 mx-auto mb-4" />
          <h2 className="text-xl font-semibold text-gray-900 mb-2">{tString('errors.accessDenied')}</h2>
          <p className="text-gray-600 mb-6">{error}</p>
          <Button color="primary" onClick={() => router.push('/auth/signin')}>
            {tString('buttons.signIn')}
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
          <p className="text-gray-600 mt-4">{tString('loading')}</p>
        </div>
      </div>
    );
  }

  // Welcome screen for first-time visitors
  if (showWelcome && business) {
    return (
      <BusinessRegistrationSuccess
        businessName={business!.name}
        businessId={businessId.toString()}
        variant="dashboard"
        onClose={() => setShowWelcome(false)}
      />
    );
  }

  const renderTabContent = () => {
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
                <h3 className="text-base lg:text-lg font-light text-gray-900 tracking-wide mb-2">{tString('overview.activeBills')}</h3>
                <p className="text-2xl lg:text-3xl font-light text-blue-600 tracking-wide">
                  {analyticsLoading ? '...' : (salesData?.bill_count || 0)}
                </p>
              </div>
              
              <div className="bg-white border border-gray-200 rounded-2xl p-6 lg:p-8 text-center shadow-sm hover:shadow-lg transition-shadow duration-300">
                <div className="w-12 h-12 lg:w-16 lg:h-16 bg-green-50 rounded-2xl flex items-center justify-center mx-auto mb-4 lg:mb-6 border border-green-100">
                  <Users className="w-6 h-6 lg:w-8 lg:h-8 text-green-600" />
                </div>
                <h3 className="text-base lg:text-lg font-light text-gray-900 tracking-wide mb-2">{tString('overview.tables')}</h3>
                <p className="text-2xl lg:text-3xl font-light text-green-600 tracking-wide">
                  {analyticsLoading ? '...' : tables.length}
                </p>
              </div>
              
              <div className="bg-white border border-gray-200 rounded-2xl p-6 lg:p-8 text-center shadow-sm hover:shadow-lg transition-shadow duration-300 sm:col-span-2 lg:col-span-1">
                <div className="w-12 h-12 lg:w-16 lg:h-16 bg-orange-50 rounded-2xl flex items-center justify-center mx-auto mb-4 lg:mb-6 border border-orange-100">
                  <BarChart3 className="w-6 h-6 lg:w-8 lg:h-8 text-orange-600" />
                </div>
                <h3 className="text-base lg:text-lg font-light text-gray-900 tracking-wide mb-2">{tString('overview.todaysRevenue')}</h3>
                <p className="text-2xl lg:text-3xl font-light text-orange-600 tracking-wide">
                  {analyticsLoading ? '...' : `$${(salesData?.total_revenue || 0).toFixed(2)}`}
                </p>
              </div>
            </div>

            {/* Business Info */}
            <div className="bg-white border border-gray-200 rounded-2xl p-6 lg:p-8 shadow-sm">
              <h3 className="text-lg lg:text-xl font-light text-gray-900 tracking-wide mb-6 lg:mb-8">{tString('overview.businessInformation')}</h3>
              <div className="grid grid-cols-1 sm:grid-cols-2 gap-6 lg:gap-8">
                <div>
                  <h4 className="font-medium text-gray-900 mb-3 tracking-wide text-sm lg:text-base">{tString('overview.settlementAddress')}</h4>
                  <p className="font-mono text-xs lg:text-sm text-gray-600 break-all bg-gray-50 p-3 lg:p-4 rounded-xl border border-gray-100">
                    {business?.settlement_address || tString('overview.notSet')}
                  </p>
                </div>
                <div>
                  <h4 className="font-medium text-gray-900 mb-3 tracking-wide text-sm lg:text-base">{tString('overview.tippingAddress')}</h4>
                  <p className="font-mono text-xs lg:text-sm text-gray-600 break-all bg-gray-50 p-3 lg:p-4 rounded-xl border border-gray-100">
                    {business?.tipping_address || tString('overview.notSet')}
                  </p>
                </div>
                <div>
                  <h4 className="font-medium text-gray-900 mb-3 tracking-wide text-sm lg:text-base">{tString('overview.taxRate')}</h4>
                  <p className="text-gray-600 text-base lg:text-lg font-light">{business?.tax_rate || 0}%</p>
                </div>
                <div>
                  <h4 className="font-medium text-gray-900 mb-3 tracking-wide text-sm lg:text-base">{tString('overview.serviceFee')}</h4>
                  <p className="text-gray-600 text-base lg:text-lg font-light">{business?.service_fee_rate || 0}%</p>
                </div>
              </div>
            </div>
          </div>
        );

      case 'analytics':
        return <Dashboard businessId={businessId.toString()} />;

      case 'blockchain':
        return (
          <div className="space-y-8">
            {/* Header */}
            <div className="bg-white border border-gray-200 rounded-2xl p-8 shadow-sm">
              <div className="flex items-center gap-4 mb-6">
                <div className="w-12 h-12 bg-blue-50 rounded-xl flex items-center justify-center border border-blue-100">
                  <Wallet className="w-6 h-6 text-blue-600" />
                </div>
                <div>
                  <h3 className="text-2xl font-medium text-gray-900">{tString('blockchain.title')}</h3>
                  <p className="text-gray-600">{tString('blockchain.subtitle')}</p>
                </div>
              </div>

              {/* Status Cards */}
              <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
                <div className="flex items-center gap-3 p-4 bg-gray-50 rounded-xl border border-gray-100">
                  <div className={`w-3 h-3 rounded-full ${
                    isConnected ? 'bg-green-400 animate-pulse' : 'bg-red-400'
                  }`}></div>
                  <span className="text-sm font-medium text-gray-700">
                    {isConnected ? 'Wallet Connected' : 'Wallet Not Connected'}
                  </span>
                  {address && (
                    <span className="text-xs font-mono text-gray-500 bg-white px-2 py-1 rounded border ml-auto">
                      {address.slice(0, 6)}...{address.slice(-4)}
                    </span>
                  )}
                </div>
                
                <div className="flex items-center gap-3 p-4 bg-blue-50 rounded-xl border border-blue-100">
                  <div className={`w-3 h-3 rounded-full ${
                    businessInfo ? 'bg-green-400' : 'bg-yellow-400'
                  }`}></div>
                  <span className="text-sm font-medium text-gray-700">
                    {businessInfo ? 'Registered on Smart Contract' : 'Not Registered on Smart Contract'}
                  </span>
                  {business?.settlement_address && (
                    <span className="text-xs font-mono text-gray-500 bg-white px-2 py-1 rounded border ml-auto">
                      {business.settlement_address.slice(0, 6)}...{business.settlement_address.slice(-4)}
                    </span>
                  )}
                </div>
              </div>
            </div>

            {/* Claimable Balances */}
            <Card className="bg-white border border-gray-200 shadow-sm">
              <CardHeader className="pb-3">
                <div className="flex items-center gap-3">
                  <div className="w-12 h-12 bg-blue-50 rounded-xl flex items-center justify-center border border-blue-100">
                    <Wallet className="w-6 h-6 text-blue-600" />
                  </div>
                  <div>
                    <h4 className="text-lg font-medium text-gray-900">{tString('blockchain.claimableEarnings.title')}</h4>
                    <p className="text-sm text-gray-600">{tString('blockchain.claimableEarnings.subtitle')}</p>
                  </div>
                </div>
              </CardHeader>
              <CardBody className="pt-0">
                <div className="space-y-6">
                  {/* Payment and Tip Amounts */}
                  <div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
                    <div className="bg-green-50 border border-green-100 rounded-xl p-4">
                      <div className="flex items-center gap-2 mb-2">
                        <DollarSign className="w-5 h-5 text-green-600" />
                        <span className="text-sm font-medium text-green-800">{tString('blockchain.claimableEarnings.paymentEarnings')}</span>
                      </div>
                      <div className="text-2xl font-medium text-green-600">
                        {claimableBalance && Array.isArray(claimableBalance) ? 
                          `$${formatUsdcAmount(claimableBalance[0] || BigInt(0))}` : 
                          '$0.00'
                        }
                      </div>
                    </div>
                    
                    <div className="bg-orange-50 border border-orange-100 rounded-xl p-4">
                      <div className="flex items-center gap-2 mb-2">
                        <DollarSign className="w-5 h-5 text-orange-600" />
                        <span className="text-sm font-medium text-orange-800">{tString('blockchain.claimableEarnings.tipEarnings')}</span>
                      </div>
                      <div className="text-2xl font-medium text-orange-600">
                        {claimableBalance && Array.isArray(claimableBalance) ? 
                          `$${formatUsdcAmount(claimableBalance[1] || BigInt(0))}` : 
                          '$0.00'
                        }
                      </div>
                    </div>
                  </div>

                  {/* Total and Claim Button */}
                  <div className="border-t border-gray-200 pt-4">
                    <div className="flex items-center justify-between mb-4">
                      <span className="text-lg font-medium text-gray-900">{tString('blockchain.claimableEarnings.totalAvailable')}</span>
                      <span className="text-2xl font-medium text-blue-600">
                        {claimableBalance && Array.isArray(claimableBalance) ? 
                          `$${formatUsdcAmount((claimableBalance[0] || BigInt(0)) + (claimableBalance[1] || BigInt(0)))}` : 
                          '$0.00'
                        }
                      </span>
                    </div>
                    
                    <Button
                      size="lg"
                      className={`w-full ${
                        claimableBalance && Array.isArray(claimableBalance) && 
                        (claimableBalance[0] > BigInt(0) || claimableBalance[1] > BigInt(0))
                          ? 'bg-blue-600 text-white hover:bg-blue-700'
                          : 'bg-gray-100 text-gray-500 cursor-not-allowed'
                      }`}
                      onClick={handleClaimEarnings}
                      disabled={
                        claimingEarnings || 
                        !claimableBalance || 
                        !Array.isArray(claimableBalance) || 
                        (claimableBalance[0] === BigInt(0) && claimableBalance[1] === BigInt(0))
                      }
                      startContent={claimingEarnings ? <Spinner size="sm" /> : <Wallet className="w-5 h-5" />}
                    >
                      {(() => {
                        if (claimingEarnings) return tString('blockchain.claiming.preparing');
                        
                        if (!claimableBalance || !Array.isArray(claimableBalance) || 
                            (claimableBalance[0] === BigInt(0) && claimableBalance[1] === BigInt(0))) {
                          return tString('blockchain.warnings.noEarnings');
                        }
                        
                        return tString('buttons.claimEarnings');
                      })()}
                    </Button>
                  </div>
                </div>
              </CardBody>
            </Card>

            {/* Withdrawal History */}
            <WithdrawalHistory businessId={businessId} isAuthenticated={isAuthenticated} authLoading={authLoading} />

            {/* Transaction Processing Overlay */}
            {claimingEarnings && (
              <div className="fixed inset-0 bg-black/50 backdrop-blur-sm z-50 flex items-center justify-center">
                <Card className="w-full max-w-md mx-4">
                  <CardBody className="text-center py-8">
                    <div className="mb-6">
                      <div className="w-16 h-16 bg-blue-50 rounded-full flex items-center justify-center mx-auto mb-4">
                        <Spinner size="lg" color="primary" />
                      </div>
                      <h3 className="text-xl font-semibold text-gray-900 mb-2">
                        {tString('blockchain.claiming.processingTitle')}
                      </h3>
                      <p className="text-gray-600 mb-4">
                        {claimingStep}
                      </p>
                      {transactionHash && (
                        <div className="bg-gray-50 rounded-lg p-3 mb-4">
                          <p className="text-xs text-gray-500 mb-1">{tString('blockchain.claimableEarnings.transactionHash')}</p>
                          <p className="text-xs font-mono text-gray-700 break-all">
                            {transactionHash}
                          </p>
                        </div>
                      )}
                    </div>
                    
                    <div className="bg-yellow-50 border border-yellow-200 rounded-lg p-4">
                      <div className="flex items-center gap-2 text-yellow-800">
                        <AlertCircle className="w-5 h-5 flex-shrink-0" />
                        <div className="text-sm">
                          <p className="font-medium">{tString('blockchain.claiming.dontClose')}</p>
                          <p>{tString('blockchain.claiming.blockchainProcessing')}</p>
                        </div>
                      </div>
                    </div>
                  </CardBody>
                </Card>
              </div>
            )}

            {/* Business Addresses */}
            <div className="bg-white border border-gray-200 rounded-2xl p-8 shadow-sm">
              <h3 className="text-xl font-medium text-gray-900 mb-6">{tString('blockchain.addressManagement.title')}</h3>
              
              <div className="space-y-6">
                {/* Payment Address */}
                <div>
                  <div className="flex items-center justify-between mb-3">
                    <h4 className="font-medium text-gray-900">{tString('blockchain.addressManagement.paymentAddress')}</h4>
                    <Button
                      size="sm"
                      variant="ghost"
                      onClick={() => {
                        setEditingPaymentAddress(!editingPaymentAddress);
                        setNewPaymentAddress((businessInfo as any)?.paymentAddress || business?.settlement_address || '');
                      }}
                      startContent={<Edit3 className="w-4 h-4" />}
                    >
                      {tString('blockchain.addressManagement.edit')}
                    </Button>
                  </div>
                  
                  {editingPaymentAddress ? (
                    <div className="space-y-3">
                      <input
                        type="text"
                        value={newPaymentAddress}
                        onChange={(e) => setNewPaymentAddress(e.target.value)}
                        placeholder="0x..."
                        className="w-full px-3 py-2 border border-gray-300 rounded-lg font-mono text-sm focus:outline-none focus:ring-2 focus:ring-blue-500"
                      />
                      <div className="flex gap-2">
                        <Button
                          size="sm"
                          className="bg-blue-600 text-white hover:bg-blue-700"
                          onClick={handleUpdatePaymentAddress}
                        >
                          {tString('blockchain.addressManagement.update')}
                        </Button>
                        <Button
                          size="sm"
                          variant="ghost"
                          onClick={() => {
                            setEditingPaymentAddress(false);
                            setNewPaymentAddress('');
                          }}
                        >
                          {tString('buttons.cancel')}
                        </Button>
                      </div>
                    </div>
                  ) : (
                    <div className="font-mono text-sm text-gray-600 bg-gray-50 p-3 rounded-lg border border-gray-100 break-all">
                      {(businessInfo as any)?.paymentAddress || business?.settlement_address || 'Not set'}
                    </div>
                  )}
                </div>

                {/* Tipping Address */}
                <div>
                  <div className="flex items-center justify-between mb-3">
                    <h4 className="font-medium text-gray-900">{tString('blockchain.addressManagement.tippingAddress')}</h4>
                    <Button
                      size="sm"
                      variant="ghost"
                      onClick={() => {
                        setEditingTippingAddress(!editingTippingAddress);
                        setNewTippingAddress((businessInfo as any)?.tippingAddress || business?.tipping_address || '');
                      }}
                      startContent={<Edit3 className="w-4 h-4" />}
                    >
                      {tString('blockchain.addressManagement.edit')}
                    </Button>
                  </div>
                  
                  {editingTippingAddress ? (
                    <div className="space-y-3">
                      <input
                        type="text"
                        value={newTippingAddress}
                        onChange={(e) => setNewTippingAddress(e.target.value)}
                        placeholder="0x..."
                        className="w-full px-3 py-2 border border-gray-300 rounded-lg font-mono text-sm focus:outline-none focus:ring-2 focus:ring-blue-500"
                      />
                      <div className="flex gap-2">
                        <Button
                          size="sm"
                          className="bg-blue-600 text-white hover:bg-blue-700"
                          onClick={handleUpdateTippingAddress}
                        >
                          {tString('blockchain.addressManagement.update')}
                        </Button>
                        <Button
                          size="sm"
                          variant="ghost"
                          onClick={() => {
                            setEditingTippingAddress(false);
                            setNewTippingAddress('');
                          }}
                        >
                          {tString('buttons.cancel')}
                        </Button>
                      </div>
                    </div>
                  ) : (
                    <div className="font-mono text-sm text-gray-600 bg-gray-50 p-3 rounded-lg border border-gray-100 break-all">
                      {(businessInfo as any)?.tippingAddress || business?.tipping_address || 'Not set'}
                    </div>
                  )}
                </div>
              </div>
            </div>

            {/* On-Chain Statistics
            <div className="bg-white border border-gray-200 rounded-2xl p-8 shadow-sm">
              <h3 className="text-xl font-medium text-gray-900 mb-6">On-Chain Statistics</h3>
              
              <div className="grid grid-cols-1 sm:grid-cols-3 gap-6">
                <div className="text-center">
                  <div className="text-2xl font-medium text-blue-600 mb-2">
                    ${(businessInfo as any)?.totalVolume ? formatUsdcAmount((businessInfo as any).totalVolume) : '0'}
                  </div>
                  <div className="text-sm text-gray-600">Total Volume</div>
                </div>
                
                <div className="text-center">
                  <div className="text-2xl font-medium text-green-600 mb-2">
                    ${(businessInfo as any)?.totalTips ? formatUsdcAmount((businessInfo as any).totalTips) : '0'}
                  </div>
                  <div className="text-sm text-gray-600">Total Tips</div>
                </div>
                
                <div className="text-center">
                  <div className="text-2xl font-medium text-gray-600 mb-2">
                    {(businessInfo as any)?.isActive ? 'Active' : 'Inactive'}
                  </div>
                  <div className="text-sm text-gray-600">Status</div>
                </div>
              </div>
            </div> */}
          </div>
        );

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

      case 'subscription':
        return <SubscriptionManagement 
          subscriptionData={{
            status: business?.subscription_status as 'active' | 'expired' | 'suspended' | 'cancelled' || 'active',
            lastPaymentDate: business?.last_payment_date || '',
            subscriptionEndDate: business?.subscription_end_date || '',
            lastPaymentAmount: business?.last_payment_amount || '0',
            totalPaid: business?.total_paid || '0',
            yearlyFee: business?.yearly_fee || '120000000',
            timeRemaining: business?.time_remaining || 0,
            remindersSent: business?.reminders_sent || 0,
          }}
          onRenewSubscription={() => {
            // TODO: Implement renewal logic
            console.log('Renew subscription');
          }}
        />;

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
                  {business?.is_active ? tString('overview.status.active') : tString('overview.status.inactive')}
                </span>
              </div>
            </div>
          </div>
          {/* Sidebar Navigation */}
          <nav className="flex-1 px-6 py-6 space-y-2">
            {[
              { key: 'overview', icon: BarChart3, label: tString('tabs.overview') },
              { key: 'analytics', icon: BarChart3, label: tString('tabs.analytics') },
              { key: 'blockchain', icon: Wallet, label: tString('blockchain.title') },
              { key: 'subscription', icon: CreditCard, label: tString('tabs.subscription') },
              { key: 'menu', icon: Menu, label: tString('tabs.menu') },
              { key: 'tables', icon: Users, label: tString('tabs.tables') },
              { key: 'counters', icon: Coffee, label: tString('tabs.counter') },
              { key: 'bills', icon: Receipt, label: tString('tabs.bills') },
              { key: 'kitchen', icon: ChefHat, label: tString('tabs.kitchen') },
              { key: 'staff', icon: UserCheck, label: tString('tabs.staff') },
              { key: 'settings', icon: Settings, label: tString('tabs.settings') },
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
                    <h2 className="text-lg font-medium text-gray-900 tracking-wide">{tString('navigation')}</h2>
                    <button
                      onClick={() => setSidebarOpen(false)}
                      className="p-2 text-gray-600 hover:text-gray-900 hover:bg-gray-100 rounded-lg transition-colors"
                    >
                      <X className="w-5 h-5" />
                    </button>
                  </div>
                  <nav className="flex-1 p-6 space-y-2">
                    {[
                      { key: 'overview', icon: BarChart3, label: tString('tabs.overview') },
                      { key: 'analytics', icon: BarChart3, label: tString('tabs.analytics') },
                      { key: 'blockchain', icon: Wallet, label: tString('blockchain.title') },
                      { key: 'subscription', icon: CreditCard, label: tString('tabs.subscription') },
                      { key: 'menu', icon: Menu, label: tString('tabs.menu') },
                      { key: 'tables', icon: Users, label: tString('tabs.tables') },
                      { key: 'counters', icon: Coffee, label: tString('tabs.counter') },
                      { key: 'bills', icon: Receipt, label: tString('tabs.bills') },
                      { key: 'staff', icon: UserCheck, label: tString('tabs.staff') },
                      { key: 'settings', icon: Settings, label: tString('tabs.settings') },
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
                  {tString('title')}
                </h1>
                <p className="text-lg text-gray-600 font-light leading-relaxed max-w-2xl">
                  {tString('blockchain.subtitle')}
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
