import React from 'react';
import {
  Modal,
  ModalContent,
  ModalHeader,
  ModalBody,
  ModalFooter,
  Button,
  Card,
  CardHeader,
  CardBody,
  Chip,
  Divider,
} from '@nextui-org/react';
import { DollarSign, ChefHat, Users, Wallet } from 'lucide-react';
import { BillWithItemsResponse } from '../../api/bills';
import AlternativePaymentManager from './AlternativePaymentManager';

interface BillDetailsModalProps {
  isOpen: boolean;
  onClose: () => void;
  bill: BillWithItemsResponse | null;
  onCloseBill: (billId: number) => void;
  onPayWithCrypto: () => void;
  onSplitBill: () => void;
  onPaymentMarked: () => void;
  tString: (key: string) => string;
}

export const BillDetailsModal: React.FC<BillDetailsModalProps> = ({
  isOpen,
  onClose,
  bill,
  onCloseBill,
  onPayWithCrypto,
  onSplitBill,
  onPaymentMarked,
  tString,
}) => {
  if (!bill) return null;

  const formatCurrency = (amount: number) => {
    return `$${amount.toFixed(2)}`;
  };

  const formatDate = (dateString: string) => {
    return new Date(dateString).toLocaleDateString();
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

  return (
    <Modal
      isOpen={isOpen}
      onClose={onClose}
      size="2xl"
      scrollBehavior="inside"
      classNames={{
        base: "max-h-[90vh]",
        body: "py-6",
      }}
    >
      <ModalContent>
        <ModalHeader className="flex items-center gap-3 pb-4 border-b border-gray-200">
          <div className="w-10 h-10 bg-blue-100 rounded-xl flex items-center justify-center">
            <DollarSign className="w-5 h-5 text-blue-600" />
          </div>
          <div>
            <h2 className="text-lg font-semibold text-gray-900">
              {tString('billDetails')}
            </h2>
            <p className="text-sm text-gray-500">
              Bill #{bill.bill.bill_number}
            </p>
          </div>
        </ModalHeader>

        <ModalBody className="space-y-6">
          {/* Bill Information */}
          <Card className="shadow-sm">
            <CardHeader className="pb-3">
              <h3 className="text-base font-semibold text-gray-900">
                {tString('billInformation')}
              </h3>
            </CardHeader>
            <CardBody className="pt-0">
              <div className="grid grid-cols-2 gap-4">
                <div>
                  <p className="text-sm font-medium text-gray-500 mb-1">
                    {tString('tableColumns.billNumber')}
                  </p>
                  <p className="font-mono text-sm font-medium text-gray-900">
                    {bill.bill.bill_number}
                  </p>
                </div>
                <div>
                  <p className="text-sm font-medium text-gray-500 mb-1">
                    {tString('table')}
                  </p>
                  <p className="text-sm text-gray-900">
                    {tString('table')} {bill.bill.table_id}
                  </p>
                </div>
                <div>
                  <p className="text-sm font-medium text-gray-500 mb-1">
                    {tString('tableColumns.status')}
                  </p>
                  <Chip
                    color={getStatusColor(bill.bill.status)}
                    variant="flat"
                    size="sm"
                  >
                    {bill.bill.status.toUpperCase()}
                  </Chip>
                </div>
                <div>
                  <p className="text-sm font-medium text-gray-500 mb-1">
                    {tString('tableColumns.created')}
                  </p>
                  <p className="text-sm text-gray-900">
                    {formatDate(bill.bill.created_at)}
                  </p>
                </div>
              </div>

              {bill.bill.notes && (
                <div className="mt-4 bg-blue-50 border border-blue-200 rounded-lg p-3">
                  <p className="text-sm font-medium text-blue-700 mb-2">
                    📝 {tString('orderNotes')}:
                  </p>
                  <p className="text-blue-600 text-sm whitespace-pre-wrap">
                    {bill.bill.notes}
                  </p>
                </div>
              )}
            </CardBody>
          </Card>

          {/* Items */}
          <Card className="shadow-sm">
            <CardHeader className="pb-3">
              <h3 className="text-base font-semibold text-gray-900">
                {tString('tableColumns.items')} ({bill.items?.length || 0})
              </h3>
            </CardHeader>
            <CardBody className="pt-0">
              {!bill.items || bill.items.length === 0 ? (
                <div className="text-center py-8">
                  <ChefHat className="w-12 h-12 mx-auto text-gray-300 mb-4" />
                  <h4 className="text-base font-medium text-gray-500 mb-2">
                    {tString('noItems')}
                  </h4>
                  <p className="text-gray-400 text-sm">
                    {tString('noItemsDescription')}
                  </p>
                </div>
              ) : (
                <div className="space-y-3">
                  {bill.items.map((item, index) => (
                    <div key={index} className="flex justify-between items-center p-3 bg-gray-50 rounded-lg">
                      <div>
                        <p className="font-medium text-gray-900">
                          {item.name || tString('unknownItem')}
                        </p>
                        <p className="text-sm text-gray-500">
                          {item.quantity || 1} × {formatCurrency(item.price || 0)}
                        </p>
                      </div>
                      <p className="font-semibold text-gray-900">
                        {formatCurrency(item.subtotal || 0)}
                      </p>
                    </div>
                  ))}
                </div>
              )}
            </CardBody>
          </Card>

          {/* Bill Summary */}
          <Card className="shadow-sm">
            <CardHeader className="pb-3">
              <h3 className="text-base font-semibold text-gray-900">
                {tString('billSummary')}
              </h3>
            </CardHeader>
            <CardBody className="pt-0">
              <div className="space-y-3">
                <div className="flex justify-between text-sm">
                  <span className="text-gray-600">{tString('subtotal')}:</span>
                  <span className="font-medium">{formatCurrency(bill.bill.subtotal)}</span>
                </div>
                <div className="flex justify-between text-sm">
                  <span className="text-gray-600">{tString('tax')}:</span>
                  <span className="font-medium">{formatCurrency(bill.bill.tax_amount)}</span>
                </div>
                <div className="flex justify-between text-sm">
                  <span className="text-gray-600">{tString('serviceFee')}:</span>
                  <span className="font-medium">{formatCurrency(bill.bill.service_fee_amount)}</span>
                </div>
                <Divider />
                <div className="flex justify-between text-lg font-semibold">
                  <span>{tString('total')}:</span>
                  <span>{formatCurrency(bill.bill.total_amount)}</span>
                </div>
                
                {bill.bill.paid_amount > 0 && (
                  <>
                    <div className="flex justify-between text-sm text-green-600">
                      <span>{tString('paid')}:</span>
                      <span className="font-medium">{formatCurrency(bill.bill.paid_amount)}</span>
                    </div>
                    {bill.bill.tip_amount > 0 && (
                      <div className="flex justify-between text-sm text-green-600">
                        <span>{tString('tips')}:</span>
                        <span className="font-medium">{formatCurrency(bill.bill.tip_amount)}</span>
                      </div>
                    )}
                    {(() => {
                      const remaining = bill.bill.total_amount - bill.bill.paid_amount;
                      if (remaining > 0) {
                        return (
                          <div className="flex justify-between text-sm text-orange-600">
                            <span>{tString('remaining')}:</span>
                            <span className="font-medium">{formatCurrency(remaining)}</span>
                          </div>
                        );
                      } else if (remaining < 0) {
                        return (
                          <div className="flex justify-between text-sm text-green-600">
                            <span>{tString('overpaid')}:</span>
                            <span className="font-medium">{formatCurrency(Math.abs(remaining))}</span>
                          </div>
                        );
                      }
                      return null;
                    })()}
                  </>
                )}
              </div>
            </CardBody>
          </Card>

          {/* Payment Options - Only show for unpaid bills */}
          {bill.bill.status === 'open' && (
            <Card className="shadow-sm">
              <CardHeader className="pb-3">
                <h3 className="text-base font-semibold text-gray-900">
                  {tString('paymentOptions')}
                </h3>
                <p className="text-sm text-gray-500">
                  {tString('paymentOptionsDescription')}
                </p>
              </CardHeader>
              <CardBody className="pt-0 space-y-4">
                {/* Crypto Payment */}
                <Button
                  color="primary"
                  size="lg"
                  onPress={onPayWithCrypto}
                  className="w-full"
                  startContent={<Wallet className="w-5 h-5" />}
                >
                  {tString('payWithCrypto')}
                </Button>

                {/* Cash/Card Payment */}
                <div className="bg-gray-50 rounded-lg p-4">
                  <h4 className="font-medium text-gray-900 mb-3">
                    {tString('cashCardPayments')}
                  </h4>
                  <AlternativePaymentManager
                    billId={bill.bill.id.toString()}
                    billTotal={bill.bill.total_amount}
                    onPaymentMarked={onPaymentMarked}
                  />
                </div>

                {/* Split Bill */}
                <Button
                  color="warning"
                  variant="bordered"
                  size="lg"
                  onPress={onSplitBill}
                  className="w-full"
                  startContent={<Users className="w-5 h-5" />}
                >
                  {tString('splitBill')}
                </Button>

                {/* Payment Summary */}
                <div className="bg-blue-50 rounded-lg p-4">
                  <div className="flex justify-between items-center">
                    <span className="text-blue-700 font-medium text-sm">
                      {tString('remainingAmount')}:
                    </span>
                    <span className="font-semibold text-blue-900">
                      {formatCurrency(bill.bill.total_amount - bill.bill.paid_amount)}
                    </span>
                  </div>
                </div>
              </CardBody>
            </Card>
          )}
        </ModalBody>

        <ModalFooter className="border-t border-gray-200 pt-4">
          <Button variant="light" onPress={onClose}>
            Close
          </Button>
          {bill.bill.status === 'open' && (
            <Button
              color="warning"
              onPress={() => {
                onCloseBill(bill.bill.id);
                onClose();
              }}
            >
              Close Bill
            </Button>
          )}
        </ModalFooter>
      </ModalContent>
    </Modal>
  );
};
