'use client';

import React, { useState, useEffect } from 'react';
import {
  Modal,
  ModalContent,
  ModalHeader,
  ModalBody,
  ModalFooter,
  Button,
  Spinner,
} from '@nextui-org/react';
import SplittingAPI from '../../api/splitting';

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

const BillSplittingFlow: React.FC<BillSplittingFlowProps> = ({
  bill,
  businessName,
  isOpen,
  onClose,
}) => {
  const [loading, setLoading] = useState(false);
  const [splitOptions, setSplitOptions] = useState<any>(null);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    if (isOpen && bill.id) {
      console.log('BillSplittingFlow: Modal opened for bill:', bill);
      loadSplitOptions();
    }
  }, [isOpen, bill.id]);

  const loadSplitOptions = async () => {
    try {
      setLoading(true);
      setError(null);
      console.log('Loading split options for bill:', bill.id);
      
      const options = await SplittingAPI.getSplitOptions(bill.id);
      console.log('Split options loaded:', options);
      setSplitOptions(options);
    } catch (err) {
      console.error('Error loading split options:', err);
      setError(err instanceof Error ? err.message : 'Failed to load split options');
    } finally {
      setLoading(false);
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
        <ModalHeader className="flex flex-col gap-1">
          <h2 className="text-2xl font-semibold">Split Bill</h2>
          <p className="text-sm text-gray-600">
            {businessName} • Bill #{bill.billNumber}
          </p>
        </ModalHeader>
        
        <ModalBody>
          <div className="space-y-6">
            <div className="text-center">
              <h3 className="text-lg font-medium">Total Amount</h3>
              <p className="text-3xl font-bold text-green-600">
                ${bill.totalAmount.toFixed(2)}
              </p>
            </div>
            
            {loading && (
              <div className="flex justify-center">
                <Spinner size="lg" />
                <span className="ml-2">Loading split options...</span>
              </div>
            )}
            
            {error && (
              <div className="text-center text-red-600">
                <p>Error: {error}</p>
              </div>
            )}
            
            {splitOptions && (
              <div className="text-center text-green-600">
                <p>✅ Split options loaded successfully!</p>
                <p className="text-sm mt-2">
                  Available methods: {Object.keys(splitOptions.split_options || {}).join(', ')}
                </p>
                <p className="text-sm">
                  Total items: {splitOptions.items?.length || 0}
                </p>
              </div>
            )}
            
            {!loading && !error && !splitOptions && (
              <div className="text-center text-gray-600">
                <p>Ready to load split options...</p>
              </div>
            )}
          </div>
        </ModalBody>
        
        <ModalFooter>
          <Button
            color="danger"
            variant="light"
            onPress={onClose}
          >
            Close
          </Button>
        </ModalFooter>
      </ModalContent>
    </Modal>
  );
};

export default BillSplittingFlow;
