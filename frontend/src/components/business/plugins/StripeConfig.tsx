import React, { useState, useEffect, useCallback } from "react";
import Image from "next/image";
import { usePathname, useRouter, useSearchParams } from "next/navigation";
import {
  Button,
  Chip,
  Switch,
  useDisclosure,
  Modal,
  ModalBody,
  ModalContent,
  ModalFooter,
  ModalHeader,
} from "@nextui-org/react";
import {
  CircleCheck,
  CircleAlert,
  Eye,
  EyeOff,
  Link2,
  Unlink,
} from "lucide-react";
import toast from "react-hot-toast";
import {
  useSimpleLocale,
  getTranslation,
} from "@/i18n/SimpleTranslationProvider";
import { businessPluginAPI } from "@/api/plugins";
import { resolvePluginImageSrc } from "@/utils/pluginImages";
import {
  buildStripeInitialConfig,
  hasSecretFormatError,
  isMaskedSecret,
  isSecretSatisfied,
} from "./configFields";

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

interface StripeConfigProps {
  plugin: Plugin;
  config: Record<string, any>;
  onConfigChange: (config: Record<string, any>) => void;
  onSave: () => void;
  onCancel: () => void;
  businessId?: string;
  /** Test seam for same-window OAuth redirect (defaults to location.assign). */
  navigateTo?: (url: string) => void;
}

const stripePanelClass =
  "rounded-2xl border border-warm-200/80 bg-white/90 p-5 shadow-sm shadow-warm-900/5 sm:p-6";
const stripeSectionTitleClass =
  "font-title text-base font-semibold tracking-0 text-ink-950";
const stripeCopyClass = "text-sm leading-6 text-ink-600";
const stripeLabelClass = "mb-2 block text-sm font-medium text-ink-700";
const stripeInputBaseClass =
  "h-12 w-full rounded-xl border-2 bg-white/90 px-4 text-sm text-ink-900 shadow-sm outline-none transition-colors";
const stripeInputDefaultClass =
  "border-warm-200 hover:border-brand/30 focus:border-brand";
const stripeInputInvalidClass = "border-rose-300 focus:border-rose-500";
const stripeValidClass = "text-xs font-medium text-emerald-700";

export default function StripeConfig({
  plugin,
  config,
  onConfigChange,
  onSave,
  onCancel,
  businessId,
  navigateTo,
}: StripeConfigProps) {
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
      const fullKey = `businessDashboard.dashboard.pluginManager.config.stripe.${key}`;
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

  const tFallback = useCallback(
    (key: string, fallback: string): string => {
      const value = t(key);
      const fullKey = `businessDashboard.dashboard.pluginManager.config.stripe.${key}`;
      return value && value !== key && value !== fullKey ? value : fallback;
    },
    [t],
  );

  const [showSecret, setShowSecret] = useState(false);
  const [isConnecting, setIsConnecting] = useState(false);
  const [isDisconnecting, setIsDisconnecting] = useState(false);
  const [formData, setFormData] = useState(() =>
    buildStripeInitialConfig(config),
  );
  const isTestMode = formData.mode !== "live";

  useEffect(() => {
    setFormData(buildStripeInitialConfig(config));
  }, [config]);

  // Toast OAuth callback result once, then strip the query params.
  useEffect(() => {
    const status = searchParams?.get("stripe_connect");
    if (!status) return;

    if (status === "success") {
      toast.success(t("oauthConnectSuccess"));
    } else if (status === "error") {
      toast.error(t("oauthConnectError"));
    }

    if (!pathname) return;
    const next = new URLSearchParams(searchParams?.toString() ?? "");
    next.delete("stripe_connect");
    next.delete("reason");
    const qs = next.toString();
    router.replace(qs ? `${pathname}?${qs}` : pathname, { scroll: false });
  }, [searchParams, pathname, router, t]);

  const connectionMode = String(formData.connection_mode || "").toLowerCase();
  const oauthStatus = String(formData.oauth_status || "").toLowerCase();
  const isOAuthConnected = connectionMode === "oauth";
  const needsReauth =
    isOAuthConnected && oauthStatus === "reauth_required";
  const hasManualKeys = Boolean(
    formData.secret_key && formData.publishable_key,
  );
  const isConnected = isOAuthConnected || hasManualKeys;

  const liveKeyDetected =
    formData.secret_key.trim().startsWith("sk_live_") && isTestMode;

  const handleInputChange = (field: string, value: any) => {
    const newFormData = { ...formData, [field]: value };
    // Switching to manual entry: clear oauth mode so save validates keys.
    if (
      (field === "secret_key" || field === "publishable_key") &&
      value &&
      !isMaskedSecret(value) &&
      connectionMode === "oauth"
    ) {
      newFormData.connection_mode = "manual";
    }
    setFormData(newFormData);
    onConfigChange(newFormData);
  };

  const validateStripeKey = (key: string, type: "secret" | "publishable") => {
    if (!key) return null;
    const validPrefixes =
      type === "secret"
        ? ["sk_test_", "sk_live_", "rk_test_", "rk_live_"]
        : ["pk_test_", "pk_live_"];
    return validPrefixes.some((prefix) => key.startsWith(prefix));
  };

  const secretKeyValidator = (key: string) => validateStripeKey(key, "secret");
  const webhookSecretValidator = (value: string) =>
    value.trim().startsWith("whsec_");

  const secretKeyValid = validateStripeKey(formData.secret_key, "secret");
  const publishableKeyValid = validateStripeKey(
    formData.publishable_key,
    "publishable",
  );
  const secretKeySatisfied = isSecretSatisfied(
    formData.secret_key,
    secretKeyValidator,
  );
  const secretKeyFormatError = hasSecretFormatError(
    formData.secret_key,
    secretKeyValidator,
  );
  const webhookSecretFormatError = hasSecretFormatError(
    formData.webhook_secret,
    webhookSecretValidator,
  );

  // OAuth connected: save allowed without merchant keys.
  // Manual: require secret + publishable.
  const canSave = isOAuthConnected && !needsReauth
    ? !webhookSecretFormatError
    : secretKeySatisfied &&
      publishableKeyValid === true &&
      !webhookSecretFormatError;

  const handleConnectOAuth = async () => {
    if (!businessId) {
      toast.error(t("businessMissing"));
      return;
    }
    setIsConnecting(true);
    try {
      const { authorization_url } =
        await businessPluginAPI.startStripeOAuth(businessId);
      if (!authorization_url) {
        // 200 with no URL: nothing to navigate to, so the button has to come
        // back — leaving it spinning strands the operator on a dead control.
        toast.error(t("oauthStartError"));
        setIsConnecting(false);
        return;
      }
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
      {liveKeyDetected && (
        <div
          role="alert"
          className="rounded-xl border border-amber-200 bg-amber-50 p-3 text-sm text-amber-900"
        >
          <strong>
            {tFallback("liveKeyWarningTitle", "Live key detected.")}
          </strong>{" "}
          {tFallback("liveKeyWarningBeforeMode", "Toggle")}{" "}
          <em>{tFallback("liveMode", "Live mode")}</em>{" "}
          {tFallback(
            "liveKeyWarningAfterMode",
            "below to confirm this key will accept real customer payments.",
          )}
        </div>
      )}

      <div className="flex items-center justify-between">
        <div>
          <p className="text-sm font-medium text-ink-950">{t("environment")}</p>
          <p className="text-xs text-ink-600">
            {isTestMode ? t("testModeNote") : t("liveModeNote")}
          </p>
        </div>
        <div className="flex items-center gap-2">
          <span className="text-sm text-ink-600">
            {tCommon("common.testMode")}
          </span>
          <Switch
            isSelected={!isTestMode}
            onValueChange={(value) =>
              handleInputChange("mode", value ? "live" : "test")
            }
            size="sm"
          />
          <span className="text-sm text-ink-600">
            {tCommon("common.liveMode")}
          </span>
        </div>
      </div>

      <div>
        <label className={stripeLabelClass}>
          {t("secretKey")} <span className="text-rose-500">*</span>
        </label>
        <div className="relative">
          <input
            type={showSecret ? "text" : "password"}
            placeholder={t("secretKeyPlaceholder")}
            value={formData.secret_key}
            onChange={(e) => handleInputChange("secret_key", e.target.value)}
            className={`${stripeInputBaseClass} pr-12 ${
              secretKeyFormatError
                ? stripeInputInvalidClass
                : stripeInputDefaultClass
            }`}
          />
          <button
            type="button"
            onClick={() => setShowSecret((v) => !v)}
            aria-label={showSecret ? t("hideSecretKey") : t("showSecretKey")}
            className="absolute inset-y-0 right-0 flex w-12 items-center justify-center rounded-r-xl text-ink-600 transition hover:text-brand-700 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand"
          >
            {showSecret ? (
              <EyeOff className="h-4 w-4" aria-hidden="true" />
            ) : (
              <Eye className="h-4 w-4" aria-hidden="true" />
            )}
          </button>
        </div>
        {isMaskedSecret(formData.secret_key) ? (
          <p className="mt-1 text-xs text-ink-600">{t("secretSavedHint")}</p>
        ) : (
          secretKeyValid && (
            <div className="mt-1 flex items-center gap-1">
              <CircleCheck className="h-3 w-3 text-emerald-600" />
              <span className={stripeValidClass}>{t("validKeyFormat")}</span>
            </div>
          )
        )}
      </div>

      <div>
        <label className={stripeLabelClass}>
          {t("publishableKey")} <span className="text-rose-500">*</span>
        </label>
        <input
          type="text"
          placeholder={t("publishableKeyPlaceholder")}
          value={formData.publishable_key}
          onChange={(e) =>
            handleInputChange("publishable_key", e.target.value)
          }
          className={`${stripeInputBaseClass} ${
            formData.publishable_key && !publishableKeyValid
              ? stripeInputInvalidClass
              : stripeInputDefaultClass
          }`}
        />
        {publishableKeyValid && (
          <div className="mt-1 flex items-center gap-1">
            <CircleCheck className="h-3 w-3 text-emerald-600" />
            <span className={stripeValidClass}>{t("validKeyFormat")}</span>
          </div>
        )}
      </div>

      <div>
        <label className={stripeLabelClass}>
          {t("webhookSecret")}{" "}
          <span className="text-xs text-ink-500">
            ({tCommon("common.optional")})
          </span>
        </label>
        <input
          type="password"
          placeholder={t("webhookSecretPlaceholder")}
          value={formData.webhook_secret}
          onChange={(e) =>
            handleInputChange("webhook_secret", e.target.value)
          }
          className={`${stripeInputBaseClass} ${
            webhookSecretFormatError
              ? stripeInputInvalidClass
              : stripeInputDefaultClass
          }`}
        />
        {webhookSecretFormatError && (
          <p className="mt-1 text-xs text-rose-600">
            {t("webhookSecretInvalid")}
          </p>
        )}
        <p className="mt-1 text-xs text-ink-600">
          {isMaskedSecret(formData.webhook_secret)
            ? t("secretSavedHint")
            : t("webhookSecretDescription")}
        </p>
      </div>
    </div>
  );

  return (
    <div className="space-y-6">
      {/* Header */}
      <div className="flex items-center gap-3">
        <div className="flex h-11 w-11 items-center justify-center rounded-2xl border border-warm-200 bg-white shadow-sm">
          <Image
            src={
              resolvePluginImageSrc(plugin) ?? "/images/plugins/stripe-logo.png"
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
          <p className={stripeCopyClass}>{t("description")}</p>
        </div>
      </div>

      {/* Connection Status — OAuth primary path */}
      <div className={stripePanelClass}>
        <div className="mb-4 flex items-center justify-between">
          <h4 className={stripeSectionTitleClass}>
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
          ) : isConnected ? (
            <Chip
              color="success"
              variant="flat"
              startContent={<CircleCheck className="h-3.5 w-3.5" />}
            >
              {tCommon("connected")}
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
              {formData.stripe_user_id ? (
                <p>
                  <span className="font-medium text-ink-900">
                    {t("accountId")}:
                  </span>{" "}
                  {String(formData.stripe_user_id)}
                </p>
              ) : null}
              <p>
                <span className="font-medium text-ink-900">{t("mode")}:</span>{" "}
                {formData.live_mode ? t("liveMode") : t("testMode")}
              </p>
              <p className="flex items-center gap-2">
                <CircleCheck size={16} className="text-emerald-600" />
                <span>{t("secureConnection")}</span>
              </p>
            </div>
            <div className="flex flex-wrap gap-2">
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
            <p className={stripeCopyClass}>{t("connectAccountMessage")}</p>
            <Button
              color="primary"
              size="lg"
              className="bg-brand font-semibold text-white shadow-sm"
              onPress={() => void handleConnectOAuth()}
              isLoading={isConnecting}
              startContent={!isConnecting ? <Link2 size={18} /> : undefined}
            >
              {t("connectWithStripe")}
            </Button>
          </div>
        )}
      </div>

      {/* Manual credentials: always available when not OAuth-connected (fallback).
          When OAuth-connected, only optional webhook secret is shown. */}
      {isOAuthConnected && !needsReauth ? (
        <div className={stripePanelClass}>
          <h4 className={`${stripeSectionTitleClass} mb-2`}>
            {t("apiConfiguration")}
          </h4>
          <p className="mb-4 text-xs text-ink-600">{t("oauthConnectedHint")}</p>
          <div>
            <label className={stripeLabelClass}>
              {t("webhookSecret")}{" "}
              <span className="text-xs text-ink-500">
                ({tCommon("common.optional")})
              </span>
            </label>
            <input
              type="password"
              placeholder={t("webhookSecretPlaceholder")}
              value={formData.webhook_secret}
              onChange={(e) =>
                handleInputChange("webhook_secret", e.target.value)
              }
              className={`${stripeInputBaseClass} ${
                webhookSecretFormatError
                  ? stripeInputInvalidClass
                  : stripeInputDefaultClass
              }`}
            />
            <p className="mt-1 text-xs text-ink-600">
              {isMaskedSecret(formData.webhook_secret)
                ? t("secretSavedHint")
                : t("webhookSecretDescription")}
            </p>
          </div>
        </div>
      ) : (
        <div className={stripePanelClass}>
          <h4 className={`${stripeSectionTitleClass} mb-2`}>
            {t("manualAdvanced")}
          </h4>
          <p className="mb-4 text-xs text-ink-600">{t("manualAdvancedHint")}</p>
          {/* Accessible label for tests / screen readers */}
          <span className="sr-only">{t("manualAdvanced")}</span>
          {renderManualCredentialsForm()}
        </div>
      )}

      <div className="border-t border-warm-200/80" />

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
            <p className="text-sm text-ink-700">{t("disconnectConfirm")}</p>
          </ModalBody>
          <ModalFooter>
            <Button variant="light" onPress={onDisconnectClose}>
              {tCommon("cancel")}
            </Button>
            <Button
              color="danger"
              isLoading={isDisconnecting}
              onPress={() => void handleDisconnect()}
            >
              {t("disconnect")}
            </Button>
          </ModalFooter>
        </ModalContent>
      </Modal>
    </div>
  );
}
