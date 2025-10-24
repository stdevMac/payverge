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

interface AddCategoryModalProps {
  isOpen: boolean;
  onOpenChange: () => void;
  categoryName: string;
  setCategoryName: (name: string) => void;
  categoryDescription: string;
  setCategoryDescription: (description: string) => void;
  onAddCategory: () => void;
  onResetForm: () => void;
  tString: (key: string) => string;
}

export default function AddCategoryModal({
  isOpen,
  onOpenChange,
  categoryName,
  setCategoryName,
  categoryDescription,
  setCategoryDescription,
  onAddCategory,
  onResetForm,
  tString,
}: AddCategoryModalProps) {
  return (
    <Modal isOpen={isOpen} onOpenChange={onOpenChange}>
      <ModalContent>
        {(onClose) => (
          <>
            <ModalHeader>{tString('categories.addCategory')}</ModalHeader>
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
              <Button color="primary" onPress={onAddCategory} isDisabled={!categoryName.trim()}>
                {tString('buttons.createCategory')}
              </Button>
            </ModalFooter>
          </>
        )}
      </ModalContent>
    </Modal>
  );
}
