"use client";

import {
  Modal,
  ModalContent,
  ModalHeader,
  ModalBody,
  Spinner,
} from "@nextui-org/react";
import { useEffect, useState } from "react";
import {
  getOpsTranscript,
  type OpsMessage,
  type OpsTranscriptTarget,
} from "@/api/adminEscalations";

export function OpsTranscriptModal({
  target,
  title,
  isOpen,
  onClose,
}: {
  target: OpsTranscriptTarget | null;
  title: string;
  isOpen: boolean;
  onClose: () => void;
}) {
  const [messages, setMessages] = useState<OpsMessage[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    if (!isOpen || !target) return;
    let stale = false;
    setLoading(true);
    setError(null);

    getOpsTranscript(target.businessId, target.threadId)
      .then((data) => data.messages)
      .then((rows) => {
        if (!stale) setMessages(rows);
      })
      .catch(() => {
        if (!stale) {
          setMessages([]);
          setError("Could not load transcript");
        }
      })
      .finally(() => {
        if (!stale) setLoading(false);
      });

    return () => {
      stale = true;
    };
  }, [isOpen, target]);

  return (
    <Modal isOpen={isOpen} onClose={onClose} size="2xl" scrollBehavior="inside">
      <ModalContent>
        <ModalHeader>{title}</ModalHeader>
        <ModalBody className="pb-6">
          {loading ? (
            <div className="flex justify-center py-8">
              <Spinner />
            </div>
          ) : error ? (
            <p className="text-danger text-sm">{error}</p>
          ) : messages.length === 0 ? (
            <p className="text-default-500 text-sm">No transcript saved.</p>
          ) : (
            <div className="space-y-3">
              {messages.map((msg) => (
                <div
                  key={msg.id}
                  className={`rounded-lg p-3 text-sm ${
                    msg.role === "user"
                      ? "bg-brand/5 border border-brand/10"
                      : "bg-default-50 border border-default-200"
                  }`}
                >
                  <p className="text-xs font-semibold uppercase tracking-wide text-default-400 mb-1">
                    {msg.role}
                  </p>
                  <p className="whitespace-pre-wrap break-words">{msg.content}</p>
                </div>
              ))}
            </div>
          )}
        </ModalBody>
      </ModalContent>
    </Modal>
  );
}
