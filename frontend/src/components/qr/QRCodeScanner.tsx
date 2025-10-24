'use client';

import React, { useState, useRef, useEffect, useCallback } from 'react';
import { Card, CardBody, Button, Spinner } from '@nextui-org/react';
import { Camera, X, QrCode } from 'lucide-react';

interface QRCodeScannerProps {
  onScan: (tableCode: string) => void;
  onError: (error: string) => void;
  onClose: () => void;
}

export const QRCodeScanner: React.FC<QRCodeScannerProps> = ({
  onScan,
  onError,
  onClose
}) => {
  const videoRef = useRef<HTMLVideoElement>(null);
  const canvasRef = useRef<HTMLCanvasElement>(null);
  const [isScanning, setIsScanning] = useState(false);
  const [hasPermission, setHasPermission] = useState<boolean | null>(null);
  const streamRef = useRef<MediaStream | null>(null);

  const startCamera = async () => {
    try {
      const stream = await navigator.mediaDevices.getUserMedia({
        video: { 
          facingMode: 'environment', // Use back camera
          width: { ideal: 1280 },
          height: { ideal: 720 }
        }
      });
      
      streamRef.current = stream;
      if (videoRef.current) {
        videoRef.current.srcObject = stream;
        videoRef.current.play();
      }
      setHasPermission(true);
      setIsScanning(true);
    } catch (error) {
      console.error('Camera access denied:', error);
      setHasPermission(false);
      onError('Camera access denied. Please allow camera permissions to scan QR codes.');
    }
  };

  const stopCamera = () => {
    if (streamRef.current) {
      streamRef.current.getTracks().forEach(track => track.stop());
      streamRef.current = null;
    }
    setIsScanning(false);
  };

  const scanQRCode = useCallback(() => {
    if (!videoRef.current || !canvasRef.current || !isScanning) return;

    const video = videoRef.current;
    const canvas = canvasRef.current;
    const context = canvas.getContext('2d');

    if (!context || video.videoWidth === 0) return;

    canvas.width = video.videoWidth;
    canvas.height = video.videoHeight;
    context.drawImage(video, 0, 0, canvas.width, canvas.height);

    const imageData = context.getImageData(0, 0, canvas.width, canvas.height);
    
    // Simple QR code detection (in real implementation, use a QR library like jsQR)
    // For now, we'll simulate QR detection by looking for URL patterns
    try {
      // This is a placeholder - in production, use jsQR or similar library
      const qrResult = detectQRCode(imageData);
      if (qrResult) {
        const tableCode = extractTableCode(qrResult);
        if (tableCode) {
          stopCamera();
          onScan(tableCode);
        }
      }
    } catch (error) {
      console.error('QR scan error:', error);
    }
  }, [isScanning, onScan]);

  // Placeholder QR detection - replace with actual QR library
  const detectQRCode = (imageData: ImageData): string | null => {
    // This would be replaced with actual QR detection logic
    // For demo purposes, return null (no QR detected)
    return null;
  };

  const extractTableCode = (qrData: string): string | null => {
    // Extract table code from QR data
    // Expected format: https://payverge.io/t/ABC123 or just ABC123
    const match = qrData.match(/\/t\/([A-Z0-9]+)/) || qrData.match(/^([A-Z0-9]+)$/);
    return match ? match[1] : null;
  };

  useEffect(() => {
    let scanInterval: NodeJS.Timeout;
    
    if (isScanning) {
      scanInterval = setInterval(scanQRCode, 100); // Scan every 100ms
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
    const tableCode = prompt('Enter table code manually:');
    if (tableCode && tableCode.trim()) {
      stopCamera();
      onScan(tableCode.trim().toUpperCase());
    }
  };

  return (
    <div className="fixed inset-0 bg-black bg-opacity-90 z-50 flex items-center justify-center">
      <div className="relative w-full max-w-md mx-4">
        <Card>
          <CardBody className="p-0">
            {/* Header */}
            <div className="flex items-center justify-between p-4 border-b">
              <div className="flex items-center gap-2">
                <QrCode className="w-5 h-5" />
                <h3 className="font-semibold">Scan QR Code</h3>
              </div>
              <Button
                isIconOnly
                variant="light"
                size="sm"
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
                  <h4 className="text-white font-medium mb-2">Camera Access Required</h4>
                  <p className="text-gray-300 text-sm mb-4">
                    Please allow camera access to scan QR codes
                  </p>
                  <Button color="primary" onPress={startCamera}>
                    Enable Camera
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
                  <canvas
                    ref={canvasRef}
                    className="hidden"
                  />
                  
                  {/* Scanning overlay */}
                  <div className="absolute inset-0 flex items-center justify-center">
                    <div className="w-48 h-48 border-2 border-white border-dashed rounded-lg flex items-center justify-center">
                      <div className="w-40 h-40 border-2 border-primary-500 rounded-lg animate-pulse" />
                    </div>
                  </div>

                  {/* Scanning indicator */}
                  <div className="absolute top-4 left-4 right-4">
                    <div className="bg-black bg-opacity-50 rounded-lg p-3 text-center">
                      <p className="text-white text-sm">
                        Position QR code within the frame
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
                  Start Scanning
                </Button>
              )}

              <Button
                variant="bordered"
                fullWidth
                onPress={handleManualEntry}
              >
                Enter Code Manually
              </Button>
            </div>
          </CardBody>
        </Card>
      </div>
    </div>
  );
};

export default QRCodeScanner;
