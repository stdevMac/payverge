'use client';

import React, { useState, useEffect, useCallback, useMemo } from 'react';
import {
  Card,
  CardHeader,
  CardBody,
  Button,
  Input,
  Textarea,
  Divider,
  Chip,
  Tabs,
  Tab,
  Switch,
  Image,
} from '@nextui-org/react';
import {
  Settings,
  Palette,
  Globe,
  CreditCard,
  Upload,
  Building2,
  Paintbrush,
  Star,
  Link,
  MapPin,
  DollarSign,
  Languages
} from 'lucide-react';
import { businessApi, BusinessAddress, UpdateBusinessRequest } from '@/api/business';
import { PrimarySpinner } from '@/components/ui/spinners/PrimarySpinner';
import SimpleImageUpload from './SimpleImageUpload';
import BannerImageUploader from './BannerImageUploader';
import CustomURLInput from './CustomURLInput';
import { useToast } from '@/contexts/ToastContext';
import CurrencySettings from './CurrencySettings';
import { useSimpleLocale, getTranslation } from '@/i18n/SimpleTranslationProvider';

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
  // New fields for enhanced features (stored locally for now)
  description?: string;
  custom_url?: string;
  phone?: string;
  website?: string;
  social_media?: {
    instagram?: string;
    facebook?: string;
    twitter?: string;
  };
}

interface DesignSettings {
  primary_color: string;
  secondary_color: string;
  font_family: string;
  menu_layout: 'grid' | 'list';
  show_images: boolean;
  show_descriptions: boolean;
  theme: 'light' | 'dark';
}

interface BusinessPageSettings {
  enabled: boolean;
  custom_url: string;
  hero_image?: string;
  about_text?: string;
  banner_images: string[];
  show_reviews: boolean;
  google_reviews_enabled: boolean;
}

export default function BusinessSettings({ businessId }: BusinessSettingsProps) {
  const { locale: currentLocale } = useSimpleLocale();
  
  // Translation helper
  const tString = (key: string): string => {
    const fullKey = `businessSettings.${key}`;
    const result = getTranslation(fullKey, currentLocale);
    return Array.isArray(result) ? result[0] || key : result as string;
  };

  const [activeTab, setActiveTab] = useState('profile');
  const { showSuccess, showError } = useToast();

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
    description: '',
    custom_url: '',
    phone: '',
    website: '',
    social_media: {
      instagram: '',
      facebook: '',
      twitter: '',
    },
  });

  const [designSettings, setDesignSettings] = useState<DesignSettings>({
    primary_color: '#3B82F6',
    secondary_color: '#10B981',
    font_family: 'Inter',
    menu_layout: 'grid',
    show_images: true,
    show_descriptions: true,
    theme: 'light',
  });

  const [businessPageSettings, setBusinessPageSettings] = useState<BusinessPageSettings>({
    enabled: false,
    custom_url: '',
    hero_image: '',
    about_text: '',
    banner_images: [],
    show_reviews: true,
    google_reviews_enabled: false,
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
          // New fields - load from backend response
          description: business.description || '',
          custom_url: business.custom_url || '',
          phone: business.phone || '',
          website: business.website || '',
          social_media: business.social_media ?
            (typeof business.social_media === 'string' ?
              JSON.parse(business.social_media) : business.social_media) : {
              instagram: '',
              facebook: '',
              twitter: '',
            },
        });

        // Load business page settings
        setBusinessPageSettings({
          enabled: business.business_page_enabled || false,
          custom_url: business.custom_url || '', // Keep for reference but use profile.custom_url
          hero_image: '', // Not implemented yet
          about_text: business.description || '', // Keep for reference but use profile.description
          banner_images: business.banner_images ?
            (typeof business.banner_images === 'string' ?
              JSON.parse(business.banner_images) : business.banner_images) : [],
          show_reviews: business.show_reviews !== undefined ? business.show_reviews : true,
          google_reviews_enabled: business.google_reviews_enabled || false,
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
        // New fields
        description: profile.description,
        custom_url: profile.custom_url,
        phone: profile.phone,
        website: profile.website,
        social_media: profile.social_media ? JSON.stringify(profile.social_media) : '',
        banner_images: JSON.stringify(businessPageSettings.banner_images),
        business_page_enabled: businessPageSettings.enabled,
        show_reviews: businessPageSettings.show_reviews,
        google_reviews_enabled: businessPageSettings.google_reviews_enabled,
      };

      await businessApi.updateBusiness(businessId, updateData);

      // Show success toast
      showSuccess(
        tString('messages.updateSuccess'),
        tString('messages.updateSuccessDescription'),
        4000
      );
    } catch (err) {
      const errorMessage = err instanceof Error ? err.message : tString('messages.updateFailed');
      setError(errorMessage);

      // Show error toast
      showError(
        tString('messages.updateFailedTitle'),
        errorMessage,
        6000
      );
    } finally {
      setIsSaving(false);
    }
  };

  const handleInputChange = (field: keyof BusinessProfile, value: any) => {
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

  const tabs = [
    {
      key: 'profile',
      title: tString('tabs.profile'),
      icon: Building2,
    },
    {
      key: 'design',
      title: tString('tabs.design'),
      icon: Palette,
    },
    {
      key: 'business-page',
      title: tString('tabs.businessPage'),
      icon: Globe,
    },
    {
      key: 'payments',
      title: tString('tabs.payments'),
      icon: CreditCard,
    },
    {
      key: 'localization',
      title: tString('tabs.localization'),
      icon: Languages,
    }
  ];

  if (isLoading) {
    return (
      <div className="flex items-center justify-center py-16">
        <PrimarySpinner />
      </div>
    );
  }

  return (
    <div className="w-full max-w-7xl mx-auto p-6">
      <div className="mb-8">
        <h1 className="text-3xl font-bold text-default-900 mb-2">{tString('title')}</h1>
        <p className="text-default-600">
          {tString('description')}
        </p>
      </div>

      {error && (
        <Card className="border-danger-200 bg-danger-50 shadow-md mb-6">
          <CardBody className="p-6">
            <div className="flex items-center gap-4">
              <div className="w-12 h-12 bg-danger-100 rounded-xl flex items-center justify-center">
                <Settings className="h-6 w-6 text-danger-600" />
              </div>
              <div>
                <h3 className="text-lg font-semibold text-danger-900">{tString('messages.error')}</h3>
                <p className="text-danger-700">{error}</p>
              </div>
            </div>
          </CardBody>
        </Card>
      )}

      {successMessage && (
        <Card className="border-success-200 bg-success-50 shadow-md mb-6">
          <CardBody className="p-6">
            <div className="flex items-center gap-4">
              <div className="w-12 h-12 bg-success-100 rounded-xl flex items-center justify-center">
                <Settings className="h-6 w-6 text-success-600" />
              </div>
              <div>
                <h3 className="text-lg font-semibold text-success-900">{tString('messages.success')}</h3>
                <p className="text-success-700">{successMessage}</p>
              </div>
            </div>
          </CardBody>
        </Card>
      )}

      <Card className="w-full shadow-lg">
        <CardBody className="p-0">
          <Tabs
            aria-label={tString('tabs.ariaLabel')}
            selectedKey={activeTab}
            onSelectionChange={(key) => setActiveTab(key as string)}
            variant="underlined"
            classNames={{
              tabList: "gap-6 w-full relative rounded-none p-0 border-b border-divider",
              cursor: "w-full bg-primary",
              tab: "max-w-fit px-6 py-4 h-12",
              tabContent: "group-data-[selected=true]:text-primary"
            }}
          >
            {tabs.map((tab) => {
              const Icon = tab.icon;
              return (
                <Tab
                  key={tab.key}
                  title={
                    <div className="flex items-center gap-2">
                      <Icon className="w-4 h-4" />
                      <span>{tab.title}</span>
                    </div>
                  }
                >
                  <div className="p-6">
                    {tab.key === 'profile' && (
                      <BusinessProfileTab
                        profile={profile}
                        onProfileChange={setProfile}
                        handleInputChange={handleInputChange}
                        handleAddressChange={handleAddressChange}
                        businessId={businessId}
                      />
                    )}
                    {tab.key === 'design' && (
                      <DesignBrandingTab
                        settings={designSettings}
                        onSettingsChange={setDesignSettings}
                      />
                    )}
                    {tab.key === 'business-page' && (
                      <BusinessPageTab
                        settings={businessPageSettings}
                        onSettingsChange={setBusinessPageSettings}
                        profile={profile}
                        onProfileChange={handleInputChange}
                        businessId={businessId}
                      />
                    )}
                    {tab.key === 'payments' && (
                      <PaymentSettingsTab
                        profile={profile}
                        onProfileChange={setProfile}
                        handleInputChange={handleInputChange}
                      />
                    )}
                    {tab.key === 'localization' && (
                      <div className="space-y-6">
                        <CurrencySettings businessId={businessId} onSave={loadBusinessProfile} />
                      </div>
                    )}
                  </div>
                </Tab>
              );
            })}
          </Tabs>
        </CardBody>
      </Card>

      {/* Save Button - Fixed at bottom */}
      <div className="mt-8 flex justify-end">
        <Button
          color="primary"
          size="lg"
          onPress={handleSave}
          isLoading={isSaving}
          isDisabled={isSaving}
          startContent={<Settings className="w-5 h-5" />}
        >
          {isSaving ? tString('buttons.saving') : tString('buttons.saveAll')}
        </Button>
      </div>
    </div>
  );
}

// Tab Components
function BusinessProfileTab({ profile, onProfileChange, handleInputChange, handleAddressChange, businessId }: {
  profile: BusinessProfile,
  onProfileChange: (profile: BusinessProfile) => void,
  handleInputChange: (field: keyof BusinessProfile, value: string | number | boolean) => void,
  handleAddressChange: (field: keyof BusinessAddress, value: string) => void,
  businessId: number
}) {
  const { locale: currentLocale } = useSimpleLocale();
  
  // Translation helper
  const tString = (key: string): string => {
    const fullKey = `businessSettings.${key}`;
    const result = getTranslation(fullKey, currentLocale);
    return Array.isArray(result) ? result[0] || key : result as string;
  };
  return (
    <div className="space-y-8">
      {/* Basic Information */}
      <Card className="shadow-md">
        <CardHeader className="pb-3">
          <div className="flex items-center gap-3">
            <div className="p-2 rounded-lg bg-primary/10">
              <Building2 className="w-5 h-5 text-primary" />
            </div>
            <h3 className="text-xl font-semibold text-default-900">{tString('profile.basicInfo')}</h3>
          </div>
        </CardHeader>
        <CardBody className="space-y-6">
          <div className="grid grid-cols-1 lg:grid-cols-2 gap-6">
            <Input
              label={tString('profile.businessName')}
              placeholder={tString('profile.businessNamePlaceholder')}
              value={profile.name}
              onValueChange={(value) => handleInputChange('name', value)}
              isRequired
              size="lg"
              variant="bordered"
            />

            <Input
              label={tString('profile.phoneNumber')}
              placeholder={tString('profile.phonePlaceholder')}
              value={profile.phone || ''}
              onValueChange={(value) => handleInputChange('phone', value)}
              size="lg"
              variant="bordered"
            />
          </div>

          <div className="grid grid-cols-1 lg:grid-cols-2 gap-6">
            <Input
              label={tString('profile.websiteUrl')}
              placeholder={tString('profile.websitePlaceholder')}
              value={profile.website || ''}
              onValueChange={(value) => handleInputChange('website', value)}
              size="lg"
              variant="bordered"
            />

            <CustomURLInput
              value={profile.custom_url || ''}
              onChange={(value) => handleInputChange('custom_url', value)}
              businessId={businessId}
              label={tString('profile.customUrl')}
              placeholder={tString('profile.customUrlPlaceholder')}
              description={tString('profile.customUrlDescription')}
            />
          </div>

          <Textarea
            label={tString('profile.description')}
            placeholder={tString('profile.descriptionPlaceholder')}
            value={profile.description || ''}
            onValueChange={(value) => handleInputChange('description', value)}
            size="lg"
            variant="bordered"
            minRows={3}
          />
        </CardBody>
      </Card>

      {/* Logo Upload */}
      <Card className="shadow-md">
        <CardHeader className="pb-3">
          <div className="flex items-center gap-3">
            <div className="p-2 rounded-lg bg-secondary/10">
              <Upload className="w-5 h-5 text-secondary" />
            </div>
            <h3 className="text-xl font-semibold text-default-900">{tString('profile.logoBranding')}</h3>
          </div>
        </CardHeader>
        <CardBody className="space-y-6">
          <SimpleImageUpload
            currentImage={profile.logo}
            onImageUploaded={(url: string) => handleInputChange('logo', url)}
            businessId={businessId}
            type="business-logo"
            title={tString('profile.businessLogo')}
            description={tString('profile.logoDescription')}
            maxSize={2}
          />
        </CardBody>
      </Card>

      {/* Address Information */}
      <Card className="shadow-md">
        <CardHeader className="pb-3">
          <div className="flex items-center gap-3">
            <div className="p-2 rounded-lg bg-warning/10">
              <MapPin className="w-5 h-5 text-warning" />
            </div>
            <h3 className="text-xl font-semibold text-default-900">{tString('profile.location')}</h3>
          </div>
        </CardHeader>
        <CardBody className="space-y-6">
          <Input
            label={tString('profile.streetAddress')}
            placeholder={tString('profile.streetPlaceholder')}
            value={profile.address.street}
            onValueChange={(value) => handleAddressChange('street', value)}
            size="lg"
            variant="bordered"
          />

          <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
            <Input
              label={tString('profile.city')}
              placeholder={tString('profile.cityPlaceholder')}
              value={profile.address.city}
              onValueChange={(value) => handleAddressChange('city', value)}
              size="lg"
              variant="bordered"
            />
            <Input
              label={tString('profile.state')}
              placeholder={tString('profile.statePlaceholder')}
              value={profile.address.state}
              onValueChange={(value) => handleAddressChange('state', value)}
              size="lg"
              variant="bordered"
            />
          </div>

          <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
            <Input
              label={tString('profile.postalCode')}
              placeholder={tString('profile.postalPlaceholder')}
              value={profile.address.postal_code}
              onValueChange={(value) => handleAddressChange('postal_code', value)}
              size="lg"
              variant="bordered"
            />
            <Input
              label={tString('profile.country')}
              placeholder={tString('profile.countryPlaceholder')}
              value={profile.address.country}
              onValueChange={(value) => handleAddressChange('country', value)}
              size="lg"
              variant="bordered"
            />
          </div>
        </CardBody>
      </Card>
    </div>
  );
}

function DesignBrandingTab({ settings, onSettingsChange }: { settings: DesignSettings, onSettingsChange: (settings: DesignSettings) => void }) {
  const { locale: currentLocale } = useSimpleLocale();
  
  // Translation helper
  const tString = (key: string): string => {
    const fullKey = `businessSettings.${key}`;
    const result = getTranslation(fullKey, currentLocale);
    return Array.isArray(result) ? result[0] || key : result as string;
  };
  return (
    <div className="space-y-8">
      <Card className="shadow-md">
        <CardHeader className="pb-3">
          <div className="flex items-center gap-3">
            <div className="p-2 rounded-lg bg-primary/10">
              <Palette className="w-5 h-5 text-primary" />
            </div>
            <h3 className="text-xl font-semibold text-default-900">{tString('design.title')}</h3>
          </div>
        </CardHeader>
        <CardBody>
          <div className="text-center py-12">
            <Paintbrush className="w-16 h-16 text-default-300 mx-auto mb-4" />
            <h4 className="text-xl font-semibold text-default-700 mb-2">{tString('design.comingSoonTitle')}</h4>
            <p className="text-default-500">{tString('design.comingSoonDescription')}</p>
          </div>
        </CardBody>
      </Card>
    </div>
  );
}

function BusinessPageTab({ settings, onSettingsChange, profile, onProfileChange, businessId }: {
  settings: BusinessPageSettings,
  onSettingsChange: (settings: BusinessPageSettings) => void,
  profile: BusinessProfile,
  onProfileChange: (field: keyof BusinessProfile, value: any) => void,
  businessId: number
}) {
  const { locale: currentLocale } = useSimpleLocale();
  
  // Translation helper
  const tString = (key: string): string => {
    const fullKey = `businessSettings.${key}`;
    const result = getTranslation(fullKey, currentLocale);
    return Array.isArray(result) ? result[0] || key : result as string;
  };
  const handleBannerUpload = useCallback((banners: string[]) => {
    onSettingsChange({
      ...settings,
      banner_images: banners
    });
  }, [settings, onSettingsChange]);

  const handleSocialMediaChange = useCallback((platform: 'instagram' | 'facebook' | 'twitter', value: string) => {
    const updatedSocialMedia = {
      ...profile.social_media,
      [platform]: value
    };
    onProfileChange('social_media', updatedSocialMedia);
  }, [profile.social_media, onProfileChange]);

  // Memoize banner images to prevent unnecessary re-renders
  const bannerImages = useMemo(() => settings.banner_images, [settings.banner_images]);

  return (
    <div className="space-y-8">
      {/* Business Page Settings */}
      <Card className="shadow-md">
        <CardHeader className="pb-3">
          <div className="flex items-center gap-3">
            <div className="p-2 rounded-lg bg-success/10">
              <Globe className="w-5 h-5 text-success" />
            </div>
            <h3 className="text-xl font-semibold text-default-900">{tString('businessPage.title')}</h3>
          </div>
        </CardHeader>
        <CardBody className="space-y-6">
          <div className="flex items-center justify-between p-4 bg-default-50 rounded-lg border">
            <div>
              <span className="font-medium">{tString('businessPage.enableTitle')}</span>
              <p className="text-sm text-default-500">{tString('businessPage.enableDescription')}</p>
            </div>
            <Switch
              isSelected={settings.enabled}
              onValueChange={(value) => onSettingsChange({ ...settings, enabled: value })}
            />
          </div>

          <CustomURLInput
            value={profile.custom_url || ''}
            onChange={(value) => onProfileChange('custom_url', value)}
            businessId={businessId}
            label={tString('businessPage.customUrlLabel')}
            placeholder={tString('businessPage.customUrlPlaceholder')}
            description={tString('businessPage.customUrlDescription')}
          />

          <Textarea
            label={tString('businessPage.aboutLabel')}
            placeholder={tString('businessPage.aboutPlaceholder')}
            value={profile.description || ''}
            onValueChange={(value) => onProfileChange('description', value)}
            size="lg"
            variant="bordered"
            minRows={4}
          />

          {/* Social Media Links */}
          <div className="space-y-4">
            <h4 className="text-lg font-semibold text-default-900">{tString('businessPage.socialMediaTitle')}</h4>
            <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
              <Input
                label={tString('businessPage.instagram')}
                placeholder={tString('businessPage.instagramPlaceholder')}
                value={profile.social_media?.instagram || ''}
                onValueChange={(value) => handleSocialMediaChange('instagram', value)}
                size="lg"
                variant="bordered"
                startContent={
                  <div className="pointer-events-none flex items-center">
                    <span className="text-default-400 text-sm">@</span>
                  </div>
                }
              />
              <Input
                label={tString('businessPage.facebook')}
                placeholder={tString('businessPage.facebookPlaceholder')}
                value={profile.social_media?.facebook || ''}
                onValueChange={(value) => handleSocialMediaChange('facebook', value)}
                size="lg"
                variant="bordered"
                startContent={
                  <div className="pointer-events-none flex items-center">
                    <span className="text-default-400 text-sm">facebook.com/</span>
                  </div>
                }
              />
              <Input
                label={tString('businessPage.twitter')}
                placeholder={tString('businessPage.twitterPlaceholder')}
                value={profile.social_media?.twitter || ''}
                onValueChange={(value) => handleSocialMediaChange('twitter', value)}
                size="lg"
                variant="bordered"
                startContent={
                  <div className="pointer-events-none flex items-center">
                    <span className="text-default-400 text-sm">@</span>
                  </div>
                }
              />
            </div>
          </div>
        </CardBody>
      </Card>

      {/* Banner Images */}
      <Card className="shadow-md">
        <CardHeader className="pb-3">
          <div className="flex items-center gap-3">
            <div className="p-2 rounded-lg bg-warning/10">
              <Paintbrush className="w-5 h-5 text-warning" />
            </div>
            <h3 className="text-xl font-semibold text-default-900">{tString('businessPage.bannerTitle')}</h3>
          </div>
        </CardHeader>
        <CardBody className="space-y-6">
          <p className="text-sm text-default-600">
            {tString('businessPage.bannerDescription')}
          </p>

          <BannerImageUploader
            bannerImages={bannerImages}
            onBannersChange={handleBannerUpload}
            businessId={businessId}
            maxImages={3}
            maxSize={10}
          />
        </CardBody>
      </Card>

      {/* Reviews Integration */}
      <Card className="shadow-md">
        <CardHeader className="pb-3">
          <div className="flex items-center gap-3">
            <div className="p-2 rounded-lg bg-primary/10">
              <Star className="w-5 h-5 text-primary" />
            </div>
            <h3 className="text-xl font-semibold text-default-900">{tString('businessPage.reviewsTitle')}</h3>
          </div>
        </CardHeader>
        <CardBody className="space-y-6">
          <div className="flex items-center justify-between p-4 bg-default-50 rounded-lg border">
            <div>
              <span className="font-medium">{tString('businessPage.showReviewsTitle')}</span>
              <p className="text-sm text-default-500">{tString('businessPage.showReviewsDescription')}</p>
            </div>
            <Switch
              isSelected={settings.show_reviews}
              onValueChange={(value) => onSettingsChange({ ...settings, show_reviews: value })}
            />
          </div>

          <div className="flex items-center justify-between p-4 bg-default-50 rounded-lg border">
            <div>
              <span className="font-medium">{tString('businessPage.googleReviewsTitle')}</span>
              <p className="text-sm text-default-500">{tString('businessPage.googleReviewsDescription')}</p>
            </div>
            <Switch
              isSelected={settings.google_reviews_enabled}
              onValueChange={(value) => onSettingsChange({ ...settings, google_reviews_enabled: value })}
              isDisabled
            />
          </div>

          <div className="bg-blue-50 p-4 rounded-lg border border-blue-200">
            <h4 className="font-semibold text-blue-900 mb-2">{tString('businessPage.comingSoonTitle')}</h4>
            <p className="text-blue-700 text-sm">
              {tString('businessPage.comingSoonDescription')}
            </p>
          </div>
        </CardBody>
      </Card>
    </div>
  );
}

function PaymentSettingsTab({ profile, onProfileChange, handleInputChange }: {
  profile: BusinessProfile,
  onProfileChange: (profile: BusinessProfile) => void,
  handleInputChange: (field: keyof BusinessProfile, value: string | number | boolean) => void
}) {
  const { locale: currentLocale } = useSimpleLocale();
  
  // Translation helper
  const tString = (key: string): string => {
    const fullKey = `businessSettings.${key}`;
    const result = getTranslation(fullKey, currentLocale);
    return Array.isArray(result) ? result[0] || key : result as string;
  };
  return (
    <div className="space-y-8">
      {/* Crypto Addresses */}
      <Card className="shadow-md">
        <CardHeader className="pb-3">
          <div className="flex items-center gap-3">
            <div className="p-2 rounded-lg bg-warning/10">
              <CreditCard className="w-5 h-5 text-warning" />
            </div>
            <h3 className="text-xl font-semibold text-default-900">{tString('payment.cryptoAddressesTitle')}</h3>
          </div>
        </CardHeader>
        <CardBody className="space-y-6">
          <Input
            label={tString('payment.settlementAddressLabel')}
            placeholder={tString('payment.settlementAddressPlaceholder')}
            value={profile.settlement_address}
            onValueChange={(value) => handleInputChange('settlement_address', value)}
            description={tString('payment.settlementAddressDescription')}
            size="lg"
            variant="bordered"
            isRequired
          />

          <Input
            label={tString('payment.tippingAddressLabel')}
            placeholder={tString('payment.tippingAddressPlaceholder')}
            value={profile.tipping_address}
            onValueChange={(value) => handleInputChange('tipping_address', value)}
            description={tString('payment.tippingAddressDescription')}
            size="lg"
            variant="bordered"
            isRequired
          />
        </CardBody>
      </Card>

      {/* Fees and Pricing */}
      <Card className="shadow-md">
        <CardHeader className="pb-3">
          <div className="flex items-center gap-3">
            <div className="p-2 rounded-lg bg-success/10">
              <DollarSign className="w-5 h-5 text-success" />
            </div>
            <h3 className="text-xl font-semibold text-default-900">{tString('payment.feesTitle')}</h3>
          </div>
        </CardHeader>
        <CardBody className="space-y-6">
          <div className="grid grid-cols-1 md:grid-cols-2 gap-6">
            <Input
              label={tString('payment.serviceFeeLabel')}
              type="number"
              placeholder={tString('payment.percentagePlaceholder')}
              value={profile.service_fee_rate.toString()}
              onValueChange={(value) => handleInputChange('service_fee_rate', parseFloat(value) || 0)}
              endContent={
                <div className="pointer-events-none flex items-center">
                  <span className="text-default-400 text-small">%</span>
                </div>
              }
              size="lg"
              variant="bordered"
            />

            <Input
              label={tString('payment.taxRateLabel')}
              type="number"
              placeholder={tString('payment.percentagePlaceholder')}
              value={profile.tax_rate.toString()}
              onValueChange={(value) => handleInputChange('tax_rate', parseFloat(value) || 0)}
              endContent={
                <div className="pointer-events-none flex items-center">
                  <span className="text-default-400 text-small">%</span>
                </div>
              }
              size="lg"
              variant="bordered"
            />
          </div>

          <div className="grid grid-cols-1 md:grid-cols-2 gap-6">
            <div className="flex items-center justify-between p-4 bg-default-50 rounded-lg border">
              <div>
                <span className="font-medium">{tString('payment.taxInclusiveTitle')}</span>
                <p className="text-sm text-default-500">{tString('payment.taxInclusiveDescription')}</p>
              </div>
              <Switch
                isSelected={profile.tax_inclusive}
                onValueChange={(value) => handleInputChange('tax_inclusive', value)}
              />
            </div>

            <div className="flex items-center justify-between p-4 bg-default-50 rounded-lg border">
              <div>
                <span className="font-medium">{tString('payment.serviceFeeInclusiveTitle')}</span>
                <p className="text-sm text-default-500">{tString('payment.serviceFeeInclusiveDescription')}</p>
              </div>
              <Switch
                isSelected={profile.service_inclusive}
                onValueChange={(value) => handleInputChange('service_inclusive', value)}
              />
            </div>
          </div>

          <div className="bg-default-50 p-6 rounded-lg border">
            <h4 className="font-semibold mb-4 text-default-900">{tString('payment.feeSummaryTitle')}</h4>
            <div className="space-y-3">
              <div className="flex justify-between items-center">
                <span className="text-default-600">{tString('payment.serviceFeeRate')}:</span>
                <Chip size="sm" color="secondary" variant="flat">{profile.service_fee_rate.toFixed(2)}%</Chip>
              </div>
              <div className="flex justify-between items-center">
                <span className="text-default-600">{tString('payment.taxRate')}:</span>
                <Chip size="sm" color="warning" variant="flat">{profile.tax_rate.toFixed(2)}%</Chip>
              </div>
              <div className="flex justify-between items-center">
                <span className="text-default-600">{tString('payment.taxInclusive')}:</span>
                <Chip size="sm" color={profile.tax_inclusive ? "success" : "default"} variant="flat">
                  {profile.tax_inclusive ? tString('common.yes') : tString('common.no')}
                </Chip>
              </div>
              <div className="flex justify-between items-center">
                <span className="text-default-600">{tString('payment.serviceInclusive')}:</span>
                <Chip size="sm" color={profile.service_inclusive ? "success" : "default"} variant="flat">
                  {profile.service_inclusive ? tString('common.yes') : tString('common.no')}
                </Chip>
              </div>
            </div>
          </div>
        </CardBody>
      </Card>
    </div>
  );
}
