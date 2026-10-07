/* eslint-disable no-restricted-syntax -- Canvas 2D API and QRCode library require hex color strings; not Tailwind classes */
import { useEffect, useRef } from "react";
import QRCode from "qrcode";

interface QRCodeWithTextProps {
  tableCode: string;
  businessName?: string;
  tableName?: string;
  showBusinessName?: boolean;
  showTableName?: boolean;
  logoUrl?: string;
  foregroundColor?: string;
  backgroundColor?: string;
  logoSize?: number;
  size?: number;
  textFont?: string;
  /** Required branding footer — callers must pass a translated string (Root D / L3-29). */
  poweredByText: string;
  // Accessible label for the rendered QR canvas. Canvas has no intrinsic
  // text, so screen readers need this to announce what the image is.
  ariaLabel?: string;
  // Fired only after the QR image has decoded and been drawn to the canvas.
  // Callers must not treat mounting an empty/failed canvas as a real preview.
  onRendered?: () => void;
}

export default function QRCodeWithText({
  tableCode,
  businessName = "",
  tableName = "",
  showBusinessName = false,
  showTableName = false,
  logoUrl,
  foregroundColor = "#000000",
  backgroundColor = "#FFFFFF",
  logoSize = 20,
  size = 300,
  textFont = "Verdana",
  poweredByText,
  ariaLabel,
  onRendered,
}: QRCodeWithTextProps) {
  const canvasRef = useRef<HTMLCanvasElement>(null);

  useEffect(() => {
    if (!canvasRef.current) return;

    const canvas = canvasRef.current;
    const ctx = canvas.getContext("2d");
    if (!ctx) return;
    let cancelled = false;

    const qrUrl = `${window.location.origin}/t/${tableCode}`;
    const padding = 20;
    const topTextHeight = showBusinessName || showTableName ? 40 : 10;
    const bottomTextHeight = 30;
    const totalHeight = size + topTextHeight + bottomTextHeight + padding * 2;

    // Set canvas size
    canvas.width = size + padding * 2;
    canvas.height = totalHeight;

    // Fill background — honor the chosen background color instead of forcing
    // white, so a dark/branded background isn't rendered as a white plate.
    ctx.fillStyle = backgroundColor;
    ctx.fillRect(0, 0, canvas.width, canvas.height);

    // Generate QR code as data URL first
    QRCode.toDataURL(
      qrUrl,
      {
        width: size,
        margin: 2,
        color: {
          dark: foregroundColor,
          light: backgroundColor,
        },
      },
      (error, url) => {
        if (cancelled) return;
        if (error) {
          console.error("QR Code generation error:", error);
          return;
        }

        // Create image from data URL
        const qrImage = new Image();
        qrImage.onload = () => {
          if (cancelled) return;
          // Draw QR code
          ctx.drawImage(qrImage, padding, topTextHeight, size, size);

          // Add logo if provided
          if (logoUrl) {
            const img = new Image();
            img.crossOrigin = "anonymous";
            img.onload = () => {
              const logoSizePixels = (size * logoSize) / 100;
              const logoX = padding + (size - logoSizePixels) / 2;
              const logoY = topTextHeight + (size - logoSizePixels) / 2;

              // Draw background plate for logo, matching the canvas
              // background so it blends with non-white themes.
              ctx.fillStyle = backgroundColor;
              ctx.fillRect(
                logoX - 5,
                logoY - 5,
                logoSizePixels + 10,
                logoSizePixels + 10,
              );

              // Draw logo
              ctx.drawImage(img, logoX, logoY, logoSizePixels, logoSizePixels);
            };
            img.src = logoUrl;
          }

          // Draw top text (business name and/or table name) in the QR's
          // foreground color so it stays legible on dark/branded backgrounds
          // instead of being drawn as invisible black-on-dark.
          if (showBusinessName || showTableName) {
            ctx.fillStyle = foregroundColor;
            ctx.textAlign = "center";
            ctx.font = `bold 16px ${textFont}`;

            const headerText = [];
            if (showBusinessName && businessName) headerText.push(businessName);
            if (showTableName && tableName) headerText.push(tableName);

            const text = headerText.join(" - ");
            ctx.fillText(text, canvas.width / 2, 28);
          }

          // Draw "Powered by Payverge" at bottom, also in the foreground color
          // so it doesn't disappear on non-white backgrounds.
          ctx.fillStyle = foregroundColor;
          ctx.textAlign = "center";
          ctx.font = `600 13px ${textFont}`;
          ctx.fillText(
            poweredByText,
            canvas.width / 2,
            topTextHeight + size + 22,
          );
          onRendered?.();
        };
        qrImage.src = url;
      },
    );

    return () => {
      cancelled = true;
    };
  }, [
    tableCode,
    businessName,
    tableName,
    showBusinessName,
    showTableName,
    logoUrl,
    foregroundColor,
    backgroundColor,
    logoSize,
    size,
    textFont,
    poweredByText,
    onRendered,
  ]);

  return (
    <canvas
      ref={canvasRef}
      role="img"
      aria-label={ariaLabel || `QR code for ${tableName || tableCode}`}
      className="max-w-full h-auto"
    />
  );
}
