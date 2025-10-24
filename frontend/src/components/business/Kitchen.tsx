'use client';

import React, { useState, useEffect, useCallback } from 'react';
import {
  Card,
  CardHeader,
  CardBody,
  Button,
  Chip,
  Modal,
  ModalContent,
  ModalHeader,
  ModalBody,
  ModalFooter,
  Spinner,
  Tabs,
  Tab,
} from '@nextui-org/react';
import { ChefHat, Clock, Users, CheckCircle, Play } from 'lucide-react';
import { getOrders, updateOrderStatus, Order, getOrderStatusColor, getOrderStatusText, parseOrderItems } from '../../api/orders';
import { useSimpleLocale, getTranslation } from '@/i18n/SimpleTranslationProvider';

interface KitchenProps {
  businessId: number;
}

const Kitchen: React.FC<KitchenProps> = ({ businessId }) => {
  // Translation setup
  const { locale } = useSimpleLocale();
  const [currentLocale, setCurrentLocale] = useState(locale);
  
  // Update translations when locale changes
  useEffect(() => {
    setCurrentLocale(locale);
  }, [locale]);
  
  // Translation helper
  const tString = useCallback((key: string): string => {
    const fullKey = `businessDashboard.dashboard.kitchenManager.${key}`;
    const result = getTranslation(fullKey, currentLocale);
    return Array.isArray(result) ? result[0] || key : result as string;
  }, [currentLocale]);
  
  const [orders, setOrders] = useState<Order[]>([]);
  const [loading, setLoading] = useState(true);
  const [selectedOrder, setSelectedOrder] = useState<Order | null>(null);
  const [showOrderModal, setShowOrderModal] = useState(false);
  const [activeTab, setActiveTab] = useState<string>('approved');
  const [actionLoading, setActionLoading] = useState<number | null>(null);

  const loadOrders = useCallback(async (status?: string) => {
    setLoading(true);
    try {
      const data = await getOrders(businessId, status);
      console.log('Loaded orders from API:', data.orders);
      
      // Filter to only show approved, in_kitchen, ready orders (not pending)
      let kitchenRelevantOrders = data.orders.filter(order => 
        ['approved', 'in_kitchen', 'ready', 'delivered'].includes(order.status)
      );
      
      // If a specific status is requested, filter further
      if (status && status !== 'all') {
        kitchenRelevantOrders = kitchenRelevantOrders.filter(order => order.status === status);
      }
      
      console.log('Filtered kitchen orders:', kitchenRelevantOrders);
      setOrders(kitchenRelevantOrders);
    } catch (error) {
      console.error('Error loading kitchen orders:', error);
    } finally {
      setLoading(false);
    }
  }, [businessId]);

  const updateOrderStatusHandler = async (orderId: number, status: string) => {
    setActionLoading(orderId);
    try {
      console.log(`Updating order ${orderId} to status: ${status} for business ${businessId}`);
      console.log('Request data:', {
        status: status,
        approved_by: 'kitchen'
      });
      
      const result = await updateOrderStatus(businessId, orderId, {
        status: status as any,
        approved_by: 'kitchen'
      });
      console.log('Order status updated successfully:', result);
      
      // Refresh orders with current tab filter to see the updated status
      console.log('Refreshing orders for current tab:', activeTab);
      await loadOrders(activeTab === 'all' ? undefined : activeTab);
      
      // Close modal if order was updated
      setShowOrderModal(false);
      console.log('Modal closed, operation complete');
    } catch (error: any) {
      console.error('Error updating order status:', error);
      console.error('Error details:', {
        message: error.message,
        response: error.response?.data,
        status: error.response?.status
      });
      alert(`Failed to update order status: ${error.response?.data?.error || error.message}`);
    } finally {
      setActionLoading(null);
    }
  };

  useEffect(() => {
    loadOrders(activeTab === 'all' ? undefined : activeTab);
    
    // Set up polling for real-time updates
    const interval = setInterval(() => {
      loadOrders(activeTab === 'all' ? undefined : activeTab);
    }, 30000); // Poll every 30 seconds

    return () => clearInterval(interval);
  }, [loadOrders, activeTab]);

  const formatTime = (dateString: string) => {
    return new Date(dateString).toLocaleTimeString([], { 
      hour: '2-digit', 
      minute: '2-digit' 
    });
  };

  const getElapsedTime = (dateString: string) => {
    const now = new Date();
    const created = new Date(dateString);
    const diffMs = now.getTime() - created.getTime();
    const diffMins = Math.floor(diffMs / 60000);
    
    if (diffMins < 60) {
      return tString('time.minutesAgo').replace('{minutes}', diffMins.toString());
    } else {
      const hours = Math.floor(diffMins / 60);
      const mins = diffMins % 60;
      return tString('time.hoursMinutesAgo').replace('{hours}', hours.toString()).replace('{minutes}', mins.toString());
    }
  };

  if (loading) {
    return (
      <div className="flex items-center justify-center py-16">
        <div className="text-center">
          <div className="w-16 h-16 bg-gray-50 rounded-2xl flex items-center justify-center mx-auto mb-6 border border-gray-100">
            <div className="w-8 h-8 border-2 border-gray-300 border-t-gray-900 rounded-full animate-spin"></div>
          </div>
          <p className="text-gray-600 font-light tracking-wide">{tString('loading')}</p>
        </div>
      </div>
    );
  }

  return (
    <div className="p-4 max-w-6xl mx-auto">
      {/* Compact Header */}
      <div className="flex justify-between items-center mb-6">
        <div>
          <h1 className="text-2xl font-light text-gray-900 tracking-wide">{tString('title')}</h1>
          <p className="text-gray-600 font-light text-sm mt-1">{tString('subtitle')}</p>
        </div>
        <button
          onClick={() => loadOrders(activeTab === 'all' ? undefined : activeTab)}
          className="bg-gray-900 text-white px-4 py-2 rounded-lg font-medium hover:bg-gray-800 transition-colors flex items-center gap-2"
        >
          {tString('buttons.refresh')}
        </button>
      </div>

      {/* Kitchen Orders */}
      <div className="bg-white border border-gray-200 rounded-lg shadow-sm">
        <div className="border-b border-gray-200 p-6">
          <div className="flex items-center gap-3">
            <div className="w-12 h-12 bg-gray-50 rounded-xl flex items-center justify-center border border-gray-100">
              <ChefHat className="w-6 h-6 text-gray-700" />
            </div>
            <div>
              <h2 className="text-xl font-light text-gray-900 tracking-wide">{tString('orders.title')}</h2>
              <p className="text-gray-600 font-light text-sm">{tString('orders.subtitle')}</p>
            </div>
          </div>
        </div>
        <div className="p-6">
          <Tabs
            selectedKey={activeTab}
            onSelectionChange={(key) => setActiveTab(key as string)}
            className="mb-4"
          >
            <Tab key="approved" title={tString('tabs.approved')} />
            <Tab key="in_kitchen" title={tString('tabs.inKitchen')} />
            <Tab key="ready" title={tString('tabs.ready')} />
            <Tab key="all" title={tString('tabs.allOrders')} />
          </Tabs>

          {orders.length === 0 ? (
            <div className="text-center py-12">
              <div className="w-16 h-16 bg-gray-50 rounded-2xl flex items-center justify-center mx-auto mb-6 border border-gray-100">
                <ChefHat className="w-8 h-8 text-gray-400" />
              </div>
              <h3 className="text-lg font-light text-gray-900 tracking-wide mb-2">{tString(`emptyStates.${activeTab}.title`)}</h3>
              <p className="text-gray-600 font-light text-sm">{tString(`emptyStates.${activeTab}.description`)}</p>
            </div>
          ) : (
            <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
              {orders.map((order) => (
                <Card 
                  key={order.id} 
                  className="cursor-pointer hover:shadow-md transition-shadow"
                  isPressable
                  onPress={() => {
                    setSelectedOrder(order);
                    setShowOrderModal(true);
                  }}
                >
                  <CardHeader className="pb-2">
                    <div className="flex justify-between items-start w-full">
                      <div>
                        <h3 className="font-semibold text-lg">#{order.order_number}</h3>
                        <p className="text-sm text-default-500">
                          Bill #{order.bill_id}
                        </p>
                      </div>
                      <div className="flex flex-col items-end gap-1">
                        <Chip size="sm" color={getOrderStatusColor(order.status)}>
                          {getOrderStatusText(order.status)}
                        </Chip>
                      </div>
                    </div>
                  </CardHeader>
                  <CardBody className="pt-0">
                    <div className="space-y-2">
                      <div className="flex items-center gap-2 text-sm">
                        <Clock className="w-4 h-4" />
                        <span>{formatTime(order.created_at)} ({getElapsedTime(order.created_at)})</span>
                      </div>
                      <div className="space-y-1">
                        <div className="flex items-center gap-2 text-sm text-default-500 mb-2">
                          <Users className="w-4 h-4" />
                          <span>{tString('order.itemsToPrepare').replace('{count}', parseOrderItems(order.items).length.toString())}</span>
                        </div>
                        {parseOrderItems(order.items).slice(0, 3).map((item, index) => (
                          <div key={index} className="flex justify-between items-center bg-default-50 rounded-lg p-2">
                            <div className="flex items-center gap-2">
                              <span className="font-medium">{item.quantity}x</span>
                              <span className="text-sm">{item.menu_item_name}</span>
                            </div>
                          </div>
                        ))}
                        {parseOrderItems(order.items).length > 3 && (
                          <div className="text-xs text-default-400 text-center">
                            {tString('order.moreItems').replace('{count}', (parseOrderItems(order.items).length - 3).toString())}
                          </div>
                        )}
                        {order.notes && (
                          <div className="bg-orange-50 border border-orange-200 rounded-lg p-2 mt-2">
                            <p className="text-xs font-medium text-orange-700 mb-1">📝 {tString('order.notes')}</p>
                            <p className="text-xs text-orange-600 line-clamp-2">
                              {order.notes.length > 60 ? `${order.notes.substring(0, 60)}...` : order.notes}
                            </p>
                          </div>
                        )}
                      </div>
                      
                      {/* Quick Action Buttons */}
                      <div className="flex gap-2 mt-3 pt-2 border-t border-default-100">
                        {order.status === 'approved' && (
                          <Button
                            size="sm"
                            color="primary"
                            startContent={<Play className="w-3 h-3" />}
                            onPress={() => updateOrderStatusHandler(order.id, 'in_kitchen')}
                            isLoading={actionLoading === order.id}
                            className="flex-1"
                          >
                            {tString('buttons.startCooking')}
                          </Button>
                        )}
                        {order.status === 'in_kitchen' && (
                          <Button
                            size="sm"
                            color="success"
                            startContent={<CheckCircle className="w-3 h-3" />}
                            onPress={() => updateOrderStatusHandler(order.id, 'ready')}
                            isLoading={actionLoading === order.id}
                            className="flex-1"
                          >
                            {tString('buttons.markReady')}
                          </Button>
                        )}
                        {order.status === 'ready' && (
                          <Button
                            size="sm"
                            color="default"
                            startContent={<CheckCircle className="w-3 h-3" />}
                            onPress={() => updateOrderStatusHandler(order.id, 'delivered')}
                            isLoading={actionLoading === order.id}
                            className="flex-1"
                          >
                            {tString('buttons.markDelivered')}
                          </Button>
                        )}
                      </div>
                    </div>
                  </CardBody>
                </Card>
              ))}
            </div>
          )}
        </div>
      </div>

      {/* Order Detail Modal */}
      <Modal 
        isOpen={showOrderModal} 
        onClose={() => setShowOrderModal(false)}
        size="2xl"
      >
        <ModalContent>
          <ModalHeader>
            <div className="flex items-center gap-2">
              <ChefHat className="w-5 h-5" />
              <span>{tString('modal.orderNumber').replace('{orderNumber}', selectedOrder?.order_number || '')}</span>
              <Chip size="sm" color={getOrderStatusColor(selectedOrder?.status || '')}>
                {getOrderStatusText(selectedOrder?.status || '')}
              </Chip>
            </div>
          </ModalHeader>
          <ModalBody>
            {selectedOrder && (
              <div className="space-y-4">
                <div className="grid grid-cols-2 gap-4">
                  <div>
                    <p className="text-sm text-default-500">{tString('modal.billId')}</p>
                    <p className="font-medium">#{selectedOrder.bill_id}</p>
                  </div>
                  <div>
                    <p className="text-sm text-default-500">{tString('modal.created')}</p>
                    <p className="font-medium">{formatTime(selectedOrder.created_at)}</p>
                  </div>
                </div>

                {selectedOrder.notes && (
                  <div className="bg-blue-50 rounded-lg p-3">
                    <p className="text-sm font-medium text-blue-700 mb-1">{tString('modal.orderNotes')}</p>
                    <p className="text-blue-600">{selectedOrder.notes}</p>
                  </div>
                )}

                <div>
                  <h4 className="font-medium mb-2">{tString('modal.itemsToPrepare')}</h4>
                  <div className="space-y-2">
                    {parseOrderItems(selectedOrder.items).map((item, index) => (
                      <div key={index} className="flex justify-between items-center bg-default-50 rounded-lg p-3">
                        <div>
                          <div className="flex items-center gap-2">
                            <span className="font-medium">{item.quantity}x</span>
                            <span>{item.menu_item_name}</span>
                          </div>
                          {item.options && item.options.length > 0 && (
                            <div className="mt-1">
                              {item.options.map((option, optionIndex) => (
                                <p key={optionIndex} className="text-sm text-blue-600">
                                  + {option.name}
                                  {option.price_change !== 0 && (
                                    <span className="ml-1">
                                      ({option.price_change > 0 ? '+' : ''}${option.price_change.toFixed(2)})
                                    </span>
                                  )}
                                </p>
                              ))}
                            </div>
                          )}
                          {item.special_requests && (
                            <p className="text-sm text-orange-600 mt-1">
                              {tString('modal.special')} {item.special_requests}
                            </p>
                          )}
                        </div>
                        <span className="text-sm font-medium">${item.subtotal.toFixed(2)}</span>
                      </div>
                    ))}
                  </div>
                </div>
              </div>
            )}
          </ModalBody>
          <ModalFooter>
            <Button variant="light" onPress={() => setShowOrderModal(false)}>
              {tString('modal.close')}
            </Button>
            {selectedOrder && (
              <div className="flex gap-2">
                {selectedOrder.status === 'approved' && (
                  <Button
                    color="primary"
                    startContent={<Play className="w-4 h-4" />}
                    onPress={() => {
                      console.log('Start Cooking button clicked for order:', selectedOrder.id);
                      updateOrderStatusHandler(selectedOrder.id, 'in_kitchen');
                    }}
                    isLoading={actionLoading === selectedOrder.id}
                  >
                    {tString('modal.startCooking')}
                  </Button>
                )}
                {selectedOrder.status === 'in_kitchen' && (
                  <Button
                    color="success"
                    startContent={<CheckCircle className="w-4 h-4" />}
                    onPress={() => updateOrderStatusHandler(selectedOrder.id, 'ready')}
                    isLoading={actionLoading === selectedOrder.id}
                  >
                    {tString('modal.markReady')}
                  </Button>
                )}
                {selectedOrder.status === 'ready' && (
                  <Button
                    color="default"
                    startContent={<CheckCircle className="w-4 h-4" />}
                    onPress={() => updateOrderStatusHandler(selectedOrder.id, 'delivered')}
                    isLoading={actionLoading === selectedOrder.id}
                  >
                    {tString('modal.markDelivered')}
                  </Button>
                )}
              </div>
            )}
          </ModalFooter>
        </ModalContent>
      </Modal>
    </div>
  );
};

export default Kitchen;
