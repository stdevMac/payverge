'use client';

import React, { useState, useEffect, useCallback } from 'react';
import { Input } from '@nextui-org/react';
import { Check, X, Loader2, AlertCircle } from 'lucide-react';
import { checkCustomURLAvailability } from '@/api/business';
import { debounce } from 'lodash';
import { useSimpleLocale, getTranslation } from '@/i18n/SimpleTranslationProvider';

interface CustomURLInputProps {
  value: string;
  onChange: (value: string) => void;
  businessId?: number;
  placeholder?: string;
  label?: string;
  description?: string;
}

const CustomURLInput = React.memo(function CustomURLInput({
  value,
  onChange,
  businessId,
  placeholder = "my-restaurant",
  label = "Custom URL",
  description = "Create a custom URL for your business page"
}: CustomURLInputProps) {
  const { locale: currentLocale } = useSimpleLocale();
  
  // Translation helper
  const tString = useCallback((key: string): string => {
    const fullKey = `customUrlInput.${key}`;
    const result = getTranslation(fullKey, currentLocale);
    return Array.isArray(result) ? result[0] || key : result as string;
  }, [currentLocale]);

  const [isChecking, setIsChecking] = useState(false);
  const [isAvailable, setIsAvailable] = useState<boolean | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [hasChecked, setHasChecked] = useState(false);

  // Function to check URL availability
  const checkURL = useCallback(async (url: string) => {
    if (!url || url.length < 2) {
      setIsAvailable(null);
      setError(null);
      setHasChecked(false);
      setIsChecking(false);
      return;
    }

    try {
      setIsChecking(true);
      setError(null);
      
      const result = await checkCustomURLAvailability(url, businessId);
      
      setIsAvailable(result.available);
      setError(result.error || null);
      setHasChecked(true);
    } catch (err) {
      setError(tString('errors.checkFailed'));
      setIsAvailable(false);
      setHasChecked(true);
    } finally {
      setIsChecking(false);
    }
  }, [businessId, tString]);

  // Debounced function to check URL availability
  // eslint-disable-next-line react-hooks/exhaustive-deps
  const debouncedCheck = useCallback(
    debounce((url: string) => checkURL(url), 500),
    [checkURL]
  );

  // Effect to trigger URL checking when value changes
  useEffect(() => {
    if (value !== undefined) {
      debouncedCheck(value);
    }

    // Cleanup function to cancel pending debounced calls
    return () => {
      debouncedCheck.cancel();
    };
  }, [value, debouncedCheck]);

  // Handle input change
  const handleChange = (newValue: string) => {
    // Clean the input - only allow alphanumeric and hyphens
    const cleanValue = newValue.toLowerCase().replace(/[^a-z0-9-]/g, '');
    onChange(cleanValue);
    
    // Reset states when user is typing
    if (newValue !== value) {
      setHasChecked(false);
      setIsAvailable(null);
      setError(null);
    }
  };

  // Get the appropriate end content (icon)
  const getEndContent = () => {
    if (isChecking) {
      return <Loader2 className="w-4 h-4 animate-spin text-default-400" />;
    }

    if (!hasChecked || !value || value.length < 2) {
      return null;
    }

    if (error || !isAvailable) {
      return <X className="w-4 h-4 text-danger" />;
    }

    if (isAvailable) {
      return <Check className="w-4 h-4 text-success" />;
    }

    return null;
  };

  // Get input color based on validation state
  const getColor = () => {
    if (!hasChecked || !value || value.length < 2) {
      return 'default';
    }

    if (error || !isAvailable) {
      return 'danger';
    }

    if (isAvailable) {
      return 'success';
    }

    return 'default';
  };

  // Get helper text
  const getHelperText = () => {
    if (isChecking) {
      return tString('status.checking');
    }

    if (value && value.length < 2) {
      return tString('validation.minLength');
    }

    if (error) {
      return error;
    }

    if (hasChecked && isAvailable) {
      return tString('status.available').replace('{url}', `payverge.com/b/${value}`);
    }

    if (hasChecked && !isAvailable) {
      return tString('status.taken');
    }

    return description;
  };

  // Get helper text color
  const getHelperColor = () => {
    if (isChecking) {
      return 'text-default-500';
    }

    if (value && value.length < 2) {
      return 'text-warning';
    }

    if (error || (hasChecked && !isAvailable)) {
      return 'text-danger';
    }

    if (hasChecked && isAvailable) {
      return 'text-success';
    }

    return 'text-default-500';
  };

  return (
    <div className="space-y-2">
      <Input
        label={label}
        placeholder={placeholder}
        value={value}
        onValueChange={handleChange}
        color={getColor() as any}
        startContent={
          <div className="flex items-center gap-2 text-default-400">
            <span className="text-sm">payverge.com/b/</span>
          </div>
        }
        endContent={getEndContent()}
        classNames={{
          input: "text-sm",
          inputWrapper: "h-12"
        }}
      />
      
      <div className={`text-xs ${getHelperColor()} flex items-center gap-1`}>
        {(value && value.length < 2) && (
          <AlertCircle className="w-3 h-3" />
        )}
        <span>{getHelperText()}</span>
      </div>

      {/* URL Preview */}
      {value && value.length >= 2 && hasChecked && isAvailable && (
        <div className="bg-success-50 border border-success-200 rounded-lg p-3">
          <div className="flex items-center gap-2">
            <Check className="w-4 h-4 text-success-600" />
            <div>
              <p className="text-sm font-medium text-success-800">
                {tString('preview.message')}
              </p>
              <p className="text-sm text-success-600 font-mono">
                https://payverge.com/b/{value}
              </p>
            </div>
          </div>
        </div>
      )}
    </div>
  );
});

export default CustomURLInput;
