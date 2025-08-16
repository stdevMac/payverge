import { axiosInstance } from "@/api";
import { UserInterface } from "@/interface";
import axios, { AxiosError } from "axios";
import { getCookieJSON } from "@/config/aws-s3/cookie-management/store.helpers";

// Set global timeout for all axios requests
axios.defaults.timeout = 15000;

// Function to get user profile
export const getUserProfile = async (
  user: string
): Promise<UserInterface | null> => {
  try {
    const response = await axiosInstance.get<UserInterface>(
      `/inside/get_user/${user}`
    );

    if (response.status === 200) {
      return response.data;
    } else {
      return null;
    }
  } catch (error: unknown) {
    if (axios.isAxiosError(error)) {
      const axiosError = error as AxiosError;

      if (axiosError.response) {
        // Special handling for 401
        if (axiosError.response.status === 401) {
          return null;
        }
        return null;
      } else if (axiosError.request) {
        return null;
      } else {
        return null;
      }
    }
    return null;
  }
};

// Function to set user language
export const setUserLanguage = async (
  address: string,
  language: string
): Promise<UserInterface | null> => {
  try {
    const response = await axiosInstance.put(
      `/inside/set_language`,{
        address: address,
        language: language,
      }
    );

    if (response.status === 200) {
      return response.data;
    } else {
      return null;
    }
  } catch (error: unknown) {
    if (axios.isAxiosError(error)) {
      const axiosError = error as AxiosError;

      if (axiosError.response) {
        // Special handling for 401
        if (axiosError.response.status === 401) {
          return null;
        }
        return null;
      } else if (axiosError.request) {
        return null;
      } else {
        return null;
      }
    }
    return null;
  }
};

// Email verification function
export const verifyEmail = async (
  address: string,
  token: string
): Promise<boolean> => {
  try {
    const response = await axiosInstance.post("/inside/verify_email", {
      address,
      token,
    });
    return response.status === 200;
  } catch (error) {
    return false;
  }
};

// Function to update notification preference
export const updateNotificationPreference = async (
  address: string,
  preference: "email" | "telegram"
): Promise<boolean> => {
  try {
    const response = await axiosInstance.put(
      "/inside/update_notification_preference",
      {
        address,
        notification_preference: preference,
      }
    );

    return response.status === 200;
  } catch (error) {
    return false;
  }
};
