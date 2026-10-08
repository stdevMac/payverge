"use client";

/**
 * Wave 4 Task 14 — honest crypto refund lifecycle UI.
 *
 * NEVER labels requested/submitted/confirming as "refunded".
 * Only status === "confirmed" is presented as refunded.
 * Mainnet signing handoff is OFF by default; UI shows manual tx-hash workflow.
 */

import React, { useCallback, useEffect, useState } from "react";
import Link from "next/link";
import { usePathname } from "next/navigation";
import { Button, Chip, Input, Textarea } from "@nextui-org/react";
import { StatusChip, type StatusTone } from "@/components/ui/StatusChip";
import toast from "react-hot-toast";
import { ExternalLink, ShieldAlert } from "lucide-react";
import {
  CryptoRefund,
  RefundDestinationView,
  UnsignedRefundTransferRequest,
  approveCryptoRefund,
  getCryptoRefundUnsignedRequest,
  getPaymentRefundDestination,
  isCryptoRefundCompleted,
  listCryptoRefunds,
  rejectCryptoRefund,
  requestCryptoRefund,
  submitCryptoRefundTx,
  type Payment,
} from "@/api/bills";
import { useWithManagerPin } from "@/components/business/managerPin/ManagerPinProvider";
import {
  useSimpleLocale,
  getTranslation,
} from "@/i18n/SimpleTranslationProvider";
import { getSafeApiErrorMessage } from "@/utils/apiError";

interface CryptoRefundPanelProps {
  businessId: number;
  billId: number;
  payment: Payment;
  onRefresh: () => void;
}

const generateIdempotencyKey = (): string => {
  if (typeof crypto !== "undefined" && typeof crypto.randomUUID === "function") {
    return crypto.randomUUID();
  }
  return `cr-${Date.now()}-${Math.random().toString(16).slice(2)}`;
};

const formatUsdc = (baseUnits: number): string => {
  // USDC 6 decimals — integer base units only.
  const whole = Math.trunc(baseUnits / 1_000_000);
  const frac = Math.abs(baseUnits % 1_000_000)
    .toString()
    .padStart(6, "0")
    .replace(/0+$/, "");
  return frac ? `${whole}.${frac}` : `${whole}`;
};

// Maps each known refund lifecycle enum to a shared StatusChip tone. Unknown
// enums fall through to `undefined` so the caller renders a neutral chip with a
// humanized fallback (and logs the surprise) rather than a raw snake_case token.
const STATUS_TONES: Record<string, StatusTone> = {
  requested: "info",
  approved: "info",
  awaiting_signature: "info",
  submitted: "warn",
  confirming: "warn",
  confirmed: "success",
  failed: "danger",
  rejected: "danger",
  cancelled: "danger",
};

// Turns an unmapped backend enum into a readable label without leaking the raw
// snake_case token (e.g. "awaiting_signature" -> "Awaiting signature").
const humanizeStatus = (status: string): string => {
  const spaced = status.replace(/[_-]+/g, " ").trim();
  if (!spaced) return status;
  return spaced.charAt(0).toUpperCase() + spaced.slice(1);
};

export const CryptoRefundPanel: React.FC<CryptoRefundPanelProps> = ({
  businessId,
  billId,
  payment,
  onRefresh,
}) => {
  const { withManagerPin } = useWithManagerPin();
  const { locale } = useSimpleLocale();
  const pathname = usePathname() || "/";
  const t = useCallback(
    (key: string): string => {
      const result = getTranslation(`billManager.cryptoRefund.${key}`, locale);
      return Array.isArray(result) ? result[0] || key : (result as string);
    },
    [locale],
  );

  // Renders a refund status as a translated, tone-colored StatusChip. Known
  // enums resolve to a billManager.cryptoRefund.status.* label + a semantic
  // tone. An unknown enum (backend added a state the UI hasn't caught up to)
  // renders a NEUTRAL chip with a humanized label — never raw snake_case — and
  // is logged so the drift surfaces.
  const renderStatusChip = useCallback(
    (status: string) => {
      const tone = STATUS_TONES[status];
      if (!tone) {
        console.warn(
          `[CryptoRefundPanel] Unmapped refund status enum: "${status}"`,
        );
        return <StatusChip tone="neutral" label={humanizeStatus(status)} />;
      }
      // Known enums resolve to a translated label. If the key were ever
      // missing, getTranslation's own leaf fallback humanizes the tail
      // (never raw snake_case) and reports the gap to telemetry.
      return <StatusChip tone={tone} label={t(`status.${status}`)} />;
    },
    [t],
  );

  const [dest, setDest] = useState<RefundDestinationView | null>(null);
  const [refunds, setRefunds] = useState<CryptoRefund[]>([]);
  const [reason, setReason] = useState("");
  const [txHash, setTxHash] = useState("");
  const [unsigned, setUnsigned] = useState<UnsignedRefundTransferRequest | null>(
    null,
  );
  const [busy, setBusy] = useState(false);
  const [loadError, setLoadError] = useState<string | null>(null);

  const load = useCallback(async () => {
    try {
      setLoadError(null);
      const [d, list] = await Promise.all([
        getPaymentRefundDestination(businessId, payment.id),
        // Scope the fetch to this payment server-side (was a business-wide
        // fetch filtered client-side, which the 100-cap could truncate).
        listCryptoRefunds(businessId, payment.id),
      ]);
      setDest(d);
      setRefunds(list);
    } catch {
      setLoadError(t("errors.loadFailed"));
    }
  }, [businessId, payment.id, t]);

  useEffect(() => {
    void load();
  }, [load]);

  const active = refunds.find(
    (r) =>
      !["confirmed", "failed", "rejected", "cancelled"].includes(r.status),
  );
  // Task 8: mainnet=off must surface as an explicit disabled control, not a
  // silent no-op. Detect from any refund row or the unsigned-request payload.
  const mainnetOff =
    refunds.some((r) => r.mainnet_submission_off) ||
    Boolean(unsigned?.mainnet_disabled);
  const settingsHref = `${pathname}?tab=settings`;
  const isCrypto =
    payment.payment_method === "crypto" ||
    payment.payment_method === "cross-chain";

  if (!isCrypto || payment.status !== "confirmed") {
    return null;
  }

  const handleRequest = async () => {
    if (!dest?.has_evidence || dest.manual_support_required) {
      toast.error(t("errors.manualSupport"));
      return;
    }
    if (reason.trim().length < 3) {
      toast.error(t("errors.reasonTooShort"));
      return;
    }
    const amount = dest.refundable_base_units ?? dest.amount_base_units ?? 0;
    if (amount <= 0) {
      toast.error(t("errors.insufficientBalance"));
      return;
    }
    const idempotencyKey = generateIdempotencyKey();
    setBusy(true);
    try {
      await withManagerPin(
        (pin) =>
          requestCryptoRefund(
            businessId,
            {
              bill_id: billId,
              payment_id: payment.id,
              amount_base_units: amount,
              reason: reason.trim(),
              idempotency_key: idempotencyKey,
            },
            { pin, idempotencyKey },
          ),
        { title: t("pin.requestTitle"), description: t("pin.requestDescription") },
      );
      toast.success(t("toasts.requested"));
      setReason("");
      await load();
    } catch (error) {
      const message = error instanceof Error ? error.message : "";
      if (message === "PIN entry cancelled") return;
      // FIND-058: never toast raw axios/gin dumps or unfiltered response.error.
      toast.error(getSafeApiErrorMessage(error, t("errors.requestFailed")));
    } finally {
      setBusy(false);
    }
  };

  const handleApprove = async (refundId: number) => {
    const idempotencyKey = generateIdempotencyKey();
    setBusy(true);
    try {
      await withManagerPin(
        (pin) =>
          approveCryptoRefund(businessId, refundId, { pin, idempotencyKey }),
        { title: t("pin.approveTitle"), description: t("pin.approveDescription") },
      );
      toast.success(t("toasts.approved"));
      const u = await getCryptoRefundUnsignedRequest(businessId, refundId);
      setUnsigned(u);
      await load();
    } catch (error) {
      const message = error instanceof Error ? error.message : "";
      if (message === "PIN entry cancelled") return;
      toast.error(getSafeApiErrorMessage(error, t("errors.approveFailed")));
    } finally {
      setBusy(false);
    }
  };

  const handleReject = async (refundId: number) => {
    const idempotencyKey = generateIdempotencyKey();
    setBusy(true);
    try {
      await withManagerPin(
        (pin) =>
          rejectCryptoRefund(businessId, refundId, reason || "rejected", {
            pin,
            idempotencyKey,
          }),
        { title: t("pin.rejectTitle"), description: t("pin.rejectDescription") },
      );
      toast.success(t("toasts.rejected"));
      await load();
    } catch (error) {
      const message = error instanceof Error ? error.message : "";
      if (message === "PIN entry cancelled") return;
      toast.error(t("errors.rejectFailed"));
    } finally {
      setBusy(false);
    }
  };

  const handleSubmitTx = async (refundId: number) => {
    const hash = txHash.trim();
    if (!hash.startsWith("0x") || hash.length < 10) {
      toast.error(t("errors.invalidTxHash"));
      return;
    }
    const idempotencyKey = generateIdempotencyKey();
    setBusy(true);
    try {
      await withManagerPin(
        (pin) =>
          submitCryptoRefundTx(businessId, refundId, hash, {
            pin,
            idempotencyKey,
          }),
        {
          title: t("pin.submitTitle"),
          description: t("pin.submitDescription"),
        },
      );
      toast.success(t("toasts.submitted"));
      setTxHash("");
      await load();
      onRefresh();
    } catch (error) {
      const message = error instanceof Error ? error.message : "";
      if (message === "PIN entry cancelled") return;
      toast.error(
        getSafeApiErrorMessage(error, t("errors.submitFailed")),
      );
    } finally {
      setBusy(false);
    }
  };

  return (
    <div className="space-y-3 rounded-2xl border border-brand-200/60 bg-brand-50/30 p-4">
      <div className="flex items-start gap-2">
        <ShieldAlert className="mt-0.5 h-4 w-4 shrink-0 text-brand-700" />
        <div>
          <h4 className="text-sm font-semibold text-ink-950">{t("title")}</h4>
          <p className="text-xs text-ink-600">{t("subtitle")}</p>
        </div>
      </div>

      {loadError && (
        <p className="text-sm text-rose-700" role="alert">
          {loadError}
        </p>
      )}

      {dest && !dest.has_evidence && (
        <div
          className="rounded-xl border border-amber-200 bg-amber-50 px-3 py-2 text-sm text-amber-900"
          role="status"
        >
          {t("manualSupportRequired")}
        </div>
      )}

      {mainnetOff && (
        <div
          className="space-y-2 rounded-xl border border-amber-200 bg-amber-50 px-3 py-2"
          role="status"
          data-testid="crypto-refund-mainnet-off"
        >
          <p className="text-sm text-amber-900">{t("mainnetOffNotice")}</p>
          <div className="flex flex-wrap items-center gap-2">
            <Button
              size="sm"
              variant="flat"
              isDisabled
              aria-label={t("mainnetOffDisabled")}
              className="bg-amber-100/80 font-medium text-amber-900"
            >
              {t("mainnetOffDisabled")}
            </Button>
            <Link
              href={settingsHref}
              className="text-sm font-medium text-brand underline decoration-brand/40 underline-offset-2 hover:text-brand-dark"
            >
              {t("mainnetOffSettingsLink")}
            </Link>
          </div>
        </div>
      )}

      {dest?.has_evidence && (
        <div className="grid gap-1 text-sm text-ink-700">
          <div>
            <span className="font-medium">{t("labels.destination")}: </span>
            {dest.masked_address}
          </div>
          <div>
            <span className="font-medium">{t("labels.network")}: </span>
            chain {dest.chain_id} · {dest.token}
          </div>
          <div>
            <span className="font-medium">{t("labels.refundable")}: </span>
            {formatUsdc(dest.refundable_base_units ?? dest.amount_base_units ?? 0)}{" "}
            USDC
          </div>
        </div>
      )}

      {refunds.map((r) => (
        <div
          key={r.id}
          className="space-y-2 rounded-xl border border-warm-200 bg-white p-3"
          data-testid={`crypto-refund-${r.id}`}
        >
          <div className="flex flex-wrap items-center gap-2">
            {renderStatusChip(r.status)}
            {isCryptoRefundCompleted(r.status) && (
              <Chip size="sm" color="success" variant="solid">
                {t("labels.refunded")}
              </Chip>
            )}
          </div>
          <p className="text-xs text-ink-500">
            {formatUsdc(r.amount_base_units)} {r.token} → {r.masked_recipient}
          </p>
          {r.submitted_tx_hash && (
            <p className="text-xs text-ink-600">
              tx: {r.submitted_tx_hash}
              {r.explorer_url && (
                <a
                  href={r.explorer_url}
                  target="_blank"
                  rel="noreferrer"
                  className="ml-2 inline-flex items-center gap-1 text-brand-700 underline"
                >
                  {t("labels.explorer")} <ExternalLink className="h-3 w-3" />
                </a>
              )}
            </p>
          )}
          {r.last_error && (
            <p className="text-xs text-rose-700">{r.last_error}</p>
          )}

          {r.status === "requested" && (
            <div className="flex flex-wrap gap-2">
              <Button
                size="sm"
                color="primary"
                isDisabled={busy}
                onPress={() => void handleApprove(r.id)}
              >
                {t("actions.approve")}
              </Button>
              <Button
                size="sm"
                variant="flat"
                color="danger"
                isDisabled={busy}
                onPress={() => void handleReject(r.id)}
              >
                {t("actions.reject")}
              </Button>
            </div>
          )}

          {(r.status === "awaiting_signature" ||
            r.status === "approved" ||
            r.status === "failed") && (
            <div className="space-y-2">
              {unsigned && unsigned.refund_id === r.id && (
                <p className="text-xs text-ink-600">{unsigned.message}</p>
              )}
              <Input
                size="sm"
                label={t("labels.txHash")}
                placeholder="0x…"
                value={txHash}
                onValueChange={setTxHash}
                description={t("labels.txHashHint")}
              />
              <Button
                size="sm"
                color="warning"
                isDisabled={busy}
                onPress={() => void handleSubmitTx(r.id)}
              >
                {t("actions.submitTx")}
              </Button>
              <p className="text-xs text-ink-500">{t("irreversibleNotice")}</p>
            </div>
          )}
        </div>
      ))}

      {!active && dest?.has_evidence && (
        <div className="space-y-2">
          <Textarea
            size="sm"
            minRows={2}
            label={t("labels.reason")}
            value={reason}
            onValueChange={setReason}
            placeholder={t("labels.reasonPlaceholder")}
          />
          <Button
            size="sm"
            color="primary"
            isDisabled={busy}
            onPress={() => void handleRequest()}
          >
            {t("actions.request")}
          </Button>
        </div>
      )}
    </div>
  );
};
