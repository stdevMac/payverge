import axios from "axios";
import { parseCookies } from "nookies";

// Debug logging to trace the actual API URL being used
console.log("🔍 NEXT_PUBLIC_API_URL:", process.env.NEXT_PUBLIC_API_URL);
console.log("🔍 All NEXT_PUBLIC env vars:", Object.keys(process.env).filter(key => key.startsWith('NEXT_PUBLIC_')));

export const axiosInstance = axios.create({
    baseURL: process.env.NEXT_PUBLIC_API_URL,
    timeout: 1000000,
});

axiosInstance.interceptors.request.use(
    (config) => {
        // Debug logging for each request
        console.log("🚀 Making request to:", (config.baseURL || '') + (config.url || ''));
        console.log("🚀 Full config:", { baseURL: config.baseURL, url: config.url, method: config.method });
        
        const cookies = parseCookies();
        const token = cookies.session_token;
        if (token) {
            // Add quotes around the token as expected by the backend
            config.headers["Authorization"] = `Bearer "${token}"`;
        }
        return config;
    },
    (error) => {
        console.error("Request error:", error);
        return Promise.reject(error);
    },
);

axiosInstance.interceptors.response.use(
    (response) => response,
    (error) => {
        if (error.response?.status === 401) {
            // Let components handle 401s
            console.error("Authentication error:", error.response.data);
        }
        return Promise.reject(error);
    },
);
