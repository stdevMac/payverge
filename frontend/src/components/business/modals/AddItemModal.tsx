'use client';

import React from 'react';
import {
  Modal,
  ModalContent,
  ModalHeader,
  ModalBody,
  ModalFooter,
  Input,
  Textarea,
  Switch,
} from '@nextui-org/react';
import { Plus, Image as ImageIcon, DollarSign, AlertTriangle, Tag } from 'lucide-react';
import { MenuCategory, MenuItemOption } from '../../../api/business';
import MultipleImageUpload from '../MultipleImageUpload';

interface AddItemModalProps {
  isOpen: boolean;
  onOpenChange: () => void;
  selectedCategoryIndex: number | null;
  menu: MenuCategory[];
  itemName: string;
  setItemName: (name: string) => void;
  itemDescription: string;
  setItemDescription: (description: string) => void;
  itemPrice: string;
  setItemPrice: (price: string) => void;
  defaultCurrency: string;
  itemImages: string[];
  setItemImages: (images: string[]) => void;
  itemAvailable: boolean;
  setItemAvailable: (available: boolean) => void;
  itemSortOrder: number;
  setItemSortOrder: (order: number) => void;
  businessId: number;
  itemOptions: MenuItemOption[];
  newOptionName: string;
  setNewOptionName: (name: string) => void;
  newOptionPrice: string;
  setNewOptionPrice: (price: string) => void;
  onAddOption: () => void;
  onRemoveOption: (index: number) => void;
  itemAllergens: string[];
  newAllergen: string;
  setNewAllergen: (allergen: string) => void;
  onAddAllergen: () => void;
  onRemoveAllergen: (allergen: string) => void;
  itemDietaryTags: string[];
  newDietaryTag: string;
  setNewDietaryTag: (tag: string) => void;
  onAddDietaryTag: () => void;
  onRemoveDietaryTag: (tag: string) => void;
  onAddItem: () => void;
  onResetForm: () => void;
  tString: (key: string) => string;
}

export default function AddItemModal({
  isOpen,
  onOpenChange,
  selectedCategoryIndex,
  menu,
  itemName,
  setItemName,
  itemDescription,
  setItemDescription,
  itemPrice,
  setItemPrice,
  defaultCurrency,
  itemImages,
  setItemImages,
  itemAvailable,
  setItemAvailable,
  itemSortOrder,
  setItemSortOrder,
  businessId,
  itemOptions,
  newOptionName,
  setNewOptionName,
  newOptionPrice,
  setNewOptionPrice,
  onAddOption,
  onRemoveOption,
  itemAllergens,
  newAllergen,
  setNewAllergen,
  onAddAllergen,
  onRemoveAllergen,
  itemDietaryTags,
  newDietaryTag,
  setNewDietaryTag,
  onAddDietaryTag,
  onRemoveDietaryTag,
  onAddItem,
  onResetForm,
  tString,
}: AddItemModalProps) {
  return (
    <Modal isOpen={isOpen} onOpenChange={onOpenChange} size="3xl" scrollBehavior="inside">
      <ModalContent>
        {(onClose) => (
          <>
            <ModalHeader className="flex items-center gap-2 text-lg font-medium text-gray-900">
              <Plus className="w-4 h-4" />
              {tString('items.addItem')}
              {selectedCategoryIndex !== null && (
                <span className="text-xs bg-blue-100 text-blue-700 px-2 py-1 rounded">
                  {menu[selectedCategoryIndex]?.name}
                </span>
              )}
            </ModalHeader>
            <ModalBody className="space-y-4">
              {/* Basic Information */}
              <div className="bg-gray-50 rounded-lg p-4">
                <h4 className="text-sm font-medium text-gray-900 flex items-center gap-2 mb-3">
                  <ImageIcon className="w-4 h-4" />
                  {tString('items.title')}
                </h4>
                <div className="grid grid-cols-1 md:grid-cols-2 gap-3">
                  <div>
                    <label className="text-xs font-medium text-gray-700 mb-1 block">{tString('items.itemName')}</label>
                    <Input
                      placeholder={tString('items.itemNamePlaceholder')}
                      value={itemName}
                      onValueChange={setItemName}
                      isRequired
                      size="sm"
                    />
                  </div>
                  <div className="flex gap-2">
                    <div className="flex-1">
                      <label className="text-xs font-medium text-gray-700 mb-1 block">{tString('items.itemPrice')}</label>
                      <Input
                        placeholder={tString('items.itemPricePlaceholder')}
                        value={itemPrice}
                        onValueChange={setItemPrice}
                        type="number"
                        step="0.01"
                        min="0"
                        startContent={<DollarSign className="w-3 h-3 text-gray-400" />}
                        isRequired
                        size="sm"
                      />
                    </div>
                    <div className="w-20">
                      <label className="text-xs font-medium text-gray-700 mb-1 block">Currency</label>
                      <div className="flex items-center h-8 px-2 bg-white rounded-lg border text-xs font-medium text-gray-700">
                        {defaultCurrency}
                      </div>
                    </div>
                  </div>
                </div>
                <div className="mt-3">
                  <label className="text-xs font-medium text-gray-700 mb-1 block">{tString('items.itemDescription')}</label>
                  <Textarea
                    placeholder={tString('items.itemDescriptionPlaceholder')}
                    value={itemDescription}
                    onValueChange={setItemDescription}
                    minRows={2}
                    size="sm"
                  />
                </div>
                <div className="flex items-center gap-4 mt-3">
                  <Switch
                    isSelected={itemAvailable}
                    onValueChange={setItemAvailable}
                    color="success"
                    size="sm"
                  >
                    <span className="text-xs">{tString('items.available')}</span>
                  </Switch>
                  <div className="w-24">
                    <label className="text-xs font-medium text-gray-700 mb-1 block">{tString('items.sortOrder')}</label>
                    <Input
                      placeholder="0"
                      value={itemSortOrder.toString()}
                      onValueChange={(value) => setItemSortOrder(parseInt(value) || 0)}
                      type="number"
                      size="sm"
                    />
                  </div>
                </div>
              </div>

              {/* Image Upload */}
              <div className="bg-gray-50 rounded-lg p-4">
                <h4 className="text-sm font-medium text-gray-900 mb-3">Images</h4>
                <MultipleImageUpload
                  images={itemImages}
                  onImagesChange={setItemImages}
                  maxImages={5}
                  businessId={businessId}
                />
              </div>

              {/* Options */}
              <div className="bg-gray-50 rounded-lg p-4">
                <h4 className="text-sm font-medium text-gray-900 flex items-center gap-2 mb-3">
                  <DollarSign className="w-4 h-4" />
                  {tString('items.options')}
                </h4>
                <div className="flex gap-2 mb-3">
                  <Input
                    placeholder={tString('items.optionNamePlaceholder')}
                    value={newOptionName}
                    onValueChange={setNewOptionName}
                    size="sm"
                    className="flex-1"
                  />
                  <Input
                    placeholder={tString('items.optionPricePlaceholder')}
                    value={newOptionPrice}
                    onValueChange={setNewOptionPrice}
                    type="number"
                    step="0.01"
                    startContent={<DollarSign className="w-3 h-3 text-gray-400" />}
                    className="w-24"
                    size="sm"
                  />
                  <button
                    onClick={onAddOption}
                    disabled={!newOptionName.trim() || !newOptionPrice}
                    className="bg-gray-900 text-white px-3 py-1 text-xs font-medium hover:bg-gray-800 transition-colors rounded-lg disabled:opacity-50"
                  >
                    Add
                  </button>
                </div>
                {itemOptions.length > 0 && (
                  <div className="flex flex-wrap gap-1">
                    {itemOptions.map((option, index) => (
                      <span
                        key={`option-${option.name}-${index}`}
                        className="inline-flex items-center gap-1 text-xs bg-blue-100 text-blue-700 px-2 py-1 rounded"
                      >
                        {option.name} (+${(option.price_change || 0).toFixed(2)})
                        <button
                          onClick={() => onRemoveOption(index)}
                          className="text-blue-500 hover:text-blue-700"
                        >
                          ×
                        </button>
                      </span>
                    ))}
                  </div>
                )}
              </div>

              {/* Allergens & Dietary Tags */}
              <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
                {/* Allergens */}
                <div className="bg-gray-50 rounded-lg p-4">
                  <h4 className="text-sm font-medium text-gray-900 flex items-center gap-2 mb-3">
                    <AlertTriangle className="w-4 h-4" />
                    {tString('items.allergens')}
                  </h4>
                  <div className="flex gap-2 mb-3">
                    <Input
                      placeholder={tString('items.addAllergen')}
                      value={newAllergen}
                      onValueChange={setNewAllergen}
                      onKeyDown={(e) => {
                        if (e.key === 'Enter') {
                          e.preventDefault();
                          onAddAllergen();
                        }
                      }}
                      size="sm"
                      className="flex-1"
                    />
                    <button
                      onClick={onAddAllergen}
                      disabled={!newAllergen.trim()}
                      className="bg-orange-100 text-orange-700 hover:bg-orange-200 px-3 py-1 text-xs font-medium transition-colors rounded-lg disabled:opacity-50"
                    >
                      Add
                    </button>
                  </div>
                  {itemAllergens.length > 0 && (
                    <div className="flex flex-wrap gap-1">
                      {itemAllergens.map((allergen, index) => (
                        <span
                          key={`allergen-${allergen}-${index}`}
                          className="inline-flex items-center gap-1 text-xs bg-orange-100 text-orange-700 px-2 py-1 rounded"
                        >
                          {allergen}
                          <button
                            onClick={() => onRemoveAllergen(allergen)}
                            className="text-orange-500 hover:text-orange-700"
                          >
                            ×
                          </button>
                        </span>
                      ))}
                    </div>
                  )}
                </div>

                {/* Dietary Tags */}
                <div className="bg-gray-50 rounded-lg p-4">
                  <h4 className="text-sm font-medium text-gray-900 flex items-center gap-2 mb-3">
                    <Tag className="w-4 h-4" />
                    {tString('items.dietaryTags')}
                  </h4>
                  <div className="flex gap-2 mb-3">
                    <Input
                      placeholder={tString('items.addDietaryTag')}
                      value={newDietaryTag}
                      onValueChange={setNewDietaryTag}
                      onKeyDown={(e) => {
                        if (e.key === 'Enter') {
                          e.preventDefault();
                          onAddDietaryTag();
                        }
                      }}
                      size="sm"
                      className="flex-1"
                    />
                    <button
                      onClick={onAddDietaryTag}
                      disabled={!newDietaryTag.trim()}
                      className="bg-green-100 text-green-700 hover:bg-green-200 px-3 py-1 text-xs font-medium transition-colors rounded-lg disabled:opacity-50"
                    >
                      Add
                    </button>
                  </div>
                  {itemDietaryTags.length > 0 && (
                    <div className="flex flex-wrap gap-1">
                      {itemDietaryTags.map((tag, index) => (
                        <span
                          key={`dietary-${tag}-${index}`}
                          className="inline-flex items-center gap-1 text-xs bg-green-100 text-green-700 px-2 py-1 rounded"
                        >
                          {tag}
                          <button
                            onClick={() => onRemoveDietaryTag(tag)}
                            className="text-green-500 hover:text-green-700"
                          >
                            ×
                          </button>
                        </span>
                      ))}
                    </div>
                  )}
                </div>
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
                onClick={onAddItem}
                disabled={!itemName.trim() || !itemPrice}
                className="bg-gray-900 text-white px-4 py-2 text-sm font-medium hover:bg-gray-800 transition-colors rounded-lg disabled:opacity-50 disabled:cursor-not-allowed"
              >
                {tString('buttons.createItem')}
              </button>
            </ModalFooter>
          </>
        )}
      </ModalContent>
    </Modal>
  );
}
