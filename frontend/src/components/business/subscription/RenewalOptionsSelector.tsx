'use client';

import React from 'react';
import { Card, CardBody, Button, Input, Checkbox, Chip } from '@nextui-org/react';
import { Clock, Star } from 'lucide-react';
import { RenewalOption } from './types';
import { formatSubscriptionTime } from './utils';

interface RenewalOptionsSelectorProps {
  renewalOptions: RenewalOption[];
  selectedOption: RenewalOption;
  onSelectOption: (option: RenewalOption) => void;
  useCustomAmount: boolean;
  onToggleCustomAmount: (use: boolean) => void;
  customAmount: string;
  onCustomAmountChange: (amount: string) => void;
  calculatedSubscriptionTime?: number;
  tString: (key: string) => string;
}

export const RenewalOptionsSelector: React.FC<RenewalOptionsSelectorProps> = ({
  renewalOptions,
  selectedOption,
  onSelectOption,
  useCustomAmount,
  onToggleCustomAmount,
  customAmount,
  onCustomAmountChange,
  calculatedSubscriptionTime,
  tString,
}) => {
  return (
    <div className="space-y-4">
      <h4 className="text-lg font-semibold">{tString('renewalModal.selectOption')}</h4>
      
      {/* Preset Options */}
      <div className="grid grid-cols-1 md:grid-cols-2 gap-3">
        {renewalOptions.map((option) => (
          <Card
            key={option.months}
            isPressable
            isHoverable
            className={`cursor-pointer transition-all ${
              !useCustomAmount && selectedOption.months === option.months
                ? 'ring-2 ring-blue-500 bg-blue-50'
                : 'hover:bg-gray-50'
            }`}
            onPress={() => {
              onSelectOption(option);
              onToggleCustomAmount(false);
            }}
          >
            <CardBody className="p-4">
              <div className="flex items-start justify-between">
                <div className="flex-1">
                  <div className="flex items-center space-x-2">
                    <h5 className="font-semibold">{option.description}</h5>
                    {option.popular && (
                      <Chip
                        size="sm"
                        color="warning"
                        variant="flat"
                        startContent={<Star size={12} />}
                      >
                        {tString('renewalModal.popular')}
                      </Chip>
                    )}
                  </div>
                  <p className="text-2xl font-bold text-blue-600 mt-1">
                    ${option.suggestedAmount}
                  </p>
                  <div className="flex items-center space-x-1 mt-2 text-sm text-gray-500">
                    <Clock size={14} />
                    <span>
                      {option.months === 1 
                        ? tString('renewalModal.timeUnits.month')
                        : tString('renewalModal.timeUnits.months').replace('{count}', option.months.toString())
                      }
                    </span>
                  </div>
                </div>
              </div>
            </CardBody>
          </Card>
        ))}
      </div>

      {/* Custom Amount Option */}
      <Card
        isPressable
        isHoverable
        className={`cursor-pointer transition-all ${
          useCustomAmount
            ? 'ring-2 ring-blue-500 bg-blue-50'
            : 'hover:bg-gray-50'
        }`}
        onPress={() => onToggleCustomAmount(true)}
      >
        <CardBody className="p-4">
          <div className="space-y-3">
            <div className="flex items-center justify-between">
              <h5 className="font-semibold">{tString('renewalModal.customAmount')}</h5>
              <Checkbox
                isSelected={useCustomAmount}
                onValueChange={onToggleCustomAmount}
              />
            </div>
            
            {useCustomAmount && (
              <div className="space-y-3">
                <Input
                  type="number"
                  label={tString('renewalModal.enterAmount')}
                  placeholder="0.00"
                  value={customAmount}
                  onValueChange={onCustomAmountChange}
                  startContent={
                    <div className="pointer-events-none flex items-center">
                      <span className="text-default-400 text-small">$</span>
                    </div>
                  }
                  min="1"
                  step="0.01"
                />
                
                {typeof calculatedSubscriptionTime === 'number' && calculatedSubscriptionTime > 0 && customAmount && parseFloat(customAmount) > 0 && (
                  <div className="bg-blue-50 rounded-lg p-3">
                    <div className="flex items-center space-x-2">
                      <Clock className="h-4 w-4 text-blue-600" />
                      <p className="text-sm text-blue-800">
                        {tString('renewalModal.calculatedTime')}: {' '}
                        <span className="font-semibold">
                          {formatSubscriptionTime(calculatedSubscriptionTime, tString)}
                        </span>
                      </p>
                    </div>
                  </div>
                )}
              </div>
            )}
          </div>
        </CardBody>
      </Card>
    </div>
  );
};
