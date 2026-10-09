import React, { useState } from "react";
import {
    Modal,
    ModalBody,
    ModalContent,
    ModalHeader,
} from "@nextui-org/react";
import AIMenuOnboarding from "../AIMenuOnboarding";
import { useModalOpenKey } from "@/hooks/useModalOpenKey";

interface AIMenuOnboardingModalProps {
    tString: (key: string) => string;
    isOpen: boolean;
    onClose: () => void;
    businessId: number;
    onImportComplete: () => void;
    initialTab: "pdf" | "wizard";
}

export function AIMenuOnboardingModal({
    tString,
    isOpen,
    onClose,
    businessId,
    onImportComplete,
    initialTab,
}: AIMenuOnboardingModalProps) {
    // L3-3: while sanitization review is up, Esc must not close this modal.
    const [allowDismiss, setAllowDismiss] = useState(true);
    // L3-13: remount body when reopened during exit fade so state cannot leak.
    const openKey = useModalOpenKey(isOpen);

    return (
        <Modal
            isOpen={isOpen}
            onClose={onClose}
            size="4xl"
            scrollBehavior="inside"
            backdrop="blur"
            isDismissable={allowDismiss}
            isKeyboardDismissDisabled={!allowDismiss}
            classNames={{
                base: "bg-warm-50 max-h-[92vh]",
                header: "border-b border-warm-200/60",
            }}
            data-testid="ai-menu-onboarding-modal"
        >
            <ModalContent key={openKey}>
                <ModalHeader className="flex flex-col gap-1">
                    <span className="text-lg font-semibold text-ink-900">
                        {tString("aiOnboarding.drawerTitle")}
                    </span>
                    <span className="text-xs font-normal text-ink-600">
                        {tString("aiOnboarding.drawerSubtitle")}
                    </span>
                </ModalHeader>
                <ModalBody className="py-6">
                    <AIMenuOnboarding
                        businessId={businessId}
                        onImportComplete={() => {
                            onImportComplete();
                            onClose();
                        }}
                        initialTab={initialTab}
                        onBlockingOverlayChange={(blocking) =>
                            setAllowDismiss(!blocking)
                        }
                    />
                </ModalBody>
            </ModalContent>
        </Modal>
    );
}
