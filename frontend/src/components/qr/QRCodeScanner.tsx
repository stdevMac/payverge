"use client";

import React, { useState, useRef, useEffect, useCallback, useId } from "react";
import { createPortal } from "react-dom";
import { Card, CardBody, Button, Spinner } from "@nextui-org/react";
import { Camera, X, QrCode } from "lucide-react";
import { useGuestTranslation } from "@/i18n/GuestTranslationProvider";
import { extractTableCodeFromURL } from "@/utils/qrValidation";
import { useDialogBehavior } from "@/hooks/useDialogBehavior";

interface QRCodeScannerProps {
  onScan: (tableCode: string) => void;
  onError: (error: string) => void;
  onClose: () => void;
}

/**
 * Pull a table code out of a scanned QR payload. Accepts either a full URL
 * (https://pos.example.com/t/demo-50-core-table-01) or a bare code. Shared with
 * /scan manual-entry validation so camera + typed paths accept the same codes.
 */
export const extractTableCode = (qrData: string): string | null =>
  extractTableCodeFromURL(qrData);

const QRCodeScanner: React.FC<QRCodeScannerProps> = ({
  onScan,
  onError,
  onClose,
}) => {
  const { t } = useGuestTranslation();
  const tString = (key: string): string => t(`scan.${key}`);
  const videoRef = useRef<HTMLVideoElement>(null);
  const canvasRef = useRef<HTMLCanvasElement>(null);
  const dialogRef = useRef<HTMLDivElement>(null);
  const titleId = useId();
  const [isScanning, setIsScanning] = useState(false);
  const [hasPermission, setHasPermission] = useState<boolean | null>(null);
  const streamRef = useRef<MediaStream | null>(null);

  useDialogBehavior({
    isOpen: true,
    onClose,
    containerRef: dialogRef,
  });

  useEffect(() => {
    const node = dialogRef.current;
    if (!node) return;
    const inerted: HTMLElement[] = [];
    Array.from(document.body.children).forEach((child) => {
      if (!(child instanceof HTMLElement)) return;
      if (child === node || child.contains(node) || node.contains(child)) return;
      if (!child.inert) {
        child.inert = true;
        inerted.push(child);
      }
    });
    return () => {
      inerted.forEach((element) => {
        element.inert = false;
      });
    };
  }, []);

  // Prefer the native BarcodeDetector when available; otherwise lazy-load jsQR
  // as a pure-JS decoder. Both resolve through the same scan loop below.
  const detectorRef = useRef<{
    detect: (s: CanvasImageSource) => Promise<Array<{ rawValue: string }>>;
  } | null>(null);
  const jsqrRef = useRef<typeof import("jsqr").default | null>(null);
  const scanBusyRef = useRef(false);

  useEffect(() => {
    let cancelled = false;
    (async () => {
      const w = window as unknown as {
        BarcodeDetector?: new (o: {
          formats: string[];
        }) => NonNullable<typeof detectorRef.current>;
      };
      if (w.BarcodeDetector) {
        try {
          detectorRef.current = new w.BarcodeDetector({ formats: ["qr_code"] });
          return;
        } catch {
          // BarcodeDetector unavailable on this browser — fall through to jsQR.
        }
      }
      const mod = await import("jsqr");
      if (!cancelled) jsqrRef.current = mod.default;
    })();
    return () => {
      cancelled = true;
    };
  }, []);

  const startCamera = async () => {
    try {
      const stream = await navigator.mediaDevices.getUserMedia({
        video: {
          facingMode: "environment", // Use back camera
          width: { ideal: 1280 },
          height: { ideal: 720 },
        },
      });

      streamRef.current = stream;
      if (videoRef.current) {
        videoRef.current.srcObject = stream;
        await videoRef.current.play();
      }
      setHasPermission(true);
      setIsScanning(true);
    } catch (error) {
      console.error("Camera access denied:", error);
      setHasPermission(false);
      onError(tString("cameraDeniedError"));
    }
  };

  const stopCamera = () => {
    if (streamRef.current) {
      streamRef.current.getTracks().forEach((track) => track.stop());
      streamRef.current = null;
    }
    setIsScanning(false);
  };

  const scanQRCode = useCallback(async () => {
    if (!videoRef.current || !canvasRef.current || !isScanning) return;
    if (scanBusyRef.current) return;

    const video = videoRef.current;
    const canvas = canvasRef.current;
    const context = canvas.getContext("2d", { willReadFrequently: true });

    if (!context || video.videoWidth === 0) return;

    scanBusyRef.current = true;
    try {
      canvas.width = video.videoWidth;
      canvas.height = video.videoHeight;
      context.drawImage(video, 0, 0, canvas.width, canvas.height);

      let qrResult: string | null = null;
      if (detectorRef.current) {
        const codes = await detectorRef.current.detect(canvas);
        qrResult = codes[0]?.rawValue ?? null;
      } else if (jsqrRef.current) {
        const imageData = context.getImageData(
          0,
          0,
          canvas.width,
          canvas.height,
        );
        qrResult =
          jsqrRef.current(imageData.data, imageData.width, imageData.height)
            ?.data ?? null;
      }

      if (qrResult) {
        const tableCode = extractTableCode(qrResult);
        if (tableCode) {
          stopCamera();
          onScan(tableCode);
        }
      }
    } catch (error) {
      console.error("QR scan error:", error);
    } finally {
      scanBusyRef.current = false;
    }
  }, [isScanning, onScan]);

  useEffect(() => {
    let scanInterval: NodeJS.Timeout;

    if (isScanning) {
      scanInterval = setInterval(() => {
        void scanQRCode();
      }, 100); // Scan every 100ms
    }

    return () => {
      if (scanInterval) clearInterval(scanInterval);
    };
  }, [isScanning, scanQRCode]);

  useEffect(() => {
    return () => {
      stopCamera();
    };
  }, []);

  const handleManualEntry = () => {
    const tableCode = prompt(tString("manualEntryPrompt"));
    if (tableCode && tableCode.trim()) {
      stopCamera();
      onScan(tableCode.trim().toUpperCase());
    }
  };

  const dialog = (
    <div
      ref={dialogRef}
      role="dialog"
      aria-modal="true"
      aria-labelledby={titleId}
      tabIndex={-1}
      className="fixed inset-0 bg-black bg-opacity-90 z-50 flex items-center justify-center"
    >
      <div className="relative w-full max-w-md mx-4">
        <Card>
          <CardBody className="p-0">
            {/* Header */}
            <div className="flex items-center justify-between p-4 border-b">
              <div className="flex items-center gap-2">
                <QrCode className="w-5 h-5" aria-hidden="true" />
                <h3 id={titleId} className="font-semibold">
                  {tString("scanner.title")}
                </h3>
              </div>
              <Button
                isIconOnly
                variant="light"
                size="sm"
                aria-label={tString("scanner.closeAria")}
                onPress={onClose}
              >
                <X className="w-4 h-4" />
              </Button>
            </div>

            {/* Camera View */}
            <div className="relative aspect-square bg-black">
              {hasPermission === null && (
                <div className="absolute inset-0 flex items-center justify-center">
                  <Spinner size="lg" color="white" />
                </div>
              )}

              {hasPermission === false && (
                <div className="absolute inset-0 flex flex-col items-center justify-center p-6 text-center">
                  <Camera className="w-12 h-12 text-gray-400 mb-4" />
                  <h4 className="text-white font-medium mb-2">
                    {tString("scanner.cameraAccessTitle")}
                  </h4>
                  <p className="text-gray-300 text-sm mb-4">
                    {tString("scanner.cameraAccessBody")}
                  </p>
                  <Button color="primary" onPress={startCamera}>
                    {tString("scanner.enableCamera")}
                  </Button>
                </div>
              )}

              {hasPermission === true && (
                <>
                  <video
                    ref={videoRef}
                    className="w-full h-full object-cover"
                    playsInline
                    muted
                  />
                  <canvas ref={canvasRef} className="hidden" />

                  {/* Scanning overlay */}
                  <div className="absolute inset-0 flex items-center justify-center">
                    <div className="w-48 h-48 border-2 border-white border-dashed rounded-lg flex items-center justify-center">
                      <div className="w-40 h-40 border-2 border-primary-500 rounded-lg motion-safe:animate-pulse" />
                    </div>
                  </div>

                  {/* Scanning indicator */}
                  <div className="absolute top-4 left-4 right-4">
                    <div className="bg-black bg-opacity-50 rounded-lg p-3 text-center">
                      <p className="text-white text-sm">
                        {tString("scanner.positionPrompt")}
                      </p>
                    </div>
                  </div>
                </>
              )}
            </div>

            {/* Controls */}
            <div className="p-4 space-y-3">
              {!isScanning && hasPermission !== false && (
                <Button
                  color="primary"
                  fullWidth
                  startContent={<Camera className="w-4 h-4" />}
                  onPress={startCamera}
                >
                  {tString("scanner.startScanning")}
                </Button>
              )}

              <Button variant="bordered" fullWidth onPress={handleManualEntry}>
                {tString("scanner.enterManually")}
              </Button>
            </div>
          </CardBody>
        </Card>
      </div>
    </div>
  );

  if (typeof document === "undefined") return dialog;
  return createPortal(dialog, document.body);
};

export default QRCodeScanner;
