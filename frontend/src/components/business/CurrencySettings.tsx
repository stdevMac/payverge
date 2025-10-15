'use client';

import React, { useState, useEffect } from 'react';
import {
  Card,
  CardBody,
  CardHeader,
  Button,
  Select,
  SelectItem,
  Spinner,
} from '@nextui-org/react';
import {
  DollarSign,
  Check,
  X,
  Info,
  Languages
} from 'lucide-react';
import {
  getSupportedCurrencies,
  getSupportedLanguages,
  getBusinessLanguages,
  updateBusinessLanguages,
  SupportedCurrency,
  SupportedLanguage,
  BusinessLanguage,
  formatCurrency,
  convertAmount
} from '../../api/currency';
import { businessApi } from '../../api/business';

interface CurrencySettingsProps {
  businessId: number;
  onSave?: () => void;
}

interface BusinessCurrencySettings {
  default_currency: string;
  display_currency: string;
}

export default function CurrencySettings({ businessId, onSave }: CurrencySettingsProps) {
  // Currency State
  const [supportedCurrencies, setSupportedCurrencies] = useState<SupportedCurrency[]>([]);
  const [settings, setSettings] = useState<BusinessCurrencySettings>({
    default_currency: 'USD',
    display_currency: 'USD'
  });
  const [originalSettings, setOriginalSettings] = useState<BusinessCurrencySettings>({
    default_currency: 'USD',
    display_currency: 'USD'
  });

  // Language State
  const [supportedLanguages, setSupportedLanguages] = useState<SupportedLanguage[]>([]);
  const [businessLanguages, setBusinessLanguages] = useState<BusinessLanguage[]>([]);
  const [selectedLanguages, setSelectedLanguages] = useState<string[]>([]);
  const [defaultLanguage, setDefaultLanguage] = useState<string>('en');

  // UI state
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<string | null>(null);

  // Sample conversion for preview
  const [sampleAmount] = useState(100);
  const [conversionPreview, setConversionPreview] = useState<{
    defaultToUSDC: number;
    displayToUSDC: number;
    defaultToDisplay: number;
  }>({ defaultToUSDC: 100, displayToUSDC: 100, defaultToDisplay: 100 });

  useEffect(() => {
    loadData();
  }, [businessId]);

  useEffect(() => {
    updateConversionPreview();
  }, [settings.default_currency, settings.display_currency]);

  const loadData = async () => {
    try {
      setLoading(true);
      setError(null);

      // Load supported currencies, languages, and business settings
      const [currencies, languages, business, businessLang] = await Promise.all([
        getSupportedCurrencies(),
        getSupportedLanguages(),
        businessApi.getBusiness(businessId),
        getBusinessLanguages(businessId)
      ]);

      setSupportedCurrencies(currencies);
      setSupportedLanguages(languages);
      setBusinessLanguages(businessLang);
      
      if (business) {
        const businessSettings = {
          default_currency: business.default_currency || 'USD',
          display_currency: business.display_currency || 'USD'
        };
        setSettings(businessSettings);
        setOriginalSettings(businessSettings);
      }

      // Set language selections - default to English if none selected
      const langCodes = businessLang.map(l => l.language_code);
      setSelectedLanguages(langCodes.length > 0 ? langCodes : ['en']);
      setDefaultLanguage(businessLang.find(l => l.is_default)?.language_code || 'en');

    } catch (err: any) {
      console.error('Error loading settings:', err);
      setError('Failed to load settings');
    } finally {
      setLoading(false);
    }
  };

  const updateConversionPreview = async () => {
    try {
      const previews = { defaultToUSDC: sampleAmount, displayToUSDC: sampleAmount, defaultToDisplay: sampleAmount };

      // Convert from default currency to USDC
      if (settings.default_currency !== 'USDC') {
        try {
          const result = await convertAmount(sampleAmount, settings.default_currency, 'USDC');
          previews.defaultToUSDC = result.converted_amount;
        } catch (error) {
          console.warn(`Failed to convert ${settings.default_currency} to USDC:`, error);
        }
      } else {
        previews.defaultToUSDC = sampleAmount;
      }

      // Convert from display currency to USDC
      if (settings.display_currency !== 'USDC') {
        try {
          const result = await convertAmount(sampleAmount, settings.display_currency, 'USDC');
          previews.displayToUSDC = result.converted_amount;
        } catch (error) {
          console.warn(`Failed to convert ${settings.display_currency} to USDC:`, error);
        }
      } else {
        previews.displayToUSDC = sampleAmount;
      }

      // Convert from default to display currency
      if (settings.default_currency !== settings.display_currency) {
        try {
          const result = await convertAmount(sampleAmount, settings.default_currency, settings.display_currency);
          previews.defaultToDisplay = result.converted_amount;
        } catch (error) {
          console.warn(`Failed to convert ${settings.default_currency} to ${settings.display_currency}:`, error);
        }
      } else {
        previews.defaultToDisplay = sampleAmount;
      }

      setConversionPreview(previews);
    } catch (error) {
      console.warn('Failed to update conversion preview:', error);
    }
  };

  const handleSaveCurrency = async () => {
    try {
      setSaving(true);
      setError(null);

      // Save currency settings
      await businessApi.updateBusiness(businessId, {
        default_currency: settings.default_currency,
        display_currency: settings.display_currency
      });

      setOriginalSettings(settings);
      onSave?.();
      
    } catch (err: any) {
      console.error('Error saving currency settings:', err);
      setError('Failed to save currency settings');
    } finally {
      setSaving(false);
    }
  };

  const handleSaveLanguage = async () => {
    try {
      setSaving(true);
      setError(null);

      // Validate language settings
      if (selectedLanguages.length === 0) {
        setError('Please select at least one language');
        setSaving(false);
        return;
      }

      if (!defaultLanguage || !selectedLanguages.includes(defaultLanguage)) {
        setError('Please select a default language from your selected languages');
        setSaving(false);
        return;
      }

      // Save language settings
      await updateBusinessLanguages(businessId, {
        language_codes: selectedLanguages,
        default_code: defaultLanguage
      });

      // Reload data to reflect changes
      await loadData();
      onSave?.();
      
    } catch (err: any) {
      console.error('Error saving language settings:', err);
      setError('Failed to save language settings');
    } finally {
      setSaving(false);
    }
  };

  const handleLanguageToggle = (languageCode: string) => {
    const newSelected = selectedLanguages.includes(languageCode)
      ? selectedLanguages.filter(l => l !== languageCode)
      : [...selectedLanguages, languageCode];
    
    setSelectedLanguages(newSelected);

    // If removing the default language, reset it
    if (!newSelected.includes(defaultLanguage)) {
      setDefaultLanguage(newSelected[0] || 'en');
    }
  };

  const hasCurrencyChanges = () => {
    return settings.default_currency !== originalSettings.default_currency ||
           settings.display_currency !== originalSettings.display_currency;
  };

  const hasLanguageChanges = () => {
    const originalLanguageCodes = businessLanguages.map(l => l.language_code).sort();
    const currentLanguageCodes = selectedLanguages.slice().sort();
    const originalDefaultLang = businessLanguages.find(l => l.is_default)?.language_code || 'en';
    
    return JSON.stringify(originalLanguageCodes) !== JSON.stringify(currentLanguageCodes) ||
           originalDefaultLang !== defaultLanguage;
  };

  const getCurrencyInfo = (code: string) => {
    return supportedCurrencies.find(c => c.code === code);
  };

  if (loading) {
    return (
      <Card className="w-full">
        <CardBody className="flex items-center justify-center py-8">
          <Spinner size="lg" />
          <p className="mt-4 text-gray-600">Loading settings...</p>
        </CardBody>
      </Card>
    );
  }

  return (
    <div className="space-y-6" style={{ minHeight: '100vh' }}>
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

      {/* CURRENCY SETTINGS CARD */}
      <Card>
        <CardHeader className="pb-4">
          <div className="flex items-center gap-3">
            <div className="flex items-center justify-center w-10 h-10 rounded-lg bg-gray-100">
              <DollarSign className="w-5 h-5 text-gray-700" />
            </div>
            <div>
              <h3 className="text-lg font-semibold text-gray-900">Currency Settings</h3>
              <p className="text-sm text-gray-600">
                Configure how currencies work in your business. All payments are processed in USDC.
              </p>
            </div>
          </div>
        </CardHeader>
        <CardBody className="space-y-6">
          {/* Default Currency */}
          <div className="space-y-3">
            <div>
              <h4 className="text-md font-medium text-gray-900">Default Currency (Pricing)</h4>
              <p className="text-sm text-gray-600">
                The currency you use to set prices for your menu items.
              </p>
            </div>
            <Select
              label="Default Currency"
              placeholder="Select default currency"
              selectedKeys={settings.default_currency ? [settings.default_currency] : undefined}
              onSelectionChange={(keys) => {
                const selected = Array.from(keys)[0] as string;
                if (selected) {
                  setSettings(prev => ({ ...prev, default_currency: selected }));
                }
              }}
              className="max-w-xs"
              variant="bordered"
              disallowEmptySelection
            >
              {supportedCurrencies.map((currency) => (
                <SelectItem 
                  key={currency.code} 
                  value={currency.code}
                  textValue={`${currency.code} - ${currency.name}`}
                >
                  {currency.symbol} {currency.code} - {currency.name}
                </SelectItem>
              ))}
            </Select>
          </div>

          {/* Display Currency */}
          <div className="space-y-3">
            <div>
              <h4 className="text-md font-medium text-gray-900">Display Currency (Customer View)</h4>
              <p className="text-sm text-gray-600">
                The currency your customers see on menus and bills.
              </p>
            </div>
            <Select
              label="Display Currency"
              placeholder="Select display currency"
              selectedKeys={settings.display_currency ? [settings.display_currency] : undefined}
              onSelectionChange={(keys) => {
                const selected = Array.from(keys)[0] as string;
                if (selected) {
                  setSettings(prev => ({ ...prev, display_currency: selected }));
                }
              }}
              className="max-w-xs"
              variant="bordered"
              disallowEmptySelection
            >
              {supportedCurrencies.map((currency) => (
                <SelectItem 
                  key={currency.code} 
                  value={currency.code}
                  textValue={`${currency.code} - ${currency.name}`}
                >
                  {currency.symbol} {currency.code} - {currency.name}
                </SelectItem>
              ))}
            </Select>
          </div>

          {/* Conversion Preview */}
          {(settings.default_currency !== 'USDC' || settings.display_currency !== 'USDC') && (
            <div className="space-y-4 pt-4 border-t border-gray-200">
              <h4 className="text-md font-medium text-gray-900">Conversion Preview</h4>
              <div className="grid grid-cols-1 md:grid-cols-3 gap-4">
                
                {/* Default to USDC */}
                <div className="bg-gray-50 rounded-lg p-4">
                  <div className="text-sm text-gray-600 mb-1">Pricing → Payment</div>
                  <div className="font-medium">
                    {formatCurrency(sampleAmount, settings.default_currency)} → {formatCurrency(conversionPreview.defaultToUSDC, 'USDC')}
                  </div>
                </div>

                {/* Default to Display */}
                {settings.default_currency !== settings.display_currency && (
                  <div className="bg-gray-50 rounded-lg p-4">
                    <div className="text-sm text-gray-600 mb-1">Pricing → Display</div>
                    <div className="font-medium">
                      {formatCurrency(sampleAmount, settings.default_currency)} → {formatCurrency(conversionPreview.defaultToDisplay, settings.display_currency)}
                    </div>
                  </div>
                )}

                {/* Display to USDC */}
                {settings.display_currency !== 'USDC' && (
                  <div className="bg-gray-50 rounded-lg p-4">
                    <div className="text-sm text-gray-600 mb-1">Display → Payment</div>
                    <div className="font-medium">
                      {formatCurrency(sampleAmount, settings.display_currency)} → {formatCurrency(conversionPreview.displayToUSDC, 'USDC')}
                    </div>
                  </div>
                )}
              </div>
            </div>
          )}

          {/* Info Box */}
          <div className="bg-gray-50 border border-gray-200 rounded-lg p-4">
            <div className="flex items-start gap-2">
              <Info className="w-4 h-4 text-gray-600 mt-0.5" />
              <div className="text-sm text-gray-700">
                <p className="font-medium">Payment Flow:</p>
                <p>Customers see prices in {getCurrencyInfo(settings.display_currency)?.name || settings.display_currency} → Real-time conversion shown → Payment made in USDC</p>
              </div>
            </div>
          </div>

          {/* Save Button */}
          <div className="flex justify-end pt-4 border-t border-gray-200">
            <Button
              color="primary"
              onPress={handleSaveCurrency}
              isLoading={saving}
              isDisabled={!hasCurrencyChanges()}
            >
              Save Currency Settings
            </Button>
          </div>
        </CardBody>
      </Card>

      {/* LANGUAGE SETTINGS CARD */}
      <Card>
        <CardHeader className="pb-4">
          <div className="flex items-center gap-3">
            <div className="flex items-center justify-center w-10 h-10 rounded-lg bg-gray-100">
              <Languages className="w-5 h-5 text-gray-700" />
            </div>
            <div>
              <h3 className="text-lg font-semibold text-gray-900">Language Settings</h3>
              <p className="text-sm text-gray-600">
                Configure languages for your menu and customer interface.
              </p>
            </div>
          </div>
        </CardHeader>
        <CardBody className="space-y-6">
          {/* Language Selection */}
          <div className="space-y-4">
            <div>
              <h4 className="text-md font-medium text-gray-900">Supported Languages</h4>
              <p className="text-sm text-gray-600">
                Select languages for your menu and customer interface.
              </p>
            </div>
            
            <div className="grid grid-cols-2 md:grid-cols-3 lg:grid-cols-4 gap-3">
              {supportedLanguages.map((language) => (
                <div
                  key={language.code}
                  onClick={() => handleLanguageToggle(language.code)}
                  className={`p-3 rounded-lg border cursor-pointer transition-colors ${
                    selectedLanguages.includes(language.code)
                      ? 'border-gray-900 bg-gray-50'
                      : 'border-gray-200 hover:border-gray-300'
                  }`}
                >
                  <div className="flex items-center justify-between">
                    <div>
                      <p className="font-medium text-sm">{language.native_name}</p>
                      <p className="text-xs text-gray-500">{language.name}</p>
                    </div>
                    {selectedLanguages.includes(language.code) && (
                      <Check className="w-4 h-4 text-gray-900" />
                    )}
                  </div>
                </div>
              ))}
            </div>
          </div>

          {/* Default Language Selection */}
          {selectedLanguages.length > 1 && (
            <div className="space-y-3">
              <div>
                <h4 className="text-md font-medium text-gray-900">Default Language</h4>
                <p className="text-sm text-gray-600">
                  Choose the primary language for your business.
                </p>
              </div>
              <Select
                label="Default Language"
                placeholder="Select default language"
                selectedKeys={defaultLanguage ? [defaultLanguage] : undefined}
                onSelectionChange={(keys) => {
                  const selected = Array.from(keys)[0] as string;
                  if (selected) {
                    setDefaultLanguage(selected);
                  }
                }}
                className="max-w-xs"
                variant="bordered"
                disallowEmptySelection
              >
                {selectedLanguages.map((langCode) => {
                  const language = supportedLanguages.find(l => l.code === langCode);
                  return language ? (
                    <SelectItem 
                      key={language.code} 
                      value={language.code}
                      textValue={`${language.native_name} (${language.name})`}
                    >
                      {language.native_name} ({language.name})
                    </SelectItem>
                  ) : null;
                })}
              </Select>
            </div>
          )}

          {/* Info Box */}
          <div className="bg-gray-50 border border-gray-200 rounded-lg p-4">
            <div className="flex items-start gap-2">
              <Info className="w-4 h-4 text-gray-600 mt-0.5" />
              <div className="text-sm text-gray-700">
                <p className="font-medium">Language Support:</p>
                <p>Selected languages will be available for menu translation and customer interface. Your default language will be used when no translation is available.</p>
              </div>
            </div>
          </div>

          {/* Save Button */}
          <div className="flex justify-end pt-4 border-t border-gray-200">
            <Button
              color="primary"
              onPress={handleSaveLanguage}
              isLoading={saving}
              isDisabled={!hasLanguageChanges()}
            >
              Save Language Settings
            </Button>
          </div>
        </CardBody>
      </Card>
    </div>
  );
}
