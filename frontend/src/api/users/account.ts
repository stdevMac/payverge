import { axiosInstance } from "../tools/instance";

// IMP-20 — Account / data export + delete (GDPR / UAE PDPL self-service).
//
// Two endpoints live behind the protected `/api/v1/inside/account/...` group.
// The frontend wraps them in a tiny client that:
//   1. Calls `/account/export`, receives the JSON snapshot inline, and
//      triggers a browser download via a Blob URL.
//   2. Calls `/account/delete` with a typed-confirmation string and returns
//      the soft-delete schedule for the UI to surface.

interface AccountExportBusiness {
  id: number;
  business_id: string;
  name: string;
  default_currency: string;
  display_currency: string;
}

export interface AccountExportPayload {
  generated_at: string;
  export_ttl_at: string;
  profile: {
    id: number;
    email: string;
    name: string;
    username: string;
    wallet_address: string;
    role: string;
    auth_method: string;
    email_verified: boolean;
    language_selected: string;
    created_at: string;
    deleted_at?: string;
    deletion_scheduled_at?: string;
  };
  businesses: AccountExportBusiness[];
}

export interface AccountDeleteResponse {
  success: boolean;
  already_pending?: boolean;
  deleted_at: string;
  deletion_scheduled_at: string;
  grace_period_days?: number;
  deletion_mode?: "anonymize_after_grace";
  hard_delete_automated?: false;
  retention_exceptions?: string[];
}

/**
 * Trigger a data export and download the resulting JSON as a file.
 *
 * The backend inlines the payload (small enough for a single round trip);
 * we wrap it in a Blob + temporary anchor so the user gets a real download
 * without having to copy-paste a giant JSON blob from devtools.
 */
export async function downloadAccountExport(): Promise<AccountExportPayload> {
  const response = await axiosInstance.post<AccountExportPayload>(
    "/inside/account/export",
  );
  const data = response.data;

  // Trigger a browser download. Guarded behind `typeof window` so this stays
  // safe under tests / SSR.
  if (typeof window !== "undefined" && typeof document !== "undefined") {
    const blob = new Blob([JSON.stringify(data, null, 2)], {
      type: "application/json",
    });
    const url = URL.createObjectURL(blob);
    const stamp = new Date().toISOString().replace(/[:.]/g, "-");
    const a = document.createElement("a");
    a.href = url;
    a.download = `payverge-account-export-${stamp}.json`;
    document.body.appendChild(a);
    a.click();
    document.body.removeChild(a);
    URL.revokeObjectURL(url);
  }

  return data;
}

/**
 * Request a soft-delete of the authenticated user's account. The backend
 * verifies `confirmEmail` matches the account's email (case-insensitive)
 * before flipping the flag. Returns the 30-day review date and any record
 * classes that may require retention.
 */
export async function requestAccountDeletion(
  confirmEmail: string,
  reason?: string,
): Promise<AccountDeleteResponse> {
  const response = await axiosInstance.post<AccountDeleteResponse>(
    "/inside/account/delete",
    {
      confirm_email: confirmEmail,
      reason: reason ?? "",
    },
  );
  return response.data;
}
