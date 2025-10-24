'use client';

import React, { useState, useEffect, useCallback } from 'react';
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
import { useSimpleLocale, getTranslation } from '@/i18n/SimpleTranslationProvider';

interface CurrencySettingsProps {
  businessId: number;
  onSave?: () => void;
}

interface BusinessCurrencySettings {
  default_currency: string;
  display_currency: string;
}

export default function CurrencySettings({ businessId, onSave }: CurrencySettingsProps) {
  const { locale: currentLocale } = useSimpleLocale();
  
  // Translation helper
  const tString = useCallback((key: string): string => {
    const fullKey = `currencySettings.${key}`;
    const result = getTranslation(fullKey, currentLocale);
    return Array.isArray(result) ? result[0] || key : result as string;
  }, [currentLocale]);

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

  const loadData = useCallback(async () => {
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
      setError(tString('errors.loadFailed'));
    } finally {
      setLoading(false);
    }
  }, [businessId, tString]);

  const updateConversionPreview = useCallback(async () => {
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
  }, [settings.default_currency, settings.display_currency, sampleAmount]);

  useEffect(() => {
    loadData();
  }, [loadData]);

  useEffect(() => {
    updateConversionPreview();
  }, [updateConversionPreview]);

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
      setError(tString('errors.saveCurrencyFailed'));
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
        setError(tString('errors.selectLanguage'));
        setSaving(false);
        return;
      }

      if (!defaultLanguage || !selectedLanguages.includes(defaultLanguage)) {
        setError(tString('errors.selectDefaultLanguage'));
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
      setError(tString('errors.saveLanguageFailed'));
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
          <p className="mt-4 text-gray-600">{tString('loading')}</p>
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
              <h3 className="text-lg font-semibold text-gray-900">{tString('currency.title')}</h3>
              <p className="text-sm text-gray-600">
                {tString('currency.description')}
              </p>
            </div>
          </div>
        </CardHeader>
        <CardBody className="space-y-6">
          {/* Default Currency */}
          <div className="space-y-3">
            <div>
              <h4 className="text-md font-medium text-gray-900">{tString('currency.defaultTitle')}</h4>
              <p className="text-sm text-gray-600">
                {tString('currency.defaultDescription')}
              </p>
            </div>
            <Select
              label={tString('currency.defaultLabel')}
              placeholder={tString('currency.defaultPlaceholder')}
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
              <h4 className="text-md font-medium text-gray-900">{tString('currency.displayTitle')}</h4>
              <p className="text-sm text-gray-600">
                {tString('currency.displayDescription')}
              </p>
            </div>
            <Select
              label={tString('currency.displayLabel')}
              placeholder={tString('currency.displayPlaceholder')}
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
              <h4 className="text-md font-medium text-gray-900">{tString('currency.conversionPreview')}</h4>
              <div className="grid grid-cols-1 md:grid-cols-3 gap-4">
                
                {/* Default to USDC */}
                <div className="bg-gray-50 rounded-lg p-4">
                  <div className="text-sm text-gray-600 mb-1">{tString('currency.pricingToPayment')}</div>
                  <div className="font-medium">
                    {formatCurrency(sampleAmount, settings.default_currency)} → {formatCurrency(conversionPreview.defaultToUSDC, 'USDC')}
                  </div>
                </div>

                {/* Default to Display */}
                {settings.default_currency !== settings.display_currency && (
                  <div className="bg-gray-50 rounded-lg p-4">
                    <div className="text-sm text-gray-600 mb-1">{tString('currency.pricingToDisplay')}</div>
                    <div className="font-medium">
                      {formatCurrency(sampleAmount, settings.default_currency)} → {formatCurrency(conversionPreview.defaultToDisplay, settings.display_currency)}
                    </div>
                  </div>
                )}

                {/* Display to USDC */}
                {settings.display_currency !== 'USDC' && (
                  <div className="bg-gray-50 rounded-lg p-4">
                    <div className="text-sm text-gray-600 mb-1">{tString('currency.displayToPayment')}</div>
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
                <p className="font-medium">{tString('currency.paymentFlow')}</p>
                <p>{tString('currency.paymentFlowDescription').replace('{currency}', getCurrencyInfo(settings.display_currency)?.name || settings.display_currency)}</p>
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
              {tString('currency.saveButton')}
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
              <h3 className="text-lg font-semibold text-gray-900">{tString('language.title')}</h3>
              <p className="text-sm text-gray-600">
                {tString('language.description')}
              </p>
            </div>
          </div>
        </CardHeader>
        <CardBody className="space-y-6">
          {/* Language Selection */}
          <div className="space-y-4">
            <div>
              <h4 className="text-md font-medium text-gray-900">{tString('language.supportedTitle')}</h4>
              <p className="text-sm text-gray-600">
                {tString('language.supportedDescription')}
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
                <h4 className="text-md font-medium text-gray-900">{tString('language.defaultTitle')}</h4>
                <p className="text-sm text-gray-600">
                  {tString('language.defaultDescription')}
                </p>
              </div>
              <Select
                label={tString('language.defaultLabel')}
                placeholder={tString('language.defaultPlaceholder')}
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
                <p className="font-medium">{tString('language.supportInfo')}</p>
                <p>{tString('language.supportInfoDescription')}</p>
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
              {tString('language.saveButton')}
            </Button>
          </div>
        </CardBody>
      </Card>
    </div>
  );
}
