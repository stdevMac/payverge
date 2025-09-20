import { axiosInstance } from './index';

export interface UploadResponse {
  location: string;
  filename: string;
  folder: string;
  business_id: number;
  url?: string; // For backward compatibility
}

// Upload file to S3
export const uploadFile = async (
  file: File, 
  type: 'business-logo' | 'menu-item' = 'business-logo',
  businessId?: number
): Promise<UploadResponse> => {
  const formData = new FormData();
  formData.append('file', file);
  formData.append('folder', type); // Use type as subfolder
  
  // If businessId is provided, add it as query parameter
  const url = businessId 
    ? `/inside/upload?business_id=${businessId}`
    : '/inside/upload';

  const response = await axiosInstance.post<UploadResponse>(url, formData, {
    headers: {
      'Content-Type': 'multipart/form-data',
    },
  });
  
  return response.data;
};

// Delete uploaded file
export const deleteFile = async (filename: string): Promise<void> => {
  await axiosInstance.delete(`/inside/upload/${filename}`);
};
