'use client';

import React, { useState } from 'react';
import { useRouter } from 'next/navigation';
import { Card, CardBody, Button, Input } from '@nextui-org/react';
import { QrCode, ArrowRight, AlertCircle } from 'lucide-react';
import QRCodeScanner from '../../components/qr/QRCodeScanner';
import { validateTableCode } from '../../utils/qrValidation';

export default function ScanPage() {
  const router = useRouter();
  const [showScanner, setShowScanner] = useState(false);
  const [manualCode, setManualCode] = useState('');
  const [isValidating, setIsValidating] = useState(false);
  const [error, setError] = useState('');

  const handleScan = async (tableCode: string) => {
    setIsValidating(true);
    setError('');
    
    try {
      const validation = await validateTableCode(tableCode);
      
      if (validation.isValid && validation.tableCode) {
        router.push(`/t/${validation.tableCode}`);
      } else {
        setError(validation.error || 'Invalid table code');
      }
    } catch (error) {
      setError('Failed to validate table code. Please try again.');
    } finally {
      setIsValidating(false);
      setShowScanner(false);
    }
  };

  const handleManualSubmit = async () => {
    if (!manualCode.trim()) {
      setError('Please enter a table code');
      return;
    }
    
    await handleScan(manualCode.trim());
  };

  const handleScanError = (error: string) => {
    setError(error);
    setShowScanner(false);
  };

  return (
    <div className="min-h-screen bg-gradient-to-br from-primary-50 to-secondary-50 flex items-center justify-center p-4">
      <div className="w-full max-w-md">
        <Card className="shadow-xl">
          <CardBody className="p-8 text-center">
            {/* Header */}
            <div className="mb-8">
              <div className="w-16 h-16 bg-primary-100 rounded-full flex items-center justify-center mx-auto mb-4">
                <QrCode className="w-8 h-8 text-primary-600" />
              </div>
              <h1 className="text-2xl font-bold text-gray-900 mb-2">
                Access Your Table
              </h1>
              <p className="text-gray-600">
                Scan the QR code at your table or enter the table code manually
              </p>
            </div>

            {/* Error Message */}
            {error && (
              <div className="mb-6 p-3 bg-danger-50 border border-danger-200 rounded-lg flex items-center gap-2">
                <AlertCircle className="w-4 h-4 text-danger-600 flex-shrink-0" />
                <p className="text-sm text-danger-700">{error}</p>
              </div>
            )}

            {/* QR Scanner Button */}
            <div className="space-y-4 mb-6">
              <Button
                color="primary"
                size="lg"
                fullWidth
                startContent={<QrCode className="w-5 h-5" />}
                onPress={() => setShowScanner(true)}
                isDisabled={isValidating}
              >
                Scan QR Code
              </Button>

              <div className="flex items-center gap-3">
                <div className="flex-1 h-px bg-gray-200" />
                <span className="text-sm text-gray-500">or</span>
                <div className="flex-1 h-px bg-gray-200" />
              </div>

              {/* Manual Entry */}
              <div className="space-y-3">
                <Input
                  placeholder="Enter table code (e.g., ABC123)"
                  value={manualCode}
                  onValueChange={setManualCode}
                  onKeyPress={(e) => {
                    if (e.key === 'Enter') {
                      handleManualSubmit();
                    }
                  }}
                  isDisabled={isValidating}
                  variant="bordered"
                />
                <Button
                  variant="bordered"
                  fullWidth
                  endContent={<ArrowRight className="w-4 h-4" />}
                  onPress={handleManualSubmit}
                  isLoading={isValidating}
                  isDisabled={!manualCode.trim()}
                >
                  Access Table
                </Button>
              </div>
            </div>

            {/* Help Text */}
            <div className="text-sm text-gray-500">
              <p className="mb-2">
                Look for the QR code on your table or ask your server for the table code.
              </p>
              <p>
                Need help? Contact restaurant staff.
              </p>
            </div>
          </CardBody>
        </Card>

        {/* QR Scanner Modal */}
        {showScanner && (
          <QRCodeScanner
            onScan={handleScan}
            onError={handleScanError}
            onClose={() => setShowScanner(false)}
          />
        )}
      </div>
    </div>
  );
}
