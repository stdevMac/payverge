"use client";

// P2-16 "Print all QR codes": one print-ready sheet for every ACTIVE table.
// Reuses the qrcode generation path from ./qrDownload.ts (per-table color
// override, else business default) and the existing useIframePrint pattern —
// the only window.print() call lives inside that client-only hook, so this
// module is "use client" and must never be imported by a server component.

import { useCallback, useMemo, useState } from "react";
import QRCode from "qrcode";
import { useIframePrint } from "../printers/useIframePrint";
import { buildQrSheetHtml, type QrSheetCard } from "./qrSheetHtml";

interface QrSheetTable {
  name: string;
  table_code: string;
  is_active: boolean;
  qr_foreground_color?: string;
  qr_background_color?: string;
}

interface QrSheetDefaults {
  qr_foreground_color?: string;
  qr_background_color?: string;
}

interface UseQrSheetPrintArgs {
  businessName: string;
  tables: QrSheetTable[];
  defaults?: QrSheetDefaults;
  strings: { scanCaption: string; documentTitle: string; poweredBy: string };
  onError?: (err: unknown) => void;
}

export function useQrSheetPrint({
  businessName,
  tables,
  defaults,
  strings,
  onError,
}: UseQrSheetPrintArgs) {
  const { print } = useIframePrint();
  const [printing, setPrinting] = useState(false);

  const activeTables = useMemo(
    () => tables.filter((t) => t.is_active && t.table_code),
    [tables],
  );
  const activeTableCount = activeTables.length;

  const printAll = useCallback(async () => {
    if (printing || activeTables.length === 0) return;
    setPrinting(true);
    try {
      const cards: QrSheetCard[] = await Promise.all(
        activeTables.map(async (t) => {
          const qrUrl = `${window.location.origin}/t/${t.table_code}`;
          /* eslint-disable no-restricted-syntax -- the qrcode library expects
             hex colors, not Tailwind tokens (mirrors qrDownload.ts). */
          const dark =
            t.qr_foreground_color || defaults?.qr_foreground_color || "#000000";
          const light =
            t.qr_background_color || defaults?.qr_background_color || "#FFFFFF";
          /* eslint-enable no-restricted-syntax */
          const qrDataUrl = await QRCode.toDataURL(qrUrl, {
            width: 512,
            margin: 2,
            color: { dark, light },
          });
          return { tableName: t.name, tableCode: t.table_code, qrDataUrl };
        }),
      );
      await print(buildQrSheetHtml(businessName, cards, strings));
    } catch (err) {
      onError?.(err);
    } finally {
      setPrinting(false);
    }
  }, [
    printing,
    activeTables,
    businessName,
    defaults,
    strings,
    print,
    onError,
  ]);

  return { printAll, printing, activeTableCount };
}
