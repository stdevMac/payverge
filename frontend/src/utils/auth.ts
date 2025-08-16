export const isAdmin = (user: { role: string } | null): boolean => {
  return Boolean(user && user.role === "admin");
};
