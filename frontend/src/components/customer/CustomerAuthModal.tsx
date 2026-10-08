"use client";

import React, { useState, useCallback, useEffect, useRef } from "react";
import {
  Modal,
  ModalContent,
  ModalHeader,
  ModalBody,
  ModalFooter,
  Button,
  Input,
  Divider,
} from "@nextui-org/react";
import { Mail, User } from "lucide-react";
import { useCustomerAuth } from "@/contexts/CustomerAuthContext";
import type { Customer } from "@/api/crm";
import PasswordField from "@/components/auth/PasswordField";
import { guestPaymentToastMessage } from "@/components/guest/guestPaymentToast";
import { useGuestTranslation } from "@/i18n/GuestTranslationProvider";
import { isValidEmail } from "@/lib/fieldValidation";
import { GuestDemoPersonalDataNotice } from "@/components/demo/DemoPersonalDataNotice";

interface CustomerAuthModalProps {
  isOpen: boolean;
  onClose: () => void;
  onSuccess?: (
    customerId: number,
    customerData: Customer,
  ) => void | Promise<void>;
  businessName?: string;
  defaultMode?: "login" | "register";
}

export default function CustomerAuthModal({
  isOpen,
  onClose,
  onSuccess,
  businessName,
  defaultMode = "login",
}: CustomerAuthModalProps) {
  const { t: guestT } = useGuestTranslation();
  const { login, register } = useCustomerAuth();
  const [mode, setMode] = useState<"login" | "register">(defaultMode);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState("");
  const formRef = useRef<HTMLFormElement>(null);
  const emailRef = useRef<HTMLInputElement>(null);
  const passwordRef = useRef<HTMLInputElement>(null);

  // Form state
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [name, setName] = useState("");

  // Update mode when defaultMode changes
  useEffect(() => {
    setMode(defaultMode);
  }, [defaultMode]);

  const clearNativeValidity = useCallback(() => {
    const form = formRef.current;
    if (!form) return;
    for (const el of Array.from(form.querySelectorAll("input"))) {
      el.setCustomValidity("");
    }
  }, []);

  // Mode switches (tab or defaultMode) must drop leftover constraint state so
  // an empty sign-in submit cannot paint a native bubble on unused signup fields.
  useEffect(() => {
    setError("");
    clearNativeValidity();
  }, [mode, clearNativeValidity]);

  const t = useCallback(
    (key: string): string => {
      return guestT(`customerAuth.${mode}.${key}`);
    },
    [guestT, mode],
  );

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    setError("");

    const normalizedEmail = email.trim().toLowerCase();
    const trimmedName = name.trim();
    if (mode === "register" && trimmedName.length === 0) {
      setError(t("nameRequired"));
      return;
    }
    if (!normalizedEmail) {
      setError(guestT("customerAuth.validation.emailRequired"));
      emailRef.current?.focus();
      return;
    }
    if (!isValidEmail(normalizedEmail)) {
      setError(guestT("customerAuth.validation.emailInvalid"));
      emailRef.current?.focus();
      return;
    }
    if (password.length === 0) {
      setError(guestT("customerAuth.validation.passwordRequired"));
      passwordRef.current?.focus();
      return;
    }
    if (mode === "register" && password.length < 8) {
      setError(t("passwordPlaceholder"));
      passwordRef.current?.focus();
      return;
    }

    setLoading(true);
    try {
      const resultCustomer =
        mode === "register"
          ? await register(normalizedEmail, password, trimmedName)
          : await login(normalizedEmail, password);

      if (onSuccess) {
        await onSuccess(resultCustomer.id, resultCustomer);
      }

      onClose();
    } catch (err: unknown) {
      // Use guestT (full key paths) — local `t` prefixes customerAuth.${mode}.
      setError(guestPaymentToastMessage(err, guestT) || t("errorOccurred"));
    } finally {
      setLoading(false);
    }
  };

  const switchMode = () => {
    setMode(mode === "login" ? "register" : "login");
    setError("");
    clearNativeValidity();
  };

  const resetAndClose = () => {
    setEmail("");
    setPassword("");
    setName("");
    setError("");
    setMode(defaultMode);
    onClose();
  };

  return (
    <Modal
      isOpen={isOpen}
      onClose={resetAndClose}
      size="md"
      classNames={{
        base: "bg-white",
        backdrop: "bg-black/50 backdrop-blur-sm",
      }}
    >
      <ModalContent>
        <form ref={formRef} noValidate onSubmit={handleSubmit}>
          <ModalHeader className="flex flex-col gap-1">
            <h2 className="text-2xl font-light text-gray-900 tracking-wide">
              {t("title")}
            </h2>
            <p className="text-sm text-gray-600 font-light">{t("subtitle")}</p>
          </ModalHeader>

          <ModalBody className="space-y-4">
            <GuestDemoPersonalDataNotice />
            {/* Show business name if provided */}
            {businessName && mode === "register" && (
              <div className="p-3 bg-green-50 rounded-lg border border-green-200">
                <p className="text-sm text-green-800">
                  {t("joinRewards")} <strong>{businessName}</strong>
                </p>
              </div>
            )}

            {/* Error message */}
            {error && (
              <div
                role="alert"
                className="p-3 bg-red-50 rounded-lg border border-red-200"
              >
                <p className="text-sm text-red-600">{error}</p>
              </div>
            )}

            {/* Name field (register only) */}
            {mode === "register" && (
              <Input
                label={t("name")}
                placeholder={t("namePlaceholder")}
                value={name}
                onValueChange={setName}
                autoComplete="name"
                startContent={<User size={16} className="text-gray-400" />}
                aria-required="true"
                classNames={{
                  input: "font-light",
                  label: "font-light",
                }}
              />
            )}

            {/* Email field */}
            <Input
              ref={emailRef}
              key={`email-${mode}`}
              type="email"
              dir="ltr"
              label={t("email")}
              placeholder={t("emailPlaceholder")}
              value={email}
              onValueChange={setEmail}
              autoComplete="email"
              startContent={<Mail size={16} className="text-gray-400" />}
              aria-required="true"
              classNames={{
                input: "font-light",
                label: "font-light",
              }}
            />

            {/* Password field */}
            <PasswordField
              key={`password-${mode}`}
              label={t("password")}
              placeholder={t("passwordPlaceholder")}
              value={password}
              onChange={(event) => setPassword(event.target.value)}
              autoComplete={
                mode === "register" ? "new-password" : "current-password"
              }
              showLabel={t("showPassword")}
              hideLabel={t("hidePassword")}
              inputRef={passwordRef}
              required={false}
              minimumLength={mode === "register" ? 8 : undefined}
              minimumLengthLabel={
                mode === "register" ? t("passwordPlaceholder") : undefined
              }
            />

            <Divider />

            {/* Switch mode link */}
            <div className="text-center text-sm">
              <span className="text-gray-600">
                {mode === "login" ? t("noAccount") : t("hasAccount")}{" "}
              </span>
              <button
                type="button"
                onClick={switchMode}
                className="text-gray-900 font-medium hover:underline"
              >
                {mode === "login" ? t("register") : t("login")}
              </button>
            </div>
          </ModalBody>

          <ModalFooter>
            <Button variant="light" onPress={resetAndClose}>
              {t("cancel")}
            </Button>
            <Button
              type="submit"
              className="bg-brand text-white font-medium hover:bg-brand-dark"
              isLoading={loading}
            >
              {loading
                ? t(mode === "register" ? "creating" : "signingIn")
                : t("button")}
            </Button>
          </ModalFooter>
        </form>
      </ModalContent>
    </Modal>
  );
}
