import React, { useState, useEffect, useCallback } from 'react';
import {
  Card,
  CardBody,
  CardHeader,
  Button,
  Table,
  TableHeader,
  TableColumn,
  TableBody,
  TableRow,
  TableCell,
  Chip,
  Modal,
  ModalContent,
  ModalHeader,
  ModalBody,
  ModalFooter,
  Divider,
  Spinner,
  Input,
  Select,
  SelectItem,
  Tabs,
  Tab,
} from '@nextui-org/react';
import { Eye, DollarSign, Clock, Users, Plus, ChefHat, CheckCircle, XCircle, Wallet, CreditCard } from 'lucide-react';
import { getBill, getBusinessBills, BillWithItemsResponse, closeBill, Bill } from '../../api/bills';
import { BillCreator } from './BillCreator';
import { getOrdersByBillId, updateOrderStatus, Order, getOrderStatusColor, getOrderStatusText, parseOrderItems } from '../../api/orders';
import AlternativePaymentManager from './AlternativePaymentManager';
import PaymentProcessor from '../payment/PaymentProcessor';
import BillSplittingFlow, { BillData } from '../splitting/BillSplittingFlow';

interface BillManagerProps {
  businessId: number;
}

export const BillManager: React.FC<BillManagerProps> = ({ businessId }) => {
  const [bills, setBills] = useState<Bill[]>([]);
  const [loading, setLoading] = useState(true);
  const [selectedBill, setSelectedBill] = useState<BillWithItemsResponse | null>(null);
  const [showBillDetails, setShowBillDetails] = useState(false);
  const [showBillCreator, setShowBillCreator] = useState(false);
  const [showPaymentProcessor, setShowPaymentProcessor] = useState(false);
  const [showBillSplitting, setShowBillSplitting] = useState(false);
  const [orders, setOrders] = useState<Record<number, Order[]>>({});
  const [actionLoading, setActionLoading] = useState<number | null>(null);
  const [activeTab, setActiveTab] = useState<string>('active');
  
  // Active Bills Filters
  const [activeSearchQuery, setActiveSearchQuery] = useState('');
  const [activeDateFrom, setActiveDateFrom] = useState<string>('');
  const [activeDateTo, setActiveDateTo] = useState<string>('');
  
  const [historySearchQuery, setHistorySearchQuery] = useState('');
  const [historyStatusFilter, setHistoryStatusFilter] = useState<'all' | 'paid' | 'closed'>('all');
  const [historyDateFrom, setHistoryDateFrom] = useState<string>('');
  const [historyDateTo, setHistoryDateTo] = useState<string>('');

  const loadOrdersForBills = useCallback(async (billsList: Bill[]) => {
    const ordersMap: Record<number, Order[]> = {};
    
    // Load orders for each bill
    for (const bill of billsList) {
      try {
        const orderData = await getOrdersByBillId(businessId, bill.id);
        ordersMap[bill.id] = orderData.orders || [];
      } catch (error) {
        // No orders for this bill, that's okay
        ordersMap[bill.id] = [];
      }
    }
    
    console.log('Orders loaded:', ordersMap);
    setOrders(ordersMap);
  }, [businessId]);

  const loadBills = useCallback(async () => {
    setLoading(true);
    try {
      const data = await getBusinessBills(businessId);
      const billsList = data.bills || [];
      setBills(billsList);
      
      // Load orders for all bills
      await loadOrdersForBills(billsList);
    } catch (error) {
      console.error('Error loading bills:', error);
    } finally {
      setLoading(false);
    }
  }, [businessId, loadOrdersForBills]);

  useEffect(() => {
    if (businessId) {
      loadBills();
    }
  }, [businessId, loadBills]);

  // Separate filtered lists for each tab
  const activeBills = React.useMemo(() => {
    let list = bills.filter((b) => b.status === 'open');

    // Search filter (bill number, table id)
    const q = activeSearchQuery.trim().toLowerCase();
    if (q) {
      list = list.filter((b) => {
        const billNo = b.bill_number?.toLowerCase() || '';
        const tableStr = `table ${b.table_id}`.toLowerCase();
        const totalStr = String(b.total_amount);
        return billNo.includes(q) || tableStr.includes(q) || totalStr.includes(q);
      });
    }

    // Date range filter (created_at)
    const from = activeDateFrom ? new Date(activeDateFrom) : null;
    const to = activeDateTo ? new Date(activeDateTo) : null;
    if (from || to) {
      list = list.filter((b) => {
        const created = new Date(b.created_at);
        const afterFrom = from ? created >= new Date(from.setHours(0, 0, 0, 0)) : true;
        const beforeTo = to ? created <= new Date(to.setHours(23, 59, 59, 999)) : true;
        return afterFrom && beforeTo;
      });
    }

    // Sort newest first
    list.sort((a, b) => new Date(b.created_at).getTime() - new Date(a.created_at).getTime());
    return list;
  }, [bills, activeSearchQuery, activeDateFrom, activeDateTo]);

  const historyBills = React.useMemo(() => {
    let list = bills.filter((b) => b.status === 'paid' || b.status === 'closed');

    // Status filter for history
    if (historyStatusFilter !== 'all') {
      list = list.filter((b) => b.status === historyStatusFilter);
    }

    // Search filter (bill number, table id)
    const q = historySearchQuery.trim().toLowerCase();
    if (q) {
      list = list.filter((b) => {
        const billNo = b.bill_number?.toLowerCase() || '';
        const tableStr = `table ${b.table_id}`.toLowerCase();
        const totalStr = String(b.total_amount);
        return billNo.includes(q) || tableStr.includes(q) || totalStr.includes(q);
      });
    }

    // Date range filter (created_at)
    const from = historyDateFrom ? new Date(historyDateFrom) : null;
    const to = historyDateTo ? new Date(historyDateTo) : null;
    if (from || to) {
      list = list.filter((b) => {
        const created = new Date(b.created_at);
        const afterFrom = from ? created >= new Date(from.setHours(0, 0, 0, 0)) : true;
        const beforeTo = to ? created <= new Date(to.setHours(23, 59, 59, 999)) : true;
        return afterFrom && beforeTo;
      });
    }

    // Sort newest first
    list.sort((a, b) => new Date(b.created_at).getTime() - new Date(a.created_at).getTime());
    return list;
  }, [bills, historyStatusFilter, historySearchQuery, historyDateFrom, historyDateTo]);

  const resetActiveFilters = () => {
    setActiveSearchQuery('');
    setActiveDateFrom('');
    setActiveDateTo('');
  };

  const resetHistoryFilters = () => {
    setHistorySearchQuery('');
    setHistoryStatusFilter('all');
    setHistoryDateFrom('');
    setHistoryDateTo('');
  };

  const handleViewBill = async (billId: number) => {
    try {
      const billDetails = await getBill(billId);
      setSelectedBill(billDetails);
      setShowBillDetails(true);
    } catch (error) {
      console.error('Error loading bill details:', error);
    }
  };

  const handleCloseBill = async (billId: number) => {
    setActionLoading(billId);
    try {
      await closeBill(billId);
      await loadBills(); // Refresh the list
    } catch (error) {
      console.error('Error closing bill:', error);
    } finally {
      setActionLoading(null);
    }
  };


  const getStatusColor = (status: string) => {
    switch (status) {
      case 'open':
        return 'success';
      case 'paid':
        return 'primary';
      case 'closed':
        return 'default';
      default:
        return 'default';
    }
  };

  const formatCurrency = (amount: number) => {
    return `$${amount.toFixed(2)}`;
  };

  const formatDate = (dateString: string) => {
    return new Date(dateString).toLocaleString();
  };

  const getOrderStatus = (billId: number) => {
    const billOrders = orders[billId] || [];
    if (billOrders.length === 0) return { status: 'none', color: 'default', text: 'No Orders' };
    
    // Check if any orders are pending approval
    const pending = billOrders.some((order: Order) => order.status === 'pending');
    if (pending) return { status: 'pending', color: 'warning', text: 'Needs Approval' };
    
    // Check if any orders are in kitchen
    const inKitchen = billOrders.some((order: Order) => order.status === 'in_kitchen');
    if (inKitchen) return { status: 'cooking', color: 'secondary', text: 'In Kitchen' };
    
    // Check if any orders are ready
    const ready = billOrders.some((order: Order) => order.status === 'ready');
    if (ready) return { status: 'ready', color: 'success', text: 'Ready' };
    
    // Check if any orders are approved but not in kitchen
    const approved = billOrders.some((order: Order) => order.status === 'approved');
    if (approved) return { status: 'approved', color: 'primary', text: 'Approved' };
    
    // All orders are delivered or cancelled
    const delivered = billOrders.every((order: Order) => order.status === 'delivered' || order.status === 'cancelled');
    if (delivered) return { status: 'completed', color: 'default', text: 'Completed' };
    
    return { status: 'unknown', color: 'default', text: 'Unknown' };
  };

  // Handle order approval
  const handleApproveOrder = async (orderId: number) => {
    setActionLoading(orderId);
    try {
      await updateOrderStatus(businessId, orderId, {
        status: 'approved',
        approved_by: 'staff' // TODO: Get actual staff member from auth context
      });
      await loadBills(); // Refresh the data
    } catch (error) {
      console.error('Error approving order:', error);
    } finally {
      setActionLoading(null);
    }
  };

  // Handle order rejection
  const handleRejectOrder = async (orderId: number) => {
    setActionLoading(orderId);
    try {
      await updateOrderStatus(businessId, orderId, {
        status: 'cancelled',
        approved_by: 'staff' // TODO: Get actual staff member from auth context
      });
      await loadBills(); // Refresh the data
    } catch (error) {
      console.error('Error rejecting order:', error);
    } finally {
      setActionLoading(null);
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

  const renderBillsTable = (billsList: Bill[], isActive: boolean) => {
    if (billsList.length === 0) {
      return (
        <div className="text-center py-8">
          <DollarSign className="w-12 h-12 mx-auto text-default-300 mb-4" />
          <h3 className="text-lg font-medium text-default-500 mb-2">
            {isActive ? 'No Active Bills' : 'No bills match your filters'}
          </h3>
          <p className="text-default-400 mb-4">
            {isActive 
              ? 'Create your first bill to get started' 
              : 'Try adjusting the status, dates, or search query'
            }
          </p>
          {isActive ? (
            <Button color="primary" onPress={() => setShowBillCreator(true)}>
              Create Bill
            </Button>
          ) : (
            <Button variant="light" onPress={isActive ? resetActiveFilters : resetHistoryFilters}>
              Reset Filters
            </Button>
          )}
        </div>
      );
    }

    return (
      <Table aria-label="Bills table">
        <TableHeader>
          <TableColumn>BILL #</TableColumn>
          <TableColumn>TABLE</TableColumn>
          <TableColumn>ITEMS</TableColumn>
          <TableColumn>TOTAL</TableColumn>
          <TableColumn>STATUS</TableColumn>
          <TableColumn>KITCHEN</TableColumn>
          <TableColumn>CREATED</TableColumn>
          <TableColumn>ACTIONS</TableColumn>
        </TableHeader>
        <TableBody>
          {billsList.map((bill) => (
            <TableRow key={bill.id}>
              <TableCell>
                <span className="font-mono text-sm">{bill.bill_number}</span>
              </TableCell>
              <TableCell>
                <div className="flex items-center gap-2">
                  <Users className="w-4 h-4 text-default-400" />
                  <span>Table {bill.table_id}</span>
                </div>
              </TableCell>
              <TableCell>
                <span className="text-sm">
                  {JSON.parse(bill.items || '[]').length} items
                </span>
              </TableCell>
              <TableCell>
                <span className="font-semibold">{formatCurrency(bill.total_amount)}</span>
              </TableCell>
              <TableCell>
                <Chip
                  color={getStatusColor(bill.status)}
                  variant="flat"
                  size="sm"
                >
                  {bill.status.toUpperCase()}
                </Chip>
              </TableCell>
              <TableCell>
                {(() => {
                  const orderStatus = getOrderStatus(bill.id);
                  return (
                    <div className="flex items-center gap-1">
                      <ChefHat className="w-3 h-3" />
                      <Chip
                        color={orderStatus.color as any}
                        variant="flat"
                        size="sm"
                      >
                        {orderStatus.text}
                      </Chip>
                    </div>
                  );
                })()}
              </TableCell>
              <TableCell>
                <div className="flex items-center gap-1 text-sm text-default-500">
                  <Clock className="w-3 h-3" />
                  {formatDate(bill.created_at)}
                </div>
              </TableCell>
              <TableCell>
                <div className="flex gap-2">
                  <Button
                    size="sm"
                    variant="light"
                    startContent={<Eye className="w-3 h-3" />}
                    onPress={() => handleViewBill(bill.id)}
                  >
                    View
                  </Button>
                  {bill.status === 'open' && (
                    <>
                      <Button
                        size="sm"
                        color="warning"
                        variant="light"
                        isLoading={actionLoading === bill.id}
                        onPress={() => handleCloseBill(bill.id)}
                      >
                        Close
                      </Button>
                    </>
                  )}
                </div>
              </TableCell>
            </TableRow>
          ))}
        </TableBody>
      </Table>
    );
  };

  return (
    <>
      {/* Pending Orders Section */}
      {(() => {
        const pendingOrders = Object.values(orders).flat().filter(order => order.status === 'pending');
        console.log('Pending orders to display:', pendingOrders);
        return pendingOrders.length > 0;
      })() && (
        <Card className="mb-6">
          <CardHeader>
            <div className="flex items-center gap-2">
              <Clock className="w-5 h-5 text-warning" />
              <h3 className="text-lg font-semibold text-warning">Pending Orders - Require Approval</h3>
            </div>
          </CardHeader>
          <CardBody>
            <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
              {(() => {
                // Deduplicate orders by ID to prevent showing the same order twice
                const allPendingOrders = Object.entries(orders).flatMap(([billId, billOrders]) => 
                  billOrders.filter(order => order.status === 'pending').map(order => ({ ...order, billId }))
                );
                const uniqueOrders = allPendingOrders.filter((order, index, array) => 
                  array.findIndex(o => o.id === order.id) === index
                );
                console.log('Unique pending orders:', uniqueOrders);
                return uniqueOrders.map(order => (
                  <Card key={order.id} className="border-warning">
                    <CardBody>
                      <div className="flex justify-between items-start mb-2">
                        <div>
                          <h4 className="font-semibold">Order #{order.order_number}</h4>
                          <p className="text-sm text-default-500">Bill #{order.billId}</p>
                        </div>
                        <Chip color="warning" size="sm">Pending</Chip>
                      </div>
                      <div className="space-y-1 mb-3">
                        {parseOrderItems(order.items).slice(0, 3).map((item, index) => (
                          <div key={index} className="text-sm">
                            {item.quantity}x {item.menu_item_name}
                          </div>
                        ))}
                        {parseOrderItems(order.items).length > 3 && (
                          <div className="text-xs text-default-400">
                            +{parseOrderItems(order.items).length - 3} more items
                          </div>
                        )}
                      </div>
                      {order.notes && (
                        <p className="text-xs text-blue-600 mb-3 bg-blue-50 p-2 rounded">
                          {order.notes}
                        </p>
                      )}
                      <div className="flex gap-2">
                        <Button
                          size="sm"
                          color="success"
                          startContent={<CheckCircle className="w-3 h-3" />}
                          onPress={() => handleApproveOrder(order.id)}
                          isLoading={actionLoading === order.id}
                        >
                          Approve
                        </Button>
                        <Button
                          size="sm"
                          color="danger"
                          variant="light"
                          startContent={<XCircle className="w-3 h-3" />}
                          onPress={() => handleRejectOrder(order.id)}
                          isLoading={actionLoading === order.id}
                        >
                          Reject
                        </Button>
                      </div>
                    </CardBody>
                  </Card>
                ));
              })()}
            </div>
          </CardBody>
        </Card>
      )}

      <Card>
        <CardHeader className="flex justify-between items-center">
          <div className="flex items-center gap-2">
            <DollarSign className="w-5 h-5" />
            <h2 className="text-xl font-semibold">Bill Management</h2>
          </div>
          <Button
            color="primary"
            startContent={<Plus className="w-4 h-4" />}
            onPress={() => setShowBillCreator(true)}
          >
            Create Bill
          </Button>
        </CardHeader>
        <CardBody>
          <Tabs
            selectedKey={activeTab}
            onSelectionChange={(key) => setActiveTab(key as 'active' | 'history')}
            className="w-full"
          >
            <Tab key="active" title={`Active Bills (${activeBills.length})`}>
              <div className="space-y-4">
                {/* Active Bills Filters */}
                <div className="flex flex-col md:flex-row gap-3 md:items-center">
                  <Input
                    value={activeSearchQuery}
                    onValueChange={setActiveSearchQuery}
                    placeholder="Search active bills..."
                    className="md:w-[280px]"
                    size="sm"
                  />
                  <div className="flex items-center gap-2">
                    <input
                      type="date"
                      value={activeDateFrom}
                      onChange={(e) => setActiveDateFrom(e.target.value)}
                      className="border rounded-medium px-3 py-1.5 text-sm text-foreground bg-transparent"
                      aria-label="From date"
                    />
                    <span className="text-default-400 text-sm">to</span>
                    <input
                      type="date"
                      value={activeDateTo}
                      onChange={(e) => setActiveDateTo(e.target.value)}
                      className="border rounded-medium px-3 py-1.5 text-sm text-foreground bg-transparent"
                      aria-label="To date"
                    />
                  </div>
                  <Button variant="light" size="sm" onPress={resetActiveFilters}>
                    Reset
                  </Button>
                </div>
                {renderBillsTable(activeBills, true)}
              </div>
            </Tab>
            <Tab key="history" title={`Bill History (${historyBills.length})`}>
              <div className="space-y-4">
                {/* History Filters */}
                <div className="flex flex-col md:flex-row gap-3 md:items-center">
                  <Input
                    value={historySearchQuery}
                    onValueChange={setHistorySearchQuery}
                    placeholder="Search bill history..."
                    className="md:w-[280px]"
                    size="sm"
                  />
                  <Select
                    selectedKeys={[historyStatusFilter]}
                    onChange={(e) => setHistoryStatusFilter((e.target.value as any) || 'all')}
                    size="sm"
                    className="w-full md:w-[160px]"
                    aria-label="Filter by status"
                  >
                    <SelectItem key="all">All statuses</SelectItem>
                    <SelectItem key="paid">Paid</SelectItem>
                    <SelectItem key="closed">Closed</SelectItem>
                  </Select>
                  <div className="flex items-center gap-2">
                    <input
                      type="date"
                      value={historyDateFrom}
                      onChange={(e) => setHistoryDateFrom(e.target.value)}
                      className="border rounded-medium px-3 py-1.5 text-sm text-foreground bg-transparent"
                      aria-label="From date"
                    />
                    <span className="text-default-400 text-sm">to</span>
                    <input
                      type="date"
                      value={historyDateTo}
                      onChange={(e) => setHistoryDateTo(e.target.value)}
                      className="border rounded-medium px-3 py-1.5 text-sm text-foreground bg-transparent"
                      aria-label="To date"
                    />
                  </div>
                  <Button variant="light" size="sm" onPress={resetHistoryFilters}>
                    Reset
                  </Button>
                </div>
                {renderBillsTable(historyBills, false)}
              </div>
            </Tab>
          </Tabs>
        </CardBody>
      </Card>

      {/* Bill Details Modal */}
      <Modal
        isOpen={showBillDetails}
        onClose={() => setShowBillDetails(false)}
        size="2xl"
        scrollBehavior="inside"
      >
        <ModalContent>
          <ModalHeader>
            <div className="flex items-center gap-2">
              <DollarSign className="w-5 h-5" />
              Bill Details - {selectedBill?.bill.bill_number}
            </div>
          </ModalHeader>
          <ModalBody>
            {selectedBill && (
              <div className="space-y-4">
                {/* Bill Info */}
                <Card>
                  <CardHeader>
                    <h3 className="text-lg font-semibold">Bill Information</h3>
                  </CardHeader>
                  <CardBody>
                    <div className="grid grid-cols-2 gap-4">
                      <div>
                        <p className="text-sm text-default-500">Bill Number</p>
                        <p className="font-mono">{selectedBill.bill.bill_number}</p>
                      </div>
                      <div>
                        <p className="text-sm text-default-500">Table</p>
                        <p>Table {selectedBill.bill.table_id}</p>
                      </div>
                      <div>
                        <p className="text-sm text-default-500">Status</p>
                        <Chip
                          color={getStatusColor(selectedBill.bill.status)}
                          variant="flat"
                          size="sm"
                        >
                          {selectedBill.bill.status.toUpperCase()}
                        </Chip>
                      </div>
                      <div>
                        <p className="text-sm text-default-500">Created</p>
                        <p>{formatDate(selectedBill.bill.created_at)}</p>
                      </div>
                    </div>
                    
                    {selectedBill.bill.notes && (
                      <div className="mt-4 bg-blue-50 border border-blue-200 rounded-lg p-3">
                        <p className="text-sm font-medium text-blue-700 mb-2">📝 Order Notes:</p>
                        <p className="text-blue-600 whitespace-pre-wrap">{selectedBill.bill.notes}</p>
                      </div>
                    )}
                  </CardBody>
                </Card>

                {/* Items */}
                <Card>
                  <CardHeader>
                    <h3 className="text-lg font-semibold">Items ({selectedBill.items.length})</h3>
                  </CardHeader>
                  <CardBody>
                    <div className="space-y-3">
                      {selectedBill.items.map((item, index) => (
                        <div key={index} className="flex justify-between items-center p-3 bg-default-50 rounded-lg">
                          <div>
                            <p className="font-medium">{item.name}</p>
                            <p className="text-sm text-default-500">
                              {item.quantity} × {formatCurrency(item.price)}
                            </p>
                          </div>
                          <p className="font-semibold">{formatCurrency(item.subtotal)}</p>
                        </div>
                      ))}
                    </div>
                  </CardBody>
                </Card>

                {/* Totals */}
                <Card>
                  <CardHeader>
                    <h3 className="text-lg font-semibold">Bill Summary</h3>
                  </CardHeader>
                  <CardBody>
                    <div className="space-y-2">
                      <div className="flex justify-between">
                        <span>Subtotal:</span>
                        <span>{formatCurrency(selectedBill.bill.subtotal)}</span>
                      </div>
                      <div className="flex justify-between">
                        <span>Tax:</span>
                        <span>{formatCurrency(selectedBill.bill.tax_amount)}</span>
                      </div>
                      <div className="flex justify-between">
                        <span>Service Fee:</span>
                        <span>{formatCurrency(selectedBill.bill.service_fee_amount)}</span>
                      </div>
                      <Divider />
                      <div className="flex justify-between font-semibold text-lg">
                        <span>Total:</span>
                        <span>{formatCurrency(selectedBill.bill.total_amount)}</span>
                      </div>
                      {selectedBill.bill.paid_amount > 0 && (
                        <>
                          <div className="flex justify-between text-success">
                            <span>Paid:</span>
                            <span>{formatCurrency(selectedBill.bill.paid_amount)}</span>
                          </div>
                          <div className="flex justify-between">
                            <span>Remaining:</span>
                            <span>{formatCurrency(selectedBill.bill.total_amount - selectedBill.bill.paid_amount)}</span>
                          </div>
                        </>
                      )}
                    </div>
                  </CardBody>
                </Card>

                {/* Payment Options - Only show for unpaid bills */}
                {selectedBill.bill.status === 'open' && (
                  <Card>
                    <CardHeader>
                      <h3 className="text-lg font-semibold">Payment Options</h3>
                      <p className="text-sm text-default-500">
                        Process payments for customers at counter or table
                      </p>
                    </CardHeader>
                    <CardBody>
                      <div className="space-y-3">
                        {/* Pay with Crypto */}
                        <Button
                          color="primary"
                          size="lg"
                          onPress={() => setShowPaymentProcessor(true)}
                          className="w-full"
                          startContent={<Wallet className="w-5 h-5" />}
                        >
                          Pay with Crypto (USDC)
                        </Button>

                        {/* Pay with Cash/Card */}
                        <div className="bg-gray-50 rounded-lg p-4">
                          <h4 className="font-medium mb-3">Cash/Card Payments</h4>
                          <AlternativePaymentManager
                            billId={selectedBill.bill.id.toString()}
                            billTotal={selectedBill.bill.total_amount}
                            onPaymentMarked={() => {
                              // Refresh bill data after payment
                              loadBills();
                              if (selectedBill) {
                                handleViewBill(selectedBill.bill.id);
                              }
                            }}
                          />
                        </div>

                        {/* Split Bill */}
                        <Button
                          color="warning"
                          variant="bordered"
                          size="lg"
                          onPress={() => setShowBillSplitting(true)}
                          className="w-full"
                          startContent={<Users className="w-5 h-5" />}
                        >
                          Split Bill
                        </Button>
                      </div>

                      {/* Payment Summary */}
                      <div className="mt-4 p-3 bg-blue-50 rounded-lg">
                        <div className="flex justify-between items-center text-sm">
                          <span className="text-blue-700 font-medium">Remaining Amount:</span>
                          <span className="font-semibold text-blue-900">
                            ${(selectedBill.bill.total_amount - selectedBill.bill.paid_amount).toFixed(2)}
                          </span>
                        </div>
                      </div>
                    </CardBody>
                  </Card>
                )}
              </div>
            )}
          </ModalBody>
          <ModalFooter>
            <Button variant="light" onPress={() => setShowBillDetails(false)}>
              Close
            </Button>
            {selectedBill?.bill.status === 'open' && (
              <Button
                color="warning"
                onPress={() => {
                  handleCloseBill(selectedBill.bill.id);
                  setShowBillDetails(false);
                }}
              >
                Close Bill
              </Button>
            )}
          </ModalFooter>
        </ModalContent>
      </Modal>

      {/* Bill Creator Modal */}
      <BillCreator
        isOpen={showBillCreator}
        onClose={() => setShowBillCreator(false)}
        businessId={businessId}
        onBillCreated={loadBills}
      />

      {/* Payment Processor Modal */}
      {selectedBill && (
        <PaymentProcessor
          isOpen={showPaymentProcessor}
          onClose={() => setShowPaymentProcessor(false)}
          billId={selectedBill.bill.id}
          amount={selectedBill.bill.total_amount - selectedBill.bill.paid_amount}
          businessName="Business"
          businessAddress={selectedBill.bill.settlement_address}
          tipAddress={selectedBill.bill.tipping_address}
          onPaymentComplete={() => {
            setShowPaymentProcessor(false);
            loadBills();
            if (selectedBill) {
              handleViewBill(selectedBill.bill.id);
            }
          }}
        />
      )}

      {/* Bill Splitting Modal */}
      {selectedBill && (
        <BillSplittingFlow
          bill={{
            id: selectedBill.bill.id,
            billNumber: selectedBill.bill.bill_number,
            items: selectedBill.items,
            subtotal: selectedBill.bill.subtotal,
            taxAmount: selectedBill.bill.tax_amount,
            serviceFeeAmount: selectedBill.bill.service_fee_amount,
            totalAmount: selectedBill.bill.total_amount,
          }}
          businessName="Business"
          businessAddress="Business Address"
          tableNumber={selectedBill.bill.table_id?.toString() || 'Counter'}
          isOpen={showBillSplitting}
          onClose={() => setShowBillSplitting(false)}
          onPaymentInitiate={() => {
            setShowBillSplitting(false);
            loadBills();
            if (selectedBill) {
              handleViewBill(selectedBill.bill.id);
            }
          }}
        />
      )}
    </>
  );
};
