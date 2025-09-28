import React, { useState, useEffect, useCallback } from 'react';
import {
  Card,
  CardBody,
  CardHeader,
  Button,
  Chip,
  Modal,
  ModalContent,
  ModalHeader,
  ModalBody,
  ModalFooter,
  Select,
  SelectItem,
  Textarea,
  Input,
  Spinner,
  Tabs,
  Tab,
} from '@nextui-org/react';
import { ChefHat, Clock, Users, AlertCircle, CheckCircle, Play, Pause } from 'lucide-react';
import { 
  getKitchenOrders, 
  updateKitchenOrderStatus, 
  updateKitchenOrderItemStatus,
  KitchenOrder,
  KitchenOrderItem
} from '../../api/kitchen';

interface KitchenProps {
  businessId: number;
}

const Kitchen: React.FC<KitchenProps> = ({ businessId }) => {
  const [orders, setOrders] = useState<KitchenOrder[]>([]);
  const [loading, setLoading] = useState(true);
  const [selectedOrder, setSelectedOrder] = useState<KitchenOrder | null>(null);
  const [showOrderModal, setShowOrderModal] = useState(false);
  const [activeTab, setActiveTab] = useState('incoming');
  const [actionLoading, setActionLoading] = useState<number | null>(null);

  const loadOrders = useCallback(async (status?: string) => {
    setLoading(true);
    try {
      const data = await getKitchenOrders(
        businessId,
        status && status !== 'all' ? status : undefined
      );
      setOrders(data.orders || []);
    } catch (error) {
      console.error('Error loading kitchen orders:', error);
    } finally {
      setLoading(false);
    }
  }, [businessId]);

  const updateOrderStatus = async (orderId: number, status: string, assignedTo?: string) => {
    setActionLoading(orderId);
    try {
      // Update the order status
      await updateKitchenOrderStatus(
        businessId,
        orderId,
        status,
        assignedTo
      );

      // Auto-update all items in the order based on the new order status
      const order = orders.find(o => o.id === orderId);
      if (order) {
        let itemStatus: string;
        switch (status) {
          case 'in_progress':
            itemStatus = 'in_progress';
            break;
          case 'ready':
            itemStatus = 'ready';
            break;
          case 'delivered':
            itemStatus = 'ready'; // Items should be ready when order is delivered
            break;
          case 'cancelled':
            itemStatus = 'cancelled';
            break;
          default:
            itemStatus = 'pending';
        }

        // Update all items to the corresponding status
        for (const item of order.items) {
          if (item.status !== itemStatus) {
            try {
              await updateKitchenOrderItemStatus(
                businessId,
                orderId,
                item.id,
                itemStatus
              );
            } catch (itemError) {
              console.error(`Error updating item ${item.id} status:`, itemError);
            }
          }
        }
      }

      await loadOrders(activeTab === 'all' ? undefined : activeTab);
    } catch (error) {
      console.error('Error updating order status:', error);
    } finally {
      setActionLoading(null);
    }
  };

  const updateItemStatus = async (orderId: number, itemId: number, status: string) => {
    try {
      await updateKitchenOrderItemStatus(
        businessId,
        orderId,
        itemId,
        status
      );
      await loadOrders(activeTab === 'all' ? undefined : activeTab);
      // Update selected order if it's open
      if (selectedOrder && selectedOrder.id === orderId) {
        const updatedOrder = orders.find(o => o.id === orderId);
        if (updatedOrder) {
          setSelectedOrder(updatedOrder);
        }
      }
    } catch (error) {
      console.error('Error updating item status:', error);
    }
  };

  useEffect(() => {
    loadOrders(activeTab === 'all' ? undefined : activeTab);
    
    // Set up polling for real-time updates
    const interval = setInterval(() => {
      loadOrders(activeTab === 'all' ? undefined : activeTab);
    }, 10000); // Poll every 10 seconds

    return () => clearInterval(interval);
  }, [loadOrders, activeTab]);

  const getStatusColor = (status: string) => {
    switch (status) {
      case 'incoming': return 'warning';
      case 'in_progress': return 'primary';
      case 'ready': return 'success';
      case 'delivered': return 'default';
      case 'cancelled': return 'danger';
      default: return 'default';
    }
  };

  const getPriorityColor = (priority: string) => {
    switch (priority) {
      case 'urgent': return 'danger';
      case 'high': return 'warning';
      case 'normal': return 'primary';
      case 'low': return 'default';
      default: return 'default';
    }
  };

  const formatTime = (dateString: string) => {
    return new Date(dateString).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' });
  };

  const getElapsedTime = (createdAt: string) => {
    const now = new Date();
    const created = new Date(createdAt);
    const diffMinutes = Math.floor((now.getTime() - created.getTime()) / (1000 * 60));
    
    if (diffMinutes < 60) {
      return `${diffMinutes}m`;
    } else {
      const hours = Math.floor(diffMinutes / 60);
      const minutes = diffMinutes % 60;
      return `${hours}h ${minutes}m`;
    }
  };

  const getItemStatusColor = (status: string) => {
    switch (status) {
      case 'pending': return 'warning';
      case 'in_progress': return 'primary';
      case 'ready': return 'success';
      case 'cancelled': return 'danger';
      default: return 'default';
    }
  };

  if (loading) {
    return (
      <Card>
        <CardBody className="flex justify-center items-center py-8">
          <Spinner size="lg" />
        </CardBody>
      </Card>
    );
  }

  return (
    <>
      <Card>
        <CardHeader className="flex justify-between items-center">
          <div className="flex items-center gap-2">
            <ChefHat className="w-5 h-5" />
            <h2 className="text-xl font-semibold">Kitchen Orders</h2>
          </div>
          <Button
            color="primary"
            size="sm"
            onPress={() => loadOrders(activeTab === 'all' ? undefined : activeTab)}
          >
            Refresh
          </Button>
        </CardHeader>
        <CardBody>
          <Tabs
            selectedKey={activeTab}
            onSelectionChange={(key) => setActiveTab(key as string)}
            className="mb-4"
          >
            <Tab key="incoming" title="Incoming" />
            <Tab key="in_progress" title="In Progress" />
            <Tab key="ready" title="Ready" />
            <Tab key="all" title="All Orders" />
          </Tabs>

          {orders.length === 0 ? (
            <div className="text-center py-8">
              <ChefHat className="w-12 h-12 mx-auto text-default-300 mb-4" />
              <h3 className="text-lg font-medium text-default-500 mb-2">No Orders</h3>
              <p className="text-default-400">No orders found for the selected status</p>
            </div>
          ) : (
            <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
              {orders.map((order) => (
                <Card 
                  key={order.id} 
                  className={`cursor-pointer hover:shadow-md transition-shadow ${
                    order.priority === 'urgent' ? 'border-2 border-red-500' : 
                    order.priority === 'high' ? 'border-2 border-orange-500' : ''
                  }`}
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
                          {order.table?.name || order.customer_name || 'Counter Order'}
                        </p>
                      </div>
                      <div className="flex flex-col items-end gap-1">
                        <Chip size="sm" color={getStatusColor(order.status)}>
                          {order.status.replace('_', ' ')}
                        </Chip>
                        {order.priority !== 'normal' && (
                          <Chip size="sm" color={getPriorityColor(order.priority)}>
                            {order.priority}
                          </Chip>
                        )}
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
                          <span>{order.items.length} items to prepare:</span>
                        </div>
                        {order.items.map((item) => (
                          <div key={item.id} className="flex justify-between items-center bg-default-50 rounded-lg p-2">
                            <div className="flex-1">
                              <div className="flex items-center gap-2">
                                <span className="font-medium">{item.quantity}x</span>
                                <span className="text-sm">{item.menu_item_name}</span>
                                <div className="flex items-center gap-1">
                                  <Chip 
                                    size="sm" 
                                    color={getItemStatusColor(item.status)}
                                    variant="flat"
                                  >
                                    {item.status.replace('_', ' ')}
                                  </Chip>
                                  {item.status === 'pending' && (
                                    <Button
                                      size="sm"
                                      color="primary"
                                      variant="flat"
                                      className="min-w-unit-16 h-6 text-xs"
                                      onPress={() => updateItemStatus(order.id, item.id, 'in_progress')}
                                    >
                                      Start
                                    </Button>
                                  )}
                                  {item.status === 'in_progress' && (
                                    <Button
                                      size="sm"
                                      color="success"
                                      variant="flat"
                                      className="min-w-unit-16 h-6 text-xs"
                                      onPress={() => updateItemStatus(order.id, item.id, 'ready')}
                                    >
                                      Ready
                                    </Button>
                                  )}
                                </div>
                              </div>
                              {item.special_requests && (
                                <p className="text-xs text-default-500 mt-1 ml-8">
                                  Note: {item.special_requests}
                                </p>
                              )}
                            </div>
                          </div>
                        ))}
                      </div>
                      {order.notes && (
                        <div className="flex items-start gap-2 text-sm">
                          <AlertCircle className="w-4 h-4 mt-0.5" />
                          <span className="text-default-600">{order.notes}</span>
                        </div>
                      )}
                    </div>
                    <div className="flex gap-2 mt-4">
                      {order.status === 'incoming' && (
                        <Button
                          size="sm"
                          color="primary"
                          startContent={<Play className="w-3 h-3" />}
                          isLoading={actionLoading === order.id}
                          onPress={() => updateOrderStatus(order.id, 'in_progress')}
                        >
                          Start
                        </Button>
                      )}
                      {order.status === 'in_progress' && (
                        <Button
                          size="sm"
                          color="success"
                          startContent={<CheckCircle className="w-3 h-3" />}
                          isLoading={actionLoading === order.id}
                          onPress={() => updateOrderStatus(order.id, 'ready')}
                        >
                          Ready
                        </Button>
                      )}
                      {order.status === 'ready' && (
                        <Button
                          size="sm"
                          color="default"
                          isLoading={actionLoading === order.id}
                          onPress={() => updateOrderStatus(order.id, 'delivered')}
                        >
                          Delivered
                        </Button>
                      )}
                    </div>
                  </CardBody>
                </Card>
              ))}
            </div>
          )}
        </CardBody>
      </Card>

      {/* Enhanced Order Details Modal */}
      <Modal 
        isOpen={showOrderModal} 
        onClose={() => setShowOrderModal(false)}
        size="3xl"
        classNames={{
          base: "bg-gradient-to-br from-white to-default-50",
          header: "border-b border-divider bg-gradient-to-r from-primary-50 to-secondary-50",
          body: "py-6",
          footer: "border-t border-divider bg-default-25"
        }}
      >
        <ModalContent>
          {selectedOrder && (
            <>
              <ModalHeader className="flex flex-col gap-1 px-6 py-4">
                <div className="flex items-center justify-between w-full">
                  <div className="flex items-center gap-3">
                    <div className="p-2 bg-primary-100 rounded-lg">
                      <ChefHat className="w-6 h-6 text-primary-600" />
                    </div>
                    <div>
                      <h2 className="text-xl font-bold text-foreground">
                        Order #{selectedOrder.order_number}
                      </h2>
                      <p className="text-sm text-default-500">
                        {selectedOrder.table?.name || selectedOrder.customer_name || 'Counter Order'}
                      </p>
                    </div>
                  </div>
                  <div className="flex items-center gap-2">
                    <Chip 
                      size="lg" 
                      color={getStatusColor(selectedOrder.status)}
                      variant="flat"
                      className="font-semibold"
                    >
                      {selectedOrder.status.replace('_', ' ').toUpperCase()}
                    </Chip>
                    {selectedOrder.priority !== 'normal' && (
                      <Chip 
                        size="lg" 
                        color={getPriorityColor(selectedOrder.priority)}
                        variant="solid"
                        className="font-semibold"
                      >
                        {selectedOrder.priority.toUpperCase()}
                      </Chip>
                    )}
                  </div>
                </div>
              </ModalHeader>

              <ModalBody className="px-6">
                {/* Order Info Cards */}
                <div className="grid grid-cols-1 md:grid-cols-3 gap-4 mb-6">
                  <Card className="bg-gradient-to-br from-blue-50 to-blue-100 border-blue-200">
                    <CardBody className="p-4">
                      <div className="flex items-center gap-3">
                        <Clock className="w-5 h-5 text-blue-600" />
                        <div>
                          <p className="text-xs text-blue-600 font-medium">ORDER TIME</p>
                          <p className="font-bold text-blue-800">{formatTime(selectedOrder.created_at)}</p>
                          <p className="text-xs text-blue-600">({getElapsedTime(selectedOrder.created_at)} ago)</p>
                        </div>
                      </div>
                    </CardBody>
                  </Card>

                  <Card className="bg-gradient-to-br from-green-50 to-green-100 border-green-200">
                    <CardBody className="p-4">
                      <div className="flex items-center gap-3">
                        <Users className="w-5 h-5 text-green-600" />
                        <div>
                          <p className="text-xs text-green-600 font-medium">ITEMS</p>
                          <p className="font-bold text-green-800">{selectedOrder.items.length} dishes</p>
                          <p className="text-xs text-green-600">to prepare</p>
                        </div>
                      </div>
                    </CardBody>
                  </Card>

                  <Card className="bg-gradient-to-br from-purple-50 to-purple-100 border-purple-200">
                    <CardBody className="p-4">
                      <div className="flex items-center gap-3">
                        <AlertCircle className="w-5 h-5 text-purple-600" />
                        <div>
                          <p className="text-xs text-purple-600 font-medium">ASSIGNED TO</p>
                          <p className="font-bold text-purple-800">
                            {selectedOrder.assigned_to || 'Unassigned'}
                          </p>
                          <p className="text-xs text-purple-600">kitchen staff</p>
                        </div>
                      </div>
                    </CardBody>
                  </Card>
                </div>

                {/* Special Notes */}
                {selectedOrder.notes && (
                  <Card className="mb-6 bg-gradient-to-r from-amber-50 to-orange-50 border-amber-200">
                    <CardBody className="p-4">
                      <div className="flex items-start gap-3">
                        <AlertCircle className="w-5 h-5 text-amber-600 mt-0.5" />
                        <div>
                          <p className="text-sm font-semibold text-amber-800 mb-1">Special Instructions</p>
                          <p className="text-sm text-amber-700">{selectedOrder.notes}</p>
                        </div>
                      </div>
                    </CardBody>
                  </Card>
                )}

                {/* Items to Cook */}
                <div>
                  <h3 className="text-lg font-bold text-foreground mb-4 flex items-center gap-2">
                    <ChefHat className="w-5 h-5" />
                    Items to Prepare
                  </h3>
                  <div className="space-y-3">
                    {selectedOrder.items.map((item, index) => (
                      <Card 
                        key={item.id} 
                        className={`transition-all duration-200 hover:shadow-md ${
                          item.status === 'ready' 
                            ? 'bg-gradient-to-r from-green-50 to-emerald-50 border-green-200' 
                            : item.status === 'in_progress'
                            ? 'bg-gradient-to-r from-blue-50 to-cyan-50 border-blue-200'
                            : 'bg-gradient-to-r from-gray-50 to-slate-50 border-gray-200'
                        }`}
                      >
                        <CardBody className="p-4">
                          <div className="flex items-center justify-between">
                            <div className="flex items-center gap-4">
                              <div className="flex items-center justify-center w-8 h-8 bg-primary-100 rounded-full">
                                <span className="text-sm font-bold text-primary-600">
                                  {index + 1}
                                </span>
                              </div>
                              <div>
                                <div className="flex items-center gap-3 mb-1">
                                  <span className="text-lg font-bold text-foreground">
                                    {item.quantity}x
                                  </span>
                                  <span className="text-base font-semibold text-foreground">
                                    {item.menu_item_name}
                                  </span>
                                </div>
                                {item.special_requests && (
                                  <div className="flex items-center gap-2 mt-2">
                                    <AlertCircle className="w-4 h-4 text-amber-500" />
                                    <span className="text-sm text-amber-700 font-medium">
                                      {item.special_requests}
                                    </span>
                                  </div>
                                )}
                              </div>
                            </div>
                            <div className="flex items-center gap-3">
                              <Chip 
                                size="md" 
                                color={getItemStatusColor(item.status)}
                                variant="flat"
                                className="font-semibold"
                              >
                                {item.status.replace('_', ' ').toUpperCase()}
                              </Chip>
                              <div className="flex gap-2">
                                {item.status === 'pending' && (
                                  <Button
                                    color="primary"
                                    size="sm"
                                    startContent={<Play className="w-4 h-4" />}
                                    onPress={() => updateItemStatus(selectedOrder.id, item.id, 'in_progress')}
                                    className="font-semibold"
                                  >
                                    Start Cooking
                                  </Button>
                                )}
                                {item.status === 'in_progress' && (
                                  <Button
                                    color="success"
                                    size="sm"
                                    startContent={<CheckCircle className="w-4 h-4" />}
                                    onPress={() => updateItemStatus(selectedOrder.id, item.id, 'ready')}
                                    className="font-semibold"
                                  >
                                    Mark Ready
                                  </Button>
                                )}
                                {item.status === 'ready' && (
                                  <Chip color="success" variant="solid" className="font-semibold">
                                    ✓ READY
                                  </Chip>
                                )}
                              </div>
                            </div>
                          </div>
                        </CardBody>
                      </Card>
                    ))}
                  </div>
                </div>
              </ModalBody>

              <ModalFooter className="px-6 py-4">
                <div className="flex justify-between items-center w-full">
                  <div className="flex gap-2">
                    {selectedOrder.status === 'incoming' && (
                      <Button
                        color="primary"
                        size="lg"
                        startContent={<Play className="w-4 h-4" />}
                        onPress={() => updateOrderStatus(selectedOrder.id, 'in_progress')}
                        className="font-semibold"
                      >
                        Start All Items
                      </Button>
                    )}
                    {selectedOrder.status === 'in_progress' && (
                      <Button
                        color="success"
                        size="lg"
                        startContent={<CheckCircle className="w-4 h-4" />}
                        onPress={() => updateOrderStatus(selectedOrder.id, 'ready')}
                        className="font-semibold"
                      >
                        Mark All Ready
                      </Button>
                    )}
                    {selectedOrder.status === 'ready' && (
                      <Button
                        color="default"
                        size="lg"
                        onPress={() => updateOrderStatus(selectedOrder.id, 'delivered')}
                        className="font-semibold"
                      >
                        Mark Delivered
                      </Button>
                    )}
                  </div>
                  <Button 
                    color="danger" 
                    variant="light" 
                    size="lg"
                    onPress={() => setShowOrderModal(false)}
                    className="font-semibold"
                  >
                    Close
                  </Button>
                </div>
              </ModalFooter>
            </>
          )}
        </ModalContent>
      </Modal>
    </>
  );
};

export default Kitchen;
