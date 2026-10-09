import React, { useState, useEffect, useCallback, useMemo } from "react";
import Image from "next/image";
import { usePathname, useRouter, useSearchParams } from "next/navigation";
import {
  Accordion,
  AccordionItem,
  Button,
  Chip,
  Input,
  Link,
  Modal,
  ModalBody,
  ModalContent,
  ModalFooter,
  ModalHeader,
  Select,
  SelectItem,
  Slider,
  useDisclosure,
} from "@nextui-org/react";
import {
  CircleCheck,
  CircleAlert,
  Settings,
  Link2,
  Unlink,
} from "lucide-react";
import toast from "react-hot-toast";
import {
  useSimpleLocale,
  getTranslation,
} from "@/i18n/SimpleTranslationProvider";
import { businessPluginAPI } from "@/api/plugins";
import { PLUGIN } from "@/constants/plugins";
import {
  buildMercadoPagoInitialConfig,
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

const COUNTRY_LABEL_KEYS: Record<string, string> = {
  AR: "argentina",
  BR: "brazil",
  CL: "chile",
  CO: "colombia",
  MX: "mexico",
  PE: "peru",
  UY: "uruguay",
};

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

interface MercadoPagoConfigProps {
  plugin: Plugin;
  config: Record<string, any>;
  onConfigChange: (config: Record<string, any>) => void;
  onSave: () => void;
  onCancel: () => void;
  businessId?: string;
  /** Test seam for same-window OAuth redirect (defaults to location.assign). */
  navigateTo?: (url: string) => void;
}

const mercadoPanelClass =
  "rounded-2xl border border-warm-200/80 bg-white/90 p-5 shadow-sm shadow-warm-900/5 sm:p-6";
const mercadoSectionTitleClass =
  "font-title text-base font-semibold tracking-0 text-ink-950";
const mercadoCopyClass = "text-sm leading-6 text-ink-600";
const mercadoValidClass = "text-xs font-medium text-emerald-700";

export default function MercadoPagoConfig({
  plugin,
  config,
  onConfigChange,
  onSave,
  onCancel,
  businessId,
  navigateTo,
}: MercadoPagoConfigProps) {
  const { locale } = useSimpleLocale();
  const searchParams = useSearchParams();
  const router = useRouter();
  const pathname = usePathname();
  const {
    isOpen: isDisconnectOpen,
    onOpen: onDisconnectOpen,
    onClose: onDisconnectClose,
  } = useDisclosure();

  const t = useCallback(
    (key: string): string => {
      const fullKey = `businessDashboard.dashboard.pluginManager.config.mercadopago.${key}`;
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

  const [formData, setFormData] = useState(() =>
    buildMercadoPagoInitialConfig(config),
  );
  const [isConnecting, setIsConnecting] = useState(false);
  const [isTesting, setIsTesting] = useState(false);
  const [isDisconnecting, setIsDisconnecting] = useState(false);
  const [testMessage, setTestMessage] = useState<string | null>(null);

  useEffect(() => {
    setFormData(buildMercadoPagoInitialConfig(config));
  }, [config]);

  // Toast OAuth callback result once, then strip the query params.
  useEffect(() => {
    const status = searchParams?.get("mp_connect");
    if (!status) return;

    if (status === "success") {
      toast.success(t("oauthConnectSuccess"));
    } else if (status === "error") {
      toast.error(t("oauthConnectError"));
    }

    if (!pathname) return;
    const next = new URLSearchParams(searchParams?.toString() ?? "");
    next.delete("mp_connect");
    next.delete("reason");
    const qs = next.toString();
    router.replace(qs ? `${pathname}?${qs}` : pathname, { scroll: false });
  }, [searchParams, pathname, router, t]);

  const connectionMode = String(formData.connection_mode || "").toLowerCase();
  const oauthStatus = String(formData.oauth_status || "").toLowerCase();
  const isOAuthConnected = connectionMode === "oauth";
  const needsReauth =
    isOAuthConnected && oauthStatus === "reauth_required";

  const handleInputChange = (field: string, value: any) => {
    const newFormData = { ...formData, [field]: value };
    setFormData(newFormData);
    onConfigChange(newFormData);
  };

  const validateMercadoPagoCredential = (credential: string) => {
    if (!credential) return null;
    return credential.startsWith("APP_USR-") && credential.length > 20;
  };

  // Display-level format validity (mask sentinel is treated as "already valid"
  // so a reconfigure of an enabled plugin doesn't render a false error).
  const accessTokenValid = isMaskedSecret(formData.access_token)
    ? true
    : validateMercadoPagoCredential(formData.access_token);
  const publicKeyValid = validateMercadoPagoCredential(formData.public_key);
  const baseUrlValid = formData.base_url
    ? isValidHttpUrl(formData.base_url)
    : null;

  const credentialsValid = {
    accessToken: isSecretSatisfied(
      formData.access_token,
      validateMercadoPagoCredential,
    ),
    publicKey: publicKeyValid === true,
    webhookSecret: isSecretSatisfied(
      formData.webhook_secret,
      (v) => v.trim().length > 0,
    ),
    baseUrl: baseUrlValid === true,
  };

  const accessTokenFormatError = hasSecretFormatError(
    formData.access_token,
    validateMercadoPagoCredential,
  );

  // OAuth mode only needs a satisfied access token (masked or fresh). Manual
  // mode still requires the full credential set.
  const canSave = isOAuthConnected
    ? credentialsValid.accessToken
    : credentialsValid.accessToken &&
      credentialsValid.publicKey &&
      credentialsValid.webhookSecret &&
      credentialsValid.baseUrl;

  const countryLabel = useMemo(() => {
    const code = String(formData.country || "AR").toUpperCase();
    const labelKey = COUNTRY_LABEL_KEYS[code];
    return labelKey ? t(`countries.${labelKey}`) : code;
  }, [formData.country, t]);

  const handleConnectOAuth = async () => {
    if (!businessId) {
      toast.error(t("businessMissing"));
      return;
    }
    setIsConnecting(true);
    try {
      const { authorization_url } =
        await businessPluginAPI.startMercadoPagoOAuth(businessId);
      if (!authorization_url) {
        // 200 with no URL: nothing to navigate to, so the button has to come
        // back — leaving it spinning strands the operator on a dead control.
        toast.error(t("oauthStartError"));
        setIsConnecting(false);
        return;
      }
      // Same-window redirect into Mercado Pago's OAuth consent screen.
      const go =
        navigateTo ??
        ((url: string) => {
          window.location.assign(url);
        });
      go(authorization_url);
    } catch {
      toast.error(t("oauthStartError"));
      setIsConnecting(false);
    }
  };

  const handleTestConnection = async () => {
    if (!businessId) {
      toast.error(t("businessMissing"));
      return;
    }
    setIsTesting(true);
    setTestMessage(null);
    try {
      const result = await businessPluginAPI.testPluginConnection(
        businessId,
        PLUGIN.mercadopago,
      );
      if (result.ok) {
        setTestMessage(result.message || t("connectionPassed"));
        toast.success(result.message || t("connectionPassed"));
      } else {
        setTestMessage(result.message || t("connectionFailed"));
        toast.error(result.message || t("connectionFailed"));
      }
    } catch {
      setTestMessage(t("connectionFailed"));
      toast.error(t("connectionFailed"));
    } finally {
      setIsTesting(false);
    }
  };

  const handleDisconnect = async () => {
    if (!businessId) {
      toast.error(t("businessMissing"));
      return;
    }
    setIsDisconnecting(true);
    try {
      await businessPluginAPI.disablePlugin(businessId, plugin.id);
      toast.success(t("disconnectSuccess"));
      onDisconnectClose();
      onCancel();
    } catch {
      toast.error(t("disconnectError"));
    } finally {
      setIsDisconnecting(false);
    }
  };

  const renderManualCredentialsForm = () => (
    <div className="space-y-4">
      <Input
        type="password"
        label={
          <>
            {t("accessToken")} <span className="text-rose-500">*</span>
          </>
        }
        aria-label={t("accessToken")}
        placeholder={t("accessTokenPlaceholder")}
        value={formData.access_token}
        onValueChange={(value) => handleInputChange("access_token", value)}
        variant="bordered"
        labelPlacement="outside"
        isInvalid={accessTokenFormatError}
        errorMessage={
          accessTokenFormatError ? t("invalidAccessTokenFormat") : undefined
        }
        description={
          isMaskedSecret(formData.access_token) ? (
            t("secretSavedHint")
          ) : accessTokenValid === true ? (
            <span
              className={`inline-flex items-center gap-1 ${mercadoValidClass}`}
            >
              <CircleCheck className="h-3 w-3 text-emerald-600" />
              {t("validFormat")}
            </span>
          ) : undefined
        }
      />

      <Input
        label={
          <>
            {t("publicKey")} <span className="text-rose-500">*</span>
          </>
        }
        aria-label={t("publicKey")}
        placeholder={t("publicKeyPlaceholder")}
        value={formData.public_key}
        onValueChange={(value) => handleInputChange("public_key", value)}
        variant="bordered"
        labelPlacement="outside"
        isInvalid={!!formData.public_key && !credentialsValid.publicKey}
        errorMessage={
          formData.public_key && !credentialsValid.publicKey
            ? t("invalidPublicKeyFormat")
            : undefined
        }
        description={
          credentialsValid.publicKey ? (
            <span
              className={`inline-flex items-center gap-1 ${mercadoValidClass}`}
            >
              <CircleCheck className="h-3 w-3 text-emerald-600" />
              {t("validFormat")}
            </span>
          ) : undefined
        }
      />

      <div className="rounded-2xl border border-warm-200 bg-warm-50/70 p-3 text-xs text-ink-700">
        <p className="mb-1 font-medium">{t("howToGetCredentials")}:</p>
        <ol className="list-inside list-decimal space-y-1">
          <li>
            {t("goToDeveloperPanel")}{" "}
            <Link
              href="https://www.mercadopago.com/developers/panel"
              target="_blank"
              rel="noopener noreferrer"
              size="sm"
            >
              MercadoPago Developer Panel
            </Link>
          </li>
          <li>{t("navigateToCredentials")}</li>
          <li>{t("copyCredentials")}</li>
          <li>{t("useTestCredentials")}</li>
        </ol>
      </div>

      <Input
        type="password"
        label={
          <>
            {t("webhookSecret")} <span className="text-rose-500">*</span>
          </>
        }
        aria-label={t("webhookSecret")}
        placeholder={t("webhookSecretPlaceholder")}
        value={formData.webhook_secret}
        onValueChange={(value) => handleInputChange("webhook_secret", value)}
        variant="bordered"
        labelPlacement="outside"
        description={
          isMaskedSecret(formData.webhook_secret)
            ? t("secretSavedHint")
            : t("webhookSecretDescription")
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
        errorMessage={baseUrlValid === false ? t("baseUrlInvalid") : undefined}
        description={t("baseUrlDescription")}
      />
    </div>
  );

  return (
    <div className="space-y-6">
      {/* Header */}
      <div className="flex items-center gap-3">
        <div className="flex h-11 w-11 items-center justify-center rounded-2xl border border-warm-200 bg-white shadow-sm">
          <Image
            src="/images/plugins/mercadopago.png"
            alt="MercadoPago"
            width={24}
            height={24}
            className="rounded"
          />
        </div>
        <div>
          <h3 className="font-title text-lg font-semibold text-ink-950">
            {t("title")}
          </h3>
          <p className={mercadoCopyClass}>{t("description")}</p>
        </div>
      </div>

      {/* Connection Status — OAuth primary path */}
      <div className={mercadoPanelClass}>
        <div className="mb-4 flex items-center justify-between">
          <h4 className={mercadoSectionTitleClass}>
            {tCommon("connectionStatus")}
          </h4>
          {needsReauth ? (
            <Chip
              color="warning"
              variant="flat"
              startContent={<CircleAlert className="h-3.5 w-3.5" />}
            >
              {t("reauthRequired")}
            </Chip>
          ) : isOAuthConnected ? (
            <Chip
              color="success"
              variant="flat"
              startContent={<CircleCheck className="h-3.5 w-3.5" />}
            >
              {t("connectedChip")}
            </Chip>
          ) : (
            <Chip
              variant="flat"
              className="border border-warm-200 bg-warm-50 text-ink-700"
              startContent={<CircleAlert className="h-3.5 w-3.5" />}
            >
              {tCommon("notConnected")}
            </Chip>
          )}
        </div>

        {needsReauth ? (
          <div className="space-y-4">
            <div className="rounded-2xl border border-amber-200 bg-amber-50/80 p-3 text-sm text-amber-900">
              <p className="font-medium">{t("reauthBannerTitle")}</p>
              <p className="mt-1 text-amber-800/90">{t("reauthBannerBody")}</p>
            </div>
            <Button
              color="primary"
              className="bg-brand text-white"
              onPress={() => void handleConnectOAuth()}
              isLoading={isConnecting}
              startContent={!isConnecting ? <Link2 size={16} /> : undefined}
            >
              {t("reconnect")}
            </Button>
          </div>
        ) : isOAuthConnected ? (
          <div className="space-y-4">
            <div className="space-y-1 text-sm text-ink-700">
              {formData.mp_user_id ? (
                <p>
                  <span className="font-medium text-ink-900">
                    {t("accountId")}:
                  </span>{" "}
                  {String(formData.mp_user_id)}
                </p>
              ) : null}
              <p>
                <span className="font-medium text-ink-900">{t("country")}:</span>{" "}
                {countryLabel}
              </p>
              <p>
                <span className="font-medium text-ink-900">{t("mode")}:</span>{" "}
                {formData.live_mode ? t("productionMode") : t("testMode")}
              </p>
            </div>
            {testMessage ? (
              <p className="text-xs text-ink-600">{testMessage}</p>
            ) : null}
            <div className="flex flex-wrap gap-2">
              <Button
                variant="bordered"
                color="primary"
                onPress={() => void handleTestConnection()}
                isLoading={isTesting}
              >
                {isTesting ? t("testingConnection") : tCommon("testConnection")}
              </Button>
              <Button
                variant="flat"
                color="danger"
                onPress={onDisconnectOpen}
                startContent={<Unlink size={16} />}
              >
                {t("disconnect")}
              </Button>
            </div>
          </div>
        ) : (
          <div className="space-y-4">
            <p className={mercadoCopyClass}>{t("connectAccountMessage")}</p>
            <Button
              color="primary"
              size="lg"
              className="bg-brand font-semibold text-white shadow-sm"
              onPress={() => void handleConnectOAuth()}
              isLoading={isConnecting}
              startContent={!isConnecting ? <Link2 size={18} /> : undefined}
            >
              {t("connectWithMercadoPago")}
            </Button>
          </div>
        )}
      </div>

      {/* Manual credentials — demoted to advanced accordion when not using OAuth
          as the primary connected path. Still available for connected OAuth
          operators who need to tweak installments below. */}
      {!isOAuthConnected || needsReauth ? (
        <Accordion variant="bordered" className="px-0">
          <AccordionItem
            key="manual-advanced"
            aria-label={t("manualAdvanced")}
            title={
              <div className="flex items-center gap-2">
                <Settings size={16} className="text-brand" />
                <span>{t("manualAdvanced")}</span>
              </div>
            }
          >
            <div className="space-y-4 pb-2">
              <p className="text-xs text-ink-600">{t("manualAdvancedHint")}</p>
              {renderManualCredentialsForm()}
            </div>
          </AccordionItem>
        </Accordion>
      ) : null}

      {/* MercadoPago Configuration (installments / country) */}
      <div className={mercadoPanelClass}>
        <h4 className={`${mercadoSectionTitleClass} mb-4`}>
          {t("apiConfiguration")}
        </h4>
        <div className="space-y-4">
          <Select
            label={
              <>
                {t("country")} <span className="text-rose-500">*</span>
              </>
            }
            aria-label={t("country")}
            selectedKeys={[formData.country || "AR"]}
            onSelectionChange={(keys) => {
              const next = Array.from(keys)[0] as string | undefined;
              if (next) handleInputChange("country", next);
            }}
            variant="bordered"
            labelPlacement="outside"
            description={t("selectOperatingCountry")}
          >
            {[
              { code: "AR", labelKey: "argentina" },
              { code: "BR", labelKey: "brazil" },
              { code: "CL", labelKey: "chile" },
              { code: "CO", labelKey: "colombia" },
              { code: "MX", labelKey: "mexico" },
              { code: "PE", labelKey: "peru" },
              { code: "UY", labelKey: "uruguay" },
            ].map(({ code, labelKey }) => (
              <SelectItem key={code} textValue={t(`countries.${labelKey}`)}>
                {t(`countries.${labelKey}`)}
              </SelectItem>
            ))}
          </Select>

          <div className="space-y-2">
            <Slider
              label={`${t("maximumInstallments")}: ${formData.installments}`}
              aria-label={t("maximumInstallments")}
              minValue={1}
              maxValue={24}
              step={1}
              value={Number(formData.installments) || 12}
              onChange={(value) =>
                handleInputChange(
                  "installments",
                  String(Array.isArray(value) ? value[0] : value),
                )
              }
              className="max-w-md"
            />
            <p className="text-xs text-ink-600">
              {t("installmentsDescription")} {formData.installments}{" "}
              {t("installments")}
            </p>
          </div>
        </div>
      </div>

      <div className="border-t border-warm-200/80" />

      {/* Actions */}
      <div className="flex justify-end gap-3">
        <button
          type="button"
          onClick={onCancel}
          className="rounded-xl px-4 py-2 text-sm font-medium text-ink-600 transition hover:bg-brand/5 hover:text-brand-700"
        >
          {tCommon("cancel")}
        </button>
        <button
          type="button"
          onClick={onSave}
          disabled={!canSave}
          className="rounded-xl bg-brand px-4 py-2 text-sm font-semibold text-white shadow-sm transition hover:bg-brand-dark disabled:cursor-not-allowed disabled:opacity-50"
        >
          {tCommon("save")}
        </button>
      </div>

      <Modal isOpen={isDisconnectOpen} onClose={onDisconnectClose}>
        <ModalContent>
          <ModalHeader>{t("disconnect")}</ModalHeader>
          <ModalBody>
            <p className={mercadoCopyClass}>{t("disconnectConfirm")}</p>
          </ModalBody>
          <ModalFooter>
            <Button variant="light" onPress={onDisconnectClose}>
              {tCommon("cancel")}
            </Button>
            <Button
              color="danger"
              onPress={() => void handleDisconnect()}
              isLoading={isDisconnecting}
            >
              {t("disconnect")}
            </Button>
          </ModalFooter>
        </ModalContent>
      </Modal>
    </div>
  );
}
