import type { PrintMenuModel } from "./types";

type ImageMeasurementStatus = "ready" | "failed" | "timeout";

export interface ImageMeasurement {
  url: string;
  width?: number;
  height?: number;
  status: ImageMeasurementStatus;
}

export interface MenuPrintAudit {
  categoryCount: number;
  itemCount: number;
  describedItemCount: number;
  averageDescriptionLength: number;
  availablePhotoCount: number;
  photoCoverage: number;
  lowResolutionUrls: string[];
  failedUrls: string[];
  duplicateUrls: string[];
  measurements?: Readonly<Record<string, ImageMeasurement>>;
}

export const MIN_FEATURE_PRINT_PX = 1200;
export const MIN_TILE_PRINT_PX = 600;

export function effectivePrintDpi(
  measurement: ImageMeasurement,
  printedSize: { widthMm: number; heightMm: number },
): number | undefined {
  if (
    measurement.status !== "ready" ||
    !measurement.width ||
    !measurement.height ||
    printedSize.widthMm <= 0 ||
    printedSize.heightMm <= 0
  ) {
    return undefined;
  }
  const horizontalDpi = measurement.width / (printedSize.widthMm / 25.4);
  const verticalDpi = measurement.height / (printedSize.heightMm / 25.4);
  return Number(Math.min(horizontalDpi, verticalDpi).toFixed(1));
}

export function auditMenuForPrint(
  model: PrintMenuModel,
  measurements: Record<string, ImageMeasurement>,
): MenuPrintAudit {
  const items = model.sections.flatMap((section) => section.items);
  const urls = items.flatMap((item) => item.imageCandidates ?? []);
  const uniqueUrls = [...new Set(urls)];
  const failedUrls = uniqueUrls.filter(
    (url) => measurements[url]?.status === "failed",
  );
  const lowResolutionUrls = uniqueUrls.filter((url) => {
    const measurement = measurements[url];
    return (
      measurement?.status === "ready" &&
      Math.min(measurement.width ?? 0, measurement.height ?? 0) <
        MIN_TILE_PRINT_PX
    );
  });
  const describedItems = items.filter((item) => item.description);
  const totalDescriptionLength = describedItems.reduce(
    (total, item) => total + (item.description?.length ?? 0),
    0,
  );

  return {
    categoryCount: model.sections.length,
    itemCount: items.length,
    describedItemCount: describedItems.length,
    averageDescriptionLength:
      describedItems.length > 0
        ? totalDescriptionLength / describedItems.length
        : 0,
    availablePhotoCount: uniqueUrls.length - failedUrls.length,
    photoCoverage:
      items.length > 0
        ? items.filter((item) => (item.imageCandidates ?? []).length > 0)
            .length / items.length
        : 0,
    lowResolutionUrls,
    failedUrls,
    duplicateUrls: urls.filter((url, index) => urls.indexOf(url) !== index),
    measurements: Object.fromEntries(
      uniqueUrls.flatMap((url) =>
        measurements[url] ? [[url, { ...measurements[url] }] as const] : [],
      ),
    ),
  };
}
