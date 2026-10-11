"use client";

import React, { useEffect, useMemo, useRef, useState } from "react";

import type { PlannedMenuDocument } from "@/lib/menuPrint/types";

const MM_TO_CSS_PX = 96 / 25.4;
const FIT_PADDING_PX = 32;
const MIN_FIT_SCALE = 0.12;

export interface PrintMenuPreviewProps {
  document: PlannedMenuDocument;
  html: string;
  selectedPage: number;
  tString: (key: string) => string;
  onSelectedPageChange: (pageIndex: number) => void;
}

interface PageFrameProps {
  document: PlannedMenuDocument;
  html: string;
  pageIndex: number;
  scale: number;
  title: string;
  overlays?: boolean;
}

function scrollIframeToPage(
  iframe: HTMLIFrameElement,
  pageIndex: number,
  pageHeightPx: number,
): void {
  try {
    const offset = Math.round(pageIndex * pageHeightPx);
    if (iframe.contentDocument?.documentElement) {
      iframe.contentDocument.documentElement.scrollTop = offset;
    }
    if (iframe.contentDocument?.body)
      iframe.contentDocument.body.scrollTop = offset;
  } catch {
    // The document remains useful at page one when browser sandbox policy is
    // stricter than expected. The print HTML itself is never modified.
  }
}

function PageFrame({
  document,
  html,
  pageIndex,
  scale,
  title,
  overlays = false,
}: PageFrameProps) {
  const iframeRef = useRef<HTMLIFrameElement>(null);
  const widthPx = document.geometry.widthMm * MM_TO_CSS_PX;
  const heightPx = document.geometry.heightMm * MM_TO_CSS_PX;
  const safe = document.geometry.safeArea;

  useEffect(() => {
    const iframe = iframeRef.current;
    if (!iframe) return;
    const alignPage = () => scrollIframeToPage(iframe, pageIndex, heightPx);
    alignPage();
    iframe.addEventListener("load", alignPage);
    return () => iframe.removeEventListener("load", alignPage);
  }, [heightPx, html, pageIndex]);

  return (
    <div
      className="relative flex-shrink-0 overflow-hidden bg-white shadow-sm ring-1 ring-ink-950/10"
      style={{ width: widthPx * scale, height: heightPx * scale }}
    >
      <div
        className="relative origin-top-left"
        style={{
          width: widthPx,
          height: heightPx,
          transform: `scale(${scale})`,
        }}
      >
        <iframe
          ref={iframeRef}
          srcDoc={html}
          sandbox="allow-same-origin"
          tabIndex={-1}
          title={title}
          className="pointer-events-none block border-0 bg-white"
          style={{ width: widthPx, height: heightPx }}
        />
        {overlays ? (
          <div
            className="pointer-events-none absolute inset-0"
            aria-hidden="true"
          >
            <div
              className="absolute border border-dashed border-amber-500/80"
              data-testid="print-safe-area-overlay"
              style={{
                top: safe.top * MM_TO_CSS_PX,
                right: safe.right * MM_TO_CSS_PX,
                bottom: safe.bottom * MM_TO_CSS_PX,
                left: safe.left * MM_TO_CSS_PX,
              }}
            />
            {document.geometry.trimGuideInsetMm > 0 ? (
              <div
                className="absolute border border-rose-500/80"
                data-testid="print-trim-guide-overlay"
                style={{
                  inset: document.geometry.trimGuideInsetMm * MM_TO_CSS_PX,
                }}
              />
            ) : null}
            {document.geometry.foldsMm.map((foldMm) => (
              <span
                key={foldMm}
                className="absolute inset-y-0 border-l border-dashed border-brand/70"
                data-testid="print-fold-overlay"
                style={{ left: foldMm * MM_TO_CSS_PX }}
              />
            ))}
          </div>
        ) : null}
      </div>
    </div>
  );
}

export function PrintMenuPreview({
  document,
  html,
  selectedPage,
  tString,
  onSelectedPageChange,
}: PrintMenuPreviewProps) {
  const [zoom, setZoom] = useState<"fit" | "actual">("fit");
  const [viewport, setViewport] = useState({ width: 0, height: 0 });
  const viewportRef = useRef<HTMLDivElement>(null);
  const widthPx = document.geometry.widthMm * MM_TO_CSS_PX;
  const heightPx = document.geometry.heightMm * MM_TO_CSS_PX;

  useEffect(() => {
    if (selectedPage >= document.pages.length) onSelectedPageChange(0);
  }, [document.pages.length, onSelectedPageChange, selectedPage]);

  useEffect(() => {
    const element = viewportRef.current;
    if (!element) return;
    const update = (width: number, height: number) =>
      setViewport({ width, height });
    update(element.clientWidth, element.clientHeight);
    if (typeof ResizeObserver === "undefined") return;
    const observer = new ResizeObserver(([entry]) => {
      if (entry) update(entry.contentRect.width, entry.contentRect.height);
    });
    observer.observe(element);
    return () => observer.disconnect();
  }, []);

  const fitScale = useMemo(() => {
    if (viewport.width <= 0 || viewport.height <= 0) return 0.5;
    return Math.min(
      1,
      Math.max(
        MIN_FIT_SCALE,
        Math.min(
          (viewport.width - FIT_PADDING_PX) / widthPx,
          (viewport.height - FIT_PADDING_PX) / heightPx,
        ),
      ),
    );
  }, [heightPx, viewport.height, viewport.width, widthPx]);
  const activeScale = zoom === "actual" ? 1 : fitScale;
  const activePage = Math.min(
    Math.max(0, selectedPage),
    Math.max(0, document.pages.length - 1),
  );

  return (
    <section aria-label={tString("print.preview.label")} className="space-y-4">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <h3 className="text-sm font-semibold text-ink-900">
          {tString("print.preview.label")}
        </h3>
        <div
          className="flex rounded-full border border-warm-300 bg-white p-1"
          role="group"
          aria-label={tString("print.preview.zoom")}
        >
          <button
            type="button"
            aria-pressed={zoom === "fit"}
            onClick={() => setZoom("fit")}
            className={[
              "rounded-full px-3 py-1 text-xs font-medium",
              zoom === "fit" ? "bg-brand text-white" : "text-ink-700",
            ].join(" ")}
          >
            {tString("print.preview.fit")}
          </button>
          <button
            type="button"
            aria-pressed={zoom === "actual"}
            onClick={() => setZoom("actual")}
            className={[
              "rounded-full px-3 py-1 text-xs font-medium",
              zoom === "actual" ? "bg-brand text-white" : "text-ink-700",
            ].join(" ")}
          >
            {tString("print.preview.actualSize")}
          </button>
        </div>
      </div>

      <div
        className="flex gap-2 overflow-x-auto pb-1"
        aria-label={tString("print.preview.pages")}
      >
        {document.pages.map((page) => {
          const selected = page.index === activePage;
          return (
            <button
              key={page.index}
              type="button"
              aria-label={`${tString("print.pageThumbnail")} ${page.index + 1}`}
              aria-pressed={selected}
              onClick={() => onSelectedPageChange(page.index)}
              className={[
                "flex-shrink-0 rounded-lg border p-1.5 transition-colors",
                "focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand",
                selected
                  ? "border-brand bg-brand-50"
                  : "border-warm-200 bg-white hover:border-warm-300",
              ].join(" ")}
            >
              <PageFrame
                document={document}
                html={html}
                pageIndex={page.index}
                scale={0.09}
                title={`${tString("print.thumbnailPage")} ${page.index + 1}`}
              />
              <span className="mt-1 block text-center text-xs font-medium text-ink-700">
                {tString("print.page")} {page.index + 1}
              </span>
            </button>
          );
        })}
      </div>

      <div
        ref={viewportRef}
        className={[
          "overflow-auto rounded-xl border border-warm-200 bg-warm-100 p-4",
          "h-[min(64vh,52rem)] min-h-[28rem]",
        ].join(" ")}
      >
        <div className="flex min-h-full min-w-full items-start justify-center">
          <PageFrame
            document={document}
            html={html}
            pageIndex={activePage}
            scale={activeScale}
            title={`${tString("print.previewPage")} ${activePage + 1}`}
            overlays
          />
        </div>
      </div>
    </section>
  );
}
