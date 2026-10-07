/**
 * Pure snapshot/restore helpers for the registration draft persisted to
 * localStorage (STORAGE_KEY in page.tsx). Extracted (like _stepValidation.ts)
 * so the draft shape is unit-testable without a heavy page render.
 */

// Single source for the funnel draft's localStorage key — shared by the
// register page (write/restore) and the success page (clear on CONFIRMED
// completion only).
export const REGISTRATION_DRAFT_STORAGE_KEY = "payverge_registration_draft";

interface RegistrationDraftAddress {
  street?: string;
  city?: string;
  state?: string;
  postal_code?: string;
  country?: string;
}

export interface RegistrationDraft {
  name?: string;
  owner_name?: string;
  email?: string;
  business_type?: string;
  address?: RegistrationDraftAddress;
}

const DRAFT_FIELDS = [
  "name",
  "owner_name",
  "email",
  "business_type",
  "address",
] as const;

export function buildRegistrationDraft(input: {
  formData: RegistrationDraft;
}): RegistrationDraft {
  const { formData } = input;
  return {
    name: formData.name,
    owner_name: formData.owner_name,
    email: formData.email,
    business_type: formData.business_type,
    address: formData.address,
  };
}

/**
 * Restores only the known form fields from a stored draft; anything else a
 * stale draft carries is dropped so it never reaches the create request.
 */
export function splitRegistrationDraft(parsed: unknown): RegistrationDraft {
  if (typeof parsed !== "object" || parsed === null || Array.isArray(parsed)) {
    return {};
  }
  const source = parsed as Record<string, unknown>;
  const draft: Record<string, unknown> = {};
  for (const field of DRAFT_FIELDS) {
    if (field in source) draft[field] = source[field];
  }
  return draft as RegistrationDraft;
}
