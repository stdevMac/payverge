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
  Chip,
  Divider,
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
    <Modal isOpen={isOpen} onOpenChange={onOpenChange} size="2xl" scrollBehavior="inside">
      <ModalContent>
        {(onClose) => (
          <>
            <ModalHeader className="flex items-center gap-2">
              <Plus className="w-5 h-5" />
              {tString('items.addItem')}
              {selectedCategoryIndex !== null && (
                <Chip size="sm" variant="flat" color="primary">
                  {menu[selectedCategoryIndex]?.name}
                </Chip>
              )}
            </ModalHeader>
            <ModalBody className="space-y-6">
              {/* Basic Information */}
              <div className="space-y-4">
                <h4 className="text-lg font-semibold text-gray-900 flex items-center gap-2">
                  <ImageIcon className="w-5 h-5" />
                  {tString('items.title')}
                </h4>
                <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
                  <Input
                    label={tString('items.itemName')}
                    placeholder={tString('items.itemNamePlaceholder')}
                    value={itemName}
                    onValueChange={setItemName}
                    isRequired
                  />
                  <div className="flex gap-2">
                    <Input
                      label={tString('items.itemPrice')}
                      placeholder={tString('items.itemPricePlaceholder')}
                      value={itemPrice}
                      onValueChange={setItemPrice}
                      type="number"
                      step="0.01"
                      min="0"
                      startContent={<DollarSign className="w-4 h-4 text-gray-400" />}
                      isRequired
                    />
                    <div className="w-32">
                      <label className="text-sm text-gray-600 mb-1 block">{tString('currency.defaultCurrency')}</label>
                      <div className="flex items-center h-10 px-3 bg-gray-50 rounded-lg border">
                        <span className="text-sm font-medium text-gray-700">{defaultCurrency}</span>
                      </div>
                      <p className="text-xs text-gray-500 mt-1">From business settings</p>
                    </div>
                  </div>
                </div>
                <Textarea
                  label={tString('items.itemDescription')}
                  placeholder={tString('items.itemDescriptionPlaceholder')}
                  value={itemDescription}
                  onValueChange={setItemDescription}
                  minRows={2}
                />
                <div className="flex items-center gap-4">
                  <Switch
                    isSelected={itemAvailable}
                    onValueChange={setItemAvailable}
                    color="success"
                  >
                    {tString('items.available')}
                  </Switch>
                  <Input
                    label={tString('items.sortOrder')}
                    placeholder="0"
                    value={itemSortOrder.toString()}
                    onValueChange={(value) => setItemSortOrder(parseInt(value) || 0)}
                    type="number"
                    className="w-32"
                  />
                </div>
              </div>

              <Divider />

              {/* Multiple Image Upload */}
              <MultipleImageUpload
                images={itemImages}
                onImagesChange={setItemImages}
                maxImages={5}
                businessId={businessId}
              />

              <Divider />

              {/* Options */}
              <div className="space-y-4">
                <h4 className="text-lg font-semibold text-gray-900 flex items-center gap-2">
                  <DollarSign className="w-5 h-5" />
                  {tString('items.options')}
                </h4>
                <div className="flex gap-2">
                  <Input
                    label={tString('items.optionName')}
                    placeholder={tString('items.optionNamePlaceholder')}
                    value={newOptionName}
                    onValueChange={setNewOptionName}
                  />
                  <Input
                    label={tString('items.optionPrice')}
                    placeholder={tString('items.optionPricePlaceholder')}
                    value={newOptionPrice}
                    onValueChange={setNewOptionPrice}
                    type="number"
                    step="0.01"
                    startContent={<DollarSign className="w-4 h-4 text-gray-400" />}
                    className="w-32"
                  />
                  <Button
                    color="primary"
                    variant="flat"
                    onPress={onAddOption}
                    isDisabled={!newOptionName.trim() || !newOptionPrice}
                  >
                    {tString('buttons.add')}
                  </Button>
                </div>
                {itemOptions.length > 0 && (
                  <div className="flex flex-wrap gap-2">
                    {itemOptions.map((option, index) => (
                      <Chip
                        key={`option-${option.name}-${index}`}
                        onClose={() => onRemoveOption(index)}
                        variant="flat"
                        color="primary"
                      >
                        {option.name} (+${(option.price_change || 0).toFixed(2)})
                      </Chip>
                    ))}
                  </div>
                )}
              </div>

              <Divider />

              {/* Allergens */}
              <div className="space-y-4">
                <h4 className="text-lg font-semibold text-gray-900 flex items-center gap-2">
                  <AlertTriangle className="w-5 h-5" />
                  {tString('items.allergens')}
                </h4>
                <div className="flex gap-2">
                  <Input
                    label={tString('items.allergens')}
                    placeholder={tString('items.addAllergen')}
                    value={newAllergen}
                    onValueChange={setNewAllergen}
                    onKeyPress={(e) => {
                      if (e.key === 'Enter') {
                        e.preventDefault();
                        onAddAllergen();
                      }
                    }}
                  />
                  <Button
                    color="warning"
                    variant="flat"
                    onPress={onAddAllergen}
                    isDisabled={!newAllergen.trim()}
                  >
                    {tString('buttons.add')}
                  </Button>
                </div>
                {itemAllergens.length > 0 && (
                  <div className="flex flex-wrap gap-2">
                    {itemAllergens.map((allergen, index) => (
                      <Chip
                        key={`allergen-${allergen}-${index}`}
                        onClose={() => onRemoveAllergen(allergen)}
                        variant="flat"
                        color="warning"
                      >
                        {allergen}
                      </Chip>
                    ))}
                  </div>
                )}
              </div>

              <Divider />

              {/* Dietary Tags */}
              <div className="space-y-4">
                <h4 className="text-lg font-semibold text-gray-900 flex items-center gap-2">
                  <Tag className="w-5 h-5" />
                  {tString('items.dietaryTags')}
                </h4>
                <div className="flex gap-2">
                  <Input
                    label={tString('items.dietaryTags')}
                    placeholder={tString('items.addDietaryTag')}
                    value={newDietaryTag}
                    onValueChange={setNewDietaryTag}
                    onKeyPress={(e) => {
                      if (e.key === 'Enter') {
                        e.preventDefault();
                        onAddDietaryTag();
                      }
                    }}
                  />
                  <Button
                    color="success"
                    variant="flat"
                    onPress={onAddDietaryTag}
                    isDisabled={!newDietaryTag.trim()}
                  >
                    {tString('buttons.add')}
                  </Button>
                </div>
                {itemDietaryTags.length > 0 && (
                  <div className="flex flex-wrap gap-2">
                    {itemDietaryTags.map((tag, index) => (
                      <Chip
                        key={`dietary-${tag}-${index}`}
                        onClose={() => onRemoveDietaryTag(tag)}
                        variant="flat"
                        color="success"
                      >
                        {tag}
                      </Chip>
                    ))}
                  </div>
                )}
              </div>
            </ModalBody>
            <ModalFooter>
              <Button color="danger" variant="light" onPress={() => {
                onResetForm();
                onClose();
              }}>
                {tString('buttons.cancel')}
              </Button>
              <Button 
                color="primary" 
                onPress={onAddItem}
                isDisabled={!itemName.trim() || !itemPrice}
              >
                {tString('buttons.createItem')}
              </Button>
            </ModalFooter>
          </>
        )}
      </ModalContent>
    </Modal>
  );
}
