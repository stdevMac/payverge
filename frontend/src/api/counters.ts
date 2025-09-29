import { axiosInstance } from './tools/instance';

// Counter interfaces
export interface Counter {
  id: number;
  business_id: number;
  counter_number: number;
  name: string;
  is_active: boolean;
  current_bill_id?: number;
  created_at: string;
  updated_at: string;
}

export interface CounterSettings {
  counter_enabled: boolean;
  counter_count: number;
  counter_prefix: string;
}

export interface UpdateCounterSettingsRequest {
  counter_enabled: boolean;
  counter_count: number;
  counter_prefix: string;
}

export interface CountersResponse {
  counters: Counter[];
  business: CounterSettings;
}

// Counter API functions

// Update counter settings for a business
export const updateCounterSettings = async (
  businessId: number,
  settings: UpdateCounterSettingsRequest
): Promise<{ message: string }> => {
  const response = await axiosInstance.put(
    `/inside/businesses/${businessId}/counters/settings`,
    settings
  );
  return response.data;
};

// Get all counters for a business
export const getBusinessCounters = async (
  businessId: number
): Promise<CountersResponse> => {
  const response = await axiosInstance.get(
    `/inside/businesses/${businessId}/counters`
  );
  return response.data;
};

// Get available counters for bill creation
export const getAvailableCounters = async (
  businessId: number
): Promise<{ counters: Counter[] }> => {
  const response = await axiosInstance.get(
    `/inside/businesses/${businessId}/counters/available`
  );
  return response.data;
};
