import { axiosInstance } from "@/api/tools/instance";

// UploadResponse is shared by tenant-scoped public and logo uploads. Callers
// consume `location`; the tenant is already in every upload URL.
export interface UploadResponse {
  location: string;
  filename: string;
  folder: string;
  business_id?: number;
}

// Upload file to S3
export const uploadFile = async (
  file: File,
  type: 'business-logo' | 'menu-item' | 'offer' | 'bundle' | 'banner' | 'gallery' | 'qr-logos' | 'partner-icons',
  businessId: number
): Promise<UploadResponse> => {
  const formData = new FormData();
  formData.append('file', file);
  formData.append('folder', type); // Use type as subfolder

  const response = await axiosInstance.post<UploadResponse>(
    `/inside/businesses/${businessId}/uploads`,
    formData,
    {
      headers: {
        'Content-Type': 'multipart/form-data',
      },
    },
  );

  return response.data;
};
