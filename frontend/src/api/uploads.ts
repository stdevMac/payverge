import { axiosInstance } from './index';

export interface UploadResponse {
  url: string;
  filename: string;
}

// Upload file to S3
export const uploadFile = async (file: File, type: 'business-logo' | 'menu-item' = 'business-logo'): Promise<UploadResponse> => {
  const formData = new FormData();
  formData.append('file', file);
  formData.append('type', type);

  const response = await axiosInstance.post<UploadResponse>('/upload', formData, {
    headers: {
      'Content-Type': 'multipart/form-data',
    },
  });
  
  return response.data;
};

// Delete uploaded file
export const deleteFile = async (filename: string): Promise<void> => {
  await axiosInstance.delete(`/upload/${filename}`);
};
