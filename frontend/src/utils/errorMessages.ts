import { AxiosError } from 'axios';

// Generic error messages for production
const GENERIC_MESSAGES = {
  NETWORK_ERROR: 'Unable to connect to the server. Please check your internet connection and try again.',
  SERVER_ERROR: 'Something went wrong on our end. Please try again later.',
  TIMEOUT_ERROR: 'The request took too long. Please try again.',
  RATE_LIMIT_ERROR: 'Too many requests. Please wait a moment and try again.',
  VALIDATION_ERROR: 'Please check your input and try again.',
  AUTHENTICATION_ERROR: 'Please sign in to continue.',
  AUTHORIZATION_ERROR: 'You do not have permission to perform this action.',
  NOT_FOUND_ERROR: 'The requested resource was not found.',
  CONFLICT_ERROR: 'This action conflicts with existing data. Please refresh and try again.',
  DEFAULT_ERROR: 'An unexpected error occurred. Please try again.'
};

export function getGenericErrorMessage(error: any): string {
  // For development, return detailed errors
  if (process.env.NODE_ENV === 'development') {
    if (error?.response?.data?.message) return error.response.data.message;
    if (error?.response?.data?.error) return error.response.data.error;
    if (error?.message) return error.message;
    return String(error);
  }

  // For production, return generic messages based on error type
  if (!error.response) {
    return GENERIC_MESSAGES.NETWORK_ERROR;
  }

  const status = error.response.status;
  
  switch (status) {
    case 400:
      return GENERIC_MESSAGES.VALIDATION_ERROR;
    case 401:
      return GENERIC_MESSAGES.AUTHENTICATION_ERROR;
    case 403:
      return GENERIC_MESSAGES.AUTHORIZATION_ERROR;
    case 404:
      return GENERIC_MESSAGES.NOT_FOUND_ERROR;
    case 408:
      return GENERIC_MESSAGES.TIMEOUT_ERROR;
    case 409:
      return GENERIC_MESSAGES.CONFLICT_ERROR;
    case 429:
      return GENERIC_MESSAGES.RATE_LIMIT_ERROR;
    case 500:
    case 502:
    case 503:
    case 504:
      return GENERIC_MESSAGES.SERVER_ERROR;
    default:
      return GENERIC_MESSAGES.DEFAULT_ERROR;
  }
}

export function sanitizeError(error: any): { message: string; status?: number } {
  const message = getGenericErrorMessage(error);
  const status = error?.response?.status;
  
  return { message, status };
}
