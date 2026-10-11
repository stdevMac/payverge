"use client";

import React, { useState, useEffect, useCallback, useMemo } from "react";
import {
  Button,
  Modal,
  ModalContent,
  ModalHeader,
  ModalBody,
  ModalFooter,
  Input,
  Textarea,
  Chip,
  useDisclosure,
  Select,
  SelectItem,
  Switch,
  Image,
} from "@nextui-org/react";
import { Plus, Package, Pencil, Trash2, Minus, Sparkles } from "lucide-react";
import { businessApi, Bundle, BundleItemRef, MenuCategory, MenuItem } from "../../api/business";
import { formatCurrency as formatBundleCurrency } from "../../api/currency";
import { asDollars } from "@/types/money";
import { useSimpleLocale, getTranslation } from "@/i18n/SimpleTranslationProvider";
import { deriveCurrencyPrefix } from "./delivery/currencyPrefix";
import ImageUpload from "./ImageUpload";
import ConfirmationModal from "./modals/ConfirmationModal";
import { runWithFeedback } from "@/lib/runWithFeedback";
import { tryParseLocaleDecimal } from "@/lib/parseLocaleDecimal";
import { DecimalInput } from "@/components/ui/DecimalInput";
import { filterAvailableMenuItems } from "./menuItemAvailability";
import {
  computeBundleSavings,
  bundleSavingsTone,
  isBundlePriceAtOrAboveRegular,
} from "./bundleSavings";
import toast from "react-hot-toast";
import { PremiumPanel } from "./premium";
import { useSharedMenu } from "@/hooks/useSharedMenu";

interface BundleFormData {
  name: string;
  description: string;
  price: number;
  currency: string;
  image: string;
  is_active: boolean;
  items: BundleItemRef[];
}

interface BundlesManagerProps {
  businessId: number;
}

const parseBundleItems = (items: Bundle["items"]): BundleItemRef[] => {
  if (!items) return [];
  if (Array.isArray(items)) {
    return items.map((item) => ({
      menu_item_id: item.menu_item_id,
      name: item.name,
      quantity: item.quantity || 1,
    }));
  }

  try {
    const parsed = JSON.parse(items);
    if (Array.isArray(parsed)) {
      if (typeof parsed[0] === "string") {
        return parsed.map((id: string) => ({ menu_item_id: id, quantity: 1 }));
      }
      return parsed.map((item: any) => ({
        menu_item_id: item.menu_item_id || item.id || "",
        name: item.name,
        quantity: item.quantity || 1,
      }));
    }
  } catch {
    return [];
  }
  return [];
};

function BundlesContentSkeleton() {
  return (
    <div className="grid grid-cols-1 gap-4 md:grid-cols-2" aria-hidden="true">
      {[0, 1].map((item) => (
        <PremiumPanel key={item} as="article" className="p-5" withTexture={false}>
          <div className="mb-4 flex items-start justify-between gap-3">
            <div className="space-y-3">
              <div className="h-5 w-40 rounded-xl bg-warm-100 motion-safe:animate-pulse" />
              <div className="h-5 w-24 rounded-full bg-brand/10 motion-safe:animate-pulse" />
            </div>
            <div className="h-8 w-20 rounded-xl bg-warm-200/80 motion-safe:animate-pulse" />
          </div>
          <div className="mb-4 h-28 rounded-2xl bg-warm-100 motion-safe:animate-pulse" />
          <div className="h-6 w-28 rounded-xl bg-warm-100 motion-safe:animate-pulse" />
        </PremiumPanel>
      ))}
    </div>
  );
}

export default function BundlesManager({ businessId }: BundlesManagerProps) {
  const { locale } = useSimpleLocale();
  const t = useCallback(
    (key: string, params?: Record<string, string | number>) =>
      getTranslation(
        `businessDashboard.dashboard.menuBuilder.bundlesManager.${key}`,
        locale,
        params,
      ) as string,
    [locale],
  );

  const [bundles, setBundles] = useState<Bundle[]>([]);
  // Menu categories come from the shared React Query cache (§3.7 fix 7) so
  // switching between the Menu / Offers / Bundles sub-tabs doesn't re-fetch the
  // full menu on every mount.
  const { data: sharedMenu = [] } = useSharedMenu(businessId);
  const menu: MenuCategory[] = sharedMenu;
  const [businessCurrency, setBusinessCurrency] = useState<string>("USD");
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [isGeneratingImage, setIsGeneratingImage] = useState(false);
  const [selectedMenuItemId, setSelectedMenuItemId] = useState<string>("");

  const { isOpen, onOpen, onOpenChange, onClose } = useDisclosure();
  const {
    isOpen: isDeleteOpen,
    onOpen: onDeleteOpen,
    onOpenChange: onDeleteOpenChange,
    onClose: onDeleteClose,
  } = useDisclosure();
  const [currentBundle, setCurrentBundle] = useState<Bundle | null>(null);
  const [bundleToDelete, setBundleToDelete] = useState<number | null>(null);
  const [formData, setFormData] = useState<BundleFormData>({
    name: "",
    description: "",
    price: 0,
    currency: businessCurrency,
    image: "",
    is_active: true,
    items: [],
  });
  // L3-17: raw string for the price field — never String(number) on change
  // (decimal separator is destroyed by numeric round-trip mid-keystroke).
  const [priceText, setPriceText] = useState("0");

  // Every menu item, availability included — this is what existing bundle
  // lines are resolved against (L3-43).
  const allMenuItems = useMemo(() => {
    const list: MenuItem[] = [];
    menu.forEach((category) => {
      category.items?.forEach((item) => list.push(item));
    });
    return list;
  }, [menu]);

  // L3-16: only available (non-86'd) items can be composed into new bundles.
  const menuItems = useMemo(
    () => filterAvailableMenuItems(allMenuItems),
    [allMenuItems],
  );

  // L3-43: built from the UNFILTERED list. A bundle saved before an item was
  // 86'd still has to price that line — resolving it against the picker's
  // filtered list showed $0, understated regularTotal, and could flip the
  // rose tone / "no savings" warning on a bundle that does save money.
  const menuItemLookup = useMemo(() => {
    const lookup = new Map<string, MenuItem>();
    allMenuItems.forEach((item) => {
      if (item.id) lookup.set(item.id, item);
      lookup.set(item.name, item);
    });
    return lookup;
  }, [allMenuItems]);

  const regularTotal = useMemo(() => {
    return formData.items.reduce((sum, ref) => {
      const item = menuItemLookup.get(ref.menu_item_id);
      if (!item) return sum;
      return sum + (item.price || 0) * (ref.quantity || 1);
    }, 0);
  }, [formData.items, menuItemLookup]);

  const fetchData = useCallback(async () => {
    await runWithFeedback(
      async () => {
        const [bundlesData, businessData] = await Promise.all([
          businessApi.getBundles(businessId),
          businessApi.getBusiness(businessId).catch(() => null),
        ]);
        setBundles(bundlesData || []);
        const resolvedCurrency = businessData?.default_currency || "USD";
        setBusinessCurrency(resolvedCurrency);
      },
      {
        error: t("messages.fetchError"),
        setBusy: setLoading,
      },
    );
  }, [businessId, t]);

  useEffect(() => {
    if (businessId) {
      fetchData().catch((err) => console.error("fetchData failed:", err));
    }
  }, [businessId, fetchData]);

  const resetForm = () => {
    setFormData({
      name: "",
      description: "",
      price: 0,
      currency: businessCurrency,
      image: "",
      is_active: true,
      items: [],
    });
    setPriceText("0");
    setCurrentBundle(null);
    setSelectedMenuItemId("");
  };

  const handleCreate = () => {
    resetForm();
    onOpen();
  };

  const handleEdit = (bundle: Bundle) => {
    setCurrentBundle(bundle);
    const parsedItems = parseBundleItems(bundle.items);
    const price = bundle.price || 0;
    setFormData({
      name: bundle.name || "",
      description: bundle.description || "",
      price,
      currency: bundle.currency || businessCurrency,
      image: bundle.image || "",
      is_active: bundle.is_active ?? true,
      items: parsedItems,
    });
    setPriceText(String(price));
    onOpen();
  };

  const handleDelete = async (id: number) => {
    setBundleToDelete(id);
    onDeleteOpen();
  };

  const confirmDelete = async () => {
    if (bundleToDelete == null) return;
    const ok = await runWithFeedback(
      async () => {
        await businessApi.deleteBundle(businessId, bundleToDelete);
        setBundles((prev) => prev.filter((bundle) => bundle.id !== bundleToDelete));
        return true;
      },
      {
        success: t("messages.deleteSuccess"),
        error: t("messages.deleteError"),
      },
    );
    // Keep the confirm modal open on failure so the operator can retry, and
    // clear the pending id only on success — otherwise the retry click hits the
    // early-return guard above because bundleToDelete was already nulled.
    if (ok) {
      setBundleToDelete(null);
      onDeleteClose();
    }
  };

  const addSelectedItem = () => {
    if (!selectedMenuItemId) return;
    const selectedItem = menuItemLookup.get(selectedMenuItemId);
    if (!selectedItem) return;

    setFormData((prev) => {
      const itemKey = selectedItem.id || selectedItem.name;
      const existing = prev.items.find((item) => item.menu_item_id === itemKey);
      if (existing) {
        return {
          ...prev,
          items: prev.items.map((item) =>
            item.menu_item_id === itemKey
              ? { ...item, quantity: item.quantity + 1 }
              : item,
          ),
        };
      }
      return {
        ...prev,
        items: [
          ...prev.items,
          {
            menu_item_id: itemKey,
            name: selectedItem.name,
            quantity: 1,
          },
        ],
      };
    });
    setSelectedMenuItemId("");
  };

  const updateItemQuantity = (menuItemId: string, quantity: number) => {
    if (quantity <= 0) {
      setFormData((prev) => ({
        ...prev,
        items: prev.items.filter((item) => item.menu_item_id !== menuItemId),
      }));
      return;
    }
    setFormData((prev) => ({
      ...prev,
      items: prev.items.map((item) =>
        item.menu_item_id === menuItemId ? { ...item, quantity } : item,
      ),
    }));
  };

  const handleGenerateImage = async () => {
    const bundleItemsForPrompt = formData.items.map((item) => {
      const resolved = menuItemLookup.get(item.menu_item_id);
      return {
        name: item.name || resolved?.name || item.menu_item_id,
        quantity: Math.max(item.quantity || 1, 1),
      };
    });

    await runWithFeedback(
      async () => {
        const generated = await businessApi.generateMenuImage(businessId, {
          name: formData.name || t("form.imageDefaultName"),
          description: formData.description || formData.name || t("form.imageDefaultDescription"),
          entity_type: "bundle",
          related_item_names: bundleItemsForPrompt.map((item) => item.name),
          bundle_items: bundleItemsForPrompt,
          bundle_price: asDollars(effectivePrice),
          currency: businessCurrency,
        });
        if (generated.url) {
          setFormData((prev) => ({ ...prev, image: generated.url }));
        }
      },
      {
        error: t("messages.generateImageError"),
        setBusy: setIsGeneratingImage,
      },
    );
  };

  // Live parse of priceText so savings preview and submit see what the
  // operator typed even before blur (L3-17 string state).
  const livePrice = tryParseLocaleDecimal(priceText);
  const effectivePrice =
    livePrice !== null && livePrice >= 0 ? livePrice : formData.price;

  // F19b: surface why the save can't proceed instead of silently no-op'ing.
  const isFormValid =
    formData.name.trim().length > 0 &&
    livePrice !== null &&
    livePrice > 0 &&
    formData.items.length > 0;

  const handleSubmit = async () => {
    if (!isFormValid || livePrice === null) {
      toast.error(t("messages.validationError"));
      return;
    }

    const payload: Bundle = {
      name: formData.name.trim(),
      description: formData.description.trim(),
      price: asDollars(livePrice),
      currency: businessCurrency,
      image: formData.image,
      is_active: formData.is_active,
      items: formData.items,
    };

    const ok = await runWithFeedback(
      async () => {
        if (currentBundle?.id) {
          await businessApi.updateBundle(businessId, currentBundle.id, payload);
        } else {
          await businessApi.createBundle(businessId, payload);
        }
        await fetchData();
        return true;
      },
      {
        success: t("messages.saveSuccess"),
        error: t("messages.saveError"),
        setBusy: setSaving,
      },
    );
    // Keep the modal open on failure so the operator can fix and retry.
    if (ok) {
      onClose();
      resetForm();
    }
  };

  return (
    <div className="space-y-6">
      <div className="flex flex-col items-start justify-between gap-4 sm:flex-row sm:items-center">
        <div>
          <h3 className="text-lg font-semibold text-ink-950">{t("title")}</h3>
          <p className="mt-1 text-sm text-ink-600">{t("subtitle")}</p>
        </div>
        <Button
          className="w-full rounded-xl bg-brand text-white shadow-sm shadow-brand/20 transition hover:bg-brand-dark sm:w-auto"
          startContent={<Plus className="w-4 h-4" />}
          onPress={handleCreate}
        >
          {t("createButton")}
        </Button>
      </div>

      {loading ? (
        <BundlesContentSkeleton />
      ) : bundles.length === 0 ? (
        <PremiumPanel className="p-8 text-center" withTexture={false}>
          <div className="mx-auto mb-4 flex h-12 w-12 items-center justify-center rounded-2xl border border-brand/15 bg-brand/5 shadow-sm shadow-brand/10">
            <Package className="h-5 w-5 text-brand" aria-hidden="true" />
          </div>
          <h4 className="mb-1 font-semibold text-ink-950">{t("empty.title")}</h4>
          <p className="mx-auto mb-5 max-w-md text-sm text-ink-600">{t("empty.subtitle")}</p>
          <Button
            className="rounded-xl bg-brand text-white shadow-sm shadow-brand/20 transition hover:bg-brand-dark"
            onPress={handleCreate}
          >
            {t("createButton")}
          </Button>
        </PremiumPanel>
      ) : (
        <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
          {bundles.map((bundle) => {
            const items = parseBundleItems(bundle.items);
            return (
              <PremiumPanel
                key={bundle.id || `${bundle.name}-${bundle.price}`}
                as="article"
                className="group p-4 transition-transform duration-200 hover:-translate-y-0.5 sm:p-5"
                withTexture={false}
              >
                <div className="flex flex-col items-start justify-between gap-3 sm:flex-row">
                  <div className="min-w-0">
                    <h4 className="truncate text-lg font-semibold text-ink-950">{bundle.name}</h4>
                    <div className="mt-2 flex flex-wrap gap-2">
                      <Chip size="sm" color={bundle.is_active ? "success" : "default"} variant="flat">
                        {bundle.is_active ? t("status.active") : t("status.inactive")}
                      </Chip>
                      <Chip size="sm" variant="bordered">
                        {t("itemsCount", { count: items.length })}
                      </Chip>
                    </div>
                  </div>
                  <div className="flex gap-2 w-full sm:w-auto justify-end">
                    <Button
                      isIconOnly
                      size="sm"
                      variant="light"
                      aria-label={t("aria.edit", { name: bundle.name })}
                      className="rounded-xl text-ink-600 hover:bg-warm-100 hover:text-ink-950"
                      onPress={() => handleEdit(bundle)}
                    >
                      <Pencil className="w-4 h-4" />
                    </Button>
                    <Button
                      isIconOnly
                      size="sm"
                      color="danger"
                      variant="light"
                      aria-label={t("aria.delete", { name: bundle.name })}
                      className="rounded-xl hover:bg-rose-50"
                      onPress={() => bundle.id && handleDelete(bundle.id)}
                    >
                      <Trash2 className="w-4 h-4" />
                    </Button>
                  </div>
                </div>
                <div className="pt-4">
                  {bundle.image && (
                    <div className="mb-4 overflow-hidden rounded-2xl border border-warm-200/80 bg-warm-50">
                      <Image src={bundle.image} alt={bundle.name} className="w-full h-36 object-cover" />
                    </div>
                  )}
                  <p className="mb-4 text-sm leading-6 text-ink-600">{bundle.description}</p>
                  <div className="inline-flex items-center rounded-xl border border-brand/15 bg-brand/5 px-3 py-2 text-lg font-bold text-brand-dark">
                    {formatBundleCurrency(bundle.price, bundle.currency || businessCurrency)}
                  </div>
                </div>
              </PremiumPanel>
            );
          })}
        </div>
      )}

      <Modal isOpen={isOpen} onOpenChange={onOpenChange} size="3xl">
        <ModalContent>
          {() => (
            <>
              <ModalHeader>{currentBundle ? t("modal.editTitle") : t("modal.createTitle")}</ModalHeader>
              <ModalBody className="space-y-3">
                <Input
                  label={t("form.nameLabel")}
                  placeholder={t("form.namePlaceholder")}
                  value={formData.name}
                  onValueChange={(val) => setFormData({ ...formData, name: val })}
                  variant="bordered"
                />
                <Textarea
                  label={t("form.descriptionLabel")}
                  placeholder={t("form.descriptionPlaceholder")}
                  value={formData.description}
                  onValueChange={(val) => setFormData({ ...formData, description: val })}
                  variant="bordered"
                />

                <div className="space-y-3">
                  <ImageUpload
                    businessId={businessId}
                    type="bundle"
                    title={t("form.imageLabel")}
                    description={t("form.imageDescription")}
                    currentImage={formData.image}
                    onImageUploaded={(imageUrl) => setFormData((prev) => ({ ...prev, image: imageUrl }))}
                  />
                  <Button
                    variant="flat"
                    startContent={<Sparkles className="w-4 h-4" />}
                    isLoading={isGeneratingImage}
                    onPress={handleGenerateImage}
                  >
                    {t("form.generateImage")}
                  </Button>
                </div>

                <div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
                  <DecimalInput
                    // L3-17: string state + blur-parse. Numeric round-trip
                    // on change turned typed 12.50 into 1250.
                    label={t("form.priceLabel")}
                    value={priceText}
                    onValueChange={setPriceText}
                    onParsedChange={(n) => {
                      if (n !== null && n >= 0) {
                        setFormData((prev) => ({ ...prev, price: n }));
                      }
                    }}
                    min={0}
                    variant="bordered"
                    startContent={<div className="pointer-events-none flex items-center"><span className="text-default-400 text-small">{deriveCurrencyPrefix(businessCurrency, "symbol")}</span></div>}
                    data-testid="bundle-price-input"
                    isInvalid={priceText.trim() !== "" && (livePrice === null || livePrice <= 0)}
                  />
                </div>

                <div className="space-y-2">
                  <div className="flex gap-2">
                    <Select
                      label={t("form.addItemLabel")}
                      selectedKeys={selectedMenuItemId ? [selectedMenuItemId] : []}
                      onChange={(e) => setSelectedMenuItemId(e.target.value)}
                      variant="bordered"
                    >
                      {menuItems.map((item) => (
                        <SelectItem key={item.id || item.name} value={item.id || item.name}>
                          {item.name}
                        </SelectItem>
                      ))}
                    </Select>
                    <Button color="primary" variant="flat" onPress={addSelectedItem} className="mt-6">
                      {t("form.addButton")}
                    </Button>
                  </div>

                  {formData.items.length > 0 ? (
                    <div className="space-y-2 border rounded-lg p-3">
                      {formData.items.map((bundleItem) => {
                        const item = menuItemLookup.get(bundleItem.menu_item_id);
                        const name = bundleItem.name || item?.name || bundleItem.menu_item_id;
                        const price = item?.price || 0;
                        return (
                          <div key={bundleItem.menu_item_id} className="flex items-center justify-between gap-2">
                            <div>
                              <p className="text-sm font-medium">{name}</p>
                              <p className="text-xs text-ink-700">{t("form.eachPrice", { value: price.toFixed(2) })}</p>
                            </div>
                            <div className="flex items-center gap-1">
                              <Button
                                isIconOnly
                                size="sm"
                                variant="light"
                                aria-label={t("aria.decreaseQuantity", { name })}
                                onPress={() => updateItemQuantity(bundleItem.menu_item_id, bundleItem.quantity - 1)}
                              >
                                <Minus className="w-3 h-3" />
                              </Button>
                              <span className="w-6 text-center text-sm">{bundleItem.quantity}</span>
                              <Button
                                isIconOnly
                                size="sm"
                                variant="light"
                                aria-label={t("aria.increaseQuantity", { name })}
                                onPress={() => updateItemQuantity(bundleItem.menu_item_id, bundleItem.quantity + 1)}
                              >
                                <Plus className="w-3 h-3" />
                              </Button>
                            </div>
                          </div>
                        );
                      })}
                    </div>
                  ) : (
                    <p className="text-xs text-ink-700">{t("form.itemSelectionPlaceholder")}</p>
                  )}
                </div>

                <div className="bg-warm-50 rounded-lg p-3 text-sm">
                  <div className="flex justify-between">
                    <span>{t("summary.regularTotal")}</span>
                    <span>{formatBundleCurrency(regularTotal, businessCurrency)}</span>
                  </div>
                  <div className="flex justify-between font-semibold">
                    <span>{t("summary.bundlePrice")}</span>
                    <span>{formatBundleCurrency(effectivePrice, businessCurrency)}</span>
                  </div>
                  {(() => {
                    // L3-18: keep signed delta; tone by sign (no Math.max clamp).
                    const savings = computeBundleSavings(
                      regularTotal,
                      effectivePrice,
                    );
                    const tone = bundleSavingsTone(savings);
                    const toneClass =
                      tone === "positive"
                        ? "text-emerald-600"
                        : tone === "negative"
                          ? "text-rose-600"
                          : "text-ink-600";
                    return (
                      <div className={`flex justify-between ${toneClass}`}>
                        <span>{t("summary.customerSaves")}</span>
                        <span>
                          {formatBundleCurrency(savings, businessCurrency)}
                        </span>
                      </div>
                    );
                  })()}
                  {isBundlePriceAtOrAboveRegular(regularTotal, effectivePrice) && (
                    <p
                      className="mt-2 text-xs text-amber-700"
                      data-testid="bundle-no-savings-warning"
                    >
                      {t("messages.noSavingsWarning")}
                    </p>
                  )}
                </div>

                <Switch
                  isSelected={formData.is_active}
                  onValueChange={(value) => setFormData({ ...formData, is_active: value })}
                >
                  {t("form.activeToggle")}
                </Switch>
              </ModalBody>
              <ModalFooter>
                <Button variant="light" onPress={onClose}>{t("buttons.cancel")}</Button>
                <Button
                  color="primary"
                  onPress={handleSubmit}
                  isLoading={saving}
                  isDisabled={!isFormValid || saving}
                >
                  {currentBundle ? t("buttons.update") : t("buttons.create")}
                </Button>
              </ModalFooter>
            </>
          )}
        </ModalContent>
      </Modal>
      <ConfirmationModal
        isOpen={isDeleteOpen}
        onOpenChange={onDeleteOpenChange}
        title={t("title")}
        description={t("confirmDelete")}
        cancelLabel={t("buttons.cancel")}
        confirmLabel={t("buttons.delete")}
        isDanger
        onConfirm={() => {
          void confirmDelete();
        }}
      />
    </div>
  );
}
