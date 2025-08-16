// Base interfaces for code-related data
export interface CodeBase {
  code: string;
  amount: string;
  expiry: string;
}

// Request interfaces for user methods
export interface CheckCodeRequest {
  code: string;
}

export interface UseCodeRequest {
  code: string;
  address: string;
}

// Response interfaces for user methods
export interface CheckCodeResponse {
  status: 'available' | 'unavailable';
  code?: string;
  amount?: string;
  expiry?: string;
  error?: string;
}

export interface UseCodeResponse {
  status?: 'success';
  txHash?: string;
  amount?: string;
  code?: string;
  error?: string;
}

// Request interfaces for admin methods
export interface CreateCodeRequest {
  amount: string;
  expiry?: string; // Optional
}

export interface GetCodeRequest {
  code: string;
}

export interface UpdateCodeRequest {
  code: string;
  amount?: string;
  expiry?: string;
  used?: boolean;
}

export interface DeleteCodeRequest {
  code: string;
}

// Response interfaces for admin methods
export interface CreateCodeResponse {
  message: string;
  code: string;
  error?: string;
}

export interface GetCodeResponse {
  code: string;
  amount: string;
  used: boolean;
  expiry: string;
  address: string | null;
  error?: string;
}

export interface UpdateCodeResponse {
  message?: string;
  error?: string;
}

export interface DeleteCodeResponse {
  message?: string;
  error?: string;
}

export interface CodeDetails {
  code: string;
  amount: string;
  used: boolean;
  expiry: string;
  address?: string | null;
}

export type GetAllCodesResponse = CodeDetails[] | { error: string };
