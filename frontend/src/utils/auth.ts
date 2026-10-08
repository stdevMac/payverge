export function resolveAdminRole(
  oauthRole?: string,
  staffRole?: string,
  storedRole?: string,
): string | null {
  return oauthRole || staffRole || storedRole || null;
}

export function isAdminRole(role: string | null | undefined): boolean {
  return role === "admin";
}

export const isAdmin = (user: { role: string } | null): boolean => {
  return Boolean(user && user.role === "admin");
};
