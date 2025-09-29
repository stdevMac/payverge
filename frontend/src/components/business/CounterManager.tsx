'use client';

import React, { useState, useEffect } from 'react';
import {
  Card,
  CardBody,
  CardHeader,
  Button,
  Switch,
  Input,
  Divider,
  Chip,
  Spinner,
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

  // Load counter data
  useEffect(() => {
    loadCounterData();
  }, [businessId]);

  const loadCounterData = async () => {
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
      setError('Failed to load counter settings');
    } finally {
      setLoading(false);
    }
  };

  const handleSaveSettings = async () => {
    try {
      setSaving(true);
      setError(null);
      setSuccess(null);

      await updateCounterSettings(businessId, settings);
      
      setSuccess('Counter settings updated successfully!');
      
      // Reload counter data to show updated counters
      await loadCounterData();
      
      // Clear success message after 3 seconds
      setTimeout(() => setSuccess(null), 3000);
    } catch (error) {
      console.error('Error updating counter settings:', error);
      setError('Failed to update counter settings');
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
      <div className="flex items-center justify-center min-h-[400px]">
        <Spinner size="lg" />
      </div>
    );
  }

  return (
    <div className="space-y-6">
      {/* Header */}
      <div className="flex items-center gap-3">
        <Coffee className="w-8 h-8 text-orange-600" />
        <div>
          <h1 className="text-2xl font-light text-gray-900 tracking-wide">Counter Management</h1>
          <p className="text-gray-600 mt-1">Configure counters for takeaway and quick service orders</p>
        </div>
      </div>

      {/* Alerts */}
      {error && (
        <Card className="border-red-200 bg-red-50">
          <CardBody className="flex flex-row items-center gap-3">
            <AlertCircle className="w-5 h-5 text-red-600 flex-shrink-0" />
            <p className="text-red-800">{error}</p>
          </CardBody>
        </Card>
      )}

      {success && (
        <Card className="border-green-200 bg-green-50">
          <CardBody className="flex flex-row items-center gap-3">
            <CheckCircle className="w-5 h-5 text-green-600 flex-shrink-0" />
            <p className="text-green-800">{success}</p>
          </CardBody>
        </Card>
      )}

      {/* Counter Settings */}
      <Card>
        <CardHeader>
          <div className="flex items-center gap-3">
            <Settings className="w-6 h-6 text-gray-700" />
            <h2 className="text-xl font-light text-gray-900 tracking-wide">Counter Settings</h2>
          </div>
        </CardHeader>
        <CardBody className="space-y-6">
          {/* Enable Counters */}
          <div className="flex items-center justify-between">
            <div>
              <h3 className="font-medium text-gray-900">Enable Counter Service</h3>
              <p className="text-sm text-gray-600 mt-1">
                Allow customers to place takeaway orders at service counters
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
                    label="Number of Counters"
                    placeholder="3"
                    value={settings.counter_count.toString()}
                    onChange={(e) => handleSettingChange('counter_count', parseInt(e.target.value) || 1)}
                    min={1}
                    max={20}
                    description="How many service counters do you have? (1-20)"
                  />
                </div>

                <div>
                  <Input
                    label="Counter Name Prefix"
                    placeholder="C"
                    value={settings.counter_prefix}
                    onChange={(e) => handleSettingChange('counter_prefix', e.target.value)}
                    maxLength={3}
                    description="Prefix for counter names (e.g., 'C' creates C1, C2, C3)"
                  />
                </div>
              </div>

              {/* Preview */}
              <div>
                <h4 className="font-medium text-gray-900 mb-3">Counter Preview</h4>
                <div className="flex flex-wrap gap-2">
                  {Array.from({ length: Math.min(settings.counter_count, 10) }, (_, i) => (
                    <Chip
                      key={i}
                      variant="flat"
                      color="primary"
                      startContent={<Coffee className="w-4 h-4" />}
                    >
                      {settings.counter_prefix}{i + 1}
                    </Chip>
                  ))}
                  {settings.counter_count > 10 && (
                    <Chip variant="flat" color="default">
                      +{settings.counter_count - 10} more
                    </Chip>
                  )}
                </div>
              </div>
            </>
          )}

          <Divider />

          {/* Save Button */}
          <div className="flex justify-end">
            <Button
              color="primary"
              onPress={handleSaveSettings}
              isLoading={saving}
              startContent={!saving && <Plus className="w-4 h-4" />}
            >
              {saving ? 'Saving...' : 'Save Counter Settings'}
            </Button>
          </div>
        </CardBody>
      </Card>

      {/* Current Counters */}
      {counters.length > 0 && (
        <Card>
          <CardHeader>
            <h2 className="text-xl font-light text-gray-900 tracking-wide">Active Counters</h2>
          </CardHeader>
          <CardBody>
            <div className="grid grid-cols-2 md:grid-cols-3 lg:grid-cols-4 xl:grid-cols-6 gap-4">
              {counters.map((counter) => (
                <Card key={counter.id} className="border-2 border-gray-200">
                  <CardBody className="text-center p-4">
                    <Coffee className="w-8 h-8 text-orange-600 mx-auto mb-2" />
                    <h3 className="font-medium text-gray-900">{counter.name}</h3>
                    <Chip
                      size="sm"
                      variant="flat"
                      color={counter.is_active ? "success" : "default"}
                      className="mt-2"
                    >
                      {counter.is_active ? "Active" : "Inactive"}
                    </Chip>
                  </CardBody>
                </Card>
              ))}
            </div>
          </CardBody>
        </Card>
      )}

      {/* Info Card */}
      <Card className="bg-blue-50 border-blue-200">
        <CardBody>
          <div className="flex items-start gap-3">
            <Coffee className="w-6 h-6 text-blue-600 flex-shrink-0 mt-0.5" />
            <div>
              <h3 className="font-medium text-blue-900 mb-2">About Counter Service</h3>
              <div className="text-sm text-blue-800 space-y-1">
                <p>• Counters are perfect for takeaway orders, coffee shops, and quick service</p>
                <p>• Staff can create bills for counter service alongside table service</p>
                <p>• Each counter can handle one order at a time</p>
                <p>• Customers can pay and collect their orders at the designated counter</p>
              </div>
            </div>
          </div>
        </CardBody>
      </Card>
    </div>
  );
};

export default CounterManager;
