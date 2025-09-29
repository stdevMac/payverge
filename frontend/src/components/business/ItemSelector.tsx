'use client';

import React, { useState, useEffect } from 'react';
import {
  Card,
  CardBody,
  CardHeader,
  Button,
  Input,
  Chip,
  Image,
  Modal,
  ModalContent,
  ModalHeader,
  ModalBody,
  ModalFooter,
  Spinner,
  Tabs,
  Tab,
} from '@nextui-org/react';
import { Search, Plus, ShoppingCart, X, Settings } from 'lucide-react';
import { MenuCategory, MenuItem, MenuItemOption } from '../../api/business';
import { BillResponse, addBillItem } from '../../api/bills';
import { ItemCustomizer } from './ItemCustomizer';

interface ItemSelectorProps {
  isOpen: boolean;
  onClose: () => void;
  menu: MenuCategory[];
  onItemsAdded?: (selectedItems: { item: MenuItem; quantity: number; selectedOptions?: MenuItemOption[]; specialRequests?: string }[]) => void;
  businessId?: number;
  currentBill?: BillResponse | null;
}

interface SelectedItem extends MenuItem {
  quantity: number;
  categoryIndex: number;
  itemIndex: number;
  selectedOptions?: MenuItemOption[];
  specialRequests?: string;
}

export const ItemSelector: React.FC<ItemSelectorProps> = ({
  isOpen,
  onClose,
  menu,
  onItemsAdded,
  businessId,
  currentBill,
}) => {
  const [searchQuery, setSearchQuery] = useState('');
  const [selectedCategory, setSelectedCategory] = useState<string>('all');
  const [selectedItems, setSelectedItems] = useState<SelectedItem[]>([]);
  const [isAddingItems, setIsAddingItems] = useState(false);
  const [showItemCustomizer, setShowItemCustomizer] = useState(false);
  const [itemToCustomize, setItemToCustomize] = useState<MenuItem & { categoryIndex: number; itemIndex: number } | null>(null);

  // Filter items based on search and category
  const filteredItems = React.useMemo(() => {
    let items: (MenuItem & { categoryName: string; categoryIndex: number; itemIndex: number })[] = [];
    
    menu.forEach((category, categoryIndex) => {
      category.items.forEach((item, itemIndex) => {
        items.push({
          ...item,
          categoryName: category.name,
          categoryIndex,
          itemIndex
        });
      });
    });

    // Filter by search query
    if (searchQuery) {
      items = items.filter(item =>
        item.name.toLowerCase().includes(searchQuery.toLowerCase()) ||
        item.description.toLowerCase().includes(searchQuery.toLowerCase())
      );
    }

    // Filter by category
    if (selectedCategory !== 'all') {
      items = items.filter(item => item.categoryName === selectedCategory);
    }

    return items;
  }, [menu, searchQuery, selectedCategory]);

  const handleAddItem = (item: MenuItem & { categoryIndex: number; itemIndex: number }) => {
    // If item has options/add-ons, open customizer
    if (item.options && item.options.length > 0) {
      setItemToCustomize(item);
      setShowItemCustomizer(true);
      return;
    }

    // Otherwise, add directly
    const existingItem = selectedItems.find(
      selected => selected.name === item.name && selected.categoryIndex === item.categoryIndex
    );

    if (existingItem) {
      setSelectedItems(prev =>
        prev.map(selected =>
          selected.name === item.name && selected.categoryIndex === item.categoryIndex
            ? { ...selected, quantity: selected.quantity + 1 }
            : selected
        )
      );
    } else {
      setSelectedItems(prev => [
        ...prev,
        {
          ...item,
          quantity: 1,
          selectedOptions: [],
          specialRequests: ''
        }
      ]);
    }
  };

  const handleCustomizedItemAdd = (item: MenuItem, quantity: number, selectedOptions: MenuItemOption[], specialRequests: string) => {
    const customizedItem: SelectedItem = {
      ...item,
      quantity,
      selectedOptions,
      specialRequests,
      categoryIndex: itemToCustomize?.categoryIndex || 0,
      itemIndex: itemToCustomize?.itemIndex || 0,
      // Calculate price with add-ons
      price: (item.price || 0) + selectedOptions.reduce((sum, option) => sum + (option.price_change || 0), 0)
    };

    setSelectedItems(prev => [...prev, customizedItem]);
  };

  const handleRemoveItem = (item: SelectedItem) => {
    setSelectedItems(prev => prev.filter(selected => 
      !(selected.name === item.name && selected.categoryIndex === item.categoryIndex)
    ));
  };

  const handleUpdateQuantity = (item: SelectedItem, newQuantity: number) => {
    if (newQuantity <= 0) {
      handleRemoveItem(item);
      return;
    }

    setSelectedItems(prev =>
      prev.map(selected =>
        selected.name === item.name && selected.categoryIndex === item.categoryIndex
          ? { ...selected, quantity: newQuantity }
          : selected
      )
    );
  };

  const handleAddItems = async () => {
    if (selectedItems.length === 0) return;

    setIsAddingItems(true);
    try {
      // If we have a current bill, add items to it via API
      if (currentBill && businessId) {
        for (const item of selectedItems) {
          const addItemRequest = {
            menu_item_id: item.name,
            name: item.name,
            price: item.price,
            quantity: item.quantity,
            options: (item.selectedOptions || []).map(option => ({
              name: option.name,
              price: option.price_change || 0
            })),
            special_requests: item.specialRequests || ''
          };

          await addBillItem(currentBill.bill.id, addItemRequest);
        }
      }

      // Call the callback with selected items for external handling
      if (onItemsAdded) {
        onItemsAdded(selectedItems.map(item => ({
          item: item,
          quantity: item.quantity,
          selectedOptions: item.selectedOptions,
          specialRequests: item.specialRequests
        })));
      }

      setSelectedItems([]);
      onClose?.();
    } catch (error) {
      console.error('Error adding items to bill:', error);
    } finally {
      setIsAddingItems(false);
    }
  };

  const totalAmount = selectedItems.reduce((sum, item) => sum + (item.price * item.quantity), 0);
  const totalItems = selectedItems.reduce((sum, item) => sum + item.quantity, 0);

  if (!isOpen) return null;

  return (
    <Modal
      isOpen={isOpen}
      onClose={onClose}
      size="5xl"
      scrollBehavior="inside"
      classNames={{
        base: "max-h-[90vh]",
        body: "p-0"
      }}
    >
      <ModalContent>
        <ModalHeader className="flex flex-col gap-1">
          <div className="flex items-center justify-between w-full">
            <div>
              <h3 className="text-lg font-semibold">Add Items to Bill</h3>
              {currentBill && (
                <p className="text-sm text-default-500">
                  Bill #{currentBill.bill.bill_number}
                </p>
              )}
            </div>
            {selectedItems.length > 0 && (
              <Chip color="primary" variant="flat">
                {totalItems} items selected
              </Chip>
            )}
          </div>
        </ModalHeader>

        <ModalBody className="p-6">
          {/* Search and Filters */}
          <div className="space-y-4 mb-6">
            <Input
              placeholder="Search menu items..."
              value={searchQuery}
              onValueChange={setSearchQuery}
              startContent={<Search className="w-4 h-4 text-default-400" />}
              variant="bordered"
              isClearable
              onClear={() => setSearchQuery('')}
            />

            <Tabs
              selectedKey={selectedCategory}
              onSelectionChange={(key) => setSelectedCategory(key as string)}
              variant="underlined"
            >
              <Tab key="all" title="All Items" />
              {menu.map((category) => (
                <Tab key={category.name} title={category.name} />
              ))}
            </Tabs>
          </div>

          {/* Items Grid */}
          <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4 mb-6">
            {filteredItems.map((item, index) => (
              <Card key={`${item.categoryIndex}-${item.itemIndex}-${index}`} className="hover:shadow-md transition-shadow">
                <CardBody className="p-4">
                  <div className="flex gap-3">
                    {item.image && (
                      <Image
                        src={item.image}
                        alt={item.name}
                        className="w-16 h-16 object-cover rounded-lg flex-shrink-0"
                      />
                    )}
                    <div className="flex-1 min-w-0">
                      <h4 className="font-medium text-sm truncate">{item.name}</h4>
                      <p className="text-xs text-default-500 line-clamp-2 mb-2">
                        {item.description}
                      </p>
                      <div className="flex items-center justify-between">
                        <span className="font-bold text-primary">
                          ${(item.price || 0).toFixed(2)}
                        </span>
                        <div className="flex gap-1">
                          {item.options && item.options.length > 0 && (
                            <Button
                              size="sm"
                              color="secondary"
                              variant="flat"
                              isIconOnly
                              onPress={() => handleAddItem(item)}
                              isDisabled={!item.is_available}
                              title="Customize"
                            >
                              <Settings className="w-3 h-3" />
                            </Button>
                          )}
                          <Button
                            size="sm"
                            color="primary"
                            variant="flat"
                            isIconOnly
                            onPress={() => handleAddItem(item)}
                            isDisabled={!item.is_available}
                            title={item.options && item.options.length > 0 ? "Customize" : "Add to cart"}
                          >
                            <Plus className="w-3 h-3" />
                          </Button>
                        </div>
                      </div>
                    </div>
                  </div>
                </CardBody>
              </Card>
            ))}
          </div>

          {filteredItems.length === 0 && (
            <div className="text-center py-8">
              <p className="text-default-500">No items found</p>
            </div>
          )}

          {/* Selected Items */}
          {selectedItems.length > 0 && (
            <Card className="border-primary-200 bg-primary-50">
              <CardHeader>
                <h4 className="font-medium">Selected Items</h4>
              </CardHeader>
              <CardBody className="pt-0">
                <div className="space-y-3">
                  {selectedItems.map((item, index) => (
                    <div key={`selected-${item.categoryIndex}-${item.itemIndex}-${index}`} className="flex items-center justify-between">
                      <div className="flex-1">
                        <p className="font-medium text-sm">{item.name}</p>
                        <p className="text-xs text-default-600">
                          ${(item.price || 0).toFixed(2)} each
                        </p>
                        {item.selectedOptions && item.selectedOptions.length > 0 && (
                          <div className="text-xs text-default-500 mt-1">
                            Add-ons: {item.selectedOptions.map(opt => opt.name).join(', ')}
                          </div>
                        )}
                        {item.specialRequests && (
                          <div className="text-xs text-default-500 mt-1">
                            Note: {item.specialRequests}
                          </div>
                        )}
                      </div>
                      <div className="flex items-center gap-2">
                        <Button
                          size="sm"
                          variant="flat"
                          isIconOnly
                          onPress={() => handleUpdateQuantity(item, item.quantity - 1)}
                        >
                          -
                        </Button>
                        <span className="w-8 text-center text-sm font-medium">
                          {item.quantity}
                        </span>
                        <Button
                          size="sm"
                          variant="flat"
                          isIconOnly
                          onPress={() => handleUpdateQuantity(item, item.quantity + 1)}
                        >
                          +
                        </Button>
                        <Button
                          size="sm"
                          color="danger"
                          variant="light"
                          isIconOnly
                          onPress={() => handleRemoveItem(item)}
                        >
                          <X className="w-3 h-3" />
                        </Button>
                      </div>
                    </div>
                  ))}
                </div>
              </CardBody>
            </Card>
          )}
        </ModalBody>

        <ModalFooter>
          <Button variant="light" onPress={onClose}>
            Cancel
          </Button>
          <Button
            color="primary"
            onPress={handleAddItems}
            isLoading={isAddingItems}
            isDisabled={selectedItems.length === 0}
            startContent={!isAddingItems && <ShoppingCart className="w-4 h-4" />}
          >
            Add to Bill (${(totalAmount || 0).toFixed(2)})
          </Button>
        </ModalFooter>
      </ModalContent>

      {/* ItemCustomizer Modal */}
      {showItemCustomizer && itemToCustomize && (
        <ItemCustomizer
          isOpen={showItemCustomizer}
          onClose={() => {
            setShowItemCustomizer(false);
            setItemToCustomize(null);
          }}
          item={itemToCustomize}
          onAddToCart={handleCustomizedItemAdd}
        />
      )}
    </Modal>
  );
};

export default ItemSelector;
