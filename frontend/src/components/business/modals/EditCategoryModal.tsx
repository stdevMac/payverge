"use client";

import React from "react";
import { Modal, ModalContent, ModalHeader, ModalBody, ModalFooter, Input, Textarea } from "@nextui-org/react";
import { Edit } from "lucide-react";

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
    <Modal isOpen={isOpen} onOpenChange={onOpenChange} size="md">
      <ModalContent>
        {(onClose) => (
          <>
            <ModalHeader className="flex items-center gap-2 text-lg font-medium text-ink-900">
              <Edit className="w-4 h-4" />
              {tString("categories.editCategory")}
            </ModalHeader>
            <ModalBody className="space-y-4">
              <div>
                <label className="text-sm font-medium text-ink-700 mb-1 block">
                  {tString("categories.categoryName")}
                </label>
                <Input
                  placeholder={tString("categories.categoryNamePlaceholder")}
                  value={categoryName}
                  onValueChange={setCategoryName}
                  isRequired
                  size="sm"
                  className="w-full"
                />
              </div>
              <div>
                <label className="text-sm font-medium text-ink-700 mb-1 block">
                  {tString("categories.categoryDescription")}
                </label>
                <Textarea
                  placeholder={tString(
                    "categories.categoryDescriptionPlaceholder",
                  )}
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
                type="button"
                onClick={() => {
                  onResetForm();
                  onClose();
                }}
                className="text-ink-700 hover:text-ink-800 px-4 py-2 text-sm font-medium transition-colors rounded-lg hover:bg-warm-100"
              >
                {tString("buttons.cancel")}
              </button>
              <button
                type="button"
                onClick={onUpdateCategory}
                disabled={!categoryName.trim()}
                className="bg-brand text-white px-4 py-2 text-sm font-medium hover:bg-brand-dark transition-colors rounded-lg disabled:opacity-50 disabled:cursor-not-allowed"
              >
                {tString("buttons.updateCategory")}
              </button>
            </ModalFooter>
          </>
        )}
      </ModalContent>
    </Modal>
  );
}
