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
} from '@nextui-org/react';
import { Eye, DollarSign, Clock, Users, Plus } from 'lucide-react';
import { getBusinessBills, getBill, closeBill, Bill, BillItem, BillResponse } from '../../api/bills';
import { BillCreator } from './BillCreator';

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
          {bills.length === 0 ? (
            <div className="text-center py-8">
              <DollarSign className="w-12 h-12 mx-auto text-default-300 mb-4" />
              <h3 className="text-lg font-medium text-default-500 mb-2">No Active Bills</h3>
              <p className="text-default-400 mb-4">Create your first bill to get started</p>
              <Button
                color="primary"
                onPress={() => setShowBillCreator(true)}
              >
                Create Bill
              </Button>
            </div>
          ) : (
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
                {bills.map((bill) => (
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
                          <Button
                            size="sm"
                            color="warning"
                            variant="light"
                            isLoading={actionLoading === bill.id}
                            onPress={() => handleCloseBill(bill.id)}
                          >
                            Close
                          </Button>
                        )}
                      </div>
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          )}
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
