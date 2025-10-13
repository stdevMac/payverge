'use client';

import React, { useState, useEffect } from 'react';
import {
  Card,
  CardBody,
  CardHeader,
  Button,
  Select,
  SelectItem,
  Chip,
  Spinner,
  Divider,
  Input
} from '@nextui-org/react';
import {
  Globe,
  DollarSign,
  Languages,
  Settings,
  Check,
  X,
  Plus,
  Trash2
} from 'lucide-react';
import {
  getSupportedCurrencies,
  getSupportedLanguages,
  getBusinessCurrencies,
  getBusinessLanguages,
  updateBusinessCurrencies,
  updateBusinessLanguages,
  SupportedCurrency,
  SupportedLanguage,
  BusinessCurrency,
  BusinessLanguage,
  formatCurrency,
  convertAmount
} from '../../api/currency';

interface CurrencyLanguageSettingsProps {
  businessId: number;
}

export default function CurrencyLanguageSettings({ businessId }: CurrencyLanguageSettingsProps) {
  // State for currencies
  const [supportedCurrencies, setSupportedCurrencies] = useState<SupportedCurrency[]>([]);
  const [businessCurrencies, setBusinessCurrencies] = useState<BusinessCurrency[]>([]);
  const [selectedCurrencies, setSelectedCurrencies] = useState<string[]>([]);
  const [preferredCurrency, setPreferredCurrency] = useState<string>('');

  // State for languages
  const [supportedLanguages, setSupportedLanguages] = useState<SupportedLanguage[]>([]);
  const [businessLanguages, setBusinessLanguages] = useState<BusinessLanguage[]>([]);
  const [selectedLanguages, setSelectedLanguages] = useState<string[]>([]);
  const [defaultLanguage, setDefaultLanguage] = useState<string>('');

  // UI state
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [activeTab, setActiveTab] = useState<'currencies' | 'languages'>('currencies');

  // Sample conversion amounts for preview
  const [sampleAmount] = useState(100); // $100 USDC equivalent
  const [conversionPreviews, setConversionPreviews] = useState<{ [key: string]: number }>({});

  useEffect(() => {
    loadData();
  }, [businessId]);

  const loadData = async () => {
    try {
      setLoading(true);
      setError(null);

      // Load supported currencies and languages
      const [currencies, languages, businessCurr, businessLang] = await Promise.all([
        getSupportedCurrencies(),
        getSupportedLanguages(),
        getBusinessCurrencies(businessId),
        getBusinessLanguages(businessId)
      ]);

      setSupportedCurrencies(currencies);
      setSupportedLanguages(languages);
      setBusinessCurrencies(businessCurr);
      setBusinessLanguages(businessLang);

      // Set current selections
      setSelectedCurrencies(businessCurr.map(c => c.currency_code));
      setPreferredCurrency(businessCurr.find(c => c.is_preferred)?.currency_code || '');
      setSelectedLanguages(businessLang.map(l => l.language_code));
      setDefaultLanguage(businessLang.find(l => l.is_default)?.language_code || '');

      // Load conversion previews for selected currencies
      await loadConversionPreviews(businessCurr.map(c => c.currency_code));

    } catch (err: any) {
      console.error('Error loading currency/language data:', err);
      setError('Failed to load settings');
    } finally {
      setLoading(false);
    }
  };

  const loadConversionPreviews = async (currencyCodes: string[]) => {
    const previews: { [key: string]: number } = {};
    
    for (const code of currencyCodes) {
      if (code !== 'USDC') {
        try {
          const result = await convertAmount(sampleAmount, 'USDC', code);
          previews[code] = result.converted_amount;
        } catch (error) {
          console.warn(`Failed to get conversion rate for ${code}`);
        }
      } else {
        previews[code] = sampleAmount;
      }
    }
    
    setConversionPreviews(previews);
  };

  const handleCurrencyToggle = (currencyCode: string) => {
    const newSelected = selectedCurrencies.includes(currencyCode)
      ? selectedCurrencies.filter(c => c !== currencyCode)
      : [...selectedCurrencies, currencyCode];
    
    setSelectedCurrencies(newSelected);

    // If removing the preferred currency, reset it
    if (!newSelected.includes(preferredCurrency)) {
      setPreferredCurrency(newSelected[0] || '');
    }

    // Update conversion previews
    loadConversionPreviews(newSelected);
  };

  const handleLanguageToggle = (languageCode: string) => {
    const newSelected = selectedLanguages.includes(languageCode)
      ? selectedLanguages.filter(l => l !== languageCode)
      : [...selectedLanguages, languageCode];
    
    setSelectedLanguages(newSelected);

    // If removing the default language, reset it
    if (!newSelected.includes(defaultLanguage)) {
      setDefaultLanguage(newSelected[0] || '');
    }
  };

  const saveCurrencySettings = async () => {
    if (selectedCurrencies.length === 0) {
      setError('Please select at least one currency');
      return;
    }

    if (!preferredCurrency || !selectedCurrencies.includes(preferredCurrency)) {
      setError('Please select a preferred currency from your selected currencies');
      return;
    }

    try {
      setSaving(true);
      setError(null);

      await updateBusinessCurrencies(businessId, {
        currency_codes: selectedCurrencies,
        preferred_code: preferredCurrency
      });

      // Reload data to reflect changes
      await loadData();
      
    } catch (err: any) {
      console.error('Error saving currency settings:', err);
      setError('Failed to save currency settings');
    } finally {
      setSaving(false);
    }
  };

  const saveLanguageSettings = async () => {
    if (selectedLanguages.length === 0) {
      setError('Please select at least one language');
      return;
    }

    if (!defaultLanguage || !selectedLanguages.includes(defaultLanguage)) {
      setError('Please select a default language from your selected languages');
      return;
    }

    try {
      setSaving(true);
      setError(null);

      await updateBusinessLanguages(businessId, {
        language_codes: selectedLanguages,
        default_code: defaultLanguage
      });

      // Reload data to reflect changes
      await loadData();
      
    } catch (err: any) {
      console.error('Error saving language settings:', err);
      setError('Failed to save language settings');
    } finally {
      setSaving(false);
    }
  };

  if (loading) {
    return (
      <Card className="w-full">
        <CardBody className="flex items-center justify-center py-8">
          <Spinner size="lg" />
          <p className="mt-4 text-gray-600">Loading currency and language settings...</p>
        </CardBody>
      </Card>
    );
  }

  return (
    <div className="space-y-6">
      {/* Tab Navigation */}
      <div className="flex space-x-1 bg-gray-100 p-1 rounded-lg">
        <button
          onClick={() => setActiveTab('currencies')}
          className={`flex-1 flex items-center justify-center gap-2 px-4 py-2 rounded-md transition-colors ${
            activeTab === 'currencies'
              ? 'bg-white text-gray-900 shadow-sm'
              : 'text-gray-600 hover:text-gray-900'
          }`}
        >
          <DollarSign className="w-4 h-4" />
          Currencies
        </button>
        <button
          onClick={() => setActiveTab('languages')}
          className={`flex-1 flex items-center justify-center gap-2 px-4 py-2 rounded-md transition-colors ${
            activeTab === 'languages'
              ? 'bg-white text-gray-900 shadow-sm'
              : 'text-gray-600 hover:text-gray-900'
          }`}
        >
          <Languages className="w-4 h-4" />
          Languages
        </button>
      </div>

      {error && (
        <Card className="border-red-200 bg-red-50">
          <CardBody className="py-4">
            <div className="flex items-center gap-2 text-red-600">
              <X className="w-4 h-4" />
              <span>{error}</span>
            </div>
          </CardBody>
        </Card>
      )}

      {/* Currency Settings */}
      {activeTab === 'currencies' && (
        <Card>
          <CardHeader className="flex gap-3">
            <div className="flex items-center justify-center w-12 h-12 rounded-xl bg-green-50">
              <DollarSign className="w-6 h-6 text-green-600" />
            </div>
            <div>
              <h4 className="text-lg font-medium text-gray-900">Currency Settings</h4>
              <p className="text-sm text-gray-600">
                Choose which currencies your customers can view prices in. All payments are processed in USDC.
              </p>
            </div>
          </CardHeader>
          <CardBody className="space-y-6">
            {/* Currency Selection */}
            <div>
              <h5 className="text-md font-medium text-gray-900 mb-3">Supported Currencies</h5>
              <div className="grid grid-cols-2 md:grid-cols-3 lg:grid-cols-4 gap-3">
                {supportedCurrencies.map((currency) => (
                  <div
                    key={currency.code}
                    onClick={() => handleCurrencyToggle(currency.code)}
                    className={`p-4 border-2 rounded-lg cursor-pointer transition-all ${
                      selectedCurrencies.includes(currency.code)
                        ? 'border-blue-500 bg-blue-50'
                        : 'border-gray-200 hover:border-gray-300'
                    }`}
                  >
                    <div className="flex items-center justify-between">
                      <div>
                        <div className="font-medium text-gray-900">{currency.code}</div>
                        <div className="text-sm text-gray-600">{currency.symbol}</div>
                      </div>
                      {selectedCurrencies.includes(currency.code) && (
                        <Check className="w-5 h-5 text-blue-500" />
                      )}
                    </div>
                    <div className="text-xs text-gray-500 mt-1">{currency.name}</div>
                    {conversionPreviews[currency.code] && (
                      <div className="text-xs text-green-600 mt-1">
                        ${sampleAmount} USDC ≈ {formatCurrency(conversionPreviews[currency.code], currency.code)}
                      </div>
                    )}
                  </div>
                ))}
              </div>
            </div>

            {/* Preferred Currency Selection */}
            {selectedCurrencies.length > 0 && (
              <div>
                <h5 className="text-md font-medium text-gray-900 mb-3">Preferred Currency</h5>
                <Select
                  placeholder="Select preferred currency"
                  selectedKeys={preferredCurrency ? [preferredCurrency] : []}
                  onSelectionChange={(keys) => {
                    const selected = Array.from(keys)[0] as string;
                    setPreferredCurrency(selected);
                  }}
                  className="max-w-xs"
                >
                  {selectedCurrencies.map((code) => {
                    const currency = supportedCurrencies.find(c => c.code === code);
                    return (
                      <SelectItem key={code} value={code}>
                        {currency?.symbol} {code} - {currency?.name}
                      </SelectItem>
                    );
                  })}
                </Select>
                <p className="text-sm text-gray-600 mt-2">
                  This will be the default currency shown to customers
                </p>
              </div>
            )}

            {/* Selected Currencies Preview */}
            {selectedCurrencies.length > 0 && (
              <div>
                <h5 className="text-md font-medium text-gray-900 mb-3">Selected Currencies</h5>
                <div className="flex flex-wrap gap-2">
                  {selectedCurrencies.map((code) => {
                    const currency = supportedCurrencies.find(c => c.code === code);
                    return (
                      <Chip
                        key={code}
                        color={code === preferredCurrency ? 'primary' : 'default'}
                        variant={code === preferredCurrency ? 'solid' : 'flat'}
                        onClose={() => handleCurrencyToggle(code)}
                      >
                        {currency?.symbol} {code}
                      </Chip>
                    );
                  })}
                </div>
              </div>
            )}

            <Divider />

            <div className="flex justify-end">
              <Button
                color="primary"
                onPress={saveCurrencySettings}
                isLoading={saving}
                isDisabled={selectedCurrencies.length === 0 || !preferredCurrency}
              >
                Save Currency Settings
              </Button>
            </div>
          </CardBody>
        </Card>
      )}

      {/* Language Settings */}
      {activeTab === 'languages' && (
        <Card>
          <CardHeader className="flex gap-3">
            <div className="flex items-center justify-center w-12 h-12 rounded-xl bg-purple-50">
              <Languages className="w-6 h-6 text-purple-600" />
            </div>
            <div>
              <h4 className="text-lg font-medium text-gray-900">Language Settings</h4>
              <p className="text-sm text-gray-600">
                Choose which languages your menu and business information will be available in.
              </p>
            </div>
          </CardHeader>
          <CardBody className="space-y-6">
            {/* Language Selection */}
            <div>
              <h5 className="text-md font-medium text-gray-900 mb-3">Supported Languages</h5>
              <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-3">
                {supportedLanguages.map((language) => (
                  <div
                    key={language.code}
                    onClick={() => handleLanguageToggle(language.code)}
                    className={`p-4 border-2 rounded-lg cursor-pointer transition-all ${
                      selectedLanguages.includes(language.code)
                        ? 'border-purple-500 bg-purple-50'
                        : 'border-gray-200 hover:border-gray-300'
                    }`}
                  >
                    <div className="flex items-center justify-between">
                      <div>
                        <div className="font-medium text-gray-900">{language.name}</div>
                        <div className="text-sm text-gray-600">{language.native_name}</div>
                      </div>
                      {selectedLanguages.includes(language.code) && (
                        <Check className="w-5 h-5 text-purple-500" />
                      )}
                    </div>
                    <div className="text-xs text-gray-500 mt-1">{language.code}</div>
                  </div>
                ))}
              </div>
            </div>

            {/* Default Language Selection */}
            {selectedLanguages.length > 0 && (
              <div>
                <h5 className="text-md font-medium text-gray-900 mb-3">Default Language</h5>
                <Select
                  placeholder="Select default language"
                  selectedKeys={defaultLanguage ? [defaultLanguage] : []}
                  onSelectionChange={(keys) => {
                    const selected = Array.from(keys)[0] as string;
                    setDefaultLanguage(selected);
                  }}
                  className="max-w-xs"
                >
                  {selectedLanguages.map((code) => {
                    const language = supportedLanguages.find(l => l.code === code);
                    return (
                      <SelectItem key={code} value={code}>
                        {language?.name} ({language?.native_name})
                      </SelectItem>
                    );
                  })}
                </Select>
                <p className="text-sm text-gray-600 mt-2">
                  This will be the default language for your menu and business information
                </p>
              </div>
            )}

            {/* Selected Languages Preview */}
            {selectedLanguages.length > 0 && (
              <div>
                <h5 className="text-md font-medium text-gray-900 mb-3">Selected Languages</h5>
                <div className="flex flex-wrap gap-2">
                  {selectedLanguages.map((code) => {
                    const language = supportedLanguages.find(l => l.code === code);
                    return (
                      <Chip
                        key={code}
                        color={code === defaultLanguage ? 'secondary' : 'default'}
                        variant={code === defaultLanguage ? 'solid' : 'flat'}
                        onClose={() => handleLanguageToggle(code)}
                      >
                        {language?.name}
                      </Chip>
                    );
                  })}
                </div>
              </div>
            )}

            <Divider />

            <div className="flex justify-end">
              <Button
                color="secondary"
                onPress={saveLanguageSettings}
                isLoading={saving}
                isDisabled={selectedLanguages.length === 0 || !defaultLanguage}
              >
                Save Language Settings
              </Button>
            </div>
          </CardBody>
        </Card>
      )}
    </div>
  );
}
