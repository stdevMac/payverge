"use client";

import React, { useEffect, useState } from "react";
import { FileText, Sparkles } from "lucide-react";
import { motion, AnimatePresence } from "framer-motion";
import PDFDigitizer from "./PDFDigitizer";
import AIWizard from "./AIWizard";
import MenuReviewEditor from "./MenuReviewEditor";
import { ExtractedMenu, GeneratedMenu } from "@/api/business";
import { useSimpleLocale, getTranslation } from "@/i18n/SimpleTranslationProvider";

interface AIMenuOnboardingProps {
    businessId: number;
    onImportComplete: () => void;
    initialTab?: "pdf" | "wizard";
    /** L3-3: true while sanitization review owns Esc (parent must not dismiss). */
    onBlockingOverlayChange?: (blocking: boolean) => void;
}

type View = "selection" | "review";

export default function AIMenuOnboarding({
    businessId,
    onImportComplete,
    initialTab = "pdf",
    onBlockingOverlayChange,
}: AIMenuOnboardingProps) {
    const [view, setView] = useState<View>("selection");
    const [extractedMenu, setExtractedMenu] = useState<ExtractedMenu | null>(null);
    const [selectedTab, setSelectedTab] = useState<"pdf" | "wizard">(initialTab);
    const { locale } = useSimpleLocale();
    const t = (key: string, params?: Record<string, string | number>) => getTranslation(`aiMenuOnboarding.${key}`, locale, params);

    useEffect(() => {
        setSelectedTab(initialTab);
    }, [initialTab]);

    const handleExtracted = (menu: ExtractedMenu) => {
        setExtractedMenu(menu);
        setView("review");
    };

    const handleGeneratedMenu = (generatedMenu: GeneratedMenu) => {
        const menu: ExtractedMenu = {
            restaurant_name: t("generatedMenuName") as string,
            currency: generatedMenu.currency || "$",
            categories: generatedMenu.categories.map((cat) => ({
                name: cat.name,
                items: cat.items.map((item) => ({
                    name: item.name,
                    price: item.price.toString(),
                    description: item.description,
                    category: cat.name,
                    allergens: item.allergens || [],
                    dietary_tags: item.dietary_tags || [],
                    add_ons: item.options?.map((opt) => ({
                        name: opt.name,
                        price: opt.price_change.toString(),
                    })) || [],
                    image_url: item.image,
                })),
            })),
        };
        setExtractedMenu(menu);
        setView("review");
    };

    const handleImport = () => {
        setView("selection");
        setExtractedMenu(null);
        onImportComplete();
    };

    const handleCancel = () => {
        setView("selection");
        setExtractedMenu(null);
    };

    // Review Editor View
    if (view === "review" && extractedMenu) {
        return (
            <motion.div
                initial={{ opacity: 0, y: 20 }}
                animate={{ opacity: 1, y: 0 }}
                exit={{ opacity: 0, y: -20 }}
                transition={{ duration: 0.3, ease: [0.4, 0, 0.2, 1] }}
            >
                <MenuReviewEditor
                    businessId={businessId}
                    extractedMenu={extractedMenu}
                    onImport={handleImport}
                    onCancel={handleCancel}
                    onSanitizationBlockingChange={onBlockingOverlayChange}
                />
            </motion.div>
        );
    }


    const tabs = [
        {
            key: "pdf" as const,
            icon: FileText,
            label: t("tabs.pdfUpload") as string,
        },
        {
            key: "wizard" as const,
            icon: Sparkles,
            label: t("tabs.aiWizard") as string,
        },
    ];

    // Selection View (PDF or AI Wizard tabs) — Pro only.
    return (
        <div className="space-y-4 sm:space-y-6">
            {/* Custom Tab Navigation */}
            <div className="relative">
                <div className="relative rounded-3xl border border-warm-200 bg-white p-1.5 shadow-sm shadow-warm-900/5">
                    <div className="grid grid-cols-2 gap-1.5">
                        {tabs.map((tab) => {
                            const isSelected = selectedTab === tab.key;
                            const Icon = tab.icon;

                            return (
                                <motion.button
                                    key={tab.key}
                                    onClick={() => setSelectedTab(tab.key)}
                                    className={`
                                        relative flex items-center justify-center gap-2 py-3 px-4 rounded-xl
                                        font-medium text-sm sm:text-base transition-colors duration-200
                                        ${isSelected
                                            ? "text-white"
                                            : "text-ink-600 hover:bg-warm-50 hover:text-ink-950"
                                        }
                                    `}
                                    whileHover={{ scale: isSelected ? 1 : 1.02 }}
                                    whileTap={{ scale: 0.98 }}
                                >
                                    {/* Selected background */}
                                    {isSelected && (
                                        <motion.div
                                            layoutId="activeTab"
                                            className="absolute inset-0 rounded-2xl bg-brand shadow-sm shadow-brand/20"
                                            initial={false}
                                            transition={{
                                                type: "spring",
                                                stiffness: 400,
                                                damping: 30,
                                            }}
                                        />
                                    )}

                                    {/* Icon with animation */}
                                    <motion.div
                                        className="relative z-10"
                                        animate={{
                                            scale: isSelected ? [1, 1.1, 1] : 1,
                                        }}
                                        transition={{
                                            duration: 0.3,
                                            ease: "easeOut",
                                        }}
                                    >
                                        <Icon className="w-4 h-4 sm:w-5 sm:h-5" />
                                    </motion.div>

                                    {/* Label */}
                                    <span className="relative z-10 hidden sm:inline">
                                        {tab.label}
                                    </span>
                                    <span className="relative z-10 sm:hidden text-xs">
                                        {t(`tabs.mobile.${tab.key}`)}
                                    </span>
                                </motion.button>
                            );
                        })}
                    </div>
                </div>
            </div>

            {/* Tab Content with Animation */}
            <AnimatePresence mode="wait">
                <motion.div
                    key={selectedTab}
                    initial={{ opacity: 0, y: 10 }}
                    animate={{ opacity: 1, y: 0 }}
                    exit={{ opacity: 0, y: -10 }}
                    transition={{
                        duration: 0.25,
                        ease: [0.4, 0, 0.2, 1],
                    }}
                >
                    {selectedTab === "pdf" ? (
                        <PDFDigitizer
                            businessId={businessId}
                            onExtracted={handleExtracted}
                        />
                    ) : (
                        <AIWizard
                            businessId={businessId}
                            onMenuGenerated={handleGeneratedMenu}
                        />
                    )}
                </motion.div>
            </AnimatePresence>
        </div>
    );
}

