'use client';

import { useState, useEffect } from 'react';
import { useRouter, useSearchParams } from 'next/navigation';
import { useAccount } from 'wagmi';
import { useAppKit } from '@reown/appkit/react';
import { Button } from '@/components/ui/Button';
import { Badge } from '@/components/ui/Badge';
import { 
  checkReferralCodeAvailability, 
  registerReferrer, 
  generateReferralCode,
  validateReferralCode,
  getReferralTierPricing 
} from '@/api/referrals';
import { 
  useRegisterBasicReferrer, 
  useRegisterPremiumReferrer,
  useApproveUsdcForReferrals,
  useUsdcAllowanceForReferrals,
  useUsdcBalance,
  useReferralCodeAvailability,
  useBasicReferrerFee,
  usePremiumReferrerFee,
  formatUsdcAmount,
  parseUsdcAmount
} from '@/contracts/hooks';
import { CONTRACT_CONSTANTS } from '@/contracts/config';

export default function BuyReferralPage() {
  const router = useRouter();
  const searchParams = useSearchParams();
  const { address, isConnected } = useAccount();
  const { open } = useAppKit();
  
  const [selectedTier, setSelectedTier] = useState<'basic' | 'premium'>('basic');
  const [referralCode, setReferralCode] = useState('');
  const [isCodeAvailable, setIsCodeAvailable] = useState<boolean | null>(null);
  const [isCheckingCode, setIsCheckingCode] = useState(false);
  const [isRegistering, setIsRegistering] = useState(false);
  const [registrationStep, setRegistrationStep] = useState<'select' | 'approve' | 'register' | 'success'>('select');
  const [error, setError] = useState<string | null>(null);

  // Contract hooks
  const { registerBasicReferrer } = useRegisterBasicReferrer();
  const { registerPremiumReferrer } = useRegisterPremiumReferrer();
  const { approveUsdcForReferrals } = useApproveUsdcForReferrals();
  const { data: usdcAllowance } = useUsdcAllowanceForReferrals();
  const { data: usdcBalance } = useUsdcBalance();
  const { data: basicFee } = useBasicReferrerFee();
  const { data: premiumFee } = usePremiumReferrerFee();
  const { data: codeAvailability } = useReferralCodeAvailability(referralCode);

  // Get tier from URL params
  useEffect(() => {
    const tier = searchParams.get('tier');
    if (tier === 'basic' || tier === 'premium') {
      setSelectedTier(tier);
    }
  }, [searchParams]);

  // Generate initial referral code
  useEffect(() => {
    if (!referralCode) {
      setReferralCode(generateReferralCode(8));
    }
  }, [referralCode]);

  // Check referral code availability using blockchain
  useEffect(() => {
    if (!referralCode) return;

    const validation = validateReferralCode(referralCode);
    if (!validation.valid) {
      setIsCodeAvailable(false);
      setError(validation.error || null);
      return;
    }

    setError(null);
    
    // Use blockchain data for availability check
    if (codeAvailability !== undefined) {
      setIsCodeAvailable(Boolean(codeAvailability));
      setIsCheckingCode(false);
    } else {
      setIsCheckingCode(true);
    }
  }, [referralCode, codeAvailability]);

  const handleGenerateNewCode = () => {
    setReferralCode(generateReferralCode(8));
    setIsCodeAvailable(null);
    setError(null);
  };

  const getRequiredAmount = (): bigint => {
    if (selectedTier === 'premium') {
      return typeof premiumFee === 'bigint' ? premiumFee : CONTRACT_CONSTANTS.PREMIUM_REFERRER_FEE; // $25 USDC
    } else {
      return typeof basicFee === 'bigint' ? basicFee : CONTRACT_CONSTANTS.BASIC_REFERRER_FEE; // $10 USDC
    }
  };

  const hasEnoughBalance = () => {
    if (!usdcBalance || typeof usdcBalance !== 'bigint') return false;
    const required = getRequiredAmount();
    return usdcBalance >= required;
  };

  const hasEnoughAllowance = () => {
    if (!usdcAllowance || typeof usdcAllowance !== 'bigint') return false;
    const required = getRequiredAmount();
    return usdcAllowance >= required;
  };

  const handleApproveUsdc = async () => {
    if (!address) return;
    
    try {
      setIsRegistering(true);
      setError(null);
      
      const amount = getRequiredAmount();
      await approveUsdcForReferrals(amount);
      
      setRegistrationStep('register');
    } catch (err: any) {
      setError(err.message || 'Failed to approve USDC');
    } finally {
      setIsRegistering(false);
    }
  };

  const handleRegisterReferrer = async () => {
    if (!address || !isCodeAvailable) return;

    try {
      setIsRegistering(true);
      setError(null);

      // Call smart contract function
      if (selectedTier === 'premium') {
        await registerPremiumReferrer(referralCode);
      } else {
        await registerBasicReferrer(referralCode);
      }

      // Optional: Register in backend for additional tracking
      try {
        await registerReferrer({
          wallet_address: address,
          referral_code: referralCode,
          tier: selectedTier,
          registration_tx_hash: 'blockchain', // Will be updated with actual tx hash
        });
      } catch (backendErr) {
        console.warn('Backend registration failed, but blockchain registration succeeded');
      }

      setRegistrationStep('success');
    } catch (err: any) {
      setError(err.message || 'Failed to register as referrer');
    } finally {
      setIsRegistering(false);
    }
  };

  const renderStepContent = () => {
    switch (registrationStep) {
      case 'select':
        return (
          <div className="max-w-4xl mx-auto">
            {/* Tier Selection */}
            <div className="grid md:grid-cols-2 gap-8 max-w-4xl mx-auto">
              {/* Basic Tier */}
              <div 
                className={`rounded-3xl p-8 border-2 cursor-pointer transition-all duration-300 group relative ${
                  selectedTier === 'basic' 
                    ? 'bg-gray-900 border-gray-900 shadow-xl' 
                    : 'bg-white border-gray-200 hover:border-gray-300 hover:shadow-lg'
                }`}
                onClick={() => setSelectedTier('basic')}
              >
                {selectedTier === 'basic' && (
                  <div className="absolute -top-3 -right-3 w-8 h-8 bg-green-500 rounded-full flex items-center justify-center">
                    <span className="text-white text-sm font-bold">✓</span>
                  </div>
                )}
                <div className="text-center mb-8">
                  <h3 className={`text-2xl font-semibold mb-3 ${selectedTier === 'basic' ? 'text-white' : 'text-gray-900'}`}>
                    Basic Referrer
                  </h3>
                  <div className={`text-5xl font-medium mb-2 ${selectedTier === 'basic' ? 'text-white' : 'text-gray-900'}`}>
                    {typeof basicFee === 'bigint' ? formatUsdcAmount(basicFee) : '$10'} USDC
                  </div>
                  <p className={`font-light ${selectedTier === 'basic' ? 'text-gray-300' : 'text-gray-500'}`}>
                    One-time registration fee
                  </p>
                </div>
                
                <div className="space-y-4 mb-8">
                  <div className="flex items-center">
                    <div className="w-2 h-2 bg-green-500 rounded-full mr-3"></div>
                    <span className={selectedTier === 'basic' ? 'text-gray-300' : 'text-gray-700'}>10% commission on referrals</span>
                  </div>
                  <div className="flex items-center">
                    <div className="w-2 h-2 bg-green-500 rounded-full mr-3"></div>
                    <span className={selectedTier === 'basic' ? 'text-gray-300' : 'text-gray-700'}>10% discount for businesses</span>
                  </div>
                  <div className="flex items-center">
                    <div className="w-2 h-2 bg-green-500 rounded-full mr-3"></div>
                    <span className={selectedTier === 'basic' ? 'text-gray-300' : 'text-gray-700'}>Custom referral code</span>
                  </div>
                  <div className="flex items-center">
                    <div className="w-2 h-2 bg-green-500 rounded-full mr-3"></div>
                    <span className={selectedTier === 'basic' ? 'text-gray-300' : 'text-gray-700'}>Real-time dashboard</span>
                  </div>
                </div>
              </div>

              {/* Premium Tier */}
              <div 
                className={`rounded-3xl p-8 border-2 cursor-pointer transition-all duration-300 group relative ${
                  selectedTier === 'premium' 
                    ? 'bg-gray-900 border-gray-900 shadow-xl' 
                    : 'bg-white border-gray-900 hover:shadow-lg'
                }`}
                onClick={() => setSelectedTier('premium')}
              >
                <div className="absolute -top-3 left-1/2 transform -translate-x-1/2">
                  <Badge className={`px-4 py-1 text-sm font-medium rounded-full ${
                    selectedTier === 'premium' 
                      ? 'bg-white text-gray-900' 
                      : 'bg-gray-900 text-white'
                  }`}>
                    RECOMMENDED
                  </Badge>
                </div>
                
                {selectedTier === 'premium' && (
                  <div className="absolute -top-3 -right-3 w-8 h-8 bg-green-500 rounded-full flex items-center justify-center">
                    <span className="text-white text-sm font-bold">✓</span>
                  </div>
                )}
                
                <div className="text-center mb-8">
                  <h3 className={`text-2xl font-semibold mb-3 ${selectedTier === 'premium' ? 'text-white' : 'text-gray-900'}`}>
                    Premium Referrer
                  </h3>
                  <div className={`text-5xl font-medium mb-2 ${selectedTier === 'premium' ? 'text-white' : 'text-gray-900'}`}>
                    {typeof premiumFee === 'bigint' ? formatUsdcAmount(premiumFee) : '$25'} USDC
                  </div>
                  <p className={`font-light ${selectedTier === 'premium' ? 'text-gray-300' : 'text-gray-500'}`}>
                    One-time registration fee
                  </p>
                </div>
                
                <div className="space-y-4 mb-8">
                  <div className="flex items-center">
                    <div className="w-2 h-2 bg-green-500 rounded-full mr-3"></div>
                    <span className={selectedTier === 'premium' ? 'text-gray-300' : 'text-gray-700'}>15% commission on referrals</span>
                  </div>
                  <div className="flex items-center">
                    <div className="w-2 h-2 bg-green-500 rounded-full mr-3"></div>
                    <span className={selectedTier === 'premium' ? 'text-gray-300' : 'text-gray-700'}>15% discount for businesses</span>
                  </div>
                  <div className="flex items-center">
                    <div className="w-2 h-2 bg-green-500 rounded-full mr-3"></div>
                    <span className={selectedTier === 'premium' ? 'text-gray-300' : 'text-gray-700'}>Custom referral code</span>
                  </div>
                  <div className="flex items-center">
                    <div className="w-2 h-2 bg-green-500 rounded-full mr-3"></div>
                    <span className={selectedTier === 'premium' ? 'text-gray-300' : 'text-gray-700'}>Priority support</span>
                  </div>
                  <div className="flex items-center">
                    <div className="w-2 h-2 bg-green-500 rounded-full mr-3"></div>
                    <span className={selectedTier === 'premium' ? 'text-gray-300' : 'text-gray-700'}>Advanced analytics</span>
                  </div>
                </div>
              </div>
            </div>


            {/* Wallet Connection & Balance */}
            {!isConnected ? (
              <div className="bg-yellow-50 border border-yellow-200 rounded-2xl p-6 mb-8">
                <h3 className="text-lg font-semibold text-yellow-800 mb-2">Connect Your Wallet</h3>
                <p className="text-yellow-700 mb-4">Connect your wallet to proceed with registration</p>
                <Button
                  onClick={() => open()}
                  className="bg-yellow-600 text-white hover:bg-yellow-700"
                >
                  Connect Wallet
                </Button>
              </div>
            ) : (
              <div className="bg-blue-50 border border-blue-200 rounded-2xl p-6 mb-8">
                <h3 className="text-lg font-semibold text-blue-800 mb-2">Wallet Connected</h3>
                <p className="text-blue-700 mb-2">Address: {address}</p>
                <p className="text-blue-700">
                  USDC Balance: ${usdcBalance ? (Number(usdcBalance) / 1000000).toFixed(2) : '0.00'}
                </p>
                {!hasEnoughBalance() && (
                  <p className="text-red-600 mt-2">
                    Insufficient USDC balance. You need ${(Number(getRequiredAmount()) / 1000000).toFixed(2)} USDC.
                  </p>
                )}
              </div>
            )}

            {/* Continue Button */}
            <div className="text-center">
              <Button
                onClick={() => setRegistrationStep(hasEnoughAllowance() ? 'register' : 'approve')}
                disabled={!isConnected || !hasEnoughBalance()}
                className="px-12 py-4 text-lg font-medium rounded-xl bg-gray-900 hover:bg-gray-800 text-white disabled:bg-gray-400 disabled:cursor-not-allowed transition-all duration-300 transform hover:scale-105"
              >
                Continue to Payment
              </Button>
              <p className="text-sm text-gray-500 mt-3">
                {selectedTier === 'premium' ? 'Premium' : 'Basic'} tier selected • ${selectedTier === 'premium' ? '25' : '10'} USDC
              </p>
            </div>
          </div>
        );

      case 'approve':
        return (
          <div className="max-w-2xl mx-auto text-center">
            <div className="bg-white rounded-2xl p-8 border border-gray-200">
              <h2 className="text-3xl font-bold text-gray-900 mb-6">Approve USDC Spending</h2>
              <p className="text-lg text-gray-600 mb-8">
                First, you need to approve the contract to spend your USDC tokens for the registration fee.
              </p>
              
              <div className="bg-gray-50 rounded-xl p-6 mb-8">
                <div className="text-sm text-gray-600 mb-2">Amount to approve:</div>
                <div className="text-2xl font-bold text-gray-900">
                  ${(Number(getRequiredAmount()) / 1000000).toFixed(2)} USDC
                </div>
              </div>

              <Button
                onClick={handleApproveUsdc}
                disabled={isRegistering}
                className={`px-12 py-4 text-lg font-semibold rounded-xl ${
                  selectedTier === 'premium'
                    ? 'bg-purple-600 hover:bg-purple-700'
                    : 'bg-blue-600 hover:bg-blue-700'
                } text-white disabled:bg-gray-400`}
              >
                {isRegistering ? 'Approving...' : 'Approve USDC'}
              </Button>
            </div>
          </div>
        );

      case 'register':
        return (
          <div className="max-w-2xl mx-auto text-center">
            <div className="bg-white rounded-2xl p-8 border border-gray-200">
              <h2 className="text-3xl font-bold text-gray-900 mb-6">Complete Registration</h2>
              <p className="text-lg text-gray-600 mb-8">
                Now complete your referrer registration by paying the registration fee.
              </p>
              
              <div className="bg-gray-50 rounded-xl p-6 mb-8">
                <div className="text-sm text-gray-600 mb-2">Registration Details:</div>
                <div className="text-xl font-semibold text-gray-900 mb-2">
                  {selectedTier === 'premium' ? 'Premium' : 'Basic'} Referrer
                </div>
                <div className="text-lg text-gray-700 mb-2">
                  Code: <span className="font-mono font-bold">{referralCode}</span>
                </div>
                <div className="text-lg text-gray-700">
                  Fee: ${(Number(getRequiredAmount()) / 1000000).toFixed(2)} USDC
                </div>
              </div>

              <Button
                onClick={handleRegisterReferrer}
                disabled={isRegistering}
                className={`px-12 py-4 text-lg font-semibold rounded-xl ${
                  selectedTier === 'premium'
                    ? 'bg-purple-600 hover:bg-purple-700'
                    : 'bg-blue-600 hover:bg-blue-700'
                } text-white disabled:bg-gray-400`}
              >
                {isRegistering ? 'Registering...' : 'Complete Registration'}
              </Button>
            </div>
          </div>
        );

      case 'success':
        return (
          <div className="max-w-3xl mx-auto">
            <div className="bg-white rounded-3xl p-8 border border-gray-200 text-center mb-8">
              <div className="w-20 h-20 bg-green-50 rounded-full flex items-center justify-center mx-auto mb-6">
                <span className="text-4xl">🎉</span>
              </div>
              
              <h2 className="text-4xl font-medium text-gray-900 mb-4 tracking-tight">Registration Complete!</h2>
              <p className="text-lg font-light text-gray-600 mb-8">
                Congratulations! You&apos;re now a {selectedTier} referrer with Payverge.
              </p>
              
              <div className="flex flex-col sm:flex-row gap-4 justify-center">
                <Button
                  onClick={() => router.push('/referrals/dashboard')}
                  className="px-8 py-3 bg-gray-900 text-white font-medium rounded-xl hover:bg-gray-800"
                >
                  Go to Dashboard
                </Button>
                <Button
                  onClick={() => router.push('/referrals')}
                  variant="secondary"
                  className="px-8 py-3 border-2 border-gray-300 text-gray-700 font-medium rounded-xl hover:border-gray-500 hover:text-gray-900"
                >
                  Back to Referrals
                </Button>
              </div>
            </div>

            {/* Referral Code Setup */}
            <div className="bg-white rounded-3xl p-8 border border-gray-200">
              <h3 className="text-2xl font-semibold text-gray-900 mb-6 text-center">Set Up Your Referral Code</h3>
              
              <div className="max-w-md mx-auto">
                <div className="flex gap-4 mb-4">
                  <div className="flex-1">
                    <input
                      type="text"
                      value={referralCode}
                      onChange={(e) => setReferralCode(e.target.value.toUpperCase())}
                      className="w-full px-4 py-3 border border-gray-300 rounded-xl focus:ring-2 focus:ring-gray-300 focus:border-transparent text-center text-lg font-mono"
                      placeholder="Enter your referral code"
                      maxLength={12}
                    />
                  </div>
                  <Button
                    onClick={handleGenerateNewCode}
                    variant="secondary"
                    className="px-6 py-3 whitespace-nowrap"
                  >
                    Generate New
                  </Button>
                </div>
                
                {/* Code Status */}
                <div className="flex items-center justify-center gap-2 mb-6">
                  {isCheckingCode ? (
                    <div className="flex items-center text-gray-600">
                      <div className="w-4 h-4 border-2 border-gray-300 border-t-gray-600 rounded-full animate-spin mr-2"></div>
                      Checking availability...
                    </div>
                  ) : isCodeAvailable === true ? (
                    <div className="flex items-center text-green-600">
                      <div className="w-4 h-4 bg-green-500 rounded-full mr-2"></div>
                      Code is available!
                    </div>
                  ) : isCodeAvailable === false ? (
                    <div className="flex items-center text-red-600">
                      <div className="w-4 h-4 bg-red-500 rounded-full mr-2"></div>
                      {error || 'Code is not available'}
                    </div>
                  ) : null}
                </div>

                <div className="bg-gray-50 rounded-xl p-6 text-center">
                  <div className="text-sm text-gray-600 mb-2">Your Referral Code:</div>
                  <div className="text-3xl font-bold font-mono text-gray-900 mb-2">{referralCode}</div>
                  <div className="text-sm text-gray-600">
                    Share this code with restaurants to start earning commissions!
                  </div>
                </div>
              </div>
            </div>
          </div>
        );

      default:
        return null;
    }
  };

  return (
    <div className="min-h-screen bg-white">
      {/* Header */}
      <div className="bg-white border-b border-gray-200">
        <div className="container mx-auto px-6 py-6">
          <div className="flex items-center justify-between">
            <div>
              <h1 className="text-2xl font-bold text-gray-900">Become a Referrer</h1>
              <p className="text-gray-600">Join our referral program and start earning</p>
            </div>
            <Button
              onClick={() => router.push('/referrals')}
              variant="secondary"
              className="px-4 py-2"
            >
              ← Back
            </Button>
          </div>
        </div>
      </div>

      {/* Progress Steps */}
      <div className="bg-white border-b border-gray-200">
        <div className="container mx-auto px-6 py-4">
          <div className="flex items-center justify-center space-x-8">
            <div className={`flex items-center ${registrationStep === 'select' ? 'text-gray-900' : 'text-gray-400'}`}>
              <div className={`w-8 h-8 rounded-full flex items-center justify-center text-sm font-semibold ${
                registrationStep === 'select' ? 'bg-gray-900 text-white' : 'bg-gray-200'
              }`}>
                1
              </div>
              <span className="ml-2 font-medium">Select Tier</span>
            </div>
            
            <div className={`flex items-center ${registrationStep === 'approve' ? 'text-gray-900' : 'text-gray-400'}`}>
              <div className={`w-8 h-8 rounded-full flex items-center justify-center text-sm font-semibold ${
                registrationStep === 'approve' ? 'bg-gray-900 text-white' : 'bg-gray-200'
              }`}>
                2
              </div>
              <span className="ml-2 font-medium">Approve</span>
            </div>
            
            <div className={`flex items-center ${registrationStep === 'register' ? 'text-gray-900' : 'text-gray-400'}`}>
              <div className={`w-8 h-8 rounded-full flex items-center justify-center text-sm font-semibold ${
                registrationStep === 'register' ? 'bg-gray-900 text-white' : 'bg-gray-200'
              }`}>
                3
              </div>
              <span className="ml-2 font-medium">Register</span>
            </div>
            
            <div className={`flex items-center ${registrationStep === 'success' ? 'text-green-600' : 'text-gray-400'}`}>
              <div className={`w-8 h-8 rounded-full flex items-center justify-center text-sm font-semibold ${
                registrationStep === 'success' ? 'bg-green-600 text-white' : 'bg-gray-200'
              }`}>
                ✓
              </div>
              <span className="ml-2 font-medium">Complete</span>
            </div>
          </div>
        </div>
      </div>

      {/* Main Content */}
      <div className="container mx-auto px-6 py-12">
        {error && (
          <div className="max-w-2xl mx-auto mb-8">
            <div className="bg-red-50 border border-red-200 rounded-xl p-4">
              <div className="flex items-center">
                <div className="w-4 h-4 bg-red-500 rounded-full mr-3"></div>
                <span className="text-red-800">{error}</span>
              </div>
            </div>
          </div>
        )}
        
        {renderStepContent()}
      </div>
    </div>
  );
}
