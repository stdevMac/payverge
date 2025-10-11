// Conditional logging utility for development vs production
const isDevelopment = process.env.NODE_ENV === 'development';

export const logger = {
  log: (...args: any[]) => {
    if (isDevelopment) {
      console.log(...args);
    }
  },
  
  warn: (...args: any[]) => {
    if (isDevelopment) {
      console.warn(...args);
    }
  },
  
  error: (...args: any[]) => {
    // Always log errors, even in production
    console.error(...args);
  },
  
  debug: (...args: any[]) => {
    if (isDevelopment) {
      console.debug(...args);
    }
  },
  
  // For payment/blockchain operations - always log for debugging
  payment: (...args: any[]) => {
    console.log('[PAYMENT]', ...args);
  },
  
  // For API operations - only in development
  api: (...args: any[]) => {
    if (isDevelopment) {
      console.log('[API]', ...args);
    }
  }
};

// Legacy support - can be used to quickly replace console.log
export const devLog = logger.log;
export const devWarn = logger.warn;
export const devError = logger.error;
