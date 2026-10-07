"use client";

import React, { useState, useMemo, useEffect } from "react";
import { createPortal } from "react-dom";
import {
    Button,
    Card,
    CardBody,
    Input,
    Textarea,
    Chip,
    Accordion,
    AccordionItem,
    Image,
    Tooltip,
    Modal,
    ModalContent,
    ModalHeader,
    ModalBody,
    ModalFooter,
    useDisclosure,
} from "@nextui-org/react";
import { Trash2, Plus, X, Sparkles, ImageIcon, Save, AlertCircle } from "lucide-react";
import { motion, AnimatePresence } from "framer-motion";
import { ExtractedMenu, MenuCategory, MenuItem, MenuSanitizeReport, importExtractedMenu, regenerateMenuItemImage } from "@/api/business";
import { asDollars } from "@/types/money";
import { parseLocaleDecimal } from "@/lib/parseLocaleDecimal";
import { mapExtractedMenu } from "./MenuReviewEditor.transform";
import { useSimpleLocale, getTranslation } from "@/i18n/SimpleTranslationProvider";
import { errMessage } from "@/utils/apiError";
import { allergenDisplayName } from "@/utils/allergenLabel";
import MenuSanitizationReview from "./MenuSanitizationReview";

interface MenuReviewEditorProps {
    businessId: number;
    extractedMenu: ExtractedMenu;
    onImport: () => void;
    onCancel: () => void;
    /** L3-3: parent AI modal must not dismiss while this is true. */
    onSanitizationBlockingChange?: (blocking: boolean) => void;
}

export default function MenuReviewEditor({
    businessId,
    extractedMenu,
    onImport,
    onCancel,
    onSanitizationBlockingChange,
}: MenuReviewEditorProps) {
    const { locale } = useSimpleLocale();
    const t = (key: string, params?: Record<string, string | number>): string => {
        const value = getTranslation(`aiMenuOnboarding.reviewEditor.${key}`, locale, params);
        return Array.isArray(value) ? value[0] ?? key : value;
    };
    const allergenT = (key: string): string => {
        const value = getTranslation(key, locale);
        return Array.isArray(value) ? value[0] ?? key : value;
    };
    const fieldIdentity = (
        name: string,
        fallbackKey: "unnamedItem" | "unnamedCategory",
    ): string => name.trim() || t(fallbackKey);

    const [categories, setCategories] = useState<MenuCategory[]>(() => mapExtractedMenu(extractedMenu));

    const [_editingItem, _setEditingItem] = useState<{ catIndex: number; itemIndex: number } | null>(null);
    const [isImporting, setIsImporting] = useState(false);
    const [regeneratingImage, setRegeneratingImage] = useState<{ catIndex: number; itemIndex: number } | null>(null);
    const [error, setError] = useState<string | null>(null);
    const [sanitizationReview, setSanitizationReview] = useState<MenuSanitizeReport | null>(null);

    // L3-3: tell the parent AI modal to ignore Esc while review owns focus.
    useEffect(() => {
        onSanitizationBlockingChange?.(sanitizationReview !== null);
        return () => onSanitizationBlockingChange?.(false);
    }, [sanitizationReview, onSanitizationBlockingChange]);

    const { isOpen: isDeleteOpen, onOpen: onDeleteOpen, onClose: onDeleteClose } = useDisclosure();
    const [deleteTarget, setDeleteTarget] = useState<{ type: 'category' | 'item'; catIndex: number; itemIndex?: number } | null>(null);

    const totalItems = useMemo(() =>
        categories.reduce((sum, cat) => sum + cat.items.length, 0),
        [categories]
    );

    const handleCategoryNameChange = (catIndex: number, name: string) => {
        setCategories((prev) => {
            const updated = [...prev];
            updated[catIndex] = { ...updated[catIndex], name };
            return updated;
        });
    };

    const handleItemChange = (catIndex: number, itemIndex: number, field: keyof MenuItem, value: any) => {
        setCategories((prev) => {
            const updated = [...prev];
            const items = [...updated[catIndex].items];
            items[itemIndex] = { ...items[itemIndex], [field]: value };
            updated[catIndex] = { ...updated[catIndex], items };
            return updated;
        });
    };

    const handleDeleteCategory = (catIndex: number) => {
        setDeleteTarget({ type: 'category', catIndex });
        onDeleteOpen();
    };

    const handleDeleteItem = (catIndex: number, itemIndex: number) => {
        setDeleteTarget({ type: 'item', catIndex, itemIndex });
        onDeleteOpen();
    };

    const confirmDelete = () => {
        if (!deleteTarget) return;

        if (deleteTarget.type === 'category') {
            setCategories((prev) => prev.filter((_, i) => i !== deleteTarget.catIndex));
        } else if (deleteTarget.itemIndex !== undefined) {
            setCategories((prev) => {
                const updated = [...prev];
                updated[deleteTarget.catIndex] = {
                    ...updated[deleteTarget.catIndex],
                    items: updated[deleteTarget.catIndex].items.filter((_, i) => i !== deleteTarget.itemIndex),
                };
                return updated;
            });
        }
        onDeleteClose();
    };

    const handleAddItem = (catIndex: number) => {
        setCategories((prev) => {
            const updated = [...prev];
            const newItem: MenuItem = {
                id: `new-${Math.random().toString(36).slice(2)}`,
                name: t("newItem") as string,
                description: "",
                price: asDollars(0),
                is_available: true,
                allergens: [],
                dietary_tags: [],
                options: [],
            };
            updated[catIndex] = {
                ...updated[catIndex],
                items: [...updated[catIndex].items, newItem],
            };
            return updated;
        });
    };

    const handleAddCategory = () => {
        setCategories((prev) => [
            ...prev,
            { name: t("newCategory") as string, description: "", items: [] },
        ]);
    };

    const handleRegenerateImage = async (catIndex: number, itemIndex: number) => {
        const item = categories[catIndex].items[itemIndex];
        setRegeneratingImage({ catIndex, itemIndex });

        try {
            const result = await regenerateMenuItemImage(
                businessId,
                item.name,
                item.description
            );
            handleItemChange(catIndex, itemIndex, 'image', result.url);
            handleItemChange(catIndex, itemIndex, 'images', [result.url]);
        } catch (err: unknown) {
            setError(errMessage(err) || (t("errors.generateImage") as string));
        } finally {
            setRegeneratingImage(null);
        }
    };

    const handleImport = async (confirmSanitization = false) => {
        setIsImporting(true);
        setError(null);

        try {
            const response = await importExtractedMenu(
                businessId,
                categories,
                extractedMenu.currency,
                confirmSanitization,
            );
            if (response.requires_confirmation) {
                setSanitizationReview(response.sanitization);
                return;
            }
            setSanitizationReview(null);
            onImport();
        } catch (err: unknown) {
            setError(errMessage(err) || (t("errors.importMenu") as string));
        } finally {
            setIsImporting(false);
        }
    };

    return (
        <div className="space-y-8 max-w-5xl mx-auto pb-20">
            {/* Header */}
            <motion.div
                initial={{ opacity: 0, y: -20 }}
                animate={{ opacity: 1, y: 0 }}
                className="sticky top-0 z-30 flex flex-col items-center justify-between gap-4 rounded-b-2xl border-b border-warm-200 bg-warm-50/90 py-4 backdrop-blur-md sm:flex-row"
            >
                <div className="text-center sm:text-left">
                    <h2 className="text-2xl font-bold text-brand">
                        {t("title")}
                    </h2>
                    <p className="text-sm font-medium text-ink-600">
                        {extractedMenu.restaurant_name} • {categories.length} {t("categories")} • {totalItems} {t("items")}
                    </p>
                </div>
                <div className="flex gap-3 w-full sm:w-auto">
                    <Button
                        variant="flat"
                        onPress={onCancel}
                        className="flex-1 sm:flex-none h-11 rounded-xl"
                    >
                        {t("cancel")}
                    </Button>
                    <Button
                        color="primary"
                        onPress={() => handleImport(false)}
                        isLoading={isImporting}
                        isDisabled={sanitizationReview !== null}
                        className="flex-1 sm:flex-none h-11 rounded-xl bg-brand shadow-lg shadow-brand/20 font-bold"
                        startContent={!isImporting && <Save className="w-4 h-4" />}
                    >
                        {t("import")}
                    </Button>
                </div>
            </motion.div>

            {/* Error */}
            <AnimatePresence>
                {error && (
                    <motion.div
                        initial={{ opacity: 0, height: 0 }}
                        animate={{ opacity: 1, height: "auto" }}
                        exit={{ opacity: 0, height: 0 }}
                    >
                        <Card className="border-rose-200 bg-rose-50 shadow-sm">
                            <CardBody className="p-4 flex flex-row items-center gap-3">
                                <AlertCircle className="w-5 h-5 text-rose-600" />
                                <div className="flex-1">
                                    <span className="font-medium text-rose-700">{error}</span>
                                </div>
                                <Button isIconOnly size="sm" variant="light" aria-label={t("dismissError")} onPress={() => setError(null)}>
                                    <X className="w-4 h-4" />
                                </Button>
                            </CardBody>
                        </Card>
                    </motion.div>
                )}
            </AnimatePresence>

            {/* L3-3: portal above the parent AI modal so Esc / focus stay on
                the review; parent is non-dismissable while this is mounted. */}
            {sanitizationReview &&
                typeof document !== "undefined" &&
                createPortal(
                    <div
                        className="fixed inset-0 z-[200] flex items-center justify-center bg-ink-950/60 p-4 backdrop-blur-sm"
                        data-testid="menu-sanitization-portal"
                    >
                        <div className="w-full max-w-3xl">
                            <MenuSanitizationReview
                                report={sanitizationReview}
                                translate={t}
                                onConfirm={() => handleImport(true)}
                                onBack={() => setSanitizationReview(null)}
                                isConfirming={isImporting}
                            />
                        </div>
                    </div>,
                    document.body,
                )}

            {/* Categories */}
            <Accordion
                selectionMode="multiple"
                defaultExpandedKeys={categories.map((_, i) => i.toString())}
                variant="splitted"
                className="px-0"
                itemClasses={{
                    base: "mb-4 border border-warm-200/90 bg-white shadow-sm shadow-warm-900/5 backdrop-blur-xl",
                    title: "font-bold text-lg text-ink-950",
                    trigger: "py-4",
                    content: "pb-6 px-4"
                }}
            >
                {categories.map((category, catIndex) => (
                    <AccordionItem
                        key={catIndex}
                        aria-label={category.name}
                        title={
                            <div className="flex items-center justify-between w-full pr-4 gap-4">
                                <div className="flex items-center gap-4 flex-1">
                                    <Input
                                        value={category.name}
                                        size="sm"
                                        variant="underlined"
                                        className="max-w-[250px] font-bold text-lg"
                                        aria-label={t("categoryNameAria", {
                                            name: fieldIdentity(category.name, "unnamedCategory"),
                                        })}
                                        onPointerDown={(e) => e.stopPropagation()}
                                        onChange={(e) => handleCategoryNameChange(catIndex, e.target.value)}
                                        classNames={{
                                            input: "font-bold"
                                        }}
                                    />
                                    <Chip size="sm" variant="flat" className="bg-brand/10 font-bold text-brand">
                                        {category.items.length} {t("items")}
                                    </Chip>
                                </div>
                                <Button
                                    isIconOnly
                                    size="sm"
                                    variant="light"
                                    color="danger"
                                    aria-label={t("deleteCategory")}
                                    onPointerDown={(e) => e.stopPropagation()}
                                    onPress={() => handleDeleteCategory(catIndex)}
                                    className="opacity-0 group-hover:opacity-100 transition-opacity"
                                >
                                    <Trash2 className="w-4 h-4" />
                                </Button>
                            </div>
                        }
                    >
                        <div className="space-y-4">
                            <AnimatePresence mode="popLayout">
                                {category.items.map((item, itemIndex) => (
                                    <motion.div
                                        key={item.id || itemIndex}
                                        initial={{ opacity: 0, scale: 0.98 }}
                                        animate={{ opacity: 1, scale: 1 }}
                                        exit={{ opacity: 0, scale: 0.95 }}
                                        transition={{ duration: 0.2, delay: itemIndex * 0.05 }}
                                    >
                                        <Card className="border border-warm-200/90 bg-white/75 shadow-none transition-colors duration-300 hover:border-brand/30 hover:bg-brand/5">
                                            <CardBody className="p-4 sm:p-5">
                                                <div className="flex flex-col sm:flex-row gap-6">
                                                    {/* Image Section */}
                                                    <div className="relative w-full sm:w-28 sm:h-28 aspect-square flex-shrink-0 group">
                                                        {item.image ? (
                                                            <div className="relative overflow-hidden rounded-2xl w-full h-full shadow-md">
                                                                <Image
                                                                    src={item.image}
                                                                    alt={item.name}
                                                                    className="w-full h-full object-cover transform scale-100 group-hover:scale-110 transition-transform duration-500"
                                                                />
                                                                <div className="absolute inset-0 bg-black/5 opacity-0 group-hover:opacity-100 transition-opacity" />
                                                            </div>
                                                        ) : (
                                                            <div className="flex h-full w-full items-center justify-center rounded-2xl border-2 border-dashed border-warm-200 bg-warm-100">
                                                                <ImageIcon className="h-8 w-8 text-ink-400" />
                                                            </div>
                                                        )}

                                                        <Tooltip content={t("generateAiImageFor", { name: fieldIdentity(item.name, "unnamedItem") })}>
                                                            <Button
                                                                isIconOnly
                                                                size="sm"
                                                                variant="solid"
                                                                aria-label={t("generateAiImageFor", {
                                                                    name: fieldIdentity(item.name, "unnamedItem"),
                                                                })}
                                                                className="absolute -bottom-2 -right-2 z-20 bg-white text-brand shadow-xl shadow-brand/20 transition-transform hover:scale-110"
                                                                isLoading={regeneratingImage?.catIndex === catIndex && regeneratingImage?.itemIndex === itemIndex}
                                                                onPress={() => handleRegenerateImage(catIndex, itemIndex)}
                                                            >
                                                                <Sparkles className="w-4 h-4" />
                                                            </Button>
                                                        </Tooltip>
                                                    </div>

                                                    {/* Details Section */}
                                                    <div className="flex-1 space-y-4">
                                                        <div className="flex flex-col sm:flex-row items-start justify-between gap-4">
                                                            <Input
                                                                value={item.name}
                                                                variant="underlined"
                                                                className="font-bold text-lg flex-1"
                                                                aria-label={t("itemNameAria", {
                                                                    name: fieldIdentity(item.name, "unnamedItem"),
                                                                })}
                                                                onChange={(e) => handleItemChange(catIndex, itemIndex, 'name', e.target.value)}
                                                                classNames={{
                                                                    input: "font-bold text-lg"
                                                                }}
                                                            />
                                                            <Input
                                                                type="text"
                                                                inputMode="decimal"
                                                                value={String(item.price ?? "")}
                                                                aria-label={t("itemPriceAria", {
                                                                    name: fieldIdentity(item.name, "unnamedItem"),
                                                                })}
                                                                variant="bordered"
                                                                size="sm"
                                                                className="w-32"
                                                                startContent={
                                                                    <span className="text-sm font-bold text-ink-500">
                                                                        {extractedMenu.currency}
                                                                    </span>
                                                                }
                                                                classNames={{ input: "font-bold text-lg text-brand" }}
                                                                onValueChange={(val) =>
                                                                    handleItemChange(
                                                                        catIndex,
                                                                        itemIndex,
                                                                        'price',
                                                                        asDollars(parseLocaleDecimal(val)),
                                                                    )
                                                                }
                                                            />
                                                        </div>

                                                        <div className="relative">
                                                            <Textarea
                                                                value={item.description}
                                                                variant="flat"
                                                                size="sm"
                                                                minRows={2}
                                                                placeholder={t("descriptionPlaceholder") as string}
                                                                aria-label={t("itemDescriptionAria", {
                                                                    name: fieldIdentity(item.name, "unnamedItem"),
                                                                })}
                                                                className="text-sm"
                                                                onChange={(e) => handleItemChange(catIndex, itemIndex, 'description', e.target.value)}
                                                                classNames={{
                                                                    input: "bg-transparent",
                                                                    inputWrapper: "bg-warm-50/70"
                                                                }}
                                                            />
                                                        </div>

                                                        {/* Allergens and Tags */}
                                                        <div className="flex flex-wrap gap-2">
                                                            <AnimatePresence>
                                                                {item.allergens?.map((allergen, i) => (
                                                                    <motion.div
                                                                        key={i}
                                                                        initial={{ opacity: 0, scale: 0.8 }}
                                                                        animate={{ opacity: 1, scale: 1 }}
                                                                        exit={{ opacity: 0, scale: 0.8 }}
                                                                    >
                                                                        <Chip
                                                                            size="sm"
                                                                            variant="flat"
                                                                            color="warning"
                                                                            onClose={() => {
                                                                                const newAllergens = item.allergens?.filter((_, idx) => idx !== i);
                                                                                handleItemChange(catIndex, itemIndex, 'allergens', newAllergens);
                                                                            }}
                                                                            className="bg-amber-100 font-medium text-amber-700"
                                                                        >
                                                                            {allergenDisplayName(allergen, allergenT, locale)}
                                                                        </Chip>
                                                                    </motion.div>
                                                                ))}
                                                            </AnimatePresence>
                                                        </div>
                                                    </div>

                                                    {/* Row Actions */}
                                                    <div className="flex sm:flex-col justify-end gap-2">
                                                        <Button
                                                            isIconOnly
                                                            size="sm"
                                                            variant="light"
                                                            color="danger"
                                                            aria-label={t("deleteItem")}
                                                            onPress={() => handleDeleteItem(catIndex, itemIndex)}
                                                            className="rounded-xl hover:bg-rose-50"
                                                        >
                                                            <Trash2 className="w-5 h-5" />
                                                        </Button>
                                                    </div>
                                                </div>
                                            </CardBody>
                                        </Card>
                                    </motion.div>
                                ))}
                            </AnimatePresence>

                            {/* Add Item Button */}
                            <motion.div whileHover={{ scale: 1.01 }} whileTap={{ scale: 0.99 }}>
                                <Button
                                    variant="flat"
                                    className="h-12 w-full rounded-xl border-2 border-dashed border-brand/20 bg-brand/10 font-bold text-brand hover:border-brand/50"
                                    startContent={<Plus className="w-5 h-5" />}
                                    onPress={() => handleAddItem(catIndex)}
                                >
                                    {t("addItem")}
                                </Button>
                            </motion.div>
                        </div>
                    </AccordionItem>
                ))}
            </Accordion>

            {/* Add Category */}
            <motion.div
                initial={{ opacity: 0 }}
                animate={{ opacity: 1 }}
                className="flex justify-center pt-4"
            >
                <Button
                    variant="bordered"
                    size="lg"
                    className="h-16 w-full rounded-2xl border-2 border-dashed border-warm-300 font-bold text-ink-700 transition-all hover:border-brand hover:text-brand"
                    startContent={<Plus className="w-6 h-6" />}
                    onPress={handleAddCategory}
                >
                    {t("addCategory")}
                </Button>
            </motion.div>

            {/* Delete Confirmation Modal */}
            <Modal
                isOpen={isDeleteOpen}
                onClose={onDeleteClose}
                backdrop="blur"
                classNames={{
                    base: "rounded-3xl border border-warm-200 bg-white/95 backdrop-blur-xl",
                }}
            >
                <ModalContent>
                    <ModalHeader className="text-2xl font-bold">{t("confirmDelete")}</ModalHeader>
                    <ModalBody className="text-lg text-ink-600">
                        {t("confirmDeleteMessage", {
                            type: deleteTarget?.type
                                ? t(`deleteTypes.${deleteTarget.type}`)
                                : "",
                        })}
                    </ModalBody>
                    <ModalFooter className="gap-3">
                        <Button variant="flat" onPress={onDeleteClose} className="h-12 rounded-xl px-6 font-bold">
                            {t("cancel")}
                        </Button>
                        <Button color="danger" onPress={confirmDelete} className="h-12 rounded-xl px-6 font-bold shadow-lg shadow-rose-500/20">
                            {t("deleteButton")}
                        </Button>
                    </ModalFooter>
                </ModalContent>
            </Modal>
        </div>
    );
}
