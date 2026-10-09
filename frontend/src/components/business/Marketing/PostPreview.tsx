"use client";

import React, { useEffect, useRef } from "react";
import { Spinner } from "@nextui-org/react";
import { ImageOff } from "lucide-react";
import {
  evictFailedImage,
  LIBRARY_THUMBNAIL_WIDTH,
  renderPost,
  type RenderPostInput,
} from "./templates/renderPost";
import {
  getCachedThumbnailUrl,
  putCachedThumbnail,
  releaseCachedThumbnail,
  retainCachedThumbnail,
  withThumbnailRenderSlot,
} from "./templates/thumbnailCache";

interface Props {
  renderInput: RenderPostInput;
  isGenerating: boolean;
  emptyLabel: string;
  previewErrorLabel: string;
  previewRetryLabel: string;
  renderingLabel: string;
  generatingLabel: string;
  onRenderStateChange?: (state: PreviewRenderState) => void;
  /**
   * When true, defer rendering until the host intersects the viewport
   * (rootMargin 320px — same pattern as useSuggestionCaptions).
   */
  lazy?: boolean;
  /**
   * Render at a reduced canvas width (Library thumbnails). Full-res stays the
   * default for export/editor previews.
   */
  targetWidth?: number;
  /**
   * Stable cache key (e.g. activity id). When set with targetWidth, the rendered
   * canvas is converted to a blob URL, cached, and the canvas backing store is
   * released so list memory stays bounded.
   */
  cacheKey?: string;
}

export type PreviewRenderState = "rendering" | "ready" | "failed";

function canvasToBlobUrl(canvas: HTMLCanvasElement): Promise<string> {
  return new Promise((resolve, reject) => {
    canvas.toBlob((blob) => {
      // Drop the large pixel buffer as soon as we have a compressed bitmap.
      canvas.width = 0;
      canvas.height = 0;
      if (!blob) {
        reject(new Error("thumbnail_blob_failed"));
        return;
      }
      resolve(URL.createObjectURL(blob));
    }, "image/jpeg", 0.82);
  });
}

export function PostPreview({
  renderInput,
  isGenerating,
  emptyLabel,
  previewErrorLabel,
  previewRetryLabel,
  renderingLabel,
  generatingLabel,
  onRenderStateChange,
  lazy = false,
  targetWidth,
  cacheKey,
}: Props) {
  const { aspect, photoUrl } = renderInput;
  const hostRef = useRef<HTMLDivElement>(null);
  const rootRef = useRef<HTMLDivElement>(null);
  const retainedKeyRef = useRef<string | null>(null);
  const [renderFailed, setRenderFailed] = React.useState(false);
  const [rendering, setRendering] = React.useState(true);
  const [renderAttempt, setRenderAttempt] = React.useState(0);
  const [nearViewport, setNearViewport] = React.useState(!lazy);
  const [blobUrl, setBlobUrl] = React.useState<string | null>(() =>
    cacheKey ? getCachedThumbnailUrl(cacheKey) : null,
  );

  // IntersectionObserver laziness (mirrors useSuggestionCaptions).
  useEffect(() => {
    if (!lazy || nearViewport) return;
    const node = rootRef.current;
    if (!node) return;
    if (typeof IntersectionObserver === "undefined") {
      setNearViewport(true);
      return;
    }
    const observer = new IntersectionObserver(
      (entries) => {
        if (!entries.some((entry) => entry.isIntersecting)) return;
        setNearViewport(true);
        observer.disconnect();
      },
      { rootMargin: "320px 0px", threshold: 0.01 },
    );
    observer.observe(node);
    return () => observer.disconnect();
  }, [lazy, nearViewport]);

  // Retain/release cache consumers for this row.
  useEffect(() => {
    if (!cacheKey) return;
    const url = retainCachedThumbnail(cacheKey);
    retainedKeyRef.current = cacheKey;
    if (url) setBlobUrl(url);
    return () => {
      if (retainedKeyRef.current) {
        releaseCachedThumbnail(retainedKeyRef.current);
        retainedKeyRef.current = null;
      }
    };
  }, [cacheKey]);

  useEffect(() => {
    if (!nearViewport) return;
    if (blobUrl && cacheKey && getCachedThumbnailUrl(cacheKey) === blobUrl) {
      setRendering(false);
      setRenderFailed(false);
      onRenderStateChange?.("ready");
      return;
    }

    if (!hostRef.current && !cacheKey) return;
    const controller = new AbortController();
    setRenderFailed(false);
    setRendering(true);
    onRenderStateChange?.("rendering");

    const run = async () => {
      const renderOpts = {
        signal: controller.signal,
        ...(targetWidth != null ? { targetWidth } : {}),
      };
      const doRender = () => renderPost(renderInput, renderOpts);
      const canvas =
        targetWidth != null
          ? await withThumbnailRenderSlot(doRender, controller.signal)
          : await doRender();

      if (controller.signal.aborted) {
        canvas.width = 0;
        canvas.height = 0;
        return;
      }

      if (cacheKey && targetWidth != null) {
        const url = await canvasToBlobUrl(canvas);
        if (controller.signal.aborted) {
          try {
            URL.revokeObjectURL(url);
          } catch {
            /* ignore */
          }
          return;
        }
        putCachedThumbnail(cacheKey, url);
        setBlobUrl(url);
        // Clear any leftover canvas node from a prior non-cached render.
        hostRef.current?.replaceChildren();
        setRendering(false);
        onRenderStateChange?.("ready");
        return;
      }

      if (!hostRef.current) {
        canvas.width = 0;
        canvas.height = 0;
        return;
      }
      canvas.style.width = "100%";
      canvas.style.height = "auto";
      canvas.style.display = "block";
      hostRef.current.replaceChildren(canvas);
      setRendering(false);
      onRenderStateChange?.("ready");
    };

    const timeout = window.setTimeout(() => {
      void run().catch((error: unknown) => {
        if (
          !controller.signal.aborted &&
          (error as { name?: string }).name !== "AbortError"
        ) {
          setRenderFailed(true);
          setRendering(false);
          onRenderStateChange?.("failed");
        }
      });
    }, 100);

    const host = hostRef.current;
    return () => {
      window.clearTimeout(timeout);
      controller.abort();
      // Drop any live canvas node when the effect re-runs or unmounts so the
      // backing store does not accumulate across list re-renders. Capture host
      // at effect time — hostRef.current may already point elsewhere in cleanup.
      if (host) {
        const canvases = host.querySelectorAll("canvas");
        canvases.forEach((node) => {
          node.width = 0;
          node.height = 0;
        });
        host.replaceChildren();
      }
    };
  }, [
    renderAttempt,
    renderInput,
    onRenderStateChange,
    nearViewport,
    targetWidth,
    cacheKey,
    blobUrl,
  ]);

  const ratio =
    aspect === "9:16" ? "9 / 16" : aspect === "1:1" ? "1 / 1" : "4 / 5";

  return (
    <div
      ref={rootRef}
      className="relative w-full overflow-hidden bg-warm-50"
      style={{ aspectRatio: ratio }}
      data-preview-lazy={lazy ? "true" : "false"}
      data-preview-target-width={
        targetWidth != null ? String(targetWidth) : undefined
      }
      data-preview-cache-key={cacheKey}
    >
      {blobUrl ? (
        // eslint-disable-next-line @next/next/no-img-element -- blob URL thumbnail
        <img
          src={blobUrl}
          alt=""
          className="absolute inset-0 h-full w-full object-cover"
          data-testid="library-thumbnail-img"
        />
      ) : (
        <div ref={hostRef} className="absolute inset-0" />
      )}
      {rendering && !isGenerating && nearViewport && !blobUrl && (
        <div
          className="absolute inset-0 flex items-center justify-center bg-warm-50/90"
          role="status"
          aria-live="polite"
        >
          <div className="flex items-center gap-2 text-xs font-semibold text-ink-600">
            <Spinner size="sm" />
            <span>{renderingLabel}</span>
          </div>
        </div>
      )}
      {!photoUrl && !isGenerating && !renderFailed && !blobUrl && (
        <div className="pointer-events-none absolute inset-0 flex items-center justify-center">
          <div className="flex items-center gap-1.5 rounded-full bg-ink-900/55 px-3 py-1.5 text-white/95 backdrop-blur-sm">
            <ImageOff className="w-4 h-4 opacity-80" aria-hidden="true" />
            <p className="text-xs">{emptyLabel}</p>
          </div>
        </div>
      )}
      {renderFailed && !isGenerating && (
        <div
          className="absolute inset-0 flex items-center justify-center bg-warm-50 px-6 text-center"
          role="alert"
        >
          <div className="flex flex-col items-center gap-2 rounded-xl border border-amber-200 bg-amber-50 px-3 py-2 text-xs font-medium text-amber-800">
            <span className="flex items-center gap-2">
              <ImageOff className="h-4 w-4 shrink-0" aria-hidden="true" />
              <span>{previewErrorLabel}</span>
            </span>
            <button
              type="button"
              className="rounded-lg border border-amber-300 bg-white px-2.5 py-1 font-semibold text-amber-900 transition hover:bg-amber-100 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-amber-500"
              onClick={() => {
                // A failed image is negative-cached for 5 minutes; retry must
                // actually re-read the already-paid asset (same URL, no
                // regeneration), not replay the cached rejection (L4-21).
                if (renderInput.photoUrl) {
                  evictFailedImage(renderInput.photoUrl);
                }
                if (renderInput.logoUrl?.trim()) {
                  evictFailedImage(renderInput.logoUrl.trim());
                }
                setRenderAttempt((attempt) => attempt + 1);
              }}
            >
              {previewRetryLabel}
            </button>
          </div>
        </div>
      )}
      {isGenerating && (
        <div
          className="absolute inset-0 flex items-center justify-center bg-ink-900/10"
          role="status"
          aria-live="polite"
        >
          <div className="flex items-center gap-2 rounded-full bg-white/90 px-3 py-2 text-xs font-semibold text-ink-700 shadow-sm">
            <Spinner color="primary" size="sm" />
            <span>{generatingLabel}</span>
          </div>
        </div>
      )}
    </div>
  );
}

export { LIBRARY_THUMBNAIL_WIDTH };
