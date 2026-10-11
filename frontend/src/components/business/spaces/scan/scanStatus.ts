import type { ScanStatus } from "@/api/spaces";

/** Canonical status order for UI (non-terminal first, then outcomes). */
export const SCAN_STATUSES: ScanStatus[] = [
  "waiting_for_phone",
  "phone_connected",
  "scanning",
  "uploading",
  "processing",
  "review_ready",
  "failed",
  "expired",
  "cancelled",
  "completed",
];

const ACTIVE_SCAN_STATUSES = new Set<string>([
  "waiting_for_phone",
  "phone_connected",
  "scanning",
  "uploading",
  "processing",
]);

const TERMINAL_SCAN_STATUSES = new Set<string>([
  "review_ready",
  "failed",
  "expired",
  "cancelled",
  "completed",
]);

export function isActiveScanStatus(status: string | null | undefined): boolean {
  return Boolean(status && ACTIVE_SCAN_STATUSES.has(status));
}

export function isTerminalScanStatus(
  status: string | null | undefined,
): boolean {
  return Boolean(status && TERMINAL_SCAN_STATUSES.has(status));
}

export function scanStatusTone(status: string): string {
  switch (status) {
    case "review_ready":
    case "completed":
      return "bg-emerald-50 text-emerald-800 border-emerald-200";
    case "failed":
    case "expired":
    case "cancelled":
      return "bg-rose-50 text-rose-800 border-rose-200";
    case "processing":
    case "uploading":
    case "scanning":
      return "bg-amber-50 text-amber-900 border-amber-200";
    case "phone_connected":
      return "bg-brand/10 text-brand border-brand/30";
    default:
      return "bg-warm-100 text-ink-600 border-warm-200";
  }
}

/** Coarse mobile UA check (phone/tablet). */
export function isMobileUserAgent(
  ua: string = typeof navigator !== "undefined" ? navigator.userAgent : "",
): boolean {
  return /Android|webOS|iPhone|iPad|iPod|BlackBerry|IEMobile|Opera Mini|Mobile/i.test(
    ua,
  );
}

/** True when getUserMedia is available (HTTPS or localhost). */
export function hasCameraApi(): boolean {
  return (
    typeof navigator !== "undefined" &&
    !!navigator.mediaDevices &&
    typeof navigator.mediaDevices.getUserMedia === "function"
  );
}

export type CaptureGuidance =
  | "move_slower"
  | "low_light"
  | "move_closer"
  | "area_not_captured"
  | "tracking_lost"
  | "quality_sufficient"
  | "hold_steady"
  | null;

export interface CapturedKeyframe {
  index: number;
  /** JPEG data URL (stripped before upload). */
  dataUrl?: string;
  pose_position?: { x: number; y: number };
  pose_yaw_deg?: number;
  metric: boolean;
  /** Device orientation snapshot when available. */
  orientation?: {
    alpha: number | null;
    beta: number | null;
    gamma: number | null;
  };
  timestamp: number;
  /** Average luma 0–255 for low-light hints. */
  luma?: number;
}

/** Build a keyframe JSON payload the backend KeyframeProcessor accepts. */
export function buildKeyframeUploadPayload(
  frames: CapturedKeyframe[],
  opts?: {
    room_width_mm?: number;
    room_height_mm?: number;
    calibration_mm?: number;
    notes?: string[];
  },
): Record<string, unknown> {
  return {
    frames: frames.map((f) => ({
      index: f.index,
      pose_position: f.pose_position ?? { x: f.index * 0.4, y: 0 },
      pose_yaw_deg: f.pose_yaw_deg,
      metric: f.metric,
      // Do not upload image blobs — metadata poses only (approximate path).
    })),
    room_width_mm: opts?.room_width_mm,
    room_height_mm: opts?.room_height_mm,
    calibration_mm: opts?.calibration_mm,
    notes: [
      "web keyframe capture (approximate — not RoomPlan / ARKit)",
      ...(opts?.notes ?? []),
    ],
    metric: false,
  };
}

/** Derive a single guidance message from recent frames. Honest, no AR claims. */
export function deriveCaptureGuidance(
  frames: CapturedKeyframe[],
  targetCount: number,
  paused: boolean,
): CaptureGuidance {
  if (paused) return "hold_steady";
  if (frames.length >= targetCount) return "quality_sufficient";
  if (frames.length === 0) return "move_closer";

  const last = frames[frames.length - 1];
  const prev = frames.length > 1 ? frames[frames.length - 2] : null;

  if (last.luma != null && last.luma < 40) return "low_light";

  if (prev && last.timestamp - prev.timestamp < 180) {
    return "move_slower";
  }

  if (
    prev?.orientation &&
    last.orientation &&
    prev.orientation.alpha != null &&
    last.orientation.alpha != null
  ) {
    const d = Math.abs(last.orientation.alpha - prev.orientation.alpha);
    const jump = Math.min(d, 360 - d);
    if (jump > 45) return "tracking_lost";
  }

  if (frames.length < Math.ceil(targetCount * 0.35)) {
    return "area_not_captured";
  }

  return "hold_steady";
}
