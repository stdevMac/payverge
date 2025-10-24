'use client';

import React from 'react';
import { Modal, ModalContent, ModalHeader, ModalBody, ModalFooter, Button, Select, SelectItem } from '@nextui-org/react';
import { Edit } from 'lucide-react';
import * as StaffAPI from '../../api/staff';

type Staff = StaffAPI.StaffMember;

interface StaffRoleUpdateModalProps {
  isOpen: boolean;
  onClose: () => void;
  selectedStaff: Staff | null;
  newRole: Staff['role'];
  setNewRole: (role: Staff['role']) => void;
  roleUpdateLoading: boolean;
  getRoleLabel: (role: Staff['role']) => string;
  getRoleDescription: (role: Staff['role']) => string;
  tString: (key: string) => string;
  handleRoleUpdateSubmit: () => void;
}

export default function StaffRoleUpdateModal({
  isOpen,
  onClose,
  selectedStaff,
  newRole,
  setNewRole,
  roleUpdateLoading,
  getRoleLabel,
  getRoleDescription,
  tString,
  handleRoleUpdateSubmit
}: StaffRoleUpdateModalProps) {
  return (
    <Modal isOpen={isOpen} onClose={onClose} size="lg">
      <ModalContent>
        <ModalHeader className="flex flex-col gap-1">
          <div className="flex items-center gap-3">
            <div className="p-2 rounded-lg bg-primary/10">
              <Edit className="w-5 h-5 text-primary" />
            </div>
            <div>
              <h3 className="text-xl font-semibold">{tString('modals.updateRole.title')}</h3>
              <p className="text-sm text-default-600 font-normal">
                {tString('modals.updateRole.updatingFor')} <strong>{selectedStaff?.name}</strong>
              </p>
            </div>
          </div>
        </ModalHeader>
        <ModalBody>
          {selectedStaff && (
            <>
              <Select
                label={tString('modals.updateRole.newRole')}
                placeholder={tString('modals.updateRole.selectRole')}
                selectedKeys={[newRole]}
                onSelectionChange={(keys) => {
                  const role = Array.from(keys)[0] as Staff['role'];
                  setNewRole(role);
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

              <div className="bg-amber-50 p-4 rounded-lg">
                <p className="text-sm text-amber-800">
                  <strong>{tString('modals.updateRole.noteTitle')}</strong> {tString('modals.updateRole.noteDescription')}
                </p>
              </div>
            </>
          )}
        </ModalBody>
        <ModalFooter>
          <Button variant="light" onPress={onClose}>
            {tString('buttons.cancel')}
          </Button>
          <Button
            color="primary"
            onPress={handleRoleUpdateSubmit}
            isLoading={roleUpdateLoading}
            className="bg-gray-900 text-white"
          >
            {tString('buttons.updateRole')}
          </Button>
        </ModalFooter>
      </ModalContent>
    </Modal>
  );
}
