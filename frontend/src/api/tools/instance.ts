import axios from "axios";
import { parseCookies } from "nookies";

export const axiosInstance = axios.create({
    baseURL: process.env.NEXT_PUBLIC_API_URL,
    timeout: 1000000,
});

axiosInstance.interceptors.request.use(
    (config) => {
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
