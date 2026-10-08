"use client";

import React, { useCallback, useEffect, useState } from "react";
import { AlertTriangle, Boxes, Power } from "lucide-react";
import {
  Button,
  Modal,
  ModalBody,
  ModalContent,
  ModalFooter,
  ModalHeader,
} from "@nextui-org/react";

import { inventoryApi } from "@/api/inventory";
import {
  getTranslation,
  useSimpleLocale,
} from "@/i18n/SimpleTranslationProvider";
import { getSafeApiErrorMessage } from "@/utils/apiError";
import ActivationPanel from "./shared/ActivationPanel";
import { btnPrimaryNextUI } from "@/components/ui/buttonStyles";

interface InventoryToggleProps {
  businessId: number;
  enabled: boolean;
  isLocked?: boolean;
  onStatusChange?: (enabled: boolean) => void | Promise<void>;
  onError?: (message: string) => void;
  variant?: "card" | "button";
}

export default function InventoryToggle({
  businessId,
  enabled,
  isLocked = false,
  onStatusChange,
  onError,
  variant = "card",
}: InventoryToggleProps) {
  const { locale } = useSimpleLocale();
  const [currentLocale, setCurrentLocale] = useState(locale);
  const [actionLoading, setActionLoading] = useState(false);
  const [showConfirmModal, setShowConfirmModal] = useState(false);
  const [showEnableModal, setShowEnableModal] = useState(false);

  useEffect(() => {
    setCurrentLocale(locale);
  }, [locale]);

  const tString = useCallback(
    (key: string): string => {
      const fullKey = `businessDashboard.inventoryManager.${key}`;
      const result = getTranslation(fullKey, currentLocale);
      return Array.isArray(result) ? result[0] || key : (result as string);
    },
    [currentLocale],
  );

  const handleToggle = async (newEnabled: boolean) => {
    setActionLoading(true);
    try {
      await inventoryApi.updateSettings(businessId, {
        inventory_enabled: newEnabled,
      });
      await onStatusChange?.(newEnabled);
      setShowConfirmModal(false);
      setShowEnableModal(false);
    } catch (error) {
      console.error("Failed to toggle inventory:", error);
      // FIND-058: safe product copy only — never raw axios/gin dumps.
      onError?.(
        getSafeApiErrorMessage(error, tString("errors.saveSettings")),
      );
    } finally {
      setActionLoading(false);
    }
  };

  if (isLocked) {
    return null;
  }

  return (
    <>
      {variant === "button" && enabled && (
        <>
          <button
            onClick={() => setShowConfirmModal(true)}
            disabled={actionLoading}
            className="flex items-center gap-2 whitespace-nowrap rounded-xl border border-warm-200 bg-white/80 px-4 py-2 font-medium text-ink-700 shadow-sm shadow-warm-900/5 transition hover:-translate-y-px hover:border-rose-300 hover:bg-rose-50 hover:text-rose-700 disabled:cursor-not-allowed disabled:opacity-60"
          >
            <Power className="w-4 h-4" />
            {tString("button.disableInventory")}
          </button>

          <Modal
            isOpen={showConfirmModal}
            onClose={() => setShowConfirmModal(false)}
          >
            <ModalContent className="rounded-3xl border border-warm-200 bg-white shadow-2xl shadow-warm-900/15">
              <ModalHeader className="flex flex-col gap-1">
                <div className="flex items-center gap-2 text-amber-600">
                  <AlertTriangle className="w-5 h-5" />
                  <span className="tracking-wide">
                    {tString("disableModal.title")}
                  </span>
                </div>
              </ModalHeader>
              <ModalBody>
                <p className="text-ink-700 leading-relaxed">
                  {tString("disableModal.description")}
                </p>
                <ul className="list-disc list-inside space-y-2 text-ink-700 ml-4">
                  <li>{tString("disableModal.features.stockTracking")}</li>
                  <li>{tString("disableModal.features.recipeMapping")}</li>
                </ul>
                <p className="text-ink-900 font-semibold mt-4 tracking-wide">
                  {tString("disableModal.confirmation")}
                </p>
              </ModalBody>
              <ModalFooter>
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

      {variant === "card" && !enabled && (
        <>
          <ActivationPanel
            icon={Boxes}
            title={tString("activationCard.title")}
            description={tString("activationCard.description")}
            features={[
              {
                title: tString("activationCard.features.stockTracking.title"),
                description: tString(
                  "activationCard.features.stockTracking.description",
                ),
              },
              {
                title: tString("activationCard.features.recipeMapping.title"),
                description: tString(
                  "activationCard.features.recipeMapping.description",
                ),
              },
            ]}
            action={
              <button
                type="button"
                onClick={() => setShowEnableModal(true)}
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

          <Modal
            isOpen={showEnableModal}
            onClose={() => setShowEnableModal(false)}
          >
            <ModalContent className="rounded-3xl border border-warm-200 bg-white shadow-2xl shadow-warm-900/15">
              <ModalHeader className="flex flex-col gap-1">
                <div className="flex items-center gap-2 text-ink-950">
                  <Boxes className="w-5 h-5" />
                  <span className="tracking-wide">
                    {tString("enableModal.title")}
                  </span>
                </div>
              </ModalHeader>
              <ModalBody>
                <p className="text-ink-700 leading-relaxed">
                  {tString("enableModal.description")}
                </p>
                <div className="mt-4">
                  <p className="font-semibold text-ink-950 mb-2 tracking-wide">
                    {tString("enableModal.featuresTitle")}
                  </p>
                  <ul className="list-disc list-inside space-y-2 text-ink-700 ml-4">
                    <li>{tString("enableModal.features.stockTracking")}</li>
                    <li>{tString("enableModal.features.recipeMapping")}</li>
                  </ul>
                </div>
              </ModalBody>
              <ModalFooter>
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
  );
}
