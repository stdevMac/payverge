import { axiosInstance } from "./tools/instance";

// Schemas (and their zod dependency, ~21 KiB) are dynamic-imported inside
// each validating call so they do not land in the homepage critical chunk
// via HybridAuthProvider's eager import of getCurrentUser/authAPI. The
// validators only run during real auth round-trips, which always happen
// after the page is interactive.
async function loadAuthSchemas() {
  return import('./schemas/auth');
}

// Types
export interface RegisterRequest {
  email: string;
  password: string;
  name: string;
  /** One-time launch cohort credential supplied by the operator. */
  invite_code?: string;
  /** Active operator locale (en / es / es-AR) — localizes the verification email. */
  language?: string;
}

export interface LoginRequest {
  email: string;
  password: string;
}

export interface AuthResponse {
  success: boolean;
  token: string;
  user_id: number;
  email: string;
  name: string;
  wallet_linked: boolean;
  wallet_address?: string;
  provider: string;
  is_new_user: boolean;
  message?: string;
  verification_required?: boolean;
  verification_url?: string;
}

/** Canonical backend error body: { error, code?, params? }. */
export interface ErrorEnvelope {
  error: string;
  code?: string;
  params?: {
    requires_email_verification?: boolean;
    verification_url?: string;
    [key: string]: unknown;
  };
}

export interface CurrentUser {
  id: number;
  email: string;
  name: string;
  role: string;
  picture?: string;
  avatar?: string;
  avatar_url?: string;
  profile_image_url?: string;
  auth_method: string;
  wallet_linked: boolean;
  wallet_address?: string;
  created_at: string;
}

interface AuthMethod {
  provider: string;
  created_at: string;
  wallet_address?: string;
  email_verified?: boolean;
}

export interface OAuthURLResponse {
  url: string;
  state: string;
}

interface LinkWalletRequest {
  address: string;
  message: string;
  signature: string;
}

export interface WalletChallengeResponse {
  challenge: string;
}

// API Functions

/**
 * Register a new user with email and password
 */
export async function register(data: RegisterRequest): Promise<AuthResponse> {
  const response = await axiosInstance.post<AuthResponse>("/auth/register", data, {
    _skipErrorToast: true,
  });
  const { validateAuthResponse } = await loadAuthSchemas();
  return validateAuthResponse(response.data);
}

/**
 * Login with email and password
 */
export async function login(data: LoginRequest): Promise<AuthResponse> {
  const response = await axiosInstance.post<AuthResponse>("/auth/login", data, {
    _skipErrorToast: true,
  });
  const { validateAuthResponse } = await loadAuthSchemas();
  return validateAuthResponse(response.data);
}

/**
 * Logout the current user
 */
export async function logout(): Promise<void> {
  await axiosInstance.post("/auth/logout");
}

/**
 * Get the current authenticated user
 */
export async function getCurrentUser(): Promise<CurrentUser> {
  const response = await axiosInstance.get<CurrentUser>("/auth/me");
  const { validateCurrentUser } = await loadAuthSchemas();
  return validateCurrentUser(response.data);
}

/**
 * Get Google OAuth URL
 */
export async function getGoogleAuthURL(inviteCode?: string): Promise<OAuthURLResponse> {
  const normalizedInvite = inviteCode?.trim();
  const response = await axiosInstance.get<OAuthURLResponse>("/auth/google", {
    params: normalizedInvite ? { invite_code: normalizedInvite } : undefined,
  });
  return response.data;
}

/**
 * Link a wallet to the current user
 */
async function linkWallet(data: LinkWalletRequest): Promise<{ success: boolean; message: string; address: string }> {
  const response = await axiosInstance.post("/auth/wallet/link", data);
  return response.data;
}

export async function getWalletChallenge(address: string): Promise<WalletChallengeResponse> {
  const response = await axiosInstance.post<WalletChallengeResponse>("/auth/challenge", { address });
  return response.data;
}

/**
 * Unlink wallet from the current user
 */
async function unlinkWallet(): Promise<{ success: boolean; message: string }> {
  const response = await axiosInstance.delete("/auth/wallet/unlink");
  return response.data;
}

/**
 * Get all authentication methods for the current user
 */
async function getAuthMethods(): Promise<{ methods: AuthMethod[] }> {
  const response = await axiosInstance.get("/auth/methods");
  return response.data;
}

/**
 * Request a password reset email
 */
export async function requestPasswordReset(email: string): Promise<{ message: string }> {
  const response = await axiosInstance.post("/auth/password/reset-request", { email });
  return response.data;
}

/**
 * Reset password with token
 */
export async function resetPassword(token: string, newPassword: string): Promise<{ message: string }> {
  const response = await axiosInstance.post("/auth/password/reset", { token, new_password: newPassword });
  return response.data;
}

/**
 * Verify email with token. `already_verified` marks the idempotent re-click of
 * a consumed link (still a success).
 */
async function verifyEmail(
  token: string,
): Promise<{ message: string; already_verified?: boolean }> {
  const response = await axiosInstance.post("/auth/email/verify", { token });
  return response.data;
}

/**
 * Request a fresh verification email. Always returns 200 (no enumeration).
 */
async function resendVerification(email: string): Promise<{ message: string }> {
  const response = await axiosInstance.post("/auth/email/resend-verification", { email });
  return response.data;
}

// Auth API object for easier imports
export const authAPI = {
  register,
  login,
  logout,
  getCurrentUser,
  getGoogleAuthURL,
  getWalletChallenge,
  linkWallet,
  unlinkWallet,
  getAuthMethods,
  requestPasswordReset,
  resetPassword,
  verifyEmail,
  resendVerification,
};
