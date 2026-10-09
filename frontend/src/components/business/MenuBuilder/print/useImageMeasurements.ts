import { useEffect, useMemo, useState } from "react";

type ImageMeasurement =
  | { status: "loading" }
  | { status: "ready"; width: number; height: number }
  | { status: "failed" }
  | { status: "timeout" };

export interface ImageMeasurementsResult {
  status: "loading" | "complete";
  measurements: Record<string, ImageMeasurement>;
}

export interface UseImageMeasurementsOptions {
  timeoutMs?: number;
}

interface ImageMeasurementState {
  batchKey: string;
  result: ImageMeasurementsResult;
}

const EMPTY_RESULT: ImageMeasurementsResult = {
  status: "complete",
  measurements: {},
};

function normalizeUrls(urls: readonly string[]): string[] {
  return Array.from(
    new Set(urls.map((url) => url.trim()).filter((url) => url.length > 0)),
  ).sort();
}

function initialLoadingResult(
  urls: readonly string[],
): ImageMeasurementsResult {
  return {
    status: "loading",
    measurements: Object.fromEntries(
      urls.map((url) => [url, { status: "loading" } as const]),
    ),
  };
}

export function useImageMeasurements(
  urls: readonly string[],
  { timeoutMs = 5000 }: UseImageMeasurementsOptions = {},
): ImageMeasurementsResult {
  const signature = JSON.stringify(normalizeUrls(urls));
  const normalizedUrls = useMemo(
    () => JSON.parse(signature) as string[],
    [signature],
  );
  const boundedTimeoutMs =
    Number.isFinite(timeoutMs) && timeoutMs >= 0 ? timeoutMs : 5000;
  const batch = useMemo(
    () => ({
      key: JSON.stringify([signature, boundedTimeoutMs]),
      initialResult:
        normalizedUrls.length === 0
          ? EMPTY_RESULT
          : initialLoadingResult(normalizedUrls),
    }),
    [boundedTimeoutMs, normalizedUrls, signature],
  );
  const { key: batchKey, initialResult } = batch;
  const [state, setState] = useState<ImageMeasurementState>(() => ({
    batchKey,
    result: initialResult,
  }));

  useEffect(() => {
    setState((current) =>
      current.batchKey === batchKey
        ? current
        : { batchKey, result: initialResult },
    );

    if (normalizedUrls.length === 0) {
      return;
    }

    let active = true;
    const settled = new Set<string>();
    const pending = normalizedUrls.map((url) => {
      const image = new Image();
      let timer: ReturnType<typeof setTimeout>;

      const settle = (measurement: ImageMeasurement) => {
        if (!active || settled.has(url)) return;

        settled.add(url);
        clearTimeout(timer);
        image.onload = null;
        image.onerror = null;
        setState((current) => {
          if (current.batchKey !== batchKey) return current;

          return {
            batchKey,
            result: {
              status:
                settled.size === normalizedUrls.length ? "complete" : "loading",
              measurements: {
                ...current.result.measurements,
                [url]: measurement,
              },
            },
          };
        });
      };

      image.onload = () => {
        settle({
          status: "ready",
          width: image.naturalWidth,
          height: image.naturalHeight,
        });
      };
      image.onerror = () => settle({ status: "failed" });
      timer = setTimeout(() => settle({ status: "timeout" }), boundedTimeoutMs);
      image.crossOrigin = "anonymous";
      image.src = url;

      return { image, timer };
    });

    return () => {
      active = false;
      pending.forEach(({ image, timer }) => {
        clearTimeout(timer);
        image.onload = null;
        image.onerror = null;
      });
    };
  }, [batchKey, boundedTimeoutMs, initialResult, normalizedUrls]);

  return state.batchKey === batchKey ? state.result : initialResult;
}
