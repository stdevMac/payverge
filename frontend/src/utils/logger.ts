const isDevelopment = process.env.NODE_ENV === 'development';

export const logger = {
  log: (...args: unknown[]) => {
    if (isDevelopment) {
      console.log(...args); // eslint-disable-line no-console
    }
  },

  warn: (...args: unknown[]) => {
    if (isDevelopment) {
      console.warn(...args);
    }
  },

  error: (...args: unknown[]) => {
    // Always log errors, even in production
    console.error(...args);
  },

  debug: (...args: unknown[]) => {
    if (isDevelopment) {
      console.debug(...args); // eslint-disable-line no-console
    }
  },

  // For payment/blockchain operations - dev only
  payment: (...args: unknown[]) => {
    if (isDevelopment) {
      console.log('[Payment]', ...args); // eslint-disable-line no-console
    }
  },

  // For API operations - only in development
  api: (...args: unknown[]) => {
    if (isDevelopment) {
      console.log('[API]', ...args); // eslint-disable-line no-console
    }
  },
};
