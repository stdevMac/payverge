"use client";

/* eslint-disable no-restricted-syntax -- qrcode library requires absolute hex color options */

import React, { useCallback, useEffect, useMemo, useRef, useState } from "react";
import {
  Button,
  Modal,
  ModalBody,
  ModalContent,
  ModalFooter,
  ModalHeader,
  Spinner,
} from "@nextui-org/react";
import {
  Check,
  Copy,
  ExternalLink,
  RefreshCw,
  Smartphone,
  X,
} from "lucide-react";
import QRCode from "qrcode";
import toast from "react-hot-toast";
import {
  spacesApi,
  type CreateScanSessionResponse,
  type LayoutDocument,
  type LayoutTable,
  type ScanSession,
  type ScanStatus,
} from "@/api/spaces";
import { useSSEEvents, type SSEEvent } from "@/hooks/useSSEEvents";
import { getSafeApiErrorMessage } from "@/utils/apiError";
import SpaceScanStatusBadge from "./SpaceScanStatusBadge";
import SpaceScanReview from "./SpaceScanReview";
import {
  hasCameraApi,
  isActiveScanStatus,
  isMobileUserAgent,
  isTerminalScanStatus,
} from "./scanStatus";
import {
  buildReviewApplyLayout,
  resolveReviewApplyMode,
} from "./reviewApplyLayout";

export interface SpaceScanSessionModalProps {
  isOpen: boolean;
  onOpenChange: (open: boolean) => void;
  businessId: number;
  spaceId: number;
  t: (key: string, params?: Record<string, string | number>) => string;
  /** Called when review is applied or user opens editor after review_ready. */
  onReviewReady: (opts: {
    spaceId: number;
    sessionId: number;
    layout?: LayoutDocument | null;
    openEditor: boolean;
  }) => void;
  /** When true on open, start a new session automatically. */
  autoStart?: boolean;
}

type ModalPhase =
  | "idle"
  | "creating"
  | "pairing"
  | "live"
  | "review"
  | "error";

function buildScanUrl(token: string, pairCode?: string | null): string {
  if (typeof window === "undefined") {
    return `/space-scan/${token}`;
  }
  const url = new URL(`/space-scan/${token}`, window.location.origin);
  if (pairCode) {
    url.searchParams.set("pair", pairCode);
  }
  return url.toString();
}

export default function SpaceScanSessionModal({
  isOpen,
  onOpenChange,
  businessId,
  spaceId,
  t,
  onReviewReady,
  autoStart = true,
}: SpaceScanSessionModalProps) {
  const [phase, setPhase] = useState<ModalPhase>("idle");
  const [session, setSession] = useState<ScanSession | null>(null);
  const [token, setToken] = useState<string | null>(null);
  const [pairCode, setPairCode] = useState<string | null>(null);
  const [qrDataUrl, setQrDataUrl] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const [copied, setCopied] = useState<"url" | "code" | null>(null);
  const [reviewLayout, setReviewLayout] = useState<LayoutDocument | null>(null);
  const [applyingReview, setApplyingReview] = useState(false);
  const [mobileDirect, setMobileDirect] = useState(false);

  const startedRef = useRef(false);
  const reviewOpenedRef = useRef(false);
  const sessionIdRef = useRef<number | null>(null);

  const scanUrl = useMemo(
    () => (token ? buildScanUrl(token, pairCode) : ""),
    [token, pairCode],
  );

  const reset = useCallback(() => {
    setPhase("idle");
    setSession(null);
    setToken(null);
    setPairCode(null);
    setQrDataUrl(null);
    setError(null);
    setBusy(false);
    setCopied(null);
    setReviewLayout(null);
    setApplyingReview(false);
    setMobileDirect(false);
    startedRef.current = false;
    reviewOpenedRef.current = false;
    sessionIdRef.current = null;
  }, []);

  const applySessionUpdate = useCallback(
    (next: Partial<ScanSession> & { status?: ScanStatus | string }) => {
      setSession((prev) => {
        if (!prev) {
          return prev;
        }
        const merged = { ...prev, ...next } as ScanSession;
        if (next.status === "review_ready" || next.status === "completed") {
          setPhase("review");
        } else if (isActiveScanStatus(String(next.status ?? prev.status))) {
          setPhase("live");
        } else if (
          next.status === "failed" ||
          next.status === "expired" ||
          next.status === "cancelled"
        ) {
          setPhase("error");
        }
        return merged;
      });
    },
    [],
  );

  const fetchSession = useCallback(async () => {
    const id = sessionIdRef.current;
    if (!id) return;
    try {
      const detail = await spacesApi.getScanSession(businessId, id);
      applySessionUpdate(detail.session);
      if (
        (detail.session.status === "review_ready" ||
          detail.session.status === "completed") &&
        detail.session.result_layout_json
      ) {
        const layout =
          typeof detail.session.result_layout_json === "object"
            ? (detail.session.result_layout_json as LayoutDocument)
            : null;
        if (layout) setReviewLayout(layout);
      }
    } catch {
      // Polling is best-effort; keep last known state.
    }
  }, [businessId, applySessionUpdate]);

  // Live status via SSE with 2s polling fallback.
  const onSseEvent = useCallback(
    (event: SSEEvent) => {
      if (event.type !== "space.scan.updated") return;
      const sid = Number(event.data.session_id);
      if (!sessionIdRef.current || sid !== sessionIdRef.current) return;
      applySessionUpdate({
        status: String(event.data.status || "") as ScanStatus,
        progress_pct: Number(event.data.progress_pct ?? 0),
        progress_message:
          typeof event.data.progress_message === "string"
            ? event.data.progress_message
            : null,
      });
    },
    [applySessionUpdate],
  );

  useSSEEvents({
    businessId,
    enabled: isOpen && Boolean(session?.id),
    onEvent: onSseEvent,
    onReconnect: () => {
      void fetchSession();
    },
  });

  useEffect(() => {
    if (!isOpen || !session?.id) return;
    if (isTerminalScanStatus(session.status) && session.status !== "review_ready") {
      return;
    }
    const id = window.setInterval(() => {
      void fetchSession();
    }, 2000);
    return () => window.clearInterval(id);
  }, [isOpen, session?.id, session?.status, fetchSession]);

  // Auto-open review when status hits review_ready.
  useEffect(() => {
    if (!session || reviewOpenedRef.current) return;
    if (session.status !== "review_ready" && session.status !== "completed") {
      return;
    }
    reviewOpenedRef.current = true;
    setPhase("review");
    void (async () => {
      try {
        const detail = await spacesApi.getScanSession(businessId, session.id);
        // Merge full session so error_code/error_message (draft_apply_failed)
        // surface immediately without waiting for the 2s poll.
        applySessionUpdate(detail.session);
        if (detail.session.result_layout_json) {
          const raw = detail.session.result_layout_json;
          if (typeof raw === "object" && raw) {
            setReviewLayout(raw as LayoutDocument);
          } else if (typeof raw === "string") {
            try {
              setReviewLayout(JSON.parse(raw) as LayoutDocument);
            } catch {
              setReviewLayout(null);
            }
          }
        }
      } catch {
        // Editor path still works via draft already written by worker.
      }
    })();
  }, [session, businessId, applySessionUpdate]);

  const startSession = useCallback(async () => {
    setPhase("creating");
    setError(null);
    setBusy(true);
    try {
      const idempotencyKey =
        typeof crypto !== "undefined" && crypto.randomUUID
          ? crypto.randomUUID()
          : `scan-${businessId}-${spaceId}-${Date.now()}`;
      const res: CreateScanSessionResponse = await spacesApi.createScanSession(
        businessId,
        spaceId,
        { idempotency_key: idempotencyKey },
      );
      const rawToken = res.token ?? null;
      const code = res.pair_code ?? null;
      setSession(res.session);
      sessionIdRef.current = res.session.id;
      setToken(rawToken);
      setPairCode(code);

      const mobile = isMobileUserAgent() && hasCameraApi();
      setMobileDirect(mobile);

      if (rawToken) {
        const url = buildScanUrl(rawToken, code);
        try {
          const dataUrl = await QRCode.toDataURL(url, {
            width: 240,
            margin: 2,
            color: { dark: "#1c1917", light: "#faf9f6" },
          });
          setQrDataUrl(dataUrl);
        } catch {
          setQrDataUrl(null);
        }

        if (mobile) {
          // Same-device phone path: skip QR, open scanner route with pair code.
          window.location.assign(url);
          return;
        }
      }

      setPhase("pairing");
    } catch (err) {
      setError(getSafeApiErrorMessage(err, t("scan.toasts.createError")));
      setPhase("error");
    } finally {
      setBusy(false);
    }
  }, [businessId, spaceId, t]);

  useEffect(() => {
    if (!isOpen) {
      reset();
      return;
    }
    if (autoStart && !startedRef.current) {
      startedRef.current = true;
      void startSession();
    }
  }, [isOpen, autoStart, reset, startSession]);

  const handleCancel = async () => {
    if (!session) {
      onOpenChange(false);
      return;
    }
    setBusy(true);
    try {
      await spacesApi.cancelScanSession(businessId, session.id);
      toast.success(t("scan.toasts.cancelled"));
      onOpenChange(false);
    } catch (err) {
      toast.error(getSafeApiErrorMessage(err, t("scan.toasts.cancelError")));
    } finally {
      setBusy(false);
    }
  };

  const handleRetry = async () => {
    if (!session) return;
    setBusy(true);
    try {
      const res = await spacesApi.retryProcessScanSession(
        businessId,
        session.id,
      );
      applySessionUpdate(res.session);
      setPhase("live");
      toast.success(t("scan.toasts.retryQueued"));
    } catch (err) {
      toast.error(getSafeApiErrorMessage(err, t("scan.toasts.retryError")));
    } finally {
      setBusy(false);
    }
  };

  const copy = async (kind: "url" | "code", value: string) => {
    try {
      await navigator.clipboard.writeText(value);
      setCopied(kind);
      window.setTimeout(() => setCopied(null), 1500);
    } catch {
      toast.error(t("scan.toasts.copyError"));
    }
  };

  const statusLabel = (status: string) => {
    const key = `scan.status.${status}`;
    const translated = t(key);
    return translated === `spacesTables.${key}` || translated === key
      ? status
      : translated;
  };

  const handleApplyReview = async (
    acceptedTables: LayoutTable[],
    layout: LayoutDocument,
  ) => {
    setApplyingReview(true);
    try {
      const draft = await spacesApi.getLayoutDraft(businessId, spaceId);
      const existing =
        draft.layout && typeof draft.layout === "object"
          ? (draft.layout as LayoutDocument)
          : null;
      // Merge only when auto-apply was refused (sticky error cleared on success).
      // Empty / scan-owned drafts use filtered replace so rejected candidates drop.
      const mode = resolveReviewApplyMode({
        draftApplyFailed: session?.error_code === "draft_apply_failed",
      });
      const finalLayout = buildReviewApplyLayout({
        mode,
        existing,
        reviewLayout: layout,
        acceptedTables,
      });

      await spacesApi.putLayoutDraft(businessId, spaceId, {
        expected_revision: draft.draft_revision,
        layout: finalLayout,
      });
      // Complete/revoke scan token so public result no longer yields the layout.
      if (session?.id) {
        try {
          await spacesApi.completeScanSession(businessId, session.id);
        } catch {
          // Non-fatal: draft is already applied; token may expire via TTL.
        }
      }
      toast.success(t("scan.toasts.reviewApplied"));
      onReviewReady({
        spaceId,
        sessionId: session?.id ?? 0,
        layout: finalLayout,
        openEditor: true,
      });
      onOpenChange(false);
    } catch (err) {
      toast.error(getSafeApiErrorMessage(err, t("scan.toasts.reviewError")));
    } finally {
      setApplyingReview(false);
    }
  };

  const openEditorOnly = () => {
    onReviewReady({
      spaceId,
      sessionId: session?.id ?? 0,
      layout: reviewLayout,
      openEditor: true,
    });
    onOpenChange(false);
  };

  return (
    <Modal
      isOpen={isOpen}
      onOpenChange={onOpenChange}
      size="lg"
      scrollBehavior="inside"
      isDismissable={!busy && phase !== "creating"}
      aria-label={t("scan.modal.aria")}
    >
      <ModalContent>
        {() => (
          <>
            <ModalHeader className="flex flex-col gap-1">
              <span className="font-title text-xl text-ink-900">
                {t("scan.modal.title")}
              </span>
              <span className="text-sm font-normal text-ink-500">
                {t("scan.modal.subtitle")}
              </span>
            </ModalHeader>
            <ModalBody>
              {phase === "creating" || phase === "idle" ? (
                <div
                  className="flex flex-col items-center gap-3 py-8"
                  data-testid="space-scan-creating"
                >
                  <Spinner color="default" />
                  <p className="text-sm text-ink-500">{t("scan.modal.creating")}</p>
                </div>
              ) : null}

              {(phase === "pairing" || phase === "live") && session ? (
                <div className="flex flex-col gap-4" data-testid="space-scan-pairing">
                  <SpaceScanStatusBadge
                    status={session.status}
                    label={statusLabel(session.status)}
                    progressPct={session.progress_pct}
                  />
                  {session.progress_message ? (
                    <p className="text-sm text-ink-500">
                      {session.progress_message}
                    </p>
                  ) : null}

                  {!mobileDirect && qrDataUrl ? (
                    <div className="flex flex-col items-center gap-3">
                      {/* eslint-disable-next-line @next/next/no-img-element -- QR data URL from qrcode lib */}
                      <img
                        src={qrDataUrl}
                        alt={t("scan.modal.qrAlt")}
                        width={240}
                        height={240}
                        className="rounded-xl border border-warm-200 bg-warm-50 p-2"
                        data-testid="space-scan-qr"
                      />
                      <p className="text-center text-sm text-ink-600">
                        {t("scan.pairHint")}
                      </p>
                    </div>
                  ) : null}

                  {pairCode ? (
                    <div
                      className="flex items-center justify-between gap-3 rounded-xl border border-warm-200 bg-warm-50 px-4 py-3"
                      data-testid="space-scan-pair-code"
                    >
                      <div>
                        <p className="text-xs font-medium uppercase tracking-wider text-ink-500">
                          {t("scan.modal.pairCode")}
                        </p>
                        <p className="font-mono text-2xl font-semibold tracking-widest text-ink-900">
                          {pairCode}
                        </p>
                      </div>
                      <Button
                        isIconOnly
                        variant="flat"
                        aria-label={t("scan.modal.copyCode")}
                        onPress={() => void copy("code", pairCode)}
                      >
                        {copied === "code" ? (
                          <Check className="h-4 w-4 text-emerald-600" />
                        ) : (
                          <Copy className="h-4 w-4" />
                        )}
                      </Button>
                    </div>
                  ) : null}

                  {scanUrl ? (
                    <div className="flex flex-col gap-2">
                      <p className="text-xs font-medium uppercase tracking-wider text-ink-500">
                        {t("scan.modal.fallbackUrl")}
                      </p>
                      <div className="flex items-center gap-2">
                        <code
                          className="min-w-0 flex-1 truncate rounded-lg border border-warm-200 bg-white px-3 py-2 text-xs text-ink-700"
                          data-testid="space-scan-url"
                        >
                          {scanUrl}
                        </code>
                        <Button
                          isIconOnly
                          variant="flat"
                          size="sm"
                          aria-label={t("scan.modal.copyUrl")}
                          onPress={() => void copy("url", scanUrl)}
                        >
                          {copied === "url" ? (
                            <Check className="h-4 w-4 text-emerald-600" />
                          ) : (
                            <Copy className="h-4 w-4" />
                          )}
                        </Button>
                        <Button
                          isIconOnly
                          variant="flat"
                          size="sm"
                          as="a"
                          href={scanUrl}
                          target="_blank"
                          rel="noopener noreferrer"
                          aria-label={t("scan.modal.openUrl")}
                        >
                          <ExternalLink className="h-4 w-4" />
                        </Button>
                      </div>
                    </div>
                  ) : (
                    <p className="text-sm text-amber-800" data-testid="space-scan-token-missing">
                      {t("scan.modal.tokenMissing")}
                    </p>
                  )}

                  <div className="flex items-center gap-2 text-xs text-ink-400">
                    <Smartphone className="h-3.5 w-3.5" />
                    <span>{t("scan.modal.waitingHint")}</span>
                  </div>
                </div>
              ) : null}

              {phase === "review" ? (
                <div className="flex flex-col gap-3">
                  {session?.error_code === "draft_apply_failed" ? (
                    <div
                      className="rounded-xl border border-amber-200 bg-amber-50 px-3 py-2 text-sm text-amber-950"
                      data-testid="space-scan-draft-apply-failed"
                      role="status"
                    >
                      <p className="font-medium">
                        {t("scan.modal.draftApplyFailedTitle")}
                      </p>
                      <p className="mt-0.5 text-amber-900/90">
                        {session.error_message ||
                          t("scan.modal.draftApplyFailedBody")}
                      </p>
                    </div>
                  ) : null}
                  <SpaceScanReview
                    layout={reviewLayout}
                    t={t}
                    isApplying={applyingReview}
                    onApply={handleApplyReview}
                    onOpenEditor={openEditorOnly}
                    onCancel={() => onOpenChange(false)}
                  />
                </div>
              ) : null}

              {phase === "error" ? (
                <div
                  className="flex flex-col gap-3 py-4"
                  data-testid="space-scan-error"
                >
                  {session ? (
                    <SpaceScanStatusBadge
                      status={session.status}
                      label={statusLabel(session.status)}
                    />
                  ) : null}
                  <p className="text-sm text-rose-800">
                    {error ||
                      session?.error_message ||
                      t("scan.modal.genericError")}
                  </p>
                  {session?.status === "failed" ? (
                    <Button
                      className="bg-brand text-white self-start"
                      startContent={<RefreshCw className="h-4 w-4" />}
                      isLoading={busy}
                      onPress={() => void handleRetry()}
                      data-testid="space-scan-retry"
                    >
                      {t("scan.retry")}
                    </Button>
                  ) : null}
                </div>
              ) : null}
            </ModalBody>
            <ModalFooter>
              {phase === "review" ? null : (
                <>
                  {isActiveScanStatus(session?.status) ? (
                    <Button
                      variant="light"
                      className="text-rose-700"
                      isLoading={busy}
                      onPress={() => void handleCancel()}
                      startContent={<X className="h-4 w-4" />}
                      data-testid="space-scan-cancel"
                    >
                      {t("scan.cancel")}
                    </Button>
                  ) : (
                    <Button
                      variant="light"
                      onPress={() => onOpenChange(false)}
                      data-testid="space-scan-close"
                    >
                      {t("scan.modal.close")}
                    </Button>
                  )}
                  {phase === "error" && !session ? (
                    <Button
                      className="bg-brand text-white"
                      onPress={() => void startSession()}
                      data-testid="space-scan-restart"
                    >
                      {t("scan.start")}
                    </Button>
                  ) : null}
                </>
              )}
            </ModalFooter>
          </>
        )}
      </ModalContent>
    </Modal>
  );
}
