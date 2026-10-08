"use client";

import React, { useState, useEffect, useCallback } from "react";
import { Button } from "@nextui-org/react";
import { AccessibleInput } from "@/components/ui/AccessibleInput";
import { CreditCard, Globe } from "lucide-react";
import Image from "next/image";
import dynamic from "next/dynamic";
import toast from "react-hot-toast";
import { useGuestTranslation } from "../../i18n/GuestTranslationProvider";
import { paymentPluginAPI, Plugin } from "../../api/plugins";
import { HOUSE_COUNTER_RAIL, PLUGIN } from "@/constants/plugins";
import { asDollars } from "@/types/money";
import { parseLocaleDecimal } from "@/lib/parseLocaleDecimal";
import { isUsableGuestSettlementAddress } from "@/lib/isEvmAddress";
import { useInstance } from "@/hooks/useInstance";
import { paymentMethodLabel } from "@/lib/paymentMethodLabels";
import { CurrencyPrice } from "../common/CurrencyConverter";
import { normalizeGuestLocale } from "@/utils/guestCurrencyFormatter";
import { deriveCurrencyPrefix } from "@/components/business/delivery/currencyPrefix";
import PaymentStatusChecker from "../payment/PaymentStatusChecker";

// @lifi/sdk (34 MB) is pulled in transitively by CrossChainPayment and by
// PaymentProcessor (viem wallet client + LI.FI wallet provider). Load each
// only when a guest actually opens that tender — see the conditional mounts
// below — so card payers never download the bridging SDK. (P-1)
const paymentModalLoading = () => (
  <div className="flex justify-center p-8 animate-pulse">
    <div className="h-12 w-12 rounded-full bg-default-200" />
  </div>
);

const PaymentProcessor = dynamic(
  () => import("../payment/PaymentProcessor"),
  {
    ssr: false,
    loading: paymentModalLoading,
  },
);

const CrossChainPayment = dynamic(
  () => import("../payment/CrossChainPayment"),
  {
    ssr: false,
    loading: paymentModalLoading,
  },
);

// Extended interface for business plugin data
interface BusinessPluginData extends Plugin {
  business_id: number;
  plugin_id: number;
  is_enabled: boolean;
  config: string;
}

interface PaymentSectionProps {
  billId: number;
  billToken: string;
  businessId: number;
  amount: number;
  businessName: string;
  businessAddress: string;
  tipAddress: string;
  tableCode: string;
  defaultCurrency?: string;
  displayCurrency?: string;
  /** Base amount for percentage tip presets. Defaults to amount. */
  tipBaseAmount?: number;
  onPaymentComplete: (paymentDetails: {
    totalPaid: number;
    tipAmount: number;
    paymentMethod: string;
    transactionId?: string;
  }) => void;
  onCashierPayment: (paymentDetails?: {
    totalPaid: number;
    tipAmount: number;
    paymentMethod: string;
  }) => void | Promise<void>;
  /** Shows loading on the pay button while a cashier request is in flight. */
  cashierPaymentLoading?: boolean;
  /** Reports checkout-in-progress so the parent can poll faster. */
  onPaymentBusyChange?: (busy: boolean) => void;
  /** When true, hides the Cashier payment option. Use for delivery pay pages
   *  where a physical cashier is not an option for the guest. */
  hideCashier?: boolean;
  /** When true, hides the "Add a tip" selector entirely and forces tip = 0.
   *  Use when a gratuity has already been collected upstream and baked into
   *  `amount` (e.g. the delivery driver tip is already in bill.total_amount),
   *  so a second pay-page tip would double-charge gratuity. */
  hideTip?: boolean;
  splitShareId?: number;
  /** Override the base path for plugin payment return/cancel redirects.
   *  Defaults to `/t/${tableCode}/bill`. Use for non-table payment contexts
   *  (e.g. `/delivery/${deliveryNumber}/pay`). */
  returnUrlBase?: string;
}

interface PaymentMethodOption {
  id: string;
  name: string;
  description: string;
  icon: React.ReactNode;
  type: "crypto" | "plugin" | "other";
  processingTime?: string;
  fees?: string;
  features?: string[];
}

function isGuestPaymentPluginAvailable(plugin: {
  name: string;
  is_enabled?: boolean;
}) {
  // The house counter rail rides in the same `plugins` list so the endpoint's
  // rail list stays honest for a venue with no processor credentials (#894),
  // but it is not a plugin: there is no checkout to create for it, and a
  // plugin chip would render an English machine label next to the localized
  // cashier tile. It is rendered once below, gated on counter_settlement_ready
  // — the drawer state is what decides whether cash can actually be settled.
  if (plugin.name === HOUSE_COUNTER_RAIL) return false;
  return !!plugin.is_enabled;
}

export default function PaymentSection({
  billId,
  billToken,
  businessId,
  amount,
  businessName,
  businessAddress,
  tipAddress: _tipAddress,
  tableCode,
  defaultCurrency = "USD",
  displayCurrency = "USD",
  tipBaseAmount,
  onPaymentComplete,
  onCashierPayment,
  cashierPaymentLoading = false,
  onPaymentBusyChange,
  hideCashier = false,
  hideTip = false,
  splitShareId,
  returnUrlBase,
}: PaymentSectionProps) {
  const { t, currentLanguage } = useGuestTranslation();
  // The server reports crypto off when no settlement RPC is wired and on the
  // public demo, which refuses every crypto payment route.
  const cryptoOff = useInstance().isOff("crypto");
  const moneyLocale = normalizeGuestLocale(currentLanguage);
  const [selectedMethod, setSelectedMethod] = useState<string>("");
  const [tipAmount, setTipAmount] = useState<number>(0);
  const [customTipInput, setCustomTipInput] = useState<string>("");
  const [availablePlugins, setAvailablePlugins] = useState<
    BusinessPluginData[]
  >([]);
  const [isUSDCPaymentOpen, setIsUSDCPaymentOpen] = useState(false);
  const [isCrossChainPaymentOpen, setIsCrossChainPaymentOpen] =
    useState(false);
  // Guards the async plugin-payment path from double-submit (double-click ->
  // two createPluginPayment calls / checkout sessions). The modal paths
  // (usdc/cross-chain) return early and are idempotent (re-open is a
  // no-op), so only the network path needs it.
  const [isProcessingPlugin, setIsProcessingPlugin] = useState(false);
  // When the plugin fetch fails transiently, every configured method (USDC,
  // cross-chain, card providers) silently disappears and only Cashier remains —
  // indistinguishable from a business that genuinely accepts cash only. Track
  // the failure so we can tell the guest and offer a retry instead of silently
  // steering them to cash. (Audit C-03.)
  const [pluginsLoadError, setPluginsLoadError] = useState(false);
  // Gate auto-select until the first plugins fetch settles so we don't lock
  // onto cashier before card methods arrive.
  const [pluginsReady, setPluginsReady] = useState(false);
  const [counterSettlementReady, setCounterSettlementReady] = useState(false);
  // Non-redirect plugin payments are confirmed by polling the server, never
  // by trusting the client-side totals (audit C-04). Holds the pending
  // payment the inline PaymentStatusChecker is verifying.
  const [inlineStatusCheck, setInlineStatusCheck] = useState<{
    paymentId: string;
    method: string;
  } | null>(null);

  useEffect(() => {
    onPaymentBusyChange?.(
      isProcessingPlugin ||
        isUSDCPaymentOpen ||
        isCrossChainPaymentOpen ||
        !!inlineStatusCheck,
    );
  }, [
    isProcessingPlugin,
    isUSDCPaymentOpen,
    isCrossChainPaymentOpen,
    inlineStatusCheck,
    onPaymentBusyChange,
  ]);

  // Load available payment plugins (locale-aware display names). Auto-retry
  // once with backoff on first paint so a transient blip does not leave the
  // guest staring at "Couldn't load payment methods" (#81).
  const loadPlugins = useCallback(async (opts?: { autoRetry?: boolean }) => {
    const maxAttempts = opts?.autoRetry === false ? 1 : 3;
    let lastError: unknown;
    for (let attempt = 1; attempt <= maxAttempts; attempt++) {
      try {
        const response = await paymentPluginAPI.getBusinessPaymentPlugins(
          businessId,
          currentLanguage,
        );
        // Cast the response to BusinessPluginData since the API returns business plugin data
        setAvailablePlugins((response.plugins || []) as BusinessPluginData[]);
        setCounterSettlementReady(response.counter_settlement_ready === true);
        setPluginsLoadError(false);
        setPluginsReady(true);
        return;
      } catch (error) {
        lastError = error;
        if (attempt < maxAttempts) {
          await new Promise((resolve) =>
            setTimeout(resolve, 250 * attempt * attempt),
          );
        }
      }
    }
    console.error("Error loading payment plugins:", lastError);
    setAvailablePlugins([]);
    setCounterSettlementReady(false);
    setPluginsLoadError(true);
    setPluginsReady(true);
    toast.error(t("payment.methodsLoadError"));
  }, [businessId, currentLanguage, t]);

  useEffect(() => {
    loadPlugins().catch((err) => console.error("loadPlugins failed:", err));
  }, [loadPlugins]);

  // When hideTip is set, a gratuity has already been collected upstream and
  // baked into `amount` (e.g. the delivery driver tip in bill.total_amount).
  // Force the tip to 0 so the charge equals the bill amount exactly — the tip
  // selector that would set tipAmount is also hidden below.
  const effectiveTip = hideTip ? 0 : tipAmount;
  const totalAmount = amount + effectiveTip;
  const tipPresetBase = Math.max(0, tipBaseAmount ?? amount);

  const getPluginSublabel = (plugin: BusinessPluginData): string => {
    // Crypto rails need a wallet; card rails do not. USDC and cross-chain
    // both require a connected wallet — they were mislabeled "No account
    // needed".
    const cryptoIds = new Set<string>([
      PLUGIN.usdcPayment,
      PLUGIN.crossChainPayment,
    ]);
    return cryptoIds.has(plugin.name)
      ? t("payment.sublabel.cryptoWallet")
      : t("payment.sublabel.noAccount");
  };

  // NEW-12: map known rail slugs to guest-message keys (all 21 locales) so tile
  // titles never depend on backend plugin display_name i18n (en/es only).
  // Unknown plugins keep the backend display_name, then a humanized slug.
  const knownGuestRails = new Set<string>([
    PLUGIN.usdcPayment,
    PLUGIN.crossChainPayment,
    PLUGIN.stripe,
    PLUGIN.paypal,
    PLUGIN.mercadopago,
  ]);
  const getPluginDisplayName = (plugin: BusinessPluginData): string => {
    if (knownGuestRails.has(plugin.name)) {
      return paymentMethodLabel(plugin.name, t);
    }
    return plugin.display_name || paymentMethodLabel(plugin.name, t);
  };

  const getPluginIcon = (plugin: BusinessPluginData) => {
    // Use the plugin image if available, otherwise fallback to icons.
    // Pass relative paths to next/image as-is — adding window.location.origin
    // turns "/images/plugins/x.png" into "http://localhost:3000/..." which
    // next/image rejects unless localhost is in remotePatterns.
    if (plugin.image && plugin.image !== "") {
      return (
        <Image
          src={plugin.image}
          alt={getPluginDisplayName(plugin)}
          width={32}
          height={32}
          className="w-8 h-8 object-contain rounded-lg"
          onError={(e) => {
            const target = e.currentTarget as HTMLImageElement;
            target.style.display = "none";
          }}
        />
      );
    }

    // IMP-10: brand-coloured fallback so the picker doesn't read as a half-
    // finished design when plugin.image is missing. Per-brand official
    // primary colour, with the brand wordmark glyph (S / PP / MP) in white.
    // Real SVG logos from each brand's press kit are a follow-up; the
    // primary win here is dropping the all-teal squares that made every
    // method look identical.
    /* eslint-disable no-restricted-syntax -- third-party brand colors */
    switch (plugin.name) {
      case PLUGIN.paypal:
        return (
          <div
            className="w-8 h-8 rounded-lg flex items-center justify-center text-white text-sm font-bold"
            style={{ background: "#003087" }}
            aria-label="PayPal"
          >
            PP
          </div>
        );
      case PLUGIN.stripe:
        return (
          <div
            className="w-8 h-8 rounded-lg flex items-center justify-center text-white text-sm font-bold"
            style={{ background: "#635BFF" }}
            aria-label="Stripe"
          >
            S
          </div>
        );
      case PLUGIN.mercadopago:
        return (
          <div
            className="w-8 h-8 rounded-lg flex items-center justify-center text-sm font-bold"
            style={{ background: "#FFE600", color: "#009EE3" }}
            aria-label="MercadoPago"
          >
            MP
          </div>
        );
    /* eslint-enable no-restricted-syntax */
      default:
        return (
          <div className="flex h-8 w-8 items-center justify-center rounded-lg bg-ink-700 text-white">
            <Globe className="h-5 w-5" strokeWidth={1.75} />
          </div>
        );
    }
  };

  // Build the guest-facing plugin-error toast from the plugin's human display
  // name, never the raw plugin id ("mercadopago"/"stripe"). Falls back to a
  // generic message when the name can't be resolved.
  const pluginErrorMessage = (methodId: string): string => {
    const plugin = availablePlugins.find((p) => p.name === methodId);
    const displayName = plugin ? getPluginDisplayName(plugin) : "";
    return displayName
      ? t("payment.pluginError", { method: displayName })
      : t("payment.pluginErrorGeneric");
  };

  const handlePayment = async () => {
    if (selectedMethod === "cashier") {
      await onCashierPayment({
        totalPaid: totalAmount,
        tipAmount: effectiveTip,
        paymentMethod: "cashier",
      });
      return;
    }

    if (selectedMethod === PLUGIN.usdcPayment) {
      if (!businessAddress) {
        toast.error(t("payment.cryptoNotConfigured"));
        return;
      }
      setIsUSDCPaymentOpen(true);
      return;
    }

    if (selectedMethod === PLUGIN.crossChainPayment) {
      if (!businessAddress) {
        toast.error(t("payment.cryptoNotConfigured"));
        return;
      }
      setIsCrossChainPaymentOpen(true);
      return;
    }

    // Handle plugin payments
    if (isProcessingPlugin) return; // ignore double-clicks while a request is in flight
    setIsProcessingPlugin(true);
    try {
      // Generate return and cancel URLs
      const currentURL = window.location.origin;
      const basePath = returnUrlBase ?? `/t/${tableCode}/bill`;
      const returnParams = new URLSearchParams({
        payment: "success",
        method: selectedMethod,
        bill_token: billToken,
      });
      const cancelParams = new URLSearchParams({
        payment: "cancelled",
        method: selectedMethod,
        bill_token: billToken,
      });
      const returnURL = `${currentURL}${basePath}?${returnParams.toString()}`;
      const cancelURL = `${currentURL}${basePath}?${cancelParams.toString()}`;

      const response = await paymentPluginAPI.createPluginPayment(billToken, {
        plugin_id: selectedMethod,
        amount: asDollars(totalAmount),
        currency: defaultCurrency,
        tip_amount: asDollars(effectiveTip),
        return_url: returnURL,
        cancel_url: cancelURL,
        metadata: {
          business_name: businessName,
          bill_id: billId,
          table_code: tableCode,
          tip_amount: effectiveTip,
          ...(splitShareId ? { split_share_id: splitShareId } : {}),
        },
      });

      // Check if we need to redirect to external payment provider
      if (response.payment_url || response.redirect_url) {
        const redirectURL = response.payment_url || response.redirect_url || "";
        if (redirectURL) {
          // Persist only the opaque server-generated paymentId (scoped to this bill)
          // so the return handler can resume status polling. Amounts, bill_id, and
          // tip data stay server-side to limit XSS exfiltration surface.
          if (response.payment_id) {
            sessionStorage.setItem(
              "payverge_payment",
              JSON.stringify({
                paymentId: response.payment_id,
                method: selectedMethod,
                billToken,
                ...(splitShareId ? { splitShareId } : {}),
              }),
            );
          }
          window.location.href = redirectURL;
        }
      } else if (response.payment_id) {
        // No provider redirect — verify against the server instead of
        // optimistically reporting the client-computed totals as paid
        // (audit C-04). The checker polls getPluginPaymentStatus to a
        // terminal status and confirms with server-reported amounts.
        setInlineStatusCheck({
          paymentId: response.payment_id,
          method: selectedMethod,
        });
      } else {
        // No redirect URL and no payment id: nothing to poll and nothing
        // was authorized — treat as a malformed plugin response rather
        // than telling the guest they paid.
        console.error(
          "Plugin payment returned neither a redirect URL nor a payment id:",
          selectedMethod,
        );
        toast.error(pluginErrorMessage(selectedMethod ?? ""));
      }
    } catch (error) {
      console.error("Error processing plugin payment:", error);
      toast.error(
        pluginErrorMessage(selectedMethod),
      );
    } finally {
      // Re-enable on error/immediate-complete. On the redirect path the page
      // navigates away, so resetting here is harmless.
      setIsProcessingPlugin(false);
    }
  };

  const toPluginMethod = (plugin: BusinessPluginData): PaymentMethodOption => ({
    id: plugin.name,
    name: getPluginDisplayName(plugin),
    description: getPluginSublabel(plugin),
    icon: getPluginIcon(plugin),
    type: "plugin" as const,
  });

  const enabledGuestPlugins = availablePlugins.filter(
    isGuestPaymentPluginAvailable,
  );

  // Crypto rails (USDC, Any-Token) settle straight to the business on-chain
  // wallet. When no settlement address is configured the tender can't complete,
  // so the option would only error on tap — hide it instead of surfacing a
  // dead choice.
  const cryptoWalletRail = new Set<string>([
    PLUGIN.usdcPayment,
    PLUGIN.crossChainPayment,
  ]);
  const isCryptoRail = (name: string) => cryptoWalletRail.has(name);

  const cardPlugins = enabledGuestPlugins.filter((p) => !isCryptoRail(p.name));
  const cryptoPlugins = cryptoOff
    ? []
    : enabledGuestPlugins.filter(
        (p) =>
          isCryptoRail(p.name) &&
          isUsableGuestSettlementAddress(businessAddress),
      );

  // Mercado Pago is first-class when enabled: full brand CTA above the peer
  // chip grid (not a peer chip). Remaining card plugins stay in the grid.
  const mercadoPagoPlugin = cardPlugins.find(
    (p) => p.name === PLUGIN.mercadopago,
  );
  const peerCardPlugins = cardPlugins.filter(
    (p) => p.name !== PLUGIN.mercadopago,
  );

  // Familiar card/cash tenders lead; crypto rails follow so guests aren't
  // steered to wallet-first options before the mainstream ones. Mercado Pago
  // is omitted here — it has its own full-width brand treatment above.
  const paymentMethods: PaymentMethodOption[] = [
    ...peerCardPlugins.map(toPluginMethod),
    ...(!hideCashier && counterSettlementReady === true
      ? [
          {
            id: "cashier",
            name: t("bill.cashier") || "Cashier",
            description: t("payment.sublabel.payAtCounter"),
            icon: (
              <div className="flex h-8 w-8 items-center justify-center rounded-lg bg-ink-700 text-white">
                <CreditCard className="h-5 w-5" strokeWidth={1.75} />
              </div>
            ),
            type: "other" as const,
          },
        ]
      : []),
    ...cryptoPlugins.map(toPluginMethod),
  ];

  const isMercadoPagoSelected = selectedMethod === PLUGIN.mercadopago;
  // Prefer Mercado Pago when available so the primary CTA is the brand button.
  const firstMethodId = mercadoPagoPlugin
    ? PLUGIN.mercadopago
    : (paymentMethods[0]?.id ?? "");
  const methodIdsKey = [
    ...(mercadoPagoPlugin ? [PLUGIN.mercadopago] : []),
    ...paymentMethods.map((m) => m.id),
  ].join("|");
  // Auto-select the first available method so the primary CTA is never a
  // dead "Select Payment Method" when methods are already visible above.
  // Wait for pluginsReady so cashier is not locked in before card rails load.
  // Re-select only if the current choice disappears after a plugin reload.
  useEffect(() => {
    if (!pluginsReady || !firstMethodId) return;
    const ids = methodIdsKey ? methodIdsKey.split("|") : [];
    if (!selectedMethod || !ids.includes(selectedMethod)) {
      setSelectedMethod(firstMethodId);
    }
  }, [pluginsReady, firstMethodId, methodIdsKey, selectedMethod]);
  // Tip preset buttons
  const tipPresets = [
    { label: "10%", value: tipPresetBase * 0.1 },
    { label: "15%", value: tipPresetBase * 0.15 },
    { label: "20%", value: tipPresetBase * 0.2 },
    { label: "25%", value: tipPresetBase * 0.25 },
  ];

  const handleTipPreset = (value: number) => {
    // Store the preset rounded to cents so a manual re-type of the displayed
    // 2-dp value compares equal under the cent-tolerant preset highlight check.
    const rounded = Math.round(value * 100) / 100;
    setTipAmount(rounded);
    setCustomTipInput(rounded.toFixed(2));
  };

  const handleCustomTipChange = (value: string) => {
    setCustomTipInput(value);
    // parseLocaleDecimal normalizes comma-decimal separators (es/fr/de/...) so
    // "5,50" tips 5.50, not a truncated 5; clamp to >= 0 so a negative tip can
    // never lower the total below the bill amount.
    setTipAmount(Math.max(0, parseLocaleDecimal(value)));
  };

  return (
    <>
      <div className="space-y-5 rounded-2xl border border-warm-200 bg-white p-5 sm:p-6">
        {/* Payment Methods */}
        <div>
          <h3 className="mb-3 text-label uppercase text-ink-500" id="guest-payment-methods-label">
            {t("bill.paymentOptions")}
          </h3>

          <div
            role="radiogroup"
            aria-labelledby="guest-payment-methods-label"
          >
          {/* Mercado Pago first-class brand treatment — full-width, not a peer chip. */}
          {mercadoPagoPlugin ? (
            <button
              type="button"
              role="radio"
              aria-checked={isMercadoPagoSelected}
              data-testid="mercadopago-brand-cta"
              onClick={() => setSelectedMethod(PLUGIN.mercadopago)}
              className={`mb-3 flex w-full items-center justify-center gap-2 rounded-xl px-4 py-3.5 text-sm font-semibold text-white shadow-sm transition-colors active:translate-y-px ${
                isMercadoPagoSelected
                  ? "ring-2 ring-ink-900 ring-offset-2"
                  : "opacity-95 hover:opacity-100"
              }`}
              /* eslint-disable-next-line no-restricted-syntax -- Mercado Pago official brand blue #009EE3 */
              style={{ backgroundColor: "#009EE3" }}
            >
              {getPluginIcon(mercadoPagoPlugin)}
              <span>{t("payment.payWithMercadoPago")}</span>
            </button>
          ) : null}
          {paymentMethods.length > 0 ? (
            <div className="grid grid-cols-2 gap-2 sm:grid-cols-4">
              {paymentMethods.map((method) => {
                const selected = selectedMethod === method.id;
                return (
                  <button
                    key={method.id}
                    type="button"
                    role="radio"
                    aria-checked={selected}
                    onClick={() => {
                      setSelectedMethod(method.id);
                      if (method.id === "cashier" && !splitShareId) {
                        setTipAmount(0);
                        setCustomTipInput("");
                      }
                    }}
                    className={`flex flex-col items-center gap-2 rounded-xl border px-3 py-3 transition-colors active:translate-y-px ${
                      selected
                        ? "border-ink-900 bg-warm-50"
                        : "border-warm-200 bg-white hover:border-ink-300"
                    }`}
                  >
                    <div className="flex items-center justify-center">
                      {method.icon}
                    </div>
                    <span
                      className={`text-center text-xs font-semibold ${
                        selected ? "text-ink-900" : "text-ink-700"
                      }`}
                    >
                      {method.name}
                    </span>
                    {method.description ? (
                      <span className="text-center text-[10px] leading-tight text-ink-500">
                        {method.description}
                      </span>
                    ) : null}
                  </button>
                );
              })}
            </div>
          ) : null}
          </div>
          {/* Plugin load failed: tell the guest and let them retry, so a
              transient API blip doesn't silently masquerade as "cash only". */}
          {pluginsReady &&
          !pluginsLoadError &&
          !mercadoPagoPlugin &&
          paymentMethods.length === 0 ? (
            <div
              className="mt-3 rounded-xl border border-amber-200 bg-amber-50 px-3 py-2.5 text-sm text-amber-950"
              data-testid="guest-no-tender"
              role="status"
            >
              {t("payment.noTenderAvailable")}
            </div>
          ) : null}
          {pluginsLoadError && (
            <div className="mt-3 flex items-center justify-between gap-3 rounded-xl border border-warm-200 bg-warm-50 px-3 py-2.5 text-xs text-ink-600">
              <span role="alert">{t("payment.methodsLoadError")}</span>
              <button
                type="button"
                onClick={() => {
                  setPluginsLoadError(false);
                  setPluginsReady(false);
                  void loadPlugins({ autoRetry: false });
                }}
                className="flex-shrink-0 rounded-lg border border-ink-300 px-3 py-1.5 font-semibold text-ink-800 transition-colors hover:border-ink-900 active:translate-y-px"
              >
                {t("bill.retry")}
              </button>
            </div>
          )}
        </div>

        {/* Tip Selection */}
        {!hideTip && (selectedMethod !== "cashier" || !!splitShareId) && (
          <div>
            <h4 className="mb-3 text-label uppercase text-ink-500" id="guest-tip-label">
              {t("bill.addTip")}
            </h4>

            <div
              role="radiogroup"
              aria-labelledby="guest-tip-label"
              className="grid grid-cols-3 gap-2 sm:grid-cols-5"
            >
              {/* No tip — leftmost slot */}
              <button
                type="button"
                role="radio"
                aria-checked={tipAmount === 0}
                onClick={() => {
                  setTipAmount(0);
                  setCustomTipInput("");
                }}
                className={`flex h-14 flex-col items-center justify-center rounded-xl border text-xs font-semibold transition-colors active:translate-y-px ${
                  tipAmount === 0
                    ? "border-ink-900 bg-ink-900 text-white"
                    : "border-warm-200 bg-white text-ink-700 hover:border-ink-300"
                }`}
              >
                <span className="leading-tight">{t("bill.noTipButton")}</span>
              </button>

              {tipPresets.map((preset) => {
                // Cent-tolerant compare: the stored tip is rounded to cents and
                // may round-trip through the input as a 2-dp string, so strict
                // float equality would drop the highlight for common amounts.
                const active = Math.abs(tipAmount - preset.value) < 0.005;
                return (
                  <button
                    key={preset.label}
                    type="button"
                    role="radio"
                    aria-checked={active}
                    onClick={() => handleTipPreset(preset.value)}
                    className={`flex h-14 flex-col items-center justify-center rounded-xl border transition-colors active:translate-y-px ${
                      active
                        ? "border-brand bg-brand text-white"
                        : "border-warm-200 bg-white text-ink-700 hover:border-ink-300"
                    }`}
                  >
                    <span className="text-sm font-semibold leading-none">
                      {preset.label}
                    </span>
                    <span
                      className={`mt-1 font-mono text-[10px] tabular-nums ${
                        active ? "text-white/80" : "text-ink-500"
                      }`}
                    >
                      <CurrencyPrice
                        amount={preset.value}
                        fromCurrency={defaultCurrency}
                        displayCurrency={displayCurrency}
                       locale={moneyLocale}
                      />
                    </span>
                  </button>
                );
              })}
            </div>

            {/* Custom Tip Input */}
            <div className="mt-3 flex items-center gap-2">
              <AccessibleInput
                label={
                  // In dual-currency venues the guest sees display-currency
                  // amounts everywhere, but the tip they type is charged in the
                  // business's charge currency. Name the charge currency in the
                  // label so "10" is unambiguously 10 of the charge currency,
                  // not the display currency. The startContent symbol alone is
                  // ambiguous when both currencies share a symbol (e.g. USD/MXN
                  // both "$").
                  defaultCurrency !== displayCurrency
                    ? t("bill.customTipCurrencyLabel", {
                        currency: defaultCurrency,
                      })
                    : t("bill.customTipLabel")
                }
                placeholder={t("bill.customTipPlaceholder")}
                value={customTipInput}
                inputMode="decimal"
                onChange={(e) => handleCustomTipChange(e.target.value)}
                startContent={
                  <div className="pointer-events-none flex items-center">
                    <span className="font-mono text-sm text-ink-500">
                      {deriveCurrencyPrefix(defaultCurrency)}
                    </span>
                  </div>
                }
                classNames={{
                  inputWrapper:
                    "bg-warm-50 border border-warm-200 data-[hover=true]:border-ink-300",
                  input: "font-mono tabular-nums",
                }}
                className="flex-1"
                size="sm"
              />
            </div>
          </div>
        )}

        {/* Payment Summary — cashier full-bill with no tip already has the
            amount in the bill hero + CTA; the Amount/Total twin is pure noise. */}
        {!(
          selectedMethod === "cashier" &&
          !splitShareId &&
          effectiveTip === 0
        ) && (
          <dl
            data-testid="payment-amount-summary"
            className="space-y-2 rounded-xl border border-warm-200 bg-warm-50 p-4 text-sm"
          >
            <div className="flex justify-between">
              {/* `amount` is the tax-inclusive balance being paid (a split share
                  or the remaining bill), NOT a pre-tax subtotal — labeling it
                  "Subtotal" contradicted the real Subtotal shown on the bill. */}
              <dt className="text-ink-600">{t("bill.amount")}</dt>
              <dd className="font-mono tabular-nums text-ink-900">
                <CurrencyPrice
                  amount={amount}
                  fromCurrency={defaultCurrency}
                  displayCurrency={displayCurrency}
                 locale={moneyLocale}
                      />
              </dd>
            </div>
            {effectiveTip > 0 && (
              <div className="flex justify-between">
                <dt className="text-ink-600">{t("bill.tip")}</dt>
                <dd className="font-mono tabular-nums text-ink-900">
                  <CurrencyPrice
                    amount={effectiveTip}
                    fromCurrency={defaultCurrency}
                    displayCurrency={displayCurrency}
                   locale={moneyLocale}
                      />
                </dd>
              </div>
            )}
            <div className="flex items-center justify-between border-t border-warm-200 pt-2">
              <dt className="text-base font-semibold text-ink-900">
                {t("common.total")}
              </dt>
              <dd className="font-mono text-lg font-semibold tabular-nums text-ink-950">
                <CurrencyPrice
                  amount={totalAmount}
                  fromCurrency={defaultCurrency}
                  displayCurrency={displayCurrency}
                 locale={moneyLocale}
                      />
              </dd>
            </div>
          </dl>
        )}

        {/* Pay Button — Mercado Pago uses official brand blue + dedicated label. */}
        {pluginsReady &&
        !pluginsLoadError &&
        !mercadoPagoPlugin &&
        paymentMethods.length === 0 ? null : (
        <Button
          size="lg"
          onPress={handlePayment}
          isDisabled={!selectedMethod || isProcessingPlugin || cashierPaymentLoading}
          isLoading={isProcessingPlugin || cashierPaymentLoading}
          className={
            isMercadoPagoSelected
              ? "h-12 w-full rounded-xl font-semibold text-white shadow-sm transition-colors active:translate-y-px disabled:cursor-not-allowed disabled:opacity-40 sm:h-14"
              : "h-12 w-full rounded-xl bg-brand font-semibold text-white shadow-sm transition-colors hover:bg-brand-dark active:translate-y-px disabled:cursor-not-allowed disabled:opacity-40 sm:h-14"
          }
          style={
            // eslint-disable-next-line no-restricted-syntax -- Mercado Pago official brand blue
            isMercadoPagoSelected ? { backgroundColor: "#009EE3" } : undefined
          }
          startContent={
            isMercadoPagoSelected && mercadoPagoPlugin
              ? getPluginIcon(mercadoPagoPlugin)
              : selectedMethod
                ? paymentMethods.find((m) => m.id === selectedMethod)?.icon
                : null
          }
        >
          {!selectedMethod ? (
            t("bill.selectPaymentMethod")
          ) : selectedMethod === "cashier" ? (
            t("bill.payWithCashier")
          ) : isMercadoPagoSelected ? (
            <span className="inline-flex items-center gap-2">
              {t("payment.payWithMercadoPago")}
              <span className="font-mono tabular-nums">·</span>
              <CurrencyPrice
                amount={totalAmount}
                fromCurrency={defaultCurrency}
                displayCurrency={displayCurrency}
                locale={moneyLocale}
              />
            </span>
          ) : (
            <span className="inline-flex items-center gap-2">
              {t("bill.payNow")}
              <span className="font-mono tabular-nums">·</span>
              <CurrencyPrice
                amount={totalAmount}
                fromCurrency={defaultCurrency}
                displayCurrency={displayCurrency}
                locale={moneyLocale}
              />
            </span>
          )}
        </Button>
        )}
      </div>

      {isUSDCPaymentOpen && (
        <PaymentProcessor
          isOpen={isUSDCPaymentOpen}
          onClose={() => setIsUSDCPaymentOpen(false)}
          billToken={billToken}
          totalAmount={totalAmount}
          tipAmount={effectiveTip}
          businessName={businessName}
          businessAddress={businessAddress}
          currency={defaultCurrency}
          splitShareId={splitShareId}
          onPaymentComplete={(paymentDetails) => {
            onPaymentComplete({
              totalPaid: paymentDetails.totalPaid,
              tipAmount: paymentDetails.tipAmount,
              paymentMethod: paymentDetails.paymentMethod,
              transactionId: paymentDetails.transactionId,
            });
          }}
        />
      )}

      {isCrossChainPaymentOpen && (
        <CrossChainPayment
          isOpen={isCrossChainPaymentOpen}
          onClose={() => setIsCrossChainPaymentOpen(false)}
          billToken={billToken}
          totalAmount={totalAmount}
          tipAmount={effectiveTip}
          businessName={businessName}
          businessAddress={businessAddress}
          currency={defaultCurrency}
          splitShareId={splitShareId}
          onPaymentComplete={(paymentDetails) => {
            onPaymentComplete({
              totalPaid: paymentDetails.totalPaid,
              tipAmount: paymentDetails.tipAmount,
              paymentMethod: paymentDetails.paymentMethod,
              transactionId: paymentDetails.transactionId,
            });
          }}
        />
      )}

      {/* Server confirmation for non-redirect plugin payments (C-04). */}
      {inlineStatusCheck && (
        <PaymentStatusChecker
          isOpen={!!inlineStatusCheck}
          onClose={() => setInlineStatusCheck(null)}
          billToken={billToken}
          paymentId={inlineStatusCheck.paymentId}
          paymentMethod={inlineStatusCheck.method}
          fallbackTotalPaid={totalAmount}
          fallbackTipAmount={effectiveTip}
          onPaymentConfirmed={(paymentDetails) => {
            setInlineStatusCheck(null);
            onPaymentComplete({
              totalPaid: paymentDetails.totalPaid,
              tipAmount: paymentDetails.tipAmount,
              paymentMethod: paymentDetails.paymentMethod,
              transactionId: paymentDetails.transactionId,
            });
          }}
        />
      )}
    </>
  );
}
