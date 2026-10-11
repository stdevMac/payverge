"use client";

import React, { useEffect, useRef, useState } from "react";
import { useRouter } from "next/navigation";
import { Card, CardBody, Button, Input } from "@nextui-org/react";
import { QrCode, ArrowRight, AlertCircle } from "lucide-react";
import QRCodeScanner from "../../components/qr/QRCodeScanner";
import { validateTableCode } from "../../utils/qrValidation";
import { useGuestTranslation } from "@/i18n/GuestTranslationProvider";

const TABLE_CODE_INPUT_ID = "scan-table-code";
const TABLE_CODE_ERROR_ID = "scan-table-code-error";

export default function ScanClient() {
  const router = useRouter();
  const { t: gt } = useGuestTranslation();
  const t = (key: string): string => gt(`scan.${key}`);

  const [showScanner, setShowScanner] = useState(false);
  const [manualCode, setManualCode] = useState("");
  const [isValidating, setIsValidating] = useState(false);
  const [error, setError] = useState("");
  const codeInputRef = useRef<HTMLInputElement>(null);

  // After an async lookup the submit control can unmount/disable and drop
  // focus to <body>. Return it to the code field once the error is painted.
  useEffect(() => {
    if (!error || isValidating) return;
    codeInputRef.current?.focus();
  }, [error, isValidating]);

  const handleScan = async (tableCode: string) => {
    setIsValidating(true);
    setError("");

    try {
      const validation = await validateTableCode(tableCode);

      if (validation.isValid && validation.tableCode) {
        router.push(`/t/${validation.tableCode}`);
      } else {
        // Map the locale-independent reason code to translated copy — never
        // show the English `error` string on this 21-locale entry screen.
        const errorKey =
          validation.errorCode === "notFound"
            ? "errors.notFound"
            : validation.errorCode === "inactive"
              ? "errors.inactive"
              : validation.errorCode === "failed"
                ? "errors.validationFailed"
                : "errors.invalidCode";
        setError(t(errorKey));
      }
    } catch (error) {
      setError(t("errors.validationFailed"));
    } finally {
      setIsValidating(false);
      setShowScanner(false);
    }
  };

  const handleManualSubmit = async () => {
    if (!manualCode.trim()) {
      setError(t("errors.emptyCode"));
      return;
    }

    await handleScan(manualCode.trim());
  };

  const handleScanError = (error: string) => {
    setError(error);
    setShowScanner(false);
  };

  const hasLookupError = Boolean(error);

  return (
    <div className="min-h-screen bg-warm-50 flex items-center justify-center p-4">
      <div className="w-full max-w-md">
        <Card className="border border-warm-200 shadow-xl">
          <CardBody className="p-8 text-center">
            {/* Header */}
            <div className="mb-8">
              <div className="w-16 h-16 bg-brand/10 rounded-full flex items-center justify-center mx-auto mb-4">
                <QrCode className="w-8 h-8 text-brand" />
              </div>
              <h1 className="font-title text-3xl text-ink-950 mb-2">
                {t("header.title")}
              </h1>
              <p className="text-ink-600">{t("header.subtitle")}</p>
            </div>

            {/* Error Message */}
            {hasLookupError && (
              <div className="mb-6 p-3 bg-danger-50 border border-danger-200 rounded-lg flex items-center gap-2">
                <AlertCircle
                  className="w-4 h-4 text-danger-600 flex-shrink-0"
                  aria-hidden
                />
                <p
                  id={TABLE_CODE_ERROR_ID}
                  role="alert"
                  aria-live="assertive"
                  className="text-sm text-danger-700"
                >
                  {error}
                </p>
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
                {t("actions.scanQr")}
              </Button>

              <div className="flex items-center gap-3">
                <div className="flex-1 h-px bg-warm-200" />
                <span className="text-sm text-ink-500">{t("actions.or")}</span>
                <div className="flex-1 h-px bg-warm-200" />
              </div>

              {/* Manual Entry */}
              <div className="space-y-3">
                <Input
                  id={TABLE_CODE_INPUT_ID}
                  ref={codeInputRef}
                  placeholder={t("actions.manualPlaceholder")}
                  value={manualCode}
                  onValueChange={setManualCode}
                  onKeyDown={(e) => {
                    if (e.key === "Enter") {
                      void handleManualSubmit();
                    }
                  }}
                  isDisabled={isValidating}
                  isInvalid={hasLookupError}
                  aria-invalid={hasLookupError}
                  aria-describedby={
                    hasLookupError ? TABLE_CODE_ERROR_ID : undefined
                  }
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
                  {t("actions.accessTable")}
                </Button>
              </div>
            </div>

            {/* Help Text */}
            <div className="text-sm text-ink-500">
              <p className="mb-2">{t("help.lookFor")}</p>
              <p>{t("help.needHelp")}</p>
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
