import toast from "react-hot-toast";
import { axiosInstance } from "../tools/instance";
import { logError } from '@/utils/errorLogger';
import { getApiErrorData, getApiErrorStatus, isApiNetworkError } from "@/utils/apiError";

export async function signOutFromSession() {
    try {
        const response = await axiosInstance.post("/auth/signout");
        if (response.status === 200) {
            // Backend clears httpOnly cookies on signout
            return response.data;
        }
    } catch (error) {
        handleApiError(error, 'signOut');
        // Signing out is security-critical: notify the user that the server
        // couldn't confirm revocation so they can retry or close the browser.
        toast.error(
            "We signed you out locally, but the server couldn't confirm. Close your browser if you're on a shared device.",
            { duration: 8000 },
        );
        // Backend clears httpOnly cookies even on error
        throw error;
    }
}

function handleApiError(error: unknown, source: string) {
    void logError(error instanceof Error ? error : String(error), 'useApi', `handleApiError:${source}`);

    const data = getApiErrorData(error);
    if (data !== undefined) {
        const status = getApiErrorStatus(error);
        if (process.env.NODE_ENV === "development") {
            console.error(`[${source}] response error (status ${status ?? "?"}):`, data);
        } else {
            // Production: log a stable message + status only; never dump the
            // raw response body (Q-4). logError() above already forwarded the
            // sanitized error to the server/Sentry.
            console.error(`[${source}] response error (status ${status ?? "?"})`);
        }
        return;
    }
    if (isApiNetworkError(error)) {
        console.error(`[${source}] request error (no response)`);
        return;
    }

    if (error instanceof Error) {
        console.error(`[${source}] error:`, error.message);
        return;
    }

    console.error(`[${source}] error:`, error);
}
