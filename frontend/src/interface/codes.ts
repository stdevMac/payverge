// Admin Code Interfaces
export interface CreateCodeRequest {
  amount: string;
  expiry?: string;
}

export interface CreateCodeResponse {
  success: boolean;
  code?: string;
  message?: string;
  error?: string;
}

export interface GetCodeRequest {
  code: string;
}

export interface CodeDetails {
  code: string;
  amount: string | number;
  created: string;
  expiry?: string;
  used: boolean;
  used_by?: string;
  used_at?: string;
  address?: string; // Used for filtering in the UI
}

export interface GetCodeResponse {
  success: boolean;
  code?: CodeDetails;
  message?: string;
  error?: string;
}

export interface UpdateCodeRequest {
  code: string;
  amount?: string;
  expiry?: string;
  used?: boolean;
}

export interface UpdateCodeResponse {
  success: boolean;
  code?: CodeDetails;
  message?: string;
  error?: string;
}

export interface DeleteCodeRequest {
  code: string;
}

export interface DeleteCodeResponse {
  success: boolean;
  message?: string;
  error?: string;
}

export interface GetAllCodesResponse {
  success: boolean;
  codes?: CodeDetails[];
  message?: string;
  error?: string;
}

// User Code Interfaces
export interface CheckCodeRequest {
  code: string;
}

export interface CheckCodeResponse {
  valid?: boolean;
  isValid?: boolean;
  amount?: string;
  message?: string;
  error?: string;
  // Additional properties from the backend response
  code?: string;
  status?: 'available' | 'unavailable' | 'invalid' | 'error';
  expiry?: string;
}

export interface UseCodeRequest {
  code: string;
  address: string;
}

export interface UseCodeResponse {
  success: boolean;
  transaction?: string;
  message?: string;
}