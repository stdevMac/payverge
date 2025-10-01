"use client";
import { Card, CardBody, Progress } from '@nextui-org/react';
import { Loader2, Shield, Coins, Building2, Check } from 'lucide-react';

interface SmartContractLoadingOverlayProps {
  isVisible: boolean;
  currentStep: 'approval' | 'registration' | 'backend' | 'upload' | 'complete';
  businessName?: string;
  error?: string | null;
}

export default function SmartContractLoadingOverlay({ 
  isVisible, 
  currentStep, 
  businessName = '',
  error 
}: SmartContractLoadingOverlayProps) {
  if (!isVisible) return null;

  const steps = [
    {
      key: 'approval',
      title: 'Approving USDC',
      description: 'Authorizing smart contract to collect registration fee',
      icon: Coins,
      progress: 25
    },
    {
      key: 'registration',
      title: 'Registering on Blockchain',
      description: 'Creating your business profile on the blockchain',
      icon: Shield,
      progress: 50
    },
    {
      key: 'backend',
      title: 'Creating Profile',
      description: 'Setting up your business dashboard and features',
      icon: Building2,
      progress: 75
    },
    {
      key: 'upload',
      title: 'Uploading Assets',
      description: 'Processing your business logo and assets',
      icon: Building2,
      progress: 90
    },
    {
      key: 'complete',
      title: 'Complete',
      description: 'Your business is ready!',
      icon: Check,
      progress: 100
    }
  ];

  const currentStepData = steps.find(step => step.key === currentStep) || steps[0];
  const currentStepIndex = steps.findIndex(step => step.key === currentStep);
  const CurrentIcon = currentStepData.icon;

  return (
    <div className="fixed inset-0 bg-black/50 backdrop-blur-sm z-50 flex items-center justify-center p-4">
      <Card className="w-full max-w-md shadow-2xl border-0 bg-white">
        <CardBody className="p-8">
          {/* Header */}
          <div className="text-center mb-8">
            <div className="w-16 h-16 bg-gray-50 border-2 border-gray-200 rounded-2xl flex items-center justify-center mx-auto mb-6 relative">
              {currentStep === 'complete' ? (
                <Check className="text-green-600" size={32} />
              ) : (
                <>
                  <CurrentIcon className="text-gray-700" size={32} />
                  <div className="absolute -inset-1">
                    <Loader2 className="w-full h-full text-gray-300 animate-spin" strokeWidth={1} />
                  </div>
                </>
              )}
            </div>
            
            <h2 className="text-2xl font-semibold text-gray-900 mb-2">
              {currentStepData.title}
            </h2>
            <p className="text-gray-600 font-light leading-relaxed">
              {currentStepData.description}
            </p>
          </div>

          {/* Progress */}
          <div className="mb-8">
            <div className="flex justify-between items-center mb-3">
              <span className="text-sm font-medium text-gray-700">
                Step {currentStepIndex + 1} of {steps.length}
              </span>
              <span className="text-sm text-gray-500">
                {currentStepData.progress}%
              </span>
            </div>
            <Progress 
              value={currentStepData.progress} 
              className="mb-4"
              classNames={{
                track: "bg-gray-100",
                indicator: "bg-gray-900"
              }}
              size="md"
            />
          </div>

          {/* Business Name */}
          {businessName && (
            <div className="bg-gray-50 p-4 rounded-xl mb-6">
              <p className="text-sm text-gray-600 mb-1">Registering Business</p>
              <p className="font-semibold text-gray-900">{businessName}</p>
            </div>
          )}

          {/* Error Message */}
          {error && (
            <div className="bg-red-50 border border-red-200 p-4 rounded-xl mb-6">
              <p className="text-red-700 text-sm font-medium">Transaction Failed</p>
              <p className="text-red-600 text-sm mt-1">{error}</p>
            </div>
          )}

          {/* Important Notice */}
          <div className="bg-blue-50 border border-blue-200 p-4 rounded-xl">
            <div className="flex items-start gap-3">
              <Shield className="text-blue-600 mt-0.5" size={16} />
              <div>
                <p className="text-blue-800 text-sm font-medium mb-1">
                  Please don&apos;t close this page
                </p>
                <p className="text-blue-700 text-xs leading-relaxed">
                  We&apos;re processing your blockchain transactions. This may take a few moments.
                </p>
              </div>
            </div>
          </div>

          {/* Step Indicators */}
          <div className="flex justify-center mt-6 gap-2">
            {steps.slice(0, -1).map((step, index) => (
              <div
                key={step.key}
                className={`w-2 h-2 rounded-full transition-colors duration-300 ${
                  index <= currentStepIndex 
                    ? 'bg-gray-900' 
                    : 'bg-gray-200'
                }`}
              />
            ))}
          </div>
        </CardBody>
      </Card>
    </div>
  );
}
