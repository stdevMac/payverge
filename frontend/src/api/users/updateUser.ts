import { axiosInstance } from "@/api";
import { UpdateUserInterface } from "@/interface";
import axios, { AxiosError } from "axios";

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
            console.log(
                "Error en la respuesta: Código de estado no esperado",
                response.status,
            );
            return false;
        }
    } catch (error: unknown) {
        // Manejo de errores específicos de Axios
        if (axios.isAxiosError(error)) {
            const axiosError = error as AxiosError;
            if (axiosError.response) {
                console.error("Error de respuesta:", axiosError.response.data);
            } else if (axiosError.request) {
                console.error("Error de solicitud:", axiosError.request);
            } else {
                console.error("Error:", axiosError.message);
            }
        } else {
            // Manejo de errores desconocidos
            console.error("Error desconocido:", error);
        }

        // En caso de error, retorna false
        return false;
    }
};
