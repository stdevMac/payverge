'use client';

import React, { useState, useEffect, useRef } from 'react';
import { Card, CardBody, CardHeader, Button, Input, Checkbox, Divider, Spinner, Textarea, Switch, Progress } from '@nextui-org/react';
import { ArrowLeft, Building2, Wallet, Settings, Upload, Camera, Check, ChevronRight, ChevronLeft, MapPin, Phone, Globe, Hash, Percent, DollarSign } from 'lucide-react';
import Link from 'next/link';
import Image from 'next/image';
import { useRouter } from 'next/navigation';
import { useAccount } from 'wagmi';
import { useUserStore } from '@/store/useUserStore';
import { useAuth } from '@/hooks/useAuth';
import { getCookie } from '@/config/aws-s3/cookie-management/store.helpers';
import { isTokenValid, decodeJwt } from '@/utils/jwt';
import { getUserProfile } from '@/api/users/profile';
import { createBusiness, CreateBusinessRequest } from '../../../../api/business';
import BusinessTutorialModal from '../../../../components/business/BusinessTutorialModal';
import SmartContractLoadingOverlay from '../../../../components/business/SmartContractLoadingOverlay';
import BusinessRegistrationSuccess from '../../../../components/business/BusinessRegistrationSuccess';
import { UserInterface as User } from '@/interface/users/users-interface';
import { 
  useRegisterBusiness, 
  useApproveUsdc, 
  useUsdcBalance, 
  useUsdcAllowance, 
  useReferrer, 
  useRegistrationFee,
  useCalculateSubscriptionTime,
  formatUsdcAmount,
  parseUsdcAmount,
  useValidateCoupon,
  useRegisterBusinessWithCoupon
} from '@/contracts/hooks';
import { RegisterBusinessParams, Referrer } from '@/contracts/types';
import { Address } from 'viem';
import { checkReferralCodeAvailability, CheckReferralCodeResponse } from '@/api/referrals';
import { uploadFileToS3, uploadProtectedFileToS3 } from '@/api/files/useUploadFileToS3/useUploadFileToS3';
import { useSimpleLocale, getTranslation } from '@/i18n/SimpleTranslationProvider';
import SimpleLanguageSwitcher from '@/components/SimpleLanguageSwitcher';


// Multi-step form steps
type FormStep = 'business' | 'location' | 'settings' | 'subscription' | 'review';

// Subscription Payment Configuration
interface SubscriptionOption {
  months: number;
  suggestedAmount: string;
  description: string;
  popular?: boolean;
}

// Dynamic subscription options based on registration fee
const getSubscriptionOptions = (registrationFee?: unknown, locale?: 'en' | 'es'): SubscriptionOption[] => {
  const yearlyFee = registrationFee && typeof registrationFee === 'bigint' 
    ? Number(formatUsdcAmount(registrationFee)) 
    : 100; // More reasonable default closer to actual registration fee
  
  return [
    {
      months: 1,
      suggestedAmount: (yearlyFee / 12).toFixed(2),
      description: getTranslation('businessRegister.subscription.options.1month', locale || 'en') as string,
    },
    {
      months: 3,
      suggestedAmount: (yearlyFee / 4).toFixed(2),
      description: getTranslation('businessRegister.subscription.options.3months', locale || 'en') as string,
    },
    {
      months: 6,
      suggestedAmount: (yearlyFee / 2).toFixed(2),
      description: getTranslation('businessRegister.subscription.options.6months', locale || 'en') as string,
      popular: true,
    },
    {
      months: 12,
      suggestedAmount: yearlyFee.toFixed(2),
      description: getTranslation('businessRegister.subscription.options.12months', locale || 'en') as string,
    },
  ];
};

interface ExtendedCreateBusinessRequest extends CreateBusinessRequest {
  referred_by_code?: string;
  description?: string;
  custom_url?: string;
  phone?: string;
  email?: string;
  website?: string;
  social_media?: string;
  banner_images?: string;
  business_page_enabled?: boolean;
  show_reviews?: boolean;
  google_reviews_enabled?: boolean;
  counter_enabled?: boolean;
  // Subscription fields (Pay-as-you-go model)
  payment_amount?: string; // USDC amount for subscription
  coupon_code?: string;
  referral_code?: string;
}

export default function BusinessRegisterPage() {
  const router = useRouter();
  const { address, isConnected } = useAccount();
  const { user } = useUserStore();
  const { isAuthenticated } = useAuth();
  const fileInputRef = useRef<HTMLInputElement>(null);
  
  // Translation setup
  const { locale } = useSimpleLocale();
  const [currentLocale, setCurrentLocale] = useState(locale);
  
  // Update translations when locale changes
  useEffect(() => {
    setCurrentLocale(locale);
  }, [locale]);

  const t = (key: string) => {
    const result = getTranslation(`businessRegister.${key}`, currentLocale);
    return Array.isArray(result) ? result : result as string;
  };
  
  const tString = (key: string): string => {
    const result = getTranslation(`businessRegister.${key}`, currentLocale);
    return Array.isArray(result) ? result[0] || key : result as string;
  };
  
  // Smart Contract Hooks
  const { registerBusiness } = useRegisterBusiness();
  const { approveUsdc } = useApproveUsdc();
  const { data: registrationFee } = useRegistrationFee();
  const { data: usdcBalance } = useUsdcBalance();
  const { data: usdcAllowance } = useUsdcAllowance();
  const { data: referrerData } = useReferrer(address || '0x0');
  
  // State
  const [currentStep, setCurrentStep] = useState<FormStep>('business');
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [loadingStep, setLoadingStep] = useState<'approval' | 'registration' | 'backend' | 'upload' | 'complete'>('approval');
  const [showSuccess, setShowSuccess] = useState(false);
  const [registeredBusiness, setRegisteredBusiness] = useState<{id: string, name: string, txHash?: string} | null>(null);
  const [showTutorial, setShowTutorial] = useState(true);
  const [logoFile, setLogoFile] = useState<File | null>(null);
  const [logoPreview, setLogoPreview] = useState<string>('');
  const [uploadingLogo, setUploadingLogo] = useState(false);
  const [referralCodeValid, setReferralCodeValid] = useState<boolean | null>(null);
  const [checkingReferralCode, setCheckingReferralCode] = useState(false);
  
  // Dynamic subscription options based on registration fee
  const subscriptionOptions = getSubscriptionOptions(registrationFee, locale);
  
  // Subscription state
  const [selectedOption, setSelectedOption] = useState<SubscriptionOption>(subscriptionOptions[2]); // Default to 6 months
  
  // Update selected option when registration fee loads
  useEffect(() => {
    if (registrationFee) {
      const updatedOptions = getSubscriptionOptions(registrationFee, locale);
      setSelectedOption(updatedOptions[2]); // Keep 6 months selected but with correct amount
    }
  }, [registrationFee, locale]);
  const [customAmount, setCustomAmount] = useState('');
  const [useCustomAmount, setUseCustomAmount] = useState(false);
  const [discountType, setDiscountType] = useState<'none' | 'coupon' | 'referral'>('none');
  const [couponCode, setCouponCode] = useState('');
  const [referralCode, setReferralCode] = useState('');
  const [couponDiscount, setCouponDiscount] = useState<bigint>(BigInt(0));
  const [referralDiscount, setReferralDiscount] = useState<number>(0);
  const [subscriptionTime, setSubscriptionTime] = useState<number>(0); // in seconds
  
  // Validate coupon with smart contract
  const { 
    isValid: isCouponValid, 
    discountAmount: couponDiscountAmount, 
    isLoading: isCouponLoading 
  } = useValidateCoupon(discountType === 'coupon' && couponCode ? couponCode : '');
  
  const [formData, setFormData] = useState<ExtendedCreateBusinessRequest>({
    name: "",
    logo: "",
    description: "",
    phone: "",
    email: "",
    website: "",
    custom_url: "",
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
    business_page_enabled: true,
    show_reviews: true,
    google_reviews_enabled: false,
    counter_enabled: false,
    referred_by_code: "",
  });

  // Form steps configuration
  const steps: { key: FormStep; title: string; description: string; icon: any }[] = [
    { key: 'business', title: tString('steps.business.title'), description: tString('steps.business.description'), icon: Building2 },
    { key: 'location', title: tString('steps.location.title'), description: tString('steps.location.description'), icon: MapPin },
    { key: 'settings', title: tString('steps.settings.title'), description: tString('steps.settings.description'), icon: Settings },
    { key: 'subscription', title: tString('steps.subscription.title'), description: tString('steps.subscription.description'), icon: DollarSign },
    { key: 'review', title: tString('steps.review.title'), description: tString('steps.review.description'), icon: Check },
  ];

  const currentStepIndex = steps.findIndex(step => step.key === currentStep);
  const progress = ((currentStepIndex + 1) / steps.length) * 100;

  // Calculate subscription time based on payment amount
  const paymentAmountWei = useCustomAmount && customAmount 
    ? parseUsdcAmount(customAmount) 
    : parseUsdcAmount(selectedOption.suggestedAmount);
  const { data: calculatedSubscriptionTime } = useCalculateSubscriptionTime(paymentAmountWei);

  // Helper function to format subscription time
  const formatSubscriptionTime = (seconds: number) => {
    const days = Math.floor(seconds / (24 * 60 * 60));
    const months = Math.floor(days / 30);
    const years = Math.floor(days / 365);
    
    if (years >= 1) {
      return `${years} year${years > 1 ? 's' : ''}`;
    } else if (months >= 1) {
      return `${months} month${months > 1 ? 's' : ''}`;
    } else {
      return `${days} day${days > 1 ? 's' : ''}`;
    }
  };

  // Helper function to calculate final payment amount with discounts
  const calculateFinalAmount = () => {
    const originalAmount = parseFloat(useCustomAmount ? customAmount || '0' : selectedOption.suggestedAmount);
    let finalAmount = originalAmount;
    
    // Apply coupon discount
    if (discountType === 'coupon' && isCouponValid) {
      const discount = Number(formatUsdcAmount(couponDiscountAmount));
      finalAmount = Math.max(0, finalAmount - discount);
    }
    
    // Apply referral discount (placeholder for now)
    if (discountType === 'referral' && referralCodeValid) {
      // TODO: Calculate referral discount from smart contract
      finalAmount = Math.max(0, finalAmount);
    }
    
    return finalAmount.toFixed(2);
  };

  // Authentication check
  useEffect(() => {
    const checkAuth = async () => {
      try {
        const token = getCookie("session_token");
        
        if (!token || !isTokenValid(token)) {
          // Wait a moment to avoid flashing error states
          setTimeout(() => {
            setError("Please sign in to continue.");
            setLoading(false);
          }, 1000);
          return;
        }

        const tokenData = decodeJwt(token);
        
        if (!tokenData.address) {
          setTimeout(() => {
            setError("Invalid session token.");
            setLoading(false);
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
        
        setLoading(false);
      } catch (error) {
        setTimeout(() => {
          setError("Authentication failed. Please try signing in again.");
          setLoading(false);
        }, 1000);
      }
    };

    checkAuth();
  }, [user]);

  // Update settlement address when wallet is connected
  useEffect(() => {
    if (address && isConnected) {
      setFormData(prev => ({
        ...prev,
        settlement_address: address,
        tipping_address: address, // Default to same address, user can change
      }));
    }
  }, [address, isConnected]);

  const handleSubmit = async () => {
    setLoading(true);
    setError(null);
    setLoadingStep('approval');

    try {
      if (!address || !isConnected) {
        throw new Error('Please connect your wallet to register a business');
      }

      // Check if we have sufficient USDC balance
      const regFee = registrationFee ? BigInt(registrationFee.toString()) : BigInt(0);
      const balance = usdcBalance ? BigInt(usdcBalance.toString()) : BigInt(0);
      
      if (regFee > 0 && balance < regFee) {
        throw new Error(`Insufficient USDC balance. Need ${formatUsdcAmount(regFee)} USDC for registration fee`);
      }

      // Step 1: Check and approve USDC if needed
      const allowance = usdcAllowance ? BigInt(usdcAllowance.toString()) : BigInt(0);
      if (regFee > 0 && allowance < regFee) {
        setLoadingStep('approval');
        await approveUsdc(regFee);
        // Wait a moment for the approval to be processed
        await new Promise(resolve => setTimeout(resolve, 2000));
      }

      // Step 2: Register business on smart contract
      setLoadingStep('registration');
      const contractParams: RegisterBusinessParams = {
        name: formData.name,
        paymentAddress: formData.settlement_address as Address,
        tippingAddress: formData.tipping_address as Address,
        referralCode: formData.referred_by_code || undefined,
      };
      
      const txHash = await registerBusiness(contractParams);
      
      // Step 3: Create business profile in backend
      setLoadingStep('backend');
      const businessData = {
        ...formData,
        blockchain_tx_hash: txHash,
        wallet_address: address,
      };
      const business = await createBusiness(businessData);
      
      // Step 4: Upload logo if provided
      if (logoFile && business.id) {
        try {
          setLoadingStep('upload');
          const timestamp = Date.now();
          const randomId = Math.random().toString(36).substring(2, 15);
          const fileName = `logo-${timestamp}-${randomId}-${logoFile.name}`;
          
          const uploadFormData = new FormData();
          uploadFormData.append('file', logoFile);
          uploadFormData.append('name', fileName);
          uploadFormData.append('folder', 'logos');
          uploadFormData.append('business_id', business.id.toString());
          
          const response = await fetch('/api/v1/inside/upload', {
            method: 'POST',
            headers: {
              'Authorization': `Bearer ${getCookie('session_token')}`,
            },
            body: uploadFormData,
          });
          
          if (response.ok) {
            const uploadResult = await response.json();
            console.log('Logo uploaded successfully:', uploadResult.location);
            
            // Update business with logo URL
            try {
              const updateResponse = await fetch(`/api/v1/inside/businesses/${business.id}`, {
                method: 'PATCH',
                headers: {
                  'Content-Type': 'application/json',
                  'Authorization': `Bearer ${getCookie('session_token')}`,
                },
                body: JSON.stringify({
                  logo: uploadResult.location
                }),
              });
              
              if (updateResponse.ok) {
                console.log('Business logo URL updated successfully');
              } else {
                console.error('Failed to update business with logo URL');
              }
            } catch (updateError) {
              console.error('Error updating business with logo URL:', updateError);
            }
          }
        } catch (logoError) {
          console.error('Logo upload failed:', logoError);
          // Don't fail the entire registration for logo upload failure
        }
      }
      
      // Step 5: Complete - Show success page
      setLoadingStep('complete');
      setRegisteredBusiness({
        id: business.id.toString(),
        name: business.name,
        txHash: txHash
      });
      
      // Wait a moment to show completion, then show success page
      setTimeout(() => {
        setLoading(false);
        setShowSuccess(true);
      }, 1500);
      
    } catch (err: any) {
      console.error('Business registration error:', err);
      setError(err.message || 'Failed to create business');
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
    // Update form data with wallet address
    setFormData(prev => ({
      ...prev,
      settlement_address: walletAddress,
      tipping_address: walletAddress,
    }));
    setShowTutorial(false);
  };

  const handleTutorialClose = () => {
    setShowTutorial(false);
  };

  // Email validation
  const isValidEmail = (email: string) => {
    const emailRegex = /^[^\s@]+@[^\s@]+\.[^\s@]+$/;
    return emailRegex.test(email);
  };

  // Logo upload handler - creates preview and stores file for upload after business creation
  const handleLogoUpload = async (file: File) => {
    if (!file) return;
    
    setUploadingLogo(true);
    try {
      // Validate file type
      const allowedTypes = ['image/jpeg', 'image/jpg', 'image/png', 'image/webp'];
      if (!allowedTypes.includes(file.type)) {
        throw new Error('Please upload a valid image file (JPEG, PNG, or WebP)');
      }
      
      // Validate file size (max 5MB)
      const maxSize = 5 * 1024 * 1024; // 5MB
      if (file.size > maxSize) {
        throw new Error('Image file size must be less than 5MB');
      }
      
      // Store the file for upload after business creation
      setLogoFile(file);
      
      // Create preview
      const reader = new FileReader();
      reader.onload = (e) => {
        setLogoPreview(e.target?.result as string);
      };
      reader.readAsDataURL(file);
      
      // Clear any previous errors
      setError(null);
      
    } catch (error: any) {
      console.error('Logo upload failed:', error);
      setError(error.message || 'Failed to process logo. Please try again.');
      setLogoFile(null);
      setLogoPreview('');
    } finally {
      setUploadingLogo(false);
    }
  };

  // Referral code validation
  const validateReferralCode = async (code: string) => {
    if (!code.trim()) {
      setReferralCodeValid(null);
      return;
    }
    
    setCheckingReferralCode(true);
    try {
      const isValid = await checkReferralCodeAvailability(code);
      setReferralCodeValid(!isValid); // If available, it means no one is using it (invalid for referral)
    } catch (error) {
      console.error('Failed to validate referral code:', error);
      setReferralCodeValid(false);
    } finally {
      setCheckingReferralCode(false);
    }
  };

  // Step navigation
  const nextStep = () => {
    const currentIndex = steps.findIndex(step => step.key === currentStep);
    if (currentIndex < steps.length - 1) {
      setCurrentStep(steps[currentIndex + 1].key);
    }
  };

  const prevStep = () => {
    const currentIndex = steps.findIndex(step => step.key === currentStep);
    if (currentIndex > 0) {
      setCurrentStep(steps[currentIndex - 1].key);
    }
  };

  const goToStep = (step: FormStep) => {
    setCurrentStep(step);
  };

  // Form validation for each step
  const isStepValid = (step: FormStep): boolean => {
    switch (step) {
      case 'business':
        return formData.name.trim().length > 0;
      case 'location':
        return formData.settlement_address.trim().length > 0 && 
               formData.tipping_address.trim().length > 0;
      case 'settings':
        return true; // Settings step is always valid
      case 'subscription':
        const paymentValid = useCustomAmount ? 
          (customAmount.trim().length > 0 && parseFloat(customAmount) > 0) : 
          selectedOption !== null;
        
        // Check discount validation
        const discountValid = discountType === 'none' || 
          (discountType === 'coupon' && couponCode.trim().length > 0 && isCouponValid) ||
          (discountType === 'referral' && referralCode.trim().length > 0 && referralCodeValid === true);
        
        return paymentValid && discountValid;
      case 'review':
        return isStepValid('business') && isStepValid('location') && isStepValid('subscription');
      default:
        return true;
    }
  };

  // Show loading screen while checking authentication
  if (loading) {
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
            <p className="text-gray-600 font-light tracking-wide">{tString('messages.loading')}</p>
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
            <h2 className="text-2xl font-light text-gray-900 mb-4 tracking-wide">{tString('messages.authenticationRequired')}</h2>
            <p className="text-gray-600 font-light tracking-wide mb-8">{error}</p>
            
            <div className="space-y-4">
              <p className="text-sm text-gray-500">{tString('messages.authenticationMessage')}</p>
              <button
                onClick={() => window.location.reload()}
                className="px-6 py-3 text-sm font-medium text-white bg-gray-900 border border-transparent rounded-md hover:bg-gray-800 transition-colors duration-200"
              >
                {tString('messages.tryAgain')}
              </button>
            </div>
            
            <div className="mt-8">
              <Link 
                href="/"
                className="text-sm text-gray-500 hover:text-gray-700 transition-colors duration-200"
              >
                ← {tString('messages.backToDashboard')}
              </Link>
            </div>
          </div>
        </div>
      </div>
    );
  }

  // Show success page after registration
  if (showSuccess && registeredBusiness) {
    return (
      <BusinessRegistrationSuccess
        businessName={registeredBusiness.name}
        businessId={registeredBusiness.id}
        transactionHash={registeredBusiness.txHash}
      />
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

      {/* Smart Contract Loading Overlay */}
      <SmartContractLoadingOverlay
        isVisible={loading}
        currentStep={loadingStep}
        businessName={formData.name}
        error={error}
      />

      <div className="min-h-screen bg-white relative overflow-hidden">
        {/* Clean, minimal background */}
        <div className="absolute inset-0 overflow-hidden pointer-events-none">
          <div className="absolute -top-40 -right-40 w-80 h-80 bg-gray-100 rounded-full blur-3xl opacity-10"></div>
          <div className="absolute -bottom-40 -left-40 w-96 h-96 bg-gray-100 rounded-full blur-3xl opacity-8"></div>
        </div>
        
        <div className="max-w-5xl mx-auto px-4 sm:px-6 py-8 sm:py-12 relative">
          {/* Header */}
          <div className="mb-8 sm:mb-16">
            <div className="flex flex-col sm:flex-row justify-between items-start sm:items-center gap-4 mb-6 sm:mb-8">
              <Link href="/">
                <Button
                  variant="light"
                  startContent={<ArrowLeft size={16} />}
                  className="text-gray-600 hover:text-gray-800 transition-all duration-300 hover:bg-white hover:shadow-sm rounded-xl px-4 py-2"
                >
                  {tString('messages.backToDashboard')}
                </Button>
              </Link>
            </div>
            <div className="text-center">
              <h1 className="text-3xl sm:text-5xl lg:text-7xl font-medium text-gray-900 mb-6 sm:mb-8 leading-tight tracking-tight">
                {tString('header.title')}
                <span className="text-gray-900 font-medium block">
                  {tString('header.titleEmphasis')}
                </span>
              </h1>
              <p className="text-lg font-light text-gray-600 leading-relaxed tracking-wide max-w-2xl mx-auto">
                {tString('header.subtitle')}
              </p>
            </div>
          </div>

          {/* Multi-Step Form */}
          <div className="space-y-8">
            {/* Step Content */}
            <Card className="shadow-sm border border-gray-200 bg-white">
              <CardBody className="p-6 sm:p-12">
                {/* Progress Section */}
                <div className="mb-8">
                  <div className="flex flex-col sm:flex-row justify-between items-start sm:items-center gap-4 sm:gap-0 mb-4">
                    <div className="text-center sm:text-left">
                      <span className="text-sm font-medium text-gray-700 uppercase tracking-wide">
                        {tString('messages.stepProgress').replace('{current}', (currentStepIndex + 1).toString()).replace('{total}', steps.length.toString())}
                      </span>
                    </div>
                    <div className="text-center sm:text-right">
                      <span className="text-2xl font-medium text-gray-900">
                        {Math.round(progress)}%
                      </span>
                      <p className="text-sm text-gray-500">{tString('messages.complete')}</p>
                    </div>
                  </div>
                  <Progress 
                    value={progress} 
                    color="primary"
                    size="md"
                    classNames={{
                      track: "bg-gray-100",
                      indicator: "bg-gray-900"
                    }}
                  />
                  
                  {/* Error Message */}
                  {error && (
                    <div className="mt-4 p-3 bg-red-50 border border-red-200 rounded-lg">
                      <p className="text-red-700 text-sm flex items-center gap-2">
                        <span className="w-2 h-2 bg-red-500 rounded-full"></span>
                        {error}
                      </p>
                    </div>
                  )}
                </div>
                {/* Business Information Step */}
                {currentStep === 'business' && (
                  <div className="space-y-8">
                    <div className="text-center mb-12">
                      <div className="w-20 h-20 bg-gray-50 border-2 border-gray-200 rounded-3xl flex items-center justify-center mx-auto mb-8 shadow-lg">
                        <Building2 className="text-gray-700" size={40} />
                      </div>
                      <h2 className="text-2xl sm:text-3xl lg:text-4xl font-light text-gray-900 mb-6 tracking-wide">{tString('businessInfo.title')}</h2>
                      <p className="text-base sm:text-lg font-light text-gray-600 leading-relaxed tracking-wide max-w-2xl mx-auto px-4 sm:px-0">{tString('businessInfo.description')}</p>
                    </div>

                    <div className="max-w-3xl mx-auto space-y-10">
                      {/* Business Name */}
                      <div>
                        <div className="group">
                          <Input
                            label={tString('businessInfo.fields.name.label')}
                            placeholder={tString('businessInfo.fields.name.placeholder')}
                            value={formData.name}
                            onChange={(e) => updateFormData('name', e.target.value)}
                            isRequired
                            size="lg"
                            startContent={<Building2 size={22} className="text-gray-600" />}
                            classNames={{
                              input: "text-lg font-medium text-gray-900",
                              inputWrapper: "h-16 bg-gray-50 border-2 border-gray-200 group-hover:border-gray-300 transition-colors duration-300 shadow-sm hover:shadow-md",
                              label: "text-base font-medium text-gray-600"
                            }}
                          />
                        </div>
                      </div>
                      {/* Logo Upload */}
                      <div className="space-y-6">
                        <div>
                          <label className="text-base font-medium text-gray-700 mb-2 block">{tString('businessInfo.logo.title')}</label>
                          <p className="text-sm text-gray-500">{tString('businessInfo.logo.description')}</p>
                        </div>
                        <div className="bg-gray-50 p-4 sm:p-8 rounded-2xl border border-gray-200 hover:border-gray-300 transition-all duration-300">
                          <div className="flex flex-col md:flex-row items-center gap-4 sm:gap-8">
                            {/* Logo Preview */}
                            <div className="relative group">
                              <div className="w-32 h-32 border border-gray-200 rounded-2xl flex items-center justify-center bg-white shadow-sm group-hover:shadow-sm transition-all duration-300">
                                {logoPreview ? (
                                  <Image 
                                    src={logoPreview} 
                                    alt="Logo preview" 
                                    width={128}
                                    height={128}
                                    className="w-full h-full object-cover rounded-2xl"
                                  />
                                ) : (
                                  <Camera size={32} className="text-gray-400" />
                                )}
                              </div>
                              {logoPreview && (
                                <div className="absolute -top-2 -right-2 w-6 h-6 bg-green-500 rounded-full flex items-center justify-center">
                                  <Check size={14} className="text-white" />
                                </div>
                              )}
                            </div>
                            
                            {/* Upload Section */}
                            <div className="flex-1 text-center md:text-left">
                              <input
                                ref={fileInputRef}
                                type="file"
                                accept="image/jpeg,image/jpg,image/png,image/webp"
                                onChange={(e) => {
                                  const file = e.target.files?.[0];
                                  if (file) {
                                    setLogoFile(file);
                                    handleLogoUpload(file);
                                  }
                                }}
                                className="hidden"
                              />
                              <Button
                                type="button"
                                variant="solid"
                                color="primary"
                                onPress={() => fileInputRef.current?.click()}
                                isLoading={uploadingLogo}
                                startContent={!uploadingLogo && <Upload size={20} />}
                                className="h-14 px-8 text-base font-medium bg-gray-900 hover:bg-gray-800 shadow-sm hover:shadow-sm transition-all duration-300"
                                size="lg"
                              >
                                {uploadingLogo ? tString('businessInfo.logo.uploading') : logoPreview ? tString('businessInfo.logo.change') : tString('businessInfo.logo.upload')}
                              </Button>
                              <div className="mt-4 space-y-1">
                                <p className="text-sm font-light text-gray-600">
                                  {tString('businessInfo.logo.guidelines')}
                                </p>
                              </div>
                            </div>
                          </div>
                        </div>
                      </div>

                      {/* Contact Information */}
                      <div className="grid grid-cols-1 md:grid-cols-2 gap-4 sm:gap-6">
                        <Input
                          label="Business Email"
                          placeholder="contact@yourbusiness.com"
                          type="email"
                          value={formData.email || ''}
                          onChange={(e) => updateFormData('email', e.target.value)}
                          variant="bordered"
                          isRequired
                          size="lg"
                          startContent={<span className="text-gray-400 text-lg">@</span>}
                          isInvalid={formData.email ? !isValidEmail(formData.email) : false}
                          errorMessage={formData.email && !isValidEmail(formData.email) ? tString('errors.invalidEmail') : ""}
                          classNames={{
                            inputWrapper: "h-14 border-2",
                            label: "font-medium"
                          }}
                        />

                        <Input
                          label={tString('businessInfo.fields.phone.label')}
                          placeholder={tString('businessInfo.fields.phone.placeholder')}
                          value={formData.phone || ''}
                          onChange={(e) => updateFormData('phone', e.target.value)}
                          variant="bordered"
                          size="lg"
                          startContent={<Phone size={20} className="text-gray-400" />}
                          classNames={{
                            inputWrapper: "h-14 border-2",
                            label: "font-medium"
                          }}
                        />
                      </div>

                      <div className="grid grid-cols-1 md:grid-cols-2 gap-6">
                        <Input
                          label={tString('businessInfo.fields.website.label')}
                          placeholder={tString('businessInfo.fields.website.placeholder')}
                          value={formData.website || ''}
                          onChange={(e) => updateFormData('website', e.target.value)}
                          variant="bordered"
                          size="lg"
                          startContent={<Globe size={20} className="text-gray-400" />}
                          classNames={{
                            inputWrapper: "h-14 border-2",
                            label: "font-medium"
                          }}
                        />

                        <Input
                          label={tString('businessInfo.fields.customUrl.label')}
                          placeholder={tString('businessInfo.fields.customUrl.placeholder')}
                          value={formData.custom_url || ''}
                          onChange={(e) => updateFormData('custom_url', e.target.value.toLowerCase().replace(/[^a-z0-9-]/g, ''))}
                          variant="bordered"
                          size="lg"
                          startContent={<span className="text-gray-400 text-sm">payverge.io/b/</span>}
                          description={tString('review.labels.publicPageUrl')}
                          classNames={{
                            inputWrapper: "h-14 border-2",
                            label: "font-medium"
                          }}
                        />
                      </div>

                      <Textarea
                        label={tString('businessInfo.fields.description.label')}
                        placeholder={tString('businessInfo.fields.description.placeholder')}
                        value={formData.description || ''}
                        onChange={(e) => updateFormData('description', e.target.value)}
                        variant="bordered"
                        minRows={4}
                        maxRows={6}
                        classNames={{
                          inputWrapper: "border-2",
                          label: "font-medium"
                        }}
                      />
                    </div>
                  </div>
                )}

                {/* Location & Wallets Step */}
                {currentStep === 'location' && (
                  <div className="space-y-8">
                    <div className="text-center mb-10">
                      <div className="w-16 h-16 bg-gray-100 border border-gray-200 rounded-2xl flex items-center justify-center mx-auto mb-6">
                        <MapPin className="text-gray-700" size={32} />
                      </div>
                      <h2 className="text-2xl sm:text-3xl lg:text-4xl font-light text-gray-900 mb-6 tracking-wide">{tString('location.title')}</h2>
                      <p className="text-base sm:text-lg font-light text-gray-600 leading-relaxed tracking-wide px-4 sm:px-0">{tString('location.description')}</p>
                    </div>

                    <div className="max-w-3xl mx-auto space-y-10">
                      {/* Physical Address */}
                      <div className="bg-gray-50 p-8 rounded-2xl border border-gray-200">
                        <h3 className="text-xl font-medium text-gray-900 mb-6 flex items-center gap-3">
                          <div className="w-8 h-8 bg-gray-100 rounded-lg flex items-center justify-center">
                            <MapPin size={18} className="text-gray-600" />
                          </div>
                          {tString('location.address.title')}
                        </h3>
                        <div className="grid grid-cols-1 md:grid-cols-2 gap-6">
                          <div className="md:col-span-2">
                            <Input
                              label={tString('location.address.street.label')}
                              placeholder={tString('location.address.street.placeholder')}
                              value={formData.address.street}
                              onChange={(e) => updateAddress('street', e.target.value)}
                              variant="bordered"
                              size="lg"
                              classNames={{
                                inputWrapper: "h-14 border-2",
                                label: "font-medium"
                              }}
                            />
                          </div>
                          <Input
                            label={tString('location.address.city.label')}
                            placeholder={tString('location.address.city.placeholder')}
                            value={formData.address.city}
                            onChange={(e) => updateAddress('city', e.target.value)}
                            variant="bordered"
                            size="lg"
                            classNames={{
                              inputWrapper: "h-14 border-2",
                              label: "font-medium"
                            }}
                          />
                          <Input
                            label={tString('location.address.state.label')}
                            placeholder={tString('location.address.state.placeholder')}
                            value={formData.address.state}
                            onChange={(e) => updateAddress('state', e.target.value)}
                            variant="bordered"
                            size="lg"
                            classNames={{
                              inputWrapper: "h-14 border-2",
                              label: "font-medium"
                            }}
                          />
                          <Input
                            label={tString('location.address.postalCode.label')}
                            placeholder={tString('location.address.postalCode.placeholder')}
                            value={formData.address.postal_code}
                            onChange={(e) => updateAddress('postal_code', e.target.value)}
                            variant="bordered"
                            size="lg"
                            classNames={{
                              inputWrapper: "h-14 border-2",
                              label: "font-medium"
                            }}
                          />
                          <Input
                            label={tString('location.address.country.label')}
                            placeholder={tString('location.address.country.placeholder')}
                            value={formData.address.country}
                            onChange={(e) => updateAddress('country', e.target.value)}
                            variant="bordered"
                            size="lg"
                            classNames={{
                              inputWrapper: "h-14 border-2",
                              label: "font-medium"
                            }}
                          />
                        </div>
                      </div>

                      {/* Crypto Addresses */}
                      <div className="bg-gray-50 p-8 rounded-2xl border border-gray-200">
                        <h3 className="text-xl font-medium text-gray-900 mb-6 flex items-center gap-3">
                          <div className="w-8 h-8 bg-gray-100 rounded-lg flex items-center justify-center">
                            <Wallet size={18} className="text-gray-600" />
                          </div>
                          {tString('location.wallets.title')}
                        </h3>
                        <div className="space-y-6">
                          <Input
                            label={tString('location.wallets.settlement.label')}
                            placeholder={tString('location.wallets.settlement.placeholder')}
                            value={formData.settlement_address}
                            onChange={(e) => updateFormData('settlement_address', e.target.value)}
                            variant="bordered"
                            size="lg"
                            isRequired
                            description={tString('location.wallets.settlement.description')}
                            startContent={<Wallet size={20} className="text-gray-400" />}
                            classNames={{
                              inputWrapper: "h-14 border-2",
                              label: "font-medium"
                            }}
                          />
                          <Input
                            label={tString('location.wallets.tipping.label')}
                            placeholder={tString('location.wallets.tipping.placeholder')}
                            value={formData.tipping_address}
                            onChange={(e) => updateFormData('tipping_address', e.target.value)}
                            variant="bordered"
                            size="lg"
                            isRequired
                            description={tString('location.wallets.tipping.description')}
                            startContent={<DollarSign size={20} className="text-gray-400" />}
                            classNames={{
                              inputWrapper: "h-14 border-2",
                              label: "font-medium"
                            }}
                          />
                          {isConnected && (
                            <div className="bg-white p-4 rounded-xl border border-gray-200">
                              <div className="flex items-center justify-between">
                                <div>
                                  <p className="font-medium text-gray-900">{tString('review.labels.connectedWallet')}</p>
                                  <p className="text-sm text-gray-600 font-mono">{address}</p>
                                </div>
                                <Button
                                  size="sm"
                                  color="primary"
                                  variant="flat"
                                  onPress={() => {
                                    updateFormData('settlement_address', address || '');
                                    updateFormData('tipping_address', address || '');
                                  }}
                                >
                                  {tString('location.wallets.settlement.useConnected')}
                                </Button>
                              </div>
                            </div>
                          )}
                        </div>
                      </div>
                    </div>
                  </div>
                )}

                {/* Settings & Referral Step */}
                {currentStep === 'settings' && (
                  <div className="space-y-8">
                    <div className="text-center mb-10">
                      <div className="w-16 h-16 bg-gray-100 border border-gray-200 rounded-2xl flex items-center justify-center mx-auto mb-6">
                        <Settings className="text-gray-700" size={32} />
                      </div>
                      <h2 className="text-2xl sm:text-3xl lg:text-4xl font-light text-gray-900 mb-6 tracking-wide">{tString('settings.title')}</h2>
                      <p className="text-base sm:text-lg font-light text-gray-600 leading-relaxed tracking-wide px-4 sm:px-0">{tString('settings.description')}</p>
                    </div>

                    <div className="max-w-3xl mx-auto space-y-10">
                      {/* Fee Settings */}
                      <div className="bg-gray-50 p-8 rounded-2xl border border-gray-200">
                        <h3 className="text-xl font-medium text-gray-900 mb-6 flex items-center gap-3">
                          <div className="w-8 h-8 bg-gray-100 rounded-lg flex items-center justify-center">
                            <Percent size={18} className="text-gray-600" />
                          </div>
                          {tString('settings.fees.title')}
                        </h3>
                        <div className="grid grid-cols-1 md:grid-cols-2 gap-6">
                          <Input
                            label={tString('settings.fees.taxRate.label')}
                            placeholder={tString('settings.fees.taxRate.placeholder')}
                            type="number"
                            step="0.01"
                            min="0"
                            max="100"
                            value={formData.tax_rate?.toString() || ''}
                            onChange={(e) => updateFormData('tax_rate', parseFloat(e.target.value) || 0)}
                            variant="bordered"
                            size="lg"
                            startContent={<Percent size={20} className="text-gray-400" />}
                            classNames={{
                              inputWrapper: "h-14 border-2",
                              label: "font-medium"
                            }}
                          />
                          <Input
                            label={tString('settings.fees.serviceFee.label')}
                            placeholder={tString('settings.fees.serviceFee.placeholder')}
                            type="number"
                            step="0.01"
                            min="0"
                            max="100"
                            value={formData.service_fee_rate?.toString() || ''}
                            onChange={(e) => updateFormData('service_fee_rate', parseFloat(e.target.value) || 0)}
                            variant="bordered"
                            size="lg"
                            startContent={<Percent size={20} className="text-gray-400" />}
                            classNames={{
                              inputWrapper: "h-14 border-2",
                              label: "font-medium"
                            }}
                          />
                        </div>
                        <div className="flex flex-col sm:flex-row gap-4 mt-6">
                          <Checkbox
                            isSelected={formData.tax_inclusive}
                            onValueChange={(checked) => updateFormData('tax_inclusive', checked)}
                            classNames={{ label: "font-medium" }}
                          >
                            {tString('settings.fees.taxInclusive')}
                          </Checkbox>
                          <Checkbox
                            isSelected={formData.service_inclusive}
                            onValueChange={(checked) => updateFormData('service_inclusive', checked)}
                            classNames={{ label: "font-medium" }}
                          >
                            {tString('settings.fees.serviceInclusive')}
                          </Checkbox>
                        </div>
                      </div>

                      {/* Business Features */}
                      <div className="bg-gray-50 p-8 rounded-2xl border border-gray-200">
                        <h3 className="text-xl font-medium text-gray-900 mb-6 flex items-center gap-3">
                          <div className="w-8 h-8 bg-gray-100 rounded-lg flex items-center justify-center">
                            <Settings size={18} className="text-gray-600" />
                          </div>
                          {tString('settings.features.title')}
                        </h3>
                        <div className="space-y-6">
                          <div className="flex items-center justify-between p-4 bg-white rounded-xl border border-gray-200">
                            <div>
                              <h4 className="font-medium text-gray-900">{tString('settings.features.businessPage')}</h4>
                              <p className="text-sm text-gray-600">{tString('settings.features.businessPageDescription')}</p>
                            </div>
                            <Switch
                              isSelected={formData.business_page_enabled}
                              onValueChange={(checked) => updateFormData('business_page_enabled', checked)}
                              color="success"
                            />
                          </div>
                          <div className="flex items-center justify-between p-4 bg-white rounded-xl border border-gray-200">
                            <div>
                              <h4 className="font-medium text-gray-900">{tString('settings.features.showReviews')}</h4>
                              <p className="text-sm text-gray-600">{tString('settings.features.showReviewsDescription')}</p>
                            </div>
                            <Switch
                              isSelected={formData.show_reviews}
                              onValueChange={(checked) => updateFormData('show_reviews', checked)}
                              color="success"
                            />
                          </div>
                          <div className="flex items-center justify-between p-4 bg-white rounded-xl border border-gray-200">
                            <div>
                              <h4 className="font-medium text-gray-900">{tString('settings.features.counter')}</h4>
                              <p className="text-sm text-gray-600">{tString('settings.features.counterDescription')}</p>
                            </div>
                            <Switch
                              isSelected={formData.counter_enabled}
                              onValueChange={(checked) => updateFormData('counter_enabled', checked)}
                              color="success"
                            />
                          </div>
                        </div>
                      </div>

                    </div>
                  </div>
                )}

                {/* Subscription Payment Step */}
                {currentStep === 'subscription' && (
                  <div className="space-y-8">
                    <div className="text-center mb-10">
                      <div className="w-16 h-16 bg-gray-100 border border-gray-200 rounded-2xl flex items-center justify-center mx-auto mb-6">
                        <DollarSign className="text-gray-700" size={32} />
                      </div>
                      <h2 className="text-2xl sm:text-3xl lg:text-4xl font-light text-gray-900 mb-6 tracking-wide">{tString('subscription.title')}</h2>
                      <p className="text-base sm:text-lg font-light text-gray-600 leading-relaxed tracking-wide px-4 sm:px-0">{tString('subscription.description')}</p>
                    </div>

                    <div className="max-w-4xl mx-auto">
                      {/* Payment Options */}
                      <div className="mb-8">
                        <h3 className="text-lg font-medium text-gray-900 mb-4">{tString('subscription.suggestedOptions')}</h3>
                        <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-3 sm:gap-4 mb-6">
                          {subscriptionOptions.map((option) => (
                            <Card
                              key={option.months}
                              isPressable
                              isHoverable
                              onPress={() => {
                                setSelectedOption(option);
                                setUseCustomAmount(false);
                                setCustomAmount('');
                              }}
                              className={`relative transition-all duration-200 ${
                                !useCustomAmount && selectedOption.months === option.months 
                                  ? 'ring-2 ring-blue-500 shadow-lg scale-105' 
                                  : 'hover:shadow-md'
                              } ${option.popular ? 'border-2 border-blue-500' : ''}`}
                            >
                              {option.popular && (
                                <div className="absolute -top-2 left-1/2 transform -translate-x-1/2">
                                  <span className="bg-gradient-to-r from-blue-500 to-purple-600 text-white px-3 py-1 rounded-full text-xs font-medium">
                                    {tString('subscription.popular')}
                                  </span>
                                </div>
                              )}
                              <CardBody className="p-4 text-center">
                                <div className="text-2xl font-bold text-gray-900 mb-1">
                                  ${option.suggestedAmount}
                                </div>
                                <div className="text-sm text-gray-600 mb-2">
                                  {option.description}
                                </div>
                                <div className="text-xs text-blue-600">
                                  {!useCustomAmount && selectedOption.months === option.months && calculatedSubscriptionTime 
                                    ? formatSubscriptionTime(Number(calculatedSubscriptionTime.toString()))
                                    : `~${option.months} month${option.months > 1 ? 's' : ''}`
                                  }
                                </div>
                              </CardBody>
                            </Card>
                          ))}
                        </div>
                        
                        {/* Custom Amount Option */}
                        <Card 
                          className={`transition-all duration-200 ${
                            useCustomAmount ? 'ring-2 ring-blue-500 shadow-lg' : 'hover:shadow-md'
                          }`}
                        >
                          <CardBody className="p-6">
                            <div className="flex items-center gap-4">
                              <Checkbox
                                isSelected={useCustomAmount}
                                onValueChange={(checked) => {
                                  setUseCustomAmount(checked);
                                  if (!checked) {
                                    setCustomAmount('');
                                  }
                                }}
                                color="primary"
                              >
                                <span className="font-medium">{tString('subscription.custom.enable')}</span>
                              </Checkbox>
                              {useCustomAmount && (
                                <Input
                                  placeholder={tString('subscription.custom.placeholder')}
                                  value={customAmount}
                                  onChange={(e) => {
                                    const value = e.target.value;
                                    const yearlyFee = registrationFee && typeof registrationFee === 'bigint' 
                                      ? Number(formatUsdcAmount(registrationFee)) 
                                      : 120;
                                    
                                    // Allow the input but we'll show warnings for amounts above yearly fee
                                    setCustomAmount(value);
                                  }}
                                  variant="bordered"
                                  size="sm"
                                  startContent={<DollarSign size={16} className="text-gray-400" />}
                                  classNames={{
                                    inputWrapper: "h-10 border-2",
                                  }}
                                  className="w-40"
                                />
                              )}
                            </div>
                            {useCustomAmount && (
                              <div className="mt-2 space-y-1">
                                {(() => {
                                  const yearlyFee = registrationFee && typeof registrationFee === 'bigint' 
                                    ? Number(formatUsdcAmount(registrationFee)) 
                                    : 120;
                                  const customAmountNum = parseFloat(customAmount || '0');
                                  const isOverYearlyFee = customAmountNum > yearlyFee;
                                  
                                  return (
                                    <>
                                      <p className="text-sm text-gray-600">
                                        {tString('messages.customAmountDescription').replace('${amount}', yearlyFee.toString())}
                                      </p>
                                      {isOverYearlyFee && (
                                        <p className="text-sm font-medium text-amber-600 bg-amber-50 px-2 py-1 rounded">
                                          {tString('messages.customAmountWarning').replace('${amount}', yearlyFee.toString())}
                                        </p>
                                      )}
                                      {customAmount && calculatedSubscriptionTime ? (
                                        <p className="text-sm font-medium text-blue-600">
                                          {tString('messages.customAmountEstimate').replace('{time}', formatSubscriptionTime(Number(calculatedSubscriptionTime.toString())))}
                                        </p>
                                      ) : null}
                                    </>
                                  );
                                })()}
                              </div>
                            )}
                          </CardBody>
                        </Card>
                      </div>

                      {/* Discount Options */}
                      <div className="space-y-6">
                        <div>
                          <h3 className="text-lg font-medium text-gray-900 mb-4">{tString('subscription.discount.title')}</h3>
                          
                          {/* Discount Type Selection */}
                          <div className="grid grid-cols-1 md:grid-cols-3 gap-4 mb-4">
                            <Card 
                              isPressable
                              onPress={() => {
                                setDiscountType('none');
                                setCouponCode('');
                                setReferralCode('');
                              }}
                              className={`transition-all duration-200 ${
                                discountType === 'none' ? 'ring-2 ring-blue-500 shadow-lg' : 'hover:shadow-md'
                              }`}
                            >
                              <CardBody className="p-4 text-center">
                                <div className="text-lg font-medium text-gray-900">{tString('subscription.discount.none')}</div>
                                <div className="text-sm text-gray-600">{tString('subscription.discount.noneDescription')}</div>
                              </CardBody>
                            </Card>
                            
                            <Card 
                              isPressable
                              onPress={() => {
                                setDiscountType('coupon');
                                setReferralCode('');
                              }}
                              className={`transition-all duration-200 ${
                                discountType === 'coupon' ? 'ring-2 ring-blue-500 shadow-lg' : 'hover:shadow-md'
                              }`}
                            >
                              <CardBody className="p-4 text-center">
                                <div className="text-lg font-medium text-gray-900">{tString('subscription.discount.coupon')}</div>
                                <div className="text-sm text-gray-600">{tString('subscription.discount.couponDescription')}</div>
                              </CardBody>
                            </Card>
                            
                            <Card 
                              isPressable
                              onPress={() => {
                                setDiscountType('referral');
                                setCouponCode('');
                              }}
                              className={`transition-all duration-200 ${
                                discountType === 'referral' ? 'ring-2 ring-blue-500 shadow-lg' : 'hover:shadow-md'
                              }`}
                            >
                              <CardBody className="p-4 text-center">
                                <div className="text-lg font-medium text-gray-900">{tString('settings.referral.title')}</div>
                                <div className="text-sm text-gray-600">{tString('subscription.discount.referralDescription')}</div>
                              </CardBody>
                            </Card>
                          </div>

                          {/* Discount Input */}
                          {discountType === 'coupon' && (
                            <div className="space-y-2">
                              <Input
                                label={tString('subscription.discount.couponLabel')}
                                placeholder={tString('subscription.discount.couponPlaceholder')}
                                value={couponCode}
                                onChange={(e) => setCouponCode(e.target.value.toUpperCase())}
                                variant="bordered"
                                size="lg"
                                startContent={<Hash size={20} className="text-gray-400" />}
                                classNames={{
                                  inputWrapper: "h-14 border-2",
                                  label: "font-medium"
                                }}
                                color={
                                  couponCode ? (
                                    isCouponLoading ? "default" :
                                    isCouponValid ? "success" : "danger"
                                  ) : "default"
                                }
                                description={
                                  couponCode && !isCouponLoading ? (
                                    isCouponValid ? 
                                      tString('validation.couponValid').replace('${amount}', formatUsdcAmount(couponDiscountAmount)) :
                                      tString('validation.couponInvalid')
                                  ) : tString('validation.couponPrompt')
                                }
                              />
                            </div>
                          )}
                          
                          {discountType === 'referral' && (
                            <Input
                              label={tString('settings.referral.title')}
                              placeholder={tString('settings.referral.placeholder')}
                              value={referralCode}
                              onChange={(e) => {
                                setReferralCode(e.target.value);
                                if (e.target.value.length >= 3) {
                                  // Validate referral code
                                  setCheckingReferralCode(true);
                                  checkReferralCodeAvailability(e.target.value)
                                    .then((response) => {
                                      setReferralCodeValid(response.available);
                                      setCheckingReferralCode(false);
                                    })
                                    .catch(() => {
                                      setReferralCodeValid(false);
                                      setCheckingReferralCode(false);
                                    });
                                } else {
                                  setReferralCodeValid(null);
                                }
                              }}
                              variant="bordered"
                              size="lg"
                              startContent={<Hash size={20} className="text-gray-400" />}
                              isInvalid={referralCodeValid === false}
                              color={referralCodeValid === true ? "success" : referralCodeValid === false ? "danger" : "default"}
                              description={
                                referralCodeValid === true 
                                  ? tString('validation.referralValid')
                                  : referralCodeValid === false 
                                  ? tString('validation.referralInvalid')
                                  : tString('validation.referralPrompt')
                              }
                              endContent={
                                checkingReferralCode ? (
                                  <Spinner size="sm" />
                                ) : referralCodeValid === true ? (
                                  <Check size={20} className="text-green-500" />
                                ) : null
                              }
                              classNames={{
                                inputWrapper: "h-14 border-2",
                                label: "font-medium"
                              }}
                            />
                          )}
                        </div>

                        {/* Payment Summary */}
                        <Card className="bg-gray-50 border border-gray-200">
                          <CardBody className="p-6">
                            <h4 className="text-lg font-medium text-gray-900 mb-4">{tString('subscription.summary.title')}</h4>
                            <div className="space-y-2">
                              <div className="flex justify-between">
                                <span className="text-gray-600">{tString('subscription.summary.subscriptionPayment')}</span>
                                <span className="font-medium">
                                  ${useCustomAmount ? customAmount || '0.00' : selectedOption.suggestedAmount}
                                </span>
                              </div>
                              {discountType === 'coupon' && isCouponValid && (
                                <div className="flex justify-between text-green-600">
                                  <span>{tString('subscription.summary.couponDiscount')}</span>
                                  <span>-${formatUsdcAmount(couponDiscountAmount)}</span>
                                </div>
                              )}
                              {discountType === 'referral' && referralCodeValid && (
                                <div className="flex justify-between text-green-600">
                                  <span>{tString('subscription.summary.referralDiscount')}</span>
                                  <span>-$0.00</span>
                                </div>
                              )}
                              <Divider />
                              <div className="flex justify-between text-lg font-semibold">
                                <span>{tString('subscription.summary.total')}</span>
                                <span>
                                  ${calculateFinalAmount()}
                                </span>
                              </div>
                              {calculatedSubscriptionTime ? (
                                <div className="text-sm text-gray-600 mt-2">
                                  {tString('review.labels.estimatedTime').replace('{time}', formatSubscriptionTime(Number(calculatedSubscriptionTime.toString())))}
                                </div>
                              ) : null}
                            </div>
                          </CardBody>
                        </Card>
                      </div>
                    </div>
                  </div>
                )}

                {/* Review Step */}
                {currentStep === 'review' && (
                  <div className="space-y-8">
                    <div className="text-center mb-10">
                      <div className="w-16 h-16 bg-gray-100 border border-gray-200 rounded-2xl flex items-center justify-center mx-auto mb-6">
                        <Check className="text-gray-700" size={32} />
                      </div>
                      <h2 className="text-2xl sm:text-3xl lg:text-4xl font-light text-gray-900 mb-6 tracking-wide">{tString('review.title')}</h2>
                      <p className="text-base sm:text-lg font-light text-gray-600 leading-relaxed tracking-wide px-4 sm:px-0">{tString('review.description')}</p>
                    </div>

                    <div className="max-w-4xl mx-auto space-y-6 sm:space-y-8">
                      {/* Business Information Review */}
                      <div className="bg-gray-50 p-8 rounded-2xl border border-gray-200">
                        <h3 className="text-xl font-medium text-gray-900 mb-6 flex items-center gap-3">
                          <div className="w-8 h-8 bg-gray-100 rounded-lg flex items-center justify-center">
                            <Building2 size={18} className="text-gray-600" />
                          </div>
                          {tString('review.sections.business')}
                        </h3>
                        <div className="grid grid-cols-1 md:grid-cols-2 gap-6">
                          <div className="bg-white p-4 rounded-xl">
                            <span className="text-sm font-medium text-gray-500 uppercase tracking-wide">{tString('review.labels.businessName')}</span>
                            <p className="text-lg font-medium text-gray-900 mt-1">{formData.name}</p>
                          </div>
                          {formData.email && (
                            <div className="bg-white p-4 rounded-xl">
                              <span className="text-sm font-medium text-gray-500 uppercase tracking-wide">{tString('review.labels.email')}</span>
                              <p className="text-lg font-medium text-gray-900 mt-1">{formData.email}</p>
                            </div>
                          )}
                          {formData.phone && (
                            <div className="bg-white p-4 rounded-xl">
                              <span className="text-sm font-medium text-gray-500 uppercase tracking-wide">{tString('review.labels.phone')}</span>
                              <p className="text-lg font-medium text-gray-900 mt-1">{formData.phone}</p>
                            </div>
                          )}
                          {formData.website && (
                            <div className="bg-white p-4 rounded-xl">
                              <span className="text-sm font-medium text-gray-500 uppercase tracking-wide">{tString('review.labels.website')}</span>
                              <p className="text-lg font-medium text-gray-900 mt-1">{formData.website}</p>
                            </div>
                          )}
                        </div>
                        {formData.description && (
                          <div className="bg-white p-4 rounded-xl mt-6">
                            <span className="text-sm font-medium text-gray-500 uppercase tracking-wide">{tString('review.labels.description')}</span>
                            <p className="text-gray-900 mt-1">{formData.description}</p>
                          </div>
                        )}
                      </div>

                      {/* Payment Addresses Review */}
                      <div className="bg-gray-50 p-8 rounded-2xl border border-gray-200">
                        <h3 className="text-xl font-medium text-gray-900 mb-6 flex items-center gap-3">
                          <div className="w-8 h-8 bg-gray-100 rounded-lg flex items-center justify-center">
                            <Wallet size={18} className="text-gray-600" />
                          </div>
                          {tString('review.sections.location')}
                        </h3>
                        <div className="grid grid-cols-1 md:grid-cols-2 gap-6">
                          <div className="bg-white p-4 rounded-xl">
                            <span className="text-sm font-medium text-gray-500 uppercase tracking-wide">{tString('review.labels.settlementAddress')}</span>
                            <p className="text-sm font-mono text-gray-900 mt-1 break-all">{formData.settlement_address}</p>
                          </div>
                          <div className="bg-white p-4 rounded-xl">
                            <span className="text-sm font-medium text-gray-500 uppercase tracking-wide">{tString('review.labels.tippingAddress')}</span>
                            <p className="text-sm font-mono text-gray-900 mt-1 break-all">{formData.tipping_address}</p>
                          </div>
                        </div>
                      </div>

                      {/* Settings Review */}
                      <div className="bg-gray-50 p-8 rounded-2xl border border-gray-200">
                        <h3 className="text-xl font-medium text-gray-900 mb-6 flex items-center gap-3">
                          <div className="w-8 h-8 bg-gray-100 rounded-lg flex items-center justify-center">
                            <Settings size={18} className="text-gray-600" />
                          </div>
                          {tString('review.sections.settings')}
                        </h3>
                        <div className="grid grid-cols-1 md:grid-cols-2 gap-6">
                          <div className="bg-white p-4 rounded-xl">
                            <span className="text-sm font-medium text-gray-500 uppercase tracking-wide">{tString('review.labels.taxRate')}</span>
                            <p className="text-lg font-semibold text-gray-900 mt-1">{formData.tax_rate}%</p>
                          </div>
                          <div className="bg-white p-4 rounded-xl">
                            <span className="text-sm font-medium text-gray-500 uppercase tracking-wide">{tString('review.labels.serviceFeeRate')}</span>
                            <p className="text-lg font-semibold text-gray-900 mt-1">{formData.service_fee_rate}%</p>
                          </div>
                        </div>
                        <div className="bg-white p-4 rounded-xl mt-6">
                          <span className="text-sm font-medium text-gray-500 uppercase tracking-wide">{tString('review.labels.features')}</span>
                          <div className="flex flex-wrap gap-2 mt-2">
                            {formData.business_page_enabled && (
                              <span className="px-3 py-1 bg-green-100 text-green-800 text-sm font-medium rounded-full">{tString('review.labels.businessPage')}</span>
                            )}
                            {formData.show_reviews && (
                              <span className="px-3 py-1 bg-blue-100 text-blue-800 text-sm font-medium rounded-full">{tString('review.labels.reviews')}</span>
                            )}
                            {formData.counter_enabled && (
                              <span className="px-3 py-1 bg-purple-100 text-purple-800 text-sm font-medium rounded-full">{tString('review.labels.counterService')}</span>
                            )}
                          </div>
                        </div>
                      </div>

                      {/* Payment Summary */}
                      <div className="bg-gray-50 p-8 rounded-2xl border border-gray-200">
                        <h3 className="text-xl font-medium text-gray-900 mb-6 flex items-center gap-3">
                          <div className="w-8 h-8 bg-gray-100 rounded-lg flex items-center justify-center">
                            <DollarSign size={18} className="text-gray-600" />
                          </div>
                          {tString('review.sections.payment')}
                        </h3>
                        <div className="bg-white p-4 rounded-xl">
                          <div className="space-y-3">
                            <div className="flex justify-between items-center">
                              <span className="text-sm font-medium text-gray-500 uppercase tracking-wide">{tString('review.labels.subscriptionPayment')}</span>
                              <p className="text-lg font-semibold text-gray-900">
                                ${useCustomAmount ? customAmount || '0.00' : selectedOption.suggestedAmount}
                              </p>
                            </div>
                            {discountType === 'coupon' && isCouponValid && (
                              <div className="flex justify-between items-center">
                                <span className="text-sm font-medium text-green-600">{tString('review.labels.couponDiscount')}</span>
                                <span className="text-lg font-semibold text-green-600">
                                  -${formatUsdcAmount(couponDiscountAmount)}
                                </span>
                              </div>
                            )}
                            {discountType === 'referral' && referralCodeValid && (
                              <div className="flex justify-between items-center">
                                <span className="text-sm font-medium text-green-600">{tString('review.labels.referralDiscount')}</span>
                                <span className="text-lg font-semibold text-green-600">
                                  -$0.00
                                </span>
                              </div>
                            )}
                            <Divider />
                            <div className="flex justify-between items-center">
                              <span className="text-lg font-bold text-gray-900">{tString('review.labels.totalPayment')}</span>
                              <p className="text-2xl font-bold text-gray-900">
                                ${calculateFinalAmount()}
                              </p>
                            </div>
                            {calculatedSubscriptionTime ? (
                              <div className="mt-3 pt-3 border-t border-gray-200">
                                <p className="text-sm text-gray-600">
                                  {tString('review.labels.subscriptionTime').replace('{time}', formatSubscriptionTime(Number(calculatedSubscriptionTime.toString())))}
                                </p>
                              </div>
                            ) : null}
                          </div>
                          <div className="mt-3 pt-3 border-t border-gray-200">
                            <p className="text-xs text-gray-500">
                              {tString('review.labels.currentUsdcBalance').replace('{balance}', usdcBalance ? formatUsdcAmount(BigInt(usdcBalance.toString())) : '0')}
                            </p>
                          </div>
                        </div>
                      </div>

                      {/* Referral Review */}
                      {formData.referred_by_code && (
                        <div className="bg-gray-50 p-8 rounded-2xl border border-gray-200">
                          <h3 className="text-xl font-medium text-gray-900 mb-4 flex items-center gap-3">
                            <div className="w-8 h-8 bg-gray-100 rounded-lg flex items-center justify-center">
                              <Hash size={18} className="text-gray-600" />
                            </div>
                            {tString('settings.referral.title')}
                          </h3>
                          <div className="bg-white p-4 rounded-xl">
                            <p className="text-emerald-800 font-medium">
                              {tString('review.labels.referralCodeUsing')} <span className="font-bold">{formData.referred_by_code}</span>
                              {referralCodeValid === true && (
                                <span className="ml-2 px-2 py-1 bg-emerald-100 text-emerald-800 text-xs font-medium rounded-full">
                                  {tString('review.labels.validDiscountApplied')}
                                </span>
                              )}
                            </p>
                          </div>
                        </div>
                      )}
                    </div>
                  </div>
                )}
              </CardBody>
            </Card>

            {/* Navigation Buttons */}
            <div className="bg-white border border-gray-200 rounded-2xl p-4 sm:p-6 shadow-sm">
              <div className="flex flex-col sm:flex-row justify-between items-stretch sm:items-center gap-4">
                <Button
                  variant="bordered"
                  onPress={prevStep}
                  isDisabled={currentStepIndex === 0}
                  startContent={<ChevronLeft size={18} />}
                  className="h-12 px-6 border-2 font-medium"
                  size="lg"
                >
                  {tString('navigation.back')}
                </Button>

                <div className="flex flex-col sm:flex-row gap-3 sm:gap-4 w-full sm:w-auto">
                  <Button
                    variant="light"
                    onPress={() => router.back()}
                    className="h-12 px-6 font-medium text-gray-600 hover:text-gray-800"
                    size="lg"
                  >
                    {tString('navigation.cancel')}
                  </Button>

                  {currentStepIndex === steps.length - 1 ? (
                    <Button
                      onPress={handleSubmit}
                      color="primary"
                      isLoading={loading}
                      isDisabled={!isStepValid(currentStep)}
                      size="lg"
                      className="h-12 px-8 bg-gray-900 hover:bg-gray-800 font-medium shadow-sm"
                    >
                      {loading ? (
                        error && error.includes('Approving') ? tString('loading.approval') :
                        error && error.includes('blockchain') ? tString('loading.registration') :
                        error && error.includes('profile') ? tString('loading.backend') :
                        tString('loading.registration')
                      ) : (
`${tString('navigation.submit')} ($${calculateFinalAmount()} USDC)`
                      )}
                    </Button>
                  ) : (
                    <Button
                      onPress={nextStep}
                      color="primary"
                      isDisabled={!isStepValid(currentStep)}
                      endContent={<ChevronRight size={18} />}
                      className="h-12 px-6 bg-gray-900 hover:bg-gray-800 font-medium shadow-sm"
                      size="lg"
                    >
                      {tString('navigation.next')}
                    </Button>
                  )}
                </div>
              </div>
            </div>
          </div>
        </div>
      </div>
    </>
  );
}

