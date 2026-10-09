/**
 * L5-25: resolve RBAC audit `changed_by` to a human name.
 *
 * The audit row may store email, display name, wallet address, or a bare
 * staff id string. Only resolving the *subject* of the modal left other
 * actors as truncated `0x…` / raw emails.
 */

type StaffActorMember = {
  id: number;
  name: string;
  email: string;
};

export type FormatStaffActorOptions = {
  subject?: StaffActorMember | null;
  teamMembers?: readonly StaffActorMember[];
  ownerAddresses?: readonly string[];
  ownerLabel?: string | null;
  /** Fallback when an owner wallet matches but no label is provided. */
  ownerFallback?: string;
};

export function formatStaffActor(
  changedBy: string,
  opts: FormatStaffActorOptions = {},
): string {
  const raw = (changedBy || "").trim();
  if (!raw) return raw;
  const lower = raw.toLowerCase();
  const subject = opts.subject;
  const team = opts.teamMembers ?? [];
  const owners = opts.ownerAddresses ?? [];

  if (subject) {
    if (subject.email && lower === subject.email.toLowerCase()) {
      return subject.name || raw;
    }
    if (subject.name && raw === subject.name) return subject.name;
    if (String(subject.id) === raw) return subject.name || raw;
  }

  for (const member of team) {
    if (member.email && lower === member.email.toLowerCase()) {
      return member.name || raw;
    }
    if (member.name && raw === member.name) return member.name;
    if (String(member.id) === raw) return member.name || raw;
  }

  if (/^0x[a-fA-F0-9]{8,}$/.test(raw)) {
    const ownerHit = owners.some(
      (addr) => addr && addr.toLowerCase() === lower,
    );
    if (ownerHit) {
      return (opts.ownerLabel || "").trim() || opts.ownerFallback || "Owner";
    }
    return `${raw.slice(0, 6)}…${raw.slice(-4)}`;
  }

  return raw;
}
