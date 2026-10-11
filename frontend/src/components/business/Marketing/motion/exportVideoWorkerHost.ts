/**
 * Main-thread host for off-main-thread motion export.
 *
 * Decodes images here (workers have no `new Image()`), transfers `ImageBitmap`s
 * into `videoWorker`, and renders the poster on this thread (`renderPostToBlob`
 * needs `document.createElement("canvas")`). Progress and cancel flow through
 * the pure `workerProtocol` message types.
 */

import {
  loadPostImages,
  renderPostToBlob,
  type RenderPostInput,
} from "../templates/renderPost";
import { detectMotionSupport } from "./capabilities";
import {
  exportVideo,
  type ExportVideoInput,
  type ExportVideoResult,
} from "./exportVideo";
import {
  isWorkerOutMessage,
  type WorkerInMessage,
  type WorkerStartMessage,
} from "./workerProtocol";

async function toBitmap(
  image: HTMLImageElement | null,
): Promise<ImageBitmap | null> {
  if (!image) return null;
  if (typeof createImageBitmap !== "function") return null;
  try {
    return await createImageBitmap(image);
  } catch {
    return null;
  }
}

function canSpawnWorker(): boolean {
  return (
    typeof Worker !== "undefined" &&
    typeof URL !== "undefined" &&
    typeof createImageBitmap === "function"
  );
}

/**
 * Run encode in a dedicated worker when the environment supports it; otherwise
 * fall back to the main-thread `exportVideo` path (OffscreenCanvas / WebCodecs
 * or MediaRecorder).
 *
 * Tests should keep injecting deps into `exportVideo` directly — this host is
 * the production download path and is intentionally not unit-tested under jsdom.
 */
export async function exportVideoPreferWorker(
  input: ExportVideoInput,
): Promise<ExportVideoResult> {
  // Injected deps mean a test (or special caller) owns the pipeline — stay on
  // the direct path so fakes are not lost inside the worker.
  if (input.deps || !canSpawnWorker()) {
    return exportVideo(input);
  }

  // MediaRecorder needs HTMLCanvasElement.captureStream (document canvas) and
  // cannot share the OffscreenCanvas worker path. Only WebCodecs goes off-thread.
  const support = detectMotionSupport();
  if (support.path !== "webcodecs") {
    return exportVideo(input);
  }

  try {
    return await exportVideoInWorker(input);
  } catch {
    // Worker construction / transfer can fail on odd embeds; degrade silently.
    return exportVideo(input);
  }
}

export async function exportVideoInWorker(
  input: ExportVideoInput,
): Promise<ExportVideoResult> {
  if (!canSpawnWorker()) {
    throw new Error("worker_unavailable");
  }

  const loaded = await loadPostImages(input.renderInput, input.signal);
  const [photo, logo, poster] = await Promise.all([
    toBitmap(loaded.photo),
    toBitmap(loaded.logo),
    renderPostToBlob(
      input.renderInput,
      input.signal ? { signal: input.signal } : undefined,
    ),
  ]);

  // If bitmap conversion failed but we still have images, fall back — the
  // worker cannot decode via `new Image()`.
  if ((loaded.photo && !photo) || (loaded.logo && !logo)) {
    photo?.close();
    logo?.close();
    return exportVideo(input);
  }

  const worker = new Worker(new URL("./videoWorker.ts", import.meta.url));

  const start: WorkerStartMessage = {
    type: "start",
    renderInput: input.renderInput,
    preset: input.preset,
    ...(input.durationMs !== undefined ? { durationMs: input.durationMs } : {}),
    ...(input.fps !== undefined ? { fps: input.fps } : {}),
    photo,
    logo,
  };

  const transfer: Transferable[] = [];
  if (photo) transfer.push(photo);
  if (logo) transfer.push(logo);

  const video = await new Promise<Blob>((resolve, reject) => {
    const onAbort = () => {
      postToWorker(worker, { type: "cancel" });
      worker.terminate();
      reject(new DOMException("Aborted", "AbortError"));
    };

    if (input.signal) {
      if (input.signal.aborted) {
        onAbort();
        return;
      }
      input.signal.addEventListener("abort", onAbort, { once: true });
    }

    worker.onmessage = (event: MessageEvent<unknown>) => {
      const msg = event.data;
      if (!isWorkerOutMessage(msg)) return;
      if (msg.type === "progress") {
        input.onProgress?.(msg.fraction);
        return;
      }
      if (msg.type === "done") {
        input.signal?.removeEventListener("abort", onAbort);
        worker.terminate();
        resolve(new Blob([msg.video], { type: msg.videoMime }));
        return;
      }
      if (msg.type === "error") {
        input.signal?.removeEventListener("abort", onAbort);
        worker.terminate();
        reject(new Error(msg.message));
      }
    };

    worker.onerror = (event) => {
      input.signal?.removeEventListener("abort", onAbort);
      worker.terminate();
      reject(event.error ?? new Error(event.message || "worker_error"));
    };

    worker.postMessage(start, transfer);
  });

  return { video, poster };
}

function postToWorker(worker: Worker, msg: WorkerInMessage): void {
  worker.postMessage(msg);
}

/** @internal test seam — re-export type for callers that build start messages. */
export type { RenderPostInput };
