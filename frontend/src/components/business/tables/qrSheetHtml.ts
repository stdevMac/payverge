// Print-sheet HTML builder for "Print all QR codes" (P2-16).
// Pure: no DOM access, fully unit-testable. The caller (useQrSheetPrint)
// renders this through useIframePrint, which owns the client-only
// window.print() call — this module must never touch window.

export interface QrSheetCard {
  /** Operator-facing table label, e.g. "Main Room 1". */
  tableName: string;
  /** Public table code — printed under the QR as the short URL. */
  tableCode: string;
  /** PNG data URL produced by the qrcode library's toDataURL. */
  qrDataUrl: string;
}

export interface QrSheetStrings {
  /** Localized "Scan to view the menu & order" caption under each QR. */
  scanCaption: string;
  /** Document title (shows in the print dialog / saved-PDF filename). */
  documentTitle: string;
  /** L3-29: same branding footer as canvas preview / PNG download. */
  poweredBy: string;
}

export function escapeHtml(value: string): string {
  return String(value)
    .replace(/&/g, "&amp;")
    .replace(/</g, "&lt;")
    .replace(/>/g, "&gt;")
    .replace(/"/g, "&quot;");
}

/** 2 columns × 3 rows per A4/Letter page. */
const CARDS_PER_PAGE = 6;

export function buildQrSheetHtml(
  businessName: string,
  cards: QrSheetCard[],
  strings: QrSheetStrings,
): string {
  const cardHtml = cards
    .map(
      (card) => `<div class="card">
  <p class="business">${escapeHtml(businessName)}</p>
  <img class="qr" src="${card.qrDataUrl}" alt="" />
  <p class="table">${escapeHtml(card.tableName)}</p>
  <p class="caption">${escapeHtml(strings.scanCaption)}</p>
  <p class="code">/t/${escapeHtml(card.tableCode)}</p>
  <p class="powered">${escapeHtml(strings.poweredBy)}</p>
</div>`,
    )
    .join("\n");

  return `<!doctype html><html><head><meta charset="utf-8"/><title>${escapeHtml(strings.documentTitle)}</title>
<style>
  @page { size: auto; margin: 10mm; }
  * { box-sizing: border-box; }
  body { font-family: system-ui, sans-serif; color: #1c1917; margin: 0; }
  .sheet { display: grid; grid-template-columns: repeat(2, 1fr); gap: 6mm; }
  .card {
    border: 1px dashed #d6d3d1; /* cut guide */
    border-radius: 4mm;
    padding: 6mm 4mm;
    text-align: center;
    break-inside: avoid;
    page-break-inside: avoid; /* WebKit print engines still read this */
  }
  /* Tear cleanly into pages of ${CARDS_PER_PAGE}. */
  .card:nth-child(${CARDS_PER_PAGE}n) { break-after: page; page-break-after: always; }
  .business { margin: 0 0 3mm; font-size: 12px; font-weight: 600; letter-spacing: 0.04em; text-transform: uppercase; color: #57534e; }
  .qr { width: 52mm; height: 52mm; max-width: 100%; }
  .table { margin: 3mm 0 0; font-size: 18px; font-weight: 700; }
  .caption { margin: 1.5mm 0 0; font-size: 11px; color: #57534e; }
  .code { margin: 1mm 0 0; font-size: 10px; color: #a8a29e; font-family: ui-monospace, monospace; }
  .powered { margin: 2mm 0 0; font-size: 10px; font-weight: 600; color: #57534e; }
</style></head><body>
<div class="sheet">
${cardHtml}
</div>
</body></html>`;
}
