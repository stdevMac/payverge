'use client';

import React, { useState, useCallback } from 'react';
import {
  Card,
  CardBody,
  Input,
  Button,
  Spinner,
  Chip,
  Divider,
  Link,
  Modal,
  ModalContent,
  ModalHeader,
  ModalBody,
  ModalFooter,
  useDisclosure
} from '@nextui-org/react';
import { 
  Search, 
  MapPin, 
  Star, 
  Users, 
  ExternalLink, 
  Check, 
  X, 
  Globe,
  AlertTriangle,
  Building2,
  Settings
} from 'lucide-react';
import { 
  searchGoogleBusinesses, 
  updateBusinessGoogleInfo, 
  removeBusinessGoogleInfo,
  formatBusinessQuery,
  type GooglePlaceResult 
} from '../../api/google';
import { useToast } from '../../contexts/ToastContext';
import { useSimpleLocale, getTranslation } from '../../i18n/SimpleTranslationProvider';

interface GoogleBusinessSearchProps {
  businessId: string;
  businessName: string;
  businessAddress?: string;
  currentGoogleInfo?: {
    google_place_id?: string;
    google_business_name?: string;
    google_review_link?: string;
    google_business_url?: string;
    google_reviews_enabled?: boolean;
  };
  onUpdate?: () => void;
}

export default function GoogleBusinessSearch({
  businessId,
  businessName,
  businessAddress,
  currentGoogleInfo,
  onUpdate
}: GoogleBusinessSearchProps) {
  const [searchQuery, setSearchQuery] = useState('');
  const [searchResults, setSearchResults] = useState<GooglePlaceResult[]>([]);
  const [isSearching, setIsSearching] = useState(false);
  const [isUpdating, setIsUpdating] = useState(false);
  const [selectedBusiness, setSelectedBusiness] = useState<GooglePlaceResult | null>(null);
  const { isOpen, onOpen, onClose } = useDisclosure();
  const { showError, showSuccess, showWarning } = useToast();
  const { locale: currentLocale } = useSimpleLocale();
  
  // Translation helper
  const tString = (key: string): string => {
    const fullKey = `googleBusiness.${key}`;
    const translation = getTranslation(fullKey, currentLocale);
    return Array.isArray(translation) ? translation[0] : translation || key;
  };

  // Keep search query empty by default - let users type what they want

  // Clear default when user starts typing their own query
  const handleSearchInputChange = (value: string) => {
    setSearchQuery(value);
  };

  const handleSearch = useCallback(async () => {
    if (!searchQuery.trim()) {
      showError(tString('errors.emptyQuery'));
      return;
    }

    setIsSearching(true);
    try {
      const response = await searchGoogleBusinesses(searchQuery.trim());
      setSearchResults(response.results);
      
      if (response.results.length === 0) {
        showWarning(tString('errors.noResults'));
      }
    } catch (error) {
      console.error('Error searching businesses:', error);
      showError(tString('errors.searchFailed'));
    } finally {
      setIsSearching(false);
    }
  }, [searchQuery, showError, showWarning]);

  const handleSelectBusiness = (business: GooglePlaceResult) => {
    setSelectedBusiness(business);
    onOpen();
  };

  const handleConfirmSelection = async () => {
    if (!selectedBusiness) return;

    setIsUpdating(true);
    try {
      await updateBusinessGoogleInfo(businessId, {
        place_id: selectedBusiness.place_id,
        business_name: selectedBusiness.name
      });
      
      showSuccess(tString('success.linked'));
      onClose();
      setSelectedBusiness(null);
      setSearchResults([]);
      setSearchQuery('');
      onUpdate?.();
    } catch (error) {
      console.error('Error updating Google business info:', error);
      showError(tString('errors.linkFailed'));
    } finally {
      setIsUpdating(false);
    }
  };

  const handleRemoveIntegration = async () => {
    setIsUpdating(true);
    try {
      await removeBusinessGoogleInfo(businessId);
      showSuccess(tString('success.removed'));
      onUpdate?.();
    } catch (error) {
      console.error('Error removing Google business info:', error);
      showError(tString('errors.removeFailed'));
    } finally {
      setIsUpdating(false);
    }
  };

  const renderBusinessCard = (business: GooglePlaceResult) => (
    <Card 
      key={business.place_id} 
      className="cursor-pointer hover:shadow-md transition-shadow"
      isPressable
      onPress={() => handleSelectBusiness(business)}
    >
      <CardBody className="p-4">
        <div className="flex justify-between items-start mb-2">
          <h4 className="font-semibold text-lg">{business.name}</h4>
          {business.rating && (
            <div className="flex items-center gap-1">
              <Star className="w-4 h-4 fill-yellow-400 text-yellow-400" />
              <span className="text-sm font-medium">{business.rating}</span>
              {business.user_ratings_total && (
                <span className="text-xs text-default-500">
                  ({business.user_ratings_total})
                </span>
              )}
            </div>
          )}
        </div>
        
        <div className="flex items-start gap-2 mb-3">
          <MapPin className="w-4 h-4 text-default-500 mt-0.5 flex-shrink-0" />
          <p className="text-sm text-default-600">{business.formatted_address}</p>
        </div>

        {business.types && business.types.length > 0 && (
          <div className="flex flex-wrap gap-1">
            {business.types.slice(0, 3).map((type) => (
              <Chip 
                key={type} 
                size="sm" 
                variant="flat" 
                className="text-xs"
              >
                {type.replace(/_/g, ' ')}
              </Chip>
            ))}
          </div>
        )}
      </CardBody>
    </Card>
  );

  // If Google business is already configured
  const isGoogleBusinessConnected = currentGoogleInfo?.google_place_id && 
    currentGoogleInfo.google_place_id.trim() !== '' &&
    currentGoogleInfo.google_reviews_enabled;

  if (isGoogleBusinessConnected) {
    return (
      <Card>
        <CardBody className="p-6">
          <div className="flex items-center justify-between mb-4">
            <div className="flex items-center gap-2">
              <Globe className="w-5 h-5 text-success" />
              <h3 className="text-lg font-semibold">{tString('connected.title')}</h3>
            </div>
            <Chip color="success" variant="flat" size="sm">
              {tString('connected.status')}
            </Chip>
          </div>

          <div className="space-y-4 mb-6">
            <div>
              <p className="text-sm text-default-500 mb-1">{tString('connected.businessName')}</p>
              <p className="font-medium">{currentGoogleInfo.google_business_name}</p>
            </div>

            <div className="space-y-2">
              <p className="text-sm text-default-500">{tString('connected.quickActions')}</p>
              
              <div className="flex flex-col gap-2">
                <Link 
                  href={`https://business.google.com/locations?hl=en`}
                  target="_blank" 
                  className="flex items-center gap-2 p-3 bg-default-50 hover:bg-default-100 rounded-lg transition-colors"
                >
                  <Settings className="w-4 h-4 text-default-600" />
                  <div className="flex-1">
                    <p className="font-medium text-sm">{tString('connected.actions.manage.title')}</p>
                    <p className="text-xs text-default-500">{tString('connected.actions.manage.description')}</p>
                  </div>
                  <ExternalLink className="w-3 h-3 text-default-400" />
                </Link>

                <Link 
                  href={currentGoogleInfo.google_review_link} 
                  target="_blank" 
                  className="flex items-center gap-2 p-3 bg-default-50 hover:bg-default-100 rounded-lg transition-colors"
                >
                  <Star className="w-4 h-4 text-default-600" />
                  <div className="flex-1">
                    <p className="font-medium text-sm">{tString('connected.actions.review.title')}</p>
                    <p className="text-xs text-default-500">{tString('connected.actions.review.description')}</p>
                  </div>
                  <ExternalLink className="w-3 h-3 text-default-400" />
                </Link>

                <Link 
                  href={currentGoogleInfo.google_business_url} 
                  target="_blank" 
                  className="flex items-center gap-2 p-3 bg-default-50 hover:bg-default-100 rounded-lg transition-colors"
                >
                  <MapPin className="w-4 h-4 text-default-600" />
                  <div className="flex-1">
                    <p className="font-medium text-sm">{tString('connected.actions.maps.title')}</p>
                    <p className="text-xs text-default-500">{tString('connected.actions.maps.description')}</p>
                  </div>
                  <ExternalLink className="w-3 h-3 text-default-400" />
                </Link>
              </div>
            </div>

            <div className="p-3 bg-success-50 rounded-lg border border-success-200">
              <div className="flex items-center gap-2">
                <Users className="w-4 h-4 text-success-600" />
                <div>
                  <p className="font-medium text-success-900 text-sm">{tString('connected.integration.title')}</p>
                  <p className="text-xs text-success-600">{tString('connected.integration.description')}</p>
                </div>
              </div>
            </div>
          </div>

          <Button
            color="danger"
            variant="light"
            onPress={handleRemoveIntegration}
            isLoading={isUpdating}
            startContent={<X className="w-4 h-4" />}
            size="sm"
          >
            {tString('connected.removeButton')}
          </Button>
        </CardBody>
      </Card>
    );
  }

  return (
    <>
      <Card>
        <CardBody className="p-6">
          <div className="flex items-center gap-2 mb-4">
            <Globe className="w-5 h-5 text-default-500" />
            <h3 className="text-lg font-semibold">{tString('search.title')}</h3>
          </div>

          <p className="text-sm text-default-600 mb-4">
            {tString('search.description')}
          </p>

          <div className="space-y-3 mb-4">
            <Input
              label={tString('search.inputLabel')}
              placeholder={tString('search.inputPlaceholder')}
              value={searchQuery}
              onChange={(e) => handleSearchInputChange(e.target.value)}
              onKeyPress={(e) => e.key === 'Enter' && handleSearch()}
              startContent={<Search className="w-4 h-4 text-default-500" />}
              description={tString('search.inputDescription')}
            />
            
            <Button
              color="primary"
              onPress={handleSearch}
              isLoading={isSearching}
              className="w-full"
            >
              {tString('search.searchButton')}
            </Button>
            
            <div className="text-xs text-default-500 space-y-1">
              <p><strong>{tString('search.tips.title')}</strong></p>
              <ul className="list-disc list-inside space-y-0.5 ml-2">
                <li>{tString('search.tips.tip1')}</li>
                <li>{tString('search.tips.tip2')}</li>
                <li>{tString('search.tips.tip3')}</li>
                <li>{tString('search.tips.tip4')}</li>
              </ul>
            </div>
          </div>

          {searchResults.length > 0 && (
            <>
              <Divider className="my-4" />
              <div className="space-y-3">
                <h4 className="font-medium">{tString('search.selectBusiness')}</h4>
                {searchResults.map(renderBusinessCard)}
              </div>
            </>
          )}

          <div className="mt-4 p-3 bg-blue-50 rounded-lg border border-blue-200">
            <div className="flex items-start gap-2">
              <AlertTriangle className="w-4 h-4 text-blue-600 mt-0.5 flex-shrink-0" />
              <div className="text-sm text-blue-700">
                <p className="font-medium mb-1">{tString('search.noProfile.title')}</p>
                <p>
                  {tString('search.noProfile.description')}{' '}
                  <Link 
                    href="https://business.google.com" 
                    target="_blank" 
                    className="underline"
                  >
                    business.google.com
                  </Link>{' '}
                  {tString('search.noProfile.suffix')}
                </p>
              </div>
            </div>
          </div>
        </CardBody>
      </Card>

      {/* Confirmation Modal */}
      <Modal isOpen={isOpen} onClose={onClose}>
        <ModalContent>
          <ModalHeader>{tString('modal.title')}</ModalHeader>
          <ModalBody>
            {selectedBusiness && (
              <div className="space-y-4">
                <p>{tString('modal.confirmText')}</p>
                
                <Card>
                  <CardBody className="p-4">
                    <h4 className="font-semibold text-lg mb-2">{selectedBusiness.name}</h4>
                    <div className="flex items-start gap-2 mb-2">
                      <MapPin className="w-4 h-4 text-default-500 mt-0.5 flex-shrink-0" />
                      <p className="text-sm text-default-600">{selectedBusiness.formatted_address}</p>
                    </div>
                    {selectedBusiness.rating && (
                      <div className="flex items-center gap-1">
                        <Star className="w-4 h-4 fill-yellow-400 text-yellow-400" />
                        <span className="text-sm font-medium">{selectedBusiness.rating}</span>
                        {selectedBusiness.user_ratings_total && (
                          <span className="text-xs text-default-500">
                            ({selectedBusiness.user_ratings_total} reviews)
                          </span>
                        )}
                      </div>
                    )}
                  </CardBody>
                </Card>

                <p className="text-sm text-default-600">
                  {tString('modal.description')}
                </p>
              </div>
            )}
          </ModalBody>
          <ModalFooter>
            <Button variant="light" onPress={onClose}>
              {tString('modal.cancelButton')}
            </Button>
            <Button
              color="primary"
              onPress={handleConfirmSelection}
              isLoading={isUpdating}
              startContent={<Check className="w-4 h-4" />}
            >
              {tString('modal.confirmButton')}
            </Button>
          </ModalFooter>
        </ModalContent>
      </Modal>
    </>
  );
}
