'use client';

import React, { useState } from 'react';
import {
  Card,
  CardBody,
  CardHeader,
  Chip,
  Divider,
  Button,
} from '@nextui-org/react';
import { Receipt, Clock, CreditCard, Wallet } from 'lucide-react';
import { BillResponse } from '../../api/bills';
import { Business } from '../../api/business';
import PaymentProcessor from '../payment/PaymentProcessor';

interface GuestBillProps {
  bill: BillResponse;
  business: Business;
  tableCode: string;
  onPaymentComplete: () => void;
  compact?: boolean;
}

export const GuestBill: React.FC<GuestBillProps> = ({
  bill,
  business,
  tableCode,
  onPaymentComplete,
  compact = false,
}) => {
  const [isPaymentModalOpen, setIsPaymentModalOpen] = useState(false);

  const formatCurrency = (amount: number) => {
    return `$${amount.toFixed(2)}`;
  };

  const formatDate = (dateString: string) => {
    return new Date(dateString).toLocaleString();
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

  const remainingAmount = bill.bill.total_amount - bill.bill.paid_amount;

  return (
    <div className="space-y-4">
      {/* Bill Header */}
      {!compact && (
        <Card>
          <CardHeader>
            <div className="flex items-center justify-between w-full">
              <div className="flex items-center gap-2">
                <Receipt className="w-5 h-5" />
                <h2 className="text-xl font-semibold">Bill Details</h2>
              </div>
              <Chip
                color={getStatusColor(bill.bill.status)}
                variant="flat"
              >
                {bill.bill.status.toUpperCase()}
              </Chip>
            </div>
          </CardHeader>
          <CardBody>
            <div className="grid grid-cols-2 gap-4">
              <div>
                <p className="text-sm text-default-500">Bill Number</p>
                <p className="font-mono text-lg">{bill.bill.bill_number}</p>
              </div>
              <div>
                <p className="text-sm text-default-500">Created</p>
                <div className="flex items-center gap-1">
                  <Clock className="w-4 h-4 text-default-400" />
                  <p>{formatDate(bill.bill.created_at)}</p>
                </div>
              </div>
            </div>
          </CardBody>
        </Card>
      )}

      {/* Items */}
      <Card>
        <CardHeader>
          <h3 className="text-lg font-semibold">Items ({bill.items.length})</h3>
        </CardHeader>
        <CardBody>
          <div className="space-y-3">
            {bill.items.map((item, index) => (
              <div key={index} className="flex justify-between items-center p-3 bg-default-50 rounded-lg">
                <div className="flex-1">
                  <p className="font-medium">{item.name}</p>
                  <p className="text-sm text-default-500">
                    Quantity: {item.quantity} × {formatCurrency(item.price)}
                  </p>
                </div>
                <p className="font-semibold text-lg">{formatCurrency(item.subtotal)}</p>
              </div>
            ))}
          </div>
        </CardBody>
      </Card>

      {/* Bill Summary */}
      <Card>
        <CardHeader>
          <h3 className="text-lg font-semibold">Bill Summary</h3>
        </CardHeader>
        <CardBody>
          <div className="space-y-3">
            <div className="flex justify-between">
              <span>Subtotal:</span>
              <span>{formatCurrency(bill.bill.subtotal)}</span>
            </div>
            
            {bill.bill.tax_amount > 0 && (
              <div className="flex justify-between">
                <span>Tax ({business.tax_rate}%):</span>
                <span>{formatCurrency(bill.bill.tax_amount)}</span>
              </div>
            )}
            
            {bill.bill.service_fee_amount > 0 && (
              <div className="flex justify-between">
                <span>Service Fee ({business.service_fee_rate}%):</span>
                <span>{formatCurrency(bill.bill.service_fee_amount)}</span>
              </div>
            )}
            
            <Divider />
            
            <div className="flex justify-between font-semibold text-xl">
              <span>Total:</span>
              <span>{formatCurrency(bill.bill.total_amount)}</span>
            </div>
            
            {bill.bill.paid_amount > 0 && (
              <>
                <div className="flex justify-between text-success">
                  <span>Paid:</span>
                  <span>{formatCurrency(bill.bill.paid_amount)}</span>
                </div>
                <div className="flex justify-between font-medium">
                  <span>Remaining:</span>
                  <span className={remainingAmount > 0 ? 'text-warning' : 'text-success'}>
                    {formatCurrency(remainingAmount)}
                  </span>
                </div>
              </>
            )}

            {bill.bill.tip_amount > 0 && (
              <div className="flex justify-between text-primary">
                <span>Tip:</span>
                <span>{formatCurrency(bill.bill.tip_amount)}</span>
              </div>
            )}
          </div>
        </CardBody>
      </Card>

      {/* Payment Actions */}
      {bill.bill.status === 'open' && remainingAmount > 0 && (
        <Card className="bg-primary-50 border-primary-200">
          <CardBody>
            <div className="text-center space-y-4">
              <div>
                <h3 className="font-semibold text-primary mb-2">Ready to Pay?</h3>
                <p className="text-sm text-primary-600 mb-4">
                  Amount due: <span className="font-bold text-lg">{formatCurrency(remainingAmount)}</span>
                </p>
              </div>
              
              <div className="flex flex-col sm:flex-row gap-3 justify-center">
                <Button
                  color="primary"
                  startContent={<Wallet className="w-4 h-4" />}
                  className="flex-1 sm:flex-none"
                  onPress={() => setIsPaymentModalOpen(true)}
                >
                  Pay with Crypto
                </Button>
                <Button
                  color="primary"
                  variant="bordered"
                  startContent={<CreditCard className="w-4 h-4" />}
                  className="flex-1 sm:flex-none"
                  isDisabled
                >
                  Pay with Card (Coming Soon)
                </Button>
              </div>
              
              <div className="text-xs text-default-500">
                <p>Settlement Address: {bill.bill.settlement_address}</p>
                <p>Tipping Address: {bill.bill.tipping_address}</p>
              </div>
            </div>
          </CardBody>
        </Card>
      )}

      {/* Bill Status Messages */}
      {bill.bill.status === 'paid' && (
        <Card className="bg-success-50 border-success-200">
          <CardBody className="text-center">
            <div className="text-success">
              <Receipt className="w-8 h-8 mx-auto mb-2" />
              <h3 className="font-semibold mb-1">Bill Paid</h3>
              <p className="text-sm">Thank you for your payment!</p>
            </div>
          </CardBody>
        </Card>
      )}

      {bill.bill.status === 'closed' && (
        <Card className="bg-default-100">
          <CardBody className="text-center">
            <div className="text-default-600">
              <Receipt className="w-8 h-8 mx-auto mb-2" />
              <h3 className="font-semibold mb-1">Bill Closed</h3>
              <p className="text-sm">This bill has been closed.</p>
            </div>
          </CardBody>
        </Card>
      )}

      {/* Payment Modal */}
      <PaymentProcessor
        isOpen={isPaymentModalOpen}
        onClose={() => setIsPaymentModalOpen(false)}
        billId={bill.bill.id}
        amount={remainingAmount}
        businessName={business.name}
        businessAddress={bill.bill.settlement_address}
        tipAddress={bill.bill.tipping_address}
        onPaymentComplete={() => {
          setIsPaymentModalOpen(false);
          onPaymentComplete();
        }}
      />
    </div>
  );
};

export default GuestBill;
