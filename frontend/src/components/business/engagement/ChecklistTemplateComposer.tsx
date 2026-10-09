"use client";

import React, { useCallback, useMemo, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  Button,
  Chip,
  Input,
  Modal,
  ModalBody,
  ModalContent,
  ModalFooter,
  ModalHeader,
  Select,
  SelectItem,
  Spinner,
  Switch,
  useDisclosure,
} from "@nextui-org/react";
import { CheckCircle2, ClipboardList, Play, Plus, X } from "lucide-react";
import {
  checklistsApi,
  type ChecklistKind,
  type ChecklistTemplate,
} from "@/api/engagement";
import { getBusinessStaff } from "@/api/staff";
import { queryKeys } from "@/api/queryKeys";
import { getTranslation, useSimpleLocale } from "@/i18n/SimpleTranslationProvider";
import { useToast } from "@/contexts/ToastContext";
import { EmptyState } from "@/components/ui/EmptyState";
import { StaffSelect } from "@/components/ui/fields/StaffSelect";

// Operator checklist-template composer (mounted in the EngagementPanel). Creates a
// reusable template (checklist:manage — manager/owner) with a kind discriminator
// and ordered, optionally-required items, then lists existing templates.
// Self-resolves the dashboard.engagement.checklists namespace. Money-free.

const KINDS: ChecklistKind[] = ["onboarding", "opening", "closing", "custom"];

interface ItemRow {
  label: string;
  required: boolean;
}

export default function ChecklistTemplateComposer({ businessId }: { businessId: string }) {
  const { locale } = useSimpleLocale();
  const toast = useToast();
  const qc = useQueryClient();

  const [name, setName] = useState("");
  const [kind, setKind] = useState<ChecklistKind>("opening");
  const [items, setItems] = useState<ItemRow[]>([{ label: "", required: true }]);
  // List-first: existing templates greet the tab; the creation form opens on
  // demand (and stays open on a true cold start, when there's nothing to list).
  const [composing, setComposing] = useState(false);

  const t = useCallback(
    (key: string): string => {
      const v = getTranslation(`dashboard.engagement.checklists.${key}`, locale);
      return typeof v === "string" ? v : key;
    },
    [locale],
  );

  const listKey = queryKeys.engagement.checklistTemplates(businessId);
  const listQuery = useQuery({ queryKey: listKey, queryFn: () => checklistsApi.listTemplates(businessId) });

  const createMutation = useMutation({
    mutationFn: () =>
      checklistsApi.createTemplate(businessId, {
        name: name.trim(),
        kind,
        items: items
          .filter((it) => it.label.trim() !== "")
          .map((it, sort_order) => ({ label: it.label.trim(), sort_order, is_required: it.required })),
      }),
    onSuccess: () => {
      toast.showSuccess(t("saved"));
      setName("");
      setKind("opening");
      setItems([{ label: "", required: true }]);
      setComposing(false);
      void qc.invalidateQueries({ queryKey: listKey });
    },
    onError: () => toast.showError(t("saveError")),
  });

  // Operator run-status list + the assign-run flow (createRun had zero call sites,
  // so operator templates never reached staff — this is the "start a run" surface).
  const runsKey = queryKeys.engagement.checklistBusinessRuns(businessId);
  const runsQuery = useQuery({
    queryKey: runsKey,
    queryFn: () => checklistsApi.listBusinessRuns(businessId),
  });
  const staffQuery = useQuery({
    queryKey: queryKeys.staff.list(businessId),
    queryFn: () => getBusinessStaff(businessId),
    staleTime: 5 * 60 * 1000,
  });
  // getBusinessStaff already returns active members; no extra filter needed.
  const staffOptions = useMemo(
    () => staffQuery.data?.staff ?? [],
    [staffQuery.data],
  );

  const assignModal = useDisclosure();
  const [assignTemplate, setAssignTemplate] = useState<ChecklistTemplate | null>(null);
  const [assignStaffId, setAssignStaffId] = useState<number | null>(null);

  const startRunMutation = useMutation({
    mutationFn: (vars: { templateId: number; staffId: number }) =>
      checklistsApi.createRun(businessId, {
        template_id: vars.templateId,
        assigned_staff_id: vars.staffId,
      }),
    onSuccess: () => {
      toast.showSuccess(t("runStarted"));
      setAssignTemplate(null);
      setAssignStaffId(null);
      assignModal.onClose();
      void qc.invalidateQueries({ queryKey: runsKey });
    },
    onError: () => toast.showError(t("runStartError")),
  });

  const openAssign = useCallback(
    (tpl: ChecklistTemplate) => {
      setAssignTemplate(tpl);
      setAssignStaffId(null);
      assignModal.onOpen();
    },
    [assignModal],
  );

  const runStatusChipColor = (status: string) =>
    status === "complete"
      ? ("success" as const)
      : status === "in_progress"
        ? ("warning" as const)
        : ("default" as const);

  const runs = useMemo(() => runsQuery.data ?? [], [runsQuery.data]);

  const canSubmit = name.trim() !== "" && items.some((it) => it.label.trim() !== "");

  const setItem = (idx: number, patch: Partial<ItemRow>) =>
    setItems((cur) => cur.map((it, i) => (i === idx ? { ...it, ...patch } : it)));
  const addItem = () => setItems((cur) => [...cur, { label: "", required: false }]);
  const removeItem = (idx: number) =>
    setItems((cur) => (cur.length <= 1 ? cur : cur.filter((_, i) => i !== idx)));

  const templates = useMemo(() => listQuery.data ?? [], [listQuery.data]);
  const coldStart = !listQuery.isLoading && templates.length === 0;
  const showForm = composing || coldStart;

  return (
    <section aria-label={t("new")} className="space-y-4">
      {!showForm && !listQuery.isLoading ? (
        <div className="flex justify-end">
          <Button
            size="sm"
            variant="flat"
            className="bg-brand/10 font-medium text-brand"
            startContent={<Plus className="h-4 w-4" />}
            onPress={() => setComposing(true)}
          >
            {t("new")}
          </Button>
        </div>
      ) : null}

      {showForm ? (
      <form
        className="space-y-4 rounded-3xl border border-warm-200 bg-white p-4 shadow-sm shadow-warm-900/5"
        onSubmit={(e) => {
          e.preventDefault();
          if (canSubmit) createMutation.mutate();
        }}
      >
        <h3 className="text-sm font-semibold text-ink-950">{t("new")}</h3>
        <Input
          label={t("name")}
          placeholder={t("namePlaceholder")}
          value={name}
          onValueChange={setName}
        />
        <Select
          label={t("kind")}
          selectedKeys={[kind]}
          onChange={(e) => setKind(e.target.value as ChecklistKind)}
          classNames={{
            trigger:
              "rounded-2xl border border-warm-200 bg-warm-50/60 shadow-none data-[hover=true]:bg-warm-50",
            value: "text-ink-800",
          }}
        >
          {KINDS.map((k) => (
            <SelectItem key={k} value={k}>
              {t(`kinds.${k}`)}
            </SelectItem>
          ))}
        </Select>

        <div className="space-y-2">
          {items.map((it, idx) => (
            <div key={idx} className="flex items-center gap-2">
              <Input
                aria-label={`${t("itemLabel")} ${idx + 1}`}
                placeholder={t("itemPlaceholder")}
                value={it.label}
                onValueChange={(v) => setItem(idx, { label: v })}
              />
              <Switch
                size="sm"
                isSelected={it.required}
                onValueChange={(v) => setItem(idx, { required: v })}
                aria-label={t("required")}
              />
              {items.length > 1 ? (
                <button
                  type="button"
                  aria-label={t("removeItem")}
                  onClick={() => removeItem(idx)}
                  className="rounded-full p-1 text-ink-400 transition hover:bg-warm-100 hover:text-ink-700"
                >
                  <X className="h-4 w-4" />
                </button>
              ) : null}
            </div>
          ))}
          <button
            type="button"
            onClick={addItem}
            className="inline-flex items-center gap-1 rounded-xl px-2 py-1 text-sm font-semibold text-brand-dark transition hover:bg-brand/10"
          >
            <Plus className="h-4 w-4" aria-hidden="true" />
            {t("addItem")}
          </button>
        </div>

        <div className="flex justify-end">
          <Button
            type="submit"
            className="bg-brand font-semibold text-white shadow-sm shadow-brand/20 hover:bg-brand-dark"
            isDisabled={!canSubmit || createMutation.isPending}
            isLoading={createMutation.isPending}
            startContent={createMutation.isPending ? undefined : <ClipboardList className="h-4 w-4" />}
          >
            {createMutation.isPending ? t("saving") : t("save")}
          </Button>
        </div>
      </form>
      ) : null}

      {listQuery.isLoading ? (
        <div role="status" aria-label={t("empty")} className="flex justify-center py-8">
          <Spinner aria-hidden="true" />
        </div>
      ) : templates.length === 0 ? (
        <EmptyState icon={ClipboardList} title={t("empty")} subtitle={t("emptyHint")} />
      ) : (
        <ul className="space-y-2">
          {templates.map((tpl) => (
            <li
              key={tpl.id}
              className="flex items-center justify-between gap-3 rounded-2xl border border-warm-200 bg-white p-3 shadow-sm shadow-warm-900/5"
            >
              <span className="min-w-0 truncate text-sm font-semibold text-ink-900">{tpl.name}</span>
              <div className="flex shrink-0 items-center gap-2">
                <Chip
                  size="sm"
                  variant="flat"
                  className="border border-warm-200 bg-warm-100 text-ink-600"
                >
                  {t(`kinds.${tpl.kind}`)}
                </Chip>
                <Button
                  size="sm"
                  variant="flat"
                  className="bg-brand/10 font-medium text-brand"
                  startContent={<Play className="h-3.5 w-3.5" />}
                  onPress={() => openAssign(tpl)}
                >
                  {t("assign")}
                </Button>
              </div>
            </li>
          ))}
        </ul>
      )}

      {/* Operator run-status list: who has (and hasn't) finished an assigned run. */}
      <div className="space-y-2">
        <h3 className="text-sm font-semibold text-ink-950">{t("runsTitle")}</h3>
        {runsQuery.isLoading ? (
          <div role="status" aria-label={t("runsTitle")} className="flex justify-center py-6">
            <Spinner aria-hidden="true" />
          </div>
        ) : runs.length === 0 ? (
          <EmptyState icon={CheckCircle2} title={t("runsEmpty")} subtitle={t("runsEmptyHint")} compact />
        ) : (
          <ul className="space-y-2">
            {runs.map((run) => (
              <li
                key={run.id}
                className="flex items-center justify-between gap-3 rounded-2xl border border-warm-200 bg-white p-3 shadow-sm shadow-warm-900/5"
              >
                <div className="min-w-0">
                  <p className="truncate text-sm font-semibold text-ink-900">
                    {run.template_name}
                  </p>
                  <p className="truncate text-xs text-ink-500">
                    {run.assigned_staff_name || t("runUnassigned")}
                  </p>
                </div>
                <Chip size="sm" variant="flat" color={runStatusChipColor(run.status)}>
                  {t(`runStatus.${run.status}`)}
                </Chip>
              </li>
            ))}
          </ul>
        )}
      </div>

      {/* Assign / start a run from a template. */}
      <Modal isOpen={assignModal.isOpen} onOpenChange={assignModal.onOpenChange} size="md">
        <ModalContent>
          {(onClose) => (
            <>
              <ModalHeader className="flex flex-col gap-1">
                <span className="text-base font-semibold text-ink-950">
                  {t("assignTitle")}
                </span>
                {assignTemplate ? (
                  <span className="text-xs font-normal text-ink-500">
                    {assignTemplate.name}
                  </span>
                ) : null}
              </ModalHeader>
              <ModalBody>
                <StaffSelect
                  label={t("assignStaff")}
                  placeholder={t("assignStaffPlaceholder")}
                  staff={staffOptions}
                  value={assignStaffId}
                  onChange={setAssignStaffId}
                />
              </ModalBody>
              <ModalFooter>
                <Button variant="light" onPress={onClose}>
                  {t("assignCancel")}
                </Button>
                <Button
                  color="primary"
                  isLoading={startRunMutation.isPending}
                  isDisabled={assignStaffId == null || startRunMutation.isPending}
                  onPress={() => {
                    if (assignTemplate && assignStaffId != null) {
                      startRunMutation.mutate({
                        templateId: assignTemplate.id,
                        staffId: assignStaffId,
                      });
                    }
                  }}
                >
                  {t("assignStart")}
                </Button>
              </ModalFooter>
            </>
          )}
        </ModalContent>
      </Modal>
    </section>
  );
}
