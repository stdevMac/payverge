"use client";

import React from "react";
import { Input } from "@nextui-org/react";
import { Phone, Star, MapPin } from "lucide-react";
import { BusinessAddress } from "@/api/business";
import {
  normalize,
  type SocialNetwork,
} from "@/lib/socialHandle";
import { PremiumPanel } from "./premium";

type SocialPlatform = SocialNetwork;

interface ContactEditorProps {
  phone: string;
  website: string;
  address: BusinessAddress;
  socialMedia: Partial<Record<SocialPlatform, string | undefined>>;
  onPhoneChange: (value: string) => void;
  onWebsiteChange: (value: string) => void;
  onAddressChange: (field: keyof BusinessAddress, value: string) => void;
  onSocialChange: (platform: SocialPlatform, value: string) => void;
  /** Parent's tStringSettings — prefixes "businessSettings."; pass key leaves. */
  t: (key: string) => string;
}

const inputClassNames = {
  inputWrapper:
    "bg-white border-warm-200 data-[hover=true]:border-brand/40 group-data-[focus=true]:border-brand",
  label: "text-ink-600",
} as const;

/**
 * Display + storage use the bare handle. Dirty legacy values (full URLs, @)
 * are re-normalized on every render so the input never shows a double-prefixed
 * mess; on change we persist only the canonical handle when valid.
 */
function displayHandle(platform: SocialPlatform, raw: string | undefined): string {
  const value = raw || "";
  const result = normalize(platform, value);
  return result.ok ? result.handle : value.replace(/^@+/, "");
}

function commitSocial(
  platform: SocialPlatform,
  raw: string,
  onSocialChange: (platform: SocialPlatform, value: string) => void,
): void {
  const result = normalize(platform, raw);
  // Persist the bare handle when valid; otherwise keep the raw input so the
  // operator can keep editing (isInvalid surfaces the validation error).
  onSocialChange(platform, result.ok ? result.handle : raw);
}

function socialError(
  platform: SocialPlatform,
  raw: string | undefined,
): string | undefined {
  const value = (raw || "").trim();
  if (!value) return undefined;
  const result = normalize(platform, value);
  return result.ok ? undefined : result.error;
}

export default function ContactEditor({
  phone,
  website,
  address,
  socialMedia,
  onPhoneChange,
  onWebsiteChange,
  onAddressChange,
  onSocialChange,
  t,
}: ContactEditorProps) {
  return (
    <div className="w-full space-y-6">
      {/* Contact details */}
      <PremiumPanel as="section" className="space-y-6 p-5 sm:p-6" withTexture>
        <div className="mb-2 flex items-center gap-3">
          <div className="flex h-11 w-11 items-center justify-center rounded-2xl border border-warm-200 bg-white text-brand shadow-sm">
            <Phone className="h-5 w-5" />
          </div>
          <div>
            <h3 className="text-lg font-semibold text-ink-950">
              {t("businessPage.contactTitle") || "Contact details"}
            </h3>
            <p className="text-sm text-ink-600">
              {t("businessPage.contactDescription") ||
                "Phone, website, address, and social links shown on your public page."}
            </p>
          </div>
        </div>

        <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
          <Input
            label={t("profile.phoneNumber")}
            placeholder={t("profile.phonePlaceholder")}
            value={phone}
            onValueChange={onPhoneChange}
            size="lg"
            variant="bordered"
            classNames={inputClassNames}
          />
          <Input
            label={t("profile.websiteUrl")}
            placeholder={t("profile.websitePlaceholder")}
            value={website}
            onValueChange={onWebsiteChange}
            size="lg"
            variant="bordered"
            classNames={inputClassNames}
          />
        </div>
      </PremiumPanel>

      {/* Address */}
      <PremiumPanel as="section" className="space-y-6 p-5 sm:p-6" withTexture>
        <div className="mb-2 flex items-center gap-3">
          <div className="flex h-11 w-11 items-center justify-center rounded-2xl border border-warm-200 bg-white text-brand shadow-sm">
            <MapPin className="h-5 w-5" />
          </div>
          <div>
            <h3 className="text-lg font-semibold text-ink-950">
              {t("profile.location")}
            </h3>
            <p className="text-sm text-ink-600">
              {t("profile.locationDescription")}
            </p>
          </div>
        </div>

        <Input
          label={t("profile.streetAddress")}
          placeholder={t("profile.streetPlaceholder")}
          value={address.street || ""}
          onValueChange={(v) => onAddressChange("street", v)}
          size="lg"
          variant="bordered"
          classNames={inputClassNames}
        />
        <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
          <Input
            label={t("profile.city")}
            placeholder={t("profile.cityPlaceholder")}
            value={address.city || ""}
            onValueChange={(v) => onAddressChange("city", v)}
            size="lg"
            variant="bordered"
            classNames={inputClassNames}
          />
          <Input
            label={t("profile.state")}
            placeholder={t("profile.statePlaceholder")}
            value={address.state || ""}
            onValueChange={(v) => onAddressChange("state", v)}
            size="lg"
            variant="bordered"
            classNames={inputClassNames}
          />
          <Input
            label={t("profile.postalCode")}
            placeholder={t("profile.postalPlaceholder")}
            value={address.postal_code || ""}
            onValueChange={(v) => onAddressChange("postal_code", v)}
            size="lg"
            variant="bordered"
            classNames={inputClassNames}
          />
          <Input
            label={t("profile.country")}
            placeholder={t("profile.countryPlaceholder")}
            value={address.country || ""}
            onValueChange={(v) => onAddressChange("country", v)}
            size="lg"
            variant="bordered"
            classNames={inputClassNames}
          />
        </div>
      </PremiumPanel>

      {/* Social links */}
      <PremiumPanel as="section" className="space-y-6 p-5 sm:p-6" withTexture>
        <div className="mb-2 flex items-center gap-3">
          <div className="flex h-11 w-11 items-center justify-center rounded-2xl border border-warm-200 bg-white text-brand shadow-sm">
            <Star className="h-5 w-5" />
          </div>
          <div>
            <h3 className="text-lg font-semibold text-ink-950">
              {t("businessPage.socialMediaTitle")}
            </h3>
            <p className="text-sm text-ink-600">
              {t("businessPage.socialMediaDescription") ||
                "Where customers can follow you"}
            </p>
          </div>
        </div>
        <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
          <Input
            label={t("businessPage.instagram")}
            placeholder={(t("businessPage.instagramPlaceholder") || "").replace(/^@/, "")}
            value={displayHandle("instagram", socialMedia.instagram)}
            onValueChange={(v) => commitSocial("instagram", v, onSocialChange)}
            isInvalid={Boolean(socialError("instagram", socialMedia.instagram))}
            errorMessage={socialError("instagram", socialMedia.instagram)}
            size="lg"
            variant="bordered"
            startContent={<span className="text-sm text-ink-500">@</span>}
            classNames={inputClassNames}
          />
          <Input
            label={t("businessPage.facebook")}
            placeholder={t("businessPage.facebookPlaceholder")}
            value={displayHandle("facebook", socialMedia.facebook)}
            onValueChange={(v) => commitSocial("facebook", v, onSocialChange)}
            isInvalid={Boolean(socialError("facebook", socialMedia.facebook))}
            errorMessage={socialError("facebook", socialMedia.facebook)}
            size="lg"
            variant="bordered"
            startContent={<span className="text-sm text-ink-500">facebook.com/</span>}
            classNames={inputClassNames}
          />
          <Input
            label={t("businessPage.twitter")}
            placeholder={(t("businessPage.twitterPlaceholder") || "").replace(/^@/, "")}
            value={displayHandle("twitter", socialMedia.twitter)}
            onValueChange={(v) => commitSocial("twitter", v, onSocialChange)}
            isInvalid={Boolean(socialError("twitter", socialMedia.twitter))}
            errorMessage={socialError("twitter", socialMedia.twitter)}
            size="lg"
            variant="bordered"
            startContent={<span className="text-sm text-ink-500">@</span>}
            classNames={inputClassNames}
          />
          <Input
            label={t("businessPage.linkedin") || "LinkedIn"}
            placeholder={t("businessPage.linkedinPlaceholder")}
            value={displayHandle("linkedin", socialMedia.linkedin)}
            onValueChange={(v) => commitSocial("linkedin", v, onSocialChange)}
            isInvalid={Boolean(socialError("linkedin", socialMedia.linkedin))}
            errorMessage={socialError("linkedin", socialMedia.linkedin)}
            size="lg"
            variant="bordered"
            startContent={<span className="text-sm text-ink-500">linkedin.com/company/</span>}
            classNames={inputClassNames}
          />
          <Input
            label={t("businessPage.youtube") || "YouTube"}
            placeholder={t("businessPage.youtubePlaceholder")}
            value={displayHandle("youtube", socialMedia.youtube)}
            onValueChange={(v) => commitSocial("youtube", v, onSocialChange)}
            isInvalid={Boolean(socialError("youtube", socialMedia.youtube))}
            errorMessage={socialError("youtube", socialMedia.youtube)}
            size="lg"
            variant="bordered"
            startContent={<span className="text-sm text-ink-500">youtube.com/@</span>}
            classNames={inputClassNames}
          />
          <Input
            label={t("businessPage.tiktok") || "TikTok"}
            placeholder={t("businessPage.tiktokPlaceholder")}
            value={displayHandle("tiktok", socialMedia.tiktok)}
            onValueChange={(v) => commitSocial("tiktok", v, onSocialChange)}
            isInvalid={Boolean(socialError("tiktok", socialMedia.tiktok))}
            errorMessage={socialError("tiktok", socialMedia.tiktok)}
            size="lg"
            variant="bordered"
            startContent={<span className="text-sm text-ink-500">tiktok.com/@</span>}
            classNames={inputClassNames}
          />
        </div>
      </PremiumPanel>
    </div>
  );
}
