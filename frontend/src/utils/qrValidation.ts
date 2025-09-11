import { getTableByCode } from '../api/bills';

export interface QRValidationResult {
  isValid: boolean;
  tableCode?: string;
  error?: string;
  tableData?: any;
}

export const validateTableCode = async (code: string): Promise<QRValidationResult> => {
  try {
    // Basic format validation
    if (!code || typeof code !== 'string') {
      return {
        isValid: false,
        error: 'Invalid table code format'
      };
    }

    // Clean and normalize the code
    const cleanCode = code.trim().toUpperCase();
    
    // Check format (alphanumeric, 3-10 characters)
    if (!/^[A-Z0-9]{3,10}$/.test(cleanCode)) {
      return {
        isValid: false,
        error: 'Table code must be 3-10 alphanumeric characters'
      };
    }

    // Verify with backend
    try {
      const tableData = await getTableByCode(cleanCode);
      
      if (!tableData || !tableData.table) {
        return {
          isValid: false,
          error: 'Table not found'
        };
      }

      // Check if table is active
      if (tableData.table.status !== 'active') {
        return {
          isValid: false,
          error: 'Table is not currently active'
        };
      }

      return {
        isValid: true,
        tableCode: cleanCode,
        tableData
      };
    } catch (apiError) {
      return {
        isValid: false,
        error: 'Table not found or inactive'
      };
    }
  } catch (error) {
    console.error('Table validation error:', error);
    return {
      isValid: false,
      error: 'Validation failed. Please try again.'
    };
  }
};

export const extractTableCodeFromURL = (url: string): string | null => {
  try {
    // Handle various URL formats:
    // https://payverge.com/t/ABC123
    // https://app.payverge.com/t/ABC123
    // /t/ABC123
    // ABC123
    
    const urlMatch = url.match(/\/t\/([A-Z0-9]+)/i);
    if (urlMatch) {
      return urlMatch[1].toUpperCase();
    }

    // Direct code format
    const directMatch = url.match(/^([A-Z0-9]{3,10})$/i);
    if (directMatch) {
      return directMatch[1].toUpperCase();
    }

    return null;
  } catch (error) {
    console.error('URL parsing error:', error);
    return null;
  }
};

export const generateTableURL = (tableCode: string): string => {
  const baseUrl = process.env.NEXT_PUBLIC_APP_URL || 'https://payverge.com';
  return `${baseUrl}/t/${tableCode}`;
};
