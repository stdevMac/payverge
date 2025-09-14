'use client';

import React, { useState, useEffect, useCallback } from 'react';
import {
  Card,
  CardBody,
  Button,
  Modal,
  ModalContent,
  ModalHeader,
  ModalBody,
  ModalFooter,
  Spinner,
  Alert,
} from '@nextui-org/react';
import { Calculator, Users, ArrowLeft, ArrowRight } from 'lucide-react';

import SplitSelector, { SplitMethod } from './SplitSelector';
import TipCalculator from './TipCalculator';
import PaymentSummary, { PaymentPerson } from './PaymentSummary';
import { useSplittingAPI, SplitResult, SplitOptions } from '../../api/splitting';
import { useSplitPaymentWebSocket } from '../../hooks/useSplitPaymentWebSocket';
import { ReceiptGenerator, ReceiptData } from '../../utils/receiptGenerator';

export interface BillData {
  id: number;
  billNumber: string;
  subtotal: number;
  taxAmount: number;
  serviceFeeAmount: number;
  totalAmount: number;
  items: Array<{
    id: string;
    name: string;
    price: number;
    quantity: number;
    subtotal: number;
  }>;
}

export interface BillSplittingFlowProps {
  bill: BillData;
  businessName: string;
  businessAddress?: string;
  tableNumber?: string;
  isOpen: boolean;
  onClose: () => void;
  onPaymentInitiate: (personId: string, amount: number, tipAmount: number) => void;
  className?: string;
}

type FlowStep = 'split' | 'tip' | 'summary';

export default function BillSplittingFlow({
  bill,
  businessName,
  businessAddress,
  tableNumber,
  isOpen,
  onClose,
  onPaymentInitiate,
  className = ''
}: BillSplittingFlowProps) {
  const [currentStep, setCurrentStep] = useState<FlowStep>('split');
  const [splitMethod, setSplitMethod] = useState<SplitMethod>('equal');
  const [splitData, setSplitData] = useState<any>(null);
  const [splitResult, setSplitResult] = useState<SplitResult | null>(null);
  const [splitOptions, setSplitOptions] = useState<SplitOptions | null>(null);
  const [tipAmount, setTipAmount] = useState(0);
  const [tipPercentage, setTipPercentage] = useState(18);
  const [people, setPeople] = useState<PaymentPerson[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [splitId, setSplitId] = useState<string>('');

  const splittingAPI = useSplittingAPI();

  // WebSocket for real-time split payment tracking
  const {
    isConnected: wsConnected,
    splitProgress,
    notifyPaymentStarted,
    notifyPaymentCompleted,
    notifyPaymentFailed
  } = useSplitPaymentWebSocket({
    billId: bill.id,
    splitId: splitId,
    onSplitPaymentReceived: (notification) => {
      // Update person payment status when payment is received
      setPeople(prev => prev.map(person => 
        person.id === notification.person_id 
          ? { ...person, status: 'completed', transactionHash: notification.transaction_hash }
          : person
      ));
    },
    onSplitPaymentUpdate: (notification) => {
      // Update person payment status on updates
      setPeople(prev => prev.map(person => 
        person.id === notification.person_id 
          ? { ...person, status: notification.status }
          : person
      ));
    }
  });

  const loadSplitOptions = useCallback(async () => {
    try {
      setLoading(true);
      const options = await splittingAPI.getSplitOptions(bill.id);
      setSplitOptions(options);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to load split options');
    } finally {
      setLoading(false);
    }
  }, [bill.id, splittingAPI]);

  useEffect(() => {
    if (isOpen && bill.id) {
      loadSplitOptions();
      // Generate unique split ID for this session
      setSplitId(`split_${bill.id}_${Date.now()}`);
    }
  }, [isOpen, bill.id, loadSplitOptions]);

  // Update people when split result changes
  useEffect(() => {
    if (splitResult) {
      const updatedPeople: PaymentPerson[] = splitResult.people.map(person => ({
        id: person.person_id,
        name: person.name,
        amount: person.base_amount + person.tax_amount + person.service_fee_amount,
        tipAmount: person.tip_amount,
        totalAmount: person.total_amount,
        status: 'pending' as const,
        items: person.items
      }));
      setPeople(updatedPeople);
    }
  }, [splitResult]);


  const handleSplitChange = async (method: SplitMethod, data: any) => {
    try {
      setLoading(true);
      setError(null);
      setSplitMethod(method);
      setSplitData(data);

      let result: SplitResult;

      switch (method) {
        case 'equal':
          result = await splittingAPI.calculateEqualSplit({
            bill_id: bill.id,
            num_people: data.num_people,
            people: data.people
          });
          break;
        case 'custom':
          result = await splittingAPI.calculateCustomSplit({
            bill_id: bill.id,
            amounts: data.amounts,
            people: data.people
          });
          break;
        case 'items':
          result = await splittingAPI.calculateItemSplit({
            bill_id: bill.id,
            item_selections: data.item_selections,
            people: data.people
          });
          break;
        default:
          throw new Error('Invalid split method');
      }

      setSplitResult(result);
    } catch (err) {
      setError('Failed to calculate split');
      console.error('Error calculating split:', err);
    } finally {
      setLoading(false);
    }
  };

  const handleTipChange = (newTipAmount: number, newTipPercentage: number) => {
    setTipAmount(newTipAmount);
    setTipPercentage(newTipPercentage);

    // Recalculate split with new tip
    if (splitResult && splitData) {
      const tipPerPerson = newTipAmount / splitResult.people.length;
      const updatedPeople = people.map(person => ({
        ...person,
        tipAmount: tipPerPerson,
        totalAmount: person.amount + tipPerPerson
      }));
      setPeople(updatedPeople);
    }
  };

  const handleNextStep = () => {
    if (currentStep === 'split' && splitResult) {
      setCurrentStep('tip');
    } else if (currentStep === 'tip') {
      setCurrentStep('summary');
    }
  };

  const handlePreviousStep = () => {
    if (currentStep === 'summary') {
      setCurrentStep('tip');
    } else if (currentStep === 'tip') {
      setCurrentStep('split');
    }
  };

  const handlePaymentInitiate = (personId: string) => {
    const person = people.find(p => p.id === personId);
    if (person) {
      // Notify WebSocket that payment has started
      notifyPaymentStarted(personId, person.name, person.amount, person.tipAmount);
      
      // Update person status to processing
      setPeople(prev => prev.map(p => 
        p.id === personId ? { ...p, status: 'processing' as const } : p
      ));
      
      onPaymentInitiate(personId, person.amount, person.tipAmount);
    }
  };

  const handleReceiptGenerate = () => {
    const receiptData: ReceiptData = {
      billId: bill.id.toString(),
      billNumber: bill.billNumber,
      businessName: businessName,
      businessAddress: businessAddress,
      tableNumber: tableNumber,
      subtotal: bill.subtotal,
      taxAmount: bill.taxAmount,
      serviceFeeAmount: bill.serviceFeeAmount,
      totalBillAmount: bill.totalAmount,
      splitMethod: splitMethod,
      people: people,
      generatedAt: new Date().toISOString(),
      items: bill.items
    };

    // Show options for receipt generation
    const action = confirm('Choose receipt format:\nOK = Download HTML\nCancel = Print');
    if (action) {
      ReceiptGenerator.downloadHTMLReceipt(receiptData);
    } else {
      ReceiptGenerator.printReceipt(receiptData);
    }
  };

  const handleShareSummary = async () => {
    const receiptData: ReceiptData = {
      billId: bill.id.toString(),
      billNumber: bill.billNumber,
      businessName: businessName,
      businessAddress: businessAddress,
      tableNumber: tableNumber,
      subtotal: bill.subtotal,
      taxAmount: bill.taxAmount,
      serviceFeeAmount: bill.serviceFeeAmount,
      totalBillAmount: bill.totalAmount,
      splitMethod: splitMethod,
      people: people,
      generatedAt: new Date().toISOString(),
      items: bill.items
    };

    const success = await ReceiptGenerator.copyShareableText(receiptData);
    if (success) {
      alert('Payment summary copied to clipboard!');
    } else {
      alert('Failed to copy to clipboard. Please try again.');
    }
  };

  const canProceedToTip = splitResult && splitResult.people.length > 0;
  const canProceedToSummary = canProceedToTip && tipAmount >= 0;

  const getStepTitle = () => {
    switch (currentStep) {
      case 'split': return 'Split the Bill';
      case 'tip': return 'Add Tip';
      case 'summary': return 'Payment Summary';
      default: return 'Bill Splitting';
    }
  };

  const renderStepContent = () => {
    if (loading) {
      return (
        <div className="flex items-center justify-center py-12">
          <Spinner size="lg" />
          <span className="ml-3">Loading...</span>
        </div>
      );
    }

    if (error) {
      return (
        <Alert color="danger" className="mb-4">
          {error}
        </Alert>
      );
    }

    switch (currentStep) {
      case 'split':
        return splitOptions ? (
          <SplitSelector
            billAmount={bill.totalAmount}
            billItems={bill.items}
            onSplitChange={handleSplitChange}
            disabled={loading}
          />
        ) : null;

      case 'tip':
        return (
          <TipCalculator
            subtotal={bill.subtotal}
            taxAmount={bill.taxAmount}
            serviceFeeAmount={bill.serviceFeeAmount}
            numPeople={splitResult?.people.length || 1}
            onTipChange={handleTipChange}
            disabled={loading}
          />
        );

      case 'summary':
        return (
          <PaymentSummary
            billId={bill.id.toString()}
            billNumber={bill.billNumber}
            subtotal={bill.subtotal}
            taxAmount={bill.taxAmount}
            serviceFeeAmount={bill.serviceFeeAmount}
            totalBillAmount={bill.totalAmount}
            people={people}
            splitMethod={splitMethod}
            onPaymentInitiate={handlePaymentInitiate}
            onReceiptGenerate={handleReceiptGenerate}
            onShareSummary={handleShareSummary}
          />
        );

      default:
        return null;
    }
  };

  return (
    <Modal 
      isOpen={isOpen} 
      onClose={onClose}
      size="5xl"
      scrollBehavior="inside"
      className={className}
    >
      <ModalContent>
        <ModalHeader>
          <div className="flex items-center gap-3">
            <Calculator className="w-6 h-6" />
            <div>
              <h3 className="text-xl font-semibold">{getStepTitle()}</h3>
              <p className="text-sm text-default-500">Bill #{bill.billNumber}</p>
            </div>
          </div>
        </ModalHeader>
        
        <ModalBody>
          {/* Step Indicator */}
          <div className="flex items-center justify-center mb-6">
            <div className="flex items-center space-x-4">
              <div className={`flex items-center justify-center w-8 h-8 rounded-full text-sm font-medium ${
                currentStep === 'split' ? 'bg-primary text-white' : 
                ['tip', 'summary'].includes(currentStep) ? 'bg-success text-white' : 'bg-default-200'
              }`}>
                1
              </div>
              <div className={`w-12 h-1 ${
                ['tip', 'summary'].includes(currentStep) ? 'bg-success' : 'bg-default-200'
              }`} />
              <div className={`flex items-center justify-center w-8 h-8 rounded-full text-sm font-medium ${
                currentStep === 'tip' ? 'bg-primary text-white' : 
                currentStep === 'summary' ? 'bg-success text-white' : 'bg-default-200'
              }`}>
                2
              </div>
              <div className={`w-12 h-1 ${
                currentStep === 'summary' ? 'bg-success' : 'bg-default-200'
              }`} />
              <div className={`flex items-center justify-center w-8 h-8 rounded-full text-sm font-medium ${
                currentStep === 'summary' ? 'bg-primary text-white' : 'bg-default-200'
              }`}>
                3
              </div>
            </div>
          </div>

          {/* Step Content */}
          <div className="min-h-[400px]">
            {renderStepContent()}
          </div>
        </ModalBody>

        <ModalFooter>
          <div className="flex justify-between w-full">
            <div>
              {currentStep !== 'split' && (
                <Button
                  variant="bordered"
                  startContent={<ArrowLeft size={16} />}
                  onPress={handlePreviousStep}
                  disabled={loading}
                >
                  Back
                </Button>
              )}
            </div>
            
            <div className="flex gap-2">
              <Button 
                variant="light" 
                onPress={onClose}
                disabled={loading}
              >
                Cancel
              </Button>
              
              {currentStep === 'split' && (
                <Button
                  color="primary"
                  endContent={<ArrowRight size={16} />}
                  onPress={handleNextStep}
                  disabled={!canProceedToTip || loading}
                >
                  Add Tip
                </Button>
              )}
              
              {currentStep === 'tip' && (
                <Button
                  color="primary"
                  endContent={<ArrowRight size={16} />}
                  onPress={handleNextStep}
                  disabled={!canProceedToSummary || loading}
                >
                  Review & Pay
                </Button>
              )}
              
              {currentStep === 'summary' && (
                <Button
                  color="success"
                  startContent={<Users size={16} />}
                  onPress={onClose}
                >
                  Done
                </Button>
              )}
            </div>
          </div>
        </ModalFooter>
      </ModalContent>
    </Modal>
  );
}
