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
import { AlertTriangle, Check, Power, Truck } from "lucide-react";
import toast from "react-hot-toast";
import {
  useSimpleLocale,
  getTranslation,
} from "@/i18n/SimpleTranslationProvider";
import { deliveryApi } from "@/api/delivery";
import { buildDeliveryToggleSettings } from "@/utils/partnerFallbackToggles";
import ActivationPanel from "./shared/ActivationPanel";
import { btnPrimaryNextUI } from "@/components/ui/buttonStyles";

interface DeliveryToggleProps {
  businessId: number;
  isLocked?: boolean; // Optional since DeliverySettings handles the lockdown
  onStatusChange?: (enabled: boolean) => void;
  variant?: "card" | "button";
  enabled?: boolean; // Optional: if provided, use this instead of loading locally
}

// Custom hook to get Delivery status - exported for use in other components.
// Mirrors useCRMStatus: takes an isLocked guard and re-enters loading when the
// lock lifts so consumers don't flash the stale disabled state. setEnabled is
// exposed so a toggle deeper in the tree can report back and keep the
// consumer's status (e.g. the DeliveryAdmin header chip) in sync.
export const useDeliveryStatus = (businessId: number, isLocked = false) => {
  const [enabled, setEnabled] = useState(false);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    let cancelled = false;

    const loadStatus = async () => {
      try {
        const settings = await deliveryApi.getDeliverySettings(businessId);
        if (cancelled) return;
        setEnabled(settings.delivery_enabled);
      } catch (error) {
        if (cancelled) return;
        console.error("Failed to load Delivery status:", error);

        // Fallback to localStorage
        const savedSettings = localStorage.getItem(
          `delivery_settings_${businessId}`,
        );
        if (savedSettings) {
          const settings = JSON.parse(savedSettings);
          if (cancelled) return;
          setEnabled(settings.delivery_enabled);
        }
      } finally {
        if (!cancelled) setLoading(false);
      }
    };

    if (!isLocked && businessId) {
      // Re-enter loading on re-runs (e.g. the access lock lifting after mount)
      // so consumers hold their current state for the whole status fetch
      // instead of flashing the stale disabled state.
      setLoading(true);
      void loadStatus();
    } else if (isLocked) {
      setLoading(false);
    }

    return () => {
      cancelled = true;
    };
  }, [businessId, isLocked]);

  return { enabled, loading, setEnabled };
};

export const DeliveryToggle: React.FC<DeliveryToggleProps> = ({
  businessId,
  isLocked = false,
  onStatusChange,
  variant = "card",
  enabled: externalEnabled,
}) => {
  const { locale } = useSimpleLocale();
  const [currentLocale, setCurrentLocale] = useState(locale);
  const [enabled, setEnabled] = useState(externalEnabled ?? false);
  const [loading, setLoading] = useState(externalEnabled === undefined);
  const [actionLoading, setActionLoading] = useState(false);
  const [showConfirmModal, setShowConfirmModal] = useState(false);
  const [showEnableModal, setShowEnableModal] = useState(false);

  // Update internal state when external prop changes
  useEffect(() => {
    if (externalEnabled !== undefined) {
      setEnabled(externalEnabled);
      setLoading(false);
    }
  }, [externalEnabled]);

  // Update translations when locale changes
  useEffect(() => {
    setCurrentLocale(locale);
  }, [locale]);

  // Translation helper
  const tString = useCallback(
    (key: string): string => {
      const fullKey = `deliveryToggle.${key}`;
      const result = getTranslation(fullKey, currentLocale);
      return Array.isArray(result) ? result[0] || key : (result as string);
    },
    [currentLocale],
  );

  // Load current status
  useEffect(() => {
    let cancelled = false;

    const loadStatus = async () => {
      // Prevent duplicate loading if already loading or external state provided
      if (externalEnabled !== undefined || !loading) return;

      try {
        const settings = await deliveryApi.getDeliverySettings(businessId);
        if (cancelled) return;
        setEnabled(settings.delivery_enabled);
      } catch (error) {
        if (cancelled) return;
        console.error("Failed to load Delivery status:", error);
        // Fallback to localStorage
        const savedSettings = localStorage.getItem(
          `delivery_settings_${businessId}`,
        );
        if (savedSettings) {
          const settings = JSON.parse(savedSettings);
          if (cancelled) return;
          setEnabled(settings.delivery_enabled);
        }
      } finally {
        if (!cancelled) setLoading(false);
      }
    };

    // Skip if is locked
    if (isLocked) {
      setLoading(false);
      return;
    }

    if (businessId) {
      void loadStatus();
    }

    return () => {
      cancelled = true;
    };
  }, [businessId, isLocked, externalEnabled, loading]);

  const handleToggle = async (newEnabled: boolean) => {
    setActionLoading(true);
    try {
      // Fresh GET only — never fall back to a stale localStorage snapshot.
      // A shadow PUT of cached zones can silently delete newer zones that
      // were added after the snapshot was written.
      const currentSettings =
        await deliveryApi.getDeliverySettings(businessId);

      const updatedSettings = buildDeliveryToggleSettings(
        // DeliverySettingsDto has no index signature — bridge through unknown
        // for the generic Record constraint.
        currentSettings as unknown as Record<string, unknown>,
        newEnabled,
      );

      await deliveryApi.updateDeliverySettings(
        businessId,
        updatedSettings as Parameters<
          typeof deliveryApi.updateDeliverySettings
        >[1],
      );

      setEnabled(newEnabled);
      onStatusChange?.(newEnabled);
      setShowConfirmModal(false);
      setShowEnableModal(false);
    } catch (error) {
      console.error("Failed to toggle Delivery:", error);
      toast.error(tString("updateError"));
    } finally {
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
                {tString("button.disable")}
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
                        tString("disableModal.features.zones"),
                        tString("disableModal.features.fees"),
                        tString("disableModal.features.tracking"),
                        tString("disableModal.features.management"),
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
                icon={Truck}
                title={tString("activationCard.title")}
                description={tString("activationCard.description")}
                features={[
                  {
                    title: tString("activationCard.features.zones.title"),
                    description: tString(
                      "activationCard.features.zones.description",
                    ),
                  },
                  {
                    title: tString("activationCard.features.fees.title"),
                    description: tString(
                      "activationCard.features.fees.description",
                    ),
                  },
                  {
                    title: tString("activationCard.features.tracking.title"),
                    description: tString(
                      "activationCard.features.tracking.description",
                    ),
                  },
                  {
                    title: tString("activationCard.features.management.title"),
                    description: tString(
                      "activationCard.features.management.description",
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
                    {tString("activationCard.button")}
                  </button>
                }
                footnote={tString("activationCard.hint")}
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
                        <Truck className="h-5 w-5" aria-hidden="true" />
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
                    <ModalFeatureList
                      items={[
                        tString("enableModal.features.zones"),
                        tString("enableModal.features.fees"),
                        tString("enableModal.features.tracking"),
                        tString("enableModal.features.management"),
                      ]}
                    />
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
