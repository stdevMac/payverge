import React from 'react';
import { Button } from '@nextui-org/react';
import { Clock, CheckCircle, XCircle } from 'lucide-react';
import { Order, parseOrderItems } from '../../api/orders';

interface PendingOrdersSectionProps {
  orders: Record<number, Order[]>;
  actionLoading: number | null;
  onApproveOrder: (orderId: number) => void;
  onRejectOrder: (orderId: number) => void;
  tString: (key: string) => string;
}

export const PendingOrdersSection: React.FC<PendingOrdersSectionProps> = ({
  orders,
  actionLoading,
  onApproveOrder,
  onRejectOrder,
  tString,
}) => {
  // Get all pending orders
  const pendingOrders = Object.values(orders)
    .flat()
    .filter(order => order.status === 'pending');

  if (pendingOrders.length === 0) {
    return null;
  }

  // Remove duplicates and add billId
  const allPendingOrders = Object.entries(orders).flatMap(([billId, billOrders]) => 
    billOrders.filter(order => order.status === 'pending').map(order => ({ ...order, billId }))
  );
  
  const uniqueOrders = allPendingOrders.filter((order, index, array) => 
    array.findIndex(o => o.id === order.id) === index
  );

  return (
    <div className="bg-orange-50 border border-orange-200 rounded-lg p-6 mb-6">
      <div className="flex items-center gap-3 mb-6">
        <div className="w-10 h-10 bg-orange-100 rounded-xl flex items-center justify-center">
          <Clock className="w-5 h-5 text-orange-600" />
        </div>
        <div>
          <h3 className="text-lg font-medium text-orange-900">
            {tString('pendingOrdersTitle')}
          </h3>
          <p className="text-sm text-orange-700">
            {uniqueOrders.length} {uniqueOrders.length === 1 ? 'order' : 'orders'} awaiting approval
          </p>
        </div>
      </div>

      <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
        {uniqueOrders.map(order => (
          <div key={order.id} className="bg-white border border-orange-200 rounded-lg p-4 shadow-sm">
            <div className="flex justify-between items-start mb-3">
              <div>
                <h4 className="font-medium text-gray-900">
                  Order #{order.order_number}
                </h4>
                <p className="text-xs text-gray-500">
                  {tString('billNumber')} #{order.billId}
                </p>
              </div>
              <div className="px-2 py-1 bg-orange-100 text-orange-700 rounded-md text-xs font-medium">
                {tString('pending')}
              </div>
            </div>

            <div className="space-y-2 mb-4">
              {parseOrderItems(order.items).slice(0, 3).map((item, index) => (
                <div key={index} className="text-sm text-gray-700">
                  <span className="font-medium">{item.quantity}×</span> {item.menu_item_name}
                </div>
              ))}
              {parseOrderItems(order.items).length > 3 && (
                <div className="text-xs text-gray-500">
                  +{parseOrderItems(order.items).length - 3} more items
                </div>
              )}
            </div>

            {order.notes && (
              <div className="bg-blue-50 border border-blue-200 rounded-lg p-3 mb-4">
                <p className="text-xs font-medium text-blue-700 mb-1">
                  📝 Notes:
                </p>
                <p className="text-sm text-blue-600">{order.notes}</p>
              </div>
            )}

            <div className="flex gap-2">
              <Button
                size="sm"
                color="success"
                startContent={<CheckCircle className="w-4 h-4" />}
                onPress={() => onApproveOrder(order.id)}
                isLoading={actionLoading === order.id}
                className="flex-1"
              >
                {tString('approve')}
              </Button>
              <Button
                size="sm"
                color="danger"
                variant="light"
                startContent={<XCircle className="w-4 h-4" />}
                onPress={() => onRejectOrder(order.id)}
                isLoading={actionLoading === order.id}
              >
                {tString('reject')}
              </Button>
            </div>
          </div>
        ))}
      </div>
    </div>
  );
};
