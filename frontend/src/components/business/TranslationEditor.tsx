'use client';

import React, { useState, useEffect } from 'react';
import {
  Card,
  CardBody,
  Input,
  Textarea,
  Button,
  Chip,
  Accordion,
  AccordionItem,
  Spinner,
} from '@nextui-org/react';
import { Globe, Edit3, Save, RotateCcw, CheckCircle, AlertCircle } from 'lucide-react';
import { 
  getMenuItemTranslations, 
  getCategoryTranslations,
  saveMenuItemTranslation,
  saveCategoryTranslation,
  SupportedLanguage 
} from '../../api/currency';

interface TranslationEditorProps {
  entityType: 'menu_item' | 'category';
  entityId: number;
  originalName: string;
  originalDescription: string;
  businessId: number;
  languages: SupportedLanguage[];
  selectedLanguageCodes: string[];
  defaultLanguageCode: string;
  onTranslationUpdate?: () => void;
}

interface TranslationData {
  name: string;
  description: string;
  isEditing: boolean;
  hasChanges: boolean;
  isSaving: boolean;
}

export default function TranslationEditor({
  entityType,
  entityId,
  originalName,
  originalDescription,
  businessId,
  languages,
  selectedLanguageCodes,
  defaultLanguageCode,
  onTranslationUpdate
}: TranslationEditorProps) {
  const [translations, setTranslations] = useState<Record<string, TranslationData>>({});
  const [isLoading, setIsLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  // Filter out the default language from translations (show original content)
  const translationLanguages = selectedLanguageCodes.filter(code => code !== defaultLanguageCode);

  // Load translations
  useEffect(() => {
    if (translationLanguages.length === 0 || !entityId) return;

    const loadTranslations = async () => {
      try {
        setIsLoading(true);
        setError(null);

        const translationData = entityType === 'menu_item' 
          ? await getMenuItemTranslations(businessId, entityId, translationLanguages)
          : await getCategoryTranslations(businessId, entityId, translationLanguages);

        const formattedTranslations: Record<string, TranslationData> = {};
        
        translationLanguages.forEach(langCode => {
          const data = translationData[langCode] || { name: '', description: '' };
          formattedTranslations[langCode] = {
            name: data.name,
            description: data.description,
            isEditing: false,
            hasChanges: false,
            isSaving: false
          };
        });

        setTranslations(formattedTranslations);
      } catch (error) {
        console.error('Failed to load translations:', error);
        setError('Failed to load translations');
      } finally {
        setIsLoading(false);
      }
    };

    loadTranslations();
  }, [entityType, entityId, businessId, translationLanguages]);

  const handleFieldChange = (langCode: string, field: 'name' | 'description', value: string) => {
    setTranslations(prev => ({
      ...prev,
      [langCode]: {
        ...prev[langCode],
        [field]: value,
        hasChanges: true
      }
    }));
  };

  const handleSaveTranslation = async (langCode: string) => {
    const translation = translations[langCode];
    if (!translation || !translation.hasChanges) return;

    try {
      setTranslations(prev => ({
        ...prev,
        [langCode]: { ...prev[langCode], isSaving: true }
      }));

      const saveFunction = entityType === 'menu_item' ? saveMenuItemTranslation : saveCategoryTranslation;
      
      // Save both name and description if they have content
      if (translation.name.trim()) {
        await saveFunction(entityId, 'name', langCode, translation.name, originalName);
      }
      if (translation.description.trim()) {
        await saveFunction(entityId, 'description', langCode, translation.description, originalDescription);
      }

      setTranslations(prev => ({
        ...prev,
        [langCode]: {
          ...prev[langCode],
          hasChanges: false,
          isEditing: false,
          isSaving: false
        }
      }));

      onTranslationUpdate?.();
    } catch (error) {
      console.error('Failed to save translation:', error);
      setError('Failed to save translation');
      setTranslations(prev => ({
        ...prev,
        [langCode]: { ...prev[langCode], isSaving: false }
      }));
    }
  };

  const handleResetTranslation = (langCode: string) => {
    // Reset to original loaded values
    setTranslations(prev => ({
      ...prev,
      [langCode]: {
        ...prev[langCode],
        hasChanges: false,
        isEditing: false
      }
    }));
  };

  const toggleEditing = (langCode: string) => {
    setTranslations(prev => ({
      ...prev,
      [langCode]: {
        ...prev[langCode],
        isEditing: !prev[langCode].isEditing
      }
    }));
  };

  if (translationLanguages.length === 0) {
    return null;
  }

  if (isLoading) {
    return (
      <Card className="border-gray-200">
        <CardBody className="p-4">
          <div className="flex items-center gap-2">
            <Spinner size="sm" />
            <span className="text-sm text-gray-600">Loading translations...</span>
          </div>
        </CardBody>
      </Card>
    );
  }

  return (
    <Card className="border-gray-200">
      <CardBody className="p-4">
        <div className="flex items-center gap-2 mb-4">
          <Globe className="w-4 h-4 text-gray-600" />
          <span className="text-sm font-medium text-gray-700">Translations</span>
          <Chip size="sm" variant="bordered" className="text-xs">
            {translationLanguages.length} languages
          </Chip>
        </div>

        {error && (
          <div className="mb-4 p-3 bg-red-50 border border-red-200 rounded-lg flex items-center gap-2">
            <AlertCircle className="w-4 h-4 text-red-500" />
            <span className="text-sm text-red-700">{error}</span>
          </div>
        )}

        <Accordion variant="bordered" className="px-0">
          {translationLanguages.map(langCode => {
            const language = languages.find(l => l.code === langCode);
            const translation = translations[langCode];
            
            if (!language || !translation) return null;

            const hasContent = translation.name.trim() || translation.description.trim();
            const isComplete = translation.name.trim() && translation.description.trim();

            return (
              <AccordionItem
                key={langCode}
                aria-label={`${language.native_name} translation`}
                title={
                  <div className="flex items-center justify-between w-full">
                    <div className="flex items-center gap-2">
                      <span className="font-medium">{language.native_name}</span>
                      <span className="text-sm text-gray-500">({language.name})</span>
                    </div>
                    <div className="flex items-center gap-2">
                      {translation.hasChanges && (
                        <Chip size="sm" color="warning" variant="flat">
                          Unsaved
                        </Chip>
                      )}
                      {hasContent && !translation.hasChanges && (
                        <Chip 
                          size="sm" 
                          color={isComplete ? "success" : "primary"} 
                          variant="flat"
                        >
                          {isComplete ? "Complete" : "Partial"}
                        </Chip>
                      )}
                      {!hasContent && (
                        <Chip size="sm" color="default" variant="flat">
                          Not translated
                        </Chip>
                      )}
                    </div>
                  </div>
                }
              >
                <div className="space-y-4 pt-2">
                  {/* Original Content Reference */}
                  <div className="p-3 bg-gray-50 rounded-lg border-l-4 border-gray-300">
                    <div className="text-xs font-medium text-gray-500 mb-2">
                      Original ({languages.find(l => l.code === defaultLanguageCode)?.native_name}):
                    </div>
                    <div className="space-y-1">
                      <div className="text-sm font-medium text-gray-700">{originalName}</div>
                      {originalDescription && (
                        <div className="text-sm text-gray-600">{originalDescription}</div>
                      )}
                    </div>
                  </div>

                  {/* Translation Fields */}
                  <div className="space-y-3">
                    <div>
                      <label className="text-sm font-medium text-gray-700 mb-1 block">
                        Name Translation
                      </label>
                      <Input
                        value={translation.name}
                        onChange={(e) => handleFieldChange(langCode, 'name', e.target.value)}
                        placeholder={`Enter ${language.native_name} name...`}
                        size="sm"
                        variant="bordered"
                      />
                    </div>

                    <div>
                      <label className="text-sm font-medium text-gray-700 mb-1 block">
                        Description Translation
                      </label>
                      <Textarea
                        value={translation.description}
                        onChange={(e) => handleFieldChange(langCode, 'description', e.target.value)}
                        placeholder={`Enter ${language.native_name} description...`}
                        size="sm"
                        variant="bordered"
                        minRows={2}
                        maxRows={4}
                      />
                    </div>
                  </div>

                  {/* Action Buttons */}
                  <div className="flex items-center gap-2 pt-2">
                    <Button
                      size="sm"
                      color="primary"
                      onPress={() => handleSaveTranslation(langCode)}
                      isDisabled={!translation.hasChanges}
                      isLoading={translation.isSaving}
                      startContent={!translation.isSaving && <Save className="w-3 h-3" />}
                    >
                      Save Translation
                    </Button>
                    
                    {translation.hasChanges && (
                      <Button
                        size="sm"
                        variant="bordered"
                        onPress={() => handleResetTranslation(langCode)}
                        startContent={<RotateCcw className="w-3 h-3" />}
                      >
                        Reset
                      </Button>
                    )}
                  </div>
                </div>
              </AccordionItem>
            );
          })}
        </Accordion>
      </CardBody>
    </Card>
  );
}
