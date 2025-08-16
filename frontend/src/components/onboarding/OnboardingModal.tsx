"use client";

import React, { useState, useEffect } from "react";
import {
  Modal,
  ModalContent,
  ModalHeader,
  ModalBody,
  ModalFooter,
  Button,
  Progress,
  Divider,
} from "@nextui-org/react";
import { useTranslation } from "@/i18n/useTranslation";
import { LanguageSelector } from "@/components/language/LanguageSelector";
import {
  IoWalletOutline,
  IoCarSportOutline,
  IoStatsChartOutline,
  IoShieldCheckmarkOutline,
  IoRocketOutline,
  IoImageOutline,
} from "react-icons/io5";

interface OnboardingStep {
  icon: React.ReactNode;
  // image?: string;
  // imageAlt?: string;
}

interface OnboardingModalProps {
  isOpen: boolean;
  onClose: () => void;
}

const steps: OnboardingStep[] = [
  {
    icon: <IoRocketOutline className="w-12 h-12 text-primary" />,
  },
  {
    icon: <IoWalletOutline className="w-12 h-12 text-primary" />,
  },
  {
    icon: <IoShieldCheckmarkOutline className="w-12 h-12 text-primary" />,
  },
  {
    icon: <IoImageOutline className="w-12 h-12 text-primary" />,
  },
  {
    icon: <IoCarSportOutline className="w-12 h-12 text-primary" />,
  },
  {
    icon: <IoStatsChartOutline className="w-12 h-12 text-primary" />,
  },
  {
    icon: <IoRocketOutline className="w-12 h-12 text-primary" />,
  },
];

export const OnboardingModal: React.FC<OnboardingModalProps> = ({
  isOpen,
  onClose,
}) => {
  const { language } = useTranslation();
  
  const { t } = useTranslation();
  const [currentStep, setCurrentStep] = useState(0);
  // We now rely on the isOpen prop from the parent component for visibility
  const [isTransitioning, setIsTransitioning] = useState(false);
  // const [imagesLoaded, setImagesLoaded] = useState<{[key: string]: boolean}>({});

  // No need to check localStorage here as that's handled by the useOnboarding hook

  const handleNext = () => {
    if (currentStep < steps.length - 1) {
      setIsTransitioning(true);
      setTimeout(() => {
        setCurrentStep(currentStep + 1);
        setIsTransitioning(false);
      }, 200);
    }
  };

  const handlePrevious = () => {
    if (currentStep > 0) {
      setIsTransitioning(true);
      setTimeout(() => {
        setCurrentStep(currentStep - 1);
        setIsTransitioning(false);
      }, 200);
    }
  };

  const handleFinish = () => {
    // The parent component will handle storing the onboarding state
    onClose();
  };

  // Visibility is now controlled by the isOpen prop

  return (
    <Modal
      isOpen={isOpen}
      onClose={handleFinish}
      size="3xl"
      classNames={{
        base: "bg-content1",
      }}
    >
      <ModalContent>
        <ModalHeader className="flex flex-col gap-1">
          <h2 className="text-2xl font-bold text-foreground">
            {t(`profile.onboarding.steps.step${currentStep}.title`)}
          </h2>
        </ModalHeader>
        <ModalBody>
          <div
            className={`space-y-6 transition-opacity duration-200 ${isTransitioning ? "opacity-0" : "opacity-100"}`}
          >
            <div className="flex flex-col items-center space-y-4">
              {steps[currentStep].icon}
              <p className="text-default-600 text-center text-lg">
                {t(`profile.onboarding.steps.step${currentStep}.description`)}
              </p>
            </div>
            
            {currentStep === 0 && (
              <div className="mt-4">
                <LanguageSelector className="mx-auto max-w-xs" />
              </div>
            )}

            <Divider className="my-4" />

            <Progress
              aria-label="Onboarding steps progress"
              value={(currentStep + 1) * (100 / steps.length)}
              className="w-full"
              color="primary"
              showValueLabel={true}
              valueLabel={`${currentStep + 1}/${steps.length}`}
            />
          </div>
        </ModalBody>
        <ModalFooter>
          <Button
            variant="light"
            onPress={handleFinish}
            className="text-default-500"
          >
            {t("profile.onboarding.navigation.skip")}
          </Button>
          <Button
            variant="light"
            onPress={handlePrevious}
            isDisabled={currentStep === 0}
          >
            {t("profile.onboarding.navigation.previous")}
          </Button>
          {currentStep < steps.length - 1 ? (
            <Button color="primary" onPress={handleNext}>
              {t("profile.onboarding.navigation.next")}
            </Button>
          ) : (
            <Button
              color="primary"
              onPress={handleFinish}
              className="bg-gradient-to-r from-primary to-primary-600"
            >
              {t("profile.onboarding.navigation.startInvesting")}
            </Button>
          )}
        </ModalFooter>
      </ModalContent>
    </Modal>
  );
};
