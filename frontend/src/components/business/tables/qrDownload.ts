import QRCode from "qrcode";

/**
 * Quick-download a table's QR code as a PNG, without going through the
 * detail drawer. Used by the Tables list row's "Download QR" action.
 *
 * L3-29: output includes the same "Powered by Payverge" footer as the
 * canvas preview and print sheet (callers pass a translated string).
 */
export interface QuickQRColors {
  foregroundColor?: string;
  backgroundColor?: string;
}

export interface DownloadTableQROptions extends QuickQRColors {
  /** Required branding footer (Root D / L3-29) — already translated. */
  poweredByText: string;
}

export async function downloadTableQR(
  tableCode: string,
  options: DownloadTableQROptions,
  size = 512,
): Promise<void> {
  const qrUrl = `${window.location.origin}/t/${tableCode}`;
  /* eslint-disable no-restricted-syntax -- the qrcode library expects
     hex colors, not Tailwind tokens. */
  const dark = options.foregroundColor || "#000000";
  const light = options.backgroundColor || "#FFFFFF";
  /* eslint-enable no-restricted-syntax */

  const qrDataUrl = await QRCode.toDataURL(qrUrl, {
    width: size,
    margin: 2,
    color: { dark, light },
  });

  const padding = 24;
  const footerHeight = 36;
  const canvas = document.createElement("canvas");
  canvas.width = size + padding * 2;
  canvas.height = size + padding * 2 + footerHeight;
  const ctx = canvas.getContext("2d");
  if (!ctx) {
    throw new Error("Could not create canvas for QR download");
  }

  ctx.fillStyle = light;
  ctx.fillRect(0, 0, canvas.width, canvas.height);

  await new Promise<void>((resolve, reject) => {
    const img = new Image();
    img.onload = () => {
      ctx.drawImage(img, padding, padding, size, size);
      ctx.fillStyle = dark;
      ctx.textAlign = "center";
      ctx.font = "600 14px system-ui, sans-serif";
      ctx.fillText(
        options.poweredByText,
        canvas.width / 2,
        padding + size + 24,
      );
      resolve();
    };
    img.onerror = () => reject(new Error("QR image decode failed"));
    img.src = qrDataUrl;
  });

  const link = document.createElement("a");
  link.download = `${tableCode || "table"}-qr.png`;
  link.href = canvas.toDataURL("image/png");
  document.body.appendChild(link);
  link.click();
  document.body.removeChild(link);
}
