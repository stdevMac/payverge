"use client";
import React, { useState } from "react";
import {
  Modal,
  ModalContent,
  ModalHeader,
  ModalBody,
  ModalFooter,
  Button,
  Input,
} from "@nextui-org/react";
import Image from "next/image";

type UserVerificationModalProps = {
  isOpen: boolean;
  government_id: string;
  birthday: string;
  proof_of_address: string;
  onClose: () => void;
  reject: () => void;
  approve: () => void;
};

export const UserVerificationModal: React.FC<UserVerificationModalProps> = ({
  isOpen,
  government_id,
  birthday,
  proof_of_address,
  onClose,
  reject,
  approve,
}) => {
  const [amount, setAmount] = useState(0);

  const handleSave = () => {
    approve();
    onClose();
  };
  const handleReject = () => {
    reject();
    onClose();
  };
  return (
    <Modal
      isOpen={isOpen}
      onOpenChange={onClose}
      isDismissable={false}
      isKeyboardDismissDisabled={true}
    >
      <ModalContent>
        {(onClose) => (
          <>
            <ModalHeader className="flex flex-col gap-1">
              Verify User
            </ModalHeader>
            <ModalBody>
              {/* Display files for Government ID */}
              <div className="mb-4">
                <h4 className="text-lg font-semibold mb-2">Government ID</h4>
                {government_id.endsWith(".pdf") ? (
                  <embed
                    src={government_id}
                    type="application/pdf"
                    width="100%"
                    height="400px"
                  />
                ) : (
                  <div className="relative w-full h-[400px]">
                    <Image
                      src={government_id}
                      alt="Government ID"
                      fill
                      style={{ objectFit: "contain" }}
                      className="max-w-full"
                    />
                  </div>
                )}
              </div>

              {/* Display birthday */}
              <div className="mb-4">
                <h4 className="text-lg font-semibold mb-2">Birthday</h4>
                <p>{birthday}</p>
              </div>

              {/* Display files for Proof of Address */}
              <div>
                <h4 className="text-lg font-semibold mb-2">Proof of Address</h4>
                {proof_of_address.endsWith(".pdf") ? (
                  <embed
                    src={proof_of_address}
                    type="application/pdf"
                    width="100%"
                    height="400px"
                  />
                ) : (
                  <div className="relative w-full h-[400px]">
                    <Image
                      src={proof_of_address}
                      alt="Proof of Address"
                      fill
                      style={{ objectFit: "contain" }}
                      className="max-w-full"
                    />
                  </div>
                )}
              </div>
            </ModalBody>
            <ModalFooter>
              <Button color="danger" variant="light" onPress={onClose}>
                Cancel
              </Button>
              <Button color="warning" onPress={handleReject}>
                Reject
              </Button>
              <Button color="primary" onPress={handleSave}>
                Approve
              </Button>
            </ModalFooter>
          </>
        )}
      </ModalContent>
    </Modal>
  );
};
