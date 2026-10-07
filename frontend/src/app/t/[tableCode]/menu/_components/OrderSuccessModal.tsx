"use client";

import React from "react";
import Link from "next/link";
import {
  Button,
  Modal,
  ModalBody,
  ModalContent,
  ModalFooter,
  ModalHeader,
} from "@nextui-org/react";
import { Check, Eye, Receipt } from "lucide-react";

interface OrderSuccessModalProps {
  isOpen: boolean;
  onClose: () => void;
  message: string;
  tableCode: string;
  t: (key: string) => string;
}

export default function OrderSuccessModal({
  isOpen,
  onClose,
  message,
  tableCode,
  t,
}: OrderSuccessModalProps) {
  return (
    <Modal
      isOpen={isOpen}
      onClose={onClose}
      size="md"
      backdrop="blur"
      classNames={{
        // Above PersistentGuestNav / Sage FAB (z-50) so success is never buried.
        wrapper: "z-[70]",
        backdrop: "z-[70]",
        base: "m-4 sm:m-6 z-[70] rounded-2xl border border-warm-200",
        body: "py-6",
        header: "border-b border-warm-200",
      }}
    >
      <ModalContent>
        <ModalHeader className="flex flex-col items-center gap-3 pb-4">
          <div className="flex h-14 w-14 items-center justify-center rounded-2xl bg-brand/10 text-brand">
            <Check className="h-7 w-7" strokeWidth={2.25} />
          </div>
          <h3 className="font-title text-heading-lg text-ink-950">
            {t("menu.orderSubmitted")}
          </h3>
        </ModalHeader>
        <ModalBody>
          <div className="space-y-4 text-center">
            <p className="text-body-sm text-ink-600">{message}</p>
            <div className="rounded-xl border border-amber-200 bg-amber-50/60 px-4 py-3">
              <p className="inline-flex items-center justify-center gap-2 text-xs text-amber-800">
                <Eye className="h-4 w-4" strokeWidth={1.75} />
                {t("menu.trackOrdersInBill")}
              </p>
            </div>
          </div>
        </ModalBody>
        <ModalFooter className="flex flex-col gap-2">
          <Button
            onPress={onClose}
            className="h-11 w-full rounded-xl bg-brand font-semibold text-white hover:bg-brand-dark"
          >
            {t("bill.gotIt")}
          </Button>
          <Button
            as={Link}
            href={`/t/${tableCode}/bill`}
            variant="bordered"
            startContent={<Receipt className="h-4 w-4" strokeWidth={1.75} />}
            className="h-11 w-full rounded-xl border-warm-200 font-semibold text-ink-700 hover:border-ink-300 hover:text-ink-900"
          >
            {t("menu.viewBill")}
          </Button>
        </ModalFooter>
      </ModalContent>
    </Modal>
  );
}
