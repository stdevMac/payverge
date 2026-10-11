"use client";

import React from "react";
import { Button } from "@nextui-org/react";
import { Printer } from "lucide-react";
import { btnSecondaryNextUI } from "@/components/ui/buttonStyles";

export interface PrintMenuButtonProps {
  tString: (key: string) => string;
  isDisabled: boolean;
  onPress: () => void;
}

/**
 * Header action that opens the restaurant menu creation output step.
 * Matches AddSourceMenu secondary button styling (bordered full-radius pill).
 */
export function PrintMenuButton({
  tString,
  isDisabled,
  onPress,
}: PrintMenuButtonProps) {
  return (
    <Button
      variant="bordered"
      radius="full"
      className={btnSecondaryNextUI}
      startContent={<Printer className="h-4 w-4" />}
      aria-label={tString("print.buttonAria")}
      isDisabled={isDisabled}
      onPress={onPress}
    >
      {tString("print.buttonLabel")}
    </Button>
  );
}
