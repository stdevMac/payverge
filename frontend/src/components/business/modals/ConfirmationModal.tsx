"use client";

import React, { useEffect, useRef, useState } from "react";
import {
    ModalContent,
    ModalHeader,
    ModalBody,
    ModalFooter,
    Button,
} from "@nextui-org/react";
import { AlertCircle } from "lucide-react";
import {
    useSimpleLocale,
    getTranslation,
} from "@/i18n/SimpleTranslationProvider";
import { LocalizedModal } from "@/components/ui/LocalizedModal";

interface ConfirmationModalProps {
    isOpen: boolean;
    onOpenChange: () => void;
    title: string;
    description: string;
    confirmLabel?: string;
    cancelLabel?: string;
    /**
     * Confirm handler. May return a Promise — the modal awaits it and always
     * closes in `finally` (success or rejection). Callers own error toasts;
     * a rejected promise does not keep the modal open.
     */
    onConfirm: () => void | Promise<void>;
    isDanger?: boolean;
    /** External loading override (OR'd with internal in-flight pending). */
    isLoading?: boolean;
}

export default function ConfirmationModal({
    isOpen,
    onOpenChange,
    title,
    description,
    confirmLabel,
    cancelLabel,
    onConfirm,
    isDanger = false,
    isLoading = false,
}: ConfirmationModalProps) {
    // Localize the default Confirm/Cancel labels so callers that omit
    // them don't accidentally flash English on Spanish-locale dashboards.
    const { locale } = useSimpleLocale();
    const resolvedConfirm =
        confirmLabel ??
        (getTranslation("common.confirm", locale) as string) ??
        "Confirm";
    const resolvedCancel =
        cancelLabel ??
        (getTranslation("common.cancel", locale) as string) ??
        "Cancel";

    // Internal pending while an awaited onConfirm is in flight (callers get a
    // spinner free without wiring isLoading). Cleared when the modal closes.
    const [pending, setPending] = useState(false);
    // Ref guards double-fire before the pending state re-render lands.
    const pendingRef = useRef(false);
    const busy = isLoading || pending;

    useEffect(() => {
        if (!isOpen) {
            pendingRef.current = false;
            setPending(false);
        }
    }, [isOpen]);

    const handleConfirm = async (onClose: () => void) => {
        if (pendingRef.current || isLoading) return;
        pendingRef.current = true;
        setPending(true);
        try {
            await onConfirm();
        } catch {
            // Callers own error reporting (toasts). Swallow so onPress does not
            // leave an unhandled rejection; close still runs in finally.
        } finally {
            onClose();
            pendingRef.current = false;
            setPending(false);
        }
    };

    return (
        <LocalizedModal
            isOpen={isOpen}
            onOpenChange={onOpenChange}
            size="md"
            classNames={{
                wrapper: "z-[80]",
                backdrop: "z-[80]",
                base: "z-[80]",
            }}
        >
            <ModalContent>
                {(onClose) => (
                    <>
                        <ModalHeader className="flex items-center gap-2">
                            {isDanger && <AlertCircle className="w-5 h-5 text-danger" />}
                            {title}
                        </ModalHeader>
                        <ModalBody>
                            <p className="text-ink-700">{description}</p>
                        </ModalBody>
                        <ModalFooter>
                            <Button
                                variant="light"
                                onPress={onClose}
                                isDisabled={busy}
                            >
                                {resolvedCancel}
                            </Button>
                            <Button
                                color={isDanger ? "danger" : "primary"}
                                isLoading={busy}
                                isDisabled={busy}
                                onPress={() => {
                                    void handleConfirm(onClose);
                                }}
                            >
                                {resolvedConfirm}
                            </Button>
                        </ModalFooter>
                    </>
                )}
            </ModalContent>
        </LocalizedModal>
    );
}
