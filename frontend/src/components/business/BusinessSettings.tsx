'use client';

import React, { useState, useEffect, useCallback } from 'react';
import {
  Card,
  CardHeader,
  CardBody,
  Button,
  Input,
  Textarea,
  Divider,
  Chip,
} from '@nextui-org/react';
import { businessApi, BusinessAddress, UpdateBusinessRequest } from '@/api/business';
import { PrimarySpinner } from '@/components/ui/spinners/PrimarySpinner';

interface BusinessSettingsProps {
  businessId: number;
}

interface BusinessProfile {
  name: string;
  logo: string;
  address: BusinessAddress;
  settlement_address: string;
  tipping_address: string;
  tax_rate: number;
  service_fee_rate: number;
  tax_inclusive: boolean;
  service_inclusive: boolean;
}

export default function BusinessSettings({ businessId }: BusinessSettingsProps) {
  const [profile, setProfile] = useState<BusinessProfile>({
    name: '',
    logo: '',
    address: {
      street: '',
      city: '',
      state: '',
      postal_code: '',
      country: '',
    },
    settlement_address: '',
    tipping_address: '',
    tax_rate: 0,
    service_fee_rate: 0,
    tax_inclusive: false,
    service_inclusive: false,
  });
  const [isLoading, setIsLoading] = useState(false);
  const [isSaving, setIsSaving] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [successMessage, setSuccessMessage] = useState<string | null>(null);

  const loadBusinessProfile = useCallback(async () => {
    try {
      setIsLoading(true);
      setError(null);
      const business = await businessApi.getBusiness(businessId);
      if (business) {
        setProfile({
          name: business.name || '',
          logo: business.logo || '',
          address: business.address || {
            street: '',
            city: '',
            state: '',
            postal_code: '',
            country: '',
          },
          settlement_address: business.settlement_address || '',
          tipping_address: business.tipping_address || '',
          tax_rate: business.tax_rate || 0,
          service_fee_rate: business.service_fee_rate || 0,
          tax_inclusive: business.tax_inclusive || false,
          service_inclusive: business.service_inclusive || false,
        });
      }
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to load business profile');
    } finally {
      setIsLoading(false);
    }
  }, [businessId]);

  useEffect(() => {
    loadBusinessProfile();
  }, [loadBusinessProfile]);

  const handleSave = async () => {
    try {
      setIsSaving(true);
      setError(null);
      setSuccessMessage(null);
      
      const updateData: UpdateBusinessRequest = {
        name: profile.name,
        logo: profile.logo,
        address: profile.address,
        settlement_address: profile.settlement_address,
        tipping_address: profile.tipping_address,
        tax_rate: profile.tax_rate,
        service_fee_rate: profile.service_fee_rate,
        tax_inclusive: profile.tax_inclusive,
        service_inclusive: profile.service_inclusive,
      };
      
      await businessApi.updateBusiness(businessId, updateData);
      setSuccessMessage('Business settings updated successfully!');
      
      // Clear success message after 3 seconds
      setTimeout(() => setSuccessMessage(null), 3000);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to update business settings');
    } finally {
      setIsSaving(false);
    }
  };

  const handleInputChange = (field: keyof BusinessProfile, value: string | number | boolean) => {
    setProfile(prev => ({
      ...prev,
      [field]: value
    }));
  };

  const handleAddressChange = (field: keyof BusinessAddress, value: string) => {
    setProfile(prev => ({
      ...prev,
      address: {
        ...prev.address,
        [field]: value
      }
    }));
  };

  if (isLoading) {
    return <PrimarySpinner />;
  }

  return (
    <div className="space-y-6">
      <div className="flex justify-between items-center">
        <h2 className="text-2xl font-bold">Business Settings</h2>
        <Button 
          color="primary" 
          onPress={handleSave}
          isLoading={isSaving}
          isDisabled={isSaving}
        >
          {isSaving ? 'Saving...' : 'Save Changes'}
        </Button>
      </div>

      {error && (
        <Card className="border-danger">
          <CardBody>
            <p className="text-danger">{error}</p>
            <Button size="sm" color="danger" variant="light" onPress={() => setError(null)}>
              Dismiss
            </Button>
          </CardBody>
        </Card>
      )}

      {successMessage && (
        <Card className="border-success">
          <CardBody>
            <p className="text-success">{successMessage}</p>
          </CardBody>
        </Card>
      )}

      {/* Business Profile Section */}
      <Card>
        <CardHeader>
          <h3 className="text-lg font-semibold">Business Profile</h3>
        </CardHeader>
        <CardBody className="space-y-4">
          <Input
            label="Business Name"
            placeholder="Enter business name"
            value={profile.name}
            onValueChange={(value) => handleInputChange('name', value)}
            isRequired
          />
          
          <Input
            label="Logo URL"
            placeholder="https://example.com/logo.png"
            value={profile.logo}
            onValueChange={(value) => handleInputChange('logo', value)}
          />
          
          <div className="space-y-3">
            <h4 className="font-medium">Business Address</h4>
            <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
              <Input
                label="Street Address"
                placeholder="123 Main St"
                value={profile.address.street}
                onValueChange={(value) => handleAddressChange('street', value)}
              />
              <Input
                label="City"
                placeholder="City name"
                value={profile.address.city}
                onValueChange={(value) => handleAddressChange('city', value)}
              />
              <Input
                label="State/Province"
                placeholder="State or Province"
                value={profile.address.state}
                onValueChange={(value) => handleAddressChange('state', value)}
              />
              <Input
                label="Postal Code"
                placeholder="12345"
                value={profile.address.postal_code}
                onValueChange={(value) => handleAddressChange('postal_code', value)}
              />
            </div>
            <Input
              label="Country"
              placeholder="Country name"
              value={profile.address.country}
              onValueChange={(value) => handleAddressChange('country', value)}
            />
          </div>
          
          <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
            <Input
              label="Settlement Address"
              placeholder="Crypto wallet address for settlements"
              value={profile.settlement_address}
              onValueChange={(value) => handleInputChange('settlement_address', value)}
            />
            <Input
              label="Tipping Address"
              placeholder="Crypto wallet address for tips"
              value={profile.tipping_address}
              onValueChange={(value) => handleInputChange('tipping_address', value)}
            />
          </div>
        </CardBody>
      </Card>

      {/* Fees and Pricing Section */}
      <Card>
        <CardHeader>
          <h3 className="text-lg font-semibold">Fees & Pricing</h3>
        </CardHeader>
        <CardBody className="space-y-4">
          <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
            <Input
              label="Service Fee Rate (%)"
              type="number"
              placeholder="0.00"
              value={profile.service_fee_rate.toString()}
              onValueChange={(value) => handleInputChange('service_fee_rate', parseFloat(value) || 0)}
              endContent={
                <div className="pointer-events-none flex items-center">
                  <span className="text-default-400 text-small">%</span>
                </div>
              }
            />
            
            <Input
              label="Tax Rate (%)"
              type="number"
              placeholder="0.00"
              value={profile.tax_rate.toString()}
              onValueChange={(value) => handleInputChange('tax_rate', parseFloat(value) || 0)}
              endContent={
                <div className="pointer-events-none flex items-center">
                  <span className="text-default-400 text-small">%</span>
                </div>
              }
            />
          </div>
          
          <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
            <div className="flex items-center justify-between p-3 bg-default-50 rounded-lg">
              <span>Tax Inclusive Pricing</span>
              <input
                type="checkbox"
                checked={profile.tax_inclusive}
                onChange={(e) => handleInputChange('tax_inclusive', e.target.checked)}
                className="rounded"
              />
            </div>
            
            <div className="flex items-center justify-between p-3 bg-default-50 rounded-lg">
              <span>Service Fee Inclusive</span>
              <input
                type="checkbox"
                checked={profile.service_inclusive}
                onChange={(e) => handleInputChange('service_inclusive', e.target.checked)}
                className="rounded"
              />
            </div>
          </div>
          
          <div className="bg-default-50 p-4 rounded-lg">
            <h4 className="font-medium mb-2">Fee Summary</h4>
            <div className="space-y-1 text-sm">
              <div className="flex justify-between">
                <span>Service Fee Rate:</span>
                <Chip size="sm" color="secondary">{profile.service_fee_rate.toFixed(2)}%</Chip>
              </div>
              <div className="flex justify-between">
                <span>Tax Rate:</span>
                <Chip size="sm" color="warning">{profile.tax_rate.toFixed(2)}%</Chip>
              </div>
              <div className="flex justify-between">
                <span>Tax Inclusive:</span>
                <Chip size="sm" color={profile.tax_inclusive ? "success" : "default"}>
                  {profile.tax_inclusive ? "Yes" : "No"}
                </Chip>
              </div>
              <div className="flex justify-between">
                <span>Service Inclusive:</span>
                <Chip size="sm" color={profile.service_inclusive ? "success" : "default"}>
                  {profile.service_inclusive ? "Yes" : "No"}
                </Chip>
              </div>
            </div>
          </div>
        </CardBody>
      </Card>

      {/* Quick Actions */}
      <Card>
        <CardHeader>
          <h3 className="text-lg font-semibold">Quick Actions</h3>
        </CardHeader>
        <CardBody>
          <div className="flex flex-wrap gap-3">
            <Button color="primary" variant="bordered" size="sm">
              Preview Business Page
            </Button>
            <Button color="secondary" variant="bordered" size="sm">
              Download QR Codes
            </Button>
            <Button color="success" variant="bordered" size="sm">
              Export Menu
            </Button>
            <Button color="warning" variant="bordered" size="sm">
              Business Analytics
            </Button>
          </div>
        </CardBody>
      </Card>
    </div>
  );
}
