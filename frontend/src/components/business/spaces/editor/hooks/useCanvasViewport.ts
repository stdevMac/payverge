"use client";

import { useCallback, useRef, useState } from "react";

export interface ViewportState {
  scale: number;
  panX: number;
  panY: number;
}

const MIN_SCALE = 0.05;
const MAX_SCALE = 4;

export function useCanvasViewport(initial?: Partial<ViewportState>) {
  const [viewport, setViewport] = useState<ViewportState>({
    scale: initial?.scale ?? 0.08,
    panX: initial?.panX ?? 40,
    panY: initial?.panY ?? 40,
  });
  const viewportRef = useRef(viewport);
  viewportRef.current = viewport;

  const setScale = useCallback((scale: number, origin?: { x: number; y: number }) => {
    setViewport((v) => {
      const next = Math.min(MAX_SCALE, Math.max(MIN_SCALE, scale));
      if (!origin) return { ...v, scale: next };
      // Keep world point under origin stable.
      const worldX = (origin.x - v.panX) / v.scale;
      const worldY = (origin.y - v.panY) / v.scale;
      return {
        scale: next,
        panX: origin.x - worldX * next,
        panY: origin.y - worldY * next,
      };
    });
  }, []);

  const zoomBy = useCallback(
    (factor: number, origin?: { x: number; y: number }) => {
      setScale(viewportRef.current.scale * factor, origin);
    },
    [setScale],
  );

  const zoomIn = useCallback(
    (origin?: { x: number; y: number }) => zoomBy(1.2, origin),
    [zoomBy],
  );

  const zoomOut = useCallback(
    (origin?: { x: number; y: number }) => zoomBy(1 / 1.2, origin),
    [zoomBy],
  );

  const panBy = useCallback((dx: number, dy: number) => {
    setViewport((v) => ({ ...v, panX: v.panX + dx, panY: v.panY + dy }));
  }, []);

  const setPan = useCallback((panX: number, panY: number) => {
    setViewport((v) => ({ ...v, panX, panY }));
  }, []);

  const fitToView = useCallback(
    (
      contentW: number,
      contentH: number,
      viewW: number,
      viewH: number,
      padding = 48,
    ) => {
      if (contentW <= 0 || contentH <= 0 || viewW <= 0 || viewH <= 0) return;
      const sx = (viewW - padding * 2) / contentW;
      const sy = (viewH - padding * 2) / contentH;
      const scale = Math.min(MAX_SCALE, Math.max(MIN_SCALE, Math.min(sx, sy)));
      const panX = (viewW - contentW * scale) / 2;
      const panY = (viewH - contentH * scale) / 2;
      setViewport({ scale, panX, panY });
    },
    [],
  );

  const reset = useCallback(() => {
    setViewport({ scale: 0.08, panX: 40, panY: 40 });
  }, []);

  /** Screen (client relative to svg) → world mm. */
  const screenToWorld = useCallback((sx: number, sy: number) => {
    const v = viewportRef.current;
    return {
      x: (sx - v.panX) / v.scale,
      y: (sy - v.panY) / v.scale,
    };
  }, []);

  /** World mm → screen. */
  const worldToScreen = useCallback((wx: number, wy: number) => {
    const v = viewportRef.current;
    return {
      x: wx * v.scale + v.panX,
      y: wy * v.scale + v.panY,
    };
  }, []);

  return {
    viewport,
    viewportRef,
    setScale,
    zoomBy,
    zoomIn,
    zoomOut,
    panBy,
    setPan,
    fitToView,
    reset,
    screenToWorld,
    worldToScreen,
    minScale: MIN_SCALE,
    maxScale: MAX_SCALE,
  };
}

export type CanvasViewportApi = ReturnType<typeof useCanvasViewport>;
