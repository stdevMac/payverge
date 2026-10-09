"use client";

import React, { useState, useEffect, useCallback, useMemo, useRef } from "react";
import {
  Button,
  Modal,
  ModalContent,
  ModalHeader,
  ModalBody,
  ModalFooter,
  Input,
  Select,
  SelectItem,
  Autocomplete,
  AutocompleteItem,
  AutocompleteSection,
  Textarea,
  Chip,
  useDisclosure,
  Switch,
} from "@nextui-org/react";
import { Plus, Tag, Pencil, Trash2, Calendar, Sparkles, AlertTriangle, ImageIcon } from "lucide-react";
import { businessApi, Offer, Bundle, getBusiness } from "../../api/business";
import { formatCurrency as formatCurrencyIntl } from "../../api/currency";
import { useSharedMenu } from "@/hooks/useSharedMenu";
import { useSimpleLocale, getTranslation } from "@/i18n/SimpleTranslationProvider";
import ImageUpload from "./ImageUpload";
import ConfirmationModal from "./modals/ConfirmationModal";
import { runWithFeedback } from "@/lib/runWithFeedback";
import { localDateKey } from "@/lib/localDate";
import { parseLocaleDecimal } from "@/lib/parseLocaleDecimal";
import {
  isOfferDiscountValid,
  offerDiscountErrorKey,
} from "./offerDiscountValidation";
import { surfaceBackendError } from "@/utils/localizedError";
import toast from "react-hot-toast";
import { PremiumPanel } from "./premium";
import {
  OFFER_EDIT_MODAL_TEST_ID,
  bindOfferModalEscape,
  dismissOwnedPicker,
  hasExpandedPickerOwnedBy,
  shouldCloseOfferModalOnEscape,
} from "./offerModalEscape";
import {
  applyOfferScopeChange,
  applyOfferTargetSelection,
} from "./offerScopeChange";
import { offerManualActive, offerStatusChip } from "./offerStatusChip";

interface OfferFormData {
  name: string;
  description: string;
  image: string;
  code: string;
  discount_type: "percentage" | "fixed";
  /**
   * L3-42: RAW STRING while editing (S-5 primitive). Storing a number here and
   * re-rendering `String(number)` ate the decimal separator — "10." parsed to
   * 10, so the next keystroke produced "105". Parsed at validation/submit only.
   */
  discount_value: string;
  is_active: boolean;
  start_date: string;
  end_date: string;
  weekday_mask: number;
  start_time: string;
  end_time: string;
  applicable_to: "all" | "category" | "item" | "bundle";
  target_id: string;
}

const WEEKDAYS = [
  "sunday",
  "monday",
  "tuesday",
  "wednesday",
  "thursday",
  "friday",
  "saturday",
] as const;

const minuteToTime = (minute?: number) => {
  if (minute === undefined || minute < 0 || minute > 1439) return "";
  return `${String(Math.floor(minute / 60)).padStart(2, "0")}:${String(minute % 60).padStart(2, "0")}`;
};

const timeToMinute = (value: string) => {
  if (!value) return undefined;
  const [hours, minutes] = value.split(":").map(Number);
  if (!Number.isInteger(hours) || !Number.isInteger(minutes)) return undefined;
  return hours * 60 + minutes;
};

interface OffersManagerProps {
  businessId: number;
}

const toDateInput = (value?: string) => {
  if (!value) return "";
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return "";
  // Local date key, not the UTC day — otherwise a stored offer date near
  // midnight shows the wrong day in the date input for non-UTC operators.
  return localDateKey(date);
};

/** Human-friendly schedule cue: prefer "Ends in N days" over raw ISO (#151). */
function offerScheduleCue(
  endDate: string | undefined,
  t: (key: string, params?: Record<string, string | number>) => string,
): string | null {
  if (!endDate) return null;
  const end = new Date(endDate);
  if (Number.isNaN(end.getTime())) return null;
  const today = new Date();
  today.setHours(0, 0, 0, 0);
  const endDay = new Date(end);
  endDay.setHours(0, 0, 0, 0);
  const days = Math.round((endDay.getTime() - today.getTime()) / 86_400_000);
  if (days < 0) return t("ended");
  if (days === 0) return t("endsToday");
  return t("endsInDays", { days });
}

function OfferCardImage({
  src,
  name,
  addPhotoLabel,
}: {
  src?: string;
  name: string;
  addPhotoLabel: string;
}) {
  const [failed, setFailed] = React.useState(false);
  React.useEffect(() => {
    setFailed(false);
  }, [src]);
  // No empty grey slab when the image is missing or broken (#151).
  if (!src || failed) {
    return (
      <div
        className="mb-4 flex h-20 items-center justify-center gap-2 rounded-2xl border border-dashed border-warm-300 bg-warm-50/80 text-ink-500"
        data-testid="offer-card-image-placeholder"
      >
        <ImageIcon className="h-4 w-4" aria-hidden="true" />
        <span className="text-xs font-medium">{addPhotoLabel}</span>
      </div>
    );
  }
  return (
    <div className="mb-4 overflow-hidden rounded-2xl border border-warm-200/80 bg-warm-50">
      {/* eslint-disable-next-line @next/next/no-img-element, jsx-a11y/no-noninteractive-element-interactions -- offer URLs are arbitrary CDN paths; onError is a media load fallback, not a user interaction */}
      <img
        src={src}
        alt={name}
        className="h-36 w-full object-cover"
        onError={() => setFailed(true)}
        data-testid="offer-card-image"
      />
    </div>
  );
}

function OffersContentSkeleton() {
  return (
    <div className="grid grid-cols-1 gap-4 md:grid-cols-2" aria-hidden="true">
      {[0, 1].map((item) => (
        <PremiumPanel key={item} as="article" className="p-5" withTexture={false}>
          <div className="mb-4 flex items-start justify-between gap-3">
            <div className="space-y-3">
              <div className="h-5 w-40 rounded-xl bg-warm-100 motion-safe:animate-pulse" />
              <div className="h-5 w-28 rounded-full bg-brand/10 motion-safe:animate-pulse" />
            </div>
            <div className="h-8 w-20 rounded-xl bg-warm-200/80 motion-safe:animate-pulse" />
          </div>
          <div className="mb-4 h-28 rounded-2xl bg-warm-100 motion-safe:animate-pulse" />
          <div className="space-y-2">
            <div className="h-3 w-full rounded-full bg-warm-200/80 motion-safe:animate-pulse" />
            <div className="h-3 w-2/3 rounded-full bg-warm-200/80 motion-safe:animate-pulse" />
          </div>
        </PremiumPanel>
      ))}
    </div>
  );
}

export default function OffersManager({ businessId }: OffersManagerProps) {
  const { locale } = useSimpleLocale();
  const t = useCallback(
    (key: string, params?: Record<string, string | number>) =>
      getTranslation(
        `businessDashboard.dashboard.menuBuilder.offersManager.${key}`,
        locale,
        params,
      ) as string,
    [locale],
  );

  const [offers, setOffers] = useState<Offer[]>([]);
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [isGeneratingImage, setIsGeneratingImage] = useState(false);
  // Menu categories from the shared React Query cache (§3.7 fix 7) so switching
  // between the Menu / Offers / Bundles sub-tabs no longer re-fetches the menu.
  const { data: menu = [] } = useSharedMenu(businessId);
  const [bundles, setBundles] = useState<Bundle[]>([]);
  // Business default currency for the "$N OFF" preview stamp; AED
  // operators saw their offers stamped in dollars before this fetch.
  const [businessCurrency, setBusinessCurrency] = useState<string>("USD");

  // Localized fixed-amount discount stamp ("AED 20.00 OFF" / "AED 20.00 de
  // descuento"). The amount is pre-formatted with the business currency; the
  // catalog key interpolates {{amount}} — no hardcoded symbol, no fallback hack.
  const fixedDiscountLabel = useCallback(
    (value: number): string => {
      const amount = formatCurrencyIntl(value, businessCurrency);
      return t("discount.fixed", { amount });
    },
    [businessCurrency, t],
  );

  useEffect(() => {
    let cancelled = false;
    if (!businessId) return;
    getBusiness(businessId)
      .then((biz) => {
        if (!cancelled && biz?.default_currency) {
          setBusinessCurrency(biz.default_currency);
        }
      })
      .catch(() => {});
    return () => {
      cancelled = true;
    };
  }, [businessId]);

  const { isOpen, onOpen, onOpenChange, onClose } = useDisclosure();
  // #665: Cancel calls ModalContent's `close`. useDisclosure.onClose alone
  // leaves NextUI's portaled overlay mounted on live. Keep a ref to the same
  // close function the footer uses.
  const dismissOfferModal = useRef(onClose);
  dismissOfferModal.current = onClose;

  useEffect(() => {
    if (!isOpen) return;
    return bindOfferModalEscape(
      () => dismissOfferModal.current(),
      () => document.querySelector(`[data-testid="${OFFER_EDIT_MODAL_TEST_ID}"]`),
    );
  }, [isOpen]);
  const {
    isOpen: isDeleteOpen,
    onOpen: onDeleteOpen,
    onOpenChange: onDeleteOpenChange,
    onClose: onDeleteClose,
  } = useDisclosure();
  const [currentOffer, setCurrentOffer] = useState<Offer | null>(null);
  const [offerToDelete, setOfferToDelete] = useState<number | null>(null);
  const [formData, setFormData] = useState<OfferFormData>({
    name: "",
    description: "",
    image: "",
    code: "",
    discount_type: "percentage",
    discount_value: "",
    is_active: true,
    start_date: "",
    end_date: "",
    weekday_mask: 127,
    start_time: "",
    end_time: "",
    applicable_to: "all",
    target_id: "",
  });
  // null = paint the committed target label; a string is in-progress typing.
  const [targetDraft, setTargetDraft] = useState<string | null>(null);

  const fetchData = useCallback(async () => {
    await runWithFeedback(
      async () => {
        const [offersData, bundlesData] = await Promise.all([
          businessApi.getOffers(businessId),
          businessApi.getBundles(businessId),
        ]);
        setOffers(offersData || []);
        setBundles(bundlesData || []);
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

  const itemTargets = useMemo(() => {
    const results: Array<{ id: string; label: string }> = [];
    menu.forEach((category) => {
      category.items?.forEach((item) => {
        const id = item.id || item.name;
        if (!id) return;
        results.push({
          id,
          label: `${item.name} (${category.name})`,
        });
      });
    });
    return results;
  }, [menu]);

  const allMenuItems = useMemo(() => {
    const items: Array<{ id: string; name: string; category: string }> = [];
    menu.forEach((category) => {
      category.items?.forEach((item) => {
        const id = item.id || item.name;
        if (!id) return;
        items.push({
          id,
          name: item.name,
          category: category.name,
        });
      });
    });
    return items;
  }, [menu]);

  const categoryTargets = useMemo(
    () =>
      // Target categories by their stable id (§3.7 fix 6) so a rename no longer
      // silently orphans the offer. Fall back to name only for a category that
      // somehow lacks an id (server assigns one on every write). The backend
      // matches a category offer by id or name, so both targets resolve.
      menu.map((category) => ({
        id: category.id || category.name,
        label: category.name,
      })),
    [menu],
  );

  // Item targets grouped by category for a searchable Autocomplete (§3.7 fix 6):
  // replaces a flat 1,200-option non-searchable Select.
  const groupedItemTargets = useMemo(
    () =>
      menu
        .map((category) => ({
          category: category.name,
          items: (category.items || [])
            .filter((item) => item.id || item.name)
            .map((item) => ({ id: item.id || item.name, label: item.name })),
        }))
        .filter((group) => group.items.length > 0),
    [menu],
  );

  const bundleTargets = useMemo(
    () =>
      bundles.map((bundle) => ({
        id: String(bundle.id),
        label: bundle.name,
      })),
    [bundles],
  );

  const currentTargets = useMemo(() => {
    switch (formData.applicable_to) {
      case "category":
        return categoryTargets;
      case "item":
        return itemTargets;
      case "bundle":
        return bundleTargets;
      default:
        return [];
    }
  }, [formData.applicable_to, categoryTargets, itemTargets, bundleTargets]);

  const selectedTargetLabel = useMemo(() => {
    if (formData.applicable_to === "all" || !formData.target_id) return "";
    if (formData.applicable_to === "item") {
      for (const group of groupedItemTargets) {
        const hit = group.items.find((item) => item.id === formData.target_id);
        if (hit) return hit.label;
      }
    }
    return (
      currentTargets.find((target) => target.id === formData.target_id)?.label ??
      ""
    );
  }, [
    formData.applicable_to,
    formData.target_id,
    groupedItemTargets,
    currentTargets,
  ]);

  const parseBundleItemsForPrompt = useCallback((items: Bundle["items"]): Array<{ menu_item_id: string; name?: string; quantity: number }> => {
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
      if (!Array.isArray(parsed)) return [];
      if (typeof parsed[0] === "string") {
        return parsed.map((id: string) => ({ menu_item_id: id, quantity: 1 }));
      }
      return parsed.map((item: any) => ({
        menu_item_id: item.menu_item_id || item.id || "",
        name: item.name,
        quantity: item.quantity || 1,
      }));
    } catch {
      return [];
    }
  }, []);

  const buildOfferStampText = useCallback(() => {
    const value = parseLocaleDecimal(formData.discount_value);
    if (formData.discount_type === "percentage") {
      // Same localized template as the offer-card stamp — the stamp text is
      // baked into the AI-generated offer image, so it must not be hardcoded
      // English for non-en operators.
      return t("discount.percentage", {
        value: value % 1 === 0 ? value.toFixed(0) : value.toFixed(2),
      });
    }
    // Show the actual business currency on the offer "stamp" preview
    // instead of always rendering "$N OFF" for an AED restaurant.
    return fixedDiscountLabel(value);
  }, [formData.discount_type, formData.discount_value, fixedDiscountLabel, t]);

  const offerRelatedItemNames = useMemo(() => {
    const dedupe = (names: string[]) => {
      const seen = new Set<string>();
      const result: string[] = [];
      names.forEach((name) => {
        const normalized = name.trim();
        if (!normalized || seen.has(normalized.toLowerCase())) return;
        seen.add(normalized.toLowerCase());
        result.push(normalized);
      });
      return result;
    };

    const target = formData.target_id;
    let related: string[] = [];

    if (formData.applicable_to === "all") {
      related = allMenuItems.slice(0, 6).map((item) => item.name);
    } else if (formData.applicable_to === "category") {
      related = allMenuItems
        .filter((item) => item.category.toLowerCase() === target.toLowerCase())
        .slice(0, 8)
        .map((item) => item.name);
    } else if (formData.applicable_to === "item") {
      const matched = allMenuItems.find(
        (item) => item.id === target || item.name.toLowerCase() === target.toLowerCase(),
      );
      related = matched ? [matched.name] : target ? [target] : [];
    } else if (formData.applicable_to === "bundle") {
      const matchedBundle = bundles.find((bundle) => String(bundle.id) === target);
      if (matchedBundle) {
        const refs = parseBundleItemsForPrompt(matchedBundle.items);
        related = refs
          .map((ref) => ref.name || allMenuItems.find((item) => item.id === ref.menu_item_id)?.name || ref.menu_item_id)
          .filter(Boolean) as string[];
      }
    }

    const normalized = dedupe(related);
    if (normalized.length > 0) return normalized.slice(0, 8);
    return dedupe(allMenuItems.slice(0, 6).map((item) => item.name));
  }, [formData.applicable_to, formData.target_id, allMenuItems, bundles, parseBundleItemsForPrompt]);

  const resetForm = () => {
    setFormData({
      name: "",
      description: "",
      image: "",
      code: "",
      discount_type: "percentage",
      discount_value: "",
      is_active: true,
      start_date: "",
      end_date: "",
      weekday_mask: 127,
      start_time: "",
      end_time: "",
      applicable_to: "all",
      target_id: "",
    });
    setCurrentOffer(null);
    setTargetDraft(null);
  };

  const handleCreate = () => {
    resetForm();
    onOpen();
  };

  const handleEdit = (offer: Offer) => {
    setCurrentOffer(offer);
    setFormData({
      name: offer.name || "",
      description: offer.description || "",
      image: offer.image || "",
      code: offer.code || "",
      discount_type: offer.discount_type || "percentage",
      discount_value: offer.discount_value ? String(offer.discount_value) : "",
      // #835: is_active on a read is the effective answer; the modal edits
      // the operator switch, so seed it from the stored column.
      is_active: offerManualActive(offer),
      start_date: toDateInput(offer.start_date),
      end_date: toDateInput(offer.end_date),
      weekday_mask: offer.weekday_mask || 127,
      start_time: minuteToTime(offer.start_minute),
      end_time: minuteToTime(offer.end_minute),
      applicable_to: offer.applicable_to || "all",
      target_id: offer.target_id || "",
    });
    setTargetDraft(null);
    onOpen();
  };

  const handleDelete = async (id: number) => {
    setOfferToDelete(id);
    onDeleteOpen();
  };

  const confirmDelete = async () => {
    if (offerToDelete == null) return;
    const ok = await runWithFeedback(
      async () => {
        await businessApi.deleteOffer(businessId, offerToDelete);
        setOffers((prev) => prev.filter((offer) => offer.id !== offerToDelete));
        return true;
      },
      {
        success: t("messages.deleteSuccess"),
        error: t("messages.deleteError"),
      },
    );
    // Clear the pending id only on success — keeping it set on failure lets a
    // retry click reach confirmDelete instead of hitting the early-return guard.
    if (ok) {
      setOfferToDelete(null);
      onDeleteClose();
    }
  };

  const handleGenerateImage = async () => {
    await runWithFeedback(
      async () => {
        const generated = await businessApi.generateMenuImage(businessId, {
          name: formData.name || t("form.imageDefaultName"),
          description: formData.description || formData.name || t("form.imageDefaultDescription"),
          entity_type: "offer",
          offer_scope: formData.applicable_to,
          offer_stamp_text: buildOfferStampText(),
          related_item_names: offerRelatedItemNames,
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

  // L3-42: the raw string is parsed exactly once, here and in validation.
  const parsedDiscountValue = parseLocaleDecimal(formData.discount_value);

  const buildPayload = (): Offer => ({
    name: formData.name.trim(),
    description: formData.description.trim(),
    image: formData.image,
    code: formData.code.trim() || undefined,
    discount_type: formData.discount_type,
    discount_value: parsedDiscountValue,
    is_active: formData.is_active,
    start_date: formData.start_date ? `${formData.start_date}T00:00:00Z` : undefined,
    end_date: formData.end_date ? `${formData.end_date}T23:59:59Z` : undefined,
    weekday_mask: formData.weekday_mask,
    start_minute: timeToMinute(formData.start_time),
    end_minute: timeToMinute(formData.end_time),
    applicable_to: formData.applicable_to,
    target_id: formData.applicable_to === "all" ? undefined : formData.target_id,
  });

  // F27: an end date before the start date yields a no-op offer.
  const dateRangeInvalid =
    !!formData.start_date &&
    !!formData.end_date &&
    formData.end_date < formData.start_date;

  // L3-14: type-aware discount bound (percentage ≤ 100).
  const discountInvalid = !isOfferDiscountValid(
    formData.discount_type,
    parsedDiscountValue,
  );
  const discountErrorKey = offerDiscountErrorKey(
    formData.discount_type,
    parsedDiscountValue,
  );

  // F25: surface why the save can't proceed instead of a silent no-op.
  const isFormValid =
    formData.name.trim().length > 0 &&
    !discountInvalid &&
    (formData.applicable_to === "all" || !!formData.target_id) &&
    formData.weekday_mask > 0 &&
    (!!formData.start_time === !!formData.end_time) &&
    !dateRangeInvalid;

  const handleSubmit = async () => {
    if (!isFormValid) {
      toast.error(
        dateRangeInvalid
          ? t("messages.dateRangeError")
          : discountErrorKey
            ? t(discountErrorKey)
            : t("messages.validationError"),
      );
      return;
    }

    const payload = buildPayload();
    setSaving(true);
    try {
      if (currentOffer?.id) {
        await businessApi.updateOffer(businessId, currentOffer.id, payload);
      } else {
        await businessApi.createOffer(businessId, payload);
      }
      await fetchData();
      toast.success(t("messages.saveSuccess"));
      onClose();
      resetForm();
    } catch (err) {
      // L3-14: route save failures through the shared error surface (not a
      // generic local string that hides the backend reason).
      toast.error(surfaceBackendError(err, locale) || t("messages.saveError"));
    } finally {
      setSaving(false);
    }
  };

  const scopeLabel = (scope?: Offer["applicable_to"]) => {
    switch (scope) {
      case "category":
        return t("scope.category");
      case "item":
        return t("scope.item");
      case "bundle":
        return t("scope.bundle");
      default:
        return t("scope.all");
    }
  };

  // Resolve an offer's target to a display label, or null when the target no
  // longer exists (renamed/deleted category/item/bundle). Legacy offers may
  // store a category NAME as the target_id; try both id and name so an existing
  // valid offer isn't shown as broken. "all"-scoped offers have no target.
  const resolveTargetLabel = useCallback(
    (offer: Offer): { label: string; missing: boolean } | null => {
      const scope = offer.applicable_to;
      const targetId = (offer.target_id || "").trim();
      if (!scope || scope === "all") return null;
      let list: Array<{ id: string; label: string }> = [];
      if (scope === "category") list = categoryTargets;
      else if (scope === "item") list = itemTargets;
      else if (scope === "bundle") list = bundleTargets;
      const match =
        list.find((tgt) => tgt.id === targetId) ||
        // Legacy name-keyed category/item targets.
        list.find((tgt) => tgt.label === targetId);
      if (match) return { label: match.label, missing: false };
      return { label: targetId || t("targetMissing"), missing: true };
    },
    [categoryTargets, itemTargets, bundleTargets, t],
  );

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
        <OffersContentSkeleton />
      ) : offers.length === 0 ? (
        <PremiumPanel className="p-8 text-center" withTexture={false}>
          <div className="mx-auto mb-4 flex h-12 w-12 items-center justify-center rounded-2xl border border-brand/15 bg-brand/5 shadow-sm shadow-brand/10">
            <Tag className="h-5 w-5 text-brand" aria-hidden="true" />
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
          {offers.map((offer) => (
            <PremiumPanel
              key={offer.id || `${offer.name}-${offer.target_id}`}
              as="article"
              className="group p-4 transition-transform duration-200 hover:-translate-y-0.5 sm:p-5"
              withTexture={false}
            >
              <div className="flex flex-col items-start justify-between gap-3 sm:flex-row">
                <div className="min-w-0">
                  <h4 className="truncate text-lg font-semibold text-ink-950">{offer.name}</h4>
                  <div className="mt-2 flex flex-wrap items-center gap-2">
                    {(() => {
                      // #835: one chip, not an Active chip contradicted by a
                      // "Paused by inventory" chip beside it.
                      const status = offerStatusChip(offer);
                      const chip = (
                        <Chip
                          size="sm"
                          color={status.tone}
                          variant="flat"
                          startContent={
                            status.tone === "warning" ? (
                              <AlertTriangle className="h-3 w-3" />
                            ) : undefined
                          }
                          data-testid={`offer-status-${offer.id ?? offer.name}`}
                        >
                          {t(status.labelKey)}
                        </Chip>
                      );
                      return status.tooltipKey ? (
                        <span title={t(status.tooltipKey)}>{chip}</span>
                      ) : (
                        chip
                      );
                    })()}
                    {offer.applicable_to && (
                      <Chip size="sm" variant="bordered">
                        {scopeLabel(offer.applicable_to)}
                      </Chip>
                    )}
                    {(() => {
                      const target = resolveTargetLabel(offer);
                      if (!target) return null;
                      return target.missing ? (
                        <Chip
                          size="sm"
                          color="warning"
                          variant="flat"
                          startContent={<AlertTriangle className="h-3 w-3" />}
                        >
                          {t("targetMissing")}
                        </Chip>
                      ) : (
                        <Chip size="sm" variant="flat">
                          {target.label}
                        </Chip>
                      );
                    })()}
                    {offer.code && (
                      <Chip size="sm" color="secondary" variant="flat">
                        {t("codeChip", { code: offer.code })}
                      </Chip>
                    )}
                  </div>
                </div>
                <div className="flex gap-2 w-full sm:w-auto justify-end">
                  <Button
                    isIconOnly
                    size="sm"
                    variant="light"
                    aria-label={t("editOfferAria")}
                    className="rounded-xl text-ink-600 hover:bg-warm-100 hover:text-ink-950"
                    onPress={() => handleEdit(offer)}
                  >
                    <Pencil className="w-4 h-4" />
                  </Button>
                  <Button
                    isIconOnly
                    size="sm"
                    color="danger"
                    variant="light"
                    aria-label={t("deleteOfferAria")}
                    className="rounded-xl hover:bg-rose-50"
                    onPress={() => offer.id && handleDelete(offer.id)}
                  >
                    <Trash2 className="w-4 h-4" />
                  </Button>
                </div>
              </div>
              <div className="pt-4">
                <OfferCardImage
                  src={offer.image}
                  name={offer.name}
                  addPhotoLabel={t("addPhoto")}
                />
                <p className="mb-4 text-sm leading-6 text-ink-600">{offer.description}</p>
                <div className="mb-2 inline-flex items-center gap-2 rounded-xl border border-brand/15 bg-brand/5 px-3 py-2 text-sm font-semibold text-brand-dark">
                  <Tag className="w-4 h-4" aria-hidden="true" />
                  <span>
                    {offer.discount_type === "percentage"
                      ? t("discount.percentage", { value: offer.discount_value })
                      : fixedDiscountLabel(offer.discount_value)}
                  </span>
                </div>
                {(() => {
                  const endsCue = offerScheduleCue(offer.end_date, t);
                  if (!endsCue) return null;
                  return (
                    <div className="mt-3 flex flex-wrap items-center gap-x-2 gap-y-1 text-xs font-medium text-ink-500">
                      <span className="inline-flex items-center gap-1.5">
                        <Calendar className="w-3 h-3" aria-hidden="true" />
                        <span
                          className="rounded-full border border-warm-200 bg-warm-50 px-2 py-0.5 text-[11px] text-ink-600"
                          data-testid="offer-ends-cue"
                        >
                          {endsCue}
                        </span>
                      </span>
                    </div>
                  );
                })()}
              </div>
            </PremiumPanel>
          ))}
        </div>
      )}

      <Modal
        isOpen={isOpen}
        onOpenChange={onOpenChange}
        onClose={onClose}
        size="3xl"
        scrollBehavior="inside"
        isDismissable
        isKeyboardDismissDisabled={false}
        placement="center"
        classNames={{
          wrapper: "items-center",
          // No `max-h` override here: NextUI pairs its own `sm:my-16` margins
          // with a margin-aware `max-h-[calc(100%-8rem)]` under
          // scrollBehavior="inside". Replacing that cap with a raw viewport
          // unit made the dialog 90vh + 8rem tall, pushing the footer (and
          // Cancel) below the fold on short screens.
          body: "min-h-0 flex-1 overflow-y-auto pb-8",
          // Footer chrome must not steal hits on Target "Show suggestions"
          // at 1024×622. Buttons keep pointer-events so Cancel/Update work.
          footer:
            "relative z-0 shrink-0 border-t border-warm-200 bg-white pointer-events-none [&>*]:pointer-events-auto",
        }}
      >
        <ModalContent
          data-testid={OFFER_EDIT_MODAL_TEST_ID}
          onKeyDown={(event) => {
            const root =
              document.querySelector(
                `[data-testid="${OFFER_EDIT_MODAL_TEST_ID}"]`,
              ) ?? event.currentTarget;
            if (hasExpandedPickerOwnedBy(root, event.target)) {
              dismissOwnedPicker(root);
              event.preventDefault();
              event.stopPropagation();
              return;
            }
            if (!shouldCloseOfferModalOnEscape(event, root)) {
              return;
            }
            event.preventDefault();
            event.stopPropagation();
            dismissOfferModal.current();
          }}
        >
          {(close) => {
            dismissOfferModal.current = close;
            return (
            <>
              <ModalHeader>{currentOffer ? t("modal.editTitle") : t("modal.createTitle")}</ModalHeader>
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
                <Input
                  label={t("form.codeLabel")}
                  placeholder={t("form.codePlaceholder")}
                  description={t("form.codeHelp")}
                  value={formData.code}
                  onValueChange={(val) => setFormData({ ...formData, code: val })}
                  variant="bordered"
                />
                <div className="space-y-3">
                  <ImageUpload
                    businessId={businessId}
                    type="offer"
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
                  <Select
                    label={t("form.discountTypeLabel")}
                    selectedKeys={[formData.discount_type]}
                    onChange={(e) => {
                      const next = e.target.value;
                      if (next !== "percentage" && next !== "fixed") return;
                      setFormData({ ...formData, discount_type: next });
                    }}
                    variant="bordered"
                  >
                    <SelectItem key="percentage" value="percentage">{t("form.discountTypePercentage")}</SelectItem>
                    <SelectItem key="fixed" value="fixed">{t("form.discountTypeFixed")}</SelectItem>
                  </Select>
                  <Input
                    type="text"
                    inputMode="decimal"
                    label={t("form.valueLabel")}
                    // L3-42: raw string in, parsed only at validation/submit —
                    // re-rendering a parsed number here ate "10." → "105".
                    value={formData.discount_value}
                    onValueChange={(val) =>
                      setFormData({
                        ...formData,
                        discount_value: val,
                      })
                    }
                    isInvalid={
                      discountInvalid && formData.discount_value.trim() !== ""
                    }
                    errorMessage={
                      discountInvalid &&
                      formData.discount_value.trim() !== "" &&
                      discountErrorKey
                        ? t(discountErrorKey)
                        : undefined
                    }
                    variant="bordered"
                    data-testid="offer-discount-value"
                  />
                </div>

                <fieldset className="space-y-3">
                  <legend className="text-sm font-medium text-ink-800">
                    {t("form.weekdaysLabel")}
                  </legend>
                  <div className="flex flex-wrap gap-2">
                    {WEEKDAYS.map((weekday, index) => {
                      const bit = 1 << index;
                      const selected = (formData.weekday_mask & bit) !== 0;
                      return (
                        <Button
                          key={weekday}
                          type="button"
                          size="sm"
                          variant={selected ? "solid" : "bordered"}
                          className={selected ? "bg-brand text-white" : "border-warm-300 text-ink-700"}
                          aria-label={t(`weekdays.${weekday}`)}
                          aria-pressed={selected}
                          onPress={() =>
                            setFormData((current) => ({
                              ...current,
                              weekday_mask: current.weekday_mask ^ bit,
                            }))
                          }
                        >
                          {t(`weekdays.${weekday}Short`)}
                        </Button>
                      );
                    })}
                  </div>
                  {formData.weekday_mask === 0 && (
                    <p className="text-sm text-rose-700">{t("messages.weekdayRequired")}</p>
                  )}
                </fieldset>

                <div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
                  <Input
                    type="time"
                    label={t("form.startTimeLabel")}
                    value={formData.start_time}
                    onChange={(event) =>
                      setFormData({ ...formData, start_time: event.target.value })
                    }
                    variant="bordered"
                  />
                  <Input
                    type="time"
                    label={t("form.endTimeLabel")}
                    value={formData.end_time}
                    onChange={(event) =>
                      setFormData({ ...formData, end_time: event.target.value })
                    }
                    variant="bordered"
                  />
                </div>
                {!!formData.start_time !== !!formData.end_time && (
                  <p className="text-sm text-rose-700">{t("messages.timePairError")}</p>
                )}

                <div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
                  <Input
                    type="date"
                    label={t("form.startDateLabel")}
                    value={formData.start_date}
                    onValueChange={(val) => setFormData({ ...formData, start_date: val })}
                    variant="bordered"
                  />
                  <Input
                    type="date"
                    label={t("form.endDateLabel")}
                    value={formData.end_date}
                    // L3-15: do not set native min= (Chrome English bubble).
                    // dateRangeInvalid drives isInvalid + localized error below.
                    onValueChange={(val) => setFormData({ ...formData, end_date: val })}
                    variant="bordered"
                    isInvalid={dateRangeInvalid}
                    errorMessage={
                      dateRangeInvalid ? t("messages.dateRangeError") : undefined
                    }
                    data-testid="offer-end-date"
                  />
                </div>
                {dateRangeInvalid && (
                  <p className="text-sm text-rose-700" role="alert">
                    {t("messages.dateRangeError")}
                  </p>
                )}

                <div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
                  <Select
                    label={t("form.appliesToLabel")}
                    selectedKeys={[formData.applicable_to]}
                    disallowEmptySelection
                    data-testid="offer-applies-to"
                    onChange={(e) => {
                      const next = applyOfferScopeChange(
                        {
                          applicable_to: formData.applicable_to,
                          target_id: formData.target_id,
                        },
                        e.target.value,
                      );
                      if (!next) return;
                      setFormData({ ...formData, ...next });
                      setTargetDraft(null);
                    }}
                    variant="bordered"
                  >
                    <SelectItem key="all" value="all">{t("scope.all")}</SelectItem>
                    <SelectItem key="category" value="category">{t("scope.category")}</SelectItem>
                    <SelectItem key="item" value="item">{t("scope.item")}</SelectItem>
                    <SelectItem key="bundle" value="bundle">{t("scope.bundle")}</SelectItem>
                  </Select>

                  {/* Searchable target picker (§3.7 fix 6). Items are grouped by
                      category; categories and bundles are a flat searchable list.
                      Selection stores the stable id (or, for a legacy category,
                      the name fallback) — never a display label. */}
                  <Autocomplete
                    isDisabled={formData.applicable_to === "all"}
                    label={t("form.targetLabel")}
                    aria-label={t("form.targetLabel")}
                    selectedKey={formData.target_id || null}
                    inputValue={targetDraft ?? selectedTargetLabel}
                    data-testid="offer-target"
                    onInputChange={(value) => {
                      setTargetDraft(value);
                      if (value === "") {
                        setFormData((prev) =>
                          prev.target_id === ""
                            ? prev
                            : { ...prev, target_id: "" },
                        );
                      }
                    }}
                    onSelectionChange={(key) => {
                      const decision = applyOfferTargetSelection(
                        key,
                        targetDraft ?? selectedTargetLabel,
                      );
                      if (decision.kind === "ignore") return;
                      if (decision.kind === "clear") {
                        setTargetDraft("");
                        setFormData((prev) => ({ ...prev, target_id: "" }));
                        return;
                      }
                      setTargetDraft(null);
                      setFormData((prev) => ({
                        ...prev,
                        target_id: decision.target_id,
                      }));
                    }}
                    variant="bordered"
                    placeholder={
                      formData.applicable_to === "all"
                        ? t("form.targetNotNeeded")
                        : t("form.targetPlaceholder")
                    }
                  >
                    {formData.applicable_to === "item"
                      ? groupedItemTargets.map((group) => (
                          <AutocompleteSection key={group.category} title={group.category}>
                            {group.items.map((target) => (
                              <AutocompleteItem key={target.id} textValue={target.label}>
                                {target.label}
                              </AutocompleteItem>
                            ))}
                          </AutocompleteSection>
                        ))
                      : currentTargets.map((target) => (
                          <AutocompleteItem key={target.id} textValue={target.label}>
                            {target.label}
                          </AutocompleteItem>
                        ))}
                  </Autocomplete>
                </div>

                <Switch
                  isSelected={formData.is_active}
                  onValueChange={(value) => setFormData({ ...formData, is_active: value })}
                >
                  {t("form.activeToggle")}
                </Switch>
              </ModalBody>
              <ModalFooter data-testid="offer-edit-footer">
                <Button
                  variant="light"
                  onPress={close}
                  data-testid="offer-edit-cancel"
                >
                  {t("buttons.cancel")}
                </Button>
                <Button
                  color="primary"
                  onPress={handleSubmit}
                  isLoading={saving}
                  isDisabled={!isFormValid || saving}
                  data-testid="offer-edit-submit"
                >
                  {currentOffer ? t("buttons.update") : t("buttons.create")}
                </Button>
              </ModalFooter>
            </>
            );
          }}
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
