/**
 * The invitation link the inviter must share by hand, or null when the invite
 * email really went out. email_sent=false covers a failed or budget-refused
 * send; features.email=false (EMAIL_PROVIDER=log) covers the instance where
 * the backend "sends" by logging a redacted line (or the content only with EMAIL_LOG_CONTENT=true), so email_sent reads true but no
 * inbox ever sees the invite.
 */
export function inviteLinkToShare(
  result: { email_sent?: boolean; invitation_url?: string } | null | undefined,
  emailOff: boolean,
): string | null {
  const url = result?.invitation_url;
  if (!url) return null;
  return result.email_sent === false || emailOff ? url : null;
}
