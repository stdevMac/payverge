'use client';

import { useState, useEffect, useCallback } from 'react';
import { useRouter } from 'next/navigation';
import { useAccount, useConnect } from 'wagmi';
import { Button } from '@/components/ui/Button';
import { Badge } from '@/components/ui/Badge';
import { 
  getReferrer, 
  getReferrerReferrals, 
  claimCommission,
  updateReferralCode,
  formatUsdcAmount 
} from '@/api/referrals';
import { useClaimCommissions } from '@/contracts/hooks';

interface DashboardData {
  referrer: any;
  referrals: any[];
}

export default function ReferralDashboardPage() {
  const router = useRouter();
  const { address, isConnected } = useAccount();
  const { connect, connectors } = useConnect();
  const { claimCommissions } = useClaimCommissions();

  const [dashboardData, setDashboardData] = useState<DashboardData | null>(null);
  const [isLoading, setIsLoading] = useState(false);
  const [isClaiming, setIsClaiming] = useState(false);
  const [isUpdatingCode, setIsUpdatingCode] = useState(false);
  const [newReferralCode, setNewReferralCode] = useState('');
  const [showUpdateCode, setShowUpdateCode] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const loadDashboardData = useCallback(async () => {
    if (!address) return;

    try {
      setIsLoading(true);
      setError(null);

      const data = await getReferrerReferrals(address);
      setDashboardData(data);
      setNewReferralCode(data.referrer.referral_code);
    } catch (err: any) {
      if (err.message?.includes('not found')) {
        setError('You are not registered as a referrer yet.');
      } else {
        setError(err.message || 'Failed to load dashboard data');
      }
    } finally {
      setIsLoading(false);
    }
  }, [address]);

  // Load dashboard data when wallet is connected
  useEffect(() => {
    if (isConnected && address) {
      loadDashboardData();
    }
  }, [address, isConnected, loadDashboardData]);

  const handleClaimCommissions = async () => {
    if (!address || !dashboardData?.referrer) return;

    try {
      setIsClaiming(true);
      setError(null);

      // Call smart contract to claim commissions
      const txHash = await claimCommissions();

      // Update backend
      await claimCommission({
        wallet_address: address,
        amount: dashboardData.referrer.claimable_commissions,
        tx_hash: (txHash as unknown) as string,
      });

      // Reload dashboard data
      await loadDashboardData();
    } catch (err: any) {
      setError(err.message || 'Failed to claim commissions');
    } finally {
      setIsClaiming(false);
    }
  };

  const handleUpdateReferralCode = async () => {
    if (!address || !newReferralCode) return;

    try {
      setIsUpdatingCode(true);
      setError(null);

      await updateReferralCode(address, {
        new_referral_code: newReferralCode,
      });

      setShowUpdateCode(false);
      await loadDashboardData();
    } catch (err: any) {
      setError(err.message || 'Failed to update referral code');
    } finally {
      setIsUpdatingCode(false);
    }
  };

  const handleLogout = () => {
    // Clear dashboard data
    setDashboardData(null);
    // Redirect to referrals page
    router.push('/referrals');
  };

  // Wallet not connected
  if (!isConnected) {
    return (
      <div className="min-h-screen bg-white flex items-center justify-center">
        <div className="max-w-md mx-auto bg-white rounded-2xl p-8 border border-gray-200 text-center">
          <div className="w-16 h-16 bg-gray-100 rounded-full flex items-center justify-center mx-auto mb-6">
            <span className="text-2xl">🔗</span>
          </div>
          <h2 className="text-2xl font-bold text-gray-900 mb-4">Connect Your Wallet</h2>
          <p className="text-gray-600 mb-6">
            Connect your wallet to access your referral dashboard
          </p>
          <Button
            onClick={() => connect({ connector: connectors[0] })}
            className="w-full py-3 bg-gray-900 text-white font-semibold rounded-xl hover:bg-gray-800"
          >
            Connect Wallet
          </Button>
        </div>
      </div>
    );
  }


  // Loading dashboard data
  if (isLoading) {
    return (
      <div className="min-h-screen bg-white flex items-center justify-center">
        <div className="text-center">
          <div className="w-12 h-12 border-4 border-gray-200 border-t-gray-600 rounded-full animate-spin mx-auto mb-4"></div>
          <p className="text-gray-600">Loading your dashboard...</p>
        </div>
      </div>
    );
  }

  // Error state
  if (error && !dashboardData) {
    return (
      <div className="min-h-screen bg-white flex items-center justify-center">
        <div className="max-w-md mx-auto bg-white rounded-2xl p-8 border border-gray-200 text-center">
          <div className="w-16 h-16 bg-gray-100 rounded-full flex items-center justify-center mx-auto mb-6">
            <span className="text-2xl">❌</span>
          </div>
          <h2 className="text-2xl font-bold text-gray-900 mb-4">Not a Referrer</h2>
          <p className="text-gray-600 mb-6">{error}</p>
          <div className="flex flex-col gap-3">
            <Button
              onClick={() => router.push('/referrals/buy')}
              className="w-full py-3 bg-gray-900 text-white font-semibold rounded-xl hover:bg-gray-800"
            >
              Become a Referrer
            </Button>
            <Button
              onClick={() => router.push('/referrals')}
              variant="secondary"
              className="w-full py-3"
            >
              Learn More
            </Button>
          </div>
        </div>
      </div>
    );
  }

  const referrer = dashboardData?.referrer;
  const referrals = dashboardData?.referrals || [];

  return (
    <div className="min-h-screen bg-white">
      {/* Header */}
      <div className="bg-white border-b border-gray-200">
        <div className="container mx-auto px-6 py-6">
          <div className="flex items-center justify-between">
            <div>
              <h1 className="text-2xl font-bold text-gray-900">Referral Dashboard</h1>
              <p className="text-gray-600">Manage your referrals and track earnings</p>
            </div>
            <div className="flex items-center gap-4">
              <div className="text-sm text-gray-600">
                {address?.slice(0, 6)}...{address?.slice(-4)}
              </div>
              <Button
                onClick={handleLogout}
                variant="secondary"
                className="px-4 py-2"
              >
                Logout
              </Button>
            </div>
          </div>
        </div>
      </div>

      <div className="container mx-auto px-6 py-8">
        {/* Stats Overview */}
        <div className="grid grid-cols-1 md:grid-cols-4 gap-6 mb-8">
          <div className="bg-white rounded-3xl p-6 border border-gray-200 hover:shadow-lg transition-all duration-300">
            <div className="flex items-center justify-between mb-2">
              <h3 className="text-sm font-medium text-gray-600">Tier</h3>
              <Badge className="bg-gray-900 text-white px-3 py-1 text-xs font-medium rounded-full">
                {referrer?.tier?.toUpperCase()}
              </Badge>
            </div>
            <div className="text-2xl font-semibold text-gray-900">
              {referrer?.tier === 'premium' ? '15%' : '10%'} Commission
            </div>
          </div>

          <div className="bg-white rounded-3xl p-6 border border-gray-200 hover:shadow-lg transition-all duration-300">
            <h3 className="text-sm font-medium text-gray-600 mb-2">Total Referrals</h3>
            <div className="text-2xl font-semibold text-gray-900">{referrer?.total_referrals || 0}</div>
          </div>

          <div className="bg-white rounded-3xl p-6 border border-gray-200 hover:shadow-lg transition-all duration-300">
            <h3 className="text-sm font-medium text-gray-600 mb-2">Total Earned</h3>
            <div className="text-2xl font-semibold text-gray-900">
              ${(parseFloat(referrer?.total_commissions || '0') / 1000000).toFixed(2)}
            </div>
          </div>

          <div className="bg-white rounded-3xl p-6 border border-gray-200 hover:shadow-lg transition-all duration-300">
            <div className="flex items-center justify-between mb-2">
              <h3 className="text-sm font-medium text-gray-600">Claimable</h3>
              {parseFloat(referrer?.claimable_commissions || '0') > 0 && (
                <Button
                  onClick={handleClaimCommissions}
                  disabled={isClaiming}
                  className="px-3 py-1 text-xs bg-green-600 text-white rounded-lg hover:bg-green-700"
                >
                  {isClaiming ? 'Claiming...' : 'Claim'}
                </Button>
              )}
            </div>
            <div className="text-2xl font-semibold text-green-600">
              ${(parseFloat(referrer?.claimable_commissions || '0') / 1000000).toFixed(2)}
            </div>
          </div>
        </div>

        {/* Referral Code Section */}
        <div className="bg-white rounded-3xl p-6 border border-gray-200 mb-8 hover:shadow-lg transition-all duration-300">
          <div className="flex items-center justify-between mb-4">
            <h3 className="text-lg font-semibold text-gray-900">Your Referral Code</h3>
            <Button
              onClick={() => setShowUpdateCode(!showUpdateCode)}
              variant="secondary"
              className="px-4 py-2 text-sm"
            >
              Update Code
            </Button>
          </div>
          
          <div className="bg-gray-50 rounded-xl p-4 mb-4">
            <div className="text-center">
              <div className="text-3xl font-bold font-mono text-gray-900 mb-2">
                {referrer?.referral_code}
              </div>
              <p className="text-sm text-gray-600">
                Share this code with restaurants to earn commissions
              </p>
            </div>
          </div>

          {showUpdateCode && (
            <div className="border-t border-gray-200 pt-4">
              <div className="flex gap-4">
                <input
                  type="text"
                  value={newReferralCode}
                  onChange={(e) => setNewReferralCode(e.target.value.toUpperCase())}
                  className="flex-1 px-4 py-2 border border-gray-300 rounded-lg focus:ring-2 focus:ring-blue-500 focus:border-transparent"
                  placeholder="Enter new referral code"
                  maxLength={12}
                />
                <Button
                  onClick={handleUpdateReferralCode}
                  disabled={isUpdatingCode || newReferralCode === referrer?.referral_code}
                  className="px-6 py-2 bg-blue-600 text-white rounded-lg hover:bg-blue-700 disabled:bg-gray-400"
                >
                  {isUpdatingCode ? 'Updating...' : 'Update'}
                </Button>
              </div>
            </div>
          )}
        </div>

        {/* Referrals List */}
        <div className="bg-white rounded-3xl border border-gray-200 hover:shadow-lg transition-all duration-300">
          <div className="p-6 border-b border-gray-200">
            <h3 className="text-lg font-semibold text-gray-900">Your Referrals</h3>
            <p className="text-gray-600">Businesses you&apos;ve successfully referred</p>
          </div>

          {referrals.length === 0 ? (
            <div className="p-12 text-center">
              <div className="w-16 h-16 bg-gray-100 rounded-full flex items-center justify-center mx-auto mb-4">
                <span className="text-2xl">📋</span>
              </div>
              <h4 className="text-lg font-semibold text-gray-900 mb-2">No Referrals Yet</h4>
              <p className="text-gray-600 mb-6">
                Start sharing your referral code to see your referrals here
              </p>
              <Button
                onClick={() => router.push('/referrals')}
                className="px-6 py-3 bg-blue-600 text-white rounded-xl hover:bg-blue-700"
              >
                Learn How to Refer
              </Button>
            </div>
          ) : (
            <div className="divide-y divide-gray-200">
              {referrals.map((referral, index) => (
                <div key={referral.id} className="p-6">
                  <div className="flex items-center justify-between">
                    <div>
                      <h4 className="font-semibold text-gray-900">
                        {referral.business?.name || `Business #${referral.business_id}`}
                      </h4>
                      <p className="text-sm text-gray-600">
                        Registered: {new Date(referral.created_at).toLocaleDateString()}
                      </p>
                    </div>
                    <div className="text-right">
                      <div className="text-lg font-semibold text-green-600">
                        +${formatUsdcAmount(referral.commission)}
                      </div>
                      <div className="text-sm text-gray-600">
                        {referral.commission_paid ? (
                          <Badge className="bg-green-100 text-green-800">Paid</Badge>
                        ) : (
                          <Badge className="bg-yellow-100 text-yellow-800">Pending</Badge>
                        )}
                      </div>
                    </div>
                  </div>
                </div>
              ))}
            </div>
          )}
        </div>

        {/* Error Display */}
        {error && (
          <div className="mt-6 bg-red-50 border border-red-200 rounded-xl p-4">
            <div className="flex items-center">
              <div className="w-4 h-4 bg-red-500 rounded-full mr-3"></div>
              <span className="text-red-800">{error}</span>
            </div>
          </div>
        )}
      </div>
    </div>
  );
}
