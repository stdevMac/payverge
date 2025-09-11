import React, { useState, useEffect, useCallback } from 'react';
import {
  Modal,
  ModalContent,
  ModalHeader,
  ModalBody,
  ModalFooter,
  Button,
  Select,
  SelectItem,
  Card,
  CardBody,
  CardHeader,
  Divider,
  Chip,
  Input,
  Spinner,
} from '@nextui-org/react';
import { Plus, Minus, ShoppingCart, X } from 'lucide-react';
import { createBill, BillItem, CreateBillRequest } from '../../api/bills';
import { Business, getBusinessTables, Table, getMenu } from '../../api/business';
import ItemSelector from './ItemSelector';
import { MenuCategory, MenuItem } from '../../api/business';

interface BillCreatorProps {
  isOpen: boolean;
  onClose: () => void;
  businessId: number;
  onBillCreated: () => void;
}

interface SelectedItem extends BillItem {
  menuItem: MenuItem;
}

export const BillCreator: React.FC<BillCreatorProps> = ({
  isOpen,
  onClose,
  businessId,
  onBillCreated,
}) => {
  const [selectedTable, setSelectedTable] = useState<Table | null>(null);
  const [tables, setTables] = useState<Table[]>([]);
  const [menu, setMenu] = useState<{ categories: MenuCategory[] }>({ categories: [] });
  const [items, setItems] = useState<SelectedItem[]>([]);
  const [isCreating, setIsCreating] = useState(false);
  const [loadingData, setLoadingData] = useState(false);
  const [showItemSelector, setShowItemSelector] = useState(false);

  const loadData = useCallback(async () => {
    setLoadingData(true);
    try {
      const [tablesResponse, menuResponse] = await Promise.all([
        getBusinessTables(businessId),
        getMenu(businessId),
      ]);
      setTables(tablesResponse.tables || []);
      setMenu(menuResponse || { categories: [] });
    } catch (error) {
      console.error('Error loading data:', error);
    } finally {
      setLoadingData(false);
    }
  }, [businessId]);

  // Load tables and menu when modal opens
  useEffect(() => {
    if (isOpen && businessId) {
      loadData();
    }
  }, [isOpen, businessId, loadData]);

  const handleItemAdded = (menuItem: MenuItem, quantity: number) => {
    const newItem: SelectedItem = {
      id: `${Date.now()}-${Math.random()}`,
      menu_item_id: menuItem.name,
      name: menuItem.name,
      price: menuItem.price,
      quantity: quantity,
      options: [],
      subtotal: menuItem.price * quantity,
      menuItem,
    };
    setItems([...items, newItem]);
  };

  const updateItemQuantity = (itemId: string, quantity: number) => {
    if (quantity <= 0) {
      removeItem(itemId);
      return;
    }
    setItems((items: SelectedItem[]) =>
      items.map((item: SelectedItem) =>
        item.id === itemId
          ? { ...item, quantity, subtotal: item.price * quantity }
          : item
      )
    );
  };

  const removeItem = (itemId: string) => {
    setItems((items: SelectedItem[]) => items.filter((item: SelectedItem) => item.id !== itemId));
  };

  const calculateTotals = () => {
    const subtotal = items.reduce((sum: number, item: SelectedItem) => sum + item.subtotal, 0);
    return { subtotal };
  };

  const handleCreateBill = async () => {
    if (!selectedTable || items.length === 0) return;

    setIsCreating(true);
    try {
      const billData: CreateBillRequest = {
        table_id: selectedTable.id,
        items: items.map((item: SelectedItem) => ({
          id: item.id,
          menu_item_id: item.menu_item_id,
          name: item.name,
          price: item.price,
          quantity: item.quantity,
          options: item.options,
          subtotal: item.subtotal,
        })),
      };

      await createBill(businessId, billData);
      onBillCreated();
      handleClose();
    } catch (error) {
      console.error('Error creating bill:', error);
    } finally {
      setIsCreating(false);
    }
  };

  const handleClose = () => {
    setSelectedTable(null);
    setItems([]);
    onClose();
  };

  const { subtotal } = calculateTotals();
  const availableTables = tables.filter(table => table.is_active);

  return (
    <Modal
      isOpen={isOpen}
      onClose={handleClose}
      size="5xl"
      scrollBehavior="inside"
      classNames={{
        base: "max-h-[90vh]",
        body: "p-0",
      }}
    >
      <ModalContent>
        <ModalHeader className="flex flex-col gap-1">
          <div className="flex items-center gap-2">
            <ShoppingCart className="w-5 h-5" />
            Create New Bill
          </div>
        </ModalHeader>
        <ModalBody>
          {loadingData ? (
            <div className="flex justify-center items-center py-8">
              <Spinner size="lg" />
            </div>
          ) : (
            <div className="flex gap-4 p-4">
              {/* Menu Selection */}
              <div className="flex-1">
                <Card>
                  <CardHeader>
                    <h3 className="text-lg font-semibold">Menu Items</h3>
                  </CardHeader>
                  <CardBody className="max-h-96 overflow-y-auto">
                    {menu.categories.map((category, categoryIndex) => (
                      <div key={categoryIndex} className="mb-4">
                        <h4 className="font-medium text-medium mb-2">{category.name}</h4>
                        <div className="grid grid-cols-1 gap-2">
                          {category.items.map((item, itemIndex) => (
                            <Card
                              key={itemIndex}
                              isPressable
                              onPress={() => handleItemAdded(item, 1)}
                              className="hover:bg-default-100"
                            >
                              <CardBody className="p-3">
                                <div className="flex justify-between items-center">
                                  <div>
                                    <p className="font-medium">{item.name}</p>
                                    {item.description && (
                                      <p className="text-small text-default-500">{item.description}</p>
                                    )}
                                  </div>
                                  <div className="flex items-center gap-2">
                                    <Chip color="primary" variant="flat">
                                      ${item.price.toFixed(2)}
                                    </Chip>
                                    <Button
                                      isIconOnly
                                      size="sm"
                                      color="primary"
                                      variant="light"
                                    >
                                      <Plus className="w-4 h-4" />
                                    </Button>
                                  </div>
                                </div>
                              </CardBody>
                            </Card>
                          ))}
                        </div>
                      </div>
                    ))}
                  </CardBody>
                </Card>
              </div>

              {/* Bill Summary */}
              <div className="w-80">
                <Card>
                  <CardHeader>
                    <h3 className="text-lg font-semibold">Bill Summary</h3>
                  </CardHeader>
                  <CardBody>
                    {/* Table Selection */}
                    <div className="mb-4">
                      <Select
                        label="Select Table"
                        placeholder="Choose a table"
                        selectedKeys={selectedTable ? [selectedTable.id.toString()] : []}
                        onSelectionChange={(keys) => {
                          const key = Array.from(keys)[0] as string;
                          const table = availableTables.find(t => t.id.toString() === key);
                          setSelectedTable(table || null);
                        }}
                      >
                        {availableTables.map((table) => (
                          <SelectItem key={table.id.toString()} value={table.id.toString()}>
                            {table.name}
                          </SelectItem>
                        ))}
                      </Select>
                    </div>

                    <Divider className="mb-4" />

                    {/* Add Items Button */}
                    <div className="mb-4">
                      <Button
                        color="primary"
                        variant="flat"
                        startContent={<Plus size={16} />}
                        onPress={() => setShowItemSelector(true)}
                        className="w-full"
                      >
                        Add Items from Menu
                      </Button>
                    </div>

                    <Divider className="mb-4" />

                    {/* Selected Items */}
                    <div className="mb-4">
                      <h4 className="font-medium mb-2">Items ({items.length})</h4>
                      <div className="max-h-48 overflow-y-auto space-y-2">
                        {items.map((item: SelectedItem) => (
                          <Card key={item.id} className="p-2">
                            <div className="flex justify-between items-start">
                              <div className="flex-1">
                                <p className="font-medium text-sm">{item.name}</p>
                                <p className="text-xs text-default-500">
                                  ${item.price.toFixed(2)} each
                                </p>
                              </div>
                              <div className="flex items-center gap-1">
                                <Button
                                  isIconOnly
                                  size="sm"
                                  variant="light"
                                  onPress={() => updateItemQuantity(item.id, item.quantity - 1)}
                                >
                                  <Minus className="w-3 h-3" />
                                </Button>
                                <Input
                                  size="sm"
                                  className="w-12"
                                  value={item.quantity.toString()}
                                  onChange={(e) => {
                                    const qty = parseInt(e.target.value) || 0;
                                    updateItemQuantity(item.id, qty);
                                  }}
                                />
                                <Button
                                  isIconOnly
                                  size="sm"
                                  variant="light"
                                  onPress={() => updateItemQuantity(item.id, item.quantity + 1)}
                                >
                                  <Plus className="w-3 h-3" />
                                </Button>
                                <Button
                                  isIconOnly
                                  size="sm"
                                  color="danger"
                                  variant="light"
                                  onPress={() => removeItem(item.id)}
                                >
                                  <X className="w-3 h-3" />
                                </Button>
                              </div>
                            </div>
                            <div className="flex justify-between items-center mt-1">
                              <span className="text-xs text-default-500">
                                {item.quantity} × ${item.price.toFixed(2)}
                              </span>
                              <span className="font-medium text-sm">
                                ${item.subtotal.toFixed(2)}
                              </span>
                            </div>
                          </Card>
                        ))}
                      </div>
                    </div>

                    <Divider className="mb-4" />

                    {/* Totals */}
                    <div className="space-y-2">
                      <div className="flex justify-between">
                        <span>Subtotal:</span>
                        <span>${subtotal.toFixed(2)}</span>
                      </div>
                    </div>
                  </CardBody>
                </Card>
              </div>
            </div>
          )}
        </ModalBody>
        <ModalFooter>
          <Button variant="light" onPress={handleClose}>
            Cancel
          </Button>
          <Button
            color="primary"
            onPress={handleCreateBill}
            isLoading={isCreating}
            isDisabled={!selectedTable || items.length === 0 || isCreating}
          >
            Create Bill
          </Button>
        </ModalFooter>
      </ModalContent>

      {/* ItemSelector Modal */}
      {showItemSelector && (
        <ItemSelector
          isOpen={showItemSelector}
          onClose={() => setShowItemSelector(false)}
          menu={menu.categories}
          onItemsAdded={(selectedItems: { item: MenuItem; quantity: number }[]) => {
            selectedItems.forEach(({ item, quantity }: { item: MenuItem; quantity: number }) => {
              handleItemAdded(item, quantity);
            });
            setShowItemSelector(false);
          }}
        />
      )}
    </Modal>
  );
};
