'use client';

import React, { useState, useEffect } from 'react';
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
import { useSimpleLocale, getTranslation } from "@/i18n/SimpleTranslationProvider";

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
  const { locale } = useSimpleLocale();
  const [translations, setTranslations] = useState<any>({});

  useEffect(() => {
    const loadTranslations = async () => {
      const t = await getTranslation(locale);
      setTranslations(t);
    };
    loadTranslations();
  }, [locale]);

  // Helper function to get nested translation
  const t = (key: string) => {
    const keys = key.split('.');
    let value = translations;
    for (const k of keys) {
      value = value?.[k];
    }
    return value || key;
  };

  // Reset connecting state when wallet connection changes
  React.useEffect(() => {
    if (isConnected && appKitConnected) {
      setIsConnecting(false);
      setIsAuthenticating(false);
    }
  }, [isConnected, appKitConnected]);

  const tutorialSteps = [
    {
      title: t("businessTutorialModal.steps.welcome.title"),
      description: t("businessTutorialModal.steps.welcome.description"),
      content: (
        <div className="text-center space-y-4">
          <IoRestaurantOutline className="w-16 h-16 text-primary mx-auto" />
          <p className="text-gray-600">
            {t("businessTutorialModal.steps.welcome.content.subtitle")}
          </p>
          <div className="bg-primary-50 p-4 rounded-lg">
            <p className="text-primary-700 font-medium">
              {t("businessTutorialModal.steps.welcome.content.callToAction")}
            </p>
          </div>
        </div>
      ),
      icon: <IoRestaurantOutline className="w-8 h-8 text-primary" />
    },
    {
      title: t("businessTutorialModal.steps.digitalMenu.title"),
      description: t("businessTutorialModal.steps.digitalMenu.description"),
      content: (
        <div className="space-y-4">
          <IoQrCodeOutline className="w-16 h-16 text-primary mx-auto" />
          <div className="space-y-3">
            <div className="flex items-start gap-3">
              <IoCheckmarkCircleOutline className="w-5 h-5 text-green-500 mt-1 flex-shrink-0" />
              <div>
                <h4 className="font-medium">{t("businessTutorialModal.steps.digitalMenu.features.qrTables.title")}</h4>
                <p className="text-gray-600 text-sm">{t("businessTutorialModal.steps.digitalMenu.features.qrTables.description")}</p>
              </div>
            </div>
            <div className="flex items-start gap-3">
              <IoCheckmarkCircleOutline className="w-5 h-5 text-green-500 mt-1 flex-shrink-0" />
              <div>
                <h4 className="font-medium">{t("businessTutorialModal.steps.digitalMenu.features.realTimeUpdates.title")}</h4>
                <p className="text-gray-600 text-sm">{t("businessTutorialModal.steps.digitalMenu.features.realTimeUpdates.description")}</p>
              </div>
            </div>
            <div className="flex items-start gap-3">
              <IoCheckmarkCircleOutline className="w-5 h-5 text-green-500 mt-1 flex-shrink-0" />
              <div>
                <h4 className="font-medium">{t("businessTutorialModal.steps.digitalMenu.features.mobileOptimized.title")}</h4>
                <p className="text-gray-600 text-sm">{t("businessTutorialModal.steps.digitalMenu.features.mobileOptimized.description")}</p>
              </div>
            </div>
          </div>
        </div>
      ),
      icon: <IoQrCodeOutline className="w-8 h-8 text-primary" />
    },
    {
      title: t("businessTutorialModal.steps.usdcPayments.title"),
      description: t("businessTutorialModal.steps.usdcPayments.description"),
      content: (
        <div className="space-y-4">
          <IoCardOutline className="w-16 h-16 text-primary mx-auto" />
          <div className="space-y-3">
            <div className="flex items-start gap-3">
              <IoCheckmarkCircleOutline className="w-5 h-5 text-green-500 mt-1 flex-shrink-0" />
              <div>
                <h4 className="font-medium">{t("businessTutorialModal.steps.usdcPayments.features.instantSettlement.title")}</h4>
                <p className="text-gray-600 text-sm">{t("businessTutorialModal.steps.usdcPayments.features.instantSettlement.description")}</p>
              </div>
            </div>
            <div className="flex items-start gap-3">
              <IoCheckmarkCircleOutline className="w-5 h-5 text-green-500 mt-1 flex-shrink-0" />
              <div>
                <h4 className="font-medium">{t("businessTutorialModal.steps.usdcPayments.features.lowerFees.title")}</h4>
                <p className="text-gray-600 text-sm">{t("businessTutorialModal.steps.usdcPayments.features.lowerFees.description")}</p>
              </div>
            </div>
            <div className="flex items-start gap-3">
              <IoCheckmarkCircleOutline className="w-5 h-5 text-green-500 mt-1 flex-shrink-0" />
              <div>
                <h4 className="font-medium">{t("businessTutorialModal.steps.usdcPayments.features.globalAccess.title")}</h4>
                <p className="text-gray-600 text-sm">{t("businessTutorialModal.steps.usdcPayments.features.globalAccess.description")}</p>
              </div>
            </div>
          </div>
        </div>
      ),
      icon: <IoCardOutline className="w-8 h-8 text-primary" />
    },
    {
      title: t("businessTutorialModal.steps.smartTipping.title"),
      description: t("businessTutorialModal.steps.smartTipping.description"),
      content: (
        <div className="space-y-4">
          <IoWalletOutline className="w-16 h-16 text-primary mx-auto" />
          <div className="space-y-3">
            <div className="flex items-start gap-3">
              <IoCheckmarkCircleOutline className="w-5 h-5 text-green-500 mt-1 flex-shrink-0" />
              <div>
                <h4 className="font-medium">{t("businessTutorialModal.steps.smartTipping.features.directToStaff.title")}</h4>
                <p className="text-gray-600 text-sm">{t("businessTutorialModal.steps.smartTipping.features.directToStaff.description")}</p>
              </div>
            </div>
            <div className="flex items-start gap-3">
              <IoCheckmarkCircleOutline className="w-5 h-5 text-green-500 mt-1 flex-shrink-0" />
              <div>
                <h4 className="font-medium">{t("businessTutorialModal.steps.smartTipping.features.flexibleOptions.title")}</h4>
                <p className="text-gray-600 text-sm">{t("businessTutorialModal.steps.smartTipping.features.flexibleOptions.description")}</p>
              </div>
            </div>
            <div className="flex items-start gap-3">
              <IoCheckmarkCircleOutline className="w-5 h-5 text-green-500 mt-1 flex-shrink-0" />
              <div>
                <h4 className="font-medium">{t("businessTutorialModal.steps.smartTipping.features.transparentTracking.title")}</h4>
                <p className="text-gray-600 text-sm">{t("businessTutorialModal.steps.smartTipping.features.transparentTracking.description")}</p>
              </div>
            </div>
          </div>
        </div>
      ),
      icon: <IoWalletOutline className="w-8 h-8 text-primary" />
    },
    {
      title: t("businessTutorialModal.steps.analytics.title"),
      description: t("businessTutorialModal.steps.analytics.description"),
      content: (
        <div className="space-y-4">
          <IoBarChartOutline className="w-16 h-16 text-primary mx-auto" />
          <div className="space-y-3">
            <div className="flex items-start gap-3">
              <IoCheckmarkCircleOutline className="w-5 h-5 text-green-500 mt-1 flex-shrink-0" />
              <div>
                <h4 className="font-medium">{t("businessTutorialModal.steps.analytics.features.realTimeDashboard.title")}</h4>
                <p className="text-gray-600 text-sm">{t("businessTutorialModal.steps.analytics.features.realTimeDashboard.description")}</p>
              </div>
            </div>
            <div className="flex items-start gap-3">
              <IoCheckmarkCircleOutline className="w-5 h-5 text-green-500 mt-1 flex-shrink-0" />
              <div>
                <h4 className="font-medium">{t("businessTutorialModal.steps.analytics.features.itemAnalytics.title")}</h4>
                <p className="text-gray-600 text-sm">{t("businessTutorialModal.steps.analytics.features.itemAnalytics.description")}</p>
              </div>
            </div>
            <div className="flex items-start gap-3">
              <IoCheckmarkCircleOutline className="w-5 h-5 text-green-500 mt-1 flex-shrink-0" />
              <div>
                <h4 className="font-medium">{t("businessTutorialModal.steps.analytics.features.exportReports.title")}</h4>
                <p className="text-gray-600 text-sm">{t("businessTutorialModal.steps.analytics.features.exportReports.description")}</p>
              </div>
            </div>
          </div>
        </div>
      ),
      icon: <IoBarChartOutline className="w-8 h-8 text-primary" />
    },
    {
      title: t("businessTutorialModal.steps.connectWallet.title"),
      description: t("businessTutorialModal.steps.connectWallet.description"),
      content: (
        <div className="space-y-6">
          <IoShieldCheckmarkOutline className="w-16 h-16 text-primary mx-auto" />
          <div className="text-center space-y-4">
            <p className="text-gray-600">
              {t("businessTutorialModal.steps.connectWallet.content.subtitle")}
            </p>
            <div className="space-y-2 text-left max-w-md mx-auto">
              <div className="flex items-center gap-2">
                <IoCheckmarkCircleOutline className="w-4 h-4 text-green-500" />
                <span className="text-sm">{t("businessTutorialModal.steps.connectWallet.content.securityList.receive")}</span>
              </div>
              <div className="flex items-center gap-2">
                <IoCheckmarkCircleOutline className="w-4 h-4 text-green-500" />
                <span className="text-sm">{t("businessTutorialModal.steps.connectWallet.content.securityList.manage")}</span>
              </div>
              <div className="flex items-center gap-2">
                <IoCheckmarkCircleOutline className="w-4 h-4 text-green-500" />
                <span className="text-sm">{t("businessTutorialModal.steps.connectWallet.content.securityList.register")}</span>
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
                  {isConnecting ? t("businessTutorialModal.buttons.connecting") : 
                   isAuthenticating ? t("businessTutorialModal.buttons.connecting") : 
                   t("businessTutorialModal.buttons.connectWallet")}
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
              {t("businessTutorialModal.progress.step").replace("{{current}}", (currentStep + 1).toString()).replace("{{total}}", tutorialSteps.length.toString())}
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
              {t("businessTutorialModal.buttons.previous") || "Previous"}
            </Button>
            
            <div className="flex gap-2">
              <Button
                variant="light"
                onPress={handleSkipTutorial}
              >
                {t("businessTutorialModal.buttons.skip") || "Skip Tutorial"}
              </Button>
              
              {isLastStep ? (
                <Button
                  color="primary"
                  onPress={handleComplete}
                  isDisabled={!isConnected || !appKitConnected || !address}
                  startContent={<IoRocketOutline className="w-4 h-4" />}
                >
                  {t("businessTutorialModal.buttons.startBuilding")}
                </Button>
              ) : (
                <Button
                  color="primary"
                  onPress={handleNext}
                >
                  {t("businessTutorialModal.buttons.next")}
                </Button>
              )}
            </div>
          </div>
        </ModalFooter>
      </ModalContent>
    </Modal>
  );
}
