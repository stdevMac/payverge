"use client";

import React, { useState, useEffect, useCallback } from "react";
import {
  Modal,
  ModalContent,
  ModalHeader,
  ModalBody,
  ModalFooter,
  Button,
} from "@nextui-org/react";
import { AlertTriangle, Bot, Check, Power } from "lucide-react";
import toast from "react-hot-toast";
import { axiosInstance } from "@/api/tools/instance";
import {
  useSimpleLocale,
  getTranslation,
} from "@/i18n/SimpleTranslationProvider";
import ActivationPanel from "../shared/ActivationPanel";
import { btnPrimaryNextUI } from "@/components/ui/buttonStyles";

interface AiWaiterToggleProps {
  businessId: number;
  isLocked: boolean;
  initialEnabled?: boolean;
  onStatusChange?: (enabled: boolean) => void;
  variant?: "card" | "button";
}

const AiWaiterToggle: React.FC<AiWaiterToggleProps> = ({
  businessId,
  isLocked,
  initialEnabled = true,
  onStatusChange,
  variant = "card",
}) => {
  const { locale } = useSimpleLocale();
  const [currentLocale, setCurrentLocale] = useState(locale);
  const [enabled, setEnabled] = useState(initialEnabled);
  const [loading, _setLoading] = useState(false);
  const [actionLoading, setActionLoading] = useState(false);
  const [showConfirmModal, setShowConfirmModal] = useState(false);
  const [showEnableModal, setShowEnableModal] = useState(false);

  // Update translations when locale changes
  useEffect(() => {
    setCurrentLocale(locale);
  }, [locale]);

  // Sync with parent
  useEffect(() => {
    setEnabled(initialEnabled);
  }, [initialEnabled]);

  // Translation helper
  const tString = useCallback(
    (key: string): string => {
      const fullKey = `aiWaiterToggle.${key}`;
      const result = getTranslation(fullKey, currentLocale);
      return Array.isArray(result) ? result[0] || key : (result as string);
    },
    [currentLocale],
  );

  const handleToggle = async (newEnabled: boolean) => {
    setActionLoading(true);
    try {
      await axiosInstance.put(`/inside/businesses/${businessId}`, {
        ai_enabled: newEnabled,
      });
      setEnabled(newEnabled);
      onStatusChange?.(newEnabled);
      setShowConfirmModal(false);
      setShowEnableModal(false);
    } catch (error) {
      console.error("Failed to toggle AI Waiter:", error);
      toast.error(tString("updateError"));
      setActionLoading(false);
    }
  };

  const handleEnableClick = () => {
    setShowEnableModal(true);
  };

  const handleDisableClick = () => {
    setShowConfirmModal(true);
  };

  return (
    <>
      {!isLocked && !loading && (
        <>
          {/* Button variant - only show if enabled */}
          {variant === "button" && enabled && (
            <>
              <button
                onClick={handleDisableClick}
                disabled={actionLoading}
                className="inline-flex items-center gap-2 whitespace-nowrap rounded-xl border border-warm-200/90 bg-white/80 px-4 py-2 font-medium text-ink-700 shadow-sm shadow-warm-300/20 transition-all duration-200 hover:-translate-y-0.5 hover:border-rose-300 hover:bg-rose-50 hover:text-rose-700 active:translate-y-0 disabled:cursor-not-allowed disabled:opacity-55"
              >
                <Power className="w-4 h-4" />
                {tString("button.disableAiWaiter")}
              </button>

              {/* Disable Confirmation Modal */}
              <Modal
                isOpen={showConfirmModal}
                onClose={() => setShowConfirmModal(false)}
              >
                <ModalContent className="rounded-2xl border border-warm-200/90 bg-white shadow-panel">
                  <ModalHeader className="flex flex-col gap-1 border-b border-warm-200/80 pb-4">
                    <div className="flex items-center gap-3 text-amber-700">
                      <span className="flex h-9 w-9 items-center justify-center rounded-xl border border-amber-200 bg-amber-50">
                        <AlertTriangle className="h-5 w-5" aria-hidden="true" />
                      </span>
                      <span className="font-semibold text-ink-950">
                        {tString("disableModal.title")}
                      </span>
                    </div>
                  </ModalHeader>
                  <ModalBody className="gap-4 py-5">
                    <p className="text-sm leading-6 text-ink-600">
                      {tString("disableModal.description")}
                    </p>
                    <ModalFeatureList
                      warning
                      items={[
                        tString("disableModal.features.concierge"),
                        tString("disableModal.features.recommendations"),
                        tString("disableModal.features.upselling"),
                        tString("disableModal.features.multilingual"),
                      ]}
                    />
                    <p className="rounded-xl border border-amber-200/80 bg-amber-50/80 px-4 py-3 text-sm font-medium leading-5 text-amber-900">
                      {tString("disableModal.confirmation")}
                    </p>
                  </ModalBody>
                  <ModalFooter className="border-t border-warm-200/80">
                    <Button
                      variant="light"
                      onPress={() => setShowConfirmModal(false)}
                      disabled={actionLoading}
                      radius="full"
                      className="font-medium text-ink-700 hover:bg-warm-100"
                    >
                      {tString("disableModal.buttons.cancel")}
                    </Button>
                    <Button
                      onPress={() => handleToggle(false)}
                      isLoading={actionLoading}
                      radius="full"
                      className="bg-rose-600 font-medium text-white hover:bg-rose-700"
                    >
                      {tString("disableModal.buttons.confirm")}
                    </Button>
                  </ModalFooter>
                </ModalContent>
              </Modal>
            </>
          )}

          {/* Card variant - only show if disabled */}
          {variant === "card" && !enabled && (
            <>
              <ActivationPanel
                icon={Bot}
                title={tString("activationCard.title")}
                description={tString("activationCard.description")}
                features={[
                  {
                    title: tString("activationCard.features.concierge.title"),
                    description: tString(
                      "activationCard.features.concierge.description",
                    ),
                  },
                  {
                    title: tString(
                      "activationCard.features.recommendations.title",
                    ),
                    description: tString(
                      "activationCard.features.recommendations.description",
                    ),
                  },
                  {
                    title: tString("activationCard.features.upselling.title"),
                    description: tString(
                      "activationCard.features.upselling.description",
                    ),
                  },
                  {
                    title: tString(
                      "activationCard.features.multilingual.title",
                    ),
                    description: tString(
                      "activationCard.features.multilingual.description",
                    ),
                  },
                ]}
                action={
                  <button
                    type="button"
                    onClick={handleEnableClick}
                    disabled={actionLoading}
                    className="inline-flex items-center gap-2 rounded-full bg-brand px-4 py-2 text-sm font-medium text-white transition-colors hover:bg-brand-dark disabled:cursor-not-allowed disabled:opacity-50"
                  >
                    {actionLoading
                      ? tString("activationCard.buttons.activating")
                      : tString("activationCard.buttons.activate")}
                  </button>
                }
                footnote={tString("activationCard.disclaimer")}
              />

              {/* Enable Confirmation Modal */}
              <Modal
                isOpen={showEnableModal}
                onClose={() => setShowEnableModal(false)}
              >
                <ModalContent className="rounded-2xl border border-warm-200/90 bg-white shadow-panel">
                  <ModalHeader className="flex flex-col gap-1 border-b border-warm-200/80 pb-4">
                    <div className="flex items-center gap-3">
                      <span className="flex h-9 w-9 items-center justify-center rounded-xl border border-brand/15 bg-brand/5 text-brand">
                        <Bot className="h-5 w-5" aria-hidden="true" />
                      </span>
                      <span className="font-semibold text-ink-950">
                        {tString("enableModal.title")}
                      </span>
                    </div>
                  </ModalHeader>
                  <ModalBody className="gap-4 py-5">
                    <p className="text-sm leading-6 text-ink-600">
                      {tString("enableModal.description")}
                    </p>
                    <div className="mt-4">
                      <p className="mb-3 text-sm font-semibold text-ink-950">
                        {tString("enableModal.featuresTitle")}
                      </p>
                      <ModalFeatureList
                        items={[
                          tString("enableModal.features.concierge"),
                          tString("enableModal.features.recommendations"),
                          tString("enableModal.features.upselling"),
                          tString("enableModal.features.multilingual"),
                        ]}
                      />
                    </div>
                  </ModalBody>
                  <ModalFooter className="border-t border-warm-200/80">
                    <Button
                      variant="light"
                      onPress={() => setShowEnableModal(false)}
                      disabled={actionLoading}
                      radius="full"
                      className="font-medium text-ink-700 hover:bg-warm-100"
                    >
                      {tString("enableModal.buttons.cancel")}
                    </Button>
                    <Button
                      radius="full"
                      className={btnPrimaryNextUI}
                      onPress={() => handleToggle(true)}
                      isLoading={actionLoading}
                    >
                      {tString("enableModal.buttons.confirm")}
                    </Button>
                  </ModalFooter>
                </ModalContent>
              </Modal>
            </>
          )}
        </>
      )}
    </>
  );
};

function ModalFeatureList({
  items,
  warning = false,
}: {
  items: string[];
  warning?: boolean;
}) {
  return (
    <ul className="grid gap-2">
      {items.map((item) => (
        <li key={item} className="flex items-start gap-2 text-sm text-ink-700">
          <span
            className={`mt-0.5 flex h-5 w-5 shrink-0 items-center justify-center rounded-full ${
              warning ? "bg-amber-100 text-amber-700" : "bg-brand/10 text-brand"
            }`}
          >
            <Check className="h-3.5 w-3.5" aria-hidden="true" />
          </span>
          <span className="leading-5">{item}</span>
        </li>
      ))}
    </ul>
  );
}

export default AiWaiterToggle;
