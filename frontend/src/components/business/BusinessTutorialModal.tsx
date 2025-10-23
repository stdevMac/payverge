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
  const [currentLocale, setCurrentLocale] = useState(locale);
  
  // Update translations when locale changes
  useEffect(() => {
    setCurrentLocale(locale);
  }, [locale]);

  // Helper function to get translation (following business registration pattern)
  const t = (key: string): string => {
    const result = getTranslation(key, currentLocale);
    return Array.isArray(result) ? result[0] || key : result as string;
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
        <div className="text-center space-y-6">
          <div className="w-20 h-20 bg-gray-100 border border-gray-200 rounded-3xl flex items-center justify-center mx-auto">
            <IoRestaurantOutline className="w-10 h-10 text-gray-700" />
          </div>
          <p className="text-lg font-light text-gray-600 leading-relaxed max-w-2xl mx-auto">
            {t("businessTutorialModal.steps.welcome.content.subtitle")}
          </p>
          <div className="bg-gray-100 p-6 rounded-2xl border border-gray-200">
            <p className="text-gray-900 font-medium text-lg">
              {t("businessTutorialModal.steps.welcome.content.callToAction")}
            </p>
          </div>
        </div>
      ),
      icon: <IoRestaurantOutline className="w-8 h-8 text-gray-700" />
    },
    {
      title: t("businessTutorialModal.steps.digitalMenu.title"),
      description: t("businessTutorialModal.steps.digitalMenu.description"),
      content: (
        <div className="space-y-6">
          <div className="w-20 h-20 bg-gray-100 border border-gray-200 rounded-3xl flex items-center justify-center mx-auto">
            <IoQrCodeOutline className="w-10 h-10 text-gray-700" />
          </div>
          <div className="space-y-4">
            <div className="flex items-start gap-4 p-4 bg-white rounded-xl border border-gray-200">
              <div className="w-8 h-8 bg-green-100 rounded-lg flex items-center justify-center flex-shrink-0">
                <IoCheckmarkCircleOutline className="w-5 h-5 text-green-600" />
              </div>
              <div>
                <h4 className="font-medium text-gray-900 mb-1">{t("businessTutorialModal.steps.digitalMenu.features.qrTables.title")}</h4>
                <p className="text-gray-600 text-sm leading-relaxed">{t("businessTutorialModal.steps.digitalMenu.features.qrTables.description")}</p>
              </div>
            </div>
            <div className="flex items-start gap-4 p-4 bg-white rounded-xl border border-gray-200">
              <div className="w-8 h-8 bg-green-100 rounded-lg flex items-center justify-center flex-shrink-0">
                <IoCheckmarkCircleOutline className="w-5 h-5 text-green-600" />
              </div>
              <div>
                <h4 className="font-medium text-gray-900 mb-1">{t("businessTutorialModal.steps.digitalMenu.features.realTimeUpdates.title")}</h4>
                <p className="text-gray-600 text-sm leading-relaxed">{t("businessTutorialModal.steps.digitalMenu.features.realTimeUpdates.description")}</p>
              </div>
            </div>
            <div className="flex items-start gap-4 p-4 bg-white rounded-xl border border-gray-200">
              <div className="w-8 h-8 bg-green-100 rounded-lg flex items-center justify-center flex-shrink-0">
                <IoCheckmarkCircleOutline className="w-5 h-5 text-green-600" />
              </div>
              <div>
                <h4 className="font-medium text-gray-900 mb-1">{t("businessTutorialModal.steps.digitalMenu.features.mobileOptimized.title")}</h4>
                <p className="text-gray-600 text-sm leading-relaxed">{t("businessTutorialModal.steps.digitalMenu.features.mobileOptimized.description")}</p>
              </div>
            </div>
          </div>
        </div>
      ),
      icon: <IoQrCodeOutline className="w-8 h-8 text-gray-700" />
    },
    {
      title: t("businessTutorialModal.steps.usdcPayments.title"),
      description: t("businessTutorialModal.steps.usdcPayments.description"),
      content: (
        <div className="space-y-6">
          <div className="w-20 h-20 bg-gray-100 border border-gray-200 rounded-3xl flex items-center justify-center mx-auto">
            <IoCardOutline className="w-10 h-10 text-gray-700" />
          </div>
          <div className="space-y-4">
            <div className="flex items-start gap-4 p-4 bg-white rounded-xl border border-gray-200">
              <div className="w-8 h-8 bg-green-100 rounded-lg flex items-center justify-center flex-shrink-0">
                <IoCheckmarkCircleOutline className="w-5 h-5 text-green-600" />
              </div>
              <div>
                <h4 className="font-medium text-gray-900 mb-1">{t("businessTutorialModal.steps.usdcPayments.features.instantSettlement.title")}</h4>
                <p className="text-gray-600 text-sm leading-relaxed">{t("businessTutorialModal.steps.usdcPayments.features.instantSettlement.description")}</p>
              </div>
            </div>
            <div className="flex items-start gap-4 p-4 bg-white rounded-xl border border-gray-200">
              <div className="w-8 h-8 bg-green-100 rounded-lg flex items-center justify-center flex-shrink-0">
                <IoCheckmarkCircleOutline className="w-5 h-5 text-green-600" />
              </div>
              <div>
                <h4 className="font-medium text-gray-900 mb-1">{t("businessTutorialModal.steps.usdcPayments.features.lowerFees.title")}</h4>
                <p className="text-gray-600 text-sm leading-relaxed">{t("businessTutorialModal.steps.usdcPayments.features.lowerFees.description")}</p>
              </div>
            </div>
            <div className="flex items-start gap-4 p-4 bg-white rounded-xl border border-gray-200">
              <div className="w-8 h-8 bg-green-100 rounded-lg flex items-center justify-center flex-shrink-0">
                <IoCheckmarkCircleOutline className="w-5 h-5 text-green-600" />
              </div>
              <div>
                <h4 className="font-medium text-gray-900 mb-1">{t("businessTutorialModal.steps.usdcPayments.features.globalAccess.title")}</h4>
                <p className="text-gray-600 text-sm leading-relaxed">{t("businessTutorialModal.steps.usdcPayments.features.globalAccess.description")}</p>
              </div>
            </div>
          </div>
        </div>
      ),
      icon: <IoCardOutline className="w-8 h-8 text-gray-700" />
    },
    {
      title: t("businessTutorialModal.steps.smartTipping.title"),
      description: t("businessTutorialModal.steps.smartTipping.description"),
      content: (
        <div className="space-y-6">
          <div className="w-20 h-20 bg-gray-100 border border-gray-200 rounded-3xl flex items-center justify-center mx-auto">
            <IoWalletOutline className="w-10 h-10 text-gray-700" />
          </div>
          <div className="space-y-4">
            <div className="flex items-start gap-4 p-4 bg-white rounded-xl border border-gray-200">
              <div className="w-8 h-8 bg-green-100 rounded-lg flex items-center justify-center flex-shrink-0">
                <IoCheckmarkCircleOutline className="w-5 h-5 text-green-600" />
              </div>
              <div>
                <h4 className="font-medium text-gray-900 mb-1">{t("businessTutorialModal.steps.smartTipping.features.directToStaff.title")}</h4>
                <p className="text-gray-600 text-sm leading-relaxed">{t("businessTutorialModal.steps.smartTipping.features.directToStaff.description")}</p>
              </div>
            </div>
            <div className="flex items-start gap-4 p-4 bg-white rounded-xl border border-gray-200">
              <div className="w-8 h-8 bg-green-100 rounded-lg flex items-center justify-center flex-shrink-0">
                <IoCheckmarkCircleOutline className="w-5 h-5 text-green-600" />
              </div>
              <div>
                <h4 className="font-medium text-gray-900 mb-1">{t("businessTutorialModal.steps.smartTipping.features.flexibleOptions.title")}</h4>
                <p className="text-gray-600 text-sm leading-relaxed">{t("businessTutorialModal.steps.smartTipping.features.flexibleOptions.description")}</p>
              </div>
            </div>
            <div className="flex items-start gap-4 p-4 bg-white rounded-xl border border-gray-200">
              <div className="w-8 h-8 bg-green-100 rounded-lg flex items-center justify-center flex-shrink-0">
                <IoCheckmarkCircleOutline className="w-5 h-5 text-green-600" />
              </div>
              <div>
                <h4 className="font-medium text-gray-900 mb-1">{t("businessTutorialModal.steps.smartTipping.features.transparentTracking.title")}</h4>
                <p className="text-gray-600 text-sm leading-relaxed">{t("businessTutorialModal.steps.smartTipping.features.transparentTracking.description")}</p>
              </div>
            </div>
          </div>
        </div>
      ),
      icon: <IoWalletOutline className="w-8 h-8 text-gray-700" />
    },
    {
      title: t("businessTutorialModal.steps.analytics.title"),
      description: t("businessTutorialModal.steps.analytics.description"),
      content: (
        <div className="space-y-6">
          <div className="w-20 h-20 bg-gray-100 border border-gray-200 rounded-3xl flex items-center justify-center mx-auto">
            <IoBarChartOutline className="w-10 h-10 text-gray-700" />
          </div>
          <div className="space-y-4">
            <div className="flex items-start gap-4 p-4 bg-white rounded-xl border border-gray-200">
              <div className="w-8 h-8 bg-green-100 rounded-lg flex items-center justify-center flex-shrink-0">
                <IoCheckmarkCircleOutline className="w-5 h-5 text-green-600" />
              </div>
              <div>
                <h4 className="font-medium text-gray-900 mb-1">{t("businessTutorialModal.steps.analytics.features.realTimeDashboard.title")}</h4>
                <p className="text-gray-600 text-sm leading-relaxed">{t("businessTutorialModal.steps.analytics.features.realTimeDashboard.description")}</p>
              </div>
            </div>
            <div className="flex items-start gap-4 p-4 bg-white rounded-xl border border-gray-200">
              <div className="w-8 h-8 bg-green-100 rounded-lg flex items-center justify-center flex-shrink-0">
                <IoCheckmarkCircleOutline className="w-5 h-5 text-green-600" />
              </div>
              <div>
                <h4 className="font-medium text-gray-900 mb-1">{t("businessTutorialModal.steps.analytics.features.itemAnalytics.title")}</h4>
                <p className="text-gray-600 text-sm leading-relaxed">{t("businessTutorialModal.steps.analytics.features.itemAnalytics.description")}</p>
              </div>
            </div>
            <div className="flex items-start gap-4 p-4 bg-white rounded-xl border border-gray-200">
              <div className="w-8 h-8 bg-green-100 rounded-lg flex items-center justify-center flex-shrink-0">
                <IoCheckmarkCircleOutline className="w-5 h-5 text-green-600" />
              </div>
              <div>
                <h4 className="font-medium text-gray-900 mb-1">{t("businessTutorialModal.steps.analytics.features.exportReports.title")}</h4>
                <p className="text-gray-600 text-sm leading-relaxed">{t("businessTutorialModal.steps.analytics.features.exportReports.description")}</p>
              </div>
            </div>
          </div>
        </div>
      ),
      icon: <IoBarChartOutline className="w-8 h-8 text-gray-700" />
    },
    {
      title: t("businessTutorialModal.steps.connectWallet.title"),
      description: t("businessTutorialModal.steps.connectWallet.description"),
      content: (
        <div className="space-y-6">
          <div className="w-20 h-20 bg-gray-100 border border-gray-200 rounded-3xl flex items-center justify-center mx-auto">
            <IoShieldCheckmarkOutline className="w-10 h-10 text-gray-700" />
          </div>
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
                <p className="text-green-700 font-medium">{t("businessTutorialModal.steps.connectWallet.content.success.title")}</p>
                <p className="text-green-600 text-sm mt-1">
                  {t("businessTutorialModal.steps.connectWallet.content.success.subtitle")}
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
                  <div className="bg-blue-50 p-3 rounded-lg">
                    <p className="text-blue-700 text-sm text-center">
                      Please sign the message to complete setup
                    </p>
                  </div>
                )}
              </div>
            )}
          </div>
        </div>
      ),
      icon: <IoShieldCheckmarkOutline className="w-8 h-8 text-gray-700" />
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

  const handleConnectWallet = () => {
    setIsConnecting(true);
    open();
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
      size="3xl"
      isDismissable={false}
      hideCloseButton
      classNames={{
        backdrop: "bg-gray-900/50 backdrop-blur-sm",
        base: "bg-white",
        body: "p-0",
        header: "p-0",
        footer: "p-0"
      }}
    >
      <ModalContent className="bg-white rounded-2xl shadow-xl">
        <ModalHeader className="p-6 sm:p-8 pb-4">
          <div className="w-full">
            <div className="flex flex-col sm:flex-row justify-between items-start sm:items-center gap-4 mb-6">
              <div className="flex items-center gap-4">
                <div className="w-12 h-12 bg-gray-50 border border-gray-200 rounded-2xl flex items-center justify-center">
                  {currentStepData.icon}
                </div>
                <div>
                  <h2 className="text-2xl font-light text-gray-900 tracking-wide">{currentStepData.title}</h2>
                  <p className="text-base font-light text-gray-600 mt-1">{currentStepData.description}</p>
                </div>
              </div>
              <div className="text-center sm:text-right">
                <span className="text-xl font-medium text-gray-900">
                  {Math.round(progress)}%
                </span>
                <p className="text-sm text-gray-500">
                  {t("businessTutorialModal.progress.step").replace("{{current}}", (currentStep + 1).toString()).replace("{{total}}", tutorialSteps.length.toString())}
                </p>
              </div>
            </div>
            <Progress 
              value={progress} 
              color="primary" 
              className="w-full"
              size="md"
              classNames={{
                track: "bg-gray-100",
                indicator: "bg-gray-900"
              }}
            />
          </div>
        </ModalHeader>
        
        <ModalBody className="px-6 sm:px-8 py-4">
          <div className="bg-gray-50 rounded-2xl p-6 sm:p-8">
            {currentStepData.content}
          </div>
        </ModalBody>
        
        <ModalFooter className="p-6 sm:p-8 pt-4">
          <div className="flex flex-col sm:flex-row justify-between items-stretch sm:items-center gap-4 w-full">
            <Button
              variant="bordered"
              onPress={handlePrevious}
              isDisabled={currentStep === 0}
              className="h-12 px-6 border-2 font-medium"
              size="lg"
            >
              {t("businessTutorialModal.buttons.previous") || "Previous"}
            </Button>
            
            <div className="flex flex-col sm:flex-row gap-3 sm:gap-4 w-full sm:w-auto">
              {!isLastStep && (
                <Button
                  variant="light"
                  onPress={handleSkipTutorial}
                  className="h-12 px-6 font-medium text-gray-600 hover:text-gray-800"
                  size="lg"
                >
                  {t("businessTutorialModal.buttons.skip") || "Skip Tutorial"}
                </Button>
              )}
              
              {isLastStep ? (
                <Button
                  color="primary"
                  onPress={handleComplete}
                  isDisabled={!isConnected || !appKitConnected || !address}
                  startContent={<IoRocketOutline className="w-4 h-4" />}
                  className="h-12 px-6 bg-gray-900 hover:bg-gray-800 font-medium shadow-sm"
                  size="lg"
                >
                  {t("businessTutorialModal.buttons.startBuilding")}
                </Button>
              ) : (
                <Button
                  color="primary"
                  onPress={handleNext}
                  className="h-12 px-6 bg-gray-900 hover:bg-gray-800 font-medium shadow-sm"
                  size="lg"
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
