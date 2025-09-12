import axios, { AxiosError, InternalAxiosRequestConfig, AxiosResponse } from "axios";
import { parseCookies } from "nookies";
import { sanitizeError } from "@/utils/errorMessages";
import { apiCache } from "@/utils/cache";

interface RetryConfig extends InternalAxiosRequestConfig {
  _retryCount?: number;
  _maxRetries?: number;
  _retryDelay?: number;
  _useCache?: boolean;
  _cacheTTL?: number;
}

const shouldRetry = (error: AxiosError): boolean => {
  // Retry on network errors, timeouts, and 5xx server errors
  if (!error.response) return true; // Network error
  const status = error.response.status;
  return status >= 500 || status === 408 || status === 429; // Server errors, timeout, rate limit
};

const delay = (ms: number): Promise<void> => 
  new Promise(resolve => setTimeout(resolve, ms));

export const axiosInstance = axios.create({
    baseURL: process.env.NEXT_PUBLIC_API_URL,
    timeout: 30000, // Reduced from 1000000 to reasonable 30s
});

axiosInstance.interceptors.request.use(
    (config: RetryConfig) => {
        // Set default retry configuration
        config._retryCount = config._retryCount || 0;
        config._maxRetries = config._maxRetries || 3;
        config._retryDelay = config._retryDelay || 1000;
        
        // Enable caching for GET requests by default
        if (config.method?.toLowerCase() === 'get') {
            config._useCache = config._useCache !== false; // Default to true for GET
            config._cacheTTL = config._cacheTTL || 5 * 60 * 1000; // 5 minutes default
            
            // Check cache for GET requests
            if (config._useCache && config.url) {
                const cachedResponse = apiCache.get(config.url, config.params);
                if (cachedResponse) {
                    // Return the cached response directly
                    return Promise.reject({
                        config,
                        response: cachedResponse,
                        isAxiosError: false,
                        _isCached: true
                    });
                }
            }
        }
        
        const cookies = parseCookies();
        const token = cookies.session_token;
        if (token && config.headers) {
            config.headers["Authorization"] = `Bearer "${token}"`;
        }
        
        return config;
    },
    (error) => {
        return Promise.reject(error);
    },
);

axiosInstance.interceptors.response.use(
    (response: AxiosResponse) => {
        const config = response.config as RetryConfig;
        
        // Cache successful GET responses
        if (config._useCache && config.method?.toLowerCase() === 'get' && config.url) {
            apiCache.set(config.url, response, config.params, config._cacheTTL);
        }
        
        // Clear cache on mutations (POST, PUT, DELETE, PATCH)
        const method = config.method?.toLowerCase();
        if (method && ['post', 'put', 'delete', 'patch'].includes(method)) {
            // Clear related cache entries (simplified approach - clear all)
            apiCache.clear();
        }
        
        return response;
    },
    async (error: any) => {
        // Handle cached responses
        if (error._isCached) {
            return Promise.resolve(error.response);
        }
        
        const config = error.config as RetryConfig;
        
        // Don't retry if no config or if we shouldn't retry this error
        if (!config || !shouldRetry(error)) {
            // Sanitize error message for production
            const sanitized = sanitizeError(error);
            const sanitizedError = new Error(sanitized.message);
            (sanitizedError as any).status = sanitized.status;
            return Promise.reject(sanitizedError);
        }

        // Don't retry if we've exceeded max retries
        if (config._retryCount! >= config._maxRetries!) {
            // Sanitize error message for production
            const sanitized = sanitizeError(error);
            const sanitizedError = new Error(sanitized.message);
            (sanitizedError as any).status = sanitized.status;
            return Promise.reject(sanitizedError);
        }

        // Increment retry count
        config._retryCount = (config._retryCount || 0) + 1;

        // Calculate delay with exponential backoff and jitter
        const baseDelay = config._retryDelay || 1000;
        const exponentialDelay = baseDelay * Math.pow(2, config._retryCount - 1);
        const maxDelay = 10000; // Cap at 10 seconds
        const delayWithJitter = Math.min(exponentialDelay, maxDelay) + Math.random() * 1000;

        // Wait before retrying
        await delay(delayWithJitter);

        // Retry the request
        return axiosInstance(config);
    },
);
