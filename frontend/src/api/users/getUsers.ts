import { axiosInstance } from "@/api";
import { FullUserInterface, FullUsersInterfaceResponse } from "@/interface";
import axios, { AxiosError } from "axios";

// Tiempo de espera global para todas las solicitudes de axios
axios.defaults.timeout = 15000;

// Función para obtener todos los usuarios
export const getAllUsers = async (): Promise<FullUsersInterfaceResponse> => {
    try {
        const response = await axiosInstance.get<FullUsersInterfaceResponse>(
            "/admin/get_all_users",
        );


        // Verifica si la respuesta tiene el código de estado 200 (OK)
        if (response.status === 200) {
            return response.data;
        } else {
            return { users: [] };
        }
    } catch (error: unknown) {
        // Manejo de errores específicos de Axios
        if (axios.isAxiosError(error)) {
            const axiosError = error as AxiosError;
            if (axiosError.response) {
            } else if (axiosError.request) {
            } else {
            }
        } else {
            // Manejo de errores desconocidos
        }
        throw error;
    }
};
