import { z } from 'zod';
import { logError } from '@/utils/errorLogger';

export const AuthResponseSchema = z.object({
  success: z.boolean(),
  token: z.string(),
  user_id: z.number(),
  email: z.string(),
  name: z.string(),
  wallet_linked: z.boolean(),
  wallet_address: z.string().optional(),
  provider: z.string(),
  is_new_user: z.boolean(),
  message: z.string().optional(),
  verification_required: z.boolean().optional(),
  verification_url: z.string().optional(),
}).loose();

export const CurrentUserSchema = z.object({
  id: z.number(),
  email: z.string(),
  name: z.string(),
  role: z.string(),
  picture: z.string().optional(),
  avatar: z.string().optional(),
  avatar_url: z.string().optional(),
  profile_image_url: z.string().optional(),
  auth_method: z.string(),
  wallet_linked: z.boolean(),
  wallet_address: z.string().optional(),
  created_at: z.string(),
}).loose();

export function validateAuthResponse<T>(data: T): T {
  const result = AuthResponseSchema.safeParse(data);
  if (!result.success) {
    void logError(
      `Auth response validation failed: ${result.error.message}`,
      'ApiValidation',
      'validateAuthResponse'
    );
    throw new Error(`Auth response validation failed: ${result.error.message}`);
  }
  return result.data as T;
}

export function validateCurrentUser<T>(data: T): T {
  const result = CurrentUserSchema.safeParse(data);
  if (!result.success) {
    void logError(
      `CurrentUser response validation failed: ${result.error.message}`,
      'ApiValidation',
      'validateCurrentUser'
    );
    throw new Error(`CurrentUser response validation failed: ${result.error.message}`);
  }
  return result.data as T;
}
