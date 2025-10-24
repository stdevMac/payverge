'use client';

import React, { useState, useEffect, useCallback } from 'react';
import { useSimpleLocale, getTranslation } from '@/i18n/SimpleTranslationProvider';
import {
  Switch,
  Input,
  Divider,
  Spinner
} from '@nextui-org/react';
import { Coffee, Plus, Settings, CheckCircle, AlertCircle } from 'lucide-react';
import { updateCounterSettings, getBusinessCounters, Counter } from '../../api/counters';

interface CounterManagerProps {
  businessId: number;
}

interface CounterSettings {
  counter_enabled: boolean;
  counter_count: number;
  counter_prefix: string;
}

export const CounterManager: React.FC<CounterManagerProps> = ({ businessId }) => {
  // Translation setup
  const { locale } = useSimpleLocale();
  const [currentLocale, setCurrentLocale] = useState(locale);
  
  // Update translations when locale changes
  useEffect(() => {
    setCurrentLocale(locale);
  }, [locale]);
  
  // Translation helper
  const tString = useCallback((key: string): string => {
    const fullKey = `businessDashboard.dashboard.counterManager.${key}`;
    const result = getTranslation(fullKey, currentLocale);
    return Array.isArray(result) ? result[0] || key : result as string;
  }, [currentLocale]);

  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [counters, setCounters] = useState<Counter[]>([]);
  const [settings, setSettings] = useState<CounterSettings>({
    counter_enabled: false,
    counter_count: 3,
    counter_prefix: 'C',
  });
  const [error, setError] = useState<string | null>(null);
  const [success, setSuccess] = useState<string | null>(null);

  const loadCounterData = useCallback(async () => {
    try {
      setLoading(true);
      const response = await getBusinessCounters(businessId);
      setCounters(response.counters || []);
      
      // If counters exist, extract settings from the first counter or business data
      if (response.counters && response.counters.length > 0) {
        setSettings({
          counter_enabled: true,
          counter_count: response.counters.length,
          counter_prefix: response.counters[0].name.replace(/\d+$/, '') || 'C',
        });
      }
    } catch (error) {
      console.error('Error loading counter data:', error);
      setError(tString('error.loadSettings'));
    } finally {
      setLoading(false);
    }
  }, [businessId, tString]);

  // Load counter data
  useEffect(() => {
    loadCounterData();
  }, [loadCounterData]);

  const handleSaveSettings = async () => {
    try {
      setSaving(true);
      setError(null);
      setSuccess(null);

      await updateCounterSettings(businessId, settings);
      
      setSuccess(tString('success.settingsUpdated'));
      
      // Reload counter data to show updated counters
      await loadCounterData();
      
      // Clear success message after 3 seconds
      setTimeout(() => setSuccess(null), 3000);
    } catch (error) {
      console.error('Error updating counter settings:', error);
      setError(tString('error.updateSettings'));
    } finally {
      setSaving(false);
    }
  };

  const handleSettingChange = (key: keyof CounterSettings, value: any) => {
    setSettings(prev => ({
      ...prev,
      [key]: value
    }));
  };

  if (loading) {
    return (
      <div className="flex items-center justify-center py-16">
        <div className="text-center">
          <div className="w-16 h-16 bg-gray-50 rounded-2xl flex items-center justify-center mx-auto mb-6 border border-gray-100">
            <div className="w-8 h-8 border-2 border-gray-300 border-t-gray-900 rounded-full animate-spin"></div>
          </div>
          <p className="text-gray-600 font-light tracking-wide">{tString('loading')}</p>
        </div>
      </div>
    );
  }

  return (
    <div className="p-4 max-w-6xl mx-auto">
      {/* Compact Header */}
      <div className="flex justify-between items-center mb-6">
        <div>
          <h1 className="text-2xl font-light text-gray-900 tracking-wide">{tString('title')}</h1>
          <p className="text-gray-600 font-light text-sm mt-1">{tString('subtitle')}</p>
        </div>
      </div>

      {/* Error Alert */}
      {error && (
        <div className="bg-red-50 border border-red-200 rounded-lg p-4 mb-6">
          <div className="flex items-center gap-3">
            <AlertCircle className="w-5 h-5 text-red-600 flex-shrink-0" />
            <p className="text-red-800 font-medium">{error}</p>
          </div>
        </div>
      )}

      {/* Success Alert */}
      {success && (
        <div className="bg-green-50 border border-green-200 rounded-lg p-4 mb-6">
          <div className="flex items-center gap-3">
            <CheckCircle className="w-5 h-5 text-green-600 flex-shrink-0" />
            <p className="text-green-800 font-medium">{success}</p>
          </div>
        </div>
      )}

      {/* Counter Settings */}
      <div className="bg-white border border-gray-200 rounded-lg shadow-sm">
        <div className="border-b border-gray-200 p-6">
          <div className="flex items-center gap-3">
            <div className="w-12 h-12 bg-gray-50 rounded-xl flex items-center justify-center border border-gray-100">
              <Settings className="w-6 h-6 text-gray-700" />
            </div>
            <div>
              <h2 className="text-xl font-light text-gray-900 tracking-wide">{tString('settings.title')}</h2>
              <p className="text-gray-600 font-light text-sm">{tString('settings.subtitle')}</p>
            </div>
          </div>
        </div>
        <div className="p-6 space-y-6">
          {/* Enable Counters */}
          <div className="flex items-center justify-between">
            <div>
              <h3 className="font-medium text-gray-900">{tString('settings.enableService')}</h3>
              <p className="text-sm text-gray-600 mt-1">
                {tString('settings.enableServiceDescription')}
              </p>
            </div>
            <Switch
              isSelected={settings.counter_enabled}
              onValueChange={(value) => handleSettingChange('counter_enabled', value)}
              color="success"
            />
          </div>

          {settings.counter_enabled && (
            <>
              <Divider />
              
              {/* Counter Count */}
              <div className="grid grid-cols-1 md:grid-cols-2 gap-6">
                <div>
                  <Input
                    type="number"
                    label={tString('settings.counterCount')}
                    placeholder="3"
                    value={settings.counter_count.toString()}
                    onChange={(e) => handleSettingChange('counter_count', parseInt(e.target.value) || 1)}
                    min={1}
                    max={20}
                    description={tString('settings.counterCountDescription')}
                  />
                </div>

                <div>
                  <Input
                    label={tString('settings.counterPrefix')}
                    placeholder="C"
                    value={settings.counter_prefix}
                    onChange={(e) => handleSettingChange('counter_prefix', e.target.value)}
                    maxLength={3}
                    description={tString('settings.counterPrefixDescription')}
                  />
                </div>
              </div>

              {/* Preview */}
              <div>
                <h4 className="font-medium text-gray-900 mb-3">{tString('settings.preview')}</h4>
                <div className="flex flex-wrap gap-2">
                  {Array.from({ length: Math.min(settings.counter_count, 10) }, (_, i) => (
                    <span
                      key={i}
                      className="inline-flex items-center gap-1 px-3 py-1 rounded-full text-sm font-medium bg-blue-100 text-blue-700"
                    >
                      <Coffee className="w-4 h-4" />
                      {settings.counter_prefix}{i + 1}
                    </span>
                  ))}
                  {settings.counter_count > 10 && (
                    <span className="inline-flex items-center px-3 py-1 rounded-full text-sm font-medium bg-gray-100 text-gray-700">
                      +{settings.counter_count - 10} more
                    </span>
                  )}
                </div>
              </div>
            </>
          )}

          <Divider />

          {/* Save Button */}
          <div className="flex justify-end">
            <button
              onClick={handleSaveSettings}
              disabled={saving}
              className="bg-gray-900 text-white px-4 py-2 rounded-lg font-medium hover:bg-gray-800 transition-colors flex items-center gap-2 disabled:opacity-50"
            >
              {!saving && <Plus className="w-4 h-4" />}
              {saving ? tString('buttons.saving') : tString('buttons.saveSettings')}
            </button>
          </div>
        </div>
      </div>

      {/* Current Counters */}
      {counters.length > 0 && (
        <div className="bg-white border border-gray-200 rounded-lg shadow-sm mb-6">
          <div className="border-b border-gray-200 p-6">
            <h2 className="text-xl font-light text-gray-900 tracking-wide">{tString('activeCounters.title')}</h2>
          </div>
          <div className="p-6">
            <div className="grid grid-cols-2 md:grid-cols-3 lg:grid-cols-4 xl:grid-cols-6 gap-4">
              {counters.map((counter) => (
                <div key={counter.id} className="border-2 border-gray-200 rounded-lg">
                  <div className="text-center p-4">
                    <Coffee className="w-8 h-8 text-orange-600 mx-auto mb-2" />
                    <h3 className="font-medium text-gray-900">{counter.name}</h3>
                    <span className={`inline-block px-2 py-1 rounded-full text-xs font-medium mt-2 ${
                      counter.is_active 
                        ? 'bg-green-100 text-green-700' 
                        : 'bg-gray-100 text-gray-700'
                    }`}>
                      {counter.is_active ? tString('status.active') : tString('status.inactive')}
                    </span>
                  </div>
                </div>
              ))}
            </div>
          </div>
        </div>
      )}

      {/* Info Card */}
      <div className="bg-blue-50 border border-blue-200 rounded-lg p-6">
        <div className="flex items-start gap-3">
          <Coffee className="w-6 h-6 text-blue-600 flex-shrink-0 mt-0.5" />
          <div>
            <h3 className="font-medium text-blue-900 mb-2">{tString('info.title')}</h3>
            <div className="text-sm text-blue-800 space-y-1">
              <p>• {tString('info.point1')}</p>
              <p>• {tString('info.point2')}</p>
              <p>• {tString('info.point3')}</p>
              <p>• {tString('info.point4')}</p>
            </div>
          </div>
        </div>
      </div>
    </div>
  );
};

export default CounterManager;
