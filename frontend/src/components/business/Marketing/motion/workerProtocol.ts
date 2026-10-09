/**
 * Pure message contract between the main-thread export host and `videoWorker`.
 *
 * No Worker APIs, no canvas, no codec — only JSON-safe shapes and type guards.
 * That is what lets the protocol be unit-tested under jsdom while the worker
 * entry itself stays an untestable thin postMessage shell.
 */

import type { MotionPresetId } from "./presets";
import type { RenderPostInput } from "../templates/renderPost";

/** Main → worker: start an encode with bitmaps already decoded on main. */
export interface WorkerStartMessage {
  type: "start";
  renderInput: RenderPostInput;
  preset: MotionPresetId;
  durationMs?: number;
  fps?: number;
  /**
   * Transferred `ImageBitmap`s (or null when the post has no photo/logo).
   * Structured-clone + transfer list; not JSON-serialised.
   */
  photo: ImageBitmap | null;
  logo: ImageBitmap | null;
}

/** Main → worker: abandon the in-flight encode. */
export interface WorkerCancelMessage {
  type: "cancel";
}

export type WorkerInMessage = WorkerStartMessage | WorkerCancelMessage;

/** Worker → main: one progress tick, 0..1. */
export interface WorkerProgressMessage {
  type: "progress";
  fraction: number;
}

/** Worker → main: finished encode. Poster is produced on the main thread. */
export interface WorkerDoneMessage {
  type: "done";
  /** MP4 (or WebM on MediaRecorder fallback) bytes. */
  video: ArrayBuffer;
  videoMime: string;
}

/** Worker → main: encode failed. */
export interface WorkerErrorMessage {
  type: "error";
  message: string;
  code?: string;
}

export type WorkerOutMessage =
  | WorkerProgressMessage
  | WorkerDoneMessage
  | WorkerErrorMessage;

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null;
}

export function isWorkerInMessage(value: unknown): value is WorkerInMessage {
  if (!isRecord(value) || typeof value.type !== "string") return false;
  if (value.type === "cancel") return true;
  if (value.type !== "start") return false;
  return (
    isRecord(value.renderInput) &&
    typeof value.preset === "string" &&
    (value.photo === null ||
      (typeof ImageBitmap !== "undefined" && value.photo instanceof ImageBitmap) ||
      // Transfer deserialization under some test doubles may yield a plain object
      // with width/height; accept that shape for the pure guard path.
      isRecord(value.photo)) &&
    (value.logo === null ||
      (typeof ImageBitmap !== "undefined" && value.logo instanceof ImageBitmap) ||
      isRecord(value.logo))
  );
}

export function isWorkerOutMessage(value: unknown): value is WorkerOutMessage {
  if (!isRecord(value) || typeof value.type !== "string") return false;
  switch (value.type) {
    case "progress":
      return typeof value.fraction === "number";
    case "done":
      return (
        value.video instanceof ArrayBuffer && typeof value.videoMime === "string"
      );
    case "error":
      return typeof value.message === "string";
    default:
      return false;
  }
}

export function isWorkerStartMessage(
  value: unknown,
): value is WorkerStartMessage {
  return isWorkerInMessage(value) && value.type === "start";
}

export function isWorkerCancelMessage(
  value: unknown,
): value is WorkerCancelMessage {
  return isWorkerInMessage(value) && value.type === "cancel";
}
