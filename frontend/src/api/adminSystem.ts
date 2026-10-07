import { axiosInstance } from "@/api/tools/instance";

interface AdminHealthCheck {
  service: string;
  status: string;
  message?: string;
  latency?: string;
}

interface AdminWorkerStatus {
  name: string;
  status: string;
  detail?: string;
  started: boolean;
}

interface AdminQueueMetric {
  name: string;
  count: number;
  label?: string;
}

export interface AdminSystemHealth {
  status: string;
  uptime: string;
  version: string;
  timestamp: string;
  checks: AdminHealthCheck[];
  workers: AdminWorkerStatus[];
  queues: AdminQueueMetric[];
}

export async function getAdminSystemHealth(): Promise<AdminSystemHealth> {
  const response = await axiosInstance.get<AdminSystemHealth>(
    "/admin/system/health",
    { _useCache: false },
  );
  return response.data;
}
