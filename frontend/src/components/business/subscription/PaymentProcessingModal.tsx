'use client';

import React from 'react';
import { Modal, ModalContent, ModalHeader, ModalBody, Progress, Button } from '@nextui-org/react';
import { CreditCard, CheckCircle, Loader, AlertCircle } from 'lucide-react';
import { ProcessingStep } from './types';

interface PaymentProcessingModalProps {
  isOpen: boolean;
  processingStep: ProcessingStep;
  tString: (key: string) => string;
}

export const PaymentProcessingModal: React.FC<PaymentProcessingModalProps> = ({
  isOpen,
  processingStep,
  tString,
}) => {
  const getStepProgress = (step: ProcessingStep) => {
    switch (step) {
      case 'approval': return 33;
      case 'renewal': return 66;
      case 'complete': return 100;
      default: return 0;
    }
  };

  const getStepIcon = (step: ProcessingStep) => {
    switch (step) {
      case 'approval': return <CreditCard className="h-6 w-6 text-blue-600" />;
      case 'renewal': return <Loader className="h-6 w-6 text-blue-600 animate-spin" />;
      case 'complete': return <CheckCircle className="h-6 w-6 text-green-600" />;
      default: return <AlertCircle className="h-6 w-6 text-gray-600" />;
    }
  };

  const getStepTitle = (step: ProcessingStep) => {
    switch (step) {
      case 'approval': return tString('processingModal.approvingPayment');
      case 'renewal': return tString('processingModal.processingRenewal');
      case 'complete': return tString('processingModal.renewalComplete');
      default: return tString('processingModal.processing');
    }
  };

  const getStepDescription = (step: ProcessingStep) => {
    switch (step) {
      case 'approval': return tString('processingModal.approvingDescription');
      case 'renewal': return tString('processingModal.processingDescription');
      case 'complete': return tString('processingModal.completeDescription');
      default: return '';
    }
  };

  return (
    <Modal 
      isOpen={isOpen} 
      isDismissable={false}
      hideCloseButton
      size="md"
      backdrop="blur"
    >
      <ModalContent>
        <ModalHeader className="flex flex-col gap-1">
          <div className="flex items-center space-x-3">
            {getStepIcon(processingStep)}
            <h3 className="text-xl font-semibold">
              {getStepTitle(processingStep)}
            </h3>
          </div>
        </ModalHeader>
        
        <ModalBody className="pb-6">
          <div className="space-y-6">
            {/* Progress Bar */}
            <div className="space-y-2">
              <Progress
                value={getStepProgress(processingStep)}
                color={processingStep === 'complete' ? 'success' : 'primary'}
                className="w-full"
              />
              <div className="flex justify-between text-sm text-gray-500">
                <span>{tString('processingModal.step')} {processingStep === 'approval' ? '1' : processingStep === 'renewal' ? '2' : '3'}/3</span>
                <span>{getStepProgress(processingStep)}%</span>
              </div>
            </div>

            {/* Step Description */}
            <div className="text-center space-y-2">
              <p className="text-gray-600">
                {getStepDescription(processingStep)}
              </p>
              
              {processingStep !== 'complete' && (
                <p className="text-sm text-gray-500">
                  {tString('processingModal.pleaseWait')}
                </p>
              )}
            </div>

            {/* Step Details */}
            <div className="bg-gray-50 rounded-lg p-4">
              <div className="space-y-3">
                <div className={`flex items-center space-x-3 ${processingStep === 'approval' ? 'text-blue-600' : 'text-gray-400'}`}>
                  <div className={`w-2 h-2 rounded-full ${processingStep === 'approval' ? 'bg-blue-600 animate-pulse' : 'bg-gray-300'}`} />
                  <span className="text-sm font-medium">{tString('processingModal.steps.approval')}</span>
                </div>
                
                <div className={`flex items-center space-x-3 ${processingStep === 'renewal' ? 'text-blue-600' : processingStep === 'complete' ? 'text-green-600' : 'text-gray-400'}`}>
                  <div className={`w-2 h-2 rounded-full ${
                    processingStep === 'renewal' 
                      ? 'bg-blue-600 animate-pulse' 
                      : processingStep === 'complete' 
                        ? 'bg-green-600' 
                        : 'bg-gray-300'
                  }`} />
                  <span className="text-sm font-medium">{tString('processingModal.steps.renewal')}</span>
                </div>
                
                <div className={`flex items-center space-x-3 ${processingStep === 'complete' ? 'text-green-600' : 'text-gray-400'}`}>
                  <div className={`w-2 h-2 rounded-full ${processingStep === 'complete' ? 'bg-green-600' : 'bg-gray-300'}`} />
                  <span className="text-sm font-medium">{tString('processingModal.steps.complete')}</span>
                </div>
              </div>
            </div>

            {/* Important Notice */}
            {processingStep !== 'complete' && (
              <div className="bg-yellow-50 border border-yellow-200 rounded-lg p-3">
                <div className="flex items-start space-x-2">
                  <AlertCircle className="h-4 w-4 text-yellow-600 mt-0.5 flex-shrink-0" />
                  <p className="text-sm text-yellow-800">
                    {tString('processingModal.importantNotice')}
                  </p>
                </div>
              </div>
            )}
          </div>
        </ModalBody>
      </ModalContent>
    </Modal>
  );
};
