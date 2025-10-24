'use client';

import React, { useState, useEffect, useCallback } from 'react';
import {
  Select,
  SelectItem,
  Chip,
  Spinner,
  Tooltip
} from '@nextui-org/react';
import { Globe, TrendingUp, TrendingDown } from 'lucide-react';
import {
  getBusinessCurrencies,
  convertAmount,
  formatCurrency,
  getCurrencySymbol,
  BusinessCurrency,
  SupportedCurrency
} from '../../api/currency';

interface CurrencyDisplayProps {
  // Core props
  amount: number; // Amount in USDC
  businessId?: number; // If provided, will load business currencies
  
  // Display options
  showSelector?: boolean; // Show currency selector dropdown
  showOriginal?: boolean; // Show original USDC amount
  className?: string;
  
  // Currency options (if not loading from business)
  availableCurrencies?: SupportedCurrency[];
  defaultCurrency?: string;
  
  // Callbacks
  onCurrencyChange?: (currency: string) => void;
}

interface CurrencyRate {
  code: string;
  rate: number;
  convertedAmount: number;
  lastUpdated: Date;
}

export default function CurrencyDisplay({
  amount,
  businessId,
  showSelector = false,
  showOriginal = false,
  className = '',
  availableCurrencies = [],
  defaultCurrency = 'USD',
  onCurrencyChange
}: CurrencyDisplayProps) {
  const [businessCurrencies, setBusinessCurrencies] = useState<BusinessCurrency[]>([]);
  const [currencyRates, setCurrencyRates] = useState<CurrencyRate[]>([]);
  const [selectedCurrency, setSelectedCurrency] = useState<string>(defaultCurrency);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const loadCurrencyRates = useCallback(async (currencyCodes: string[]) => {
    const rates: CurrencyRate[] = [];

    for (const code of currencyCodes) {
      try {
        if (code === 'USDC') {
          rates.push({
            code,
            rate: 1,
            convertedAmount: amount,
            lastUpdated: new Date()
          });
        } else {
          const result = await convertAmount(amount, 'USDC', code);
          rates.push({
            code,
            rate: result.converted_amount / amount,
            convertedAmount: result.converted_amount,
            lastUpdated: new Date()
          });
        }
      } catch (error) {
        console.warn(`Failed to get rate for ${code}:`, error);
      }
    }

    setCurrencyRates(rates);
  }, [amount]);

  const loadBusinessCurrencies = useCallback(async () => {
    if (!businessId) return;

    try {
      setLoading(true);
      setError(null);

      const currencies = await getBusinessCurrencies(businessId);
      setBusinessCurrencies(currencies);

      // Load rates for business currencies
      const currencyCodes = currencies.map(c => c.currency_code);
      await loadCurrencyRates(currencyCodes);

      // Set preferred currency as default
      const preferred = currencies.find(c => c.is_preferred);
      if (preferred) {
        setSelectedCurrency(preferred.currency_code);
      }

    } catch (err: any) {
      console.error('Error loading business currencies:', err);
      setError('Failed to load currencies');
    } finally {
      setLoading(false);
    }
  }, [businessId, loadCurrencyRates]);

  useEffect(() => {
    if (businessId) {
      loadBusinessCurrencies();
    } else if (availableCurrencies.length > 0) {
      loadCurrencyRates(availableCurrencies.map(c => c.code));
    }
  }, [businessId, availableCurrencies, loadBusinessCurrencies, loadCurrencyRates]);

  useEffect(() => {
    if (currencyRates.length > 0 && !currencyRates.find(r => r.code === selectedCurrency)) {
      // If selected currency is not available, select the first one or preferred one
      const preferredCurrency = businessCurrencies.find(c => c.is_preferred)?.currency_code;
      setSelectedCurrency(preferredCurrency || currencyRates[0]?.code || defaultCurrency);
    }
  }, [currencyRates, businessCurrencies, selectedCurrency, defaultCurrency]);

  const handleCurrencyChange = (currency: string) => {
    setSelectedCurrency(currency);
    onCurrencyChange?.(currency);
  };

  const getCurrentRate = (): CurrencyRate | null => {
    return currencyRates.find(r => r.code === selectedCurrency) || null;
  };

  const formatDisplayAmount = (rate: CurrencyRate): string => {
    return formatCurrency(rate.convertedAmount, rate.code);
  };

  if (loading) {
    return (
      <div className={`flex items-center gap-2 ${className}`}>
        <Spinner size="sm" />
        <span className="text-sm text-gray-500">Loading rates...</span>
      </div>
    );
  }

  if (error) {
    return (
      <div className={`text-red-500 text-sm ${className}`}>
        {formatCurrency(amount, 'USDC')}
      </div>
    );
  }

  const currentRate = getCurrentRate();
  if (!currentRate) {
    return (
      <div className={`${className}`}>
        {formatCurrency(amount, 'USDC')}
      </div>
    );
  }

  return (
    <div className={`flex items-center gap-2 ${className}`}>
      {/* Main amount display */}
      <div className="flex items-center gap-1">
        <span className="font-medium">
          {formatDisplayAmount(currentRate)}
        </span>
        
        {/* Rate change indicator (if we had historical data) */}
        {false && ( // Placeholder for rate change feature
          <Tooltip content="Rate change from yesterday">
            <div className="flex items-center">
              <TrendingUp className="w-3 h-3 text-green-500" />
              <span className="text-xs text-green-500">+2.1%</span>
            </div>
          </Tooltip>
        )}
      </div>

      {/* Currency selector */}
      {showSelector && currencyRates.length > 1 && (
        <Select
          size="sm"
          selectedKeys={[selectedCurrency]}
          onSelectionChange={(keys) => {
            const selected = Array.from(keys)[0] as string;
            handleCurrencyChange(selected);
          }}
          className="min-w-[100px]"
          startContent={<Globe className="w-3 h-3" />}
        >
          {currencyRates.map((rate) => (
            <SelectItem key={rate.code} value={rate.code}>
              {rate.code}
            </SelectItem>
          ))}
        </Select>
      )}

      {/* Original USDC amount */}
      {showOriginal && selectedCurrency !== 'USDC' && (
        <Chip size="sm" variant="flat" color="default">
          {formatCurrency(amount, 'USDC')}
        </Chip>
      )}
    </div>
  );
}

// Simplified version for just displaying converted amounts
interface SimpleCurrencyDisplayProps {
  amount: number;
  fromCurrency?: string;
  toCurrency: string;
  className?: string;
}

export function SimpleCurrencyDisplay({
  amount,
  fromCurrency = 'USDC',
  toCurrency,
  className = ''
}: SimpleCurrencyDisplayProps) {
  const [convertedAmount, setConvertedAmount] = useState<number | null>(null);
  const [loading, setLoading] = useState(false);

  useEffect(() => {
    if (fromCurrency === toCurrency) {
      setConvertedAmount(amount);
      return;
    }

    const loadConversion = async () => {
      try {
        setLoading(true);
        const result = await convertAmount(amount, fromCurrency, toCurrency);
        setConvertedAmount(result.converted_amount);
      } catch (error) {
        console.error('Conversion failed:', error);
        setConvertedAmount(null);
      } finally {
        setLoading(false);
      }
    };

    loadConversion();
  }, [amount, fromCurrency, toCurrency]);

  if (loading) {
    return <Spinner size="sm" className={className} />;
  }

  if (convertedAmount === null) {
    return (
      <span className={`text-gray-500 ${className}`}>
        {formatCurrency(amount, fromCurrency)}
      </span>
    );
  }

  return (
    <span className={className}>
      {formatCurrency(convertedAmount, toCurrency)}
    </span>
  );
}

// Hook for currency conversion
export function useCurrencyConverter() {
  const [loading, setLoading] = useState(false);

  const convert = async (
    amount: number,
    fromCurrency: string,
    toCurrency: string
  ): Promise<number | null> => {
    if (fromCurrency === toCurrency) {
      return amount;
    }

    try {
      setLoading(true);
      const result = await convertAmount(amount, fromCurrency, toCurrency);
      return result.converted_amount;
    } catch (error) {
      console.error('Currency conversion failed:', error);
      return null;
    } finally {
      setLoading(false);
    }
  };

  return { convert, loading };
}
