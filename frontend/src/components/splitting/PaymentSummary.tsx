'use client';

import React, { useState } from 'react';
import {
  Card,
  CardBody,
  CardHeader,
  Button,
  Divider,
  Avatar,
  Chip,
  Progress,
  Modal,
  ModalContent,
  ModalHeader,
  ModalBody,
  ModalFooter,
  Spinner,
} from '@nextui-org/react';
import { 
  CreditCard, 
  Users, 
  CheckCircle, 
  Clock, 
  AlertCircle,
  Receipt,
  Download,
  Share2,
  Copy
} from 'lucide-react';

export interface PaymentPerson {
  id: string;
  name: string;
  amount: number;
  tipAmount: number;
  totalAmount: number;
  status: 'pending' | 'processing' | 'completed' | 'failed';
  paymentId?: string;
  transactionHash?: string;
  items?: Array<{
    id: string;
    name: string;
    price: number;
    quantity: number;
  }>;
}

export interface PaymentSummaryProps {
  billId: string;
  billNumber: string;
  subtotal: number;
  taxAmount: number;
  serviceFeeAmount: number;
  totalBillAmount: number;
  people: PaymentPerson[];
  splitMethod: 'equal' | 'custom' | 'items';
  onPaymentInitiate: (personId: string) => void;
  onReceiptGenerate: () => void;
  onShareSummary: () => void;
  className?: string;
}

export default function PaymentSummary({
  billId,
  billNumber,
  subtotal,
  taxAmount,
  serviceFeeAmount,
  totalBillAmount,
  people,
  splitMethod,
  onPaymentInitiate,
  onReceiptGenerate,
  onShareSummary,
  className = ''
}: PaymentSummaryProps) {
  const [selectedPerson, setSelectedPerson] = useState<PaymentPerson | null>(null);
  const [isDetailModalOpen, setIsDetailModalOpen] = useState(false);

  const formatCurrency = (amount: number) => `$${amount.toFixed(2)}`;

  const getStatusColor = (status: PaymentPerson['status']) => {
    switch (status) {
      case 'completed': return 'success';
      case 'processing': return 'warning';
      case 'failed': return 'danger';
      default: return 'default';
    }
  };

  const getStatusIcon = (status: PaymentPerson['status']) => {
    switch (status) {
      case 'completed': return <CheckCircle size={16} />;
      case 'processing': return <Spinner size="sm" />;
      case 'failed': return <AlertCircle size={16} />;
      default: return <Clock size={16} />;
    }
  };

  const getStatusText = (status: PaymentPerson['status']) => {
    switch (status) {
      case 'completed': return 'Paid';
      case 'processing': return 'Processing';
      case 'failed': return 'Failed';
      default: return 'Pending';
    }
  };

  const totalPaid = people.reduce((sum, person) => 
    person.status === 'completed' ? sum + person.totalAmount : sum, 0
  );
  
  const totalTips = people.reduce((sum, person) => 
    person.status === 'completed' ? sum + person.tipAmount : sum, 0
  );

  const completedPayments = people.filter(p => p.status === 'completed').length;
  const paymentProgress = people.length > 0 ? (completedPayments / people.length) * 100 : 0;

  const handlePersonClick = (person: PaymentPerson) => {
    setSelectedPerson(person);
    setIsDetailModalOpen(true);
  };

  const copyToClipboard = (text: string) => {
    navigator.clipboard.writeText(text);
  };

  const getSplitMethodLabel = () => {
    switch (splitMethod) {
      case 'equal': return 'Equal Split';
      case 'custom': return 'Custom Amounts';
      case 'items': return 'Item-based Split';
      default: return 'Split';
    }
  };

  return (
    <div className={`space-y-6 ${className}`}>
      {/* Summary Header */}
      <Card>
        <CardHeader>
          <div className="flex items-center justify-between w-full">
            <div className="flex items-center gap-3">
              <Receipt className="w-6 h-6" />
              <div>
                <h3 className="text-lg font-semibold">Payment Summary</h3>
                <p className="text-sm text-default-500">Bill #{billNumber}</p>
              </div>
            </div>
            <Chip color="primary" variant="flat">
              {getSplitMethodLabel()}
            </Chip>
          </div>
        </CardHeader>
        <CardBody className="space-y-4">
          {/* Bill Breakdown */}
          <div className="space-y-2">
            <div className="flex justify-between text-sm">
              <span>Subtotal:</span>
              <span>{formatCurrency(subtotal)}</span>
            </div>
            {taxAmount > 0 && (
              <div className="flex justify-between text-sm">
                <span>Tax:</span>
                <span>{formatCurrency(taxAmount)}</span>
              </div>
            )}
            {serviceFeeAmount > 0 && (
              <div className="flex justify-between text-sm">
                <span>Service Fee:</span>
                <span>{formatCurrency(serviceFeeAmount)}</span>
              </div>
            )}
            <Divider />
            <div className="flex justify-between font-semibold">
              <span>Bill Total:</span>
              <span>{formatCurrency(totalBillAmount)}</span>
            </div>
            <div className="flex justify-between text-sm text-success">
              <span>Tips Added:</span>
              <span>{formatCurrency(totalTips)}</span>
            </div>
            <div className="flex justify-between text-lg font-bold text-primary">
              <span>Total with Tips:</span>
              <span>{formatCurrency(totalBillAmount + totalTips)}</span>
            </div>
          </div>

          {/* Payment Progress */}
          <div className="space-y-2">
            <div className="flex justify-between text-sm">
              <span>Payment Progress</span>
              <span>{completedPayments} of {people.length} paid</span>
            </div>
            <Progress 
              value={paymentProgress} 
              color="success" 
              className="max-w-full"
            />
            <div className="flex justify-between text-sm text-default-500">
              <span>Paid: {formatCurrency(totalPaid)}</span>
              <span>Remaining: {formatCurrency((totalBillAmount + totalTips) - totalPaid)}</span>
            </div>
          </div>

          {/* Action Buttons */}
          <div className="flex gap-2 pt-2">
            <Button
              size="sm"
              variant="bordered"
              startContent={<Download size={16} />}
              onPress={onReceiptGenerate}
            >
              Receipt
            </Button>
            <Button
              size="sm"
              variant="bordered"
              startContent={<Share2 size={16} />}
              onPress={onShareSummary}
            >
              Share
            </Button>
          </div>
        </CardBody>
      </Card>

      {/* Individual Payments */}
      <Card>
        <CardHeader>
          <div className="flex items-center gap-2">
            <Users className="w-5 h-5" />
            <h3 className="text-lg font-semibold">Individual Payments</h3>
          </div>
        </CardHeader>
        <CardBody className="space-y-3">
          {people.map((person) => (
            <div
              key={person.id}
              className="flex items-center justify-between p-4 bg-default-50 rounded-lg hover:bg-default-100 cursor-pointer transition-colors"
              onClick={() => handlePersonClick(person)}
            >
              <div className="flex items-center gap-3">
                <Avatar 
                  size="sm" 
                  name={person.name}
                  className="bg-primary text-white"
                />
                <div>
                  <p className="font-medium">{person.name}</p>
                  <div className="flex items-center gap-2">
                    <span className="text-sm text-default-500">
                      {formatCurrency(person.amount)} + {formatCurrency(person.tipAmount)} tip
                    </span>
                  </div>
                </div>
              </div>
              
              <div className="flex items-center gap-3">
                <div className="text-right">
                  <p className="font-semibold">{formatCurrency(person.totalAmount)}</p>
                  <Chip
                    size="sm"
                    color={getStatusColor(person.status)}
                    variant="flat"
                    startContent={getStatusIcon(person.status)}
                  >
                    {getStatusText(person.status)}
                  </Chip>
                </div>
                
                {person.status === 'pending' && (
                  <Button
                    size="sm"
                    color="primary"
                    startContent={<CreditCard size={16} />}
                    onPress={() => {
                      onPaymentInitiate(person.id);
                    }}
                  >
                    Pay Now
                  </Button>
                )}
              </div>
            </div>
          ))}
        </CardBody>
      </Card>

      {/* Person Detail Modal */}
      <Modal 
        isOpen={isDetailModalOpen} 
        onClose={() => setIsDetailModalOpen(false)}
        size="md"
      >
        <ModalContent>
          {selectedPerson && (
            <>
              <ModalHeader>
                <div className="flex items-center gap-3">
                  <Avatar 
                    size="sm" 
                    name={selectedPerson.name}
                    className="bg-primary text-white"
                  />
                  <div>
                    <h3 className="font-semibold">{selectedPerson.name}</h3>
                    <p className="text-sm text-default-500">Payment Details</p>
                  </div>
                </div>
              </ModalHeader>
              <ModalBody>
                <div className="space-y-4">
                  {/* Payment Status */}
                  <div className="flex items-center justify-between p-3 bg-default-50 rounded-lg">
                    <span className="font-medium">Status:</span>
                    <Chip
                      color={getStatusColor(selectedPerson.status)}
                      variant="flat"
                      startContent={getStatusIcon(selectedPerson.status)}
                    >
                      {getStatusText(selectedPerson.status)}
                    </Chip>
                  </div>

                  {/* Amount Breakdown */}
                  <div className="space-y-2">
                    <h4 className="font-medium">Amount Breakdown</h4>
                    <div className="space-y-1 text-sm">
                      <div className="flex justify-between">
                        <span>Bill Amount:</span>
                        <span>{formatCurrency(selectedPerson.amount)}</span>
                      </div>
                      <div className="flex justify-between">
                        <span>Tip:</span>
                        <span>{formatCurrency(selectedPerson.tipAmount)}</span>
                      </div>
                      <Divider />
                      <div className="flex justify-between font-semibold">
                        <span>Total:</span>
                        <span>{formatCurrency(selectedPerson.totalAmount)}</span>
                      </div>
                    </div>
                  </div>

                  {/* Items (for item-based split) */}
                  {selectedPerson.items && selectedPerson.items.length > 0 && (
                    <div className="space-y-2">
                      <h4 className="font-medium">Items</h4>
                      <div className="space-y-1">
                        {selectedPerson.items.map((item) => (
                          <div key={item.id} className="flex justify-between text-sm p-2 bg-default-50 rounded">
                            <span>{item.name} × {item.quantity}</span>
                            <span>{formatCurrency(item.price * item.quantity)}</span>
                          </div>
                        ))}
                      </div>
                    </div>
                  )}

                  {/* Transaction Details */}
                  {selectedPerson.transactionHash && (
                    <div className="space-y-2">
                      <h4 className="font-medium">Transaction Details</h4>
                      <div className="p-3 bg-default-50 rounded-lg">
                        <div className="flex items-center justify-between">
                          <span className="text-sm text-default-500">Transaction Hash:</span>
                          <Button
                            size="sm"
                            variant="light"
                            isIconOnly
                            onPress={() => copyToClipboard(selectedPerson.transactionHash!)}
                          >
                            <Copy size={14} />
                          </Button>
                        </div>
                        <p className="text-xs font-mono break-all mt-1">
                          {selectedPerson.transactionHash}
                        </p>
                      </div>
                    </div>
                  )}
                </div>
              </ModalBody>
              <ModalFooter>
                <Button 
                  variant="light" 
                  onPress={() => setIsDetailModalOpen(false)}
                >
                  Close
                </Button>
                {selectedPerson.status === 'pending' && (
                  <Button 
                    color="primary"
                    startContent={<CreditCard size={16} />}
                    onPress={() => {
                      setIsDetailModalOpen(false);
                      onPaymentInitiate(selectedPerson.id);
                    }}
                  >
                    Pay Now
                  </Button>
                )}
              </ModalFooter>
            </>
          )}
        </ModalContent>
      </Modal>
    </div>
  );
}
