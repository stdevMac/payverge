/**
 * Dedicated worker entry for motion export.
 *
 * Deliberately thin: decode the protocol message, call `exportVideo` with an
 * `OffscreenCanvas` surface and transferred `ImageBitmap`s, post progress and
 * the finished video buffer. No logic that is not already tested in
 * `workerProtocol.ts` or elsewhere — jsdom cannot run this file.
 *
 * Bundled via `new Worker(new URL("./videoWorker.ts", import.meta.url))` from
 * the main-thread host in `exportVideoWorkerHost.ts`.
 */

import { renderScene, toSceneImage, type SceneImages } from "../scene/renderScene";
import type { LoadedPostImages } from "../templates/renderPost";
import { exportVideo, type ExportVideoDeps } from "./exportVideo";
import {
  isWorkerCancelMessage,
  isWorkerStartMessage,
  type WorkerOutMessage,
  type WorkerStartMessage,
} from "./workerProtocol";

/**
 * Minimal worker global shape. Avoids depending on the DOM lib's
 * `DedicatedWorkerGlobalScope` (not always in the TS project lib list).
 */
interface VideoWorkerScope {
  postMessage(message: unknown, transfer?: Transferable[]): void;
  onmessage: ((event: MessageEvent<unknown>) => void) | null;
}

const workerScope = self as unknown as VideoWorkerScope;

let cancelled = false;

function post(msg: WorkerOutMessage, transfer?: Transferable[]): void {
  if (transfer && transfer.length > 0) {
    workerScope.postMessage(msg, transfer);
  } else {
    workerScope.postMessage(msg);
  }
}

/**
 * Duck-typed loaded images: buildPostScene only needs naturalWidth/Height for
 * crop math; paint uses structural SceneImages built from the real bitmaps.
 */
function loadedFromBitmaps(msg: WorkerStartMessage): LoadedPostImages {
  const photo = msg.photo
    ? ({
        naturalWidth: msg.photo.width,
        naturalHeight: msg.photo.height,
      } as HTMLImageElement)
    : null;
  const logo = msg.logo
    ? ({
        naturalWidth: msg.logo.width,
        naturalHeight: msg.logo.height,
      } as HTMLImageElement)
    : null;
  return { photo, logo };
}

function sceneImagesFromBitmaps(msg: WorkerStartMessage): SceneImages {
  const out: SceneImages = {};
  const photoUrl = msg.photo ? msg.renderInput.photoUrl : "";
  const logoUrl = msg.logo ? (msg.renderInput.logoUrl ?? "") : "";
  if (msg.photo && photoUrl) out[photoUrl] = toSceneImage(msg.photo);
  if (msg.logo && logoUrl) out[logoUrl] = toSceneImage(msg.logo);
  return out;
}

async function runStart(msg: WorkerStartMessage): Promise<void> {
  cancelled = false;
  const loaded = loadedFromBitmaps(msg);
  const sceneImages = sceneImagesFromBitmaps(msg);

  const deps: Partial<ExportVideoDeps> = {
    loadImages: async () => loaded,
    // Poster is rendered on the main thread (document canvas); worker only encodes video.
    renderPoster: async () => new Blob([], { type: "image/png" }),
    paintScene: (ctx, scene) => {
      renderScene(ctx, scene, sceneImages);
    },
  };

  try {
    const result = await exportVideo({
      renderInput: msg.renderInput,
      preset: msg.preset,
      ...(msg.durationMs !== undefined ? { durationMs: msg.durationMs } : {}),
      ...(msg.fps !== undefined ? { fps: msg.fps } : {}),
      onProgress: (fraction) => {
        if (!cancelled) post({ type: "progress", fraction });
      },
      deps,
    });

    if (cancelled) return;

    const videoBuffer = await result.video.arrayBuffer();
    post(
      {
        type: "done",
        video: videoBuffer,
        videoMime: result.video.type || "video/mp4",
      },
      [videoBuffer],
    );
  } catch (error) {
    if (cancelled) return;
    const message =
      error instanceof Error ? error.message : String(error ?? "export_failed");
    post({
      type: "error",
      message,
      code: error instanceof Error ? error.name : undefined,
    });
  }
}

workerScope.onmessage = (event: MessageEvent<unknown>) => {
  const data = event.data;
  if (isWorkerCancelMessage(data)) {
    cancelled = true;
    return;
  }
  if (isWorkerStartMessage(data)) {
    void runStart(data);
    return;
  }
  post({ type: "error", message: "worker_protocol_unknown_message" });
};
