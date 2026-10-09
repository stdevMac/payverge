import { axiosInstance } from "@/api/tools/instance";

export interface ErrorLog {
  id: number;
  created_at: string;
  timestamp?: string;
  message: string;
  error?: string;
  source: string;
  component: string;
  function: string;
  stack?: string;
  request_id?: string;
}

export const errorLogsAPI = {
  getErrors: async (params?: {
    source?: string;
    component?: string;
    offset?: number;
    limit?: number;
  }) => {
    const response = await axiosInstance.get("/admin/errors", {
      params,
      _useCache: false,
    });
    return response.data;
  },
};
