'use client';

import React, { useState, useEffect, useCallback } from 'react';
import {
  Card,
  CardBody,
  CardHeader,
  Button,
  Input,
  Slider,
  Divider,
  ButtonGroup,
  Chip,
} from '@nextui-org/react';
import { DollarSign, Percent, Calculator } from 'lucide-react';

export interface TipCalculatorProps {
  subtotal: number;
  taxAmount: number;
  serviceFeeAmount: number;
  numPeople?: number;
  onTipChange: (tipAmount: number, tipPercentage: number) => void;
  disabled?: boolean;
  className?: string;
}

export default function TipCalculator({
  subtotal,
  taxAmount,
  serviceFeeAmount,
  numPeople = 1,
  onTipChange,
  disabled = false,
  className = ''
}: TipCalculatorProps) {
  const [tipMethod, setTipMethod] = useState<'percentage' | 'amount'>('percentage');
  const [tipPercentage, setTipPercentage] = useState(18);
  const [tipAmount, setTipAmount] = useState(0);
  const [customPercentage, setCustomPercentage] = useState('');
  const [customAmount, setCustomAmount] = useState('');

  const baseAmount = subtotal + taxAmount + serviceFeeAmount;

  const formatCurrency = (amount: number) => `$${amount.toFixed(2)}`;

  // Calculate tip amount based on percentage
  const calculateTipFromPercentage = useCallback((percentage: number) => {
    return (subtotal * percentage) / 100;
  }, [subtotal]);

  // Calculate tip percentage based on amount
  const calculatePercentageFromTip = useCallback((amount: number) => {
    return subtotal > 0 ? (amount / subtotal) * 100 : 0;
  }, [subtotal]);

  // Update tip amount when percentage changes
  useEffect(() => {
    if (tipMethod === 'percentage') {
      const newTipAmount = calculateTipFromPercentage(tipPercentage);
      setTipAmount(newTipAmount);
      onTipChange(newTipAmount, tipPercentage);
    }
  }, [tipPercentage, tipMethod, subtotal, onTipChange, calculateTipFromPercentage]);

  // Update tip percentage when amount changes
  useEffect(() => {
    if (tipMethod === 'amount') {
      const newTipPercentage = calculatePercentageFromTip(tipAmount);
      setTipPercentage(newTipPercentage);
      onTipChange(tipAmount, newTipPercentage);
    }
  }, [tipAmount, tipMethod, subtotal, onTipChange, calculatePercentageFromTip]);

  const handlePresetPercentage = (percentage: number) => {
    setTipMethod('percentage');
    setTipPercentage(percentage);
    setCustomPercentage('');
  };

  const handleCustomPercentageChange = (value: string) => {
    setCustomPercentage(value);
    const percentage = parseFloat(value);
    if (!isNaN(percentage) && percentage >= 0 && percentage <= 100) {
      setTipMethod('percentage');
      setTipPercentage(percentage);
    }
  };

  const handleCustomAmountChange = (value: string) => {
    setCustomAmount(value);
    const amount = parseFloat(value);
    if (!isNaN(amount) && amount >= 0) {
      setTipMethod('amount');
      setTipAmount(amount);
    }
  };

  const handleSliderChange = (value: number | number[]) => {
    const percentage = Array.isArray(value) ? value[0] : value;
    setTipMethod('percentage');
    setTipPercentage(percentage);
    setCustomPercentage('');
  };

  const totalWithTip = baseAmount + tipAmount;
  const perPersonAmount = totalWithTip / numPeople;
  const tipPerPerson = tipAmount / numPeople;

  const getTipQuality = (percentage: number) => {
    if (percentage >= 20) return { label: 'Excellent', color: 'success' as const };
    if (percentage >= 18) return { label: 'Great', color: 'primary' as const };
    if (percentage >= 15) return { label: 'Good', color: 'warning' as const };
    if (percentage >= 10) return { label: 'Fair', color: 'default' as const };
    return { label: 'Low', color: 'danger' as const };
  };

  const tipQuality = getTipQuality(tipPercentage);

  return (
    <Card className={className}>
      <CardHeader>
        <div className="flex items-center gap-2">
          <DollarSign className="w-5 h-5" />
          <h3 className="text-lg font-semibold">Tip Calculator</h3>
          <Chip 
            size="sm" 
            color={tipQuality.color}
            variant="flat"
          >
            {tipQuality.label}
          </Chip>
        </div>
      </CardHeader>
      <CardBody className="space-y-6">
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
            <span>{formatCurrency(baseAmount)}</span>
          </div>
        </div>

        {/* Preset Tip Percentages */}
        <div className="space-y-3">
          <p className="text-sm font-medium">Quick Tip Options</p>
          <div className="grid grid-cols-4 gap-2">
            {[15, 18, 20, 25].map((percentage) => (
              <Button
                key={percentage}
                size="sm"
                variant={tipPercentage === percentage && tipMethod === 'percentage' ? 'solid' : 'bordered'}
                color={tipPercentage === percentage && tipMethod === 'percentage' ? 'primary' : 'default'}
                onPress={() => handlePresetPercentage(percentage)}
                disabled={disabled}
              >
                {percentage}%
              </Button>
            ))}
          </div>
        </div>

        {/* Tip Slider */}
        <div className="space-y-3">
          <div className="flex justify-between items-center">
            <p className="text-sm font-medium">Tip Percentage</p>
            <span className="text-sm text-default-500">{tipPercentage.toFixed(1)}%</span>
          </div>
          <Slider
            size="sm"
            step={0.5}
            minValue={0}
            maxValue={30}
            value={tipPercentage}
            onChange={handleSliderChange}
            className="max-w-full"
            isDisabled={disabled}
            color="primary"
          />
        </div>

        {/* Custom Input Options */}
        <div className="grid grid-cols-2 gap-4">
          <div className="space-y-2">
            <p className="text-sm font-medium">Custom %</p>
            <Input
              type="number"
              size="sm"
              placeholder="0"
              value={customPercentage}
              onValueChange={handleCustomPercentageChange}
              endContent={<Percent size={16} className="text-default-400" />}
              min="0"
              max="100"
              step="0.1"
              disabled={disabled}
            />
          </div>
          <div className="space-y-2">
            <p className="text-sm font-medium">Custom Amount</p>
            <Input
              type="number"
              size="sm"
              placeholder="0.00"
              value={customAmount}
              onValueChange={handleCustomAmountChange}
              startContent={<span className="text-default-400">$</span>}
              min="0"
              step="0.01"
              disabled={disabled}
            />
          </div>
        </div>

        <Divider />

        {/* Tip Summary */}
        <div className="space-y-3">
          <div className="flex justify-between items-center">
            <span className="font-medium">Tip Amount:</span>
            <span className="text-lg font-bold text-primary">
              {formatCurrency(tipAmount)}
            </span>
          </div>
          
          <div className="flex justify-between items-center">
            <span className="font-medium">Total with Tip:</span>
            <span className="text-xl font-bold">
              {formatCurrency(totalWithTip)}
            </span>
          </div>

          {numPeople > 1 && (
            <>
              <Divider />
              <div className="space-y-2 bg-default-50 p-3 rounded-lg">
                <p className="text-sm font-medium text-center">Per Person ({numPeople} people)</p>
                <div className="flex justify-between text-sm">
                  <span>Tip per person:</span>
                  <span className="font-semibold">{formatCurrency(tipPerPerson)}</span>
                </div>
                <div className="flex justify-between">
                  <span className="font-medium">Total per person:</span>
                  <span className="text-lg font-bold text-primary">
                    {formatCurrency(perPersonAmount)}
                  </span>
                </div>
              </div>
            </>
          )}
        </div>

        {/* No Tip Option */}
        <div className="flex justify-center">
          <Button
            size="sm"
            variant="light"
            color="default"
            onPress={() => {
              setTipMethod('amount');
              setTipAmount(0);
              setCustomAmount('0');
              setCustomPercentage('');
            }}
            disabled={disabled}
          >
            No Tip
          </Button>
        </div>
      </CardBody>
    </Card>
  );
}
