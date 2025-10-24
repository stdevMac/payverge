'use client';

import React from 'react';
import { Modal, ModalContent, ModalHeader, ModalBody, ModalFooter, Button, Input, Select, SelectItem } from '@nextui-org/react';
import { UserPlus, AlertCircle } from 'lucide-react';
import { toast } from 'react-hot-toast';
import * as StaffAPI from '../../api/staff';

type Staff = StaffAPI.StaffMember;

interface StaffInviteModalProps {
  isOpen: boolean;
  onClose: () => void;
  inviteForm: {
    email: string;
    name: string;
    role: Staff['role'];
  };
  setInviteForm: React.Dispatch<React.SetStateAction<{
    email: string;
    name: string;
    role: Staff['role'];
  }>>;
  inviteLoading: boolean;
  getRoleLabel: (role: Staff['role']) => string;
  getRoleDescription: (role: Staff['role']) => string;
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
  handleInviteStaff
}: StaffInviteModalProps) {
  // Form validation
  const validateForm = () => {
    if (!inviteForm.name.trim()) {
      toast.error(tString('error.nameRequired'));
      return false;
    }
    if (!inviteForm.email.trim()) {
      toast.error(tString('error.emailRequired'));
      return false;
    }
    if (!inviteForm.email.includes('@')) {
      toast.error(tString('error.emailInvalid'));
      return false;
    }
    if (!inviteForm.role) {
      toast.error(tString('error.roleRequired'));
      return false;
    }
    return true;
  };

  const handleSubmit = () => {
    if (validateForm()) {
      handleInviteStaff();
    }
  };
  return (
    <Modal isOpen={isOpen} onClose={onClose} size="2xl">
      <ModalContent>
        <ModalHeader className="flex flex-col gap-1">
          <div className="flex items-center gap-3">
            <div className="p-2 rounded-lg bg-primary/10">
              <UserPlus className="w-5 h-5 text-primary" />
            </div>
            <div>
              <h3 className="text-xl font-semibold">{tString('modals.invite.title')}</h3>
              <p className="text-sm text-default-600 font-normal">{tString('modals.invite.noteDescription')}</p>
            </div>
          </div>
        </ModalHeader>
        <ModalBody>
          <div className="space-y-4">
            <Input
              label={tString('modals.invite.fullName')}
              placeholder={tString('modals.invite.fullNamePlaceholder')}
              value={inviteForm.name}
              onValueChange={(value) => setInviteForm(prev => ({ ...prev, name: value }))}
              variant="bordered"
              size="lg"
            />
            
            <Input
              label={tString('modals.invite.email')}
              placeholder={tString('modals.invite.emailPlaceholder')}
              type="email"
              value={inviteForm.email}
              onValueChange={(value) => setInviteForm(prev => ({ ...prev, email: value }))}
              variant="bordered"
              size="lg"
            />
            
            <Select
              label={tString('modals.invite.role')}
              placeholder={tString('modals.invite.rolePlaceholder')}
              selectedKeys={[inviteForm.role]}
              onSelectionChange={(keys) => {
                const role = Array.from(keys)[0] as Staff['role'];
                setInviteForm(prev => ({ ...prev, role }));
              }}
              variant="bordered"
              size="lg"
            >
              {(['manager', 'server', 'host', 'kitchen'] as Staff['role'][]).map((role) => (
                <SelectItem key={role} value={role} textValue={getRoleLabel(role)}>
                  <div className="flex flex-col">
                    <span className="font-medium">{getRoleLabel(role)}</span>
                    <span className="text-xs text-default-500">{getRoleDescription(role)}</span>
                  </div>
                </SelectItem>
              ))}
            </Select>

            <div className="bg-blue-50 p-4 rounded-lg">
              <p className="text-sm text-blue-800">
                <strong>{tString('modals.invite.noteTitle')}</strong> {tString('modals.invite.noteDescription')}
              </p>
            </div>
          </div>
        </ModalBody>
        <ModalFooter>
          <Button variant="light" onPress={onClose}>
            {tString('modals.invite.cancel')}
          </Button>
          <Button
            color="primary"
            onPress={handleSubmit}
            isLoading={inviteLoading}
            startContent={!inviteLoading && <UserPlus className="w-4 h-4" />}
            className="bg-gray-900 text-white"
          >
            {tString('modals.invite.sendInvitation')}
          </Button>
        </ModalFooter>
      </ModalContent>
    </Modal>
  );
}
