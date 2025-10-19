'use client';

import React, { useState } from 'react';
import {
  Modal,
  ModalContent,
  ModalHeader,
  ModalBody,
  ModalFooter,
  Button,
  Card,
  CardBody,
  Checkbox,
  Input,
  Divider,
  Chip,
  Image,
} from '@nextui-org/react';
import { Plus, Minus } from 'lucide-react';
import { MenuItem, MenuItemOption } from '../../api/business';
import { useSimpleLocale, getTranslation } from '@/i18n/SimpleTranslationProvider';

interface ItemCustomizerProps {
  isOpen: boolean;
  onClose: () => void;
  item: MenuItem;
  onAddToCart: (item: MenuItem, quantity: number, selectedOptions: MenuItemOption[], specialRequests: string) => void;
}

interface SelectedOption extends MenuItemOption {
  selected: boolean;
}

export const ItemCustomizer: React.FC<ItemCustomizerProps> = ({
  isOpen,
  onClose,
  item,
  onAddToCart,
}) => {
  const { locale: currentLocale } = useSimpleLocale();
  
  // Translation helper
  const tString = (key: string): string => {
    const fullKey = `itemCustomizer.${key}`;
    const result = getTranslation(fullKey, currentLocale);
    return Array.isArray(result) ? result[0] || key : result as string;
  };

  const [quantity, setQuantity] = useState(1);
  const [selectedOptions, setSelectedOptions] = useState<SelectedOption[]>(
    (item.options || []).map(option => ({ ...option, selected: false }))
  );
  const [specialRequests, setSpecialRequests] = useState('');

  const handleOptionToggle = (optionIndex: number) => {
    setSelectedOptions(prev =>
      prev.map((option, index) =>
        index === optionIndex
          ? { ...option, selected: !option.selected }
          : option
      )
    );
  };

  const calculateTotalPrice = () => {
    const basePrice = item.price || 0;
    const optionsPrice = selectedOptions
      .filter(option => option.selected)
      .reduce((sum, option) => sum + (option.price_change || 0), 0);
    return (basePrice + optionsPrice) * quantity;
  };

  const handleAddToCart = () => {
    const finalSelectedOptions = selectedOptions
      .filter(option => option.selected)
      .map(({ selected, ...option }) => option); // Remove the 'selected' property

    onAddToCart(item, quantity, finalSelectedOptions, specialRequests);
    
    // Reset form
    setQuantity(1);
    setSelectedOptions((item.options || []).map(option => ({ ...option, selected: false })));
    setSpecialRequests('');
    onClose();
  };

  const handleClose = () => {
    // Reset form when closing
    setQuantity(1);
    setSelectedOptions((item.options || []).map(option => ({ ...option, selected: false })));
    setSpecialRequests('');
    onClose();
  };

  const totalPrice = calculateTotalPrice();
  const hasOptions = item.options && item.options.length > 0;

  return (
    <Modal
      isOpen={isOpen}
      onClose={handleClose}
      size="2xl"
      scrollBehavior="inside"
    >
      <ModalContent>
        <ModalHeader className="flex flex-col gap-1">
          <h3 className="text-xl font-semibold">{tString('title')}</h3>
        </ModalHeader>
        
        <ModalBody>
          {/* Item Details */}
          <Card>
            <CardBody className="p-4">
              <div className="flex gap-4">
                {item.image && (
                  <Image
                    src={item.image}
                    alt={item.name}
                    className="w-24 h-24 object-cover rounded-lg flex-shrink-0"
                  />
                )}
                <div className="flex-1">
                  <h4 className="text-lg font-semibold">{item.name}</h4>
                  <p className="text-default-600 text-sm mb-2">{item.description}</p>
                  <Chip color="primary" variant="flat">
                    {tString('basePrice')}: ${(item.price || 0).toFixed(2)}
                  </Chip>
                </div>
              </div>
            </CardBody>
          </Card>

          {/* Quantity Selector */}
          <Card>
            <CardBody className="p-4">
              <h5 className="font-medium mb-3">{tString('quantity')}</h5>
              <div className="flex items-center gap-3">
                <Button
                  isIconOnly
                  size="sm"
                  variant="flat"
                  onPress={() => setQuantity(Math.max(1, quantity - 1))}
                  isDisabled={quantity <= 1}
                >
                  <Minus className="w-4 h-4" />
                </Button>
                <Input
                  size="sm"
                  className="w-20"
                  value={quantity.toString()}
                  onChange={(e) => {
                    const value = parseInt(e.target.value) || 1;
                    setQuantity(Math.max(1, value));
                  }}
                />
                <Button
                  isIconOnly
                  size="sm"
                  variant="flat"
                  onPress={() => setQuantity(quantity + 1)}
                >
                  <Plus className="w-4 h-4" />
                </Button>
              </div>
            </CardBody>
          </Card>

          {/* Add-ons/Options */}
          {hasOptions && (
            <Card>
              <CardBody className="p-4">
                <h5 className="font-medium mb-3">{tString('addOnsOptions')}</h5>
                <div className="space-y-3">
                  {selectedOptions.map((option, index) => (
                    <div key={index} className="flex items-center justify-between">
                      <Checkbox
                        isSelected={option.selected}
                        onValueChange={() => handleOptionToggle(index)}
                      >
                        <div className="flex flex-col">
                          <span className="text-sm font-medium">{option.name}</span>
                          {option.is_required && (
                            <span className="text-xs text-danger">{tString('required')}</span>
                          )}
                        </div>
                      </Checkbox>
                      <Chip
                        size="sm"
                        color={option.price_change === 0 ? "default" : option.price_change > 0 ? "success" : "danger"}
                        variant="flat"
                      >
                        {option.price_change === 0 
                          ? tString('free')
                          : option.price_change > 0 
                            ? `+$${option.price_change.toFixed(2)}`
                            : `-$${Math.abs(option.price_change).toFixed(2)}`
                        }
                      </Chip>
                    </div>
                  ))}
                </div>
              </CardBody>
            </Card>
          )}

          {/* Special Requests */}
          <Card>
            <CardBody className="p-4">
              <h5 className="font-medium mb-3">{tString('specialRequests')}</h5>
              <Input
                placeholder={tString('specialRequestsPlaceholder')}
                value={specialRequests}
                onValueChange={setSpecialRequests}
                variant="bordered"
                maxLength={200}
              />
              <p className="text-xs text-default-500 mt-1">
                {specialRequests.length}/200 {tString('characters')}
              </p>
            </CardBody>
          </Card>

          <Divider />

          {/* Price Summary */}
          <Card className="bg-primary-50 border-primary-200">
            <CardBody className="p-4">
              <div className="space-y-2">
                <div className="flex justify-between text-sm">
                  <span>{tString('basePriceQuantity')} ({quantity}x):</span>
                  <span>${((item.price || 0) * quantity).toFixed(2)}</span>
                </div>
                {selectedOptions.some(option => option.selected) && (
                  <div className="flex justify-between text-sm">
                    <span>{tString('addOnsQuantity')} ({quantity}x):</span>
                    <span>
                      ${(selectedOptions
                        .filter(option => option.selected)
                        .reduce((sum, option) => sum + (option.price_change || 0), 0) * quantity
                      ).toFixed(2)}
                    </span>
                  </div>
                )}
                <Divider />
                <div className="flex justify-between font-semibold text-lg">
                  <span>{tString('total')}:</span>
                  <span className="text-primary">${totalPrice.toFixed(2)}</span>
                </div>
              </div>
            </CardBody>
          </Card>
        </ModalBody>

        <ModalFooter>
          <Button variant="light" onPress={handleClose}>
            {tString('cancel')}
          </Button>
          <Button color="primary" onPress={handleAddToCart}>
            {tString('addToCart')} - ${totalPrice.toFixed(2)}
          </Button>
        </ModalFooter>
      </ModalContent>
    </Modal>
  );
};