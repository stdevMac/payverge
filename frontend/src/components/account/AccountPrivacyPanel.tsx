"use client";

import { useCallback, useState } from "react";
import {
  Button,
  Card,
  CardBody,
  Input,
  Modal,
  ModalBody,
  ModalContent,
  ModalFooter,
  ModalHeader,
  Textarea,
} from "@nextui-org/react";
import { Download, ShieldAlert } from "lucide-react";

import {
  downloadAccountExport,
  requestAccountDeletion,
} from "@/api/users/account";
import { axiosInstance } from "@/api/tools/instance";

// IMP-20 — Privacy panel rendered on the user's dashboard. Two actions:
//   1. "Download my data" — calls POST /inside/account/export, then triggers
//      a browser download of the returned JSON snapshot.
//   2. "Delete account" — opens a typed-confirmation modal. The user must
//      type their account email/wallet to enable the destructive button.
//      On success we sign them out and redirect to the landing page.
//
// Both flows surface a loading + status message instead of toasts so the
// state is glanceable on a page that already has plenty of chrome.

export interface AccountPrivacyPanelProps {
  // Whichever identifier the user signed in with — email is preferred,
  // wallet address is the fallback for pure-Web3 accounts. The string
  // shown here is also what the delete modal compares against.
  accountIdentifier: string;
  // Pre-localised copy from the dashboard's translation helper. Keeping the
  // strings external means we don't have to extend the (currently English)
  // dashboard JSON before the feature ships.
  copy?: Partial<AccountPrivacyCopy>;
}

export interface AccountPrivacyCopy {
  sectionTitle: string;
  sectionDescription: string;
  exportTitle: string;
  exportDescription: string;
  exportButton: string;
  exportingButton: string;
  exportSuccess: string;
  exportError: string;
  deleteTitle: string;
  deleteDescription: string;
  deleteButton: string;
  modalTitle: string;
  modalIntro: string;
  modalConfirmLabel: string;
  modalReasonLabel: string;
  modalReasonPlaceholder: string;
  modalCancel: string;
  modalConfirm: string;
  modalConfirming: string;
  modalError: string;
  modalSuccess: string;
}

const DEFAULT_COPY: AccountPrivacyCopy = {
  sectionTitle: "Privacy",
  sectionDescription:
    "Download a copy of your Payverge account data or request that we delete it.",
  exportTitle: "Download my data",
  exportDescription:
    "We will assemble a JSON snapshot of your profile and the businesses you own.",
  exportButton: "Download data export",
  exportingButton: "Preparing your export…",
  exportSuccess:
    "Your export was downloaded. Snapshot is valid for 24 hours — re-run the export if you need fresher data.",
  exportError:
    "We couldn't prepare your export. Please try again or contact support if the issue persists.",
  deleteTitle: "Delete my account",
  deleteDescription:
    "Disables your account and starts a 30-day review window. Some tax, fiscal, billing, fraud, security, or legal records may need to be retained.",
  deleteButton: "Request account deletion",
  modalTitle: "Delete your Payverge account?",
  modalIntro:
    "This disables your account and sets a deletion-review date 30 days from now. Final deletion is reviewed because some records may need to be retained. Type the value below to confirm.",
  modalConfirmLabel: "Type your account email to confirm",
  modalReasonLabel: "Reason (optional)",
  modalReasonPlaceholder: "What's prompting you to delete? (helps us improve)",
  modalCancel: "Keep my account",
  modalConfirm: "Delete my account",
  modalConfirming: "Scheduling deletion…",
  modalError:
    "Account deletion failed. Make sure the confirmation matches and try again.",
  modalSuccess:
    "Account disabled and deletion review scheduled. Signing you out…",
};

function mergeCopy(override?: Partial<AccountPrivacyCopy>): AccountPrivacyCopy {
  if (!override) return DEFAULT_COPY;
  return { ...DEFAULT_COPY, ...override };
}

type Banner = { kind: "ok" | "err"; text: string } | null;

export function AccountPrivacyPanel({
  accountIdentifier,
  copy: copyOverride,
}: AccountPrivacyPanelProps) {
  const copy = mergeCopy(copyOverride);
  const [exporting, setExporting] = useState(false);
  const [exportBanner, setExportBanner] = useState<Banner>(null);

  const [deleteOpen, setDeleteOpen] = useState(false);
  const [deleting, setDeleting] = useState(false);
  const [confirmText, setConfirmText] = useState("");
  const [reasonText, setReasonText] = useState("");
  const [deleteBanner, setDeleteBanner] = useState<Banner>(null);

  const normalisedIdentifier = (accountIdentifier ?? "").trim().toLowerCase();
  const canConfirmDelete =
    normalisedIdentifier.length > 0 &&
    confirmText.trim().toLowerCase() === normalisedIdentifier;

  const handleExport = useCallback(async () => {
    setExporting(true);
    setExportBanner(null);
    try {
      await downloadAccountExport();
      setExportBanner({ kind: "ok", text: copy.exportSuccess });
    } catch (err) {
      // Best-effort error surface — the axios layer already redacts secrets.
      console.error("[AccountPrivacyPanel] export failed", err);
      setExportBanner({ kind: "err", text: copy.exportError });
    } finally {
      setExporting(false);
    }
  }, [copy.exportError, copy.exportSuccess]);

  const handleConfirmDelete = useCallback(async () => {
    if (!canConfirmDelete) return;
    setDeleting(true);
    setDeleteBanner(null);
    try {
      await requestAccountDeletion(confirmText.trim(), reasonText.trim());
      setDeleteBanner({ kind: "ok", text: copy.modalSuccess });
      // Sign the user out, then send them to the marketing landing. We do
      // this via direct axios call rather than the auth provider's hook so
      // the panel stays standalone — even an OAuth user's tokens get nuked.
      try {
        await axiosInstance.post("/auth/logout");
      } catch (logoutErr) {
        // Non-fatal: the backend already flagged the row.
        console.warn(
          "[AccountPrivacyPanel] post-delete logout failed",
          logoutErr,
        );
      }
      if (typeof window !== "undefined") {
        setTimeout(() => {
          window.location.href = "/";
        }, 1200);
      }
    } catch (err) {
      console.error("[AccountPrivacyPanel] delete failed", err);
      setDeleteBanner({ kind: "err", text: copy.modalError });
    } finally {
      setDeleting(false);
    }
  }, [
    canConfirmDelete,
    confirmText,
    reasonText,
    copy.modalError,
    copy.modalSuccess,
  ]);

  const closeModal = useCallback(() => {
    if (deleting) return;
    setDeleteOpen(false);
    setConfirmText("");
    setReasonText("");
    setDeleteBanner(null);
  }, [deleting]);

  return (
    <section className="mb-12">
      <div className="flex items-center gap-3 mb-6 ml-1">
        <div className="p-2 bg-ink-900 text-white rounded-lg shadow-sm">
          <ShieldAlert className="w-4 h-4" />
        </div>
        <h2 className="text-xl font-bold text-ink-900 tracking-tight leading-none">
          {copy.sectionTitle}
        </h2>
      </div>

      <p className="text-sm text-ink-700 mb-4 max-w-2xl">
        {copy.sectionDescription}
      </p>

      <div className="grid grid-cols-1 md:grid-cols-2 gap-6">
        <Card className="border border-ink-300 shadow-sm">
          <CardBody className="p-6 space-y-4">
            <div className="flex items-center gap-3">
              <Download className="w-5 h-5 text-ink-700" aria-hidden />
              <h3 className="text-base font-semibold text-ink-900">
                {copy.exportTitle}
              </h3>
            </div>
            <p className="text-sm text-ink-600">{copy.exportDescription}</p>
            <Button
              onPress={handleExport}
              isDisabled={exporting}
              className="bg-ink-900 text-white font-semibold rounded-xl"
              aria-label={copy.exportButton}
            >
              {exporting ? copy.exportingButton : copy.exportButton}
            </Button>
            {exportBanner && (
              <p
                role="status"
                className={
                  exportBanner.kind === "ok"
                    ? "text-sm text-emerald-700"
                    : "text-sm text-rose-700"
                }
              >
                {exportBanner.text}
              </p>
            )}
          </CardBody>
        </Card>

        <Card className="border border-rose-200 shadow-sm">
          <CardBody className="p-6 space-y-4">
            <div className="flex items-center gap-3">
              <ShieldAlert className="w-5 h-5 text-rose-700" aria-hidden />
              <h3 className="text-base font-semibold text-rose-900">
                {copy.deleteTitle}
              </h3>
            </div>
            <p className="text-sm text-rose-800/80">{copy.deleteDescription}</p>
            <Button
              onPress={() => setDeleteOpen(true)}
              variant="bordered"
              className="border-rose-500 text-rose-700 font-semibold rounded-xl"
            >
              {copy.deleteButton}
            </Button>
          </CardBody>
        </Card>
      </div>

      <Modal
        isOpen={deleteOpen}
        onClose={closeModal}
        isDismissable={!deleting}
        size="lg"
      >
        <ModalContent>
          <ModalHeader className="text-rose-700">{copy.modalTitle}</ModalHeader>
          <ModalBody className="space-y-4">
            <p className="text-sm text-ink-700">{copy.modalIntro}</p>
            <Input
              label={copy.modalConfirmLabel}
              placeholder={accountIdentifier}
              value={confirmText}
              onValueChange={setConfirmText}
              isDisabled={deleting}
              autoComplete="off"
              spellCheck={false}
            />
            <Textarea
              label={copy.modalReasonLabel}
              placeholder={copy.modalReasonPlaceholder}
              value={reasonText}
              onValueChange={setReasonText}
              isDisabled={deleting}
              maxLength={1024}
            />
            {deleteBanner && (
              <p
                role="status"
                className={
                  deleteBanner.kind === "ok"
                    ? "text-sm text-emerald-700"
                    : "text-sm text-rose-700"
                }
              >
                {deleteBanner.text}
              </p>
            )}
          </ModalBody>
          <ModalFooter>
            <Button variant="light" onPress={closeModal} isDisabled={deleting}>
              {copy.modalCancel}
            </Button>
            <Button
              color="danger"
              isDisabled={!canConfirmDelete || deleting}
              onPress={handleConfirmDelete}
            >
              {deleting ? copy.modalConfirming : copy.modalConfirm}
            </Button>
          </ModalFooter>
        </ModalContent>
      </Modal>
    </section>
  );
}
