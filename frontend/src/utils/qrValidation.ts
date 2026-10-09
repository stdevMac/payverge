import { getTableByCode } from '../api/bills';

// Stable, locale-independent reason codes. The /scan screen (21-locale guest
// entry) maps these to translated copy — the English `error` strings are for
// logging only and must not be shown to guests.
type QRValidationErrorCode =
  | 'format'
  | 'notFound'
  | 'inactive'
  | 'failed';

export interface QRValidationResult {
  isValid: boolean;
  tableCode?: string;
  error?: string;
  errorCode?: QRValidationErrorCode;
  tableData?: any;
}

/** Demo + production table codes are slug-like: demo-50-core-table-01, T1, ABC123. */
const TABLE_CODE_RE = /^[A-Z0-9][A-Z0-9_-]{1,62}$/;

const normalizeTableCode = (code: string): string =>
  code.trim().toUpperCase();

export const isValidTableCodeFormat = (code: string): boolean =>
  TABLE_CODE_RE.test(normalizeTableCode(code));

export const validateTableCode = async (code: string): Promise<QRValidationResult> => {
  try {
    // Basic format validation
    if (!code?.trim()) {
      return {
        isValid: false,
        errorCode: 'format',
        error: 'Invalid table code format'
      };
    }

    // Clean and normalize the code (preserve hyphens/underscores used by demos)
    const cleanCode = normalizeTableCode(code);

    // Reject only clearly garbage input; length/charset must match live codes
    // like demo-50-core-table-01 (~22 chars with hyphens), not the legacy
    // 3–10 alphanumeric-only gate that rejected every demo table.
    if (!TABLE_CODE_RE.test(cleanCode)) {
      return {
        isValid: false,
        errorCode: 'format',
        error: 'Table code must be 2-63 alphanumeric characters (hyphens/underscores allowed)'
      };
    }

    // Verify with backend
    try {
      const tableData = await getTableByCode(cleanCode);

      if (!tableData || !tableData.table) {
        return {
          isValid: false,
          errorCode: 'notFound',
          error: 'Table not found'
        };
      }

      // A successful /guest/table/:code response is ALREADY an active table:
      // GetActiveTableWithBusinessByCode filters `is_active = true` server-side,
      // so a 200 IS an active table. The public payload has no `status` field
      // (the Table model has no such column), so the old `status !== 'active'`
      // check compared undefined to 'active' and rejected EVERY valid code —
      // breaking the entire in-app scan + manual-entry flow. Guard on the field
      // the backend actually sends (`is_active`), defensively, instead.
      if (tableData.table.is_active === false) {
        return {
          isValid: false,
          errorCode: 'inactive',
          error: 'Table is not currently active'
        };
      }

      // Prefer the canonical code from the API when present so /t/ routing
      // matches whatever the backend stored (case/hyphen form).
      const canonical =
        (typeof tableData.table.table_code === "string" &&
          tableData.table.table_code.trim()) ||
        cleanCode;

      return {
        isValid: true,
        tableCode: canonical,
        tableData
      };
    } catch (apiError) {
      return {
        isValid: false,
        errorCode: 'notFound',
        error: 'Table not found or inactive'
      };
    }
  } catch (error) {
    console.error('Table validation error:', error);
    return {
      isValid: false,
      errorCode: 'failed',
      error: 'Validation failed. Please try again.'
    };
  }
};

export const extractTableCodeFromURL = (url: string): string | null => {
  try {
    // Handle various URL formats:
    // https://pos.example.com/t/ABC123
    // https://pos.example.com/t/demo-50-core-table-01
    // /t/ABC123
    // ABC123
    
    const urlMatch = url.match(
      /\/t\/([A-Za-z0-9][A-Za-z0-9_-]{0,62})(?:[/?#]|$)/i,
    );
    if (urlMatch) {
      return urlMatch[1].toUpperCase();
    }

    // Direct code format (slug-friendly)
    const directMatch = url.match(/^([A-Za-z0-9][A-Za-z0-9_-]{1,62})$/);
    if (directMatch) {
      return directMatch[1].toUpperCase();
    }

    return null;
  } catch (error) {
    console.error('URL parsing error:', error);
    return null;
  }
};
