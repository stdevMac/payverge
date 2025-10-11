'use client';

import React, { useState, useMemo } from 'react';
import {
  Card,
  CardBody,
  CardHeader,
  Chip,
  Divider,
  Button,
  Modal,
  ModalContent,
  ModalHeader,
  ModalBody,
  ModalFooter,
  useDisclosure,
} from '@nextui-org/react';
import { Receipt, Clock, CreditCard, Wallet, Users } from 'lucide-react';
import { BillWithItemsResponse } from '../../api/bills';
import { Business } from '../../api/business';
import PaymentProcessor from '../payment/PaymentProcessor';
import BillSplittingFlow, { BillData } from '../splitting/BillSplittingFlow';
// import ParticipantTracker from '../blockchain/ParticipantTracker'; // Temporarily disabled for debugging

interface GuestBillProps {
  bill: BillWithItemsResponse;
  business: Business;
  tableCode: string;
  onPaymentComplete: (paymentDetails: { totalPaid: number; tipAmount: number }) => void;
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
  const [isSplittingModalOpen, setIsSplittingModalOpen] = useState(false);
  const { isOpen: isCashierModalOpen, onOpen: onCashierModalOpen, onClose: onCashierModalClose } = useDisclosure();

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
      case 'cancelled':
        return 'danger';
      default:
        return 'default';
    }
  };

  // Consolidate items by name and price, summing quantities
  const consolidateItems = (items: any[]) => {
    const itemMap = new Map();
    
    items.forEach(item => {
      // Create a unique key based on name and price
      const key = `${item.name}-${item.price}`;
      
      if (itemMap.has(key)) {
        // Item already exists, add to quantity and subtotal
        const existingItem = itemMap.get(key);
        existingItem.quantity += item.quantity;
        existingItem.subtotal += item.subtotal;
      } else {
        // New item, add to map
        itemMap.set(key, {
          name: item.name,
          price: item.price,
          quantity: item.quantity,
          subtotal: item.subtotal,
          menu_item_id: item.menu_item_id
        });
      }
    });
    
    return Array.from(itemMap.values());
  };

  // Memoize expensive calculations to prevent re-computation on every render
  const consolidatedItems = useMemo(() => consolidateItems(bill.items), [bill.items]);

  const remainingAmount = useMemo(() => 
    bill.bill.total_amount - bill.bill.paid_amount, 
    [bill.bill.total_amount, bill.bill.paid_amount]
  );

  // Convert bill data for splitting component (use stable IDs and memoize)
  const billData: BillData = useMemo(() => ({
    id: bill.bill.id,
    billNumber: bill.bill.bill_number,
    subtotal: bill.bill.subtotal,
    taxAmount: bill.bill.tax_amount,
    serviceFeeAmount: bill.bill.service_fee_amount,
    totalAmount: bill.bill.total_amount,
    items: bill.items.map((item, index) => ({
      id: item.id?.toString() || `item_${bill.bill.id}_${index}`, // Use stable ID based on bill ID and index
      name: item.name,
      price: item.price,
      quantity: item.quantity,
      subtotal: item.subtotal
    }))
  }), [bill.bill.id, bill.bill.bill_number, bill.bill.subtotal, bill.bill.tax_amount, 
       bill.bill.service_fee_amount, bill.bill.total_amount, bill.items]);

  const handleSplitPaymentInitiate = (personId: string, amount: number, tipAmount: number) => {
    // Close splitting modal and open payment processor with split amount
    setIsSplittingModalOpen(false);
    setIsPaymentModalOpen(true);
    // Note: PaymentProcessor will need to be updated to handle split payments
  };

  return (
    <div className="space-y-8">
      {/* Bill Header */}
      {!compact && (
        <div className="bg-white border border-gray-200 rounded-2xl p-6 shadow-sm">
          <div className="flex items-center justify-between mb-6">
            <div className="flex items-center gap-3">
              <div className="w-12 h-12 bg-gray-50 rounded-xl flex items-center justify-center">
                <Receipt className="w-6 h-6 text-gray-600" />
              </div>
              <h2 className="text-2xl font-light text-gray-900 tracking-wide">Bill Details</h2>
            </div>
            <div className={`px-4 py-2 rounded-xl text-sm font-medium tracking-wide ${
              bill.bill.status === 'open' ? 'bg-green-100 text-green-700' :
              bill.bill.status === 'paid' ? 'bg-blue-100 text-blue-700' :
              'bg-gray-100 text-gray-700'
            }`}>
              {bill.bill.status.toUpperCase()}
            </div>
          </div>
          
          <div className="grid grid-cols-2 gap-6">
            <div>
              <p className="text-sm text-gray-500 font-light mb-1">Bill Number</p>
              <p className="font-mono text-lg text-gray-900">{bill.bill.bill_number}</p>
            </div>
            <div>
              <p className="text-sm text-gray-500 font-light mb-1">Created</p>
              <div className="flex items-center gap-2">
                <Clock className="w-4 h-4 text-gray-400" />
                <p className="text-gray-900">{formatDate(bill.bill.created_at)}</p>
              </div>
            </div>
          </div>
        </div>
      )}

      {/* Items */}
      <div className="bg-white border border-gray-200 rounded-2xl p-6 shadow-sm">
        <div className="flex justify-between items-center mb-6">
          <h3 className="text-xl font-light text-gray-900 tracking-wide">
            Items
          </h3>
        </div>
        <div className="space-y-4">
          {consolidatedItems.map((item, index) => (
            <div key={index} className="flex justify-between items-center p-4 bg-gray-50 rounded-xl border border-gray-100">
              <div className="flex-1">
                <p className="font-medium text-gray-900 tracking-wide">{item.name}</p>
                <p className="text-sm text-gray-500 font-light mt-1">
                  Quantity: {item.quantity} × {formatCurrency(item.price)}
                </p>
              </div>
              <p className="font-medium text-lg text-gray-900 tracking-wide">{formatCurrency(item.subtotal)}</p>
            </div>
          ))}
        </div>
      </div>

      {/* Bill Summary */}
      <div className="bg-white border border-gray-200 rounded-2xl p-6 shadow-sm">
        <h3 className="text-xl font-light text-gray-900 tracking-wide mb-6">Bill Summary</h3>
        <div className="space-y-4">
          <div className="flex justify-between text-gray-600">
            <span className="font-light">Subtotal:</span>
            <span className="font-medium">{formatCurrency(bill.bill.subtotal)}</span>
          </div>
          
          {bill.bill.tax_amount > 0 && (
            <div className="flex justify-between text-gray-600">
              <span className="font-light">Tax ({business.tax_rate}%):</span>
              <span className="font-medium">{formatCurrency(bill.bill.tax_amount)}</span>
            </div>
          )}
          
          {bill.bill.service_fee_amount > 0 && (
            <div className="flex justify-between text-gray-600">
              <span className="font-light">Service Fee ({business.service_fee_rate}%):</span>
              <span className="font-medium">{formatCurrency(bill.bill.service_fee_amount)}</span>
            </div>
          )}
          
          <div className="border-t border-gray-200 pt-4">
            <div className="flex justify-between font-medium text-xl text-gray-900 tracking-wide">
              <span>Total:</span>
              <span>{formatCurrency(bill.bill.total_amount)}</span>
            </div>
          </div>
          
          {bill.bill.paid_amount > 0 && (
            <>
              <div className="flex justify-between text-green-600">
                <span className="font-light">Paid:</span>
                <span className="font-medium">{formatCurrency(bill.bill.paid_amount)}</span>
              </div>
              <div className="flex justify-between font-medium">
                <span className="text-gray-600">Remaining:</span>
                <span className={remainingAmount > 0 ? 'text-orange-600' : 'text-green-600'}>
                  {formatCurrency(remainingAmount)}
                </span>
              </div>
            </>
          )}

          {bill.bill.tip_amount > 0 && (
            <div className="flex justify-between text-blue-600">
              <span className="font-light">Tip:</span>
              <span className="font-medium">{formatCurrency(bill.bill.tip_amount)}</span>
            </div>
          )}
        </div>
      </div>

      {/* Participant Tracker - Temporarily disabled for debugging */}
      {/* {bill.bill.status === 'open' && (
        <ParticipantTracker
          billId={bill.bill.id}
          totalAmount={bill.bill.total_amount * 1000000} // Convert to wei (6 decimals)
          className="mb-6"
          refreshInterval={5000} // Refresh every 5 seconds
        />
      )} */}

      {/* Payment Options */}
      {bill.bill.status === 'open' && (
        <div className="bg-white border border-gray-200 rounded-2xl p-6 shadow-sm">
          <h3 className="text-lg font-semibold text-gray-900 mb-4">Payment Options</h3>
          
          <div className="space-y-3">
            {/* Pay with Crypto */}
            <Button
              color="primary"
              size="lg"
              onPress={() => setIsPaymentModalOpen(true)}
              className="w-full"
              startContent={<Wallet className="w-5 h-5" />}
            >
              Pay with Crypto (USDC)
            </Button>

            {/* Pay with Cashier */}
            <Button
              color="secondary"
              variant="bordered"
              size="lg"
              onPress={onCashierModalOpen}
              className="w-full"
              startContent={<CreditCard className="w-5 h-5" />}
            >
              Pay with Cashier (Cash/Card)
            </Button>

            {/* Split Bill */}
            <Button
              color="warning"
              variant="bordered"
              size="lg"
              onPress={() => setIsSplittingModalOpen(true)}
              className="w-full"
              startContent={<Users className="w-5 h-5" />}
            >
              Split Bill
            </Button>
          </div>

          {/* Payment Summary */}
          <div className="mt-4 p-3 bg-gray-50 rounded-lg">
            <div className="flex justify-between items-center text-sm">
              <span className="text-gray-600">Remaining Amount:</span>
              <span className="font-semibold text-gray-900">
                {formatCurrency(remainingAmount)}
              </span>
            </div>
          </div>
        </div>
      )}

      {/* Bill Status Messages */}
      {bill.bill.status === 'paid' && (
        <div className="bg-green-50 border border-green-200 rounded-2xl p-8 text-center shadow-sm">
          <div className="w-16 h-16 bg-green-100 rounded-2xl flex items-center justify-center mx-auto mb-4">
            <Receipt className="w-8 h-8 text-green-600" />
          </div>
          <h3 className="text-xl font-light text-gray-900 tracking-wide mb-2">Bill Paid</h3>
          <p className="text-gray-600 font-light">Thank you for your payment!</p>
        </div>
      )}

      {bill.bill.status === 'closed' && (
        <div className="bg-gray-50 border border-gray-200 rounded-2xl p-8 text-center shadow-sm">
          <div className="w-16 h-16 bg-gray-100 rounded-2xl flex items-center justify-center mx-auto mb-4">
            <Receipt className="w-8 h-8 text-gray-400" />
          </div>
          <h3 className="text-xl font-light text-gray-700 tracking-wide mb-2">Bill Closed</h3>
          <p className="text-gray-500 font-light">This bill has been closed.</p>
        </div>
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
        onPaymentComplete={(paymentDetails) => {
          setIsPaymentModalOpen(false);
          onPaymentComplete(paymentDetails);
        }}
      />

      {/* Bill Splitting Modal */}
      <BillSplittingFlow
        bill={billData}
        businessName={business.name}
        businessAddress={typeof business.address === 'string' ? business.address : business.address?.street}
        tableNumber={tableCode}
        isOpen={isSplittingModalOpen}
        onClose={() => setIsSplittingModalOpen(false)}
        onPaymentInitiate={handleSplitPaymentInitiate}
      />

      {/* Cashier Payment Modal */}
      <Modal isOpen={isCashierModalOpen} onClose={onCashierModalClose} size="lg">
        <ModalContent>
          <ModalHeader className="flex flex-col gap-1">
            Pay with Cashier
          </ModalHeader>
          <ModalBody>
            <div className="text-center space-y-4">
              <div className="w-16 h-16 bg-blue-100 rounded-full flex items-center justify-center mx-auto">
                <CreditCard className="w-8 h-8 text-blue-600" />
              </div>
              
              <div>
                <h3 className="text-lg font-semibold text-gray-900 mb-2">
                  Please pay at the cashier
                </h3>
                <p className="text-gray-600 mb-4">
                  Go to the cashier and pay with cash, card, or any other method they accept. 
                  They will mark your payment in the system.
                </p>
              </div>

              <div className="bg-gray-50 rounded-lg p-4">
                <div className="flex justify-between items-center">
                  <span className="text-gray-600">Amount to pay:</span>
                  <span className="text-xl font-bold text-gray-900">
                    {formatCurrency(remainingAmount)}
                  </span>
                </div>
                <div className="flex justify-between items-center mt-2">
                  <span className="text-sm text-gray-500">Bill #:</span>
                  <span className="text-sm font-mono text-gray-700">
                    {bill.bill.bill_number}
                  </span>
                </div>
              </div>

              <div className="text-sm text-gray-500">
                💡 The cashier will update your payment status automatically
              </div>
            </div>
          </ModalBody>
          <ModalFooter>
            <Button 
              color="primary" 
              onPress={onCashierModalClose}
              className="w-full"
            >
              Got it!
            </Button>
          </ModalFooter>
        </ModalContent>
      </Modal>
    </div>
  );
};

export default GuestBill;
