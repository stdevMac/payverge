/* eslint-disable no-restricted-syntax -- QR color props are user data / library contracts, not Tailwind classes */
"use client";

import React, { useState } from "react";
import {
  Modal,
  ModalContent,
  ModalHeader,
  ModalBody,
  ModalFooter,
  Button,
  Input,
  Slider,
  Checkbox,
  Select,
  SelectItem,
} from "@nextui-org/react";

import { Upload } from "lucide-react";
import toast from "react-hot-toast";
import {
  useSimpleLocale,
  getTranslation,
} from "@/i18n/SimpleTranslationProvider";
import { uploadFile } from "@/api/uploads";
import QRCodeWithText from "./QRCodeWithText";

interface Table {
  id: number;
  name: string;
  table_code: string;
  qr_code: string;
  is_active: boolean;
  qr_logo_url?: string;
  qr_foreground_color?: string;
  qr_background_color?: string;
  qr_logo_size?: number;
  qr_show_business_name?: boolean;
  qr_show_table_name?: boolean;
  qr_text_font?: string;
}

interface QRCustomizationModalProps {
  isOpen: boolean;
  onClose: () => void;
  table: Table;
  businessId: number;
  businessName?: string;
  onSave: (
    customization: {
      qr_logo_url: string;
      qr_foreground_color: string;
      qr_background_color: string;
      qr_logo_size: number;
      qr_show_business_name: boolean;
      qr_show_table_name: boolean;
      qr_text_font: string;
    },
    applyToAll?: boolean,
  ) => void;
  /** True while the parent persists the (possibly batched) save. */
  isSaving?: boolean;
}

export default function QRCustomizationModal({
  isOpen,
  onClose,
  table,
  businessId,
  businessName = "",
  onSave,
  isSaving = false,
}: QRCustomizationModalProps) {
  const { locale } = useSimpleLocale();

  const t = (key: string): string => {
    const fullKey = `businessDashboard.dashboard.tableManager.qrCustomization.${key}`;
    const result = getTranslation(fullKey, locale);
    return Array.isArray(result) ? result[0] || key : (result as string);
  };

  const [logoUrl, setLogoUrl] = useState(table.qr_logo_url || "");
  const [foregroundColor, setForegroundColor] = useState(
    table.qr_foreground_color || "#000000",
  );
  const [backgroundColor, setBackgroundColor] = useState(
    table.qr_background_color || "#FFFFFF",
  );
  const [logoSize, setLogoSize] = useState(table.qr_logo_size || 20);
  const [showBusinessName, setShowBusinessName] = useState(
    table.qr_show_business_name || false,
  );
  const [showTableName, setShowTableName] = useState(
    table.qr_show_table_name || false,
  );
  const [textFont, setTextFont] = useState(table.qr_text_font || "Verdana");
  const [uploading, setUploading] = useState(false);

  // 3- or 6-digit hex (with leading #). The native color picker always emits
  // valid hex; only the free-text inputs can drift, so we validate them and
  // feed the preview the last valid value instead of silently going stale.
  const isValidHex = (value: string) => /^#([0-9a-fA-F]{3}|[0-9a-fA-F]{6})$/.test(value);
  const foregroundValid = isValidHex(foregroundColor);
  const backgroundValid = isValidHex(backgroundColor);
  const previewForeground = foregroundValid ? foregroundColor : "#000000";
  const previewBackground = backgroundValid ? backgroundColor : "#FFFFFF";

  const handleLogoUpload = async (e: React.ChangeEvent<HTMLInputElement>) => {
    const file = e.target.files?.[0];
    if (!file) return;

    // Validate file size (max 2MB)
    if (file.size > 2 * 1024 * 1024) {
      toast.error(t("logoTooLarge"));
      return;
    }

    // Validate file type
    if (!file.type.startsWith("image/")) {
      toast.error(t("invalidFileType"));
      return;
    }

    setUploading(true);

    try {
      const data = await uploadFile(file, "qr-logos", businessId);
      setLogoUrl(data.location);
    } catch (error) {
      console.error("Logo upload error:", error);
      toast.error(t("uploadFailed"));
    } finally {
      setUploading(false);
    }
  };

  const handleSave = (applyToAll: boolean = false) => {
    // Don't persist an invalid hex the preview is already ignoring.
    if (!foregroundValid || !backgroundValid) {
      toast.error(t("invalidHex"));
      return;
    }
    // Do NOT close here: onSave is async and the parent owns closing on
    // success (handleSaveCustomization → onCustomizeClose). Closing eagerly
    // made isSaving dead and the partial-failure retry path unreachable —
    // the modal vanished before the fan-out settled (R3-OK QR modal).
    onSave(
      {
        qr_logo_url: logoUrl,
        qr_foreground_color: foregroundColor,
        qr_background_color: backgroundColor,
        qr_logo_size: logoSize,
        qr_show_business_name: showBusinessName,
        qr_show_table_name: showTableName,
        qr_text_font: textFont,
      },
      applyToAll,
    );
  };

  const handleReset = () => {
    setLogoUrl("");
    setForegroundColor("#000000");
    setBackgroundColor("#FFFFFF");
    setLogoSize(20);
    setShowBusinessName(false);
    setShowTableName(false);
    setTextFont("Verdana");
  };

  return (
    <Modal isOpen={isOpen} onClose={onClose} size="2xl" scrollBehavior="inside">
      <ModalContent className="rounded-3xl border border-warm-200 bg-white shadow-2xl shadow-warm-900/15">
        <ModalHeader className="flex flex-col gap-1 border-b border-warm-200/70 bg-warm-50/70">
          <h3 className="text-xl font-semibold text-ink-950">{t("title")}</h3>
          <p className="text-sm text-ink-600">
            {t("subtitle")} - {table.name}
          </p>
        </ModalHeader>
        <ModalBody className="bg-gradient-to-br from-white via-warm-50/50 to-brand/5">
          <div className="grid grid-cols-1 md:grid-cols-2 gap-6">
            {/* Preview Section */}
            <div className="flex flex-col items-center gap-4">
              <h4 className="text-base font-semibold text-ink-950">
                {t("preview")}
              </h4>
              <div className="w-full max-w-[340px] rounded-3xl border border-warm-200/80 bg-white p-4 shadow-sm shadow-warm-900/5">
                <QRCodeWithText
                  tableCode={table.table_code}
                  businessName={businessName}
                  tableName={table.name}
                  showBusinessName={showBusinessName}
                  showTableName={showTableName}
                  logoUrl={logoUrl}
                  foregroundColor={previewForeground}
                  backgroundColor={previewBackground}
                  logoSize={logoSize}
                  textFont={textFont}
                  size={300}
                  poweredByText={t("poweredBy")}
                  ariaLabel={`${t("preview")} — ${table.name}`}
                />
              </div>
            </div>

            {/* Customization Controls */}
            <div className="space-y-6">
              <div>
                <h4 className="text-base font-semibold text-ink-950 mb-4">
                  {t("customization")}
                </h4>

                {/* Logo Upload */}
                <div className="space-y-2 mb-4">
                  <label className="text-sm font-semibold text-ink-800">
                    {t("logo")}
                  </label>
                  <div className="flex gap-2">
                    <Input
                      value={logoUrl}
                      onChange={(e) => setLogoUrl(e.target.value)}
                      placeholder={t("logoUrlPlaceholder")}
                      size="sm"
                      classNames={{
                        inputWrapper: "border-warm-200 bg-white/90 shadow-sm",
                        input: "text-ink-900 placeholder:text-warm-500",
                      }}
                    />
                    <label htmlFor="logo-upload">
                      <Button
                        as="span"
                        size="sm"
                        isIconOnly
                        isLoading={uploading}
                        aria-label={t("uploadLogoAria")}
                        className="rounded-xl border border-warm-200 bg-white text-ink-700 shadow-sm shadow-warm-900/5"
                      >
                        <Upload size={16} />
                      </Button>
                    </label>
                    <input
                      id="logo-upload"
                      type="file"
                      accept="image/*"
                      onChange={handleLogoUpload}
                      className="hidden"
                    />
                  </div>
                  <p className="text-xs text-warm-700">{t("logoHelp")}</p>
                </div>

                {/* Logo Size */}
                {logoUrl && (
                  <div className="space-y-2 mb-4">
                    <label className="text-sm font-semibold text-ink-800">
                      {t("logoSize")}: {logoSize}%
                    </label>
                    <Slider
                      size="sm"
                      step={1}
                      minValue={10}
                      maxValue={30}
                      value={logoSize}
                      onChange={(value) => setLogoSize(value as number)}
                      className="max-w-md"
                    />
                  </div>
                )}

                {/* Foreground Color */}
                <div className="space-y-2 mb-4">
                  <label className="text-sm font-semibold text-ink-800">
                    {t("foregroundColor")}
                  </label>
                  <div className="flex gap-2 items-center">
                    <input
                      type="color"
                      value={foregroundColor}
                      onChange={(e) => setForegroundColor(e.target.value)}
                      className="w-12 h-10 rounded-xl border border-warm-300 bg-white p-1 cursor-pointer shadow-sm shadow-warm-900/5"
                    />
                    <Input
                      value={foregroundColor}
                      onChange={(e) => setForegroundColor(e.target.value)}
                      size="sm"
                      placeholder="#000000"
                      isInvalid={!foregroundValid}
                      errorMessage={!foregroundValid ? t("invalidHex") : undefined}
                      classNames={{
                        inputWrapper: "border-warm-200 bg-white/90 shadow-sm",
                        input: "font-mono text-ink-900 placeholder:text-warm-500",
                      }}
                    />
                  </div>
                </div>

                {/* Background Color */}
                <div className="space-y-2 mb-4">
                  <label className="text-sm font-semibold text-ink-800">
                    {t("backgroundColor")}
                  </label>
                  <div className="flex gap-2 items-center">
                    <input
                      type="color"
                      value={backgroundColor}
                      onChange={(e) => setBackgroundColor(e.target.value)}
                      className="w-12 h-10 rounded-xl border border-warm-300 bg-white p-1 cursor-pointer shadow-sm shadow-warm-900/5"
                    />
                    <Input
                      value={backgroundColor}
                      onChange={(e) => setBackgroundColor(e.target.value)}
                      size="sm"
                      placeholder="#FFFFFF"
                      isInvalid={!backgroundValid}
                      errorMessage={!backgroundValid ? t("invalidHex") : undefined}
                      classNames={{
                        inputWrapper: "border-warm-200 bg-white/90 shadow-sm",
                        input: "font-mono text-ink-900 placeholder:text-warm-500",
                      }}
                    />
                  </div>
                </div>

                {/* Text Display Options */}
                <div className="space-y-3 mb-4 pt-4 border-t border-warm-200">
                  <h5 className="text-sm font-semibold text-ink-800">
                    {t("textDisplay")}
                  </h5>

                  <Checkbox
                    size="sm"
                    isSelected={showBusinessName}
                    onValueChange={setShowBusinessName}
                  >
                    {t("showBusinessName")}
                  </Checkbox>

                  <Checkbox
                    size="sm"
                    isSelected={showTableName}
                    onValueChange={setShowTableName}
                  >
                    {t("showTableName")}
                  </Checkbox>

                  {/* Font Selector */}
                  {(showBusinessName || showTableName) && (
                    <div className="space-y-2 mt-3">
                      <label className="text-sm font-semibold text-ink-800">
                        {t("textFont")}
                      </label>
                      <Select
                        size="sm"
                        selectedKeys={[textFont]}
                        onChange={(e) => setTextFont(e.target.value)}
                        classNames={{
                          trigger: "border-warm-200 bg-white/90 shadow-sm",
                          value: "text-ink-900",
                        }}
                      >
                        <SelectItem key="Arial" value="Arial">
                          {t("fontArial")}
                        </SelectItem>
                        <SelectItem key="Helvetica" value="Helvetica">
                          {t("fontHelvetica")}
                        </SelectItem>
                        <SelectItem
                          key="Times New Roman"
                          value="Times New Roman"
                        >
                          {t("fontTimesNewRoman")}
                        </SelectItem>
                        <SelectItem key="Courier" value="Courier">
                          {t("fontCourier")}
                        </SelectItem>
                        <SelectItem key="Georgia" value="Georgia">
                          {t("fontGeorgia")}
                        </SelectItem>
                        <SelectItem key="Verdana" value="Verdana">
                          {t("fontVerdana")}
                        </SelectItem>
                      </Select>
                    </div>
                  )}
                </div>

                {/* Reset Button */}
                <Button
                  size="sm"
                  variant="flat"
                  onPress={handleReset}
                  className="w-full rounded-xl bg-warm-100 font-semibold text-ink-800"
                >
                  {t("reset")}
                </Button>
              </div>
            </div>
          </div>
        </ModalBody>
        <ModalFooter className="border-t border-warm-200/70 bg-white">
          <Button variant="light" onPress={onClose} isDisabled={isSaving} className="rounded-xl">
            {t("cancel")}
          </Button>
          <Button
            variant="flat"
            onPress={() => handleSave(true)}
            isLoading={isSaving}
            isDisabled={!foregroundValid || !backgroundValid || isSaving}
            className="rounded-xl bg-brand/10 font-semibold text-brand-dark"
          >
            {t("applyToAll")}
          </Button>
          <Button
            onPress={() => handleSave(false)}
            isLoading={isSaving}
            isDisabled={!foregroundValid || !backgroundValid || isSaving}
            className="rounded-xl bg-brand font-semibold text-white shadow-sm shadow-brand/20 hover:bg-brand-dark"
          >
            {t("save")}
          </Button>
        </ModalFooter>
      </ModalContent>
    </Modal>
  );
}
