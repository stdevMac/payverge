// hooks/useApi.ts
import { axiosInstance } from "@/api";
import { destroyCookie } from "nookies";

// Función para obtener la sesión actual
export async function getSession() {
    try {
        const response = await axiosInstance.get("/auth/session");
        if (response.status === 200) {
            return response.data;
        }
    } catch (error) {
        handleApiError(error);
        return false;
    }
}

// Función para obtener el token CSRF
export async function getCsrfToken(data: { address: string | undefined }) {
    try {
        const response = await axiosInstance.post("/auth/challenge", data);
        if (response.status === 200) {
            return response.data.challenge;
        }
    } catch (error) {
        handleApiError(error);
        throw error;
    }
}

// Función para iniciar sesión
export async function signIn(data: { message: string; signature: string }) {
    try {
        const response = await axiosInstance.post("/auth/signin", data);
        return response;
    } catch (error) {
        handleApiError(error);
        throw error;
    }
}

// Función para cerrar sesión
export async function signOutFromSession() {
    try {
        const response = await axiosInstance.post("/auth/signout");
        if (response.status === 200) {
            // Clear all auth-related cookies
            destroyCookie(null, "token", { path: "/" });
            destroyCookie(null, "session_token", { path: "/" });
            destroyCookie(null, "persist-web3-login", { path: "/" });
            return response.data;
        }
    } catch (error) {
        handleApiError(error);
        // Still destroy cookies even if API call fails
        destroyCookie(null, "token", { path: "/" });
        destroyCookie(null, "session_token", { path: "/" });
        destroyCookie(null, "persist-web3-login", { path: "/" });
        throw error;
    }
}

function handleApiError(error: any) {
    if (error.response) {
        console.error("Error de respuesta:", error.response.data);
    } else if (error.request) {
        console.error("Error de solicitud:", error.request);
    } else {
        console.error("Error:", error.message);
    }
}
