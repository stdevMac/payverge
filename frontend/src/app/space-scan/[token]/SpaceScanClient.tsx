"use client";

import React, {
  useCallback,
  useEffect,
  useMemo,
  useRef,
  useState,
} from "react";
import Link from "next/link";
import { useSearchParams } from "next/navigation";
import { Button, Input, Progress, Spinner } from "@nextui-org/react";
import {
  AlertTriangle,
  Camera,
  Check,
  Pause,
  Play,
  Upload,
  X,
} from "lucide-react";
import {
  publicSpaceScanApi,
  sha256Hex,
  type LayoutDocument,
  type PublicScanSessionMeta,
  type ScanStatus,
} from "@/api/spaces";
import { getSessionInfo } from "@/api/auth/sessionInfo";
import {
  getApiErrorCode,
  getApiErrorStatus,
  getSafeApiErrorMessage,
} from "@/utils/apiError";
import {
  useSimpleLocale,
  getTranslation,
} from "@/i18n/SimpleTranslationProvider";
import SpaceScanStatusBadge from "@/components/business/spaces/scan/SpaceScanStatusBadge";
import SpaceScanReview from "@/components/business/spaces/scan/SpaceScanReview";
import {
  buildKeyframeUploadPayload,
  deriveCaptureGuidance,
  hasCameraApi,
  isTerminalScanStatus,
  type CapturedKeyframe,
  type CaptureGuidance,
} from "@/components/business/spaces/scan/scanStatus";

const TARGET_FRAMES = 24;
const CAPTURE_INTERVAL_MS = 450;

type ClientPhase =
  | "loading"
  | "auth"
  | "unsupported"
  | "instructions"
  | "scanning"
  | "uploading"
  | "processing"
  | "review"
  | "done"
  | "error";

export interface SpaceScanClientProps {
  token: string;
}

export default function SpaceScanClient({ token }: SpaceScanClientProps) {
  const { locale } = useSimpleLocale();
  const searchParams = useSearchParams();
  const pairFromUrl = searchParams?.get("pair") ?? "";

  const t = useCallback(
    (key: string, params?: Record<string, string | number>): string => {
      const result = getTranslation(`spacesTables.${key}`, locale, params);
      return Array.isArray(result) ? result[0] || key : (result as string);
    },
    [locale],
  );

  const [phase, setPhase] = useState<ClientPhase>("loading");
  const [meta, setMeta] = useState<PublicScanSessionMeta | null>(null);
  const [pairCode, setPairCode] = useState(pairFromUrl);
  const [authHint, setAuthHint] = useState<"staff" | "guest" | "unknown">(
    "unknown",
  );
  const [error, setError] = useState<string | null>(null);
  const [status, setStatus] = useState<ScanStatus | string>("waiting_for_phone");
  const [progressPct, setProgressPct] = useState(0);
  const [frames, setFrames] = useState<CapturedKeyframe[]>([]);
  const [paused, setPaused] = useState(false);
  const [guidance, setGuidance] = useState<CaptureGuidance>(null);
  const [uploadProgress, setUploadProgress] = useState(0);
  const [reviewLayout, setReviewLayout] = useState<LayoutDocument | null>(null);
  /** CAS base for public apply-review (from GET /result draft_revision). */
  const [draftRevision, setDraftRevision] = useState<number | null>(null);
  const [draftApplyFailed, setDraftApplyFailed] = useState(false);
  const [cancelConfirm, setCancelConfirm] = useState(false);
  const [connecting, setConnecting] = useState(false);
  const [dirtyCapture, setDirtyCapture] = useState(false);

  const videoRef = useRef<HTMLVideoElement | null>(null);
  const canvasRef = useRef<HTMLCanvasElement | null>(null);
  const streamRef = useRef<MediaStream | null>(null);
  const captureTimerRef = useRef<ReturnType<typeof setInterval> | null>(null);
  const orientationRef = useRef<{
    alpha: number | null;
    beta: number | null;
    gamma: number | null;
  }>({ alpha: null, beta: null, gamma: null });
  const framesRef = useRef<CapturedKeyframe[]>([]);
  const uploadingRef = useRef(false);

  const stopCamera = useCallback(() => {
    if (captureTimerRef.current) {
      clearInterval(captureTimerRef.current);
      captureTimerRef.current = null;
    }
    streamRef.current?.getTracks().forEach((tr) => tr.stop());
    streamRef.current = null;
    if (videoRef.current) {
      videoRef.current.srcObject = null;
    }
  }, []);

  // beforeunload when capture in progress
  useEffect(() => {
    if (!dirtyCapture) return;
    const handler = (e: BeforeUnloadEvent) => {
      e.preventDefault();
      e.returnValue = "";
    };
    window.addEventListener("beforeunload", handler);
    return () => window.removeEventListener("beforeunload", handler);
  }, [dirtyCapture]);

  useEffect(() => {
    return () => stopCamera();
  }, [stopCamera]);

  // Device orientation (optional metadata)
  useEffect(() => {
    const onOrient = (e: DeviceOrientationEvent) => {
      orientationRef.current = {
        alpha: e.alpha,
        beta: e.beta,
        gamma: e.gamma,
      };
    };
    window.addEventListener("deviceorientation", onOrient);
    return () => window.removeEventListener("deviceorientation", onOrient);
  }, []);

  const loadMeta = useCallback(async () => {
    setPhase("loading");
    setError(null);
    try {
      const m = await publicSpaceScanApi.getMeta(token);
      setMeta(m);
      setStatus(m.status);
      setProgressPct(m.progress_pct ?? 0);

      if (m.status === "review_ready" || m.status === "completed") {
        try {
          const result = await publicSpaceScanApi.getResult(token);
          setReviewLayout(
            (result.layout as LayoutDocument) ?? null,
          );
          if (typeof result.draft_revision === "number") {
            setDraftRevision(result.draft_revision);
          }
          setDraftApplyFailed(
            result.draft_apply_failed === true ||
              result.error_code === "draft_apply_failed",
          );
          setPhase("review");
          return;
        } catch {
          setPhase("done");
          return;
        }
      }

      if (isTerminalScanStatus(m.status)) {
        setPhase("error");
        setError(t(`scan.status.${m.status}`));
        return;
      }

      // Auth / compatibility
      let sessionAuth: "staff" | "guest" = "guest";
      try {
        const info = await getSessionInfo();
        if (info.authenticated && (info.type === "staff" || info.type === "user" || info.type === "web3")) {
          sessionAuth = "staff";
        }
      } catch {
        sessionAuth = "guest";
      }
      setAuthHint(sessionAuth);

      if (!hasCameraApi()) {
        setPhase("unsupported");
        return;
      }

      // Logged-in staff/owner: try connect with session cookies (no pair code).
      // Falls back to the pair-code auth screen on failure.
      if (sessionAuth === "staff") {
        try {
          const res = await publicSpaceScanApi.connect(token, {
            device_meta: {
              user_agent:
                typeof navigator !== "undefined" ? navigator.userAgent : "",
              platform:
                typeof navigator !== "undefined" ? navigator.platform : "",
              capture_mode: "web_keyframes",
              webxr: false,
              auth_path: "session",
            },
          });
          setStatus(res.status);
          setProgressPct(res.progress_pct);
          setPhase("instructions");
          return;
        } catch {
          // Pair code still required when JWT lacks business access.
        }
      }

      setPhase("auth");
    } catch (err) {
      const code = getApiErrorCode(err);
      const status = getApiErrorStatus(err);
      if (code === "not_found" || status === 404) {
        setError(t("scan.mobile.sessionNotFound"));
      } else {
        setError(t("scan.mobile.metaError"));
      }
      setPhase("error");
    }
  }, [token, t]);

  useEffect(() => {
    void loadMeta();
  }, [loadMeta]);

  useEffect(() => {
    if (typeof document === "undefined") return;
    document.title = t("scan.mobile.docTitle");
  }, [t]);

  // Poll status while processing / after upload
  useEffect(() => {
    if (phase !== "processing" && phase !== "uploading") return;
    const id = window.setInterval(async () => {
      try {
        const m = await publicSpaceScanApi.getMeta(token);
        setStatus(m.status);
        setProgressPct(m.progress_pct ?? 0);
        if (m.status === "review_ready" || m.status === "completed") {
          try {
            const result = await publicSpaceScanApi.getResult(token);
            setReviewLayout((result.layout as LayoutDocument) ?? null);
            if (typeof result.draft_revision === "number") {
              setDraftRevision(result.draft_revision);
            }
            setDraftApplyFailed(
              result.draft_apply_failed === true ||
                result.error_code === "draft_apply_failed",
            );
          } catch {
            // Token may be revoked after process; draft still exists server-side.
          }
          setPhase("review");
          setDirtyCapture(false);
        } else if (m.status === "failed" || m.status === "expired" || m.status === "cancelled") {
          setPhase("error");
          setError(t(`scan.status.${m.status}`));
          setDirtyCapture(false);
        }
      } catch {
        // keep polling
      }
    }, 2000);
    return () => window.clearInterval(id);
  }, [phase, token, t]);

  const connect = useCallback(async () => {
    setConnecting(true);
    setError(null);
    try {
      const res = await publicSpaceScanApi.connect(token, {
        pair_code: pairCode.trim() || undefined,
        device_meta: {
          user_agent:
            typeof navigator !== "undefined" ? navigator.userAgent : "",
          platform:
            typeof navigator !== "undefined" ? navigator.platform : "",
          capture_mode: "web_keyframes",
          webxr: false,
        },
      });
      setStatus(res.status);
      setProgressPct(res.progress_pct);
      setPhase("instructions");
    } catch (err) {
      setError(getSafeApiErrorMessage(err, t("scan.mobile.connectError")));
    } finally {
      setConnecting(false);
    }
  }, [token, pairCode, t]);

  const sampleLuma = (ctx: CanvasRenderingContext2D, w: number, h: number) => {
    try {
      const data = ctx.getImageData(0, 0, Math.min(w, 64), Math.min(h, 64)).data;
      let sum = 0;
      let n = 0;
      for (let i = 0; i < data.length; i += 16) {
        sum += 0.2126 * data[i] + 0.7152 * data[i + 1] + 0.0722 * data[i + 2];
        n += 1;
      }
      return n ? sum / n : 128;
    } catch {
      return 128;
    }
  };

  const captureFrame = useCallback(() => {
    const video = videoRef.current;
    const canvas = canvasRef.current;
    if (!video || !canvas || video.readyState < 2) return;
    if (framesRef.current.length >= TARGET_FRAMES) return;

    const w = video.videoWidth || 640;
    const h = video.videoHeight || 480;
    canvas.width = Math.min(w, 640);
    canvas.height = Math.min(h, 480);
    const ctx = canvas.getContext("2d");
    if (!ctx) return;
    ctx.drawImage(video, 0, 0, canvas.width, canvas.height);
    const luma = sampleLuma(ctx, canvas.width, canvas.height);
    const index = framesRef.current.length;
    const yaw =
      orientationRef.current.alpha != null
        ? orientationRef.current.alpha
        : undefined;

    const frame: CapturedKeyframe = {
      index,
      metric: false,
      pose_position: {
        // Approximate walk path — not metric AR; processor marks approximate.
        x: Math.cos((index / TARGET_FRAMES) * Math.PI * 2) * (index * 0.15),
        y: Math.sin((index / TARGET_FRAMES) * Math.PI * 2) * (index * 0.15),
      },
      pose_yaw_deg: yaw,
      orientation: { ...orientationRef.current },
      timestamp: Date.now(),
      luma,
    };
    framesRef.current = [...framesRef.current, frame];
    setFrames(framesRef.current);
    setGuidance(
      deriveCaptureGuidance(framesRef.current, TARGET_FRAMES, false),
    );
    setDirtyCapture(true);

    void publicSpaceScanApi
      .updateStatus(token, {
        status: "scanning",
        progress_pct: Math.min(
          40,
          Math.round((framesRef.current.length / TARGET_FRAMES) * 40),
        ),
        progress_message: `captured ${framesRef.current.length}/${TARGET_FRAMES}`,
      })
      .catch(() => undefined);
  }, [token]);

  const pausedRef = useRef(paused);
  useEffect(() => {
    pausedRef.current = paused;
    setGuidance(
      deriveCaptureGuidance(framesRef.current, TARGET_FRAMES, paused),
    );
  }, [paused]);

  const startScanning = useCallback(async () => {
    setError(null);
    try {
      const stream = await navigator.mediaDevices.getUserMedia({
        video: {
          facingMode: { ideal: "environment" },
          width: { ideal: 1280 },
          height: { ideal: 720 },
        },
        audio: false,
      });
      streamRef.current = stream;
      if (videoRef.current) {
        videoRef.current.srcObject = stream;
        await videoRef.current.play();
      }
      framesRef.current = [];
      setFrames([]);
      setPaused(false);
      pausedRef.current = false;
      setPhase("scanning");
      setStatus("scanning");
      setDirtyCapture(true);
      await publicSpaceScanApi
        .updateStatus(token, {
          status: "scanning",
          progress_pct: 10,
          progress_message: "scanning",
        })
        .catch(() => undefined);
    } catch (err) {
      setError(getSafeApiErrorMessage(err, t("scan.mobile.cameraError")));
      setPhase("unsupported");
    }
  }, [token, t]);

  // Capture loop — only while scanning; pause via pausedRef.
  useEffect(() => {
    if (phase !== "scanning") return;
    if (captureTimerRef.current) {
      clearInterval(captureTimerRef.current);
    }
    captureTimerRef.current = setInterval(() => {
      if (pausedRef.current) return;
      if (framesRef.current.length >= TARGET_FRAMES) {
        if (captureTimerRef.current) {
          clearInterval(captureTimerRef.current);
          captureTimerRef.current = null;
        }
        setGuidance("quality_sufficient");
        return;
      }
      captureFrame();
    }, CAPTURE_INTERVAL_MS);
    return () => {
      if (captureTimerRef.current) {
        clearInterval(captureTimerRef.current);
        captureTimerRef.current = null;
      }
    };
  }, [phase, captureFrame]);

  const uploadAndProcess = useCallback(async () => {
    if (uploadingRef.current) return;
    if (framesRef.current.length < 4) {
      setError(t("scan.mobile.needMoreFrames"));
      return;
    }
    uploadingRef.current = true;
    setPhase("uploading");
    setUploadProgress(10);
    setStatus("uploading");
    stopCamera();

    try {
      await publicSpaceScanApi.updateStatus(token, {
        status: "uploading",
        progress_pct: 45,
        progress_message: "uploading keyframes",
      });

      const payload = buildKeyframeUploadPayload(framesRef.current, {
        // Honest defaults — approximate rectangular shell for local/dev path.
        room_width_mm: 8000,
        room_height_mm: 6000,
        notes: [
          `frames=${framesRef.current.length}`,
          "no WebXR / RoomPlan on this capture path",
        ],
      });
      const bodyStr = JSON.stringify(payload);
      const checksum = await sha256Hex(bodyStr);
      const idempotencyKey = `keyframes-${token.slice(0, 12)}-${checksum.slice(0, 16)}`;

      setUploadProgress(40);
      await publicSpaceScanApi.upload(token, {
        upload_kind: "keyframes",
        content_type: "application/json",
        checksum_sha256: checksum,
        payload,
        idempotency_key: idempotencyKey,
      });
      setUploadProgress(75);

      const complete = await publicSpaceScanApi.completeUpload(token);
      setStatus(complete.status);
      setUploadProgress(100);
      setPhase("processing");
      setProgressPct(55);
      setDirtyCapture(false);
    } catch (err) {
      setError(getSafeApiErrorMessage(err, t("scan.mobile.uploadError")));
      setPhase("error");
      uploadingRef.current = false;
    }
  }, [token, stopCamera, t]);

  const statusLabel = (s: string) => {
    const translated = t(`scan.status.${s}`);
    return translated.includes("scan.status.") ? s : translated;
  };

  const guidanceText = useMemo(() => {
    if (!guidance) return null;
    return t(`scan.guidance.${guidance}`);
  }, [guidance, t]);

  const handleCancelConfirmed = () => {
    stopCamera();
    setDirtyCapture(false);
    setCancelConfirm(false);
    setPhase("auth");
    setFrames([]);
    framesRef.current = [];
  };

  return (
    <div
      className="mx-auto flex min-h-[100dvh] max-w-lg flex-col bg-warm-50 px-4 py-6 text-ink-900"
      data-testid="space-scan-client"
    >
      <header className="mb-4">
        <p className="text-xs font-medium uppercase tracking-wider text-ink-500">
          Payverge
        </p>
        <h1 className="font-title text-2xl text-ink-900">
          {t("scan.mobile.title")}
        </h1>
        {meta ? (
          <p className="mt-1 text-sm text-ink-600">
            {meta.space_name
              ? t("scan.mobile.spaceLabel", { name: meta.space_name })
              : meta.business_name}
          </p>
        ) : null}
        {status ? (
          <div className="mt-2">
            <SpaceScanStatusBadge
              status={status}
              label={statusLabel(String(status))}
              progressPct={progressPct}
            />
          </div>
        ) : null}
      </header>

      <main className="flex flex-1 flex-col gap-4">
        {phase === "loading" ? (
          <div
            className="flex flex-1 flex-col items-center justify-center gap-3"
            data-testid="space-scan-mobile-loading"
          >
            <Spinner />
            <p className="text-sm text-ink-500">{t("scan.mobile.loading")}</p>
          </div>
        ) : null}

        {phase === "unsupported" ? (
          <div
            className="flex flex-col gap-4 rounded-2xl border border-warm-200 bg-white p-4"
            data-testid="space-scan-unsupported"
          >
            <div className="flex items-start gap-2 text-amber-900">
              <AlertTriangle className="mt-0.5 h-5 w-5 shrink-0" />
              <div>
                <p className="font-medium">{t("scan.mobile.unsupportedTitle")}</p>
                <p className="mt-1 text-sm text-ink-600">
                  {t("scan.mobile.unsupportedBody")}
                </p>
              </div>
            </div>
            <Button
              as={Link}
              href="/dashboard"
              className="bg-brand text-white font-medium"
              data-testid="space-scan-manual-editor-link"
            >
              {t("scan.mobile.openManualEditor")}
            </Button>
            {/* /dashboard is the real operator hub (lists businesses → Tables → Spaces). */}
            <p className="text-xs text-ink-400">
              {t("scan.mobile.loginForEditor")}
            </p>
          </div>
        ) : null}

        {phase === "auth" ? (
          <div
            className="flex flex-col gap-4 rounded-2xl border border-warm-200 bg-white p-4"
            data-testid="space-scan-auth"
          >
            {authHint === "staff" ? (
              <p className="text-sm text-ink-700">{t("scan.mobile.staffContinue")}</p>
            ) : (
              <div className="space-y-2 text-sm text-ink-700">
                <p>{t("scan.mobile.pairInstructions")}</p>
                <p className="text-ink-500">{t("scan.mobile.noTokenAccess")}</p>
                <Link
                  href="/staff/login"
                  className="inline-flex text-sm font-medium text-brand underline"
                >
                  {t("scan.mobile.staffLogin")}
                </Link>
              </div>
            )}
            <Input
              label={t("scan.mobile.pairCodeLabel")}
              placeholder="123456"
              value={pairCode}
              onValueChange={setPairCode}
              inputMode="numeric"
              maxLength={6}
              autoComplete="one-time-code"
              data-testid="space-scan-pair-input"
            />
            {error ? (
              <p className="text-sm text-rose-700" role="alert">
                {error}
              </p>
            ) : null}
            <Button
              className="bg-brand text-white font-medium"
              isLoading={connecting}
              onPress={() => void connect()}
              data-testid="space-scan-connect"
            >
              {t("scan.mobile.connect")}
            </Button>
          </div>
        ) : null}

        {phase === "instructions" ? (
          <div
            className="flex flex-col gap-4 rounded-2xl border border-warm-200 bg-white p-4"
            data-testid="space-scan-instructions"
          >
            <p className="text-sm text-ink-700">{t("scan.mobile.instructionsBody")}</p>
            <ul className="list-disc space-y-1 pl-5 text-sm text-ink-600">
              <li>{t("scan.mobile.tipSlow")}</li>
              <li>{t("scan.mobile.tipLight")}</li>
              <li>{t("scan.mobile.tipCorners")}</li>
              <li>{t("scan.mobile.tipApproximate")}</li>
            </ul>
            <Button
              className="bg-brand text-white font-medium"
              startContent={<Camera className="h-4 w-4" />}
              onPress={() => void startScanning()}
              data-testid="space-scan-start-capture"
            >
              {t("scan.mobile.startCapture")}
            </Button>
          </div>
        ) : null}

        {phase === "scanning" ? (
          <div className="flex flex-col gap-3" data-testid="space-scan-capture">
            <div className="relative overflow-hidden rounded-2xl border border-warm-300 bg-ink-900 aspect-[3/4]">
              <video
                ref={videoRef}
                className="h-full w-full object-cover"
                playsInline
                muted
                autoPlay
              />
              <canvas ref={canvasRef} className="hidden" />
              <div className="pointer-events-none absolute inset-x-0 bottom-0 bg-gradient-to-t from-ink-900/80 to-transparent p-3">
                <p className="text-center text-sm font-medium text-white">
                  {guidanceText}
                </p>
                <p className="mt-1 text-center text-xs text-white/80">
                  {t("scan.mobile.frameCount", {
                    n: frames.length,
                    total: TARGET_FRAMES,
                  })}
                </p>
              </div>
            </div>
            <Progress
              aria-label={t("scan.mobile.captureProgress")}
              value={(frames.length / TARGET_FRAMES) * 100}
              className="max-w-full"
              classNames={{ indicator: "bg-brand" }}
            />
            <div className="flex flex-wrap gap-2">
              <Button
                variant="bordered"
                className="border-warm-300"
                startContent={
                  paused ? (
                    <Play className="h-4 w-4" />
                  ) : (
                    <Pause className="h-4 w-4" />
                  )
                }
                onPress={() => setPaused((p) => !p)}
                data-testid="space-scan-pause"
              >
                {paused ? t("scan.mobile.resume") : t("scan.mobile.pause")}
              </Button>
              <Button
                className="bg-brand text-white font-medium"
                isDisabled={frames.length < 4}
                startContent={<Upload className="h-4 w-4" />}
                onPress={() => void uploadAndProcess()}
                data-testid="space-scan-finish-upload"
              >
                {t("scan.mobile.finishUpload")}
              </Button>
              <Button
                variant="light"
                className="text-rose-700"
                startContent={<X className="h-4 w-4" />}
                onPress={() => setCancelConfirm(true)}
                data-testid="space-scan-cancel-capture"
              >
                {t("scan.cancel")}
              </Button>
            </div>
            {cancelConfirm ? (
              <div
                className="rounded-xl border border-rose-200 bg-rose-50 p-3 text-sm"
                data-testid="space-scan-cancel-confirm"
              >
                <p className="text-rose-900">{t("scan.mobile.cancelConfirm")}</p>
                <div className="mt-2 flex gap-2">
                  <Button size="sm" variant="light" onPress={() => setCancelConfirm(false)}>
                    {t("scan.mobile.keepScanning")}
                  </Button>
                  <Button
                    size="sm"
                    className="bg-rose-600 text-white"
                    onPress={handleCancelConfirmed}
                  >
                    {t("scan.mobile.confirmCancel")}
                  </Button>
                </div>
              </div>
            ) : null}
            {error ? (
              <p className="text-sm text-rose-700" role="alert">
                {error}
              </p>
            ) : null}
          </div>
        ) : null}

        {phase === "uploading" || phase === "processing" ? (
          <div
            className="flex flex-col items-center gap-3 rounded-2xl border border-warm-200 bg-white p-6"
            data-testid="space-scan-processing"
          >
            <Spinner />
            <p className="text-sm font-medium text-ink-800">
              {phase === "uploading"
                ? t("scan.mobile.uploading")
                : t("scan.mobile.processing")}
            </p>
            <Progress
              aria-label={t("scan.progress", {
                pct: phase === "uploading" ? uploadProgress : progressPct,
              })}
              value={phase === "uploading" ? uploadProgress : progressPct}
              className="w-full"
              classNames={{ indicator: "bg-brand" }}
            />
            <p className="text-xs text-ink-500">{t("scan.mobile.realProcessing")}</p>
          </div>
        ) : null}

        {phase === "review" ? (
          <SpaceScanReview
            layout={reviewLayout}
            t={t}
            onApply={async (tables, layout) => {
              // Persist filtered layout via public apply-review (not local-only).
              try {
                setError(null);
                if (draftApplyFailed) {
                  // Operator draft exists server-side; public replace is refused.
                  // Complete is not attempted — open operator dashboard/editor.
                  setError(t("scan.mobile.draftHasContent"));
                  return;
                }
                let rev = draftRevision;
                if (rev == null || rev <= 0) {
                  const fresh = await publicSpaceScanApi.getResult(token);
                  rev =
                    typeof fresh.draft_revision === "number"
                      ? fresh.draft_revision
                      : 0;
                  if (fresh.draft_apply_failed || fresh.error_code === "draft_apply_failed") {
                    setDraftApplyFailed(true);
                    setError(t("scan.mobile.draftHasContent"));
                    return;
                  }
                  setDraftRevision(rev);
                }
                if (rev <= 0) {
                  setError(t("scan.toasts.reviewError"));
                  return;
                }
                await publicSpaceScanApi.applyReview(
                  token,
                  { ...layout, tables },
                  rev,
                );
                setReviewLayout({ ...layout, tables });
                setPhase("done");
              } catch (err) {
                setError(
                  getSafeApiErrorMessage(err, t("scan.toasts.reviewError")),
                );
              }
            }}
            onOpenEditor={() => {
              // Operator hub: pick business → Tables → Spaces & Tables editor.
              // eslint-disable-next-line @next/next/no-location-assign-relative-destination -- full reload into the operator dashboard from the guest scan surface
              window.location.href = "/dashboard";
            }}
          />
        ) : null}

        {phase === "done" ? (
          <div
            className="flex flex-col gap-3 rounded-2xl border border-emerald-200 bg-emerald-50 p-4"
            data-testid="space-scan-done"
          >
            <div className="flex items-center gap-2 text-emerald-900">
              <Check className="h-5 w-5" />
              <p className="font-medium">{t("scan.mobile.doneTitle")}</p>
            </div>
            <p className="text-sm text-ink-700">{t("scan.mobile.doneBody")}</p>
            <Button
              as={Link}
              href="/dashboard"
              className="bg-brand text-white font-medium"
              data-testid="space-scan-done-dashboard-link"
            >
              {t("scan.mobile.backToDashboard")}
            </Button>
          </div>
        ) : null}

        {phase === "error" ? (
          <div
            className="flex flex-col gap-3 rounded-2xl border border-rose-200 bg-rose-50 p-4"
            data-testid="space-scan-mobile-error"
          >
            <p className="text-sm text-rose-900" role="alert">
              {error || t("scan.modal.genericError")}
            </p>
            <div className="flex flex-wrap gap-2">
              <Button
                variant="bordered"
                className="border-warm-300"
                onPress={() => {
                  uploadingRef.current = false;
                  void loadMeta();
                }}
              >
                {t("scan.mobile.retry")}
              </Button>
              {framesRef.current.length >= 4 ? (
                <Button
                  className="bg-brand text-white"
                  onPress={() => {
                    uploadingRef.current = false;
                    void uploadAndProcess();
                  }}
                  data-testid="space-scan-retry-upload"
                >
                  {t("scan.mobile.retryUpload")}
                </Button>
              ) : null}
            </div>
          </div>
        ) : null}
      </main>
    </div>
  );
}
