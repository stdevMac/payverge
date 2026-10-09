import React, { useState } from "react";
import {
  Modal,
  ModalContent,
  ModalHeader,
  ModalBody,
  ModalFooter,
  Button,
  Textarea,
} from "@nextui-org/react";

interface CancelDeliveryModalProps {
  isOpen: boolean;
  onClose: () => void;
  onConfirm: (reason: string) => Promise<void>;
  tString: (key: string) => string;
  /** When true, prepend a "this order may already be paid" warning. */
  showPaidWarning?: boolean;
}

export function CancelDeliveryModal({
  isOpen,
  onClose,
  onConfirm,
  tString,
  showPaidWarning = false,
}: CancelDeliveryModalProps) {
  const [reason, setReason] = useState("");
  const [loading, setLoading] = useState(false);

  const handleConfirm = async () => {
    setLoading(true);
    try {
      // Only clear the typed reason + close on success. If onConfirm rejects
      // (cancel failed on the backend), keep the modal open with the reason
      // intact so the operator can retry instead of losing what they typed and
      // seeing a false "cancelled" toast.
      await onConfirm(reason);
      setReason("");
      onClose();
    } catch {
      // Swallow: the parent handler surfaces the error toast. Leaving the modal
      // open (reason preserved) is the recovery affordance.
    } finally {
      setLoading(false);
    }
  };

  const handleClose = () => {
    setReason("");
    onClose();
  };

  return (
    <Modal isOpen={isOpen} onClose={handleClose} placement="center">
      <ModalContent>
        <ModalHeader className="text-base font-semibold">
          {tString("dispatch.cancel.title")}
        </ModalHeader>
        <ModalBody>
          {showPaidWarning && (
            <p className="text-sm text-amber-700 bg-amber-50 border border-amber-200 rounded-lg p-2 mb-3">
              {tString("dispatch.cancelPaidWarning")}
            </p>
          )}
          <p className="text-sm text-ink-700 mb-3">
            {tString("dispatch.cancel.description")}
          </p>
          {/* DEL-UX-3: the reason is rendered verbatim to the guest on the
              public tracking page — the operator must know the audience before
              typing an internal note into a guest-facing field. */}
          <p className="text-xs text-ink-500 mb-3">
            {tString("dispatch.cancel.visibleToCustomer")}
          </p>
          <Textarea
            placeholder={tString("dispatch.cancel.reasonPlaceholder")}
            value={reason}
            onValueChange={setReason}
            minRows={2}
            maxRows={4}
            isDisabled={loading}
          />
        </ModalBody>
        <ModalFooter>
          <Button variant="light" onPress={handleClose} isDisabled={loading}>
            {tString("dispatch.cancel.back")}
          </Button>
          <Button
            color="danger"
            onPress={handleConfirm}
            isLoading={loading}
            isDisabled={!reason.trim()}
          >
            {tString("dispatch.cancel.confirm")}
          </Button>
        </ModalFooter>
      </ModalContent>
    </Modal>
  );
}
