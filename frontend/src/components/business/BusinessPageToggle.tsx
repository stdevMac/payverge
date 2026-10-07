"use client";

import React, { useState } from "react";
import { Power, Check, Globe } from "lucide-react";
import {
    Modal,
    ModalContent,
    ModalHeader,
    ModalBody,
    ModalFooter,
    Button,
    Input,
} from "@nextui-org/react";
import {
    useSimpleLocale,
    getTranslation,
} from "@/i18n/SimpleTranslationProvider";
import ActivationPanel from "./shared/ActivationPanel";
import { btnPrimaryNextUI } from "@/components/ui/buttonStyles";

interface BusinessPageToggleProps {
    enabled: boolean;
    onToggle: (enabled: boolean) => Promise<void>;
    loading?: boolean;
    variant?: "card" | "button";
}

const DISABLE_CONFIRM_WORD = "DISABLE";

export const BusinessPageToggle: React.FC<BusinessPageToggleProps> = ({
    enabled,
    onToggle,
    loading = false,
    variant = "card",
}) => {
    const { locale } = useSimpleLocale();
    const [showConfirmModal, setShowConfirmModal] = useState(false);
    const [showEnableModal, setShowEnableModal] = useState(false);
    const [actionLoading, setActionLoading] = useState(false);
    const [disableConfirmText, setDisableConfirmText] = useState("");

    const tString = (key: string): string => {
        const fullKey = `businessSettings.businessPage.${key}`;
        const result = getTranslation(fullKey, locale);
        return Array.isArray(result) ? result[0] || key : (result as string);
    };

    const handleToggle = async (newEnabled: boolean) => {
        setActionLoading(true);
        try {
            await onToggle(newEnabled);
            setShowConfirmModal(false);
            setShowEnableModal(false);
            setDisableConfirmText("");
        } catch (error) {
            console.error("Failed to toggle:", error);
        } finally {
            setActionLoading(false);
        }
    };

    const closeDisableModal = () => {
        setShowConfirmModal(false);
        setDisableConfirmText("");
    };

    if (loading) return null;

    // Button Variant (To Disable)
    if (variant === "button") {
        if (!enabled) return null;

        const typedOk =
            disableConfirmText.trim().toUpperCase() === DISABLE_CONFIRM_WORD;

        return (
            <>
                <Button
                    onPress={() => setShowConfirmModal(true)}
                    isDisabled={loading || actionLoading}
                    variant="bordered"
                    className="border-warm-300 font-semibold text-ink-700 hover:bg-warm-50"
                    startContent={<Power className="w-4 h-4" />}
                >
                    {tString("button.disable") || "Disable Page"}
                </Button>

                <Modal isOpen={showConfirmModal} onClose={closeDisableModal}>
                    <ModalContent>
                        <ModalHeader className="flex flex-col gap-1">
                            {tString("disableModal.title") || "Disable Business Page?"}
                        </ModalHeader>
                        <ModalBody>
                            <p className="text-ink-600">
                                {tString("disableModal.description") ||
                                    "Your business page will no longer be publicly accessible. Customers won't be able to view your hours, gallery, or specials."}
                            </p>
                            <div className="mt-2 rounded-2xl border border-rose-200 bg-rose-50 p-4 space-y-2">
                                <p className="text-sm font-semibold text-rose-700">
                                    {tString("disableModal.qrConsequence") ||
                                        "Table QR codes that open this page will stop working for guests until you publish again."}
                                </p>
                                <p className="text-sm text-rose-700">
                                    {tString("disableModal.confirmation") ||
                                        "Are you sure you want to disable it?"}
                                </p>
                            </div>
                            <Input
                                className="mt-3"
                                label={
                                    tString("disableModal.typeConfirmLabel") ||
                                    `Type ${DISABLE_CONFIRM_WORD} to confirm`
                                }
                                placeholder={DISABLE_CONFIRM_WORD}
                                value={disableConfirmText}
                                onValueChange={setDisableConfirmText}
                                variant="bordered"
                                autoComplete="off"
                            />
                        </ModalBody>
                        <ModalFooter>
                            <Button
                                variant="light"
                                onPress={closeDisableModal}
                                disabled={actionLoading}
                                radius="full"
                                className="font-medium text-ink-700 hover:bg-warm-100"
                            >
                                {tString("disableModal.buttons.cancel") || "Cancel"}
                            </Button>
                            <Button
                                onPress={() => handleToggle(false)}
                                isLoading={actionLoading}
                                isDisabled={!typedOk}
                                radius="full"
                                className="bg-rose-600 font-medium text-white hover:bg-rose-700"
                            >
                                {tString("disableModal.buttons.confirm") || "Disable Page"}
                            </Button>
                        </ModalFooter>
                    </ModalContent>
                </Modal>
            </>
        );
    }

    // Card Variant (To Enable)
    if (variant === "card" && enabled) return null;

    return (
        <>
            <ActivationPanel
                icon={Globe}
                title={tString("activationCard.title") || "Premium Business Page"}
                description={
                    tString("activationCard.description") ||
                    "Establish your digital presence with a professional business page. Showcase your gallery, hours, and features to attract more customers."
                }
                features={[
                    {
                        title:
                            tString("activationCard.features.showcase.title") ||
                            "Visual Showcase",
                        description:
                            tString(
                                "activationCard.features.showcase.description",
                            ) || "Gallery, banner, and brand identity",
                    },
                    {
                        title:
                            tString("activationCard.features.seo.title") ||
                            "Search Optimized",
                        description:
                            tString("activationCard.features.seo.description") ||
                            "Custom URL and Google integration",
                    },
                ]}
                action={
                    <Button
                        radius="full"
                        className="bg-brand font-medium text-white hover:bg-brand-dark"
                        onPress={() => setShowEnableModal(true)}
                        isLoading={actionLoading}
                    >
                        {tString("activationCard.buttons.activate") ||
                            "Activate Business Page"}
                    </Button>
                }
                footnote={
                    tString("activationCard.footnote") ||
                    "You can turn your business page off anytime from settings."
                }
            />

            <Modal isOpen={showEnableModal} onClose={() => setShowEnableModal(false)}>
                <ModalContent>
                    <ModalHeader className="flex flex-col gap-1">
                        {tString("enableModal.title") || "Enable Business Page?"}
                    </ModalHeader>
                    <ModalBody>
                        <p className="mb-4 text-ink-600">
                            {tString("enableModal.description") || "This will promote your page to the public. You can customize the content before sharing the URL."}
                        </p>
                        <ul className="space-y-3">
                            <li className="flex items-center gap-3 text-sm text-ink-700">
                                <div className="flex h-6 w-6 flex-shrink-0 items-center justify-center rounded-full bg-emerald-50 text-emerald-700">
                                    <Check className="h-3 w-3" />
                                </div>
                                {tString("activationCard.features.showcase.title") || "Visual Showcase"}
                            </li>
                            <li className="flex items-center gap-3 text-sm text-ink-700">
                                <div className="flex h-6 w-6 flex-shrink-0 items-center justify-center rounded-full bg-emerald-50 text-emerald-700">
                                    <Check className="h-3 w-3" />
                                </div>
                                {tString("activationCard.features.seo.title") || "Search Optimized"}
                            </li>
                        </ul>
                    </ModalBody>
                    <ModalFooter>
                        <Button
                            variant="light"
                            onPress={() => setShowEnableModal(false)}
                            disabled={actionLoading}
                            radius="full"
                            className="font-medium text-ink-700 hover:bg-warm-100"
                        >
                            {tString("enableModal.buttons.cancel") || "Cancel"}
                        </Button>
                        <Button
                            radius="full"
                            className={btnPrimaryNextUI}
                            onPress={() => handleToggle(true)}
                            isLoading={actionLoading}
                        >
                            {tString("enableModal.buttons.confirm") || "Activate Page"}
                        </Button>
                    </ModalFooter>
                </ModalContent>
            </Modal>
        </>
    );
};
