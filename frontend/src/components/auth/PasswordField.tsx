"use client";

import type { ChangeEvent, Ref } from "react";
import { useState } from "react";
import { Eye, EyeOff, Lock } from "lucide-react";
import { AccessibleInput } from "@/components/ui/AccessibleInput";

interface PasswordFieldProps {
  label: string;
  value: string;
  onChange: (event: ChangeEvent<HTMLInputElement>) => void;
  autoComplete: "current-password" | "new-password";
  showLabel: string;
  hideLabel: string;
  placeholder?: string;
  minimumLength?: number;
  minimumLengthLabel?: string;
  inputRef?: Ref<HTMLInputElement>;
  isInvalid?: boolean;
  errorMessage?: string;
  required?: boolean;
}

export default function PasswordField({
  label,
  value,
  onChange,
  autoComplete,
  showLabel,
  hideLabel,
  placeholder,
  minimumLength,
  minimumLengthLabel,
  inputRef,
  isInvalid = false,
  errorMessage,
  required = true,
}: PasswordFieldProps) {
  const [visible, setVisible] = useState(false);
  const minimumMet = minimumLength ? value.length >= minimumLength : false;

  return (
    <div>
      <AccessibleInput
        ref={inputRef}
        type={visible ? "text" : "password"}
        label={label}
        placeholder={placeholder}
        value={value}
        onChange={onChange}
        startContent={<Lock aria-hidden="true" className="h-5 w-5 text-ink-400" />}
        autoComplete={autoComplete}
        minLength={minimumLength}
        required={required}
        isInvalid={isInvalid}
        errorMessage={errorMessage}
        endContent={
          <button
            type="button"
            onClick={() => setVisible((current) => !current)}
            aria-label={visible ? hideLabel : showLabel}
            aria-pressed={visible}
            className="rounded-sm text-ink-400 hover:text-ink-700 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand focus-visible:ring-offset-1"
          >
            {visible ? (
              <EyeOff aria-hidden="true" className="h-5 w-5" />
            ) : (
              <Eye aria-hidden="true" className="h-5 w-5" />
            )}
          </button>
        }
        classNames={{
          inputWrapper:
            "border-2 border-warm-200 bg-white hover:border-warm-300",
        }}
      />
      {minimumLength && minimumLengthLabel ? (
        <p
          role="status"
          aria-live="polite"
          className={`mt-1.5 text-xs ${minimumMet ? "text-emerald-700" : "text-ink-500"}`}
        >
          <span aria-hidden="true">{minimumMet ? "✓" : "○"} </span>
          {minimumLengthLabel}
        </p>
      ) : null}
    </div>
  );
}
