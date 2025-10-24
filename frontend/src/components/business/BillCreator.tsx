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
  Textarea,
  Spinner,
} from '@nextui-org/react';
import { Plus, Minus, ShoppingCart, X, Search } from 'lucide-react';
import { createBill, BillItem, CreateBillRequest, getOpenBusinessBills, Bill } from '../../api/bills';
import { Business, getBusinessTables, Table, getMenu } from '../../api/business';
import { MenuCategory, MenuItem, MenuItemOption } from '../../api/business';
import { ItemCustomizer } from './ItemCustomizer';
import { createOrder, CreateOrderRequest } from '../../api/orders';
import { getAvailableCounters, Counter } from '../../api/counters';
import { useSimpleLocale, getTranslation } from '@/i18n/SimpleTranslationProvider';

interface BillCreatorProps {
  isOpen: boolean;
  onClose: () => void;
  businessId: number;
  onBillCreated: () => void;
}

interface SelectedItem extends BillItem {
  menuItem: MenuItem;
  selectedOptions?: MenuItemOption[];
  specialRequests?: string;
}

export const BillCreator: React.FC<BillCreatorProps> = ({
  isOpen,
  onClose,
  businessId,
  onBillCreated,
}) => {
  const { locale: currentLocale } = useSimpleLocale();
  
  // Translation helper
  const tString = (key: string): string => {
    const fullKey = `billCreator.${key}`;
    const result = getTranslation(fullKey, currentLocale);
    return Array.isArray(result) ? result[0] || key : result as string;
  };
  
  const [selectedTable, setSelectedTable] = useState<Table | null>(null);
  const [selectedCounter, setSelectedCounter] = useState<Counter | null>(null);
  const [tables, setTables] = useState<Table[]>([]);
  const [counters, setCounters] = useState<Counter[]>([]);
  const [bills, setBills] = useState<Bill[]>([]);
  const [menu, setMenu] = useState<{ categories: MenuCategory[] }>({ categories: [] });
  const [items, setItems] = useState<SelectedItem[]>([]);
  const [isCreating, setIsCreating] = useState(false);
  const [loadingData, setLoadingData] = useState(false);
  const [searchQuery, setSearchQuery] = useState('');
  const [notes, setNotes] = useState('');
  const [showItemCustomizer, setShowItemCustomizer] = useState(false);
  const [itemToCustomize, setItemToCustomize] = useState<MenuItem | null>(null);

  const loadData = useCallback(async () => {
    setLoadingData(true);
    try {
      const [tablesResponse, menuResponse, billsResponse] = await Promise.all([
        getBusinessTables(businessId),
        getMenu(businessId),
        getOpenBusinessBills(businessId),
      ]);
      setTables(tablesResponse.tables || []);
      setBills(billsResponse.bills || []);
      
      // Debug logging
      if (process.env.NODE_ENV === 'development') {
        console.log('Loaded data:', {
          tables: tablesResponse.tables?.length || 0,
          bills: billsResponse.bills?.length || 0,
          billStatuses: billsResponse.bills?.map(bill => ({ id: bill.id, status: bill.status, table_id: bill.table_id })) || []
        });
      }
      
      // Try to load counters, but don't fail if they're not available
      try {
        const countersResponse = await getAvailableCounters(businessId);
        setCounters(countersResponse.counters || []);
      } catch (error) {
        console.log('Counters not available or not enabled for this business');
        setCounters([]);
      }
      
      // Safely handle menu response
      if (menuResponse && menuResponse.categories) {
        let categories;
        
        // Handle string or array categories
        if (typeof menuResponse.categories === 'string') {
          try {
            categories = JSON.parse(menuResponse.categories);
          } catch (parseError) {
            console.error('Failed to parse menu categories:', parseError);
            categories = [];
          }
        } else {
          categories = menuResponse.categories;
        }
        
        // Ensure categories is an array
        if (Array.isArray(categories)) {
          setMenu({ categories });
        } else {
          console.error('Menu categories is not an array:', categories);
          setMenu({ categories: [] });
        }
      } else {
        setMenu({ categories: [] });
      }
    } catch (error) {
      console.error('Error loading data:', error);
      setMenu({ categories: [] }); // Ensure safe fallback
      setBills([]); // Ensure safe fallback
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

  const handleItemClick = (menuItem: MenuItem) => {
    // If item has options/add-ons, open customizer
    if (menuItem.options && menuItem.options.length > 0) {
      setItemToCustomize(menuItem);
      setShowItemCustomizer(true);
      return;
    }

    // Otherwise, add directly with quantity 1
    handleItemAdded(menuItem, 1);
  };

  const handleItemAdded = (menuItem: MenuItem, quantity: number, selectedOptions?: MenuItemOption[], specialRequests?: string) => {
    // Calculate price including add-ons
    const addOnPrice = selectedOptions ? selectedOptions.reduce((sum, option) => sum + (option.price_change || 0), 0) : 0;
    const totalPrice = menuItem.price + addOnPrice;
    
    const newItem: SelectedItem = {
      id: `${Date.now()}-${Math.random()}`,
      menu_item_id: menuItem.name,
      name: menuItem.name,
      price: totalPrice,
      quantity: quantity,
      options: selectedOptions ? selectedOptions.map(option => ({
        name: option.name,
        price: option.price_change || 0
      })) : [],
      subtotal: totalPrice * quantity,
      menuItem,
      selectedOptions,
      specialRequests,
    };
    setItems([...items, newItem]);
  };

  const handleCustomizedItemAdd = (item: MenuItem, quantity: number, selectedOptions: MenuItemOption[], specialRequests: string) => {
    handleItemAdded(item, quantity, selectedOptions, specialRequests);
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
    if ((!selectedTable && !selectedCounter) || items.length === 0) return;

    setIsCreating(true);
    try {
      // First, create the bill
      const billData: CreateBillRequest = {
        table_id: selectedTable?.id,
        counter_id: selectedCounter?.id,
        notes: notes.trim(),
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

      const createdBill = await createBill(businessId, billData);

      // Then, create the kitchen order
      const orderLocation = selectedTable ? `Table ${selectedTable.name}` : `Counter ${selectedCounter?.name}`;
      const orderNotes = notes.trim() 
        ? `Order for ${orderLocation}\n\nNotes: ${notes.trim()}`
        : `Order for ${orderLocation}`;
      const orderData: CreateOrderRequest = {
        bill_id: createdBill.bill.id,
        notes: orderNotes,
        items: items.map((item: SelectedItem) => ({
          menu_item_name: item.name,
          quantity: item.quantity,
          price: item.price,
          options: (item.selectedOptions || []).map(option => ({
            ...option,
            id: option.id || `option-${option.name.toLowerCase().replace(/\s+/g, '-')}`,
            is_required: option.is_required || false
          })), // Include selected add-ons/options with proper IDs
          special_requests: item.specialRequests || '',
        })),
      };

      await createOrder(businessId, orderData);

      onBillCreated();
      resetForm();
    } catch (error) {
      console.error('Error creating bill and order:', error);
    } finally {
      setIsCreating(false);
    }
  };

  const resetForm = () => {
    setSelectedTable(null);
    setSelectedCounter(null);
    setItems([]);
    setSearchQuery('');
    setNotes('');
    setBills([]);
    setCounters([]);
    onClose();
  };

  // Filter menu items based on search query
  const filteredMenu = React.useMemo(() => {
    if (!searchQuery.trim()) {
      return menu.categories;
    }

    const query = searchQuery.toLowerCase().trim();
    return menu.categories.map(category => ({
      ...category,
      items: category.items.filter(item =>
        item.name.toLowerCase().includes(query) ||
        item.description.toLowerCase().includes(query)
      )
    })).filter(category => category.items.length > 0);
  }, [menu.categories, searchQuery]);

  const { subtotal } = calculateTotals();
  
  // Filter out tables that already have open bills
  const availableTables = tables.filter(table => {
    if (!table.is_active) return false;
    
    // Check if table has any open bills (status not 'paid' or 'closed')
    const hasOpenBill = bills.some(bill => 
      bill.table_id === table.id && 
      bill.status !== 'paid' && 
      bill.status !== 'closed'
    );
    
    // Debug logging
    if (process.env.NODE_ENV === 'development') {
      console.log(`Table ${table.name} (ID: ${table.id}):`, {
        is_active: table.is_active,
        hasOpenBill,
        relatedBills: bills.filter(bill => bill.table_id === table.id).map(bill => ({
          id: bill.id,
          status: bill.status,
          table_id: bill.table_id
        }))
      });
    }
    
    return !hasOpenBill;
  });

  return (
    <Modal
      isOpen={isOpen}
      onClose={resetForm}
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
            {tString('title')}
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
                  <CardHeader className="pb-2">
                    <div className="w-full space-y-3">
                      <h3 className="text-lg font-semibold">{tString('menu.title')}</h3>
                      <Input
                        placeholder={tString('menu.searchPlaceholder')}
                        value={searchQuery}
                        onValueChange={setSearchQuery}
                        startContent={<Search className="w-4 h-4 text-default-400" />}
                        variant="bordered"
                        size="sm"
                        isClearable
                        onClear={() => setSearchQuery('')}
                      />
                    </div>
                  </CardHeader>
                  <CardBody className="max-h-96 overflow-y-auto pt-2">
                    {filteredMenu.length === 0 ? (
                      <div className="text-center py-8">
                        <Search className="w-12 h-12 mx-auto text-default-300 mb-4" />
                        <h4 className="text-lg font-medium text-default-500 mb-2">
                          {searchQuery ? tString('menu.noItemsFound') : tString('menu.noMenuItems')}
                        </h4>
                        <p className="text-default-400">
                          {searchQuery 
                            ? tString('menu.noItemsMatch').replace('{query}', searchQuery)
                            : tString('menu.noItemsAvailable')
                          }
                        </p>
                        {searchQuery && (
                          <Button
                            variant="light"
                            color="primary"
                            size="sm"
                            onPress={() => setSearchQuery('')}
                            className="mt-3"
                          >
                            {tString('menu.clearSearch')}
                          </Button>
                        )}
                      </div>
                    ) : (
                      filteredMenu.map((category, categoryIndex) => (
                        <div key={categoryIndex} className="mb-4">
                          <h4 className="font-medium text-medium mb-2">
                            {category.name}
                            {searchQuery && (
                              <Chip size="sm" variant="flat" color="primary" className="ml-2">
                                {category.items.length} {category.items.length !== 1 ? tString('menu.items') : tString('menu.item')}
                              </Chip>
                            )}
                          </h4>
                          <div className="grid grid-cols-1 gap-2">
                            {category.items.map((item, itemIndex) => (
                              <Card
                                key={itemIndex}
                                isPressable
                                onPress={() => handleItemClick(item)}
                                className="hover:bg-default-100"
                              >
                                <CardBody className="p-3">
                                  <div className="flex justify-between items-center">
                                    <div className="flex-1">
                                      <div className="flex items-center gap-2">
                                        <p className="font-medium">{item.name}</p>
                                        {item.options && item.options.length > 0 && (
                                          <Chip size="sm" color="secondary" variant="dot">
                                            {tString('menu.addOns')}
                                          </Chip>
                                        )}
                                      </div>
                                      {item.description && (
                                        <p className="text-small text-default-500">{item.description}</p>
                                      )}
                                    </div>
                                    <div className="flex items-center gap-2">
                                      <Chip color="primary" variant="flat">
                                        ${(item.price || 0).toFixed(2)}
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
                      ))
                    )}
                  </CardBody>
                </Card>
              </div>

              {/* Bill Summary */}
              <div className="w-80">
                <Card>
                  <CardHeader>
                    <h3 className="text-lg font-semibold">{tString('billSummary.title')}</h3>
                  </CardHeader>
                  <CardBody>
                    {/* Table/Counter Selection */}
                    <div className="mb-4 space-y-4">
                      {/* Table Selection */}
                      <div>
                        <Select
                          label={tString('form.selectTable')}
                          placeholder={availableTables.length > 0 ? tString('form.chooseTable') : tString('form.noTablesAvailable')}
                          selectedKeys={selectedTable ? [selectedTable.id.toString()] : []}
                          onSelectionChange={(keys) => {
                            const key = Array.from(keys)[0] as string;
                            const table = availableTables.find(t => t.id.toString() === key);
                            setSelectedTable(table || null);
                            if (table) setSelectedCounter(null); // Clear counter selection
                          }}
                          isDisabled={availableTables.length === 0 || selectedCounter !== null}
                        >
                          {availableTables.map((table) => (
                            <SelectItem key={table.id.toString()} value={table.id.toString()}>
                              {table.name}
                            </SelectItem>
                          ))}
                        </Select>
                        {availableTables.length === 0 && counters.length === 0 && (
                          <p className="text-small text-warning mt-2">
                            {tString('form.allTablesHaveBills')}
                          </p>
                        )}
                        {availableTables.length === 0 && counters.length > 0 && !selectedCounter && (
                          <p className="text-small text-default-400 mt-2">
                            {tString('form.allTablesOccupied')}
                          </p>
                        )}
                      </div>

                      {/* Counter Selection */}
                      <div>
                        <div className="flex items-center justify-center mb-2">
                          <div className="flex-1 border-t border-gray-300"></div>
                          <span className="px-3 text-small text-gray-500">{tString('form.or')}</span>
                          <div className="flex-1 border-t border-gray-300"></div>
                        </div>
                        
                        {counters.length > 0 ? (
                          <div>
                            <Select
                              label={tString('form.selectCounter')}
                              placeholder={tString('form.chooseCounter')}
                              selectedKeys={selectedCounter ? [selectedCounter.id.toString()] : []}
                              onSelectionChange={(keys) => {
                                const key = Array.from(keys)[0] as string;
                                const counter = counters.find(c => c.id.toString() === key);
                                setSelectedCounter(counter || null);
                                if (counter) setSelectedTable(null); // Clear table selection
                              }}
                              isDisabled={selectedTable !== null}
                            >
                              {counters.map((counter) => (
                                <SelectItem key={counter.id.toString()} value={counter.id.toString()}>
                                  {counter.name}
                                </SelectItem>
                              ))}
                            </Select>
                            <p className="text-small text-default-400 mt-1">
                              {tString('form.perfectForTakeaway')}
                            </p>
                          </div>
                        ) : (
                          <div className="text-center py-4">
                            <p className="text-small text-default-500 mb-2">
                              {tString('form.noCountersAvailable')}
                            </p>
                            <p className="text-tiny text-default-400">
                              {tString('form.enableCountersMessage')}
                            </p>
                          </div>
                        )}
                      </div>

                      {/* Order Notes */}
                      <div>
                        <Textarea
                          label={tString('form.orderNotes')}
                          placeholder={tString('form.orderNotesPlaceholder')}
                          value={notes}
                          onValueChange={setNotes}
                          variant="bordered"
                          minRows={2}
                          maxRows={4}
                          description={tString('form.orderNotesDescription')}
                        />
                      </div>
                    </div>

                    <Divider className="mb-4" />

                    {/* Selected Items */}
                    <div className="mb-4">
                      <h4 className="font-medium mb-2">{tString('billSummary.items')} ({items.length})</h4>
                      <div className="max-h-48 overflow-y-auto space-y-2">
                        {items.map((item: SelectedItem) => (
                          <Card key={item.id} className="p-2">
                            <div className="flex justify-between items-start">
                              <div className="flex-1">
                                <p className="font-medium text-sm">{item.name}</p>
                                <p className="text-xs text-default-500">
                                  ${(item.price || 0).toFixed(2)} {tString('billSummary.each')}
                                </p>
                                {item.selectedOptions && item.selectedOptions.length > 0 && (
                                  <div className="text-xs text-default-400 mt-1">
                                    {tString('billSummary.addOns')}: {item.selectedOptions.map(opt => opt.name).join(', ')}
                                  </div>
                                )}
                                {item.specialRequests && (
                                  <div className="text-xs text-default-400 mt-1">
                                    {tString('billSummary.note')}: {item.specialRequests}
                                  </div>
                                )}
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
                                {item.quantity} × ${(item.price || 0).toFixed(2)}
                              </span>
                              <span className="font-medium text-sm">
                                ${(item.subtotal || 0).toFixed(2)}
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
                        <span>{tString('billSummary.subtotal')}</span>
                        <span>${(subtotal || 0).toFixed(2)}</span>
                      </div>
                    </div>
                  </CardBody>
                </Card>
              </div>
            </div>
          )}
        </ModalBody>
        <ModalFooter>
          <Button variant="light" onPress={resetForm}>
            {tString('buttons.cancel')}
          </Button>
          <Button
            color="primary"
            onPress={handleCreateBill}
            isLoading={isCreating}
            isDisabled={(!selectedTable && !selectedCounter) || items.length === 0 || isCreating}
          >
            {tString('buttons.createBill')}
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
