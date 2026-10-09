"use client";

import React, { useState, useEffect, useCallback } from "react";
import { AlertTriangle, Check, ChefHat, Power } from "lucide-react";
import toast from "react-hot-toast";
import {
  getKitchenOrdersStatus,
  toggleKitchenAndOrders,
} from "@/api/kitchenOrders";
import {
  Modal,
  ModalContent,
  ModalHeader,
  ModalBody,
  ModalFooter,
  Button,
} from "@nextui-org/react";
import {
  useSimpleLocale,
  getTranslation,
} from "@/i18n/SimpleTranslationProvider";
import ActivationPanel from "./shared/ActivationPanel";
import { btnPrimaryNextUI } from "@/components/ui/buttonStyles";

interface KitchenOrdersToggleProps {
  businessId: number;
  isLocked: boolean;
  onStatusChange?: (enabled: boolean) => void;
  variant?: "card" | "button"; // card = big activation UI, button = compact header button
  /** When provided, skip internal fetch and use this value directly. */
  externalEnabled?: boolean;
  /** When true alongside externalEnabled, treat the status as still loading. */
  externalLoading?: boolean;
}

export const KitchenOrdersToggle: React.FC<KitchenOrdersToggleProps> = ({
  businessId,
  isLocked,
  onStatusChange,
  variant = "card",
  externalEnabled,
  externalLoading,
}) => {
  const useExternal = externalEnabled !== undefined;

  const { locale } = useSimpleLocale();
  const [currentLocale, setCurrentLocale] = useState(locale);
  const [internalEnabled, setInternalEnabled] = useState(false);
  const [internalLoading, setInternalLoading] = useState(true);
  const [actionLoading, setActionLoading] = useState(false);
  const [showConfirmModal, setShowConfirmModal] = useState(false);
  const [showEnableModal, setShowEnableModal] = useState(false);

  // Derived state: use external when provided, otherwise internal
  const enabled = useExternal ? externalEnabled : internalEnabled;
  const loading = useExternal ? (externalLoading ?? false) : internalLoading;

  // Update translations when locale changes
  useEffect(() => {
    setCurrentLocale(locale);
  }, [locale]);

  // Translation helper
  const tString = useCallback(
    (key: string): string => {
      const fullKey = `kitchenOrdersToggle.${key}`;
      const result = getTranslation(fullKey, currentLocale);
      return Array.isArray(result) ? result[0] || key : (result as string);
    },
    [currentLocale],
  );

  // Load current status (only when not using external state)
  useEffect(() => {
    if (useExternal) {
      setInternalLoading(false);
      return;
    }

    const loadStatus = async () => {
      if (!internalLoading) return;

      try {
        const status = await getKitchenOrdersStatus(businessId);
        setInternalEnabled(status.kitchen_enabled && status.orders_enabled);
      } catch (error) {
        console.error("Failed to load kitchen/orders status:", error);
      } finally {
        setInternalLoading(false);
      }
    };

    if (!isLocked) {
      loadStatus().catch((err) => console.error("loadStatus failed:", err));
    } else {
      setInternalLoading(false);
    }
  }, [businessId, isLocked, internalLoading, useExternal]);

  const handleToggle = async (newEnabled: boolean) => {
    setActionLoading(true);
    try {
      await toggleKitchenAndOrders(businessId, newEnabled);
      setInternalEnabled(newEnabled);
      onStatusChange?.(newEnabled);
      setShowConfirmModal(false);
      setShowEnableModal(false);
      // Removed window.location.reload() to prevent full page re-renders
      // Props and local state should handle the update
    } catch (error) {
      console.error("Failed to toggle kitchen/orders:", error);
      toast.error(tString("updateError"));
    } finally {
      // K-12: was only reset in the catch — success left the control
      // permanently disabled whenever the component stayed mounted.
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
      {(!isLocked && !loading) && (
        <>
          {/* Button variant - disable when enabled */}
          {variant === "button" && enabled && (
            <>
              <button
                onClick={handleDisableClick}
                disabled={actionLoading}
                className="inline-flex items-center gap-2 whitespace-nowrap rounded-xl border border-warm-200/90 bg-white/80 px-4 py-2 font-medium text-ink-700 shadow-sm shadow-warm-300/20 transition-all duration-200 hover:-translate-y-0.5 hover:border-rose-300 hover:bg-rose-50 hover:text-rose-700 active:translate-y-0 disabled:cursor-not-allowed disabled:opacity-55"
              >
                <Power className="w-4 h-4" />
                {tString("button.disableOrdering")}
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
                      items={[
                        tString("disableModal.features.createOrders"),
                        tString("disableModal.features.approveOrders"),
                        tString("disableModal.features.manageKitchen"),
                        tString("disableModal.features.trackStatus"),
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

          {/* Button variant - enable when disabled */}
          {variant === "button" && !enabled && (
            <button
              onClick={handleEnableClick}
              disabled={actionLoading}
              className="inline-flex items-center gap-2 whitespace-nowrap rounded-xl border border-brand/25 bg-brand/5 px-4 py-2 font-medium text-brand shadow-sm shadow-warm-300/20 transition-all duration-200 hover:-translate-y-0.5 hover:border-brand/45 hover:bg-brand/10 active:translate-y-0 disabled:cursor-not-allowed disabled:opacity-55"
            >
              <Power className="w-4 h-4" />
              {tString("button.enableOrdering")}
            </button>
          )}

          {/* Card variant - only show if disabled */}
          {variant === "card" && !enabled && (
            <ActivationPanel
              icon={ChefHat}
              title={tString("activationCard.title")}
              description={tString("activationCard.description")}
              features={[
                {
                  title: tString("activationCard.ordersManagement.title"),
                  description: tString(
                    "activationCard.ordersManagement.description",
                  ),
                },
                {
                  title: tString("activationCard.kitchenWorkflow.title"),
                  description: tString(
                    "activationCard.kitchenWorkflow.description",
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
          )}

          {/* Enable Confirmation Modal — shared by card + button variants */}
          {!enabled && (
            <Modal isOpen={showEnableModal} onClose={() => setShowEnableModal(false)}>
              <ModalContent className="rounded-2xl border border-warm-200/90 bg-white shadow-panel">
                <ModalHeader className="flex flex-col gap-1 border-b border-warm-200/80 pb-4">
                  <div className="flex items-center gap-3">
                    <span className="flex h-9 w-9 items-center justify-center rounded-xl border border-brand/15 bg-brand/5 text-brand">
                      <Power className="h-5 w-5" aria-hidden="true" />
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
                  <div>
                    <p className="mb-3 text-sm font-semibold text-ink-950">
                      {tString("enableModal.featuresTitle")}
                    </p>
                    <ModalFeatureList
                      items={[
                        tString("enableModal.features.createOrders"),
                        tString("enableModal.features.approveOrders"),
                        tString("enableModal.features.manageKitchen"),
                        tString("enableModal.features.trackStatus"),
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
          )}
        </>
      )}
    </>
  );
};

function ModalFeatureList({ items }: { items: string[] }) {
  return (
    <ul className="grid gap-2">
      {items.map((item) => (
        <li key={item} className="flex items-start gap-2 text-sm text-ink-700">
          <span className="mt-0.5 flex h-5 w-5 shrink-0 items-center justify-center rounded-full bg-brand/10 text-brand">
            <Check className="h-3.5 w-3.5" aria-hidden="true" />
          </span>
          <span className="leading-5">{item}</span>
        </li>
      ))}
    </ul>
  );
}
