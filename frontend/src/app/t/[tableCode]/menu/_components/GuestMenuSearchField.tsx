"use client";

import { Input } from "@nextui-org/react";
import { Search } from "lucide-react";

export function GuestMenuSearchField({
  value,
  onChange,
  label,
}: {
  value: string;
  onChange: (value: string) => void;
  label: string;
}) {
  return (
    <Input
      aria-label={label}
      placeholder={label}
      value={value}
      onChange={(event) => onChange(event.target.value)}
      startContent={
        <Search className="h-4 w-4 text-ink-400" strokeWidth={1.75} />
      }
      classNames={{
        input: "text-sm",
        inputWrapper:
          "h-10 bg-white border-warm-200 data-[hover=true]:bg-white data-[hover=true]:border-ink-300",
      }}
      size="sm"
    />
  );
}
