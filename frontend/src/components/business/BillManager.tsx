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
import { Eye, DollarSign, Clock, Users, Plus, ChefHat } from 'lucide-react';
import { getBusinessBills, getBill, closeBill, Bill, BillItem, BillResponse } from '../../api/bills';
import { BillCreator } from './BillCreator';
import { createKitchenOrder } from '../../api/kitchen';

interface BillManagerProps {
  businessId: number;
}

export const BillManager: React.FC<BillManagerProps> = ({ businessId }) => {
  const [bills, setBills] = useState<Bill[]>([]);
  const [loading, setLoading] = useState(true);
  const [selectedBill, setSelectedBill] = useState<BillResponse | null>(null);
  const [showBillDetails, setShowBillDetails] = useState(false);
  const [showBillCreator, setShowBillCreator] = useState(false);
  const [actionLoading, setActionLoading] = useState<number | null>(null);
  const [kitchenLoading, setKitchenLoading] = useState<number | null>(null);
  
  // Tab management
  const [activeTab, setActiveTab] = useState<'active' | 'history'>('active');
  
  // Active Bills Filters
  const [activeSearchQuery, setActiveSearchQuery] = useState('');
  const [activeDateFrom, setActiveDateFrom] = useState<string>('');
  const [activeDateTo, setActiveDateTo] = useState<string>('');
  
  // History Filters
  const [historySearchQuery, setHistorySearchQuery] = useState('');
  const [historyStatusFilter, setHistoryStatusFilter] = useState<'all' | 'paid' | 'closed'>('all');
  const [historyDateFrom, setHistoryDateFrom] = useState<string>('');
  const [historyDateTo, setHistoryDateTo] = useState<string>('');

  const loadBills = useCallback(async () => {
    setLoading(true);
    try {
      const response = await getBusinessBills(businessId);
      setBills(response.bills || []);
    } catch (error) {
      console.error('Error loading bills:', error);
    } finally {
      setLoading(false);
    }
  }, [businessId]);

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

  const handleSendToKitchen = async (billId: number) => {
    setKitchenLoading(billId);
    try {
      // First get the full bill details with items
      const billDetails = await getBill(billId);
      
      // Convert bill items to kitchen order format
      const kitchenOrderItems = billDetails.items.map((item: BillItem) => ({
        menu_item_name: item.name,
        quantity: item.quantity,
        price: item.price,
        special_requests: item.options?.join(', ') || '',
      }));

      await createKitchenOrder(businessId, {
        bill_id: billDetails.bill.id,
        table_id: billDetails.bill.table_id,
        order_type: 'table',
        priority: 'normal',
        notes: `Bill #${billDetails.bill.bill_number}`,
        items: kitchenOrderItems,
      });

      // Show success message or update UI
      console.log('Order sent to kitchen successfully');
    } catch (error) {
      console.error('Error sending order to kitchen:', error);
    } finally {
      setKitchenLoading(null);
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
                        color="primary"
                        variant="light"
                        startContent={<ChefHat className="w-3 h-3" />}
                        isLoading={kitchenLoading === bill.id}
                        onPress={() => handleSendToKitchen(bill.id)}
                      >
                        Kitchen
                      </Button>
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
    </>
  );
};
