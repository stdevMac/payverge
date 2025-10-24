'use client';

import React from 'react';
import {
  Modal,
  ModalContent,
  ModalHeader,
  ModalBody,
  ModalFooter,
  Button,
  Input,
  Textarea,
} from '@nextui-org/react';
import { Edit } from 'lucide-react';

interface EditCategoryModalProps {
  isOpen: boolean;
  onOpenChange: () => void;
  categoryName: string;
  setCategoryName: (name: string) => void;
  categoryDescription: string;
  setCategoryDescription: (description: string) => void;
  onUpdateCategory: () => void;
  onResetForm: () => void;
  tString: (key: string) => string;
}

export default function EditCategoryModal({
  isOpen,
  onOpenChange,
  categoryName,
  setCategoryName,
  categoryDescription,
  setCategoryDescription,
  onUpdateCategory,
  onResetForm,
  tString,
}: EditCategoryModalProps) {
  return (
    <Modal isOpen={isOpen} onOpenChange={onOpenChange}>
      <ModalContent>
        {(onClose) => (
          <>
            <ModalHeader className="flex items-center gap-2">
              <Edit className="w-5 h-5" />
              {tString('categories.editCategory')}
            </ModalHeader>
            <ModalBody>
              <Input
                label={tString('categories.categoryName')}
                placeholder={tString('categories.categoryNamePlaceholder')}
                value={categoryName}
                onValueChange={setCategoryName}
                isRequired
              />
              <Textarea
                label={tString('categories.categoryDescription')}
                placeholder={tString('categories.categoryDescriptionPlaceholder')}
                value={categoryDescription}
                onValueChange={setCategoryDescription}
              />
            </ModalBody>
            <ModalFooter>
              <Button color="danger" variant="light" onPress={() => {
                onResetForm();
                onClose();
              }}>
                {tString('buttons.cancel')}
              </Button>
              <Button color="primary" onPress={onUpdateCategory} isDisabled={!categoryName.trim()}>
                {tString('buttons.updateCategory')}
              </Button>
            </ModalFooter>
          </>
        )}
      </ModalContent>
    </Modal>
  );
}
