/* eslint-disable no-restricted-syntax -- QRCode library requires hex color strings */
"use client";

import React, { useState, useEffect, useCallback, useMemo } from "react";
import {
  Input,
  Button,
  Tooltip,
  Modal,
  ModalContent,
  ModalHeader,
  ModalBody,
  ModalFooter,
  useDisclosure,
} from "@nextui-org/react";
import { Check, X, Loader2, AlertCircle, Copy, QrCode, ExternalLink, Download } from "lucide-react";
import { checkCustomURLAvailability } from "@/api/business";
import { debounce } from "lodash";
import {
  useSimpleLocale,
  getTranslation,
} from "@/i18n/SimpleTranslationProvider";
import Image from "next/image";
import { useToast } from "@/contexts/ToastContext";
import { useSiteUrl } from "@/hooks/useSiteUrl";

/** First-segment tokens that must never be claimed as a public /b/<slug>. */
const RESERVED_PUBLIC_SLUG_PREFIXES = [
  "admin",
  "api",
  "demo",
  "b",
  "internal",
] as const;

/** True when slug equals a reserved token or starts with reserved- (Finding 35). */
export function isReservedPublicSlug(slug: string): boolean {
  const s = slug.trim().toLowerCase();
  if (!s) return false;
  return RESERVED_PUBLIC_SLUG_PREFIXES.some((p) => s === p || s.startsWith(`${p}-`));
}

interface CustomURLInputProps {
  value: string;
  onChange: (value: string) => void;
  businessId?: number;
  placeholder?: string;
  label?: string;
  description?: string;
  /** The currently-persisted slug — drives the always-visible 'Your live page' zone. */
  savedUrl?: string;
  /** Lifts the availability-check result to the parent so save can block a known-taken slug. */
  onAvailabilityChange?: (state: { checked: boolean; available: boolean | null }) => void;
}

const CustomURLInput = React.memo(function CustomURLInput({
  value,
  onChange,
  businessId,
  placeholder,
  label,
  description,
  savedUrl,
  onAvailabilityChange,
}: CustomURLInputProps) {
  const { locale: currentLocale } = useSimpleLocale();

  // Translation helper
  const tString = useCallback(
    (key: string): string => {
      const fullKey = `businessSettings.customUrlInput.${key}`;
      const result = getTranslation(fullKey, currentLocale);
      return Array.isArray(result) ? result[0] || key : (result as string);
    },
    [currentLocale],
  );

  // Get translated default values
  const translatedPlaceholder = placeholder || tString("defaultPlaceholder");
  const translatedLabel = label || tString("defaultLabel");
  const translatedDescription = description || tString("defaultDescription");

  // Storefront links live on this deployment's origin (runtime PUBLIC_URL).
  // The hook hydrates with the server's value, so an unset PUBLIC_URL (page
  // origin in the browser) does not mismatch.
  const siteUrl = useSiteUrl();
  const siteHost = siteUrl.replace(/^https?:\/\//, "");
  const liveUrl = savedUrl && savedUrl.length >= 2 ? `${siteUrl}/b/${savedUrl}` : "";

  // The typed value is the business's own already-saved slug — it's the CURRENT
  // URL, not a candidate to check for availability.
  const isCurrentUrl = Boolean(savedUrl && value === savedUrl && value.length >= 2);

  const [isChecking, setIsChecking] = useState(false);
  const [isAvailable, setIsAvailable] = useState<boolean | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [hasChecked, setHasChecked] = useState(false);
  const [qrCodeDataUrl, setQrCodeDataUrl] = useState<string>("");
  // The slug the QR modal is currently showing — set at generate time so the
  // modal caption matches the button that opened it (typed vs saved slug).
  const [qrSlug, setQrSlug] = useState<string>("");
  const { isOpen, onOpen, onClose } = useDisclosure();
  const { showSuccess, showError } = useToast();

  // Function to check URL availability
  const checkURL = useCallback(
    async (url: string) => {
      if (!url || url.length < 2) {
        setIsAvailable(null);
        setError(null);
        setHasChecked(false);
        setIsChecking(false);
        onAvailabilityChange?.({ checked: false, available: null });
        return;
      }

      // Reject reserved first-segment prefixes client-side (Finding 35) so we
      // never suggest /b/demo-*, /b/admin-*, etc. Backend validateCustomURL
      // enforces the same list.
      if (isReservedPublicSlug(url)) {
        setIsChecking(false);
        setIsAvailable(false);
        setError(tString("validation.reserved"));
        setHasChecked(true);
        onAvailabilityChange?.({ checked: true, available: false });
        return;
      }

      try {
        setIsChecking(true);
        setError(null);

        const result = await checkCustomURLAvailability(url, businessId);

        setIsAvailable(result.available);
        setError(result.error || null);
        setHasChecked(true);
        onAvailabilityChange?.({ checked: true, available: result.available });
      } catch (err) {
        setError(tString("errors.checkFailed"));
        setIsAvailable(false);
        setHasChecked(true);
        onAvailabilityChange?.({ checked: true, available: false });
      } finally {
        setIsChecking(false);
      }
    },
    [businessId, tString, onAvailabilityChange],
  );

  // Debounced function to check URL availability
  const debouncedCheck = useMemo(
    () =>
      debounce((url: string) => {
        void checkURL(url);
      }, 500),
    [checkURL],
  );

  // Copy a specific slug's URL to clipboard. The input's endContent passes the
  // just-typed `value`; the "Your live page" zone passes the persisted
  // `savedUrl`. Previously both copied `savedUrl || value` — so copying next to
  // a newly typed available slug grabbed the OLD saved URL.
  const copySlug = useCallback(
    async (slug: string) => {
      if (!slug || slug.length < 2) return;
      const fullUrl = `${siteUrl}/b/${slug}`;
      try {
        await navigator.clipboard.writeText(fullUrl);
        showSuccess(tString("copySuccess"));
      } catch (err) {
        console.error("Failed to copy URL:", err);
        const msg = tString("errors.copyFailed");
        showError(
          msg.endsWith("errors.copyFailed") ? "Couldn't copy the link" : msg,
        );
      }
    },
    [showSuccess, showError, tString, siteUrl],
  );

  // Generate QR Code for a specific slug (typed vs saved).
  const generateQRCodeFor = useCallback(
    async (slug: string) => {
      if (!slug || slug.length < 2) return;

      const fullUrl = `${siteUrl}/b/${slug}`;
      try {
        // Dynamic import with proper typing
        const QRCode = (await import("qrcode" as any)).default as any;

        const qrDataUrl = await QRCode.toDataURL(fullUrl, {
          width: 256,
          margin: 2,
          color: {
            "dark": "#000000",
            light: "#FFFFFF",
          },
        });
        setQrCodeDataUrl(qrDataUrl);
        setQrSlug(slug);
        onOpen();
      } catch (err) {
        console.error("Failed to generate QR code:", err);
      }
    },
    [onOpen, siteUrl],
  );

  // Download QR Code as PNG (uses the slug the modal was opened with).
  const downloadQRCode = useCallback(() => {
    if (!qrCodeDataUrl) return;
    const a = document.createElement("a");
    a.href = qrCodeDataUrl;
    a.download = `${qrSlug || "page"}-qr.png`;
    document.body.appendChild(a);
    a.click();
    document.body.removeChild(a);
  }, [qrCodeDataUrl, qrSlug]);

  // Effect to check URL when value changes
  useEffect(() => {
    // The own already-saved slug is the current URL — never spend an
    // availability call on it, and clear any stale availability state.
    if (isCurrentUrl) {
      debouncedCheck.cancel();
      setIsChecking(false);
      setIsAvailable(null);
      setError(null);
      setHasChecked(false);
      onAvailabilityChange?.({ checked: false, available: null });
      return;
    }

    if (value && value.length >= 2) {
      debouncedCheck(value);
    }

    // Cleanup function to cancel pending debounced calls
    return () => {
      debouncedCheck.cancel();
    };
  }, [value, debouncedCheck, isCurrentUrl, onAvailabilityChange]);

  // Handle input change
  const handleChange = (newValue: string) => {
    // Clean the input - only allow alphanumeric and hyphens
    const cleanValue = newValue.toLowerCase().replace(/[^a-z0-9-]/g, "");
    onChange(cleanValue);

    // Reset states when user is typing
    if (newValue !== value) {
      setHasChecked(false);
      setIsAvailable(null);
      setError(null);
    }
  };

  // Get the appropriate end content (icon + action buttons)
  const getEndContent = () => {
    if (isChecking) {
      return <Loader2 className="w-4 h-4 animate-spin text-ink-400" />;
    }

    if (!hasChecked || !value || value.length < 2) {
      return null;
    }

    if (error || !isAvailable) {
      return <X className="w-4 h-4 text-rose-600" />;
    }

    if (isAvailable) {
      return (
        <div className="flex items-center gap-1">
          <Check className="w-4 h-4 text-emerald-600" />
          <Tooltip content={tString("tooltips.copyUrl")}>
            <Button
              isIconOnly
              size="sm"
              variant="light"
              aria-label={tString("tooltips.copyUrl")}
              onPress={() => copySlug(value)}
              className="min-w-unit-6 w-6 h-6"
            >
              <Copy className="w-3 h-3" />
            </Button>
          </Tooltip>
          <Tooltip content={tString("tooltips.generateQr")}>
            <Button
              isIconOnly
              size="sm"
              variant="light"
              aria-label={tString("tooltips.generateQr")}
              onPress={() => generateQRCodeFor(value)}
              className="min-w-unit-6 w-6 h-6"
            >
              <QrCode className="w-3 h-3" />
            </Button>
          </Tooltip>
        </div>
      );
    }

    return null;
  };

  // Get input color based on validation state
  const getColor = () => {
    if (isCurrentUrl) {
      return "success";
    }

    if (!hasChecked || !value || value.length < 2) {
      return "default";
    }

    if (error || !isAvailable) {
      return "danger";
    }

    if (isAvailable) {
      return "success";
    }

    return "default";
  };

  // Get helper text (remove URL from here since we show it in input)
  const getHelperText = () => {
    if (isCurrentUrl) {
      return tString("status.currentUrl");
    }

    if (isChecking) {
      return tString("status.checking");
    }

    if (value && value.length < 2) {
      return tString("validation.minLength");
    }

    if (error) {
      return error;
    }

    if (hasChecked && isAvailable) {
      return tString("status.available").replace("{url}", "");
    }

    if (hasChecked && !isAvailable) {
      return tString("status.taken");
    }

    return translatedDescription;
  };

  // Get helper text color
  const getHelperColor = () => {
    if (isCurrentUrl) {
      return "text-emerald-600";
    }

    if (isChecking) {
      return "text-ink-500";
    }

    if (value && value.length < 2) {
      return "text-amber-600";
    }

    if (error || (hasChecked && !isAvailable)) {
      return "text-rose-600";
    }

    if (hasChecked && isAvailable) {
      return "text-emerald-600";
    }

    return "text-ink-500";
  };

  return (
    <div className="space-y-2">
      {liveUrl && (
        <div className="flex flex-wrap items-center gap-2 rounded-2xl border border-warm-200 bg-warm-50/70 px-3 py-2 shadow-sm shadow-warm-900/5">
          <span className="text-xs font-medium text-ink-500">{tString("yourLivePage")}</span>
          <span className="text-sm font-mono text-ink-800">{liveUrl}</span>
          <div className="ml-auto flex items-center gap-1">
            <a
              href={liveUrl}
              target="_blank"
              rel="noopener noreferrer"
              title={tString("tooltips.openLivePage")}
              className="inline-flex h-7 items-center gap-1 rounded-xl px-2 text-xs font-semibold text-brand hover:bg-white"
            >
              <ExternalLink className="w-3.5 h-3.5" aria-hidden="true" />
              {tString("openLivePage")}
            </a>
            <Button isIconOnly size="sm" variant="light" aria-label={tString("tooltips.copyUrl")} onPress={() => savedUrl && copySlug(savedUrl)} className="min-w-unit-7 w-7 h-7">
              <Copy className="w-3.5 h-3.5" />
            </Button>
            <Button isIconOnly size="sm" variant="light" aria-label={tString("tooltips.generateQr")} onPress={() => savedUrl && generateQRCodeFor(savedUrl)} className="min-w-unit-7 w-7 h-7">
              <QrCode className="w-3.5 h-3.5" />
            </Button>
          </div>
        </div>
      )}
      <Input
        label={translatedLabel}
        placeholder={translatedPlaceholder}
        value={value}
        onValueChange={handleChange}
        variant="bordered"
        color={getColor() as any}
        startContent={
          <div className="flex items-center gap-2 text-ink-500">
            <span className="text-sm">{siteHost}/b/</span>
          </div>
        }
        endContent={getEndContent()}
        classNames={{
          input: "text-sm",
          inputWrapper: "h-12",
        }}
      />

      <div
        className={`text-xs ${getHelperColor()} flex items-center gap-1`}
        role="status"
        aria-live="polite"
      >
        {value && value.length < 2 && <AlertCircle className="w-3 h-3" />}
        <span>{getHelperText()}</span>
      </div>

      {/* QR Code Modal */}
      <Modal isOpen={isOpen} onClose={onClose} size="sm">
        <ModalContent className="overflow-hidden rounded-3xl border border-warm-200 bg-white shadow-2xl shadow-warm-900/15">
          {(onClose) => (
            <>
              <ModalHeader className="flex flex-col gap-1 border-b border-warm-200/80 bg-warm-50/70 px-6 py-5 text-lg font-semibold text-ink-950">
                {tString("qrModal.title")}
              </ModalHeader>
              <ModalBody className="flex flex-col items-center px-6 py-5">
                {qrCodeDataUrl && (
                  <>
                    <Image
                      src={qrCodeDataUrl}
                      alt="QR Code"
                      width={192}
                      height={192}
                      className="w-48 h-48 rounded-2xl border border-warm-200"
                    />
                    <p className="mt-2 text-center text-sm text-ink-600">
                      {tString("qrModal.description")}
                    </p>
                    <p className="text-center font-mono text-xs text-ink-500">
                      {siteUrl}/b/{qrSlug}
                    </p>
                  </>
                )}
              </ModalBody>
              <ModalFooter className="border-t border-warm-200/80 bg-warm-50/70 px-6 py-4">
                <Button
                  variant="flat"
                  startContent={<Download className="w-4 h-4" />}
                  onPress={downloadQRCode}
                  className="bg-brand/10 font-semibold text-brand-dark hover:bg-brand/15"
                >
                  {tString("qrModal.download")}
                </Button>
                <Button
                  onPress={onClose}
                  className="bg-brand font-semibold text-white shadow-sm shadow-brand/20 hover:bg-brand-dark"
                >
                  {tString("qrModal.close")}
                </Button>
              </ModalFooter>
            </>
          )}
        </ModalContent>
      </Modal>
    </div>
  );
});

export default CustomURLInput;
