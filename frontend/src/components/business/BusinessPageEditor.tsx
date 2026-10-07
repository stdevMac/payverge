"use client";

import React, { useState, useEffect, useCallback, useId, useMemo } from "react";
import { Textarea, Switch, Divider, Button, Select, SelectItem } from "@nextui-org/react";
import { NamedSwitch } from "@/components/ui/NamedSwitch";
import {
    Globe,
    Paintbrush,
    Camera,
    Clock,
    Award,
    ExternalLink,
    Star,
    Phone,
    AlertTriangle,
    ImagePlus,
} from "lucide-react";
import {
    businessApi,
    BusinessAddress,
    UpdateBusinessRequest,
    BusinessGalleryImage,
    BusinessOperatingHours,
    BusinessOperatingException,
    BusinessSpecialFeature,
    BusinessDesignSettings,
} from "@/api/business";
import { getBusinessByCustomUrl } from "@/api/publicBusiness";
import { PrimarySpinner } from "@/components/ui/spinners/PrimarySpinner";
import { useToast } from "@/contexts/ToastContext";
import {
    useSimpleLocale,
    getTranslation,
} from "@/i18n/SimpleTranslationProvider";
import { useBusinessAccess } from "@/hooks/useBusinessAccess";
import { useUnsavedChangesGuard } from "@/hooks/useUnsavedChangesGuard";
import BannerImageUploader from "./BannerImageUploader";
import SimpleImageUpload from "./SimpleImageUpload";
import CustomURLInput from "./CustomURLInput";
import GalleryImageUploader from "./GalleryImageUploader";
import OperatingHoursEditor from "./OperatingHoursEditor";
import SpecialFeaturesEditor from "./SpecialFeaturesEditor";
import GoogleBusinessSearch from "./GoogleBusinessSearch";
import { TIMEZONE_OPTIONS } from "@/utils/timezones";
import { parseSocialMedia, parseBannerImages, parseDesignSettings } from "@/utils/businessDataParsers";
import DashboardLockedTabView from "./DashboardLockedTabView";
import { BusinessPageToggle } from "./BusinessPageToggle";
import DesignCustomization from "./DesignCustomization";
import SaveBar from "./SaveBar";
import ContactEditor from "./ContactEditor";
import { DashboardTabTransition, PremiumPanel } from "./premium";
import DashboardTabShell from "./shared/DashboardTabShell";
import IconTile from "@/components/ui/IconTile";
import { pluginAPI } from "@/api/plugins";
import {
    reviewProvidersFromEnabledPlugins,
    type ReviewProvider,
} from "@/lib/reviewProviders";
import { PLUGIN } from "@/constants/plugins";
import {
    BUSINESS_PAGE_SECTIONS,
    type BusinessPageSection,
} from "@/utils/businessUrl";
import { useUrlState } from "@/hooks/useUrlState";

interface BusinessPageEditorProps {
    businessId: number;
}

// Local interfaces matching BusinessSettings.tsx
interface BusinessProfile {
    name: string;
    logo?: string;
    address: BusinessAddress;
    description?: string;
    custom_url?: string;
    phone?: string;
    website?: string;
    social_media?: {
        instagram?: string;
        facebook?: string;
        twitter?: string;
        linkedin?: string;
        youtube?: string;
        tiktok?: string;
    };
    google_place_id?: string;
    google_business_name?: string;
    google_review_link?: string;
    google_business_url?: string;
    google_reviews_enabled?: boolean;
    timezone?: string;
}

interface BusinessPageSettings {
    enabled: boolean;
    custom_url: string;
    banner_images: string[];
    show_reviews: boolean;
    google_reviews_enabled: boolean;
    welcome_message?: string;
    about_story?: string;
    show_welcome_message?: boolean;
    show_about_story?: boolean;
    show_gallery?: boolean;
    show_operating_hours?: boolean;
    show_special_features?: boolean;
}

/** Per-section dirty flags so Save only PUTs changed sections (Stream 9 #2). */
type DirtySections = {
    profile: boolean; // updateBusiness (content + contact + banners + show_*)
    design: boolean;
    hours: boolean;
    features: boolean;
    gallery: boolean;
};

const CLEAN_SECTIONS: DirtySections = {
    profile: false,
    design: false,
    hours: false,
    features: false,
    gallery: false,
};

/** es-AR public /b/ copy often lives under `es`; try the regional locale first. */
export function publicCopyOverlayLocales(locale: string): string[] {
    const current = locale.trim();
    if (!current || current.toLowerCase() === "en") return [];
    const locales = [current];
    const base = current.split("-")[0];
    if (base && base.toLowerCase() !== current.toLowerCase()) {
        locales.push(base);
    }
    return locales;
}

export async function loadPublicBusinessCopyOverlay(
    customUrl: string,
    locales: string[],
    seedDescription: string,
) {
    let best: Awaited<ReturnType<typeof getBusinessByCustomUrl>> | null = null;
    for (const language of locales) {
        try {
            const localized = await getBusinessByCustomUrl(customUrl, language);
            if (!localized) continue;
            if (!best) best = localized;
            if (
                localized.description &&
                localized.description !== seedDescription
            ) {
                return localized;
            }
        } catch {
            // Keep trying parent locales (es-AR → es).
        }
    }
    return best;
}

export default function BusinessPageEditor({ businessId }: BusinessPageEditorProps) {
    const { locale: currentLocale } = useSimpleLocale();
    const { showSuccess, showError } = useToast();
    const settingsA11yId = useId();

    // Helper for tString compatibility with BusinessSettings keys
    const tStringSettings = useCallback(
        (key: string): string => {
            const fullKey = `businessSettings.${key}`;
            const result = getTranslation(fullKey, currentLocale);
            return Array.isArray(result) ? result[0] || key : (result as string);
        },
        [currentLocale],
    );

    // State
    const [isLoading, setIsLoading] = useState(true);
    const [isSaving, setIsSaving] = useState(false);
    const [dirtySections, setDirtySections] = useState<DirtySections>(CLEAN_SECTIONS);
    const [loadFailed, setLoadFailed] = useState(false);
    const [slugTaken, setSlugTaken] = useState(false);

    const isDirty = useMemo(
        () => Object.values(dirtySections).some(Boolean),
        [dirtySections],
    );
    const markDirty = useCallback((section: keyof DirtySections) => {
        setDirtySections((prev) => (prev[section] ? prev : { ...prev, [section]: true }));
    }, []);

    // H2a: prompt before leaving (tab close / reload / in-app tab switch) while
    // there are unsaved page edits.
    useUnsavedChangesGuard(isDirty, "business-page-editor");

    const [profile, setProfile] = useState<BusinessProfile>({
        name: "",
        logo: "",
        address: { street: "", city: "", state: "", postal_code: "", country: "" },
        description: "",
        custom_url: "",
        phone: "",
        website: "",
        social_media: { instagram: "", facebook: "", twitter: "", linkedin: "", youtube: "", tiktok: "" },
        // M13: leave empty until the backend supplies a value or the operator
        // explicitly picks one — never silently default to Asia/Dubai.
        timezone: "",
    });

    const [settings, setSettings] = useState<BusinessPageSettings>({
        enabled: false,
        custom_url: "",
        banner_images: [],
        show_reviews: true,
        google_reviews_enabled: false,
        welcome_message: "",
        about_story: "",
        show_welcome_message: false,
        show_about_story: false,
        show_gallery: false,
        show_operating_hours: false,
        show_special_features: false,
    });

    const [galleryImages, setGalleryImages] = useState<BusinessGalleryImage[]>([]);
    const [operatingHours, setOperatingHours] = useState<BusinessOperatingHours[]>([]);
    const [operatingExceptions, setOperatingExceptions] = useState<
        BusinessOperatingException[]
    >([]);
    const [specialFeatures, setSpecialFeatures] = useState<BusinessSpecialFeature[]>([]);
    /**
     * Read-only storefront defaults (#591). Not editable on this screen and
     * deliberately kept OUT of `profile`/`settings` so they can never ride
     * along in a save payload — they exist so the live preview renders in the
     * storefront's own language and currency instead of hardcoded en/USD.
     */
    const [storefrontDefaults, setStorefrontDefaults] = useState<{
        default_currency?: string;
        display_currency?: string;
        default_language?: string;
    }>({});
    /** Enabled review plugins (Trustpilot, …) drive the Reviews provider list. */
    const [reviewProviders, setReviewProviders] = useState<ReviewProvider[]>(() =>
        reviewProvidersFromEnabledPlugins([]),
    );
    const [trustpilotConfig, setTrustpilotConfig] = useState<{
        business_name?: string;
        trustpilot_url?: string;
    } | null>(null);

    /* eslint-disable no-restricted-syntax -- design setting defaults are API-stored user data, not Tailwind */
    const [designSettings, setDesignSettings] = useState<BusinessDesignSettings>({
        primary_color: "#1a6b6a",
        secondary_color: "#2a8b8a",
        font_family: "Inter",
        theme: "light",
        menu_layout: "grid",
        show_images: true,
        show_descriptions: true,
        header_style: "banner",
        corner_radius: "medium",
        shadow_intensity: "subtle",
        background_pattern: "none",
        pattern_opacity: 0.1,
        hero_layout: "centered",
        section_density: "comfortable",
    });
    /* eslint-enable no-restricted-syntax */

    // Operational lock (admin suspend/close) and RBAC access
    const { hasAccess, loading: accessLoading } = useBusinessAccess(businessId);

    // Load Data
    const loadData = useCallback(async () => {
        try {
            setIsLoading(true);
            setLoadFailed(false);
            const [business, images, hours, exceptionRows, features, pluginsRes] =
                await Promise.all([
                businessApi.getBusiness(businessId),
                businessApi.getBusinessGalleryImages(businessId),
                businessApi.getBusinessOperatingHours(businessId),
                (businessApi.getBusinessOperatingExceptions?.(businessId) ??
                    Promise.resolve([])).catch(() => []),
                businessApi.getBusinessSpecialFeatures(businessId),
                pluginAPI.business.getBusinessPlugins(String(businessId)).catch(() => ({
                    plugins: [] as Awaited<
                        ReturnType<typeof pluginAPI.business.getBusinessPlugins>
                    >["plugins"],
                })),
            ]);

            if (business) {
                setProfile({
                    name: business.name || "",
                    logo: business.logo || "",
                    address: business.address || { street: "", city: "", state: "", postal_code: "", country: "" },
                    description: business.description || "",
                    custom_url: business.custom_url || "",
                    phone: business.phone || "",
                    website: business.website || "",
                    social_media: parseSocialMedia(business.social_media) as {
                        instagram?: string;
                        facebook?: string;
                        twitter?: string;
                        linkedin?: string;
                        youtube?: string;
                        tiktok?: string;
                    },
                    google_place_id: business.google_place_id,
                    google_business_name: business.google_business_name,
                    google_review_link: business.google_review_link,
                    google_business_url: business.google_business_url,
                    google_reviews_enabled: business.google_reviews_enabled,
                    // M13: honor the stored timezone; leave blank (not
                    // Asia/Dubai) when the backend hasn't set one.
                    timezone: business.timezone || "",
                });

                setSettings({
                    enabled: business.business_page_enabled || false,
                    custom_url: business.custom_url || "",
                    banner_images: parseBannerImages(business.banner_images),
                    show_reviews: business.show_reviews !== undefined ? business.show_reviews : true,
                    google_reviews_enabled: business.google_reviews_enabled || false,
                    welcome_message: business.welcome_message || "",
                    about_story: business.about_story || "",
                    show_welcome_message: business.show_welcome_message || false,
                    show_about_story: business.show_about_story || false,
                    show_gallery: business.show_gallery || false,
                    show_operating_hours: business.show_operating_hours || false,
                    show_special_features: business.show_special_features || false,
                });

                setStorefrontDefaults({
                    default_currency: (business as any).default_currency,
                    display_currency: (business as any).display_currency,
                    default_language: (business as any).default_language,
                });

                if ((business as any).design_settings) {
                    setDesignSettings(parseDesignSettings((business as any).design_settings));
                }
            }

            setGalleryImages(images);
            setOperatingHours(hours);
            setOperatingExceptions(exceptionRows);
            setSpecialFeatures(features);

            const pluginRows = pluginsRes?.plugins ?? [];
            const enabledNames = pluginRows
                .filter((p) => p.is_enabled)
                .map((p) => p.plugin?.name || "")
                .filter(Boolean);
            setReviewProviders(reviewProvidersFromEnabledPlugins(enabledNames));

            const trustpilotRow = pluginRows.find(
                (p) =>
                    p.is_enabled &&
                    (p.plugin?.name || "").toLowerCase() === PLUGIN.trustpilot,
            );
            if (trustpilotRow?.config) {
                try {
                    const parsed =
                        typeof trustpilotRow.config === "string"
                            ? JSON.parse(trustpilotRow.config)
                            : trustpilotRow.config;
                    setTrustpilotConfig({
                        business_name: parsed?.business_name,
                        trustpilot_url: parsed?.trustpilot_url,
                    });
                } catch {
                    setTrustpilotConfig({});
                }
            } else {
                setTrustpilotConfig(null);
            }

            setDirtySections(CLEAN_SECTIONS);

            if (
                business?.custom_url &&
                currentLocale &&
                currentLocale !== "en"
            ) {
                const overlayLocales = publicCopyOverlayLocales(currentLocale);
                const seedDescription = business.description || "";
                try {
                    const localized = await loadPublicBusinessCopyOverlay(
                        business.custom_url,
                        overlayLocales,
                        seedDescription,
                    );
                    if (localized?.description) {
                        setProfile((prev) => ({
                            ...prev,
                            description: localized.description,
                        }));
                    }
                    if (localized?.welcome_message) {
                        setSettings((prev) => ({
                            ...prev,
                            welcome_message: localized.welcome_message || prev.welcome_message,
                        }));
                    }
                    if (localized?.about_story) {
                        setSettings((prev) => ({
                            ...prev,
                            about_story: localized.about_story || prev.about_story,
                        }));
                    }
                } catch {
                    // Public overlay is best-effort; keep the operator API payload.
                }
            }

        } catch (error) {
            console.error("Failed to load business page data:", error);
            showError(tStringSettings("businessPage.toasts.errorTitle"), tStringSettings("businessPage.toasts.loadPageError"));
            setLoadFailed(true);
        } finally {
            setIsLoading(false);
        }
    }, [businessId, showError, tStringSettings, currentLocale]);

    useEffect(() => {
        if (!accessLoading) {
            if (!hasAccess) {
                setIsLoading(false);
            } else {
                loadData();
            }
        }
    }, [loadData, accessLoading, hasAccess]);

    // Handlers — each marks only its owning section dirty so handleSave can
    // PUT changed sections only (avoids gallery delete+retranslate on typo fix).
    const handleBannerUpload = useCallback((banners: string[]) => {
        setSettings(prev => ({ ...prev, banner_images: banners }));
        markDirty("profile");
    }, [markDirty]);

    const handleLogoUpload = useCallback((url: string) => {
        setProfile((prev) => ({ ...prev, logo: url }));
        markDirty("profile");
    }, [markDirty]);

    const handleSocialMediaChange = useCallback((platform: "instagram" | "facebook" | "twitter" | "linkedin" | "youtube" | "tiktok", value: string) => {
        setProfile(prev => ({
            ...prev,
            social_media: {
                ...prev.social_media,
                [platform]: value
            }
        }));
        markDirty("profile");
    }, [markDirty]);

    const handlePhoneChange = useCallback((value: string) => {
        setProfile(prev => ({ ...prev, phone: value }));
        markDirty("profile");
    }, [markDirty]);

    const handleWebsiteChange = useCallback((value: string) => {
        setProfile(prev => ({ ...prev, website: value }));
        markDirty("profile");
    }, [markDirty]);

    const handleAddressChange = useCallback((field: keyof BusinessAddress, value: string) => {
        setProfile(prev => ({ ...prev, address: { ...prev.address, [field]: value } }));
        markDirty("profile");
    }, [markDirty]);

    const handleGalleryImagesChange = useCallback((images: BusinessGalleryImage[]) => {
        setGalleryImages(images);
        markDirty("gallery");
    }, [markDirty]);

    const handleOperatingHoursChange = useCallback((hours: BusinessOperatingHours[]) => {
        setOperatingHours(hours);
        markDirty("hours");
    }, [markDirty]);

    const handleOperatingExceptionsChange = useCallback(
        (rows: BusinessOperatingException[]) => {
            setOperatingExceptions(rows);
            markDirty("hours");
        },
        [markDirty],
    );

    const handleSpecialFeaturesChange = useCallback((features: BusinessSpecialFeature[]) => {
        setSpecialFeatures(features);
        markDirty("features");
    }, [markDirty]);

    const handleDesignSettingsChange = useCallback((ds: BusinessDesignSettings) => {
        setDesignSettings(ds);
        markDirty("design");
    }, [markDirty]);

    // Merge the Google link/unlink result in place. Previously this called
    // loadData(), which re-fetched everything and silently discarded unsaved
    // edits on the other tabs (DI-6). Only the google_* slice changes here.
    const handleGoogleInfoMerge = useCallback((info: {
        google_place_id: string;
        google_business_name: string;
        google_review_link: string;
        google_business_url: string;
        google_reviews_enabled: boolean;
    }) => {
        setProfile((prev) => ({ ...prev, ...info }));
        setSettings((prev) => ({ ...prev, google_reviews_enabled: info.google_reviews_enabled }));
    }, []);

    // Stable identity so CustomURLInput's React.memo holds and its debounced
    // availability check isn't re-created on every parent render.
    const handleSlugAvailabilityChange = useCallback(
        (s: { checked: boolean; available: boolean | null }) => {
            setSlugTaken(s.checked && s.available === false);
        },
        [],
    );

    const handleSave = async () => {
        // P3: a failed load leaves this editor armed with DEFAULT state — saving
        // would overwrite real gallery/description data with blanks under a
        // success toast. Block Save until a reload succeeds.
        if (loadFailed) {
            showError(
                tStringSettings("businessPage.toasts.errorTitle"),
                tStringSettings("businessPage.toasts.loadFailedSaveBlocked"),
            );
            return;
        }

        try {
            setIsSaving(true);

            if (slugTaken) {
                setIsSaving(false);
                showError(
                    tStringSettings("businessPage.toasts.errorTitle"),
                    tStringSettings("businessPage.customUrlTakenError"),
                );
                return;
            }

            // Diff-aware save: only PUT sections the operator actually edited.
            // Gallery PUT used to always re-run delete-all + 21-locale translate.
            type PendingSection = {
                key: keyof DirtySections;
                labelKey: string;
                run: () => Promise<unknown>;
            };
            const pending: PendingSection[] = [];

            if (dirtySections.profile) {
                const updateData: UpdateBusinessRequest = {
                    logo: profile.logo,
                    description: profile.description,
                    custom_url: profile.custom_url,
                    phone: profile.phone,
                    website: profile.website,
                    address: profile.address,
                    social_media: profile.social_media ? JSON.stringify(profile.social_media) : "",
                    // M13: only send timezone when the operator (or backend) has an
                    // actual value. Omitting it leaves the stored column untouched.
                    ...(profile.timezone ? { timezone: profile.timezone } : {}),
                    // business_page_enabled intentionally omitted — publish toggle owns it.
                    banner_images: JSON.stringify(settings.banner_images.filter(Boolean)),
                    show_reviews: settings.show_reviews,
                    welcome_message: settings.welcome_message,
                    about_story: settings.about_story,
                    show_welcome_message: settings.show_welcome_message,
                    show_about_story: settings.show_about_story,
                    show_gallery: settings.show_gallery,
                    show_operating_hours: settings.show_operating_hours,
                    show_special_features: settings.show_special_features,
                };
                pending.push({
                    key: "profile",
                    labelKey: "businessProfile",
                    run: () => businessApi.updateBusiness(businessId, updateData),
                });
            }
            if (dirtySections.design) {
                pending.push({
                    key: "design",
                    labelKey: "design",
                    run: () => businessApi.updateBusinessDesignSettings(businessId, designSettings),
                });
            }
            if (dirtySections.hours) {
                pending.push({
                    key: "hours",
                    labelKey: "operatingHours",
                    run: async () => {
                        await businessApi.updateBusinessOperatingHours(
                            businessId,
                            operatingHours,
                        );
                        await businessApi.updateBusinessOperatingExceptions(
                            businessId,
                            operatingExceptions.map((row) => ({
                                exception_date: row.exception_date,
                                open_time: row.open_time ?? null,
                                close_time: row.close_time ?? null,
                                kitchen_close_time: row.kitchen_close_time ?? null,
                                is_closed: row.is_closed !== false,
                                label: row.label || "",
                            })),
                        );
                    },
                });
            }
            if (dirtySections.features) {
                pending.push({
                    key: "features",
                    labelKey: "specialFeatures",
                    run: () => businessApi.updateBusinessSpecialFeatures(businessId, specialFeatures),
                });
            }
            if (dirtySections.gallery) {
                pending.push({
                    key: "gallery",
                    labelKey: "galleryImages",
                    // Send full rows (incl. id) so the backend upsert preserves IDs.
                    run: () => businessApi.updateBusinessGalleryImages(businessId, galleryImages),
                });
            }

            if (pending.length === 0) {
                setIsSaving(false);
                return;
            }

            const results = await Promise.allSettled(pending.map((p) => p.run()));

            const failures = results
                .map((r, i) => ({
                    result: r,
                    key: pending[i].key,
                    label: tStringSettings(
                        `businessPage.toasts.partialSave.sections.${pending[i].labelKey}`,
                    ),
                }))
                .filter((r) => r.result.status === "rejected");

            const succeededKeys = pending
                .filter((_, i) => results[i].status === "fulfilled")
                .map((p) => p.key);
            if (succeededKeys.length > 0) {
                setDirtySections((prev) => {
                    const next = { ...prev };
                    for (const k of succeededKeys) next[k] = false;
                    return next;
                });
            }

            if (failures.length === 0) {
                // Reflect the just-saved slug so the live-page zone (driven by
                // the persisted custom_url) appears without a manual reload.
                if (dirtySections.profile) {
                    setSettings((prev) => ({ ...prev, custom_url: profile.custom_url ?? "" }));
                }
                showSuccess(
                    tStringSettings("businessPage.toasts.saveSuccess.title") || "Success",
                    tStringSettings("businessPage.toasts.saveSuccess.message") || "Business page settings saved successfully"
                );
            } else if (failures.length < results.length) {
                const failedNames = failures.map((f) => f.label).join(", ");
                showError(
                    tStringSettings("businessPage.toasts.partialSave.title"),
                    tStringSettings("businessPage.toasts.partialSave.message").replace(
                        "{sections}",
                        failedNames,
                    )
                );
            } else {
                showError(
                    tStringSettings("businessPage.toasts.saveError.title") || "Error",
                    tStringSettings("businessPage.toasts.saveError.message") || "Failed to save settings"
                );
            }
        } catch (error) {
            console.error("Failed to save settings:", error);
            showError(
                tStringSettings("businessPage.toasts.saveError.title") || "Error",
                tStringSettings("businessPage.toasts.saveError.message") || "Failed to save settings"
            );
        } finally {
            setIsSaving(false);
        }
    };

    const handleTogglePage = async (enabled: boolean) => {
        try {
            // Update state immediately for UI
            setSettings(prev => ({ ...prev, enabled }));

            // The publish toggle is a publish action, not a save. Send a minimal
            // patch — the backend preserves every absent column — so flipping
            // publish never commits the operator's unsaved, in-progress edits.
            const updateData: UpdateBusinessRequest = {
                business_page_enabled: enabled,
            };

            await businessApi.updateBusiness(businessId, updateData);
            showSuccess(
                tStringSettings("businessPage.toasts.publishSuccess.title") || "Success",
                enabled
                    ? (tStringSettings("businessPage.toasts.publishSuccess.enabled") || "Business page published")
                    : (tStringSettings("businessPage.toasts.publishSuccess.disabled") || "Business page disabled")
            );
        } catch (error) {
            console.error("Failed to toggle business page:", error);
            showError(
                tStringSettings("businessPage.toasts.statusError.title") || "Error",
                tStringSettings("businessPage.toasts.statusError.message") || "Failed to update business page status"
            );
            // Revert state on error
            setSettings(prev => ({ ...prev, enabled: !enabled }));
        }
    };

    const bannerImages = useMemo(() => settings.banner_images, [settings.banner_images]);
    // #225: `?section=` deep-links (e.g. Settings → Edit contact) land on the
    // matching Business Page sub-tab instead of always bouncing to Essentials.
    const [activeTab, setActiveTab] = useUrlState({
        key: "section",
        valid: BUSINESS_PAGE_SECTIONS,
        fallback: "essentials" as BusinessPageSection,
        // Mirror Settings: only own `section` while this dashboard tab is
        // active so a still-mounted editor cannot rewrite Settings'
        // `section=notifications` (or similar) back to Essentials (#225).
        activeWhen: { key: "tab", values: ["business-page"] },
    });
    const customUrlSlug = settings.custom_url?.trim() || "";
    const publicPagePath = customUrlSlug ? `/b/${customUrlSlug}` : "";

    // Section keys → shared strip items. `icon` is a LucideIcon COMPONENT (the
    // SegmentedTab contract), not a rendered element — the shell's shared
    // SegmentedTabs renders it and owns the WAI-ARIA tablist + keyboard nav.
    const tabs = [
        { key: "essentials", label: tStringSettings("businessPage.tabs.essentials") || "Essentials", icon: Globe },
        { key: "design", label: tStringSettings("businessPage.tabs.lookAndFeel") || "Look & feel", icon: Paintbrush },
        { key: "operations", label: tStringSettings("businessPage.tabs.operations") || "Operations", icon: Clock },
        { key: "reviews", label: tStringSettings("businessPage.tabs.reviews") || "Reviews", icon: Star },
        { key: "contact", label: tStringSettings("businessPage.tabs.contact") || "Contact", icon: Phone },
    ];

    const sectionTitleClass = "text-lg font-semibold text-ink-950";
    const sectionDescriptionClass = "text-sm text-ink-600";
    const hintClass = "text-xs text-warm-600 mt-1";

    return (
        <DashboardTabShell
            loading={
                isLoading || accessLoading ? (
                    <div className="flex justify-center items-center py-16">
                        <PrimarySpinner />
                    </div>
                ) : null
            }
            locked={
                !accessLoading && !hasAccess ? (
                    <DashboardLockedTabView
                        title={tStringSettings("businessPage.title")}
                        subtitle={tStringSettings("businessPage.enableDescription")}
                        businessId={businessId}
                    />
                ) : null
            }
            header={{
                title: tStringSettings("businessPage.title"),
                // Honest subtitle: "Create a public page…" only while draft.
                // "Published" only when the page is routable: the publish gate
                // (ResolveHome, /b/<slug>) also needs a saved custom_url.
                subtitle: loadFailed
                    ? tStringSettings("businessPage.unknownStatusDescription")
                    : !settings.enabled
                    ? tStringSettings("businessPage.enableDescription")
                    : customUrlSlug
                    ? (
                          tStringSettings("businessPage.publishedDescriptionWithUrl") ||
                          "Your public page is live at /b/{slug}. Manage content, hours, and reviews shown to guests."
                      ).replace("{slug}", customUrlSlug)
                    : tStringSettings("businessPage.needsUrlDescription") ||
                      "Published, but guests cannot reach it until you save a Custom URL Slug.",
                status: {
                    label: loadFailed
                        ? tStringSettings("businessPage.status.unknown") || "Status unavailable"
                        : !settings.enabled
                        ? tStringSettings("businessPage.status.draft") || "Draft"
                        : customUrlSlug
                        ? tStringSettings("businessPage.status.published") || "Published"
                        : tStringSettings("businessPage.status.needsUrl") || "Needs a URL",
                    tone: loadFailed || (settings.enabled && !customUrlSlug)
                        ? "attention"
                        : settings.enabled
                          ? "positive"
                          : "neutral",
                },
                actions: (
                    <>
                        {/* Header stays non-destructive: Preview / Set URL.
                            Unpublish lives in the Essentials danger zone (#214). */}
                        {settings.enabled && publicPagePath ? (
                            <a
                                href={publicPagePath}
                                target="_blank"
                                rel="noopener noreferrer"
                                className="inline-flex h-10 min-w-20 items-center justify-center gap-2 rounded-medium border border-brand/30 bg-brand text-white px-4 text-small font-medium transition-colors hover:bg-brand-dark focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand"
                            >
                                <ExternalLink className="w-4 h-4" aria-hidden="true" />
                                {tStringSettings("businessPage.button.preview") || "Preview page"}
                            </a>
                        ) : !publicPagePath ? (
                            <button
                                type="button"
                                onClick={() => setActiveTab("essentials")}
                                className="inline-flex h-10 min-w-20 items-center justify-center gap-2 rounded-medium border border-warm-200 bg-white/80 px-4 text-small font-medium text-ink-700 transition-colors hover:bg-white focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand"
                            >
                                <ExternalLink className="w-4 h-4" aria-hidden="true" />
                                {tStringSettings("businessPage.button.setUrl") || "Set your page URL"}
                            </button>
                        ) : (
                            // Draft-with-slug residual (L6-27): public URL exists
                            // but page is unpublished. Preview link fixed in
                            // 834be07fc/d03e1eae5 for published; this branch was empty.
                            <div className="flex flex-wrap items-center gap-2">
                                <button
                                    type="button"
                                    disabled
                                    title={
                                        tStringSettings("businessPage.button.previewDraftTooltip") ||
                                        "Publish to open the public page"
                                    }
                                    aria-label={
                                        tStringSettings("businessPage.button.previewDraftDisabled") ||
                                        "Publish to open the public page"
                                    }
                                    className="inline-flex h-10 min-w-20 cursor-not-allowed items-center justify-center gap-2 rounded-medium border border-warm-200 bg-warm-50 px-4 text-small font-medium text-ink-400 opacity-80"
                                    data-testid="draft-preview-disabled"
                                >
                                    <ExternalLink className="w-4 h-4" aria-hidden="true" />
                                    {tStringSettings("businessPage.button.preview") || "Preview page"}
                                </button>
                                <button
                                    type="button"
                                    onClick={() => setActiveTab("design")}
                                    className="inline-flex h-10 min-w-20 items-center justify-center gap-2 rounded-medium border border-warm-200 bg-white/80 px-4 text-small font-medium text-ink-700 transition-colors hover:bg-white focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand"
                                    data-testid="draft-design-preview-link"
                                >
                                    {tStringSettings("businessPage.button.previewLiveDesign") ||
                                        "Open design live preview"}
                                </button>
                            </div>
                        )}
                    </>
                ),
            }}
            tabs={{
                items: tabs,
                activeKey: activeTab,
                onChange: (key: string) => {
                    if ((BUSINESS_PAGE_SECTIONS as readonly string[]).includes(key)) {
                        setActiveTab(key as BusinessPageSection);
                    }
                },
                ariaLabel:
                    tStringSettings("businessPage.tabs.ariaLabel") ||
                    "Business page sections",
            }}
        >

            {/* Enable/Disable Toggle Card — never invite publish on a failed load. */}
            {!loadFailed ? (
            <BusinessPageToggle
                enabled={settings.enabled}
                onToggle={handleTogglePage}
                variant="card"
            />
            ) : null}

            {loadFailed && (
                <div
                    role="alert"
                    className="mb-4 flex items-center justify-between gap-4 rounded-2xl border border-red-200 bg-red-50 p-4"
                >
                    <p className="text-sm text-red-800">
                        {tStringSettings("businessPage.toasts.loadPageError")}
                    </p>
                    <Button
                        size="sm"
                        variant="bordered"
                        className="border-red-300 text-red-800"
                        onPress={() => void loadData()}
                    >
                        {tStringSettings("businessPage.retryLoad")}
                    </Button>
                </div>
            )}

            {/* Content editor is always available — including for drafts.
                Publish is a separate minimal toggle (handleTogglePage) and does
                not gate editing. The public /b/{slug} route still 404s while
                unpublished. The tab strip lives in the shell's SegmentedTabs
                slot; this panel holds only the active section's form content. */}
            <PremiumPanel withTexture={false}>
                    <div className="p-4 sm:p-6">
                        <DashboardTabTransition tabKey={activeTab}>
                        {/* Essentials Tab */}
                        {activeTab === "essentials" && (
                            <div className="space-y-8 w-full">
                                <div className="space-y-6">
                                    <div className="flex items-center gap-3 mb-2">
                                        <IconTile icon={Globe} />
                                        <div>
                                            <h3 className={sectionTitleClass}>
                                                {tStringSettings("businessPage.essentialsTitle") || "Basic Information"}
                                            </h3>
                                            <p className={sectionDescriptionClass}>
                                                {tStringSettings("businessPage.essentialsDescription") || "URL slug and short pitch shown on your public page"}
                                            </p>
                                        </div>
                                    </div>
                                    <CustomURLInput
                                        value={profile.custom_url || ""}
                                        onChange={(value) => { setProfile({ ...profile, custom_url: value }); markDirty("profile"); }}
                                        businessId={businessId}
                                        label={tStringSettings("businessPage.customUrlLabel")}
                                        placeholder={tStringSettings("businessPage.customUrlPlaceholder")}
                                        description={tStringSettings("businessPage.customUrlDescription")}
                                        savedUrl={settings.custom_url}
                                        onAvailabilityChange={handleSlugAvailabilityChange}
                                    />

                                    <Textarea
                                        label={tStringSettings("businessPage.aboutLabel")}
                                        placeholder={tStringSettings("businessPage.aboutPlaceholder")}
                                        value={profile.description || ""}
                                        onValueChange={(value) => { setProfile({ ...profile, description: value }); markDirty("profile"); }}
                                        size="lg"
                                        variant="bordered"
                                        minRows={4}
                                        classNames={{
                                            inputWrapper: "bg-white border-warm-200 data-[hover=true]:border-brand/30 group-data-[focus=true]:border-brand",
                                            label: "text-ink-700",
                                            input: "text-ink-900",
                                        }}
                                    />
                                </div>

                                <Divider className="my-6" />
                                {/* INT-1: phone/website/address/social moved to the
                                    dedicated Contact tab. Essentials keeps identity only. */}
                                <div className="space-y-6">
                                    <div className="flex items-center gap-3 mb-2">
                                        <IconTile icon={Globe} />
                                        <div>
                                            <h3 className={sectionTitleClass}>
                                                {tStringSettings("businessPage.storyTitle") || "Welcome & story"}
                                            </h3>
                                            <p className={sectionDescriptionClass}>
                                                {tStringSettings("businessPage.storyDescription") || "Optional greeting and longer story shown on your page"}
                                            </p>
                                        </div>
                                    </div>
                                    <div className="space-y-3">
                                        <Switch
                                            isSelected={settings.show_welcome_message}
                                            onValueChange={(v) => { setSettings((p) => ({ ...p, show_welcome_message: v })); markDirty("profile"); }}
                                        >
                                            {tStringSettings("businessPage.showWelcomeMessage") || "Show welcome message"}
                                        </Switch>
                                        <Textarea
                                            label={tStringSettings("businessPage.welcomeMessageLabel") || "Welcome message"}
                                            value={settings.welcome_message || ""}
                                            onValueChange={(v) => { setSettings((p) => ({ ...p, welcome_message: v })); markDirty("profile"); }}
                                            variant="bordered"
                                            minRows={2}
                                        />
                                    </div>
                                    <div className="space-y-3">
                                        <Switch
                                            isSelected={settings.show_about_story}
                                            onValueChange={(v) => { setSettings((p) => ({ ...p, show_about_story: v })); markDirty("profile"); }}
                                        >
                                            {tStringSettings("businessPage.showAboutStory") || "Show about story"}
                                        </Switch>
                                        <Textarea
                                            label={tStringSettings("businessPage.aboutStoryLabel") || "About story"}
                                            value={settings.about_story || ""}
                                            onValueChange={(v) => { setSettings((p) => ({ ...p, about_story: v })); markDirty("profile"); }}
                                            variant="bordered"
                                            minRows={4}
                                        />
                                    </div>
                                </div>

                                {/* #214: unpublish lives in Essentials, not beside Preview. */}
                                {settings.enabled ? (
                                    <div
                                        data-testid="business-page-danger-zone"
                                        className="rounded-2xl border border-rose-200 bg-rose-50/50 p-4 space-y-4"
                                    >
                                        <div className="flex items-start gap-3">
                                            <IconTile icon={AlertTriangle} />
                                            <div className="min-w-0 flex-1">
                                                <h3 className={sectionTitleClass}>
                                                    {tStringSettings("businessPage.dangerZone.title") ||
                                                        "Unpublish this page"}
                                                </h3>
                                                <p className={sectionDescriptionClass}>
                                                    {tStringSettings("businessPage.dangerZone.description") ||
                                                        "Takes the public page offline. Table QR codes that open this page will stop working until you publish again."}
                                                </p>
                                            </div>
                                        </div>
                                        <div className="flex items-center justify-end">
                                            <BusinessPageToggle
                                                enabled={settings.enabled}
                                                onToggle={handleTogglePage}
                                                variant="button"
                                            />
                                        </div>
                                    </div>
                                ) : null}
                            </div>
                        )}

                        {/* Look & feel Tab (IA-2): merged Design + Visuals —
                            colors/fonts/layout (incl. Phase-2 menu_layout/header_style)
                            ON TOP, then banner + gallery. All state/handlers are
                            component-level and shared; the merge is JSX-only. */}
                        {activeTab === "design" && (
                            <div className="space-y-8 w-full">
                                <DesignCustomization
                                    businessId={businessId}
                                    designSettings={designSettings}
                                    onDesignSettingsChange={handleDesignSettingsChange}
                                    customUrl={profile.custom_url}
                                    previewModel={{
                                        name: profile.name,
                                        logo: profile.logo,
                                        description: profile.description,
                                        phone: profile.phone,
                                        website: profile.website,
                                        address: profile.address,
                                        social_media: profile.social_media,
                                        banner_images: settings.banner_images,
                                        operatingHours,
                                        operatingExceptions,
                                        showOperatingHours: settings.show_operating_hours,
                                        timezone: profile.timezone,
                                        customUrl: profile.custom_url,
                                        // About tab content + section toggles so the
                                        // preview shows the same panel guests land on.
                                        welcomeMessage: settings.welcome_message,
                                        aboutStory: settings.about_story,
                                        showWelcomeMessage: settings.show_welcome_message,
                                        showAboutStory: settings.show_about_story,
                                        showGallery: settings.show_gallery,
                                        showSpecialFeatures: settings.show_special_features,
                                        galleryImages,
                                        specialFeatures,
                                        showReviews: settings.show_reviews,
                                        googleReviewsEnabled: settings.google_reviews_enabled,
                                        googlePlaceId: profile.google_place_id,
                                        googleBusinessName: profile.google_business_name,
                                        googleBusinessUrl: profile.google_business_url,
                                        googleReviewLink: profile.google_review_link,
                                        defaultCurrency: storefrontDefaults.default_currency,
                                        displayCurrency: storefrontDefaults.display_currency,
                                        defaultLanguage: storefrontDefaults.default_language,
                                    }}
                                />

                                <Divider className="my-6" />

                                <div className="space-y-6">
                                    <div className="flex items-center gap-3 mb-4">
                                        <IconTile icon={ImagePlus} />
                                        <div>
                                            <h3 className={sectionTitleClass}>
                                                {tStringSettings("businessPage.logoTitle")}
                                            </h3>
                                            <p className={sectionDescriptionClass}>
                                                {tStringSettings("businessPage.logoDescription")}
                                            </p>
                                        </div>
                                    </div>
                                    <SimpleImageUpload
                                        currentImage={profile.logo}
                                        onImageUploaded={handleLogoUpload}
                                        businessId={businessId}
                                        type="business-logo"
                                        maxSize={2}
                                    />
                                </div>

                                <Divider className="my-6" />

                                <div className="space-y-6">
                                    <div className="flex items-center gap-3 mb-4">
                                        <IconTile icon={Paintbrush} />
                                        <div>
                                            <h3 className={sectionTitleClass}>
                                                {tStringSettings("businessPage.bannerTitle")}
                                            </h3>
                                            <p className={sectionDescriptionClass}>
                                                {tStringSettings("businessPage.bannerDescription")}
                                            </p>
                                        </div>
                                    </div>

                                    <BannerImageUploader
                                        bannerImages={bannerImages}
                                        onBannersChange={handleBannerUpload}
                                        businessId={businessId}
                                        maxImages={3}
                                        maxSize={10}
                                    />
                                </div>

                                <Divider className="my-6" />

                                <div className="space-y-6">
                                    <div className="flex items-center justify-between">
                                        <div className="flex items-center gap-3">
                                            <IconTile icon={Camera} />
                                            <div>
                                                <h3
                                                    id={`${settingsA11yId}-gallery-label`}
                                                    className={sectionTitleClass}
                                                >
                                                    {tStringSettings("businessPage.photoGalleryTitle")}
                                                </h3>
                                                <p
                                                    id={`${settingsA11yId}-gallery-desc`}
                                                    className={sectionDescriptionClass}
                                                >
                                                    {tStringSettings("businessPage.showPhotoGalleryDescription")}
                                                </p>
                                            </div>
                                        </div>
                                        <NamedSwitch
                                            name={tStringSettings("businessPage.photoGalleryTitle")}
                                            labelId={`${settingsA11yId}-gallery-label`}
                                            descriptionId={`${settingsA11yId}-gallery-desc`}
                                            isSelected={settings.show_gallery || false}
                                            onValueChange={(value) => { setSettings({ ...settings, show_gallery: value }); markDirty("profile"); }}
                                            color="secondary"
                                        />
                                    </div>

                                    {/* IA-7 #2: always render the editor — the
                                        Show switch above controls only PUBLIC
                                        visibility (the storefront honors
                                        show_gallery independently). */}
                                    <div className="animate-in fade-in slide-in-from-top-4 duration-300">
                                        <GalleryImageUploader
                                            businessId={businessId}
                                            images={galleryImages}
                                            onImagesChange={handleGalleryImagesChange}
                                        />
                                    </div>
                                    <p className={hintClass}>
                                        {tStringSettings("businessPage.showOnPublicPageHint")}
                                    </p>
                                </div>
                            </div>
                        )}

                        {/* Operations Tab */}
                        {activeTab === "operations" && (
                            <div className="space-y-8 w-full">
                                <div className="space-y-6">
                                    <div className="flex items-center gap-3 mb-4">
                                        <IconTile icon={Clock} />
                                        <div>
                                            <h3 className={sectionTitleClass}>
                                                {tStringSettings("businessPage.operatingHoursTitle")}
                                            </h3>
                                            <p className={sectionDescriptionClass}>
                                                {tStringSettings("businessPage.operatingHoursDescription") || "Manage your business hours and timezone"}
                                            </p>
                                        </div>
                                    </div>

                                    <div className="grid grid-cols-1 md:grid-cols-2 gap-6 rounded-3xl border border-warm-200/80 bg-warm-50/70 p-6 shadow-sm shadow-warm-900/5">
                                        <div className="space-y-2">
                                            {/* M13: empty selection keeps the field blank until the
                                                operator explicitly picks a timezone. */}
                                            <Select
                                                label={tStringSettings("businessPage.timezoneLabel") || "Business timezone"}
                                                aria-label={tStringSettings("businessPage.timezoneLabel") || "Business timezone"}
                                                placeholder={tStringSettings("businessPage.timezonePlaceholder") || "Choose a timezone…"}
                                                selectedKeys={profile.timezone ? [profile.timezone] : []}
                                                onSelectionChange={(keys) => {
                                                    const next = Array.from(keys)[0] as string | undefined;
                                                    if (next == null) return;
                                                    setProfile({ ...profile, timezone: next });
                                                    markDirty("profile");
                                                }}
                                                variant="bordered"
                                                labelPlacement="outside"
                                                classNames={{ trigger: "bg-white" }}
                                            >
                                                {TIMEZONE_OPTIONS.map((tz) => (
                                                    <SelectItem
                                                        key={tz.value}
                                                        textValue={`${tz.label} (${tz.offset})`}
                                                    >
                                                        {tz.label} ({tz.offset})
                                                    </SelectItem>
                                                ))}
                                            </Select>
                                        </div>
                                        <div className="flex items-center justify-between rounded-2xl border border-warm-200 bg-white p-4 shadow-sm shadow-warm-900/5">
                                            <div>
                                                <span
                                                    id={`${settingsA11yId}-hours-label`}
                                                    className="font-semibold text-ink-950 text-sm"
                                                >
                                                    {tStringSettings("businessPage.showOnPage") || "Show on page"}
                                                </span>
                                                <p
                                                    id={`${settingsA11yId}-hours-desc`}
                                                    className="text-xs text-ink-600"
                                                >
                                                    {tStringSettings("businessPage.visibleToCustomers") || "Visible to customers"}
                                                </p>
                                            </div>
                                            <NamedSwitch
                                                name={tStringSettings("businessPage.showOnPage") || "Show on page"}
                                                labelId={`${settingsA11yId}-hours-label`}
                                                descriptionId={`${settingsA11yId}-hours-desc`}
                                                isSelected={settings.show_operating_hours || false}
                                                onValueChange={(value) => { setSettings({ ...settings, show_operating_hours: value }); markDirty("profile"); }}
                                            />
                                        </div>
                                    </div>

                                    {/* IA-7 #2: always editable; the Show switch
                                        controls only PUBLIC visibility. */}
                                    <div className="mt-4 animate-in fade-in slide-in-from-top-4 duration-300">
                                        <OperatingHoursEditor
                                            businessId={businessId}
                                            hours={operatingHours}
                                            onHoursChange={handleOperatingHoursChange}
                                            exceptions={operatingExceptions}
                                            onExceptionsChange={handleOperatingExceptionsChange}
                                        />
                                    </div>
                                    <p className={hintClass}>
                                        {tStringSettings("businessPage.showOnPublicPageHint")}
                                    </p>
                                </div>

                                <Divider className="my-6" />

                                <div className="space-y-6">
                                    <div className="flex items-center justify-between">
                                        <div className="flex items-center gap-3">
                                            <IconTile icon={Award} />
                                            <div>
                                                <h3
                                                    id={`${settingsA11yId}-features-label`}
                                                    className={sectionTitleClass}
                                                >
                                                    {tStringSettings("businessPage.specialFeaturesTitle")}
                                                </h3>
                                                <p
                                                    id={`${settingsA11yId}-features-desc`}
                                                    className={sectionDescriptionClass}
                                                >
                                                    {tStringSettings("businessPage.showSpecialFeaturesDescription")}
                                                </p>
                                            </div>
                                        </div>
                                        <NamedSwitch
                                            name={tStringSettings("businessPage.specialFeaturesTitle")}
                                            labelId={`${settingsA11yId}-features-label`}
                                            descriptionId={`${settingsA11yId}-features-desc`}
                                            isSelected={settings.show_special_features || false}
                                            onValueChange={(value) => { setSettings({ ...settings, show_special_features: value }); markDirty("profile"); }}
                                        />
                                    </div>

                                    {/* IA-7 #2: always editable; the Show switch
                                        controls only PUBLIC visibility. */}
                                    <div className="animate-in fade-in slide-in-from-top-4 duration-300">
                                        <SpecialFeaturesEditor
                                            businessId={businessId}
                                            features={specialFeatures}
                                            onFeaturesChange={handleSpecialFeaturesChange}
                                        />
                                    </div>
                                    <p className={hintClass}>
                                        {tStringSettings("businessPage.showOnPublicPageHint")}
                                    </p>
                                </div>
                            </div>
                        )}

                        {/* Reviews Tab */}
                        {activeTab === "reviews" && (
                            <div className="space-y-8 w-full">
                                <div className="space-y-6">
                                    <div className="flex items-center justify-between mb-6">
                                        <div className="flex items-center gap-3">
                                            <IconTile icon={Star} />
                                            <div>
                                                <h3
                                                    id={`${settingsA11yId}-reviews-label`}
                                                    className={sectionTitleClass}
                                                >
                                                    {tStringSettings("businessPage.reviewsTitle")}
                                                </h3>
                                                <p
                                                    id={`${settingsA11yId}-reviews-desc`}
                                                    className={sectionDescriptionClass}
                                                >
                                                    {tStringSettings("businessPage.showReviewsDescription")}
                                                </p>
                                            </div>
                                        </div>
                                        <NamedSwitch
                                            name={tStringSettings("businessPage.reviewsTitle")}
                                            labelId={`${settingsA11yId}-reviews-label`}
                                            descriptionId={`${settingsA11yId}-reviews-desc`}
                                            isSelected={settings.show_reviews}
                                            onValueChange={(value) => { setSettings({ ...settings, show_reviews: value }); markDirty("profile"); }}
                                        />
                                    </div>

                                    {/* Providers driven by enabled review plugins
                                        (plus first-party Google) — never hardcode
                                        Google-only. */}
                                    <div className="space-y-6" data-testid="review-providers">
                                        {reviewProviders.map((provider) => {
                                            if (provider.id === "google") {
                                                return (
                                                    <div
                                                        key="google"
                                                        data-testid="review-provider-google"
                                                    >
                                                        <GoogleBusinessSearch
                                                            businessId={businessId.toString()}
                                                            businessName={profile.name}
                                                            businessAddress={`${profile.address.street}, ${profile.address.city}, ${profile.address.state} ${profile.address.postal_code}`}
                                                            currentGoogleInfo={{
                                                                google_place_id: profile.google_place_id,
                                                                google_business_name: profile.google_business_name,
                                                                google_review_link: profile.google_review_link,
                                                                google_business_url: profile.google_business_url,
                                                                google_reviews_enabled: profile.google_reviews_enabled,
                                                            }}
                                                            onUpdate={handleGoogleInfoMerge}
                                                        />
                                                    </div>
                                                );
                                            }
                                            if (provider.id === "trustpilot") {
                                                const url = trustpilotConfig?.trustpilot_url;
                                                const name = trustpilotConfig?.business_name;
                                                return (
                                                    <div
                                                        key="trustpilot"
                                                        data-testid="review-provider-trustpilot"
                                                        className="rounded-2xl border border-warm-200/80 bg-white p-5 shadow-sm shadow-warm-900/5"
                                                    >
                                                        <div className="flex items-start gap-3">
                                                            <IconTile icon={Star} />
                                                            <div className="min-w-0 flex-1 space-y-1">
                                                                <h4 className="text-base font-semibold text-ink-950">
                                                                    {tStringSettings("businessPage.reviewProviders.trustpilot") ||
                                                                        "Trustpilot"}
                                                                </h4>
                                                                <p className="text-sm text-ink-600">
                                                                    {name || url
                                                                        ? tStringSettings(
                                                                              "businessPage.reviewProviders.trustpilotConnected",
                                                                          ) ||
                                                                          "Connected via the Trustpilot plugin."
                                                                        : tStringSettings(
                                                                              "businessPage.reviewProviders.trustpilotEnabled",
                                                                          ) ||
                                                                          "Trustpilot plugin is enabled for this business."}
                                                                </p>
                                                                {name ? (
                                                                    <p className="text-sm font-medium text-ink-900">
                                                                        {name}
                                                                    </p>
                                                                ) : null}
                                                                {url ? (
                                                                    <a
                                                                        href={url}
                                                                        target="_blank"
                                                                        rel="noopener noreferrer"
                                                                        className="inline-flex items-center gap-1.5 text-sm font-medium text-brand hover:underline"
                                                                    >
                                                                        <ExternalLink className="h-3.5 w-3.5" aria-hidden />
                                                                        {tStringSettings(
                                                                            "businessPage.reviewProviders.viewTrustpilot",
                                                                        ) || "View Trustpilot profile"}
                                                                    </a>
                                                                ) : (
                                                                    <p className="text-xs text-ink-500">
                                                                        {tStringSettings(
                                                                            "businessPage.reviewProviders.configureInPlugins",
                                                                        ) ||
                                                                            "Finish configuration under Plugins → Trustpilot."}
                                                                    </p>
                                                                )}
                                                            </div>
                                                        </div>
                                                    </div>
                                                );
                                            }
                                            return null;
                                        })}
                                    </div>
                                </div>
                            </div>
                        )}

                        {/* Contact Tab (INT-1) — dedicated tab per the spec IA.
                            Phone / website / address / social, persisted via the
                            existing handleSave → updateBusiness path (same Business
                            columns as Settings → single source of truth). Email is
                            a deferred Phase-4 follow-up (needs a backend column). */}
                        {activeTab === "contact" && (
                            <div className="space-y-8 w-full">
                                <ContactEditor
                                    phone={profile.phone || ""}
                                    website={profile.website || ""}
                                    address={profile.address}
                                    socialMedia={profile.social_media || {}}
                                    onPhoneChange={handlePhoneChange}
                                    onWebsiteChange={handleWebsiteChange}
                                    onAddressChange={handleAddressChange}
                                    onSocialChange={handleSocialMediaChange}
                                    t={tStringSettings}
                                />
                            </div>
                        )}
                        </DashboardTabTransition>
                    </div>
                </PremiumPanel>
            {/* SaveBar always mounted so draft operators can persist content
                without publishing first; dirty flag still gates the active CTA. */}
            <SaveBar
                mode="button"
                isSaving={isSaving}
                dirty={isDirty && !loadFailed}
                onSave={handleSave}
                labels={{
                  save: tStringSettings("saveBar.save"),
                  saving: tStringSettings("saveBar.saving"),
                  unsaved: tStringSettings("saveBar.unsaved"),
                  auto: tStringSettings("saveBar.auto"),
                  clean: tStringSettings("saveBar.clean"),
                }}
            />
        </DashboardTabShell>
    );
}
