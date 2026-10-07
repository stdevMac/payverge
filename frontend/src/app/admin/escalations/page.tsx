"use client";

import { useState, useEffect, useCallback, useMemo } from "react";
import {
  Card,
  CardBody,
  CardHeader,
  Button,
  Spinner,
  Chip,
  Pagination,
  Modal,
  ModalContent,
  ModalHeader,
  ModalBody,
  ModalFooter,
  Select,
  SelectItem,
  Textarea,
  useDisclosure,
} from "@nextui-org/react";
import { RefreshCw, LifeBuoy, AlertCircle, MessageSquare, Pencil } from "lucide-react";
import {
  getAdminEscalations,
  patchEscalation,
  parseEscalationTranscriptTarget,
  ESCALATION_STATUSES,
  type Escalation,
  type OpsTranscriptTarget,
} from "@/api/adminEscalations";
import { apiErrorDetail } from "@/utils/apiError";
import {
  OpsTranscriptModal,
  formatAdminDate,
} from "@/components/admin/primitives";
import { EmptyState } from "@/components/ui/EmptyState";

const PAGE_SIZE = 20;

function snippet(text: string, max = 160): string {
  if (!text) return "";
  const clean = text.replace(/\s+/g, " ").trim();
  return clean.length > max ? `${clean.slice(0, max)}…` : clean;
}

function escalationStatusColor(
  status: string,
): "default" | "warning" | "success" | "primary" {
  switch (status) {
    case "open":
      return "warning";
    case "in_progress":
      return "primary";
    case "resolved":
    case "closed":
      return "success";
    default:
      return "default";
  }
}

export default function AdminEscalationsPage() {
  const [rows, setRows] = useState<Escalation[]>([]);
  const [total, setTotal] = useState(0);
  const [page, setPage] = useState(1);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [transcriptTarget, setTranscriptTarget] = useState<OpsTranscriptTarget | null>(null);
  const [editing, setEditing] = useState<Escalation | null>(null);
  const [editStatus, setEditStatus] = useState("open");
  const [editNotes, setEditNotes] = useState("");
  const [saving, setSaving] = useState(false);
  const editModal = useDisclosure();
  const transcriptModal = useDisclosure();

  const totalPages = useMemo(
    () => Math.max(1, Math.ceil(total / PAGE_SIZE)),
    [total],
  );

  const fetchEscalations = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      const data = await getAdminEscalations(page, PAGE_SIZE);
      setRows(data.escalations);
      setTotal(data.total);
    } catch (err: unknown) {
      setError(apiErrorDetail(err) || "Failed to load escalations");
    } finally {
      setLoading(false);
    }
  }, [page]);

  useEffect(() => {
    fetchEscalations();
  }, [fetchEscalations]);

  const openEdit = (row: Escalation) => {
    setEditing(row);
    setEditStatus(row.status || "open");
    setEditNotes(row.admin_notes || "");
    editModal.onOpen();
  };

  const saveEscalation = async () => {
    if (!editing) return;
    setSaving(true);
    try {
      const updated = await patchEscalation(editing.id, {
        status: editStatus,
        admin_notes: editNotes,
      });
      setRows((list) =>
        list.map((row) => (row.id === updated.id ? updated : row)),
      );
      editModal.onClose();
    } catch (err: unknown) {
      setError(apiErrorDetail(err) || "Failed to update escalation");
    } finally {
      setSaving(false);
    }
  };

  const openTranscript = (row: Escalation) => {
    const target = parseEscalationTranscriptTarget(
      row.session_ref,
      row.business_id ?? undefined,
    );
    if (!target) return;
    setTranscriptTarget(target);
    transcriptModal.onOpen();
  };

  const canShowTranscript = (row: Escalation) =>
    Boolean(
      parseEscalationTranscriptTarget(
        row.session_ref,
        row.business_id ?? undefined,
      ),
    );

  return (
    <div className="max-w-7xl mx-auto space-y-6">
      <div className="flex items-start justify-between gap-4">
        <p className="text-default-500">
          Support escalations from the Ops assistant{" "}
          {loading ? "" : `— ${total} total`}
        </p>
        <Button
          variant="bordered"
          onPress={fetchEscalations}
          isLoading={loading}
          startContent={!loading && <RefreshCw className="w-4 h-4" />}
        >
          Refresh
        </Button>
      </div>

      {error && (
        <Card className="border-2 border-danger">
          <CardBody>
            <div className="flex items-start gap-3">
              <AlertCircle className="w-6 h-6 text-danger flex-shrink-0 mt-1" />
              <p className="text-sm text-default-500">{error}</p>
            </div>
          </CardBody>
        </Card>
      )}

      <Card>
        <CardHeader className="flex items-center gap-2">
          <LifeBuoy className="w-5 h-5 text-brand" />
          <p className="text-md font-semibold">Raised Escalations</p>
        </CardHeader>
        <CardBody>
          {loading ? (
            <div className="flex justify-center py-12">
              <Spinner />
            </div>
          ) : rows.length === 0 ? (
            <EmptyState
              compact
              icon={LifeBuoy}
              title="No escalations raised yet"
              subtitle="Support escalations from the Ops assistant will appear here."
              data-testid="admin-escalations-empty"
            />
          ) : (
            <>
              <div className="overflow-x-auto">
                <table className="w-full text-sm">
                  <thead>
                    <tr className="border-b border-default-200 text-left text-default-500">
                      <th className="py-2 pr-4 font-medium">Created</th>
                      <th className="py-2 pr-4 font-medium">Source</th>
                      <th className="py-2 pr-4 font-medium">Issue</th>
                      <th className="py-2 pr-4 font-medium">Contact</th>
                      <th className="py-2 pr-4 font-medium">Status</th>
                      <th className="py-2 pr-4 font-medium">Actions</th>
                    </tr>
                  </thead>
                  <tbody>
                    {rows.map((row) => (
                      <tr key={row.id} className="border-b border-default-100 align-top">
                        <td className="py-3 pr-4 whitespace-nowrap text-default-500">
                          {formatAdminDate(row.created_at, true)}
                        </td>
                        <td className="py-3 pr-4">
                          <Chip size="sm" variant="flat">{row.source}</Chip>
                        </td>
                        <td className="py-3 pr-4 max-w-[22rem] text-default-600">
                          {snippet(row.issue || row.transcript_summary) || "—"}
                          {row.business_id ? (
                            <p className="text-xs text-default-400">Business #{row.business_id}</p>
                          ) : null}
                        </td>
                        <td className="py-3 pr-4">
                          {row.contact_email ? (
                            <a href={`mailto:${row.contact_email}`} className="text-brand text-xs hover:underline">
                              {row.contact_email}
                            </a>
                          ) : (
                            "—"
                          )}
                        </td>
                        <td className="py-3 pr-4">
                          <Chip size="sm" variant="flat" color={escalationStatusColor(row.status)}>
                            {row.status || "open"}
                          </Chip>
                        </td>
                        <td className="py-3 pr-4">
                          <div className="flex gap-1">
                            {canShowTranscript(row) ? (
                              <Button
                                size="sm"
                                variant="light"
                                isIconOnly
                                aria-label="View transcript"
                                onPress={() => openTranscript(row)}
                              >
                                <MessageSquare size={16} />
                              </Button>
                            ) : null}
                            <Button
                              size="sm"
                              variant="light"
                              isIconOnly
                              aria-label="Update escalation"
                              onPress={() => openEdit(row)}
                            >
                              <Pencil size={16} />
                            </Button>
                          </div>
                        </td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
              {totalPages > 1 && (
                <div className="flex justify-center pt-6">
                  <Pagination
                    total={totalPages}
                    page={page}
                    onChange={setPage}
                    showControls
                    classNames={{ cursor: "bg-brand text-white" }}
                  />
                </div>
              )}
            </>
          )}
        </CardBody>
      </Card>

      <Modal isOpen={editModal.isOpen} onClose={editModal.onClose}>
        <ModalContent>
          <ModalHeader>Update escalation</ModalHeader>
          <ModalBody className="gap-4">
            <Select
              label="Status"
              selectedKeys={[editStatus]}
              onChange={(e) => setEditStatus(e.target.value)}
            >
              {ESCALATION_STATUSES.map((s) => (
                <SelectItem key={s} value={s}>
                  {s.replace("_", " ")}
                </SelectItem>
              ))}
            </Select>
            <Textarea
              label="Admin notes"
              value={editNotes}
              onValueChange={setEditNotes}
              minRows={3}
            />
          </ModalBody>
          <ModalFooter>
            <Button variant="light" onPress={editModal.onClose}>
              Cancel
            </Button>
            <Button color="primary" className="bg-brand" isLoading={saving} onPress={saveEscalation}>
              Save
            </Button>
          </ModalFooter>
        </ModalContent>
      </Modal>

      <OpsTranscriptModal
        isOpen={transcriptModal.isOpen}
        onClose={transcriptModal.onClose}
        target={transcriptTarget}
        title="Escalation transcript"
      />
    </div>
  );
}
