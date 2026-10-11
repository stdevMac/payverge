"use client";

import React from "react";
import {
  Modal,
  ModalContent,
  ModalHeader,
  ModalBody,
  ModalFooter,
  Button,
  Input,
} from "@nextui-org/react";
import { Copy, MailWarning } from "lucide-react";
import { toast } from "react-hot-toast";

interface InviteLinkFallbackModalProps {
  isOpen: boolean;
  email: string;
  invitationUrl: string;
  onClose: () => void;
  tString: (key: string) => string;
}

// P2-21: shown when the backend created the invitation but reported
// email_sent:false (provider outage / email unconfigured). The inviter is
// already authorized to mint invitations, so showing them the tokenized link
// grants no new privilege — it just makes delivery possible out-of-band.
export default function InviteLinkFallbackModal({
  isOpen,
  email,
  invitationUrl,
  onClose,
  tString,
}: InviteLinkFallbackModalProps) {
  const handleCopy = async () => {
    try {
      await navigator.clipboard.writeText(invitationUrl);
      toast.success(tString("inviteFallback.copied"));
    } catch {
      toast.error(tString("inviteFallback.copyFailed"));
    }
  };

  return (
    <Modal isOpen={isOpen} onClose={onClose} size="lg">
      <ModalContent>
        <ModalHeader className="flex items-center gap-2">
          <MailWarning className="w-5 h-5 text-amber-600" />
          {tString("inviteFallback.title")}
        </ModalHeader>
        <ModalBody>
          <p className="text-sm text-ink-600">
            {tString("inviteFallback.description").replace("{email}", email)}
          </p>
          <Input
            isReadOnly
            value={invitationUrl}
            variant="bordered"
            aria-label={tString("inviteFallback.linkLabel")}
            endContent={
              <Button
                isIconOnly
                size="sm"
                variant="light"
                aria-label={tString("inviteFallback.copy")}
                onPress={handleCopy}
              >
                <Copy className="w-4 h-4" />
              </Button>
            }
          />
        </ModalBody>
        <ModalFooter>
          <Button color="primary" onPress={onClose}>
            {tString("inviteFallback.done")}
          </Button>
        </ModalFooter>
      </ModalContent>
    </Modal>
  );
}
