import React, { useState, useEffect, useCallback } from "react";
import Image from "next/image";
import {
  Button,
  Chip,
  Input,
  Link,
  Select,
  SelectItem,
  Accordion,
  AccordionItem,
  Modal,
  ModalBody,
  ModalContent,
  ModalFooter,
  ModalHeader,
} from "@nextui-org/react";
import { CircleCheck, CircleAlert, Shield, ExternalLink, Settings, Link2 } from "lucide-react";
import toast from "react-hot-toast";
import {
  useSimpleLocale,
  getTranslation,
} from "@/i18n/SimpleTranslationProvider";
import { resolvePluginImageSrc } from "@/utils/pluginImages";
import {
  buildPayPalInitialConfig,
  hasSecretFormatError,
  isMaskedSecret,
  isSecretSatisfied,
} from "./configFields";

function isValidHttpUrl(value: string): boolean {
  try {
    const url = new URL(value.trim());
    return url.protocol === "http:" || url.protocol === "https:";
  } catch {
    return false;
  }
}

interface Plugin {
  id: number;
  name: string;
  display_name: string;
  description: string;
  image: string;
  category: string;
  version: string;
  features: string;
  is_enabled: boolean;
  config: string;
  config_schema?: string;
}

interface PayPalConfigProps {
  plugin: Plugin;
  config: Record<string, any>;
  onConfigChange: (config: Record<string, any>) => void;
  onSave: () => void;
  onCancel: () => void;
}

const paypalPanelClass =
  "rounded-2xl border border-warm-200/80 bg-white/90 p-5 shadow-sm shadow-warm-900/5 sm:p-6";
const paypalSectionTitleClass =
  "font-title text-base font-semibold tracking-0 text-ink-950";
const paypalCopyClass = "text-sm leading-6 text-ink-600";
const paypalLabelClass = "mb-2 block text-sm font-medium text-ink-700";
const paypalInputBaseClass =
  "h-12 w-full rounded-xl border-2 bg-white/90 px-4 text-sm text-ink-900 shadow-sm outline-none transition-colors";
const paypalInputDefaultClass =
  "border-warm-200 hover:border-brand/30 focus:border-brand";
const paypalInputInvalidClass = "border-rose-300 focus:border-rose-500";
const paypalStatusPillClass =
  "flex items-center gap-1 rounded-full border px-2.5 py-1 text-xs font-semibold";
const paypalHelpBoxClass =
  "rounded-2xl border border-warm-200 bg-warm-50/70 p-3 text-xs text-ink-700";

function normalizePayPalEnvironment(value: unknown): "sandbox" | "live" {
  if (typeof value !== "string") {
    return "sandbox";
  }

  const normalized = value.trim().toLowerCase();
  if (normalized === "live" || normalized === "production") {
    return "live";
  }

  return "sandbox";
}

export default function PayPalConfig({
  plugin,
  config,
  onConfigChange,
  onSave,
  onCancel,
}: PayPalConfigProps) {
  const { locale } = useSimpleLocale();
  const initialEnvironment = normalizePayPalEnvironment(config.environment);

  // Translation helper
  const t = useCallback(
    (key: string): string => {
      const fullKey = `businessDashboard.dashboard.pluginManager.config.paypal.${key}`;
      const result = getTranslation(fullKey, locale);
      return Array.isArray(result) ? result[0] || key : (result as string);
    },
    [locale],
  );

  const tCommon = useCallback(
    (key: string): string => {
      const fullKey = `businessDashboard.dashboard.pluginManager.config.${key}`;
      const result = getTranslation(fullKey, locale);
      return Array.isArray(result) ? result[0] || key : (result as string);
    },
    [locale],
  );

  const [isConnected, setIsConnected] = useState(false);
  const [isSetupGuideOpen, setIsSetupGuideOpen] = useState(false);
  const [environment, setEnvironment] = useState<"sandbox" | "live">("sandbox");
  const [formData, setFormData] = useState(() => ({
    ...buildPayPalInitialConfig(config),
    environment: initialEnvironment,
  }));

  const isClientIdValid = (clientId: string) =>
    Boolean(clientId) && clientId.length > 20 && clientId.length < 200;
  const isClientSecretValid = (clientSecret: string) =>
    Boolean(clientSecret) &&
    clientSecret.length > 20 &&
    clientSecret.length < 200;

  const credentialsValid = {
    clientId: isClientIdValid(formData.client_id),
    // client_secret is masked server-side; keeping the mask means "keep the
    // stored secret", so Save may proceed without re-entering it.
    clientSecret: isSecretSatisfied(formData.client_secret, isClientSecretValid),
  };
  const clientSecretFormatError = hasSecretFormatError(
    formData.client_secret,
    isClientSecretValid,
  );
  const webhookIdValid =
    typeof formData.webhook_id === "string" &&
    /^[A-Za-z0-9]+$/.test(formData.webhook_id.trim());
  const baseUrlValid = formData.base_url
    ? isValidHttpUrl(formData.base_url)
    : null;

  useEffect(() => {
    if (
      config.environment &&
      normalizePayPalEnvironment(config.environment) !== config.environment
    ) {
      onConfigChange({
        ...config,
        environment: normalizePayPalEnvironment(config.environment),
      });
    }
  }, [config, onConfigChange]);

  useEffect(() => {
    setIsConnected(
      Boolean(
        plugin.is_enabled &&
          credentialsValid.clientId &&
          credentialsValid.clientSecret &&
          webhookIdValid,
      ),
    );

    // Update environment state
    setEnvironment(normalizePayPalEnvironment(formData.environment));
  }, [
    credentialsValid.clientId,
    credentialsValid.clientSecret,
    formData.environment,
    webhookIdValid,
    plugin.is_enabled,
  ]);

  const handleInputChange = (field: string, value: any) => {
    const normalizedValue =
      field === "environment" ? normalizePayPalEnvironment(value) : value;
    const newFormData = { ...formData, [field]: normalizedValue };
    setFormData(newFormData);
    onConfigChange(newFormData);
  };

  const handleConnectPayPal = () => {
    setIsSetupGuideOpen(true);
  };

  const handleOpenDeveloperDashboard = () => {
    window.open(
      "https://developer.paypal.com/developer/applications/create",
      "paypal-setup",
      "width=1200,height=800,scrollbars=yes,resizable=yes,noopener,noreferrer",
    );
    toast.success(t("setupGuideToast"));
    setIsSetupGuideOpen(false);
  };

  // Honest local format check. This does NOT contact PayPal — a real credential
  // probe runs server-side on save (Initialize fetches an OAuth token), and the
  // enabled plugin can be verified via the /plugins/:id/test endpoint. Labeling
  // this "Test Connection" implied a live check it never performed, so it is
  // surfaced as "Check format" with copy that says save verifies the keys.
  const handleCheckFormat = () => {
    if (!formData.client_id || !formData.client_secret) {
      toast.error(t("credentialsRequiredError"));
      return;
    }

    if (
      credentialsValid.clientId &&
      (isMaskedSecret(formData.client_secret) ||
        isClientSecretValid(formData.client_secret))
    ) {
      toast.success(t("formatLooksValid"));
    } else {
      toast.error(t("formatInvalid"));
    }
  };

  return (
    <>
      <div className="space-y-6">
      {/* Header */}
      <div className="flex items-center gap-3">
        <div className="flex h-11 w-11 items-center justify-center rounded-2xl border border-warm-200 bg-white shadow-sm">
          <Image
            src={
              resolvePluginImageSrc(plugin) ?? "/images/plugins/paypal-logo.png"
            }
            alt={plugin.display_name}
            width={24}
            height={24}
            className="rounded"
          />
        </div>
        <div>
          <h3 className="font-title text-lg font-semibold text-ink-950">
            {t("title")}
          </h3>
          <p className={paypalCopyClass}>{t("description")}</p>
        </div>
      </div>

      {/* Connection Status */}
      <div className={paypalPanelClass}>
        <div className="flex items-center justify-between mb-4">
          <h4 className={paypalSectionTitleClass}>
            {tCommon("connectionStatus")}
          </h4>
          {isConnected ? (
            <span className={`${paypalStatusPillClass} border-emerald-200 bg-emerald-50 text-emerald-700`}>
              <CircleCheck className="w-3 h-3" />
              {tCommon("connected")}
            </span>
          ) : (
            <span className={`${paypalStatusPillClass} border-warm-200 bg-warm-50 text-ink-700`}>
              <CircleAlert className="w-3 h-3" />
              {tCommon("notConnected")}
            </span>
          )}
        </div>
        <div className="space-y-3">
          {isConnected ? (
            <div className="space-y-2">
              <p className="flex items-center gap-2 text-sm text-ink-700">
                <CircleCheck size={16} className="text-emerald-600" />
                {t("credentialsConfigured")}
              </p>
              <p className="flex items-center gap-2 text-sm text-ink-700">
                <Shield size={16} className="text-brand" />
                {t("environment")}:{" "}
                <span className="font-medium">
                  {environment === "sandbox"
                    ? t("sandboxTesting")
                    : t("liveProduction")}
                </span>
              </p>
              <div className="flex items-center gap-2 text-sm text-ink-700">
                <Shield size={16} className="text-brand" />
                <span>{t("secureConnection")}</span>
              </div>
            </div>
          ) : (
            <div className="space-y-3">
              <p className={paypalCopyClass}>
                {t("connectAccountMessage")}
              </p>
              <div className="flex gap-3">
                <Button
                  color="primary"
                  variant="flat"
                  size="sm"
                  onPress={handleCheckFormat}
                >
                  {t("checkFormat")}
                </Button>
                <Button
                  color="primary"
                  variant="flat"
                  size="sm"
                  onPress={handleConnectPayPal}
                >
                  <svg
                    width="16"
                    height="16"
                    viewBox="0 0 24 24"
                    fill="currentColor"
                  >
                    <path d="M7.076 21.337H2.47a.641.641 0 0 1-.633-.74L4.944.901C5.026.382 5.474 0 5.998 0h7.46c2.57 0 4.578.543 5.69 1.81 1.01 1.15 1.304 2.42 1.012 4.287-.023.143-.047.288-.077.437-.983 5.05-4.349 6.797-8.647 6.797h-2.19c-.524 0-.968.382-1.05.9l-1.12 7.106zm14.146-14.42a3.35 3.35 0 0 0-.607-.541c-.013.076-.026.175-.041.26-.983 5.05-4.349 6.797-8.647 6.797h-2.19c-.524 0-.968.382-1.05.9L7.076 21.337H2.47a.641.641 0 0 1-.633-.74L4.944.901C5.026.382 5.474 0 5.998 0h7.46c2.57 0 4.578.543 5.69 1.81.394.45.67.99.837 1.607.013.05.024.101.037.15z" />
                  </svg>
                  {t("connectWithPayPal")}
                </Button>
                <Button
                  variant="bordered"
                  onPress={() =>
                    window.open(
                      "https://developer.paypal.com/developer/applications/create",
                      "_blank",
                      "noopener,noreferrer",
                    )
                  }
                  endContent={<ExternalLink size={16} />}
                >
                  {t("manualSetup")}
                </Button>
              </div>
              <p className="text-xs text-ink-600">{t("setupInstructions")}</p>
            </div>
          )}
        </div>
      </div>

      {/* PayPal Credentials */}
      <div className={paypalPanelClass}>
        <h4 className={`${paypalSectionTitleClass} mb-4`}>
          {t("apiConfiguration")}
        </h4>
        <div className="space-y-4">
          <Select
            label={t("environment")}
            aria-label={t("environment")}
            selectedKeys={[formData.environment || "sandbox"]}
            onSelectionChange={(keys) => {
              const next = Array.from(keys)[0] as string | undefined;
              if (!next) return;
              handleInputChange("environment", next);
              setEnvironment(next as "sandbox" | "live");
            }}
            variant="bordered"
            labelPlacement="outside"
          >
            {[
              { key: "sandbox", label: t("sandbox") },
              { key: "live", label: t("production") },
            ].map((opt) => (
              <SelectItem key={opt.key} textValue={opt.label}>
                {opt.label}
              </SelectItem>
            ))}
          </Select>

          <div className="border-t border-warm-200/80" />

          {/* API Credentials */}
          <div className="space-y-4">
            <h5 className="text-sm font-semibold text-ink-950">{t("apiCredentials")}</h5>
            <Input
              label={
                <>
                  {t("clientId")} <span className="text-rose-500">*</span>
                </>
              }
              aria-label={t("clientId")}
              placeholder={t("clientIdPlaceholder")}
              value={formData.client_id}
              onValueChange={(value) => handleInputChange("client_id", value)}
              variant="bordered"
              labelPlacement="outside"
              isInvalid={!!formData.client_id && !credentialsValid.clientId}
              errorMessage={
                formData.client_id && !credentialsValid.clientId
                  ? t("clientIdDescription")
                  : undefined
              }
              description={
                credentialsValid.clientId ? (
                  <span className="inline-flex items-center gap-1 text-xs font-medium text-emerald-700">
                    <CircleCheck className="h-3 w-3" />
                    {t("validFormat")}
                  </span>
                ) : undefined
              }
            />

            <Input
              type="password"
              label={
                <>
                  {t("clientSecret")} <span className="text-rose-500">*</span>
                </>
              }
              aria-label={t("clientSecret")}
              placeholder={t("clientSecretPlaceholder")}
              value={formData.client_secret}
              onValueChange={(value) =>
                handleInputChange("client_secret", value)
              }
              variant="bordered"
              labelPlacement="outside"
              isInvalid={clientSecretFormatError}
              errorMessage={
                clientSecretFormatError
                  ? t("clientSecretDescription")
                  : undefined
              }
              description={
                isMaskedSecret(formData.client_secret) ? (
                  t("secretSavedHint")
                ) : isClientSecretValid(formData.client_secret) ? (
                  <span className="inline-flex items-center gap-1 text-xs font-medium text-emerald-700">
                    <CircleCheck className="h-3 w-3" />
                    {t("validFormat")}
                  </span>
                ) : undefined
              }
            />

            <Input
              type="url"
              label={
                <>
                  {t("baseUrl")} <span className="text-rose-500">*</span>
                </>
              }
              aria-label={t("baseUrl")}
              placeholder={t("baseUrlPlaceholder")}
              value={formData.base_url}
              onValueChange={(value) => handleInputChange("base_url", value)}
              variant="bordered"
              labelPlacement="outside"
              isInvalid={baseUrlValid === false}
              errorMessage={
                baseUrlValid === false ? t("baseUrlInvalid") : undefined
              }
              description={t("baseUrlDescription")}
            />

            <div className="space-y-3">
              <div className={paypalHelpBoxClass}>
                <p className="font-medium mb-1">
                  {t("credentialsHelpTitle")}
                </p>
                <ol className="list-decimal list-inside space-y-1">
                  <li>
                    {t("credentialsHelpGoTo")}{" "}
                    <Link
                      href="https://developer.paypal.com"
                      target="_blank"
                      rel="noopener noreferrer"
                      size="sm"
                    >
                      PayPal Developer
                    </Link>
                  </li>
                  <li>{t("credentialsHelpLogin")}</li>
                  <li>{t("credentialsHelpCreateApp")}</li>
                  <li>
                    {t("credentialsHelpCopyKeys")}
                  </li>
                  <li>{t("credentialsHelpSandbox")}</li>
                </ol>
              </div>

              {credentialsValid.clientId && credentialsValid.clientSecret && (
                <button
                  onClick={handleCheckFormat}
                  className="bg-brand text-white px-4 py-2 rounded-lg font-medium hover:bg-brand-dark transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-dark focus-visible:ring-offset-2"
                >
                  {t("checkFormat")}
                </button>
              )}
            </div>
          </div>
        </div>
      </div>

      {/* Advanced Configuration - Collapsible */}
      <Accordion variant="bordered">
        <AccordionItem
          key="payment-urls"
          aria-label={t("paymentUrls") || "Payment URLs"}
          title={
            <div className="flex items-center gap-2">
              <Link2 size={16} className="text-brand" />
              <span>{t("paymentUrls")}</span>
              <Chip size="sm" variant="flat" color="default">
                {t("optional")}
              </Chip>
            </div>
          }
        >
          <div className="space-y-4 pb-4">
            <div>
              <label className={paypalLabelClass}>
                {t("returnUrl")}
              </label>
              <input
                type="text"
                placeholder={t("returnUrlPlaceholder")}
                value={formData.return_url}
                onChange={(e) =>
                  handleInputChange("return_url", e.target.value)
                }
                className={`${paypalInputBaseClass} ${paypalInputDefaultClass}`}
              />
            </div>

            <div>
              <label className={paypalLabelClass}>
                {t("cancelUrl")}
              </label>
              <input
                type="text"
                placeholder={t("cancelUrlPlaceholder")}
                value={formData.cancel_url}
                onChange={(e) =>
                  handleInputChange("cancel_url", e.target.value)
                }
                className={`${paypalInputBaseClass} ${paypalInputDefaultClass}`}
              />
            </div>

            <div className="rounded-2xl border border-brand/15 bg-brand/5 p-3 text-xs text-ink-700">
              <p className="font-medium mb-1">{t("paymentUrlsInfo")}</p>
              <ul className="list-disc list-inside space-y-1">
                <li>{t("returnUrlInfo")}</li>
                <li>{t("cancelUrlInfo")}</li>
                <li>{t("defaultBehaviorInfo")}</li>
              </ul>
            </div>
          </div>
        </AccordionItem>

        <AccordionItem
          key="advanced-settings"
          aria-label={t("advancedSettings") || "Advanced Settings"}
          title={
            <div className="flex items-center gap-2">
              <Settings size={16} className="text-brand" />
              <span>{t("advancedSettings")}</span>
              <Chip size="sm" variant="flat" color="danger">
                {t("required")}
              </Chip>
            </div>
          }
        >
          <div className="space-y-4 pb-4">
            <div>
              <label className={paypalLabelClass}>
                {t("webhookId")}
              </label>
              <input
                type="text"
                placeholder={t("webhookIdPlaceholder")}
                value={formData.webhook_id}
                onChange={(e) =>
                  handleInputChange("webhook_id", e.target.value)
                }
                className={`${paypalInputBaseClass} ${
                  formData.webhook_id && !webhookIdValid
                    ? paypalInputInvalidClass
                    : paypalInputDefaultClass
                }`}
              />
              {formData.webhook_id && !webhookIdValid && (
                <p className="mt-1 text-xs text-rose-600">
                  {t("webhookIdDescription")}
                </p>
              )}
            </div>

            <div>
              <label className={paypalLabelClass}>
                {t("brandName")}
              </label>
              <input
                type="text"
                placeholder={t("brandNamePlaceholder")}
                value={formData.brand_name}
                onChange={(e) =>
                  handleInputChange("brand_name", e.target.value)
                }
                className={`${paypalInputBaseClass} ${paypalInputDefaultClass}`}
              />
            </div>

            <div className="rounded-2xl border border-brand/15 bg-brand/5 p-3 text-xs text-ink-700">
              <p className="font-medium mb-1">{t("advancedFeaturesInfo")}</p>
              <ul className="list-disc list-inside space-y-1">
                <li>{t("webhookInfo")}</li>
                <li>{t("brandNameInfo")}</li>
                <li>{t("enhancementInfo")}</li>
              </ul>
            </div>
          </div>
        </AccordionItem>
      </Accordion>

      <div className="border-t border-warm-200/80" />

        {/* Actions */}
        <div className="flex gap-3 justify-end">
          <button
            onClick={onCancel}
            className="rounded-xl px-4 py-2 text-sm font-medium text-ink-600 transition hover:bg-brand/5 hover:text-brand-700"
          >
            {tCommon("cancel")}
          </button>
          <button
            onClick={onSave}
            disabled={
              !credentialsValid.clientId ||
              !credentialsValid.clientSecret ||
              !webhookIdValid ||
              baseUrlValid !== true
            }
            className="rounded-xl bg-brand px-4 py-2 text-sm font-semibold text-white shadow-sm transition hover:bg-brand-dark disabled:cursor-not-allowed disabled:opacity-50"
          >
            {tCommon("save")}
          </button>
        </div>
      </div>

      <Modal
        isOpen={isSetupGuideOpen}
        onClose={() => setIsSetupGuideOpen(false)}
      >
        <ModalContent>
          <ModalHeader className="flex flex-col gap-1">
            {t("setupGuideTitle")}
          </ModalHeader>
          <ModalBody>
            <p className={paypalCopyClass}>
              {t("setupGuideDescription")}
            </p>
            <ol className="list-inside list-decimal space-y-2 text-sm text-ink-700">
              <li>{t("setupGuideStep1")}</li>
              <li>{t("setupGuideStep2")}</li>
              <li>{t("setupGuideStep3")}</li>
              <li>{t("setupGuideStep4")}</li>
              <li>{t("setupGuideStep5")}</li>
            </ol>
            <div className="rounded-lg border border-brand/10 bg-brand/10 px-3 py-2 text-sm text-brand-dark">
              {t("environmentStatus")}: {environment === "sandbox" ? t("sandboxLabel") : t("liveLabel")}
            </div>
          </ModalBody>
          <ModalFooter>
            <Button
              variant="light"
              onPress={() => setIsSetupGuideOpen(false)}
            >
              {tCommon("close")}
            </Button>
            <Button color="primary" onPress={handleOpenDeveloperDashboard}>
              {t("openPayPalDashboard")}
            </Button>
          </ModalFooter>
        </ModalContent>
      </Modal>
    </>
  );
}
