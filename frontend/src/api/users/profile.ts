import { axiosInstance } from "../tools/instance";
import { UserInterface } from "@/interface";
import axios from "axios";

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
  } catch {
    // Any failure (network, 401, 4xx/5xx) resolves to a null profile.
    return null;
  }
};
