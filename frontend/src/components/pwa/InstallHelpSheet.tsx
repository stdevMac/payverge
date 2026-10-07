"use client";

import {
  Button,
  Modal,
  ModalBody,
  ModalContent,
  ModalFooter,
  ModalHeader,
} from "@nextui-org/react";
import { useReducedMotion } from "framer-motion";

interface Props {
  mode: "manual-install" | "unsupported" | null;
  manualTitle: string;
  steps: string[];
  doneLabel: string;
  unsupportedTitle: string;
  unsupportedBody: string;
  closeLabel: string;
  onClose(): void;
  onDone(): void;
}

export default function InstallHelpSheet({
  mode,
  manualTitle,
  steps,
  doneLabel,
  unsupportedTitle,
  unsupportedBody,
  closeLabel,
  onClose,
  onDone,
}: Props) {
  const isManual = mode === "manual-install";
  const title = isManual ? manualTitle : unsupportedTitle;
  const prefersReducedMotion = !!useReducedMotion();

  return (
    <Modal
      isOpen={mode !== null}
      onOpenChange={(open) => {
        if (!open) onClose();
      }}
      placement="bottom-center"
      scrollBehavior="inside"
      size="sm"
      hideCloseButton
      disableAnimation={prefersReducedMotion}
    >
      <ModalContent className="rounded-2xl border border-warm-200 bg-white text-ink-950 shadow-panel">
        <ModalHeader className="pb-2 text-heading-sm">{title}</ModalHeader>
        <ModalBody>
          {isManual ? (
            <ol className="space-y-3 text-sm text-ink-700">
              {steps.map((step, index) => (
                <li key={`${index}-${step}`} className="flex gap-3">
                  <span
                    aria-hidden="true"
                    className="grid h-7 w-7 shrink-0 place-items-center rounded-full bg-brand/10 font-semibold text-brand-dark"
                  >
                    {index + 1}
                  </span>
                  <span className="pt-1">{step}</span>
                </li>
              ))}
            </ol>
          ) : (
            <p className="text-sm leading-6 text-ink-600">{unsupportedBody}</p>
          )}
        </ModalBody>
        <ModalFooter className="pt-4 pb-[max(1rem,env(safe-area-inset-bottom))]">
          <Button
            variant="light"
            onPress={onClose}
            disableRipple
            className="min-h-11 rounded-xl text-ink-700"
          >
            {closeLabel}
          </Button>
          {isManual ? (
            <Button
              color="primary"
              onPress={onDone}
              disableRipple
              className="min-h-11 rounded-xl font-semibold"
            >
              {doneLabel}
            </Button>
          ) : null}
        </ModalFooter>
      </ModalContent>
    </Modal>
  );
}
