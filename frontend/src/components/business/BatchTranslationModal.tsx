'use client';

import React, { useState, useEffect } from 'react';
import {
  Modal,
  ModalContent,
  ModalHeader,
  ModalBody,
  ModalFooter,
  Button,
  Progress,
  Chip,
  Card,
  CardBody,
} from '@nextui-org/react';
import { Globe, CheckCircle, Clock, AlertCircle } from 'lucide-react';
import { translateEntireMenu, getTranslationStatus, TranslationStatus } from '../../api/currency';

interface BatchTranslationModalProps {
  isOpen: boolean;
  onClose: () => void;
  businessId: number;
  selectedLanguages: string[];
  supportedLanguages: Array<{ code: string; name: string; native_name: string }>;
  onTranslationComplete: () => void;
}

export default function BatchTranslationModal({
  isOpen,
  onClose,
  businessId,
  selectedLanguages,
  supportedLanguages,
  onTranslationComplete
}: BatchTranslationModalProps) {
  const [isTranslating, setIsTranslating] = useState(false);
  const [translationStatus, setTranslationStatus] = useState<TranslationStatus | null>(null);
  const [error, setError] = useState<string | null>(null);

  const startTranslation = async () => {
    try {
      setIsTranslating(true);
      setError(null);
      
      const response = await translateEntireMenu(businessId, selectedLanguages);
      
      // Start polling for status
      pollTranslationStatus(response.job_id);
      
    } catch (err) {
      console.error('Failed to start translation:', err);
      setError('Failed to start translation. Please try again.');
      setIsTranslating(false);
    }
  };

  const pollTranslationStatus = async (jobId: string) => {
    const maxAttempts = 30; // 30 seconds max
    let attempts = 0;

    const poll = async () => {
      try {
        const status = await getTranslationStatus(jobId);
        setTranslationStatus(status);

        if (status.status === 'completed') {
          setIsTranslating(false);
          setTimeout(() => {
            onTranslationComplete();
            onClose();
          }, 2000); // Show success for 2 seconds
        } else if (status.status === 'failed') {
          setError('Translation failed. Please try again.');
          setIsTranslating(false);
        } else if (attempts < maxAttempts) {
          attempts++;
          setTimeout(poll, 1000); // Poll every second
        } else {
          setError('Translation is taking longer than expected. Please refresh and check again.');
          setIsTranslating(false);
        }
      } catch (err) {
        console.error('Failed to get translation status:', err);
        if (attempts < maxAttempts) {
          attempts++;
          setTimeout(poll, 1000);
        } else {
          setError('Failed to check translation status.');
          setIsTranslating(false);
        }
      }
    };

    poll();
  };

  const getLanguageNames = () => {
    return selectedLanguages.map(code => {
      const lang = supportedLanguages.find(l => l.code === code);
      return lang ? lang.native_name : code;
    }).join(', ');
  };

  const getStatusIcon = () => {
    if (error) return <AlertCircle className="w-5 h-5 text-danger" />;
    if (translationStatus?.status === 'completed') return <CheckCircle className="w-5 h-5 text-success" />;
    return <Clock className="w-5 h-5 text-primary" />;
  };

  const getStatusColor = () => {
    if (error) return 'danger';
    if (translationStatus?.status === 'completed') return 'success';
    return 'primary';
  };

  // Reset state when modal opens/closes
  useEffect(() => {
    if (!isOpen) {
      setIsTranslating(false);
      setTranslationStatus(null);
      setError(null);
    }
  }, [isOpen]);

  return (
    <Modal isOpen={isOpen} onClose={onClose} size="lg" isDismissable={!isTranslating}>
      <ModalContent>
        <ModalHeader className="flex items-center gap-2">
          <Globe className="w-5 h-5" />
          Translate Menu
        </ModalHeader>
        
        <ModalBody>
          {!isTranslating && !translationStatus && (
            <div className="space-y-4">
              <Card>
                <CardBody>
                  <div className="space-y-3">
                    <h3 className="text-lg font-semibold">Ready to translate your menu?</h3>
                    <p className="text-gray-600">
                      This will automatically translate all your menu categories and items into the selected languages.
                    </p>
                    
                    <div className="space-y-2">
                      <p className="text-sm font-medium">Target Languages:</p>
                      <div className="flex flex-wrap gap-2">
                        {selectedLanguages.map(code => {
                          const lang = supportedLanguages.find(l => l.code === code);
                          return lang ? (
                            <Chip key={code} size="sm" variant="flat" color="primary">
                              {lang.native_name}
                            </Chip>
                          ) : null;
                        })}
                      </div>
                    </div>

                    <div className="bg-blue-50 p-3 rounded-lg">
                      <p className="text-sm text-blue-800">
                        <strong>Note:</strong> This process will create translations for all existing menu content. 
                        You can always edit the translations manually later.
                      </p>
                    </div>
                  </div>
                </CardBody>
              </Card>
            </div>
          )}

          {(isTranslating || translationStatus) && (
            <div className="space-y-4">
              <Card>
                <CardBody>
                  <div className="space-y-4">
                    <div className="flex items-center gap-3">
                      {getStatusIcon()}
                      <div>
                        <h3 className="font-semibold">
                          {error ? 'Translation Failed' : 
                           translationStatus?.status === 'completed' ? 'Translation Complete!' :
                           'Translating Menu...'}
                        </h3>
                        <p className="text-sm text-gray-600">
                          {error ? error :
                           translationStatus?.status === 'completed' ? 'All content has been successfully translated.' :
                           `Translating to: ${getLanguageNames()}`}
                        </p>
                      </div>
                    </div>

                    {!error && translationStatus && (
                      <Progress
                        value={translationStatus.progress}
                        color={getStatusColor()}
                        showValueLabel
                        className="w-full"
                      />
                    )}

                    {isTranslating && !error && (
                      <div className="flex items-center justify-center py-4">
                        <div className="animate-spin rounded-full h-8 w-8 border-b-2 border-primary"></div>
                      </div>
                    )}

                    {translationStatus?.status === 'completed' && (
                      <div className="bg-green-50 p-3 rounded-lg">
                        <p className="text-sm text-green-800">
                          ✨ Your menu is now available in multiple languages! 
                          Use the &quot;View Menu In&quot; selector to see the translations.
                        </p>
                      </div>
                    )}
                  </div>
                </CardBody>
              </Card>
            </div>
          )}
        </ModalBody>

        <ModalFooter>
          {!isTranslating && !translationStatus && (
            <>
              <Button variant="light" onPress={onClose}>
                Cancel
              </Button>
              <Button 
                color="primary" 
                onPress={startTranslation}
                isDisabled={selectedLanguages.length === 0}
              >
                Start Translation
              </Button>
            </>
          )}

          {(isTranslating || translationStatus) && (
            <Button 
              variant="light" 
              onPress={onClose}
              isDisabled={isTranslating}
            >
              {translationStatus?.status === 'completed' ? 'Done' : 'Close'}
            </Button>
          )}
        </ModalFooter>
      </ModalContent>
    </Modal>
  );
}
