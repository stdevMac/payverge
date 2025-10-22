'use client';

import React, { useState, useEffect, useCallback, useMemo } from 'react';
import { Card, CardBody, Button, Spinner, Image, Badge, Modal, ModalContent, ModalHeader, ModalBody, ModalFooter, Divider } from '@nextui-org/react';
import { ArrowLeft, MapPin, ShoppingCart, Plus, Minus, ChefHat } from 'lucide-react';
import { useParams } from 'next/navigation';
import Link from 'next/link';
import dynamic from 'next/dynamic';
import PersistentGuestNav from '../../../../components/navigation/PersistentGuestNav';
import { GuestTranslationProvider } from '../../../../i18n/GuestTranslationProvider';

// Lazy load heavy components
const GuestMenu = dynamic(() => import('../../../../components/guest/GuestMenu'), {
  loading: () => <div className="flex justify-center p-8"><Spinner size="lg" /></div>
});
import { BillWithItemsResponse, getTableByCode, getOpenBillByTableCode, createBillByTableCode, getMenuByTableCode } from '../../../../api/bills';
import { Business, MenuCategory, businessApi } from '../../../../api/business';
import { createGuestOrder, getOrdersByBillId } from '../../../../api/orders';
import { useGuestTranslation } from '../../../../i18n/GuestTranslationProvider';
import { FloatingLanguageSelector } from '@/components/guest/FloatingLanguageSelector';

interface Table {
  id: number;
  business_id: number;
  name: string;
  code: string;
  seats: number;
  status: string;
  qr_code_url: string;
}

interface TableData {
  table: Table;
  business: Business;
  menu: {
    categories: string;
  };
  categories: MenuCategory[];
}

interface CartItem {
  name: string;
  price: number;
  quantity: number;
  specialRequests?: string;
  addOns?: Array<{
    name: string;
    price: number;
  }>;
}

// Internal component that uses the hook
function GuestMenuPageContent() {
  const { t, setBusinessId } = useGuestTranslation();
  const params = useParams();
  const tableCode = params.tableCode as string;
  
  const [tableData, setTableData] = useState<TableData | null>(null);
  const [currentBill, setCurrentBill] = useState<BillWithItemsResponse | null>(null);
  const [loading, setLoading] = useState(true);
  const [cart, setCart] = useState<CartItem[]>([]);
  const [showCart, setShowCart] = useState(false);
  
  // Currency settings
  const [businessCurrencies, setBusinessCurrencies] = useState({
    default_currency: 'USD',
    display_currency: 'USD'
  });
  const [orderLoading, setOrderLoading] = useState(false);
  const [selectedLanguage, setSelectedLanguage] = useState<string>('');
  const [translatedCategories, setTranslatedCategories] = useState<MenuCategory[]>([]);

  const loadTableData = useCallback(async () => {
    setLoading(true);
    try {
      // Load table data and bill data in parallel for better performance
      const [tableResponse, billResponse] = await Promise.allSettled([
        getTableByCode(tableCode),
        getOpenBillByTableCode(tableCode)
      ]);

      if (tableResponse.status === 'fulfilled') {
        setTableData(tableResponse.value);
        
        // Set business ID for translation provider
        if (tableResponse.value?.business?.id) {
          setBusinessId(tableResponse.value.business.id);
        }
        
        // Load business currency settings
        try {
          const business = await businessApi.getBusiness(tableResponse.value.business.id);
          setBusinessCurrencies({
            default_currency: business.default_currency || 'USD',
            display_currency: business.display_currency || 'USD'
          });
        } catch (error) {
          console.error('Error loading business currency settings:', error);
        }
      } else {
        console.error('Error loading table data:', tableResponse.reason);
      }

      if (billResponse.status === 'fulfilled') {
        setCurrentBill(billResponse.value);
      } else {
        setCurrentBill(null);
      }
    } catch (error) {
      console.error('Error loading data:', error);
    } finally {
      setLoading(false);
    }
  }, [tableCode]);

  const loadTranslatedMenu = useCallback(async (languageCode: string) => {
    console.log('Loading translated menu for table:', tableCode, 'language:', languageCode);
    try {
      const menuData = await getMenuByTableCode(tableCode, languageCode);
      console.log('Menu data received:', {
        hasCategories: !!menuData.categories,
        hasParsedCategories: !!menuData.parsed_categories,
        categoriesType: typeof menuData.categories,
        language: menuData.language
      });
      
      // Use translated categories if available, otherwise fall back to original
      const categories = menuData.parsed_categories || menuData.categories;
      
      console.log('Setting translated categories:', {
        categoriesLength: categories?.length,
        firstCategoryName: categories?.[0]?.name
      });
      
      setTranslatedCategories(categories || []);
    } catch (error) {
      console.error('Error loading translated menu:', error);
      // Fall back to original categories if translation fails
      if (tableData?.categories) {
        console.log('Falling back to original categories');
        setTranslatedCategories(tableData.categories);
      }
    }
  }, [tableCode, tableData?.categories]);

  const handleLanguageChange = useCallback((languageCode: string) => {
    console.log('Menu page: Language changed to:', languageCode);
    setSelectedLanguage(languageCode);
    loadTranslatedMenu(languageCode);
  }, [loadTranslatedMenu]);

  // Initialize language and menu data when table data loads
  useEffect(() => {
    if (tableData?.business?.id && tableData?.categories) {
      const savedLanguage = localStorage.getItem(`guest-language-${tableData.business.id}`);
      
      if (savedLanguage && !selectedLanguage) {
        console.log('Menu page: Restoring saved language and loading translated menu:', savedLanguage);
        setSelectedLanguage(savedLanguage);
        // Load translated menu immediately
        loadTranslatedMenu(savedLanguage);
      } else if (!selectedLanguage) {
        console.log('No saved language, initializing with original categories');
        setTranslatedCategories(tableData.categories);
      }
    }
  }, [tableData?.business?.id, tableData?.categories, selectedLanguage, loadTranslatedMenu]);

  // Listen for language changes from FloatingLanguageSelector
  useEffect(() => {
    const handleGuestLanguageChange = (event: CustomEvent) => {
      const { language, businessId } = event.detail;
      console.log('Menu page: Received guestLanguageChange event:', { language, businessId });
      
      // Only handle if it's for our business
      if (businessId === tableData?.business?.id) {
        handleLanguageChange(language);
      }
    };

    window.addEventListener('guestLanguageChange', handleGuestLanguageChange as EventListener);
    
    return () => {
      window.removeEventListener('guestLanguageChange', handleGuestLanguageChange as EventListener);
    };
  }, [handleLanguageChange, tableData?.business?.id]);

  const addToCart = useCallback((
    itemName: string, 
    price: number, 
    quantity: number = 1, 
    specialRequests?: string,
    addOns?: Array<{ name: string; price: number }>
  ) => {
    setCart(prevCart => {
      // Create a unique identifier that includes add-ons
      const addOnsString = addOns ? addOns.map(addon => addon.name).sort().join(',') : '';
      const existingItem = prevCart.find(item => 
        item.name === itemName && 
        item.specialRequests === specialRequests &&
        (item.addOns ? item.addOns.map(addon => addon.name).sort().join(',') : '') === addOnsString
      );
      
      if (existingItem) {
        return prevCart.map(item =>
          item.name === itemName && 
          item.specialRequests === specialRequests &&
          (item.addOns ? item.addOns.map(addon => addon.name).sort().join(',') : '') === addOnsString
            ? { ...item, quantity: item.quantity + quantity }
            : item
        );
      } else {
        return [...prevCart, { name: itemName, price, quantity, specialRequests, addOns }];
      }
    });
  }, []);

  const updateCartItemQuantity = useCallback((index: number, newQuantity: number) => {
    if (newQuantity <= 0) {
      setCart(prevCart => prevCart.filter((_, i) => i !== index));
    } else {
      setCart(prevCart => 
        prevCart.map((item, i) => 
          i === index ? { ...item, quantity: newQuantity } : item
        )
      );
    }
  }, []);

  const clearCart = useCallback(() => {
    setCart([]);
  }, []);

  // Memoize expensive cart calculations
  const cartTotal = useMemo(() => {
    return cart.reduce((total, item) => {
      const baseTotal = item.price * item.quantity;
      const addOnsTotal = item.addOns 
        ? item.addOns.reduce((addOnSum, addon) => addOnSum + (addon.price * item.quantity), 0)
        : 0;
      return total + baseTotal + addOnsTotal;
    }, 0);
  }, [cart]);

  const cartItemCount = useMemo(() => {
    return cart.reduce((total, item) => total + item.quantity, 0);
  }, [cart]);

  // Memoize business data to prevent unnecessary re-renders
  const businessData = useMemo(() => tableData?.business, [tableData?.business]);

  const handleCreateBill = useCallback(async () => {
    if (!tableData) return;
    
    setOrderLoading(true);
    try {
      const newBill = await createBillByTableCode(tableCode);
      // Reload the bill with items after creation
      const billWithItems = await getOpenBillByTableCode(tableCode);
      setCurrentBill(billWithItems);
      
      // Create guest order for approval
      const kitchenOrderItems = cart.map(item => {
        let itemName = item.name;
        let totalPrice = item.price;
        let specialRequests = item.specialRequests || '';
        
        // Include add-ons in the item name and price
        if (item.addOns && item.addOns.length > 0) {
          const addOnNames = item.addOns.map(addon => addon.name).join(', ');
          itemName = `${item.name} (${addOnNames})`;
          totalPrice = item.price + item.addOns.reduce((sum, addon) => sum + addon.price, 0);
          
          // Add add-ons to special requests if not already there
          const addOnDetails = item.addOns.map(addon => 
            `${addon.name}${addon.price > 0 ? ` (+$${addon.price.toFixed(2)})` : ''}`
          ).join(', ');
          specialRequests = specialRequests 
            ? `${specialRequests}. Add-ons: ${addOnDetails}`
            : `Add-ons: ${addOnDetails}`;
        }
        
        return {
          menu_item_name: itemName,
          quantity: item.quantity,
          price: totalPrice,
          special_requests: specialRequests,
        };
      });

      console.log('Creating initial kitchen order with data:', {
        business_id: tableData.business.id,
        bill_id: newBill.bill.id,
        table_id: tableData.table.id,
        items: kitchenOrderItems
      });

      try {
        const orderResult = await createGuestOrder(tableCode, {
          bill_id: newBill.bill.id,
          items: kitchenOrderItems,
          notes: `Table ${tableData.table.name} - Initial Order`,
        });
        
        console.log('Order created successfully and pending approval:', orderResult);
      } catch (orderError) {
        console.error('Failed to create order:', orderError);
        // Don't fail the entire operation if order creation fails
        alert('Order placed, but failed to send for approval. Please notify staff.');
      }
      
      // Clear cart after successful order
      clearCart();
      setShowCart(false);
      
      // Show success feedback
      console.log('Order placed and sent to kitchen automatically');
      
      // Reload bill data
      await loadTableData();
    } catch (error) {
      console.error('Error creating bill:', error);
    } finally {
      setOrderLoading(false);
    }
  }, [tableCode, tableData, cart, clearCart, loadTableData]);

  const handleAddItemsToBill = useCallback(async () => {
    if (!currentBill || cart.length === 0 || !tableData) return;
    
    setOrderLoading(true);
    try {
      // Create a new order for additional items
      const orderItems = cart.map(item => {
        let itemName = item.name;
        let totalPrice = item.price;
        let specialRequests = item.specialRequests || '';
        
        // Include add-ons in the item name and price
        if (item.addOns && item.addOns.length > 0) {
          const addOnNames = item.addOns.map(addon => addon.name).join(', ');
          itemName = `${item.name} (${addOnNames})`;
          totalPrice = item.price + item.addOns.reduce((sum, addon) => sum + addon.price, 0);
          
          // Add add-ons to special requests if not already there
          const addOnDetails = item.addOns.map(addon => 
            `${addon.name}${addon.price > 0 ? ` (+$${addon.price.toFixed(2)})` : ''}`
          ).join(', ');
          specialRequests = specialRequests 
            ? `${specialRequests}. Add-ons: ${addOnDetails}`
            : `Add-ons: ${addOnDetails}`;
        }
        
        return {
          menu_item_name: itemName,
          quantity: item.quantity,
          price: totalPrice,
          special_requests: specialRequests,
        };
      });

      console.log('Creating order with data:', {
        business_id: tableData.business.id,
        bill_id: currentBill.bill.id,
        items: orderItems
      });

      try {
        const orderResult = await createGuestOrder(tableCode, {
          bill_id: currentBill.bill.id,
          items: orderItems,
          notes: `Table ${tableData.table.name} - Additional Items`,
        });
        
        console.log('Additional order created successfully and pending approval:', orderResult);
      } catch (orderError) {
        console.error('Failed to create additional order:', orderError);
        // Don't fail the entire operation if order creation fails
        alert('Items added to bill, but failed to send for approval. Please notify staff.');
      }
      
      // Clear cart after adding to bill
      clearCart();
      setShowCart(false);
      
      // Show success feedback
      console.log('Additional items sent to kitchen automatically');
      
      // Reload bill data
      await loadTableData();
    } catch (error) {
      console.error('Error adding items to bill:', error);
    } finally {
      setOrderLoading(false);
    }
  }, [currentBill, cart, clearCart, loadTableData, tableData]);

  const handleAddToBill = useCallback(async (itemName: string, price: number, quantity: number = 1) => {
    if (!currentBill) {
      console.log('No current bill available');
      return;
    }
    
    try {
      console.log('Adding item to bill:', { itemName, price, quantity });
      
      const orderItems = [{
        menu_item_name: itemName,
        quantity,
        price: price,
        special_requests: ''
      }];
      
      await createGuestOrder(tableCode, {
        bill_id: currentBill.bill.id,
        items: orderItems,
        notes: `Table ${tableData?.table.name || 'Unknown'} - Single Item Order`
      });
      console.log('Order created successfully');
      
      // Reload bill data
      const billResponse = await getOpenBillByTableCode(tableCode);
      setCurrentBill(billResponse);
    } catch (error) {
      console.error('Error adding item to bill:', error);
    }
  }, [currentBill?.bill.id, tableCode]); // eslint-disable-line react-hooks/exhaustive-deps

  useEffect(() => {
    loadTableData();
  }, [tableCode]); // eslint-disable-line react-hooks/exhaustive-deps

  if (loading) {
    return (
      <div className="min-h-screen flex items-center justify-center">
        <Spinner size="lg" />
      </div>
    );
  }

  if (!tableData) {
    return (
      <div className="min-h-screen flex items-center justify-center">
        <Card className="max-w-md">
          <CardBody className="text-center py-8">
            <h2 className="text-xl font-semibold mb-2">{t('errors.tableNotFound')}</h2>
            <p className="text-default-500">
              {t('errors.tableNotFoundDescription')}
            </p>
          </CardBody>
        </Card>
      </div>
    );
  }

  const { business, categories } = tableData;

  // Use translated categories if available, otherwise fall back to original
  const displayCategories = translatedCategories.length > 0 ? translatedCategories : categories;
  
  // Safety check for categories data (similar to BillCreator fix)
  const safeCategories = Array.isArray(displayCategories) ? displayCategories : [];

  console.log('Menu page - categories data:', { 
    originalCategories: categories,
    translatedCategories,
    displayCategories,
    safeCategories, 
    selectedLanguage,
    isArray: Array.isArray(displayCategories),
    translatedCategoriesLength: translatedCategories.length,
    safeCategoriesLength: safeCategories.length,
    firstOriginalCategory: categories?.[0]?.name,
    firstTranslatedCategory: translatedCategories?.[0]?.name,
    firstDisplayCategory: displayCategories?.[0]?.name,
    // Debug menu items
    firstOriginalItem: categories?.[0]?.items?.[0]?.name,
    firstTranslatedItem: translatedCategories?.[0]?.items?.[0]?.name,
    firstDisplayItem: displayCategories?.[0]?.items?.[0]?.name
  });

  return (
        <div className="min-h-screen bg-white relative overflow-hidden">
      {/* Subtle animated background elements */}
      <div className="absolute inset-0 overflow-hidden pointer-events-none">
        <div className="absolute -top-40 -right-40 w-80 h-80 bg-gradient-to-br from-blue-50 to-purple-50 rounded-full blur-3xl opacity-30 animate-pulse"></div>
        <div className="absolute -bottom-40 -left-40 w-96 h-96 bg-gradient-to-tr from-gray-50 to-blue-50 rounded-full blur-3xl opacity-20 animate-pulse" style={{ animationDelay: '2s' }}></div>
      </div>

      {/* Clean Header */}
      <div className="relative z-10 bg-white/80 backdrop-blur-xl border-b border-gray-100">
        <div className="max-w-4xl mx-auto px-6 py-6">
          <div className="flex items-center gap-6">
            <Link href={`/t/${tableCode}`}>
              <Button
                isIconOnly
                variant="light"
                className="text-gray-600 hover:text-gray-900 hover:bg-gray-100 border border-gray-200 rounded-xl"
                size="lg"
              >
                <ArrowLeft className="w-6 h-6" />
              </Button>
            </Link>
            
            <div className="flex items-center gap-4 flex-1">
              {business.logo && (
                <div className="w-12 h-12 rounded-xl overflow-hidden border border-gray-100 shadow-sm">
                  <Image
                    src={business.logo}
                    alt={business.name}
                    className="w-full h-full object-cover"
                  />
                </div>
              )}
              <div>
                <h1 className="text-2xl font-light text-gray-900 tracking-wide">
                  {business.name}
                </h1>
                <p className="text-gray-500 font-light">{t('menu.title')}</p>
              </div>
            </div>


            {/* Shopping Cart Button */}
            <Button
              isIconOnly
              variant="light"
              className="text-gray-600 hover:text-gray-900 hover:bg-gray-100 border border-gray-200 rounded-xl relative"
              size="lg"
              onPress={() => setShowCart(true)}
            >
              <ShoppingCart className="w-6 h-6" />
              {cartItemCount > 0 && (
                <Badge
                  content={cartItemCount}
                  color="primary"
                  className="absolute -top-1 -right-1"
                >
                  <span></span>
                </Badge>
              )}
            </Button>
          </div>
        </div>
      </div>


      {/* Menu Content */}
      <div className="relative z-10 max-w-4xl mx-auto px-6 py-8 pb-28">
        <GuestMenu
          key={`menu-${selectedLanguage}-${safeCategories.length}-${safeCategories[0]?.items?.[0]?.name || 'empty'}`}
          categories={safeCategories}
          business={business}
          tableCode={tableCode}
          currentBill={currentBill}
          selectedLanguage={selectedLanguage}
          defaultCurrency={businessCurrencies.default_currency}
          displayCurrency={businessCurrencies.display_currency}
          onAddToBill={handleAddToBill}
          onAddToCart={addToCart}
        />
      </div>

      {/* Persistent Navigation */}
      <PersistentGuestNav 
        tableCode={tableCode}
        currentBill={currentBill}
      />

      {/* Shopping Cart Modal */}
      <Modal 
        isOpen={showCart} 
        onClose={() => setShowCart(false)}
        size="2xl"
        scrollBehavior="inside"
      >
        <ModalContent>
          <ModalHeader className="flex flex-col gap-1">
            <div className="flex items-center gap-2">
              <ShoppingCart className="w-5 h-5" />
<span>{t('menu.yourOrder')}</span>
            </div>
            <p className="text-sm text-default-500 font-normal">
{t('menu.itemsInCartCount', { count: cart.length })}
            </p>
          </ModalHeader>
          <ModalBody>
            {cart.length === 0 ? (
              <div className="text-center py-8">
                <ShoppingCart className="w-12 h-12 mx-auto text-default-300 mb-4" />
                <h3 className="text-lg font-medium text-default-500 mb-2">{t('menu.cartEmptyM')}</h3>
                <p className="text-default-400">{t('menu.cartEmptyDescriptionM')}</p>
              </div>
            ) : (
              <div className="space-y-4">
                {cart.map((item, index) => (
                  <div key={index} className="flex justify-between items-center p-4 bg-default-50 rounded-lg">
                    <div className="flex-1">
                      <h4 className="font-medium">{item.name}</h4>
                      <p className="text-sm text-default-600">
                        ${item.price.toFixed(2)} base
                        {item.addOns && item.addOns.length > 0 && (
                          <span> + ${item.addOns.reduce((sum, addon) => sum + addon.price, 0).toFixed(2)} add-ons</span>
                        )}
                        <span className="font-medium"> = ${(item.price + (item.addOns?.reduce((sum, addon) => sum + addon.price, 0) || 0)).toFixed(2)} each</span>
                      </p>
                      {item.addOns && item.addOns.length > 0 && (
                        <div className="text-sm text-default-500 mt-1">
                          <span className="font-medium">Add-ons: </span>
                          {item.addOns.map((addon, addonIndex) => (
                            <span key={addonIndex}>
                              {addon.name} (+${addon.price.toFixed(2)})
                              {addonIndex < item.addOns!.length - 1 ? ', ' : ''}
                            </span>
                          ))}
                        </div>
                      )}
                      {item.specialRequests && (
                        <p className="text-sm text-default-500 mt-1">Note: {item.specialRequests}</p>
                      )}
                    </div>
                    <div className="flex items-center gap-3">
                      <div className="flex items-center gap-2">
                        <Button
                          isIconOnly
                          size="sm"
                          variant="light"
                          onPress={() => updateCartItemQuantity(index, item.quantity - 1)}
                        >
                          <Minus className="w-3 h-3" />
                        </Button>
                        <span className="w-8 text-center font-medium">{item.quantity}</span>
                        <Button
                          isIconOnly
                          size="sm"
                          variant="light"
                          onPress={() => updateCartItemQuantity(index, item.quantity + 1)}
                        >
                          <Plus className="w-3 h-3" />
                        </Button>
                      </div>
                      <div className="text-right min-w-[60px]">
                        <p className="font-semibold">${(item.price * item.quantity).toFixed(2)}</p>
                      </div>
                    </div>
                  </div>
                ))}
                
                <Divider />
                
                <div className="flex justify-between items-center text-lg font-semibold">
                  <span>{t('menu.cartTotalM')}</span>
                  <span>${cartTotal.toFixed(2)}</span>
                </div>
              </div>
            )}
          </ModalBody>
          <ModalFooter>
            <Button 
              variant="light" 
              onPress={() => setShowCart(false)}
            >
              {t('menu.continueShopping')}
            </Button>
            {cart.length > 0 && (
              <>
                {!currentBill && (
                  <Button 
                    color="danger" 
                    variant="light"
                    onPress={clearCart}
                  >
                    {t('menu.clearCart')}
                  </Button>
                )}
                {!currentBill ? (
                  <Button 
                    color="primary"
                    startContent={<Plus className="w-4 h-4" />}
                    onPress={handleCreateBill}
                    isLoading={orderLoading}
                  >
                    {orderLoading ? t('menu.orderProcessing') : t('menu.placeOrder')}
                  </Button>
                ) : (
                  <Button 
                    color="primary"
                    startContent={<Plus className="w-4 h-4" />}
                    onPress={handleAddItemsToBill}
                    isLoading={orderLoading}
                  >
                    {orderLoading ? t('menu.orderProcessing') : t('menu.placeOrder')}
                  </Button>
                )}
              </>
            )}
          </ModalFooter>
        </ModalContent>
      </Modal>
      {/* Floating Language Selector */}
      {business?.id && (
        <FloatingLanguageSelector
          tableCode={tableCode}
        />  
      )}
      </div>
  );
}

// Main page component that provides the context
export default function GuestMenuPage() {
  return (
    <GuestTranslationProvider businessId={undefined}>
      <GuestMenuPageContent />
    </GuestTranslationProvider>
  );
}
