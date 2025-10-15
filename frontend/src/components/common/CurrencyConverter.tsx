'use client';

import React, { useState, useEffect } from 'react';
import { Chip } from '@nextui-org/react';
import { convertAmount, formatCurrency } from '../../api/currency';

interface CurrencyConverterProps {
  amount: number;
  fromCurrency: string; // Business default currency (pricing)
  displayCurrency: string; // What customer sees
  showUSDCConversion?: boolean; // Show USDC equivalent
  className?: string;
}

interface ConversionRates {
  displayAmount: number;
  usdcAmount: number;
}

export default function CurrencyConverter({ 
  amount, 
  fromCurrency, 
  displayCurrency, 
  showUSDCConversion = true,
  className = ""
}: CurrencyConverterProps) {
  const [rates, setRates] = useState<ConversionRates>({
    displayAmount: amount,
    usdcAmount: amount
  });
  const [loading, setLoading] = useState(false);

  useEffect(() => {
    const loadConversions = async () => {
      if (amount <= 0) return;
      
      try {
        setLoading(true);
        
        let displayAmount = amount;
        let usdcAmount = amount;

        // Convert from business default currency to display currency
        if (fromCurrency !== displayCurrency) {
          const displayResult = await convertAmount(amount, fromCurrency, displayCurrency);
          displayAmount = displayResult.converted_amount;
        }

        // Convert from display currency to USDC (for payment)
        if (displayCurrency !== 'USDC') {
          const usdcResult = await convertAmount(displayAmount, displayCurrency, 'USDC');
          usdcAmount = usdcResult.converted_amount;
        }

        setRates({ displayAmount, usdcAmount });
      } catch (error) {
        console.error('Currency conversion failed:', error);
        // Fallback to original amounts
        setRates({ displayAmount: amount, usdcAmount: amount });
      } finally {
        setLoading(false);
      }
    };

    loadConversions();
  }, [amount, fromCurrency, displayCurrency]);

  if (loading) {
    return (
      <div className={`animate-pulse ${className}`}>
        <div className="h-6 bg-gray-200 rounded w-20"></div>
      </div>
    );
  }

  return (
    <div className={`space-y-1 ${className}`}>
      {/* Primary display price */}
      <div className="font-semibold text-lg">
        {formatCurrency(rates.displayAmount, displayCurrency)}
      </div>
      
      {/* USDC conversion (if different and requested) */}
      {showUSDCConversion && displayCurrency !== 'USDC' && (
        <div className="flex items-center gap-2">
          <Chip size="sm" variant="flat" color="success">
            ≈ ${rates.usdcAmount.toFixed(2)} USDC
          </Chip>
          <span className="text-xs text-gray-500">Payment amount</span>
        </div>
      )}
    </div>
  );
}

// Simplified version for just displaying converted price
export function CurrencyPrice({ 
  amount, 
  fromCurrency, 
  displayCurrency, 
  className = "" 
}: Omit<CurrencyConverterProps, 'showUSDCConversion'>) {
  const [displayAmount, setDisplayAmount] = useState(amount);
  const [loading, setLoading] = useState(false);

  useEffect(() => {
    const convertPrice = async () => {
      if (amount <= 0 || fromCurrency === displayCurrency) {
        setDisplayAmount(amount);
        return;
      }
      
      try {
        setLoading(true);
        const result = await convertAmount(amount, fromCurrency, displayCurrency);
        setDisplayAmount(result.converted_amount);
      } catch (error) {
        console.error('Price conversion failed:', error);
        setDisplayAmount(amount);
      } finally {
        setLoading(false);
      }
    };

    convertPrice();
  }, [amount, fromCurrency, displayCurrency]);

  if (loading) {
    return <div className={`animate-pulse h-6 bg-gray-200 rounded w-16 ${className}`}></div>;
  }

  return (
    <span className={className}>
      {formatCurrency(displayAmount, displayCurrency)}
    </span>
  );
}
