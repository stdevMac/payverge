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
  Select,
  SelectItem,
} from "@nextui-org/react";
import { UserPlus } from "lucide-react";
import * as StaffAPI from "../../api/staff";
import { isValidEmail } from "@/lib/fieldValidation";
import { useInstance } from "@/hooks/useInstance";

type Staff = StaffAPI.StaffMember;

interface StaffInviteModalProps {
  isOpen: boolean;
  onClose: () => void;
  inviteForm: {
    email: string;
    name: string;
    role: Staff["role"];
  };
  setInviteForm: React.Dispatch<
    React.SetStateAction<{
      email: string;
      name: string;
      role: Staff["role"];
    }>
  >;
  inviteLoading: boolean;
  getRoleLabel: (role: Staff["role"]) => string;
  getRoleDescription: (role: Staff["role"]) => string;
  tString: (key: string) => string;
  handleInviteStaff: () => void;
}

export default function StaffInviteModal({
  isOpen,
  onClose,
  inviteForm,
  setInviteForm,
  inviteLoading,
  getRoleLabel,
  getRoleDescription,
  tString,
  handleInviteStaff,
}: StaffInviteModalProps) {
  // With email off the invite is not mailed: the operator copies the link.
  const noteKey = useInstance().isOff("email")
    ? "modals.invite.noteDescriptionNoEmail"
    : "modals.invite.noteDescription";
  // Single localized validation system (L5-16) — no native type=email bubbles
  // and no weak includes("@") check.
  const [errors, setErrors] = useState<{
    name?: string;
    email?: string;
    role?: string;
  }>({});

  const validateForm = () => {
    const next: typeof errors = {};
    if (!inviteForm.name.trim()) {
      next.name = tString("error.nameRequired");
    }
    if (!inviteForm.email.trim()) {
      next.email = tString("error.emailRequired");
    } else if (!isValidEmail(inviteForm.email)) {
      next.email = tString("error.emailInvalid");
    }
    if (!inviteForm.role) {
      next.role = tString("error.roleRequired");
    }
    setErrors(next);
    return Object.keys(next).length === 0;
  };

  const handleSubmit = () => {
    if (validateForm()) {
      handleInviteStaff();
    }
  };
  return (
    <Modal
      isOpen={isOpen}
      onClose={() => {
        setErrors({});
        onClose();
      }}
      size="2xl"
    >
      <ModalContent>
        <ModalHeader className="flex flex-col gap-1">
          <div className="flex items-center gap-3">
            <div className="p-2 rounded-lg bg-primary/10">
              <UserPlus className="w-5 h-5 text-primary" />
            </div>
            <div>
              <h3 className="text-xl font-semibold">
                {tString("modals.invite.title")}
              </h3>
              <p className="text-sm text-default-600 font-normal">
                {tString(noteKey)}
              </p>
            </div>
          </div>
        </ModalHeader>
        <ModalBody>
          <div className="space-y-4">
            <Input
              label={tString("modals.invite.fullName")}
              placeholder={tString("modals.invite.fullNamePlaceholder")}
              value={inviteForm.name}
              onValueChange={(value) =>
                setInviteForm((prev) => ({ ...prev, name: value }))
              }
              variant="bordered"
              size="lg"
              isRequired
              isInvalid={!!errors.name}
              errorMessage={errors.name}
              data-testid="staff-invite-name"
            />

            <Input
              label={tString("modals.invite.email")}
              placeholder={tString("modals.invite.emailPlaceholder")}
              type="text"
              inputMode="email"
              value={inviteForm.email}
              onValueChange={(value) =>
                setInviteForm((prev) => ({ ...prev, email: value }))
              }
              variant="bordered"
              size="lg"
              isRequired
              isInvalid={!!errors.email}
              errorMessage={errors.email}
              data-testid="staff-invite-email"
            />

            <Select
              label={tString("modals.invite.role")}
              placeholder={tString("modals.invite.rolePlaceholder")}
              selectedKeys={[inviteForm.role]}
              onSelectionChange={(keys) => {
                const role = Array.from(keys)[0] as Staff["role"];
                setInviteForm((prev) => ({ ...prev, role }));
              }}
              variant="bordered"
              size="lg"
              isRequired
              isInvalid={!!errors.role}
              errorMessage={errors.role}
            >
              {(
                ["manager", "server", "host", "kitchen"] as Staff["role"][]
              ).map((role) => (
                <SelectItem
                  key={role}
                  value={role}
                  textValue={getRoleLabel(role)}
                >
                  <div className="flex flex-col">
                    <span className="font-medium">{getRoleLabel(role)}</span>
                    <span className="text-xs text-default-500">
                      {getRoleDescription(role)}
                    </span>
                  </div>
                </SelectItem>
              ))}
            </Select>

            <div className="bg-brand/10 p-4 rounded-lg">
              <p className="text-sm text-brand-dark">
                <strong>{tString("modals.invite.noteTitle")}</strong>{" "}
                {tString(noteKey)}
              </p>
            </div>
          </div>
        </ModalBody>
        <ModalFooter>
          <Button variant="light" onPress={onClose}>
            {tString("modals.invite.cancel")}
          </Button>
          <Button
            color="primary"
            onPress={handleSubmit}
            isLoading={inviteLoading}
            startContent={!inviteLoading && <UserPlus className="w-4 h-4" />}
            className="bg-brand text-white hover:bg-brand-dark"
          >
            {tString("modals.invite.sendInvitation")}
          </Button>
        </ModalFooter>
      </ModalContent>
    </Modal>
  );
}
