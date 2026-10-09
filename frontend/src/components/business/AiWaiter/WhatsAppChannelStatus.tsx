"use client";

import React, { useCallback, useEffect, useState } from "react";
import { Button, Chip, Spinner } from "@nextui-org/react";
import { axiosInstance } from "@/api/tools/instance";
import { useInstance } from "@/hooks/useInstance";
import { getTranslation } from "@/i18n/SimpleTranslationProvider";
import {
  DATE_TIME_SHORT,
  formatBusinessDateTime,
} from "@/utils/businessTime";

type WhatsAppChannelState = {
  status: "pairing" | "connecting" | "connected" | "degraded" | "disconnected" | string;
  last_error_code?: string;
  last_connected_at?: string;
  retry_at?: string;
};

/**
 * GET /whatsapp/status wire shape. `built` is false on the default backend
 * image (the channel is compiled out; see docs/self-hosting/whatsapp.md),
 * `requested` mirrors the server's WHATSAPP_ENABLED switch, and `enabled` is
 * true only once the channel actually started (built AND requested AND the
 * manager came up).
 */
type WhatsAppStatusResponse = WhatsAppChannelState & {
  built?: boolean;
  enabled?: boolean;
  requested?: boolean;
};

/**
 * POST /whatsapp/connect and GET /whatsapp/qr wire shape. `qr_code` is the raw
 * pairing payload whatsmeow wants scanned; it rotates every ~20s while the
 * phone has not scanned yet, so the card re-reads it from /qr.
 */
type WhatsAppPairingResponse = {
  status?: string;
  qr_code?: string;
};

/** How often the card re-reads the rotating pairing code while pairing. */
export const WHATSAPP_QR_POLL_MS = 3000;

/**
 * Consecutive transient /qr failures (network, 5xx) after which the card stops
 * polling, says so, and re-reads status instead of retrying forever.
 */
export const WHATSAPP_QR_MAX_FAILURES = 5;

/** /qr answers that will not change on retry: auth, locked business, not mounted. */
const QR_DEFINITIVE_REFUSALS = new Set([401, 403, 404]);

type Availability = { built: boolean; enabled: boolean; requested: boolean };

type Props = {
  businessId: number;
  /** Operator locale — required so status copy is never hard-English (L4-3). */
  locale: string;
};

function statusColor(status: string): "success" | "warning" | "danger" | "default" | "primary" {
  switch (status) {
    case "connected":
      return "success";
    case "degraded":
      return "warning";
    case "pairing":
    case "connecting":
      return "primary";
    case "disconnected":
      return "default";
    default:
      return "default";
  }
}

export function formatWhatsAppStatusLabel(
  status: string,
  t: (key: string, vars?: Record<string, string | number>) => string,
): string {
  const key = `whatsapp.status.${status}`;
  const label = t(key);
  return label === key ? status : label;
}

export function formatWhatsAppError(
  code: string | undefined,
  t: (key: string, vars?: Record<string, string | number>) => string,
): string | null {
  if (!code) return null;
  const key = `whatsapp.errors.${code}`;
  const label = t(key);
  if (label !== key) return label;
  return t("whatsapp.errors.fallback");
}

function formatChannelTime(iso: string | undefined, locale: string): string {
  if (!iso) return "";
  return formatBusinessDateTime(iso, locale, null, DATE_TIME_SHORT);
}

/**
 * Instance gate: when GET /api/v1/instance confirms WhatsApp is off on this
 * install, render nothing and skip the status probe entirely.
 */
export default function WhatsAppChannelStatus(props: Props) {
  const { isOff } = useInstance();
  if (isOff("whatsapp")) return null;
  return <WhatsAppChannelStatusCard {...props} />;
}

function WhatsAppChannelStatusCard({ businessId, locale }: Props) {
  const t = useCallback(
    (key: string, vars?: Record<string, string | number>) => {
      const value = getTranslation(`aiWaiterDashboard.${key}`, locale as "en" | "es" | "es-AR", vars);
      return Array.isArray(value) ? value.join(" ") : String(value ?? key);
    },
    [locale],
  );
  const [state, setState] = useState<WhatsAppChannelState | null>(null);
  // null until the first status response: the card stays hidden rather than
  // flashing on servers whose binary has no WhatsApp channel at all.
  const [availability, setAvailability] = useState<Availability | null>(null);
  const [loading, setLoading] = useState(true);
  const [busy, setBusy] = useState(false);
  const [actionFailed, setActionFailed] = useState(false);
  // Raw pairing code to scan and its rendered QR image. Only an operator who
  // can reach the settings:write /qr route ever gets one.
  const [pairingCode, setPairingCode] = useState<string | null>(null);
  const [qrImage, setQrImage] = useState<string | null>(null);
  // Set when /qr refuses this viewer (e.g. settings:read only or a suspended
  // business: 403), is not mounted (404), or keeps failing, so the card
  // stops asking instead of polling every few seconds.
  const [qrUnavailable, setQrUnavailable] = useState(false);

  const refresh = useCallback(async () => {
    setLoading(true);
    try {
      const { data } = await axiosInstance.get<WhatsAppStatusResponse>(
        `/inside/businesses/${businessId}/whatsapp/status`,
      );
      // Strict `=== true`: a backend that predates the field is treated as
      // not built, so the default (GPL-free) image never shows the channel.
      setAvailability({
        built: data?.built === true,
        enabled: data?.enabled === true,
        requested: data?.requested === true,
      });
      setState({
        status: data?.status || "disconnected",
        last_error_code: data?.last_error_code,
        last_connected_at: data?.last_connected_at,
        retry_at: data?.retry_at,
      });
    } catch {
      // Keep the last known availability: an error before the first answer
      // leaves the card hidden; after it, the channel reads as disconnected.
      setState({ status: "disconnected" });
    } finally {
      setLoading(false);
    }
  }, [businessId]);

  useEffect(() => {
    void refresh();
  }, [refresh]);

  const status = state?.status || "disconnected";
  const pollQr = availability?.enabled === true && status === "pairing" && !qrUnavailable;

  // whatsmeow rotates the pairing code while the phone has not scanned, so
  // keep reading the latest one. An empty answer means pairing ended (paired,
  // expired or disconnected): re-read status once it is no longer "pairing".
  useEffect(() => {
    if (!pollQr) return undefined;
    let cancelled = false;
    let failures = 0;
    const tick = async () => {
      try {
        // Background poll: failures are handled here, so no global toast
        // every few seconds while the backend is unreachable.
        const { data } = await axiosInstance.get<WhatsAppPairingResponse>(
          `/inside/businesses/${businessId}/whatsapp/qr`,
          { _skipErrorToast: true },
        );
        if (cancelled) return;
        failures = 0;
        setPairingCode(data?.qr_code || null);
        if (!data?.qr_code && data?.status !== "pairing") void refresh();
      } catch (err: unknown) {
        if (cancelled) return;
        // The shared axios interceptor rejects with a sanitized Error that
        // carries the HTTP status. A definitive refusal stops the poll at
        // once; a network blip or 5xx keeps the current code and retries,
        // up to WHATSAPP_QR_MAX_FAILURES ticks in a row.
        const httpStatus =
          (err as { response?: { status?: number }; status?: number })?.response?.status ??
          (err as { status?: number })?.status;
        if (httpStatus !== undefined && QR_DEFINITIVE_REFUSALS.has(httpStatus)) {
          setPairingCode(null);
          setQrUnavailable(true);
          return;
        }
        failures += 1;
        if (failures >= WHATSAPP_QR_MAX_FAILURES) {
          cancelled = true;
          setPairingCode(null);
          setQrUnavailable(true);
          setActionFailed(true);
          void refresh();
        }
      }
    };
    void tick();
    const timer = setInterval(() => void tick(), WHATSAPP_QR_POLL_MS);
    return () => {
      cancelled = true;
      clearInterval(timer);
    };
  }, [pollQr, businessId, refresh]);

  useEffect(() => {
    if (!pairingCode) {
      setQrImage(null);
      return undefined;
    }
    let cancelled = false;
    void (async () => {
      try {
        const QRCode = (await import("qrcode")).default;
        /* eslint-disable no-restricted-syntax -- qrcode expects hex, not Tailwind tokens */
        const dataUrl = await QRCode.toDataURL(pairingCode, {
          errorCorrectionLevel: "M",
          margin: 1,
          width: 224,
          color: { dark: "#1c1917", light: "#ffffff" },
        });
        /* eslint-enable no-restricted-syntax */
        if (!cancelled) setQrImage(dataUrl);
      } catch {
        if (!cancelled) setQrImage(null);
      }
    })();
    return () => {
      cancelled = true;
    };
  }, [pairingCode]);

  const runAction = async (action: "connect" | "disconnect") => {
    setBusy(true);
    setActionFailed(false);
    if (action === "disconnect") setPairingCode(null);
    try {
      const { data } = await axiosInstance.post<WhatsAppPairingResponse>(
        `/inside/businesses/${businessId}/whatsapp/${action}`,
      );
      if (action === "connect") {
        setQrUnavailable(false);
        setPairingCode(data?.qr_code || null);
      }
    } catch {
      setActionFailed(true);
    }
    try {
      await refresh();
    } finally {
      setBusy(false);
    }
  };

  if (!availability?.built) return null;

  const errorCopy = formatWhatsAppError(state?.last_error_code, t);
  // Connect/disconnect routes are only mounted when the server runs the
  // channel (built AND WHATSAPP_ENABLED=true); otherwise offer no actions.
  const enabled = availability.enabled;
  // Built and WHATSAPP_ENABLED=true, yet the manager is not running: the
  // backend failed to start the channel, so "set the variable" would mislead.
  const startFailed = !enabled && availability.requested;
  const showConnect = enabled && (status === "disconnected" || status === "degraded");
  const showDisconnect =
    enabled && (status === "connected" || status === "pairing" || status === "connecting");

  return (
    <div
      className="rounded-xl border border-warm-200 bg-warm-50/60 p-4"
      data-testid="whatsapp-channel-status"
    >
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div>
          <p className="text-sm font-semibold text-ink-900">{t("whatsapp.title")}</p>
          <p className="text-xs text-ink-500">{t("whatsapp.subtitle")}</p>
        </div>
        {loading ? (
          <Spinner size="sm" />
        ) : (
          <Chip size="sm" color={statusColor(status)} variant="flat">
            {formatWhatsAppStatusLabel(status, t)}
          </Chip>
        )}
      </div>
      {!loading && (
        <div className="mt-3 space-y-1 text-xs text-ink-600">
          {startFailed && (
            <p data-testid="whatsapp-start-failed" role="status">
              {t("whatsapp.startFailed")}
            </p>
          )}
          {!enabled && !startFailed && (
            <p data-testid="whatsapp-not-enabled">{t("whatsapp.notEnabled")}</p>
          )}
          {actionFailed && <p role="alert">{t("whatsapp.actionFailed")}</p>}
          {state?.last_connected_at && status === "connected" && (
            <p>
              {t("whatsapp.lastConnected", {
                time: formatChannelTime(state.last_connected_at, locale),
              })}
            </p>
          )}
          {status === "degraded" && errorCopy && <p role="status">{errorCopy}</p>}
          {state?.retry_at && status === "degraded" && (
            <p>
              {t("whatsapp.retryAt", {
                time: formatChannelTime(state.retry_at, locale),
              })}
            </p>
          )}
        </div>
      )}
      {enabled && status === "pairing" && qrImage && (
        <div className="mt-3 flex flex-col items-center gap-2" data-testid="whatsapp-pairing">
          {/* eslint-disable-next-line @next/next/no-img-element -- QR data URL from qrcode lib */}
          <img
            src={qrImage}
            alt={t("whatsapp.qrAlt")}
            width={224}
            height={224}
            className="rounded-xl border border-warm-200 bg-white p-2"
            data-testid="whatsapp-qr"
          />
          <p className="max-w-xs text-center text-xs text-ink-600">{t("whatsapp.scanHint")}</p>
        </div>
      )}
      <div className="mt-3 flex flex-wrap gap-2">
        {showConnect && (
          <Button size="sm" color="primary" isLoading={busy} onPress={() => void runAction("connect")}>
            {status === "degraded" ? t("whatsapp.reconnect") : t("whatsapp.connect")}
          </Button>
        )}
        {showDisconnect && (
          <Button
            size="sm"
            variant="bordered"
            isLoading={busy}
            onPress={() => void runAction("disconnect")}
          >
            {t("whatsapp.disconnect")}
          </Button>
        )}
      </div>
    </div>
  );
}
