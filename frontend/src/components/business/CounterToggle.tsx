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
import { Power, AlertTriangle, Check, Coffee } from "lucide-react";
import toast from "react-hot-toast";
import {
    useSimpleLocale,
    getTranslation,
} from "@/i18n/SimpleTranslationProvider";
import {
    updateCounterSettings,
    getBusinessCounters,
} from "../../api/counters";
import { surfaceBackendError } from "@/utils/localizedError";
import ActivationPanel from "./shared/ActivationPanel";
import { btnPrimaryNextUI } from "@/components/ui/buttonStyles";

interface CounterToggleProps {
    businessId: number;
    isLocked?: boolean;
    onStatusChange?: (enabled: boolean) => void;
    variant?: "card" | "button";
    enabled?: boolean;
}

export const CounterToggle: React.FC<CounterToggleProps> = ({
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

    useEffect(() => {
        if (externalEnabled !== undefined) {
            setEnabled(externalEnabled);
            setLoading(false);
        }
    }, [externalEnabled]);

    useEffect(() => {
        setCurrentLocale(locale);
    }, [locale]);

    const tString = useCallback(
        (key: string): string => {
            const fullKey = `businessDashboard.dashboard.counterManager.toggle.${key}`;
            const result = getTranslation(fullKey, currentLocale);
            return Array.isArray(result) ? result[0] || key : (result as string);
        },
        [currentLocale],
    );

    useEffect(() => {
        const loadStatus = async () => {
            if (externalEnabled !== undefined || !loading) return;

            try {
                const response = await getBusinessCounters(businessId);
                // L2-31 / L2-36: enablement is business.counter_enabled only —
                // inactive counter rows still exist after soft-disable.
                setEnabled(Boolean(response.business?.counter_enabled));
            } catch (error) {
                console.error("Failed to load counter status:", error);
            } finally {
                setLoading(false);
            }
        };

        if (isLocked) {
            setLoading(false);
            return;
        }

        if (businessId) {
            loadStatus().catch((err) => console.error("loadStatus failed:", err));
        }
    }, [businessId, isLocked, externalEnabled, loading]);

    const handleToggle = async (newEnabled: boolean) => {
        setActionLoading(true);
        try {
            // Get current settings to preserve count and prefix
            let currentSettings = {
                counter_enabled: false,
                counter_count: 3,
                counter_prefix: "C",
            };

            try {
                const response = await getBusinessCounters(businessId);
                // L2-31/L2-35: trust business settings; never derive prefix from
                // counter names or enablement from row count.
                if (response.business) {
                    currentSettings = {
                        counter_enabled: Boolean(response.business.counter_enabled),
                        counter_count: Math.max(
                            response.business.counter_count || 3,
                            1,
                        ),
                        counter_prefix: (
                            response.business.counter_prefix || "C"
                        ).trim(),
                    };
                }
            } catch {
                // Use defaults
            }

            // L2-34: require a non-empty trimmed prefix — do not silently
            // substitute "C" over an intentionally blank field.
            const prefix = currentSettings.counter_prefix.trim();
            if (!prefix) {
                toast.error(tString("prefixRequired") || tString("updateError"));
                return;
            }
            const payload = {
                counter_enabled: newEnabled,
                counter_count: Math.max(currentSettings.counter_count || 3, 1),
                counter_prefix: prefix,
            };

            await updateCounterSettings(businessId, payload);

            setEnabled(newEnabled);
            onStatusChange?.(newEnabled);
            setShowConfirmModal(false);
            setShowEnableModal(false);
        } catch (error) {
            console.error("Failed to toggle counter service:", error);
            // surfaceBackendError is pure — its result must be toasted, otherwise
            // the toggle failure is completely silent.
            toast.error(
                surfaceBackendError(error, currentLocale, tString("updateError")),
            );
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
                                className="border border-warm-200 text-ink-700 hover:text-rose-700 hover:border-rose-300 hover:bg-rose-50 px-4 py-2 rounded-lg font-medium transition-colors flex items-center gap-2 whitespace-nowrap"
                            >
                                <Power className="w-4 h-4" />
                                {tString("disable")}
                            </button>

                            {/* Disable Confirmation Modal */}
                            <Modal
                                isOpen={showConfirmModal}
                                onClose={() => setShowConfirmModal(false)}
                            >
                                <ModalContent className="overflow-hidden rounded-3xl border border-warm-200 bg-white shadow-2xl shadow-warm-900/15">
                                    <ModalHeader className="flex flex-col gap-1 border-b border-warm-200/80 bg-warm-50/70 px-6 py-5">
                                        <div className="flex items-center gap-2 text-amber-600">
                                            <AlertTriangle className="w-5 h-5" />
                                            <span className="font-semibold tracking-wide">
                                                {tString("disableModal.title")}
                                            </span>
                                        </div>
                                    </ModalHeader>
                                    <ModalBody className="py-5">
                                        <p className="leading-relaxed text-ink-600">
                                            {tString("disableModal.description")}
                                        </p>
                                    </ModalBody>
                                    <ModalFooter className="border-t border-warm-200/80 bg-warm-50/70 px-6 py-4">
                                        <Button
                                            variant="light"
                                            onPress={() => setShowConfirmModal(false)}
                                            disabled={actionLoading}
                                            radius="full"
                                            className="font-medium text-ink-700 hover:bg-warm-100"
                                        >
                                            {tString("disableModal.cancel")}
                                        </Button>
                                        <Button
                                            onPress={() => handleToggle(false)}
                                            isLoading={actionLoading}
                                            radius="full"
                                            className="bg-rose-600 font-medium text-white hover:bg-rose-700"
                                        >
                                            {tString("disableModal.confirm")}
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
                                icon={Coffee}
                                title={tString("title")}
                                description={tString("description")}
                                features={[
                                    "naming",
                                    "bills",
                                    "honesty",
                                ].map((feature) => ({
                                    title: tString(`features.${feature}.title`),
                                    description: tString(
                                        `features.${feature}.description`,
                                    ),
                                }))}
                                action={
                                    <button
                                        type="button"
                                        onClick={handleEnableClick}
                                        disabled={actionLoading}
                                        className="inline-flex items-center gap-2 rounded-full bg-brand px-4 py-2 text-sm font-medium text-white transition-colors hover:bg-brand-dark disabled:cursor-not-allowed disabled:opacity-50"
                                    >
                                        {tString("enable")}
                                    </button>
                                }
                                footnote={tString("footnote")}
                            />

                            {/* Enable Confirmation Modal */}
                            <Modal
                                isOpen={showEnableModal}
                                onClose={() => setShowEnableModal(false)}
                            >
                                <ModalContent className="overflow-hidden rounded-3xl border border-warm-200 bg-white shadow-2xl shadow-warm-900/15">
                                    <ModalHeader className="flex flex-col gap-1 border-b border-warm-200/80 bg-warm-50/70 px-6 py-5">
                                        <div className="flex items-center gap-2 text-ink-950">
                                            <Coffee className="w-5 h-5" />
                                            <span className="font-semibold tracking-wide">
                                                {tString("enableModal.title")}
                                            </span>
                                        </div>
                                    </ModalHeader>
                                    <ModalBody className="py-5">
                                        <p className="leading-relaxed text-ink-600">
                                            {tString("enableModal.description")}
                                        </p>
                                        <div className="rounded-2xl border border-warm-200 bg-warm-50/70 p-4">
                                            <h4 className="mb-3 font-semibold text-ink-950">
                                                {tString("enableModal.featuresTitle")}
                                            </h4>
                                            <ul className="space-y-2 text-sm text-ink-600">
                                                {(["naming", "bills", "honesty"] as const).map(
                                                    (feature) => (
                                                        <li
                                                            key={feature}
                                                            className="flex items-start gap-2"
                                                        >
                                                            <Check className="mt-0.5 h-4 w-4 flex-shrink-0 text-ink-950" />
                                                            <span>
                                                                {tString(
                                                                    `features.${feature}.title`,
                                                                )}
                                                            </span>
                                                        </li>
                                                    ),
                                                )}
                                            </ul>
                                        </div>
                                    </ModalBody>
                                    <ModalFooter className="border-t border-warm-200/80 bg-warm-50/70 px-6 py-4">
                                        <Button
                                            variant="light"
                                            onPress={() => setShowEnableModal(false)}
                                            disabled={actionLoading}
                                            radius="full"
                                            className="font-medium text-ink-700 hover:bg-warm-100"
                                        >
                                            {tString("enableModal.cancel")}
                                        </Button>
                                        <Button
                                            radius="full"
                                            className={btnPrimaryNextUI}
                                            onPress={() => handleToggle(true)}
                                            isLoading={actionLoading}
                                        >
                                            {tString("enableModal.confirm")}
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
