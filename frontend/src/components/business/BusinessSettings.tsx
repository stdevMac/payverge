"use client";

import dynamic from "next/dynamic";
import React, { useState, useEffect, useCallback, useRef } from "react";
import {
  Button,
  Divider,
  Input,
  Select,
  SelectItem,
  useDisclosure,
} from "@nextui-org/react";
import {
  CreditCard,
  Upload,
  Building2,
  Languages,
  Bell,
  AlertTriangle,
} from "lucide-react";
import {
  businessApi,
  BusinessAddress,
  UpdateBusinessRequest,
} from "@/api/business";
import { SettingsSkeleton } from "./SettingsSkeleton";
import SimpleImageUpload from "./SimpleImageUpload";
import { KitchenOrdersToggle } from "./KitchenOrdersToggle";
import SaveBar from "./SaveBar";
import ConfirmationModal from "./modals/ConfirmationModal";

import { useToast } from "@/contexts/ToastContext";
import CurrencySettings, { CurrencySettingsHandle } from "./CurrencySettings";

import { useBusinessAccess } from "@/hooks/useBusinessAccess";
import { useBusinessUrlId } from "@/hooks/useBusinessUrlId";
import { useDirtyForm } from "@/hooks/useDirtyForm";
import { useUnsavedChangesGuard } from "@/hooks/useUnsavedChangesGuard";
import DashboardLockedTabView from "./DashboardLockedTabView";
import {
  useSimpleLocale,
  getTranslation,
} from "@/i18n/SimpleTranslationProvider";
import { BUSINESS_TYPE_OPTIONS } from "@/lib/geoDefaults";
import { useRouter } from "next/navigation";
import Link from "next/link";
import {
  getBusinessPageEditorPath,
  SETTINGS_SECTIONS,
  type SettingsSection,
} from "@/utils/businessUrl";
import { useUrlState } from "@/hooks/useUrlState";
import DashboardTabShell from "./shared/DashboardTabShell";
import IconTile from "@/components/ui/IconTile";
import { DashboardTabTransition, PremiumPanel } from "./premium";
import { useAuth } from "@/providers/HybridAuthProvider";
import { getSafeApiErrorMessage } from "@/utils/apiError";

interface BusinessSettingsProps {
  businessId: number;
}

interface BusinessProfile {
  name: string;
  logo: string;
  address: BusinessAddress;
  settlement_address: string;
  tipping_address: string;
  tax_rate: number;
  service_fee_rate: number;
  tax_inclusive: boolean;
  service_inclusive: boolean;
  // Contact (phone/website/address) is edited on Business Page → Contact;
  // Settings Profile shows them read-only. Keep values for that display.
  phone?: string;
  website?: string;
  business_type?: string;
}

const PaymentSettingsTab = dynamic(
  () => import("./BusinessPaymentSettingsTab"),
  {
    ssr: false,
  },
);

const NotificationPreferencesTab = dynamic(
  () => import("./NotificationPreferencesTab"),
  { ssr: false },
);

export default function BusinessSettings({
  businessId,
}: BusinessSettingsProps) {
  const { locale: currentLocale } = useSimpleLocale();
  const { isStaffUser } = useAuth();

  // Translation helper — read the LIVE locale so labels react to language
  // switches without a remount/refresh.
  const tString = useCallback(
    (key: string): string => {
      const fullKey = `businessSettings.${key}`;
      const result = getTranslation(fullKey, currentLocale);
      return Array.isArray(result) ? result[0] || key : (result as string);
    },
    [currentLocale],
  );

  // Every tab on this screen mutates business settings through an endpoint
  // that refuses suspended / closed businesses. Fail closed while the lock
  // state is loading so a locked business never gets an enabled save action.
  const {
    hasAccess,
    isSuspended,
    loading: accessLoading,
  } = useBusinessAccess(businessId);
  const gated = accessLoading || !hasAccess || isSuspended;

  // L6-41 / Root D: `?section=` is the source of the settings sub-tab.
  // Write-back on every in-page change; invalid values normalize to profile
  // and rewrite the URL. (Deleted the old "read once so later clicks aren't
  // fought by the URL" rationale — with write-back the URL is authoritative.)
  const [activeTab, setActiveTab] = useUrlState({
    key: "section",
    valid: SETTINGS_SECTIONS,
    fallback: "profile" as SettingsSection,
    // Settings and Business Page share `?section=`. Do not rewrite while the
    // URL already names another tab — otherwise a still-mounted Settings
    // treats `section=contact` as garbage, omits it, and the Contact
    // deep-link lands on Essentials (#225).
    activeWhen: { key: "tab", values: ["settings"] },
  });
  const { showSuccess, showError } = useToast();

  const [profile, setProfile] = useState<BusinessProfile>({
    name: "",
    logo: "",
    address: {
      street: "",
      city: "",
      state: "",
      postal_code: "",
      country: "",
    },
    settlement_address: "",
    tipping_address: "",
    tax_rate: 0,
    service_fee_rate: 0,
    tax_inclusive: false,
    service_inclusive: false,
    phone: "",
    website: "",
    business_type: "other",
  });

  const [isLoading, setIsLoading] = useState(false);
  const [isSaving, setIsSaving] = useState(false);
  // Deep-compare profile against the last loaded/saved snapshot so reverting
  // an edit clears dirty (and Save disables via SaveBar's isDisabled).
  const { dirty: isDirty, markClean } = useDirtyForm(profile);

  // H3: the payout addresses as loaded from the backend. Used to detect when a
  // PREVIOUSLY NON-EMPTY address is being changed, which requires an explicit
  // confirmation before the save proceeds.
  const [originalPayoutAddresses, setOriginalPayoutAddresses] = useState<{
    settlement_address: string;
    tipping_address: string;
  }>({ settlement_address: "", tipping_address: "" });
  const addressChangeDisclosure = useDisclosure();

  // H3/M17: authoritative payment-settings validity, computed from the shared
  // profile so it holds regardless of whether the Payments tab is mounted (the
  // child tab renders its own inline isInvalid states from the same values).
  // A non-empty payout address must match the canonical EVM shape, and rates
  // must sit within [0, 100].
  const paymentValid = React.useMemo(() => {
    const evm = /^0x[a-fA-F0-9]{40}$/;
    const settlement = profile.settlement_address.trim();
    const tipping = profile.tipping_address.trim();
    const settlementOk = settlement.length === 0 || evm.test(settlement);
    const tippingOk = tipping.length === 0 || evm.test(tipping);
    // Tips must not share the sales settlement wallet — tip-pool reporting
    // becomes unreconcilable when both addresses are identical.
    const walletsDistinct =
      settlement.length === 0 ||
      tipping.length === 0 ||
      settlement.toLowerCase() !== tipping.toLowerCase();
    const rateOk = (r: number) => Number.isFinite(r) && r >= 0 && r <= 100;
    return (
      settlementOk &&
      tippingOk &&
      walletsDistinct &&
      rateOk(profile.tax_rate) &&
      rateOk(profile.service_fee_rate)
    );
  }, [
    profile.settlement_address,
    profile.tipping_address,
    profile.tax_rate,
    profile.service_fee_rate,
  ]);

  // IMP-30: the localization tab is owned by CurrencySettings — the
  // page-header "Save All Changes" delegates to it via a ref so we keep
  // a single canonical save CTA on this page.
  const currencyRef = useRef<CurrencySettingsHandle | null>(null);
  const [localizationDirty, setLocalizationDirty] = useState(false);

  // H2a: profile/payments dirty stays registered even on other Settings
  // sub-tabs so a sidebar leave still prompts. CurrencySettings registers
  // its own guard. Notifications auto-save and must not inherit this flag
  // into the footer (#224 / #268).
  useUnsavedChangesGuard(isDirty, "business-settings");

  // H3/M17: the SaveBar reflects dirtiness, but the button-mode tabs (profile /
  // payments) that persist via handleSave must not offer Save while the payment
  // address/rate fields are invalid. Localization saves via its own path, so it
  // is unaffected. Notifications auto-save — never show the profile dirty
  // pill on that sub-tab. The registry above still tracks profile dirty so a
  // beforeunload / sidebar leave can prompt.
  const saveBarDirty =
    activeTab === "notifications"
      ? false
      : activeTab === "localization"
        ? localizationDirty
        : isDirty && paymentValid;

  const loadBusinessProfile = useCallback(async () => {
    try {
      setIsLoading(true);
      const business = await businessApi.getBusiness(businessId);
      if (business) {
        const next: BusinessProfile = {
          name: business.name || "",
          logo: business.logo || "",
          address: business.address || {
            street: "",
            city: "",
            state: "",
            postal_code: "",
            country: "",
          },
          settlement_address: business.settlement_address || "",
          tipping_address: business.tipping_address || "",
          tax_rate: business.tax_rate || 0,
          service_fee_rate: business.service_fee_rate || 0,
          tax_inclusive: business.tax_inclusive || false,
          service_inclusive: business.service_inclusive || false,
          // Contact display (edited on Business Page → Contact)
          phone: business.phone || "",
          website: business.website || "",
          business_type: business.business_type || "other",
        };
        setProfile(next);
        markClean(next);

        // H3: remember the payout addresses as loaded so we can detect a
        // change to a previously-non-empty address before saving.
        setOriginalPayoutAddresses({
          settlement_address: business.settlement_address || "",
          tipping_address: business.tipping_address || "",
        });

        // Load business page settings

        // Load hospitality data
      }
    } catch (err) {
      const message = getSafeApiErrorMessage(
        err,
        tString("messages.loadProfileError"),
      );
      showError(tString("messages.error"), message);
    } finally {
      setIsLoading(false);
    }
  }, [businessId, showError, tString, markClean]);

  useEffect(() => {
    loadBusinessProfile();
  }, [loadBusinessProfile]);

  // Actual persistence for the profile / payments tabs. Kept separate from the
  // Save-CTA entry point so the H3 address-change confirmation can gate it.
  // Stream 9 #5: profile and payments submit disjoint field sets so saving one
  // tab never re-submits the other's mount-time snapshot (last-save-wins race).
  // Phone/website/address live on Business Page → Contact only.
  const persistProfile = async (
    section: "profile" | "payments" = "profile",
  ) => {
    try {
      setIsSaving(true);

      // P2-23: send ONLY the fields this screen section edits. The backend PUT
      // is a pointer-based partial patch (UpdateBusiness), so omitted fields
      // are untouched server-side.
      const updateData: UpdateBusinessRequest =
        section === "payments"
          ? {
              tax_rate: profile.tax_rate,
              service_fee_rate: profile.service_fee_rate,
              tax_inclusive: profile.tax_inclusive,
              service_inclusive: profile.service_inclusive,
            }
          : {
              name: profile.name,
              logo: profile.logo,
              business_type: profile.business_type,
            };
      // P1-12: owner-only wallet fields. Staff sessions must not submit them —
      // the backend strips them anyway (stripOwnerOnlyFieldsForStaff) and
      // discloses the drop via skipped_fields; omitting keeps the payload honest.
      if (section === "payments" && !isStaffUser) {
        updateData.settlement_address = profile.settlement_address;
        updateData.tipping_address = profile.tipping_address;
      }

      const updated = await businessApi.updateBusiness(businessId, updateData);

      // P1-12: rehydrate the payout baseline from the PUT response (server
      // truth), never from local form state. Staff projections omit wallet
      // fields entirely — keep the previously loaded baseline in that case.
      if (section === "payments") {
        const serverSettlement =
          typeof updated?.settlement_address === "string"
            ? updated.settlement_address
            : originalPayoutAddresses.settlement_address;
        const serverTipping =
          typeof updated?.tipping_address === "string"
            ? updated.tipping_address
            : originalPayoutAddresses.tipping_address;
        setOriginalPayoutAddresses({
          settlement_address: serverSettlement,
          tipping_address: serverTipping,
        });
        if (!isStaffUser) {
          setProfile((prev) => {
            const next = {
              ...prev,
              settlement_address: serverSettlement,
              tipping_address: serverTipping,
            };
            markClean(next);
            return next;
          });
        } else {
          markClean();
        }
      } else {
        markClean();
      }

      const skipped = updated?.skipped_fields;
      if (skipped && skipped.length > 0) {
        // The backend accepted the save but ignored owner-only fields this
        // session may not change — never celebrate that as a full success.
        showError(
          tString("messages.someFieldsNotSavedTitle"),
          tString("messages.someFieldsNotSavedDescription"),
          6000,
        );
      } else {
        showSuccess(
          tString("messages.updateSuccess"),
          tString("messages.updateSuccessDescription"),
          4000,
        );
      }
    } catch (err) {
      const fallback = tString("messages.updateFailed");
      const errorMessage = getSafeApiErrorMessage(err, fallback);

      showError(tString("messages.updateFailedTitle"), errorMessage, 6000);
    } finally {
      setIsSaving(false);
    }
  };

  // Any change to payout wallets (including first fill) needs an explicit
  // confirm — these are the highest-trust fields on the page.
  const payoutAddressChanged =
    profile.settlement_address.trim() !==
      originalPayoutAddresses.settlement_address.trim() ||
    profile.tipping_address.trim() !==
      originalPayoutAddresses.tipping_address.trim();

  // H3: build the old → new confirmation body for whichever payout address(es)
  // changed. Falls back to an em dash when a side is empty.
  const buildAddressChangeDescription = (): string => {
    const dash = "—";
    const changedSettlement =
      profile.settlement_address.trim() !==
      originalPayoutAddresses.settlement_address.trim();
    const changedTipping =
      profile.tipping_address.trim() !==
      originalPayoutAddresses.tipping_address.trim();
    // Prefer whichever address actually changed; if both changed, settlement
    // leads the sentence and tipping is appended.
    const primaryOld = changedSettlement
      ? originalPayoutAddresses.settlement_address
      : originalPayoutAddresses.tipping_address;
    const primaryNew = changedSettlement
      ? profile.settlement_address
      : profile.tipping_address;

    let body = tString("payment.confirmAddressChangeDescription")
      .replace("{old}", primaryOld || dash)
      .replace("{new}", primaryNew || dash);

    if (changedSettlement && changedTipping) {
      body = `${body} ${tString("payment.tippingAddressLabel")}: ${
        originalPayoutAddresses.tipping_address || dash
      } → ${profile.tipping_address || dash}`;
    }
    body = `${body} ${tString("payment.confirmAddressChangeTipNotice")}`;
    return body;
  };

  const handleSave = async () => {
    const section = activeTab === "payments" ? "payments" : "profile";

    // H3/M17: never persist invalid payment settings when saving payments.
    if (section === "payments" && !paymentValid) {
      const settlement = profile.settlement_address.trim();
      const tipping = profile.tipping_address.trim();
      const sameWallet =
        settlement.length > 0 &&
        tipping.length > 0 &&
        settlement.toLowerCase() === tipping.toLowerCase();
      showError(
        tString("messages.updateFailedTitle"),
        sameWallet
          ? tString("payment.tipWalletMustDiffer")
          : tString("messages.formValidationErrors"),
        6000,
      );
      return;
    }

    // Confirm before saving any payout-wallet change (funds are unrecoverable
    // if the wrong address is saved).
    if (section === "payments" && payoutAddressChanged) {
      addressChangeDisclosure.onOpen();
      return;
    }

    await persistProfile(section);
  };

  // IMP-30: page-header save on the localization tab delegates to
  // CurrencySettings — keeps it as the single canonical save CTA on
  // this page. Per-section error toasts come from CurrencySettings
  // itself (currencyStatus / languageStatus banners).
  const handleSaveLocalization = async () => {
    if (!currencyRef.current) return;
    try {
      setIsSaving(true);
      await currencyRef.current.save();
      // Localization dirty is owned by CurrencySettings via onDirtyChange;
      // profile baseline is unchanged by a localization save.
    } finally {
      setIsSaving(false);
    }
  };

  const handleInputChange = (field: keyof BusinessProfile, value: any) => {
    setProfile((prev) => ({
      ...prev,
      [field]: value,
    }));
  };

  const tabs = [
    {
      key: "profile",
      title: tString("tabs.profile"),
      icon: Building2,
    },
    {
      key: "payments",
      title: tString("tabs.payments"),
      icon: CreditCard,
    },
    {
      key: "localization",
      title: tString("tabs.localization"),
      icon: Languages,
    },
    {
      key: "notifications",
      title: tString("notifications.tabTitle"),
      icon: Bell,
    },
  ];
  return (
    <DashboardTabShell
      loading={isLoading ? <SettingsSkeleton /> : null}
      header={{
        title: tString("title"),
        subtitle: tString("description"),
      }}
      tabs={{
        items: tabs.map((tab) => ({
          key: tab.key,
          label: tab.title,
          icon: tab.icon,
        })),
        activeKey: activeTab,
        onChange: (key: string) => {
          if ((SETTINGS_SECTIONS as readonly string[]).includes(key)) {
            setActiveTab(key as SettingsSection);
          }
        },
        ariaLabel: tString("title"),
      }}
    >
      <PremiumPanel className="p-4 md:p-6" withTexture={false}>
        <DashboardTabTransition tabKey={activeTab}>
          {activeTab === "profile" &&
            (gated ? (
              <DashboardLockedTabView
                title={tString("tabs.profile")}
                businessId={businessId}
              />
            ) : (
              <BusinessProfileTab
                profile={profile}
                handleInputChange={handleInputChange}
                businessId={businessId}
              />
            ))}
          {activeTab === "payments" &&
            (gated ? (
              <DashboardLockedTabView
                title={tString("tabs.payments")}
                businessId={businessId}
              />
            ) : (
              <PaymentSettingsTab
                profile={profile}
                handleInputChange={handleInputChange}
                walletFieldsDisabled={isStaffUser}
              />
            ))}
          {activeTab === "localization" &&
            (gated ? (
              <DashboardLockedTabView
                title={tString("tabs.localization")}
                businessId={businessId}
              />
            ) : (
              <div className="space-y-6">
                <CurrencySettings
                  ref={currencyRef}
                  businessId={businessId}
                  onSave={loadBusinessProfile}
                  onDirtyChange={setLocalizationDirty}
                />
              </div>
            ))}
          {activeTab === "notifications" &&
            (gated ? (
              <DashboardLockedTabView
                title={tString("notifications.tabTitle")}
                businessId={businessId}
              />
            ) : (
              <NotificationPreferencesTab businessId={businessId} />
            ))}
        </DashboardTabTransition>
      </PremiumPanel>
      {!gated && (
        <SaveBar
          mode={activeTab === "notifications" ? "auto" : "button"}
          isSaving={isSaving}
          dirty={saveBarDirty}
          onSave={
            activeTab === "localization" ? handleSaveLocalization : handleSave
          }
          labels={{
            save: tString("saveBar.save"),
            saving: tString("saveBar.saving"),
            unsaved: tString("saveBar.unsaved"),
            auto: tString("saveBar.auto"),
            clean: tString("saveBar.clean"),
          }}
        />
      )}
      {/* H3: confirm before overwriting an existing payout address — funds sent
          to the old address won't reach the new one. */}
      <ConfirmationModal
        isOpen={addressChangeDisclosure.isOpen}
        onOpenChange={addressChangeDisclosure.onOpenChange}
        title={tString("payment.confirmAddressChangeTitle")}
        description={buildAddressChangeDescription()}
        confirmLabel={tString("payment.confirmAddressChangeConfirm")}
        cancelLabel={tString("payment.confirmAddressChangeCancel")}
        onConfirm={() => {
          void persistProfile("payments");
        }}
        isDanger
      />
    </DashboardTabShell>
  );
}

// Tab Components
function BusinessProfileTab({
  profile,
  handleInputChange,
  businessId,
}: {
  profile: BusinessProfile;
  handleInputChange: (
    field: keyof BusinessProfile,
    value: string | number | boolean,
  ) => void;
  businessId: number;
}) {
  const { locale: currentLocale } = useSimpleLocale();
  const router = useRouter();
  const businessUrlId = useBusinessUrlId(businessId);

  // Translation helper
  const tString = (key: string): string => {
    const fullKey = `businessSettings.${key}`;
    const result = getTranslation(fullKey, currentLocale);
    return Array.isArray(result) ? result[0] || key : (result as string);
  };

  return (
    <div className="space-y-8 w-full">
      {/* Basic Information */}
      <div className="space-y-6">
        <div className="flex items-center gap-3 mb-2">
          <IconTile icon={Building2} />
          <div>
            <h3 className="text-lg font-semibold text-ink-950">
              {tString("profile.basicInfo")}
            </h3>
            <p className="text-sm text-ink-600">
              {tString("profile.basicInfoDescription") ||
                "Name, contact, and short description shown to customers"}
            </p>
          </div>
        </div>

        <Input
          label={tString("profile.businessName")}
          placeholder={tString("profile.businessNamePlaceholder")}
          value={profile.name}
          onValueChange={(value) => handleInputChange("name", value)}
          isRequired
          variant="bordered"
        />

        <Select
          label={tString("profile.businessType")}
          placeholder={tString("profile.businessTypePlaceholder")}
          aria-label={tString("profile.businessType")}
          selectedKeys={[profile.business_type || "other"]}
          onSelectionChange={(keys) => {
            // NextUI can fire an empty selection on mount. Ignoring it keeps
            // the profile form from lighting "unsaved changes" before any
            // operator edit (#224).
            const next = Array.from(keys)[0] as string | undefined;
            if (!next || next === (profile.business_type || "other")) return;
            handleInputChange("business_type", next);
          }}
          variant="bordered"
        >
          {BUSINESS_TYPE_OPTIONS.map((b) => (
            <SelectItem key={b.value}>
              {
                getTranslation(
                  `businessRegister.${b.labelKey}`,
                  currentLocale,
                ) as string
              }
            </SelectItem>
          ))}
        </Select>

        {/* Stream 9 #5: phone / website / address are owned by Business Page →
            Contact. Profile shows read-only values + a deep-link so the two
            tabs cannot last-save-wins overwrite each other. */}
        <div className="rounded-2xl border border-warm-200 bg-warm-50/60 p-4 space-y-3">
          <div className="flex flex-wrap items-start justify-between gap-3">
            <div>
              <h4 className="text-sm font-semibold text-ink-900">
                {tString("profile.contactReadOnlyTitle")}
              </h4>
              <p className="text-xs text-ink-600 mt-0.5">
                {tString("profile.contactReadOnlyDescription")}
              </p>
            </div>
            <Button
              as={Link}
              href={getBusinessPageEditorPath(
                {
                  id: businessId,
                  business_id: businessUrlId,
                },
                "contact",
              )}
              size="sm"
              variant="bordered"
              className="border-warm-300"
              data-testid="edit-contact-on-business-page"
            >
              {tString("profile.editContactOnBusinessPage")}
            </Button>
          </div>
          <dl className="grid grid-cols-1 sm:grid-cols-2 gap-3 text-sm">
            <div>
              <dt className="text-xs font-medium text-ink-500">
                {tString("profile.phoneNumber")}
              </dt>
              <dd className="text-ink-900">
                {profile.phone?.trim() || tString("profile.contactEmpty")}
              </dd>
            </div>
            <div>
              <dt className="text-xs font-medium text-ink-500">
                {tString("profile.websiteUrl")}
              </dt>
              <dd className="text-ink-900 break-all">
                {profile.website?.trim() || tString("profile.contactEmpty")}
              </dd>
            </div>
            <div className="sm:col-span-2">
              <dt className="text-xs font-medium text-ink-500">
                {tString("profile.location")}
              </dt>
              <dd className="text-ink-900">
                {[
                  profile.address.street,
                  profile.address.city,
                  profile.address.state,
                  profile.address.postal_code,
                  profile.address.country,
                ]
                  .map((p) => p?.trim())
                  .filter(Boolean)
                  .join(", ") || tString("profile.contactEmpty")}
              </dd>
            </div>
          </dl>
        </div>
      </div>

      <Divider className="my-6" />

      {/* Logo */}
      <div className="space-y-6">
        <div className="flex items-center gap-3 mb-2">
          <IconTile icon={Upload} />
          <div>
            <h3 className="text-lg font-semibold text-ink-950">
              {tString("profile.logoBranding")}
            </h3>
            <p className="text-sm text-ink-600">
              {tString("profile.logoDescription")}
            </p>
          </div>
        </div>

        <SimpleImageUpload
          currentImage={profile.logo}
          onImageUploaded={(url: string) => handleInputChange("logo", url)}
          businessId={businessId}
          type="business-logo"
          maxSize={2}
        />
      </div>

      {/* Reopen setup wizard */}
      <div className="border-t border-warm-200 pt-6 mt-6">
        <div className="flex items-center justify-between">
          <div>
            <h4 className="text-sm font-semibold text-ink-950">
              {tString("reopenWizard")}
            </h4>
            <p className="text-sm text-warm-600">
              {tString("reopenWizardDescription")}
            </p>
          </div>
          <Button
            variant="bordered"
            size="sm"
            className="rounded-xl border-warm-300 font-semibold text-ink-800"
            onPress={() => {
              if (typeof window !== "undefined") {
                window.sessionStorage.setItem(
                  `payverge_hub_force_show_${businessId}`,
                  "1",
                );
              }
              router.push(`/business/${businessUrlId}/dashboard?tab=overview`);
            }}
          >
            {tString("reopenWizard")}
          </Button>
        </div>
      </div>

      {/* Guest ordering is a destructive service switch (#225) — not a
          profile field under the logo. Confirmation lives in KitchenOrdersToggle. */}
      <div
        data-testid="settings-guest-ordering"
        className="rounded-2xl border border-rose-200 bg-rose-50/50 p-4 space-y-4"
      >
        <div className="flex items-start gap-3">
          <IconTile icon={AlertTriangle} />
          <div className="min-w-0 flex-1">
            <h3 className="text-lg font-semibold text-ink-950">
              {tString("profile.guestOrderingTitle")}
            </h3>
            <p className="text-sm text-ink-600">
              {tString("profile.guestOrderingDescription")}
            </p>
          </div>
        </div>
        <div className="flex items-center justify-end">
          <KitchenOrdersToggle
            businessId={businessId}
            isLocked={false}
            variant="button"
          />
        </div>
      </div>
    </div>
  );
}
