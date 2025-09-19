'use client';

import React, { useState } from 'react';
import {
  Modal,
  ModalContent,
  ModalHeader,
  ModalBody,
  ModalFooter,
  Button,
  Card,
  CardBody,
  Progress,
  Divider
} from '@nextui-org/react';
import {
  IoWalletOutline,
  IoCodeSlashOutline,
  IoStatsChartOutline,
  IoShieldCheckmarkOutline,
  IoRocketOutline,
  IoImageOutline,
  IoDocumentTextOutline,
  IoBarChartOutline,
  IoRestaurantOutline,
  IoQrCodeOutline,
  IoCardOutline,
  IoCheckmarkCircleOutline
} from 'react-icons/io5';
import { useAppKit } from '@reown/appkit/react';
import { useAccount } from 'wagmi';
import { useAppKitAccount } from '@reown/appkit/react';

interface BusinessTutorialModalProps {
  isOpen: boolean;
  onClose: () => void;
  onComplete: (walletAddress: string) => void;
}

export default function BusinessTutorialModal({ isOpen, onClose, onComplete }: BusinessTutorialModalProps) {
  const [currentStep, setCurrentStep] = useState(0);
  const { open } = useAppKit();
  const { address, isConnected } = useAccount();
  const { isConnected: appKitConnected } = useAppKitAccount();
  const [isConnecting, setIsConnecting] = useState(false);
  const [isAuthenticating, setIsAuthenticating] = useState(false);

  // Reset connecting state when wallet connection changes
  React.useEffect(() => {
    if (isConnected && appKitConnected) {
      setIsConnecting(false);
      setIsAuthenticating(false);
    }
  }, [isConnected, appKitConnected]);

  const tutorialSteps = [
    {
      title: "Welcome to Payverge",
      description: "Transform your hospitality business with crypto payments",
      content: (
        <div className="text-center space-y-4">
          <IoRestaurantOutline className="w-16 h-16 text-primary mx-auto" />
          <p className="text-gray-600">
            Payverge is the modern payment solution for restaurants, cafes, bars, and hospitality businesses. 
            Accept USDC payments instantly with complete transparency.
          </p>
          <div className="bg-primary-50 p-4 rounded-lg">
            <p className="text-primary-700 font-medium">
              Ready to modernize your payment system? Let&apos;s get started!
            </p>
          </div>
        </div>
      ),
      icon: <IoRestaurantOutline className="w-8 h-8 text-primary" />
    },
    {
      title: "Digital Menu & QR Codes",
      description: "Create beautiful digital menus accessible via QR codes",
      content: (
        <div className="space-y-4">
          <IoQrCodeOutline className="w-16 h-16 text-primary mx-auto" />
          <div className="space-y-3">
            <div className="flex items-start gap-3">
              <IoCheckmarkCircleOutline className="w-5 h-5 text-green-500 mt-1 flex-shrink-0" />
              <div>
                <h4 className="font-medium">QR Code Tables</h4>
                <p className="text-gray-600 text-sm">Each table gets a unique QR code for instant menu access</p>
              </div>
            </div>
            <div className="flex items-start gap-3">
              <IoCheckmarkCircleOutline className="w-5 h-5 text-green-500 mt-1 flex-shrink-0" />
              <div>
                <h4 className="font-medium">Real-time Updates</h4>
                <p className="text-gray-600 text-sm">Update menu items, prices, and availability instantly</p>
              </div>
            </div>
            <div className="flex items-start gap-3">
              <IoCheckmarkCircleOutline className="w-5 h-5 text-green-500 mt-1 flex-shrink-0" />
              <div>
                <h4 className="font-medium">Mobile Optimized</h4>
                <p className="text-gray-600 text-sm">Beautiful, responsive design that works on any device</p>
              </div>
            </div>
          </div>
        </div>
      ),
      icon: <IoQrCodeOutline className="w-8 h-8 text-primary" />
    },
    {
      title: "USDC Payments",
      description: "Accept secure cryptocurrency payments instantly",
      content: (
        <div className="space-y-4">
          <IoCardOutline className="w-16 h-16 text-primary mx-auto" />
          <div className="space-y-3">
            <div className="flex items-start gap-3">
              <IoCheckmarkCircleOutline className="w-5 h-5 text-green-500 mt-1 flex-shrink-0" />
              <div>
                <h4 className="font-medium">Instant Settlement</h4>
                <p className="text-gray-600 text-sm">Receive USDC payments immediately in your wallet</p>
              </div>
            </div>
            <div className="flex items-start gap-3">
              <IoCheckmarkCircleOutline className="w-5 h-5 text-green-500 mt-1 flex-shrink-0" />
              <div>
                <h4 className="font-medium">Lower Fees</h4>
                <p className="text-gray-600 text-sm">Just 2% platform fee - no hidden charges</p>
              </div>
            </div>
            <div className="flex items-start gap-3">
              <IoCheckmarkCircleOutline className="w-5 h-5 text-green-500 mt-1 flex-shrink-0" />
              <div>
                <h4 className="font-medium">Global Access</h4>
                <p className="text-gray-600 text-sm">Accept payments from anyone with a crypto wallet worldwide</p>
              </div>
            </div>
          </div>
        </div>
      ),
      icon: <IoCardOutline className="w-8 h-8 text-primary" />
    },
    {
      title: "Smart Tipping",
      description: "Crypto tipping directly to your designated wallet",
      content: (
        <div className="space-y-4">
          <IoWalletOutline className="w-16 h-16 text-primary mx-auto" />
          <div className="space-y-3">
            <div className="flex items-start gap-3">
              <IoCheckmarkCircleOutline className="w-5 h-5 text-green-500 mt-1 flex-shrink-0" />
              <div>
                <h4 className="font-medium">Direct to Staff</h4>
                <p className="text-gray-600 text-sm">Tips go directly to your designated tipping wallet</p>
              </div>
            </div>
            <div className="flex items-start gap-3">
              <IoCheckmarkCircleOutline className="w-5 h-5 text-green-500 mt-1 flex-shrink-0" />
              <div>
                <h4 className="font-medium">Flexible Options</h4>
                <p className="text-gray-600 text-sm">Customers can add custom tip amounts or choose presets</p>
              </div>
            </div>
            <div className="flex items-start gap-3">
              <IoCheckmarkCircleOutline className="w-5 h-5 text-green-500 mt-1 flex-shrink-0" />
              <div>
                <h4 className="font-medium">Transparent Tracking</h4>
                <p className="text-gray-600 text-sm">All tips are recorded on-chain for complete transparency</p>
              </div>
            </div>
          </div>
        </div>
      ),
      icon: <IoWalletOutline className="w-8 h-8 text-primary" />
    },
    {
      title: "Analytics & Insights",
      description: "Comprehensive business intelligence and reporting",
      content: (
        <div className="space-y-4">
          <IoBarChartOutline className="w-16 h-16 text-primary mx-auto" />
          <div className="space-y-3">
            <div className="flex items-start gap-3">
              <IoCheckmarkCircleOutline className="w-5 h-5 text-green-500 mt-1 flex-shrink-0" />
              <div>
                <h4 className="font-medium">Real-time Dashboard</h4>
                <p className="text-gray-600 text-sm">Monitor sales, tips, and performance in real-time</p>
              </div>
            </div>
            <div className="flex items-start gap-3">
              <IoCheckmarkCircleOutline className="w-5 h-5 text-green-500 mt-1 flex-shrink-0" />
              <div>
                <h4 className="font-medium">Item Analytics</h4>
                <p className="text-gray-600 text-sm">Track your best-selling items and optimize your menu</p>
              </div>
            </div>
            <div className="flex items-start gap-3">
              <IoCheckmarkCircleOutline className="w-5 h-5 text-green-500 mt-1 flex-shrink-0" />
              <div>
                <h4 className="font-medium">Export Data</h4>
                <p className="text-gray-600 text-sm">Export reports in CSV or JSON format for accounting</p>
              </div>
            </div>
          </div>
        </div>
      ),
      icon: <IoBarChartOutline className="w-8 h-8 text-primary" />
    },
    {
      title: "Connect Your Wallet",
      description: "Connect your wallet to receive payments and manage your business",
      content: (
        <div className="space-y-6">
          <IoShieldCheckmarkOutline className="w-16 h-16 text-primary mx-auto" />
          <div className="text-center space-y-4">
            <p className="text-gray-600">
              To get started, you&apos;ll need to connect your crypto wallet. This wallet will be used to:
            </p>
            <div className="space-y-2 text-left max-w-md mx-auto">
              <div className="flex items-center gap-2">
                <IoCheckmarkCircleOutline className="w-4 h-4 text-green-500" />
                <span className="text-sm">Receive payment settlements</span>
              </div>
              <div className="flex items-center gap-2">
                <IoCheckmarkCircleOutline className="w-4 h-4 text-green-500" />
                <span className="text-sm">Manage your business settings</span>
              </div>
              <div className="flex items-center gap-2">
                <IoCheckmarkCircleOutline className="w-4 h-4 text-green-500" />
                <span className="text-sm">Access analytics and reports</span>
              </div>
            </div>
            {isConnected && appKitConnected && address ? (
              <div className="bg-green-50 p-4 rounded-lg">
                <IoCheckmarkCircleOutline className="w-6 h-6 text-green-500 mx-auto mb-2" />
                <p className="text-green-700 font-medium">Wallet Connected & Authenticated!</p>
                <p className="text-green-600 text-sm mt-1">
                  {address.slice(0, 6)}...{address.slice(-4)}
                </p>
                <p className="text-green-500 text-xs mt-1">
                  ✓ SIWE Authentication Complete
                </p>
              </div>
            ) : (
              <div className="space-y-3">
                <Button
                  color="primary"
                  size="lg"
                  onClick={() => {
                    setIsConnecting(true);
                    open();
                  }}
                  isLoading={isConnecting || isAuthenticating}
                  startContent={<IoWalletOutline className="w-5 h-5" />}
                  className="w-full max-w-xs mx-auto"
                >
                  {isConnecting ? 'Connecting...' : 
                   isAuthenticating ? 'Authenticating...' : 
                   'Connect & Sign'}
                </Button>
                {isConnected && !appKitConnected && (
                  <div className="bg-yellow-50 p-3 rounded-lg">
                    <p className="text-yellow-700 text-sm text-center">
                      Please sign the message to authenticate with Payverge
                    </p>
                  </div>
                )}
              </div>
            )}
          </div>
        </div>
      ),
      icon: <IoShieldCheckmarkOutline className="w-8 h-8 text-primary" />
    }
  ];

  const handleNext = () => {
    if (currentStep < tutorialSteps.length - 1) {
      setCurrentStep(currentStep + 1);
    }
  };

  const handleSkipTutorial = () => {
    // Skip to the last step (wallet connection)
    setCurrentStep(tutorialSteps.length - 1);
  };

  const handlePrevious = () => {
    if (currentStep > 0) {
      setCurrentStep(currentStep - 1);
    }
  };

  const handleComplete = () => {
    if (isConnected && appKitConnected && address) {
      onComplete(address);
    }
  };

  const progress = ((currentStep + 1) / tutorialSteps.length) * 100;
  const currentStepData = tutorialSteps[currentStep];
  const isLastStep = currentStep === tutorialSteps.length - 1;

  return (
    <Modal 
      isOpen={isOpen} 
      onClose={onClose}
      size="2xl"
      isDismissable={false}
      hideCloseButton
      classNames={{
        backdrop: "bg-zinc-900/50 backdrop-blur-sm"
      }}
    >
      <ModalContent>
        <ModalHeader className="flex flex-col gap-2 pb-2">
          <div className="flex items-center justify-between w-full">
            <div className="flex items-center gap-3">
              {currentStepData.icon}
              <div>
                <h2 className="text-xl font-bold">{currentStepData.title}</h2>
                <p className="text-sm text-gray-500 font-normal">{currentStepData.description}</p>
              </div>
            </div>
            <div className="text-sm text-gray-500">
              {currentStep + 1} of {tutorialSteps.length}
            </div>
          </div>
          <Progress 
            value={progress} 
            color="primary" 
            className="w-full"
            size="sm"
          />
        </ModalHeader>
        
        <ModalBody className="py-6">
          <Card className="border-none shadow-none">
            <CardBody className="p-6">
              {currentStepData.content}
            </CardBody>
          </Card>
        </ModalBody>
        
        <ModalFooter className="pt-2">
          <div className="flex justify-between w-full">
            <Button
              variant="light"
              onPress={handlePrevious}
              isDisabled={currentStep === 0}
            >
              Previous
            </Button>
            
            <div className="flex gap-2">
              <Button
                variant="light"
                onPress={handleSkipTutorial}
              >
                Skip Tutorial
              </Button>
              
              {isLastStep ? (
                <Button
                  color="primary"
                  onPress={handleComplete}
                  isDisabled={!isConnected || !appKitConnected || !address}
                  startContent={<IoRocketOutline className="w-4 h-4" />}
                >
                  Start Building
                </Button>
              ) : (
                <Button
                  color="primary"
                  onPress={handleNext}
                >
                  Next
                </Button>
              )}
            </div>
          </div>
        </ModalFooter>
      </ModalContent>
    </Modal>
  );
}
