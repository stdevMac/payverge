'use client';

import React, { useState, useEffect } from 'react';
import { Card, CardBody, CardHeader, Button, Input, Checkbox, Divider } from '@nextui-org/react';
import { ArrowLeft, Building2, Wallet, Settings } from 'lucide-react';
import Link from 'next/link';
import { useRouter } from 'next/navigation';
import { createBusiness, CreateBusinessRequest } from '../../../api/business';
import BusinessTutorialModal from '../../../components/business/BusinessTutorialModal';

export default function BusinessRegisterPage() {
  const router = useRouter();
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [showTutorial, setShowTutorial] = useState(true);
  const [connectedWallet, setConnectedWallet] = useState<string | null>(null);
  const [formData, setFormData] = useState<CreateBusinessRequest>({
    name: "",
    logo: "",
    address: {
      street: "",
      city: "",
      state: "",
      postal_code: "",
      country: "",
    },
    settlement_address: "",
    tipping_address: "",
    tax_rate: 0,
    service_fee_rate: 0,
    tax_inclusive: false,
    service_inclusive: false,
  });

  // Update settlement address when wallet is connected
  useEffect(() => {
    if (connectedWallet) {
      setFormData(prev => ({
        ...prev,
        settlement_address: connectedWallet,
        tipping_address: connectedWallet, // Default to same address, user can change
      }));
    }
  }, [connectedWallet]);

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    setLoading(true);
    setError(null);

    try {
      const business = await createBusiness(formData);
      router.push(`/business/${business.id}/dashboard`);
    } catch (err: any) {
      setError(err.message || 'Failed to create business');
    } finally {
      setLoading(false);
    }
  };

  const updateFormData = (field: string, value: any) => {
    setFormData(prev => ({
      ...prev,
      [field]: value
    }));
  };

  const updateAddress = (field: string, value: string) => {
    setFormData(prev => ({
      ...prev,
      address: {
        ...prev.address,
        [field]: value
      }
    }));
  };

  const handleTutorialComplete = (walletAddress: string) => {
    setConnectedWallet(walletAddress);
    setShowTutorial(false);
  };

  const handleTutorialClose = () => {
    setShowTutorial(false);
  };

  return (
    <>
      {/* Tutorial Modal */}
      <BusinessTutorialModal
        isOpen={showTutorial}
        onClose={handleTutorialClose}
        onComplete={handleTutorialComplete}
      />

      <div className="min-h-screen bg-gray-50 py-8">
        <div className="max-w-2xl mx-auto px-4">
        {/* Header */}
        <div className="mb-8">
          <Link href="/dashboard">
            <Button
              variant="light"
              startContent={<ArrowLeft size={16} />}
              className="text-gray-600 mb-4"
            >
              Back to Dashboard
            </Button>
          </Link>
          <h1 className="text-3xl font-bold text-gray-900">Register Your Business</h1>
          <p className="text-gray-600 mt-2">
            Set up your business to start accepting crypto payments with Payverge
          </p>
        </div>

        {/* Error Message */}
        {error && (
          <Card className="mb-6 border-danger-200 bg-danger-50">
            <CardBody>
              <p className="text-danger-700">{error}</p>
            </CardBody>
          </Card>
        )}

        <form onSubmit={handleSubmit} className="space-y-8">
          {/* Basic Information */}
          <Card>
            <CardHeader className="flex gap-3">
              <Building2 className="text-primary-600" size={24} />
              <div>
                <h3 className="text-lg font-semibold">Basic Information</h3>
                <p className="text-sm text-gray-500">Tell us about your business</p>
              </div>
            </CardHeader>
            <CardBody className="space-y-4">
              <Input
                label="Business Name"
                placeholder="Enter your business name"
                value={formData.name}
                onChange={(e) => updateFormData('name', e.target.value)}
                isRequired
                variant="bordered"
              />
              
              <Input
                label="Logo URL (Optional)"
                placeholder="https://example.com/logo.png"
                value={formData.logo}
                onChange={(e) => updateFormData('logo', e.target.value)}
                variant="bordered"
              />

              <Divider />

              <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
                <Input
                  label="Street Address"
                  placeholder="123 Main St"
                  value={formData.address.street}
                  onChange={(e) => updateAddress('street', e.target.value)}
                  variant="bordered"
                />
                <Input
                  label="City"
                  placeholder="San Francisco"
                  value={formData.address.city}
                  onChange={(e) => updateAddress('city', e.target.value)}
                  variant="bordered"
                />
              </div>

              <div className="grid grid-cols-1 md:grid-cols-3 gap-4">
                <Input
                  label="State"
                  placeholder="CA"
                  value={formData.address.state}
                  onChange={(e) => updateAddress('state', e.target.value)}
                  variant="bordered"
                />
                <Input
                  label="Postal Code"
                  placeholder="94105"
                  value={formData.address.postal_code}
                  onChange={(e) => updateAddress('postal_code', e.target.value)}
                  variant="bordered"
                />
                <Input
                  label="Country"
                  placeholder="United States"
                  value={formData.address.country}
                  onChange={(e) => updateAddress('country', e.target.value)}
                  variant="bordered"
                />
              </div>
            </CardBody>
          </Card>

          {/* Wallet Configuration */}
          <Card>
            <CardHeader className="flex gap-3">
              <Wallet className="text-primary-600" size={24} />
              <div>
                <h3 className="text-lg font-semibold">Wallet Configuration</h3>
                <p className="text-sm text-gray-500">Configure where payments will be sent</p>
              </div>
            </CardHeader>
            <CardBody className="space-y-4">
              <Input
                label="Settlement Address"
                placeholder="0x..."
                value={formData.settlement_address}
                onChange={(e) => updateFormData('settlement_address', e.target.value)}
                description="Where business payments will be sent"
                isRequired
                variant="bordered"
                classNames={{
                  input: "font-mono text-sm"
                }}
              />
              
              <Input
                label="Tipping Address"
                placeholder="0x..."
                value={formData.tipping_address}
                onChange={(e) => updateFormData('tipping_address', e.target.value)}
                description="Where tips will be sent"
                isRequired
                variant="bordered"
                classNames={{
                  input: "font-mono text-sm"
                }}
              />
            </CardBody>
          </Card>

          {/* Tax & Service Fees */}
          <Card>
            <CardHeader className="flex gap-3">
              <Settings className="text-primary-600" size={24} />
              <div>
                <h3 className="text-lg font-semibold">Tax & Service Fees</h3>
                <p className="text-sm text-gray-500">Configure your pricing structure</p>
              </div>
            </CardHeader>
            <CardBody className="space-y-4">
              <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
                <Input
                  type="number"
                  label="Tax Rate (%)"
                  placeholder="8.5"
                  value={formData.tax_rate.toString()}
                  onChange={(e) => updateFormData('tax_rate', parseFloat(e.target.value) || 0)}
                  min="0"
                  max="100"
                  step="0.01"
                  variant="bordered"
                />
                
                <Input
                  type="number"
                  label="Service Fee (%)"
                  placeholder="18.0"
                  value={formData.service_fee_rate.toString()}
                  onChange={(e) => updateFormData('service_fee_rate', parseFloat(e.target.value) || 0)}
                  min="0"
                  max="100"
                  step="0.01"
                  variant="bordered"
                />
              </div>

              <div className="space-y-2">
                <Checkbox
                  isSelected={formData.tax_inclusive}
                  onValueChange={(checked) => updateFormData('tax_inclusive', checked)}
                >
                  Tax inclusive pricing
                </Checkbox>
                <Checkbox
                  isSelected={formData.service_inclusive}
                  onValueChange={(checked) => updateFormData('service_inclusive', checked)}
                >
                  Service fee inclusive pricing
                </Checkbox>
              </div>
            </CardBody>
          </Card>

          {/* Submit Button */}
          <div className="flex justify-end space-x-4">
            <Button
              variant="bordered"
              onPress={() => router.back()}
            >
              Cancel
            </Button>
            <Button
              type="submit"
              color="primary"
              isLoading={loading}
              isDisabled={!formData.name || !formData.settlement_address || !formData.tipping_address}
            >
              {loading ? 'Creating Business...' : 'Create Business'}
            </Button>
          </div>
        </form>
        </div>
      </div>
    </>
  );
}
