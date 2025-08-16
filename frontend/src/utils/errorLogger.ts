import axios from 'axios';

interface ErrorLogPayload {
  timestamp: string;
  error: string;
  component: string;
  function: string;
  additionalInfo?: Record<string, any>;
}

export const logError = async (
  error: Error | string,
  component: string,
  functionName: string,
  additionalInfo?: Record<string, any>
) => {
  const errorPayload: ErrorLogPayload = {
    timestamp: new Date().toISOString(),
    error: error instanceof Error ? error.message : error,
    component,
    function: functionName,
    additionalInfo,
  };

  try {
    // Send error to backend logging endpoint
    await axios.post(
      `${process.env.NEXT_PUBLIC_API_URL}/logs/error`,
      errorPayload
    );

    // Also log to console in development
    if (process.env.NEXT_PUBLIC_ENV === 'development') {
      console.error('Error logged:', errorPayload);
    }
  } catch (loggingError) {
    // Fallback to console if logging fails
    console.error('Failed to log error:', errorPayload);
    console.error('Logging error:', loggingError);
  }
};
