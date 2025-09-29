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
          <h3 className="text-xl font-semibold">Customize Item</h3>
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
                    Base Price: ${(item.price || 0).toFixed(2)}
                  </Chip>
                </div>
              </div>
            </CardBody>
          </Card>

          {/* Quantity Selector */}
          <Card>
            <CardBody className="p-4">
              <h5 className="font-medium mb-3">Quantity</h5>
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
                <h5 className="font-medium mb-3">Add-ons & Options</h5>
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
                            <span className="text-xs text-danger">Required</span>
                          )}
                        </div>
                      </Checkbox>
                      <Chip
                        size="sm"
                        color={option.price_change === 0 ? "default" : option.price_change > 0 ? "success" : "danger"}
                        variant="flat"
                      >
                        {option.price_change === 0 
                          ? "Free" 
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
              <h5 className="font-medium mb-3">Special Requests</h5>
              <Input
                placeholder="Any special instructions or modifications..."
                value={specialRequests}
                onValueChange={setSpecialRequests}
                variant="bordered"
                maxLength={200}
              />
              <p className="text-xs text-default-500 mt-1">
                {specialRequests.length}/200 characters
              </p>
            </CardBody>
          </Card>

          <Divider />

          {/* Price Summary */}
          <Card className="bg-primary-50 border-primary-200">
            <CardBody className="p-4">
              <div className="space-y-2">
                <div className="flex justify-between text-sm">
                  <span>Base Price ({quantity}x):</span>
                  <span>${((item.price || 0) * quantity).toFixed(2)}</span>
                </div>
                {selectedOptions.some(option => option.selected) && (
                  <div className="flex justify-between text-sm">
                    <span>Add-ons ({quantity}x):</span>
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
                  <span>Total:</span>
                  <span className="text-primary">${totalPrice.toFixed(2)}</span>
                </div>
              </div>
            </CardBody>
          </Card>
        </ModalBody>

        <ModalFooter>
          <Button variant="light" onPress={handleClose}>
            Cancel
          </Button>
          <Button
            color="primary"
            onPress={handleAddToCart}
            startContent={<Plus className="w-4 h-4" />}
          >
            Add to Bill - ${totalPrice.toFixed(2)}
          </Button>
        </ModalFooter>
      </ModalContent>
    </Modal>
  );
};

export default ItemCustomizer;
