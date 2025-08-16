import { axiosInstance } from "@/api";
import { UpdateUserInterface } from "@/interface";
import axios, { AxiosError } from "axios";

// Tiempo de espera global para todas las solicitudes de axios
axios.defaults.timeout = 15000;

export interface ClaimRewardsOfFleetInterface {
    address: string;
    fleet_id: string;
}
export interface ClaimRewardsOfReferralsInterface {
    address: string;
    fleet_id: string;
    amount: number;
}

export const claimRewardsOfFleet = async (
    data: ClaimRewardsOfFleetInterface,
): Promise<boolean> => {
    try {
        // Realiza la solicitud PUT para actualizar el usuario
        const response = await axiosInstance.post<ClaimRewardsOfFleetInterface>(
            `/inside/rewards_per_fleet`,
            data,
        );

        // Verifica si la respuesta tiene el código de estado 200 (OK)
        if (response.status === 200) {
            return true;
        } else {

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

export const claimRewardsOfReferrals = async (
    data: ClaimRewardsOfReferralsInterface,
): Promise<boolean> => {
    try {
        // Realiza la solicitud PUT para actualizar el usuario
        const response = await axiosInstance.post(
            `/inside/rewards_from_referrals`,
            data,
        );

        // Verifica si la respuesta tiene el código de estado 200 (OK)
        if (response.status === 200) {
            return true;
        } else {
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
