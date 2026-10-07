/** Return the launch-cohort credential from a URL query without persisting it. */
export function getLaunchInviteCode(search?: string): string | undefined {
  const source =
    search ?? (typeof window !== "undefined" ? window.location.search : "");
  const params = new URLSearchParams(source);
  const value = (params.get("invite_code") ?? "").trim();
  return value || undefined;
}
