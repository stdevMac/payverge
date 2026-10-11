import { axiosInstance } from "../tools/instance";
import { UpdateUserInterface } from "@/interface";
import axios from "axios";
import { logError } from '@/utils/errorLogger';
import { getApiErrorData, isApiNetworkError } from "@/utils/apiError";

// Tiempo de espera global para todas las solicitudes de axios
axios.defaults.timeout = 15000;

// Función para actualizar un usuario
export const updateUser = async (
    data: UpdateUserInterface,
): Promise<boolean> => {
    try {
        // Realiza la solicitud PUT para actualizar el usuario
        const response = await axiosInstance.put<UpdateUserInterface>(
            `/inside/update_user`,
            data,
        );

        // Verifica si la respuesta tiene el código de estado 200 (OK)
        if (response.status === 200) {
            return true;
        } else {
            return false;
        }
    } catch (error: unknown) {
        void logError(error instanceof Error ? error : String(error), 'updateUser', 'updateUser');
        const data = getApiErrorData(error);
        if (data !== undefined) {
            console.error("Error de respuesta:", data);
        } else if (isApiNetworkError(error)) {
            console.error("Error de solicitud (sin respuesta)");
        } else {
            console.error("Error:", error instanceof Error ? error.message : error);
        }

        // En caso de error, retorna false
        return false;
    }
};
