'use client';

import { useState, useEffect } from 'react';
import { useRouter, useSearchParams } from 'next/navigation';
import { useAccount, useConnect } from 'wagmi';
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
  formatUsdcAmount,
  parseUsdcAmount
} from '@/contracts/hooks';
import { CONTRACT_CONSTANTS } from '@/contracts/config';

export default function BuyReferralPage() {
  const router = useRouter();
  const searchParams = useSearchParams();
  const { address, isConnected } = useAccount();
  const { connect, connectors } = useConnect();
  
  const [selectedTier, setSelectedTier] = useState<'basic' | 'premium'>('basic');
  const [referralCode, setReferralCode] = useState('');
  const [isCodeAvailable, setIsCodeAvailable] = useState<boolean | null>(null);
  const [isCheckingCode, setIsCheckingCode] = useState(false);
  const [isRegistering, setIsRegistering] = useState(false);
  const [registrationStep, setRegistrationStep] = useState<'select' | 'approve' | 'register' | 'success'>('select');
  const [error, setError] = useState<string | null>(null);

  const tierPricing = getReferralTierPricing();
  
  // Contract hooks
  const { registerBasicReferrer } = useRegisterBasicReferrer();
  const { registerPremiumReferrer } = useRegisterPremiumReferrer();
  const { approveUsdcForReferrals } = useApproveUsdcForReferrals();
  const { data: usdcAllowance } = useUsdcAllowanceForReferrals();
  const { data: usdcBalance } = useUsdcBalance();

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

  // Check referral code availability with debounce
  useEffect(() => {
    if (!referralCode) return;

    const validation = validateReferralCode(referralCode);
    if (!validation.valid) {
      setIsCodeAvailable(false);
      setError(validation.error || null);
      return;
    }

    setError(null);
    setIsCheckingCode(true);

    const timeoutId = setTimeout(async () => {
      try {
        const result = await checkReferralCodeAvailability(referralCode);
        setIsCodeAvailable(result.available);
        if (!result.available && result.error) {
          setError(result.error);
        }
      } catch (err) {
        setError('Failed to check code availability');
        setIsCodeAvailable(false);
      } finally {
        setIsCheckingCode(false);
      }
    }, 500);

    return () => clearTimeout(timeoutId);
  }, [referralCode]);

  const handleGenerateNewCode = () => {
    setReferralCode(generateReferralCode(8));
    setIsCodeAvailable(null);
    setError(null);
  };

  const getRequiredAmount = () => {
    return selectedTier === 'premium' 
      ? CONTRACT_CONSTANTS.PREMIUM_REFERRER_FEE 
      : CONTRACT_CONSTANTS.BASIC_REFERRER_FEE;
  };

  const hasEnoughBalance = () => {
    if (!usdcBalance) return false;
    return usdcBalance >= getRequiredAmount();
  };

  const hasEnoughAllowance = () => {
    if (!usdcAllowance) return false;
    return usdcAllowance >= getRequiredAmount();
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
      const contractFn = selectedTier === 'premium' ? registerPremiumReferrer : registerBasicReferrer;
      const txHash = await contractFn(referralCode);

      // Register in backend
      await registerReferrer({
        wallet_address: address,
        referral_code: referralCode,
        tier: selectedTier,
        registration_tx_hash: (txHash as unknown) as string,
      });

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
                className={`bg-white rounded-3xl p-8 border cursor-pointer transition-all duration-300 group ${
                  selectedTier === 'basic' 
                    ? 'border-gray-900 shadow-lg' 
                    : 'border-gray-200 hover:shadow-lg'
                }`}
                onClick={() => setSelectedTier('basic')}
              >
                <div className="text-center mb-8">
                  <h3 className="text-2xl font-semibold text-gray-900 mb-3">Basic Referrer</h3>
                  <div className="text-5xl font-medium text-gray-900 mb-2">$10</div>
                  <p className="text-gray-500 font-light">One-time registration fee</p>
                </div>
                
                <div className="space-y-4 mb-8">
                  <div className="flex items-center">
                    <div className="w-2 h-2 bg-green-500 rounded-full mr-3"></div>
                    <span className="text-gray-700">10% commission on referrals</span>
                  </div>
                  <div className="flex items-center">
                    <div className="w-2 h-2 bg-green-500 rounded-full mr-3"></div>
                    <span className="text-gray-700">10% discount for businesses</span>
                  </div>
                  <div className="flex items-center">
                    <div className="w-2 h-2 bg-green-500 rounded-full mr-3"></div>
                    <span className="text-gray-700">Custom referral code</span>
                  </div>
                  <div className="flex items-center">
                    <div className="w-2 h-2 bg-green-500 rounded-full mr-3"></div>
                    <span className="text-gray-700">Real-time dashboard</span>
                  </div>
                </div>
              </div>

              {/* Premium Tier */}
              <div 
                className={`bg-white rounded-3xl p-8 border-2 cursor-pointer transition-all duration-300 group relative ${
                  selectedTier === 'premium' 
                    ? 'border-gray-900 shadow-lg' 
                    : 'border-gray-900 hover:shadow-lg'
                }`}
                onClick={() => setSelectedTier('premium')}
              >
                <div className="absolute -top-3 left-1/2 transform -translate-x-1/2">
                  <Badge className="bg-gray-900 text-white px-4 py-1 text-sm font-medium rounded-full">
                    RECOMMENDED
                  </Badge>
                </div>
                
                <div className="text-center mb-8">
                  <h3 className="text-2xl font-semibold text-gray-900 mb-3">Premium Referrer</h3>
                  <div className="text-5xl font-medium text-gray-900 mb-2">$25</div>
                  <p className="text-gray-500 font-light">One-time registration fee</p>
                </div>
                
                <div className="space-y-4 mb-8">
                  <div className="flex items-center">
                    <div className="w-2 h-2 bg-green-500 rounded-full mr-3"></div>
                    <span className="text-gray-700">15% commission on referrals</span>
                  </div>
                  <div className="flex items-center">
                    <div className="w-2 h-2 bg-green-500 rounded-full mr-3"></div>
                    <span className="text-gray-700">15% discount for businesses</span>
                  </div>
                  <div className="flex items-center">
                    <div className="w-2 h-2 bg-green-500 rounded-full mr-3"></div>
                    <span className="text-gray-700">Custom referral code</span>
                  </div>
                  <div className="flex items-center">
                    <div className="w-2 h-2 bg-green-500 rounded-full mr-3"></div>
                    <span className="text-gray-700">Priority support</span>
                  </div>
                  <div className="flex items-center">
                    <div className="w-2 h-2 bg-green-500 rounded-full mr-3"></div>
                    <span className="text-gray-700">Advanced analytics</span>
                  </div>
                </div>
              </div>
            </div>

            {/* Referral Code Input */}
            <div className="bg-white rounded-3xl p-8 border border-gray-200 mb-8 hover:shadow-lg transition-all duration-300">
              <h3 className="text-xl font-semibold text-gray-900 mb-4">Choose Your Referral Code</h3>
              <div className="flex gap-4 mb-4">
                <div className="flex-1">
                  <input
                    type="text"
                    value={referralCode}
                    onChange={(e) => setReferralCode(e.target.value.toUpperCase())}
                    className="w-full px-4 py-3 border border-gray-300 rounded-xl focus:ring-2 focus:ring-blue-500 focus:border-transparent"
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
              <div className="flex items-center gap-2">
                {isCheckingCode ? (
                  <div className="flex items-center text-gray-600">
                    <div className="w-4 h-4 border-2 border-gray-300 border-t-blue-600 rounded-full animate-spin mr-2"></div>
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
            </div>

            {/* Wallet Connection & Balance */}
            {!isConnected ? (
              <div className="bg-yellow-50 border border-yellow-200 rounded-2xl p-6 mb-8">
                <h3 className="text-lg font-semibold text-yellow-800 mb-2">Connect Your Wallet</h3>
                <p className="text-yellow-700 mb-4">Connect your wallet to proceed with registration</p>
                <Button
                  onClick={() => connect({ connector: connectors[0] })}
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
                disabled={!isConnected || !isCodeAvailable || !hasEnoughBalance()}
                className={`px-12 py-4 text-lg font-semibold rounded-xl ${
                  selectedTier === 'premium'
                    ? 'bg-purple-600 hover:bg-purple-700'
                    : 'bg-blue-600 hover:bg-blue-700'
                } text-white disabled:bg-gray-400 disabled:cursor-not-allowed`}
              >
                Continue to Payment
              </Button>
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
          <div className="max-w-2xl mx-auto text-center">
            <div className="bg-white rounded-2xl p-8 border border-gray-200">
              <div className="w-16 h-16 bg-green-100 rounded-full flex items-center justify-center mx-auto mb-6">
                <span className="text-3xl">🎉</span>
              </div>
              
              <h2 className="text-3xl font-bold text-gray-900 mb-4">Registration Complete!</h2>
              <p className="text-lg text-gray-600 mb-8">
                Congratulations! You&apos;re now a {selectedTier} referrer with Payverge.
              </p>
              
              <div className="bg-green-50 rounded-xl p-6 mb-8">
                <div className="text-sm text-green-600 mb-2">Your Referral Code:</div>
                <div className="text-2xl font-bold font-mono text-green-800">{referralCode}</div>
                <div className="text-sm text-green-600 mt-2">
                  Share this code with restaurants to start earning!
                </div>
              </div>

              <div className="flex flex-col sm:flex-row gap-4 justify-center">
                <Button
                  onClick={() => router.push('/referrals/dashboard')}
                  className="px-8 py-3 bg-blue-600 text-white font-semibold rounded-xl hover:bg-blue-700"
                >
                  Go to Dashboard
                </Button>
                <Button
                  onClick={() => router.push('/referrals')}
                  variant="secondary"
                  className="px-8 py-3"
                >
                  Back to Referrals
                </Button>
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
