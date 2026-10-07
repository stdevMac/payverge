"use client";

import React, { useEffect, useState } from "react";
import {
  Modal,
  ModalBody,
  ModalContent,
  ModalFooter,
  ModalHeader,
  Button,
} from "@nextui-org/react";
import { ShieldAlert } from "lucide-react";
import {
  useSimpleLocale,
  getTranslation,
} from "@/i18n/SimpleTranslationProvider";

/**
 * ManagerPinModal — numeric-keypad PIN entry surfaced before destructive
 * bill operations (comps, voids, refunds). The component is intentionally
 * dumb: it owns nothing beyond the digit buffer, and defers the actual
 * verification (and any retry / lockout policy) to the caller via the
 * `onSubmit(pin)` callback. The caller resolves a Promise — a rejection
 * with `message` keeps the modal open and shows the message inline; a
 * resolution closes the modal.
 */

export interface ManagerPinModalProps {
  isOpen: boolean;
  /** Heading shown above the keypad. Defaults to the localized "Manager PIN required". */
  title?: string;
  /** Short description of why the PIN is required (e.g. "Voiding burger x2"). */
  description?: string;
  /** Min/max digit constraints — kept loose by default to match the backend. */
  minLength?: number;
  maxLength?: number;
  /**
   * Called when the user submits the PIN. Resolve to close the modal,
   * reject with `{ message: string }` to surface an inline error and keep
   * the modal open (typical for `pin_invalid`).
   */
  onSubmit: (pin: string) => Promise<void>;
  /** Called when the user closes/cancels the modal. */
  onCancel: () => void;
}

const KEYS: string[] = [
  "1",
  "2",
  "3",
  "4",
  "5",
  "6",
  "7",
  "8",
  "9",
  "clear",
  "0",
  "back",
];

export default function ManagerPinModal({
  isOpen,
  title,
  description,
  minLength = 4,
  maxLength = 6,
  onSubmit,
  onCancel,
}: ManagerPinModalProps) {
  const { locale } = useSimpleLocale();
  const t = React.useCallback(
    (key: string, params?: Record<string, string | number>): string => {
      const result = getTranslation(`managerPin.${key}`, locale, params);
      return Array.isArray(result) ? result[0] || key : (result as string);
    },
    [locale],
  );
  // Callers may pass an already-translated title (e.g. "Confirm void"); fall
  // back to the localized default when they don't.
  const resolvedTitle = title ?? t("title");

  const [pin, setPin] = useState("");
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);

  // Reset state whenever the modal toggles open — this guards against stale
  // PIN digits leaking across two consecutive prompts (e.g. void → comp).
  useEffect(() => {
    if (isOpen) {
      setPin("");
      setError(null);
      setSubmitting(false);
    }
  }, [isOpen]);

  const tooShort = pin.length < minLength;

  const handleAppend = React.useCallback(
    (digit: string) => {
      if (submitting) return;
      if (pin.length >= maxLength) return;
      setError(null);
      setPin((prev) => prev + digit);
    },
    [maxLength, pin.length, submitting],
  );

  const handleBackspace = React.useCallback(() => {
    if (submitting) return;
    setError(null);
    setPin((prev) => prev.slice(0, -1));
  }, [submitting]);

  const handleClear = () => {
    if (submitting) return;
    setError(null);
    setPin("");
  };

  const handleConfirm = React.useCallback(
    async () => {
      if (submitting || tooShort) return;
      setSubmitting(true);
      setError(null);
      try {
        await onSubmit(pin);
        setPin("");
      } catch (err: unknown) {
        let message = t("invalidPin");
        if (err && typeof err === "object" && "message" in err) {
          const candidate = (err as { message?: unknown }).message;
          if (typeof candidate === "string" && candidate.trim() !== "") {
            message = candidate;
          }
        } else if (typeof err === "string" && err.trim() !== "") {
          message = err;
        }
        setError(message);
      } finally {
        setSubmitting(false);
      }
    },
    [onSubmit, pin, submitting, t, tooShort],
  );

  // Hardware-keyboard support: on a desktop or a tablet with a keyboard, let
  // operators type the PIN (digits), Backspace to delete, Enter to confirm
  // when long enough, and Escape to cancel — instead of mousing each on-screen
  // key. The on-screen keypad still works for touch-only devices.
  useEffect(() => {
    if (!isOpen) return;
    const onKeyDown = (e: KeyboardEvent) => {
      if (submitting) return;
      if (e.key >= "0" && e.key <= "9" && e.key.length === 1) {
        e.preventDefault();
        handleAppend(e.key);
      } else if (e.key === "Backspace") {
        e.preventDefault();
        handleBackspace();
      } else if (e.key === "Enter") {
        e.preventDefault();
        if (!tooShort) void handleConfirm();
      } else if (e.key === "Escape") {
        e.preventDefault();
        onCancel();
      }
    };
    window.addEventListener("keydown", onKeyDown);
    return () => window.removeEventListener("keydown", onKeyDown);
  }, [
    handleAppend,
    handleBackspace,
    handleConfirm,
    isOpen,
    onCancel,
    submitting,
    tooShort,
  ]);

  return (
    <Modal
      isOpen={isOpen}
      onClose={onCancel}
      size="sm"
      placement="center"
      isDismissable={!submitting}
      hideCloseButton={submitting}
    >
      <ModalContent>
        <>
          <ModalHeader className="flex items-center gap-2">
            <ShieldAlert className="h-5 w-5 text-warning" aria-hidden="true" />
            {resolvedTitle}
          </ModalHeader>
          <ModalBody className="space-y-4">
            {description ? (
              <p className="text-sm text-ink-600">{description}</p>
            ) : null}

            <div
              className="flex items-center justify-center gap-3"
              aria-label={t("aria.display")}
            >
              {Array.from({ length: maxLength }).map((_, idx) => {
                const filled = idx < pin.length;
                return (
                  <span
                    key={idx}
                    className={`h-4 w-4 rounded-full border ${
                      filled
                        ? "border-ink-900 bg-ink-900"
                        : "border-warm-300 bg-transparent"
                    }`}
                  />
                );
              })}
            </div>

            {error ? (
              <p
                className="text-center text-sm font-medium text-danger"
                role="alert"
              >
                {error}
              </p>
            ) : null}

            <div className="grid grid-cols-3 gap-2">
              {KEYS.map((key) => {
                if (key === "clear") {
                  return (
                    <Button
                      key="clear"
                      variant="flat"
                      onPress={handleClear}
                      isDisabled={submitting || pin.length === 0}
                      aria-label={t("aria.clear")}
                    >
                      {t("clear")}
                    </Button>
                  );
                }
                if (key === "back") {
                  return (
                    <Button
                      key="back"
                      variant="flat"
                      onPress={handleBackspace}
                      isDisabled={submitting || pin.length === 0}
                      aria-label={t("aria.backspace")}
                    >
                      ⌫
                    </Button>
                  );
                }
                return (
                  <Button
                    key={key}
                    variant="bordered"
                    onPress={() => handleAppend(key)}
                    isDisabled={submitting || pin.length >= maxLength}
                    className="h-12 text-lg font-semibold"
                    aria-label={t("aria.digit", { digit: key })}
                  >
                    {key}
                  </Button>
                );
              })}
            </div>
          </ModalBody>
          <ModalFooter>
            <Button
              variant="light"
              onPress={onCancel}
              isDisabled={submitting}
            >
              {t("cancel")}
            </Button>
            <Button
              color="primary"
              onPress={handleConfirm}
              isDisabled={tooShort || submitting}
              isLoading={submitting}
            >
              {t("confirm")}
            </Button>
          </ModalFooter>
        </>
      </ModalContent>
    </Modal>
  );
}
