'use client';

import React from 'react';
import { useSimpleLocale, getTranslation } from '@/i18n/SimpleTranslationProvider';
import {
  Button,
  Input,
  Modal,
  ModalContent,
  ModalHeader,
  ModalBody,
  ModalFooter,
  Switch,
} from '@nextui-org/react';
import { Table } from '@/api/business';

interface TableModalsProps {
  // Create Modal
  isCreateOpen: boolean;
  onCreateOpenChange: () => void;
  tableName: string;
  setTableName: (name: string) => void;
  handleCreateTable: () => void;
  
  // Edit Modal
  isEditOpen: boolean;
  onEditOpenChange: () => void;
  editingTable: Table | null;
  editName: string;
  setEditName: (name: string) => void;
  editActive: boolean;
  setEditActive: (active: boolean) => void;
  handleUpdateTable: () => void;
}

export default function TableModals({
  isCreateOpen,
  onCreateOpenChange,
  tableName,
  setTableName,
  handleCreateTable,
  isEditOpen,
  onEditOpenChange,
  editingTable,
  editName,
  setEditName,
  editActive,
  setEditActive,
  handleUpdateTable,
}: TableModalsProps) {
  // Translation setup
  const { locale } = useSimpleLocale();
  const [currentLocale, setCurrentLocale] = React.useState(locale);
  
  // Update translations when locale changes
  React.useEffect(() => {
    setCurrentLocale(locale);
  }, [locale]);
  
  // Translation helper
  const tString = (key: string): string => {
    const fullKey = `businessDashboard.dashboard.tableManager.${key}`;
    const result = getTranslation(fullKey, currentLocale);
    return Array.isArray(result) ? result[0] || key : result as string;
  };

  return (
    <>
      {/* Create Table Modal */}
      <Modal isOpen={isCreateOpen} onOpenChange={onCreateOpenChange}>
        <ModalContent>
          {(onClose) => (
            <>
              <ModalHeader>{tString('modals.create.title')}</ModalHeader>
              <ModalBody>
                <Input
                  label={tString('modals.create.tableName')}
                  placeholder={tString('modals.create.placeholder')}
                  value={tableName}
                  onValueChange={setTableName}
                  isRequired
                />
              </ModalBody>
              <ModalFooter>
                <Button color="danger" variant="light" onPress={onClose}>
                  {tString('modals.create.cancel')}
                </Button>
                <Button color="primary" onPress={handleCreateTable}>
                  {tString('modals.create.create')}
                </Button>
              </ModalFooter>
            </>
          )}
        </ModalContent>
      </Modal>

      {/* Edit Table Modal */}
      <Modal isOpen={isEditOpen} onOpenChange={onEditOpenChange}>
        <ModalContent>
          {(onClose) => (
            <>
              <ModalHeader>{tString('modals.edit.title')}</ModalHeader>
              <ModalBody>
                <Input
                  label={tString('modals.edit.tableName')}
                  value={editName}
                  onValueChange={setEditName}
                  isRequired
                />
                <div className="flex items-center justify-between">
                  <span>{tString('modals.edit.activeStatus')}</span>
                  <Switch
                    isSelected={editActive}
                    onValueChange={setEditActive}
                  />
                </div>
              </ModalBody>
              <ModalFooter>
                <Button color="danger" variant="light" onPress={onClose}>
                  {tString('modals.edit.cancel')}
                </Button>
                <Button color="primary" onPress={handleUpdateTable}>
                  {tString('modals.edit.save')}
                </Button>
              </ModalFooter>
            </>
          )}
        </ModalContent>
      </Modal>
    </>
  );
}
