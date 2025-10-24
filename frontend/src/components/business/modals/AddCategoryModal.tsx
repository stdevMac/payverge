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
    <Modal isOpen={isOpen} onOpenChange={onOpenChange} size="md">
      <ModalContent>
        {(onClose) => (
          <>
            <ModalHeader className="text-lg font-medium text-gray-900">
              {tString('categories.addCategory')}
            </ModalHeader>
            <ModalBody className="space-y-4">
              <div>
                <label className="text-sm font-medium text-gray-700 mb-1 block">
                  {tString('categories.categoryName')}
                </label>
                <Input
                  placeholder={tString('categories.categoryNamePlaceholder')}
                  value={categoryName}
                  onValueChange={setCategoryName}
                  isRequired
                  size="sm"
                  className="w-full"
                />
              </div>
              <div>
                <label className="text-sm font-medium text-gray-700 mb-1 block">
                  {tString('categories.categoryDescription')}
                </label>
                <Textarea
                  placeholder={tString('categories.categoryDescriptionPlaceholder')}
                  value={categoryDescription}
                  onValueChange={setCategoryDescription}
                  size="sm"
                  minRows={3}
                  className="w-full"
                />
              </div>
            </ModalBody>
            <ModalFooter className="gap-2">
              <button
                onClick={() => {
                  onResetForm();
                  onClose();
                }}
                className="text-gray-600 hover:text-gray-800 px-4 py-2 text-sm font-medium transition-colors rounded-lg hover:bg-gray-100"
              >
                {tString('buttons.cancel')}
              </button>
              <button
                onClick={onAddCategory}
                disabled={!categoryName.trim()}
                className="bg-gray-900 text-white px-4 py-2 text-sm font-medium hover:bg-gray-800 transition-colors rounded-lg disabled:opacity-50 disabled:cursor-not-allowed"
              >
                {tString('buttons.createCategory')}
              </button>
            </ModalFooter>
          </>
        )}
      </ModalContent>
    </Modal>
  );
}
