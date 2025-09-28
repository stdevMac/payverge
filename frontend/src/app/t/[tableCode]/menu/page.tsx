'use client';

import React, { useState, useEffect, useCallback } from 'react';
import { Card, CardBody, Button, Spinner, Image, Badge, Modal, ModalContent, ModalHeader, ModalBody, ModalFooter, Divider } from '@nextui-org/react';
import { ArrowLeft, MapPin, ShoppingCart, Plus, Minus, ChefHat } from 'lucide-react';
import { useParams } from 'next/navigation';
import Link from 'next/link';
import GuestMenu from '../../../../components/guest/GuestMenu';
import { PersistentGuestNav } from '../../../../components/navigation/PersistentGuestNav';
import { BillResponse, getTableByCode, getOpenBillByTableCode, addBillItem } from '../../../../api/bills';
import { Business, MenuCategory } from '../../../../api/business';
import { createKitchenOrder } from '../../../../api/kitchen';

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
}

export default function GuestMenuPage() {
  const params = useParams();
  const tableCode = params.tableCode as string;
  
  const [tableData, setTableData] = useState<TableData | null>(null);
  const [currentBill, setCurrentBill] = useState<BillResponse | null>(null);
  const [loading, setLoading] = useState(true);
  const [cart, setCart] = useState<CartItem[]>([]);
  const [showCart, setShowCart] = useState(false);
  const [orderLoading, setOrderLoading] = useState(false);

  const loadTableData = useCallback(async () => {
    setLoading(true);
    try {
      const tableResponse = await getTableByCode(tableCode);
      setTableData(tableResponse);

      try {
        const billResponse = await getOpenBillByTableCode(tableCode);
        setCurrentBill(billResponse);
      } catch {
        setCurrentBill(null);
      }
    } catch (error) {
      console.error('Error loading table data:', error);
    } finally {
      setLoading(false);
    }
  }, [tableCode]);

  const addToCart = useCallback((itemName: string, price: number, specialRequests?: string) => {
    setCart(prevCart => {
      const existingItem = prevCart.find(item => 
        item.name === itemName && item.specialRequests === specialRequests
      );
      
      if (existingItem) {
        return prevCart.map(item =>
          item.name === itemName && item.specialRequests === specialRequests
            ? { ...item, quantity: item.quantity + 1 }
            : item
        );
      } else {
        return [...prevCart, { name: itemName, price, quantity: 1, specialRequests }];
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

  const getCartTotal = useCallback(() => {
    return cart.reduce((total, item) => total + (item.price * item.quantity), 0);
  }, [cart]);

  const getCartItemCount = useCallback(() => {
    return cart.reduce((total, item) => total + item.quantity, 0);
  }, [cart]);

  const handleSendToKitchen = useCallback(async () => {
    if (!tableData || cart.length === 0) return;
    
    setOrderLoading(true);
    try {
      const kitchenOrderItems = cart.map(item => ({
        menu_item_name: item.name,
        quantity: item.quantity,
        price: item.price,
        special_requests: item.specialRequests || '',
      }));

      await createKitchenOrder(tableData.business.id, {
        table_id: tableData.table.id,
        order_type: 'table',
        priority: 'normal',
        notes: `Table ${tableData.table.name} - Guest Order`,
        items: kitchenOrderItems,
      });

      // Clear cart after successful order
      clearCart();
      setShowCart(false);
      
      // Show success message
      console.log('Order sent to kitchen successfully');
    } catch (error) {
      console.error('Error sending order to kitchen:', error);
    } finally {
      setOrderLoading(false);
    }
  }, [tableData, cart, clearCart]);

  const handleAddToBill = useCallback(async (itemName: string, price: number, quantity: number = 1) => {
    if (!currentBill) {
      console.log('No current bill available');
      return;
    }
    
    try {
      console.log('Adding item to bill:', { itemName, price, quantity });
      
      const addItemRequest = {
        menu_item_id: itemName,
        name: itemName,
        price: price,
        quantity,
        options: []
      };
      
      await addBillItem(currentBill.bill.id, addItemRequest);
      console.log('Item added successfully');
      
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
            <h2 className="text-xl font-semibold mb-2">Table Not Found</h2>
            <p className="text-default-500">
              The table code &quot;{tableCode}&quot; could not be found.
            </p>
          </CardBody>
        </Card>
      </div>
    );
  }

  const { business, categories } = tableData;

  // Safety check for categories data (similar to BillCreator fix)
  const safeCategories = Array.isArray(categories) ? categories : [];

  console.log('Menu page - categories data:', { 
    categories, 
    safeCategories, 
    isArray: Array.isArray(categories) 
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
                <p className="text-gray-500 font-light">Menu</p>
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
              {getCartItemCount() > 0 && (
                <Badge
                  content={getCartItemCount()}
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
          categories={safeCategories}
          business={business}
          tableCode={tableCode}
          currentBill={currentBill}
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
              <span>Your Order</span>
            </div>
            <p className="text-sm text-default-500 font-normal">
              {cart.length} {cart.length === 1 ? 'item' : 'items'} in cart
            </p>
          </ModalHeader>
          <ModalBody>
            {cart.length === 0 ? (
              <div className="text-center py-8">
                <ShoppingCart className="w-12 h-12 mx-auto text-default-300 mb-4" />
                <h3 className="text-lg font-medium text-default-500 mb-2">Your cart is empty</h3>
                <p className="text-default-400">Add items from the menu to get started</p>
              </div>
            ) : (
              <div className="space-y-4">
                {cart.map((item, index) => (
                  <div key={index} className="flex justify-between items-center p-4 bg-default-50 rounded-lg">
                    <div className="flex-1">
                      <h4 className="font-medium">{item.name}</h4>
                      <p className="text-sm text-default-600">${item.price.toFixed(2)} each</p>
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
                  <span>Total:</span>
                  <span>${getCartTotal().toFixed(2)}</span>
                </div>
              </div>
            )}
          </ModalBody>
          <ModalFooter>
            <Button 
              variant="light" 
              onPress={() => setShowCart(false)}
            >
              Continue Shopping
            </Button>
            {cart.length > 0 && (
              <>
                <Button 
                  color="danger" 
                  variant="light"
                  onPress={clearCart}
                >
                  Clear Cart
                </Button>
                <Button 
                  color="primary"
                  startContent={<ChefHat className="w-4 h-4" />}
                  onPress={handleSendToKitchen}
                  isLoading={orderLoading}
                >
                  Send to Kitchen
                </Button>
              </>
            )}
          </ModalFooter>
        </ModalContent>
      </Modal>
    </div>
  );
}
