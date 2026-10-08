"use client";

import React from "react";
import { Input } from "@nextui-org/react";
import { Clock } from "lucide-react";

export interface TimeFieldProps {
  label: string;
  /** 24h "HH:MM" wire value. */
  value: string;
  onChange: (value: string) => void;
  isRequired?: boolean;
  isDisabled?: boolean;
  /** Accessible name when no visible label is wanted. */
  ariaLabel?: string;
  className?: string;
  /** S-5 / L5-28: inline field error (e.g. Fin === Inicio). */
  isInvalid?: boolean;
  errorMessage?: string;
  /** Optional test id for the underlying input. */
  "data-testid"?: string;
}

/**
 * App-consistent time control. Wraps a native `<input type="time">` (kept for
 * the OS-native picker + accessibility) inside NextUI's themed Input shell so it
 * matches every other bordered field in the product instead of reading as a raw
 * HTML control. Value is the 24h "HH:MM" wire string the schedule APIs expect.
 */
export function TimeField({
  label,
  value,
  onChange,
  isRequired = false,
  isDisabled = false,
  ariaLabel,
  className,
  isInvalid = false,
  errorMessage,
  "data-testid": dataTestId,
}: TimeFieldProps) {
  return (
    <Input
      type="time"
      label={label}
      aria-label={ariaLabel ?? label}
      value={value}
      onValueChange={onChange}
      isRequired={isRequired}
      isDisabled={isDisabled}
      isInvalid={isInvalid}
      errorMessage={errorMessage}
      labelPlacement="outside"
      variant="bordered"
      radius="lg"
      size="md"
      data-testid={dataTestId}
      startContent={<Clock className="h-4 w-4 shrink-0 text-ink-400" aria-hidden />}
      classNames={{ label: "text-sm font-medium text-ink-700", inputWrapper: className }}
    />
  );
}
