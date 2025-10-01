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
import { useRegisterBusiness, useReferrer, formatUsdcAmount, useRegistrationFee, useUsdcBalance, useUsdcAllowance, useApproveUsdc, parseUsdcAmount } from '@/contracts/hooks';
import { RegisterBusinessParams, Referrer } from '@/contracts/types';
import { Address } from 'viem';
import { checkReferralCodeAvailability } from '@/api/referrals';
import { uploadFileToS3, uploadProtectedFileToS3 } from '@/api/files/useUploadFileToS3/useUploadFileToS3';


// Multi-step form steps
type FormStep = 'business' | 'location' | 'settings' | 'review';

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
}

export default function BusinessRegisterPage() {
  const router = useRouter();
  const { address, isConnected } = useAccount();
  const { user } = useUserStore();
  const { isAuthenticated } = useAuth();
  const fileInputRef = useRef<HTMLInputElement>(null);
  
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
    { key: 'business', title: 'Business Info', description: 'Name, logo & contact details', icon: Building2 },
    { key: 'location', title: 'Location & Wallets', description: 'Address & payment setup', icon: MapPin },
    { key: 'settings', title: 'Settings & Referral', description: 'Fees, features & referral', icon: Settings },
    { key: 'review', title: 'Review & Submit', description: 'Confirm and create', icon: Check },
  ];

  const currentStepIndex = steps.findIndex(step => step.key === currentStep);
  const progress = ((currentStepIndex + 1) / steps.length) * 100;

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
              'Authorization': `Bearer ${getCookie('token')}`,
            },
            body: uploadFormData,
          });
          
          if (response.ok) {
            const uploadResult = await response.json();
            console.log('Logo uploaded successfully:', uploadResult.location);
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
        return referralCodeValid !== false; // null (empty) or true is ok
      case 'review':
        return isStepValid('business') && isStepValid('location');
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

      <div className="min-h-screen bg-gradient-to-b from-gray-50 to-white relative overflow-hidden">
        {/* Clean, minimal background */}
        <div className="absolute inset-0 overflow-hidden pointer-events-none">
          <div className="absolute -top-40 -right-40 w-80 h-80 bg-gray-100 rounded-full blur-3xl opacity-10"></div>
          <div className="absolute -bottom-40 -left-40 w-96 h-96 bg-gray-100 rounded-full blur-3xl opacity-8"></div>
        </div>
        
        <div className="max-w-5xl mx-auto px-6 py-12 relative">
          {/* Header */}
          <div className="mb-16">
            <Link href="/dashboard">
              <Button
                variant="light"
                startContent={<ArrowLeft size={16} />}
                className="text-gray-600 mb-8 hover:text-gray-800 transition-all duration-300 hover:bg-white hover:shadow-sm rounded-xl px-4 py-2"
              >
                Back to Dashboard
              </Button>
            </Link>
            <div className="text-center">
              <div className="inline-flex items-center px-4 py-2 bg-gray-50 border border-gray-200 rounded-full text-sm font-medium text-gray-700 mb-8 hover:bg-white hover:shadow-sm transition-all duration-300">
                <div className="w-2 h-2 bg-green-500 rounded-full mr-2 animate-pulse"></div>
                Business Registration
              </div>
              <h1 className="text-6xl lg:text-7xl font-medium text-gray-900 mb-6 leading-tight tracking-tight">
                Register Your
                <span className="text-gray-900 font-semibold block">
                  Business
                </span>
              </h1>
              <p className="text-xl font-light text-gray-600 leading-relaxed tracking-wide max-w-3xl mx-auto">
                Set up your business to start accepting crypto payments with Payverge&apos;s cutting-edge platform
              </p>
            </div>
          </div>

          {/* Progress Section */}
          <div className="mb-12">
            <div className="bg-white rounded-2xl shadow-sm border border-gray-200 p-8">
              <div className="flex justify-between items-center mb-6">
                <div>
                  <span className="text-sm font-semibold text-gray-700 uppercase tracking-wide">
                    Step {currentStepIndex + 1} of {steps.length}
                  </span>
                  <p className="text-lg font-medium text-gray-900 mt-1">
                    {steps[currentStepIndex]?.title}
                  </p>
                </div>
                <div className="text-right">
                  <span className="text-2xl font-bold text-gray-900">
                    {Math.round(progress)}%
                  </span>
                  <p className="text-sm text-gray-500">Complete</p>
                </div>
              </div>
              <Progress 
                value={progress} 
                className="mb-8"
                color="primary"
                size="md"
                classNames={{
                  track: "bg-gray-100",
                  indicator: "bg-gray-900"
                }}
              />
            
              {/* Step Navigation */}
              <div className="flex justify-center">
                <div className="flex items-center gap-6">
                  {steps.map((step, index) => {
                    const Icon = step.icon;
                    const isActive = step.key === currentStep;
                    const isCompleted = index < currentStepIndex;
                    const isAccessible = index <= currentStepIndex;
                    
                    return (
                      <div key={step.key} className="flex items-center">
                        <button
                          type="button"
                          onClick={() => isAccessible && goToStep(step.key)}
                          disabled={!isAccessible}
                          className={`group flex flex-col items-center p-4 rounded-2xl transition-all duration-300 hover:scale-105 ${
                            isActive 
                              ? 'bg-gray-50 border-2 border-gray-300 shadow-lg' 
                              : isCompleted
                              ? 'bg-green-50 border-2 border-green-200 hover:shadow-md'
                              : isAccessible
                              ? 'bg-white border-2 border-gray-200 hover:border-gray-300 hover:shadow-md'
                              : 'bg-gray-50 border-2 border-gray-100 cursor-not-allowed opacity-50'
                          }`}
                        >
                          <div className={`w-14 h-14 rounded-2xl flex items-center justify-center mb-3 transition-all duration-300 ${
                            isActive 
                              ? 'bg-gray-900 text-white shadow-lg' 
                              : isCompleted
                              ? 'bg-green-500 text-white'
                              : isAccessible
                              ? 'bg-gray-100 text-gray-600 group-hover:bg-gray-200'
                              : 'bg-gray-100 text-gray-400'
                          }`}>
                            {isCompleted ? (
                              <Check size={24} />
                            ) : (
                              <Icon size={24} />
                            )}
                          </div>
                          <span className={`text-sm font-semibold text-center mb-1 transition-colors ${
                            isActive ? 'text-gray-900' : isCompleted ? 'text-green-700' : 'text-gray-700'
                          }`}>
                            {step.title}
                          </span>
                          <span className="text-xs text-gray-500 text-center max-w-20 leading-tight">
                            {step.description}
                          </span>
                        </button>
                        
                        {/* Enhanced Connector Line */}
                        {index < steps.length - 1 && (
                          <div className={`w-12 h-1 mx-3 rounded-full transition-all duration-500 ${
                            isCompleted ? 'bg-green-400' : 'bg-gray-200'
                          }`} />
                        )}
                      </div>
                    );
                  })}
              </div>
            </div>
            </div>
          </div>

          {/* Error Message */}
          {error && (
            <Card className="mb-6 border-danger-200 bg-danger-50">
              <CardBody>
                <p className="text-danger-700 flex items-center gap-2">
                  <span className="w-2 h-2 bg-danger-500 rounded-full"></span>
                  {error}
                </p>
              </CardBody>
            </Card>
          )}

          {/* Multi-Step Form */}
          <div className="space-y-8">
            {/* Step Content */}
            <Card className="shadow-xl border-0 bg-white/80 backdrop-blur-sm">
              <CardBody className="p-12">
                {/* Business Information Step */}
                {currentStep === 'business' && (
                  <div className="space-y-8">
                    <div className="text-center mb-12">
                      <div className="w-20 h-20 bg-gray-50 border-2 border-gray-200 rounded-3xl flex items-center justify-center mx-auto mb-8 shadow-lg">
                        <Building2 className="text-gray-700" size={40} />
                      </div>
                      <h2 className="text-4xl font-medium text-gray-900 mb-4 tracking-tight">Business Information</h2>
                      <p className="text-xl font-light text-gray-600 leading-relaxed tracking-wide max-w-2xl mx-auto">Let&apos;s get your business set up on Payverge&apos;s cutting-edge platform</p>
                    </div>

                    <div className="max-w-3xl mx-auto space-y-10">
                      {/* Business Name */}
                      <div>
                        <div className="group">
                          <Input
                            label="Business Name"
                            placeholder="Enter your business name"
                            value={formData.name}
                            onChange={(e) => updateFormData('name', e.target.value)}
                            isRequired
                            variant="bordered"
                            size="lg"
                            startContent={<Building2 size={22} className="text-gray-400" />}
                            classNames={{
                              input: "text-lg font-medium",
                              inputWrapper: "h-16 border-2 group-hover:border-gray-300 transition-colors duration-300 shadow-sm hover:shadow-md",
                              label: "text-base font-semibold text-gray-700"
                            }}
                          />
                        </div>
                      </div>

                      {/* Logo Upload */}
                      <div className="space-y-6">
                        <div>
                          <label className="text-base font-semibold text-gray-700 mb-2 block">Business Logo</label>
                          <p className="text-sm text-gray-500">Upload your business logo to build brand recognition</p>
                        </div>
                        <div className="bg-gray-50 p-8 rounded-2xl border-2 border-dashed border-gray-200 hover:border-gray-300 transition-all duration-300">
                          <div className="flex flex-col md:flex-row items-center gap-8">
                            {/* Logo Preview */}
                            <div className="relative group">
                              <div className="w-32 h-32 border-2 border-gray-200 rounded-2xl flex items-center justify-center bg-white shadow-sm group-hover:shadow-md transition-all duration-300">
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
                                className="h-14 px-8 text-base font-semibold bg-gray-900 hover:bg-gray-800 shadow-lg hover:shadow-xl transition-all duration-300"
                                size="lg"
                              >
                                {uploadingLogo ? 'Processing...' : logoPreview ? 'Change Logo' : 'Upload Logo'}
                              </Button>
                              <div className="mt-4 space-y-1">
                                <p className="text-sm font-medium text-gray-600">
                                  JPEG, PNG, or WebP • Max 5MB
                                </p>
                                <p className="text-xs text-gray-500">
                                  Recommended: Square format, 400x400px minimum
                                </p>
                              </div>
                            </div>
                          </div>
                        </div>
                      </div>

                      {/* Contact Information */}
                      <div className="grid grid-cols-1 md:grid-cols-2 gap-6">
                        <Input
                          label="Business Email"
                          placeholder="contact@yourbusiness.com"
                          type="email"
                          value={formData.email || ''}
                          onChange={(e) => updateFormData('email', e.target.value)}
                          variant="bordered"
                          size="lg"
                          startContent={<span className="text-gray-400 text-lg">@</span>}
                          isInvalid={formData.email ? !isValidEmail(formData.email) : false}
                          errorMessage={formData.email && !isValidEmail(formData.email) ? "Please enter a valid email" : ""}
                          classNames={{
                            inputWrapper: "h-14 border-2",
                            label: "font-medium"
                          }}
                        />

                        <Input
                          label="Phone Number"
                          placeholder="+1 (555) 123-4567"
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
                          label="Website"
                          placeholder="https://yourbusiness.com"
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
                          label="Custom URL Slug"
                          placeholder="your-business-name"
                          value={formData.custom_url || ''}
                          onChange={(e) => updateFormData('custom_url', e.target.value.toLowerCase().replace(/[^a-z0-9-]/g, ''))}
                          variant="bordered"
                          size="lg"
                          startContent={<span className="text-gray-400 text-sm">payverge.com/</span>}
                          description="This will be your public business page URL"
                          classNames={{
                            inputWrapper: "h-14 border-2",
                            label: "font-medium"
                          }}
                        />
                      </div>

                      <Textarea
                        label="Business Description"
                        placeholder="Describe your business, cuisine type, specialties..."
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
                      <h2 className="text-3xl font-medium text-gray-900 mb-3 tracking-tight">Location & Payment Setup</h2>
                      <p className="text-lg font-light text-gray-600 leading-relaxed tracking-wide">Configure your business location and crypto wallets</p>
                    </div>

                    <div className="max-w-3xl mx-auto space-y-10">
                      {/* Physical Address */}
                      <div className="bg-gray-50 p-8 rounded-2xl border border-gray-200">
                        <h3 className="text-xl font-semibold text-gray-900 mb-6 flex items-center gap-3">
                          <div className="w-8 h-8 bg-gray-600 rounded-lg flex items-center justify-center">
                            <MapPin size={18} className="text-white" />
                          </div>
                          Business Address
                        </h3>
                        <div className="grid grid-cols-1 md:grid-cols-2 gap-6">
                          <div className="md:col-span-2">
                            <Input
                              label="Street Address"
                              placeholder="123 Main Street"
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
                            label="City"
                            placeholder="New York"
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
                            label="State/Province"
                            placeholder="NY"
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
                            label="Postal Code"
                            placeholder="10001"
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
                            label="Country"
                            placeholder="United States"
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
                        <h3 className="text-xl font-semibold text-gray-900 mb-6 flex items-center gap-3">
                          <div className="w-8 h-8 bg-gray-600 rounded-lg flex items-center justify-center">
                            <Wallet size={18} className="text-white" />
                          </div>
                          Payment Wallets
                        </h3>
                        <div className="space-y-6">
                          <Input
                            label="Settlement Address"
                            placeholder="0x..."
                            value={formData.settlement_address}
                            onChange={(e) => updateFormData('settlement_address', e.target.value)}
                            variant="bordered"
                            size="lg"
                            isRequired
                            description="Where customer payments will be sent"
                            startContent={<Wallet size={20} className="text-gray-400" />}
                            classNames={{
                              inputWrapper: "h-14 border-2",
                              label: "font-medium"
                            }}
                          />
                          <Input
                            label="Tipping Address"
                            placeholder="0x..."
                            value={formData.tipping_address}
                            onChange={(e) => updateFormData('tipping_address', e.target.value)}
                            variant="bordered"
                            size="lg"
                            isRequired
                            description="Where customer tips will be sent"
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
                                  <p className="font-medium text-gray-900">Connected Wallet</p>
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
                                  Use This Address
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
                      <h2 className="text-3xl font-medium text-gray-900 mb-3 tracking-tight">Business Settings</h2>
                      <p className="text-lg font-light text-gray-600 leading-relaxed tracking-wide">Configure fees, features, and referral options</p>
                    </div>

                    <div className="max-w-3xl mx-auto space-y-10">
                      {/* Fee Settings */}
                      <div className="bg-gray-50 p-8 rounded-2xl border border-gray-200">
                        <h3 className="text-xl font-semibold text-gray-900 mb-6 flex items-center gap-3">
                          <div className="w-8 h-8 bg-gray-600 rounded-lg flex items-center justify-center">
                            <Percent size={18} className="text-white" />
                          </div>
                          Fee Configuration
                        </h3>
                        <div className="grid grid-cols-1 md:grid-cols-2 gap-6">
                          <Input
                            label="Tax Rate (%)"
                            placeholder="8.25"
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
                            label="Service Fee Rate (%)"
                            placeholder="3.00"
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
                            Tax Inclusive Pricing
                          </Checkbox>
                          <Checkbox
                            isSelected={formData.service_inclusive}
                            onValueChange={(checked) => updateFormData('service_inclusive', checked)}
                            classNames={{ label: "font-medium" }}
                          >
                            Service Fee Inclusive
                          </Checkbox>
                        </div>
                      </div>

                      {/* Business Features */}
                      <div className="bg-gray-50 p-8 rounded-2xl border border-gray-200">
                        <h3 className="text-xl font-semibold text-gray-900 mb-6 flex items-center gap-3">
                          <div className="w-8 h-8 bg-gray-600 rounded-lg flex items-center justify-center">
                            <Settings size={18} className="text-white" />
                          </div>
                          Business Features
                        </h3>
                        <div className="space-y-6">
                          <div className="flex items-center justify-between p-4 bg-white rounded-xl border border-gray-200">
                            <div>
                              <h4 className="font-semibold text-gray-900">Business Page</h4>
                              <p className="text-sm text-gray-600">Create a public page for your business</p>
                            </div>
                            <Switch
                              isSelected={formData.business_page_enabled}
                              onValueChange={(checked) => updateFormData('business_page_enabled', checked)}
                              color="success"
                            />
                          </div>
                          <div className="flex items-center justify-between p-4 bg-white rounded-xl border border-gray-200">
                            <div>
                              <h4 className="font-semibold text-gray-900">Show Reviews</h4>
                              <p className="text-sm text-gray-600">Display customer reviews on your page</p>
                            </div>
                            <Switch
                              isSelected={formData.show_reviews}
                              onValueChange={(checked) => updateFormData('show_reviews', checked)}
                              color="success"
                            />
                          </div>
                          <div className="flex items-center justify-between p-4 bg-white rounded-xl border border-gray-200">
                            <div>
                              <h4 className="font-semibold text-gray-900">Counter Service</h4>
                              <p className="text-sm text-gray-600">Enable takeaway/quick service counters</p>
                            </div>
                            <Switch
                              isSelected={formData.counter_enabled}
                              onValueChange={(checked) => updateFormData('counter_enabled', checked)}
                              color="success"
                            />
                          </div>
                        </div>
                      </div>

                      {/* Referral Code */}
                      <div className="bg-gray-50 p-8 rounded-2xl border border-gray-200">
                        <h3 className="text-xl font-semibold text-gray-900 mb-6 flex items-center gap-3">
                          <div className="w-8 h-8 bg-gray-600 rounded-lg flex items-center justify-center">
                            <Hash size={18} className="text-white" />
                          </div>
                          Referral Code (Optional)
                        </h3>
                        <div className="max-w-md">
                          <Input
                            label="Referral Code"
                            placeholder="Enter referral code"
                            value={formData.referred_by_code || ''}
                            onChange={(e) => {
                              const code = e.target.value.toUpperCase();
                              updateFormData('referred_by_code', code);
                              if (code.length >= 3) {
                                validateReferralCode(code);
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
                                ? "✅ Valid referral code! You'll get a discount on registration."
                                : referralCodeValid === false 
                                ? "❌ Invalid referral code. Please check and try again."
                                : "Enter a referral code to get a discount (optional)"
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
                          
                          {referrerData && (referrerData as Referrer).isActive ? (
                            <div className="mt-6 p-4 bg-green-50 border border-gray-200 rounded-xl">
                              <p className="text-sm text-green-800">
                                <strong>Great news!</strong> You&apos;re already a referrer on Payverge. 
                                You can still use someone else&apos;s referral code to get a discount on your business registration.
                              </p>
                            </div>
                          ) : null}
                        </div>
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
                      <h2 className="text-3xl font-medium text-gray-900 mb-3 tracking-tight">Review & Submit</h2>
                      <p className="text-lg font-light text-gray-600 leading-relaxed tracking-wide">Please review your information before creating your business</p>
                    </div>

                    <div className="max-w-4xl mx-auto space-y-8">
                      {/* Business Information Review */}
                      <div className="bg-gray-50 p-8 rounded-2xl border border-gray-200">
                        <h3 className="text-xl font-semibold text-gray-900 mb-6 flex items-center gap-3">
                          <div className="w-8 h-8 bg-gray-600 rounded-lg flex items-center justify-center">
                            <Building2 size={18} className="text-white" />
                          </div>
                          Business Information
                        </h3>
                        <div className="grid grid-cols-1 md:grid-cols-2 gap-6">
                          <div className="bg-white p-4 rounded-xl">
                            <span className="text-sm font-medium text-gray-500 uppercase tracking-wide">Business Name</span>
                            <p className="text-lg font-semibold text-gray-900 mt-1">{formData.name}</p>
                          </div>
                          {formData.email && (
                            <div className="bg-white p-4 rounded-xl">
                              <span className="text-sm font-medium text-gray-500 uppercase tracking-wide">Email</span>
                              <p className="text-lg font-semibold text-gray-900 mt-1">{formData.email}</p>
                            </div>
                          )}
                          {formData.phone && (
                            <div className="bg-white p-4 rounded-xl">
                              <span className="text-sm font-medium text-gray-500 uppercase tracking-wide">Phone</span>
                              <p className="text-lg font-semibold text-gray-900 mt-1">{formData.phone}</p>
                            </div>
                          )}
                          {formData.website && (
                            <div className="bg-white p-4 rounded-xl">
                              <span className="text-sm font-medium text-gray-500 uppercase tracking-wide">Website</span>
                              <p className="text-lg font-semibold text-gray-900 mt-1">{formData.website}</p>
                            </div>
                          )}
                        </div>
                        {formData.description && (
                          <div className="bg-white p-4 rounded-xl mt-6">
                            <span className="text-sm font-medium text-gray-500 uppercase tracking-wide">Description</span>
                            <p className="text-gray-900 mt-1">{formData.description}</p>
                          </div>
                        )}
                      </div>

                      {/* Payment Addresses Review */}
                      <div className="bg-gray-50 p-8 rounded-2xl border border-gray-200">
                        <h3 className="text-xl font-semibold text-gray-900 mb-6 flex items-center gap-3">
                          <div className="w-8 h-8 bg-gray-600 rounded-lg flex items-center justify-center">
                            <Wallet size={18} className="text-white" />
                          </div>
                          Payment Addresses
                        </h3>
                        <div className="grid grid-cols-1 md:grid-cols-2 gap-6">
                          <div className="bg-white p-4 rounded-xl">
                            <span className="text-sm font-medium text-gray-500 uppercase tracking-wide">Settlement Address</span>
                            <p className="text-sm font-mono text-gray-900 mt-1 break-all">{formData.settlement_address}</p>
                          </div>
                          <div className="bg-white p-4 rounded-xl">
                            <span className="text-sm font-medium text-gray-500 uppercase tracking-wide">Tipping Address</span>
                            <p className="text-sm font-mono text-gray-900 mt-1 break-all">{formData.tipping_address}</p>
                          </div>
                        </div>
                      </div>

                      {/* Settings Review */}
                      <div className="bg-gray-50 p-8 rounded-2xl border border-gray-200">
                        <h3 className="text-xl font-semibold text-gray-900 mb-6 flex items-center gap-3">
                          <div className="w-8 h-8 bg-gray-600 rounded-lg flex items-center justify-center">
                            <Settings size={18} className="text-white" />
                          </div>
                          Business Settings
                        </h3>
                        <div className="grid grid-cols-1 md:grid-cols-2 gap-6">
                          <div className="bg-white p-4 rounded-xl">
                            <span className="text-sm font-medium text-gray-500 uppercase tracking-wide">Tax Rate</span>
                            <p className="text-lg font-semibold text-gray-900 mt-1">{formData.tax_rate}%</p>
                          </div>
                          <div className="bg-white p-4 rounded-xl">
                            <span className="text-sm font-medium text-gray-500 uppercase tracking-wide">Service Fee Rate</span>
                            <p className="text-lg font-semibold text-gray-900 mt-1">{formData.service_fee_rate}%</p>
                          </div>
                        </div>
                        <div className="bg-white p-4 rounded-xl mt-6">
                          <span className="text-sm font-medium text-gray-500 uppercase tracking-wide">Features</span>
                          <div className="flex flex-wrap gap-2 mt-2">
                            {formData.business_page_enabled && (
                              <span className="px-3 py-1 bg-green-100 text-green-800 text-sm font-medium rounded-full">Business Page</span>
                            )}
                            {formData.show_reviews && (
                              <span className="px-3 py-1 bg-blue-100 text-blue-800 text-sm font-medium rounded-full">Reviews</span>
                            )}
                            {formData.counter_enabled && (
                              <span className="px-3 py-1 bg-purple-100 text-purple-800 text-sm font-medium rounded-full">Counter Service</span>
                            )}
                          </div>
                        </div>
                      </div>

                      {/* Registration Fee */}
                      <div className="bg-gray-50 p-8 rounded-2xl border border-gray-200">
                        <h3 className="text-xl font-semibold text-gray-900 mb-6 flex items-center gap-3">
                          <div className="w-8 h-8 bg-gray-600 rounded-lg flex items-center justify-center">
                            <DollarSign size={18} className="text-white" />
                          </div>
                          Registration Fee
                        </h3>
                        <div className="bg-white p-4 rounded-xl">
                          <div className="flex justify-between items-center">
                            <span className="text-sm font-medium text-gray-500 uppercase tracking-wide">Platform Registration Fee</span>
                            <p className="text-lg font-semibold text-gray-900">
                              {registrationFee ? `${formatUsdcAmount(BigInt(registrationFee.toString()))} USDC` : 'Loading...'}
                            </p>
                          </div>
                          {formData.referred_by_code && referralCodeValid && (
                            <div className="mt-3 pt-3 border-t border-gray-200">
                              <div className="flex justify-between items-center text-sm">
                                <span className="text-emerald-600 font-medium">Referral Discount Applied</span>
                                <span className="text-emerald-600 font-medium">-10% or -15%</span>
                              </div>
                            </div>
                          )}
                          <div className="mt-3 pt-3 border-t border-gray-200">
                            <p className="text-xs text-gray-500">
                              Current USDC Balance: {usdcBalance ? formatUsdcAmount(BigInt(usdcBalance.toString())) : '0'} USDC
                            </p>
                          </div>
                        </div>
                      </div>

                      {/* Referral Review */}
                      {formData.referred_by_code && (
                        <div className="bg-gray-50 p-8 rounded-2xl border border-gray-200">
                          <h3 className="text-xl font-semibold text-gray-900 mb-4 flex items-center gap-3">
                            <div className="w-8 h-8 bg-gray-600 rounded-lg flex items-center justify-center">
                              <Hash size={18} className="text-white" />
                            </div>
                            Referral Code
                          </h3>
                          <div className="bg-white p-4 rounded-xl">
                            <p className="text-emerald-800 font-medium">
                              Using referral code: <span className="font-bold">{formData.referred_by_code}</span>
                              {referralCodeValid === true && (
                                <span className="ml-2 px-2 py-1 bg-emerald-100 text-emerald-800 text-xs font-medium rounded-full">
                                  ✅ Valid - Discount Applied
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
            <div className="bg-white border border-gray-200 rounded-2xl p-6 shadow-sm">
              <div className="flex justify-between items-center">
                <Button
                  variant="bordered"
                  onPress={prevStep}
                  isDisabled={currentStepIndex === 0}
                  startContent={<ChevronLeft size={18} />}
                  className="h-12 px-6 border-2 font-medium"
                  size="lg"
                >
                  Previous
                </Button>

                <div className="flex gap-4">
                  <Button
                    variant="light"
                    onPress={() => router.back()}
                    className="h-12 px-6 font-medium text-gray-600 hover:text-gray-800"
                    size="lg"
                  >
                    Cancel
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
                        error && error.includes('Approving') ? 'Approving USDC...' :
                        error && error.includes('blockchain') ? 'Registering on Blockchain...' :
                        error && error.includes('profile') ? 'Creating Profile...' :
                        'Creating Business...'
                      ) : (
                        registrationFee ? `Create Business (${formatUsdcAmount(BigInt(registrationFee.toString()))} USDC)` : 'Create Business'
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
                      Next Step
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
