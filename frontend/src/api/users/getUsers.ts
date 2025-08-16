import { axiosInstance } from "@/api";
import { FullUserInterface, FullUsersInterfaceResponse } from "@/interface";
import axios, { AxiosError } from "axios";

// Tiempo de espera global para todas las solicitudes de axios
axios.defaults.timeout = 15000;

// Función para obtener todos los usuarios
export const getAllUsers = async (): Promise<FullUsersInterfaceResponse> => {
    try {
        console.log("Request to get all users");
        const response = await axiosInstance.get<FullUsersInterfaceResponse>(
            "/admin/get_all_users",
        );

        console.log("Response: ", response);

        // Verifica si la respuesta tiene el código de estado 200 (OK)
        if (response.status === 200) {
            return response.data;
        } else {
            console.log(
                "Error en la respuesta: Código de estado no esperado",
                response.status,
            );
            return { users: [] };
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
        throw error;
    }
};
