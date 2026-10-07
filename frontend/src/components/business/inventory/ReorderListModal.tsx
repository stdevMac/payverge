"use client";

import React, { useCallback, useMemo } from "react";
import {
  Modal,
  ModalContent,
  ModalHeader,
  ModalBody,
  ModalFooter,
  Button,
} from "@nextui-org/react";
import { EmptyState } from "@/components/ui/EmptyState";
import { ClipboardList } from "lucide-react";
import { localDateKey } from "@/lib/localDate";
import { useIframePrint } from "../printers/useIframePrint";
import { downloadCsv } from "./inventoryCsv";
import {
  buildReorderCsv,
  type ReorderRow,
} from "./reorderList";

interface ReorderListModalProps {
  isOpen: boolean;
  onClose: () => void;
  rows: ReorderRow[];
  t: (key: string) => string;
}

function buildPrintHtml(rows: ReorderRow[], headers: string[], title: string): string {
  const head = headers.map((h) => `<th>${escapeHtml(h)}</th>`).join("");
  const body = rows
    .map(
      (r) =>
        `<tr><td>${escapeHtml(r.name)}</td><td>${escapeHtml(r.unit)}</td><td>${r.onHand}</td><td>${r.threshold}</td><td>${r.suggested}</td></tr>`,
    )
    .join("");
  return `<!doctype html><html><head><meta charset="utf-8"/><title>${escapeHtml(title)}</title>
<style>
  body { font-family: system-ui, sans-serif; color: #1c1917; padding: 24px; }
  h1 { font-size: 18px; margin: 0 0 16px; }
  table { width: 100%; border-collapse: collapse; font-size: 13px; }
  th, td { border: 1px solid #e7e5e4; padding: 8px 10px; text-align: left; }
  th { background: #fafaf9; }
</style></head><body>
<h1>${escapeHtml(title)}</h1>
<table><thead><tr>${head}</tr></thead><tbody>${body}</tbody></table>
</body></html>`;
}

function escapeHtml(value: string): string {
  return String(value)
    .replace(/&/g, "&amp;")
    .replace(/</g, "&lt;")
    .replace(/>/g, "&gt;")
    .replace(/"/g, "&quot;");
}

export default function ReorderListModal({
  isOpen,
  onClose,
  rows,
  t,
}: ReorderListModalProps) {
  const { print } = useIframePrint();

  const headers = useMemo(
    () => [
      t("reorder.colName"),
      t("reorder.colUnit"),
      t("reorder.colOnHand"),
      t("reorder.colThreshold"),
      t("reorder.colSuggested"),
    ],
    [t],
  );

  const handleExport = useCallback(() => {
    const csv = buildReorderCsv(rows, headers);
    downloadCsv(`reorder_list_${localDateKey()}.csv`, csv);
  }, [rows, headers]);

  const handlePrint = useCallback(() => {
    void print(buildPrintHtml(rows, headers, t("reorder.title")));
  }, [print, rows, headers, t]);

  return (
    <Modal isOpen={isOpen} onClose={onClose} size="2xl" scrollBehavior="inside">
      <ModalContent>
        {(close) => (
          <>
            <ModalHeader className="flex flex-col gap-1">
              <span className="text-lg font-semibold text-ink-900">
                {t("reorder.title")}
              </span>
              <span className="text-sm font-normal text-ink-600">
                {t("reorder.subtitle")}
              </span>
            </ModalHeader>
            <ModalBody>
              {rows.length === 0 ? (
                <EmptyState
                  compact
                  icon={ClipboardList}
                  title={t("reorder.empty")}
                  subtitle=""
                />
              ) : (
                <div className="overflow-x-auto rounded-xl border border-warm-200">
                  <table className="min-w-full text-sm">
                    <thead className="bg-warm-50 text-left text-xs font-semibold uppercase tracking-wide text-ink-600">
                      <tr>
                        {headers.map((h) => (
                          <th key={h} className="px-3 py-2">
                            {h}
                          </th>
                        ))}
                      </tr>
                    </thead>
                    <tbody className="divide-y divide-warm-100">
                      {rows.map((row) => (
                        <tr key={row.id} className="text-ink-800">
                          <td className="px-3 py-2 font-medium text-ink-900">
                            {row.name}
                          </td>
                          <td className="px-3 py-2">{row.unit}</td>
                          <td className="px-3 py-2 tabular-nums">{row.onHand}</td>
                          <td className="px-3 py-2 tabular-nums">
                            {row.threshold}
                          </td>
                          <td className="px-3 py-2 tabular-nums font-semibold">
                            {row.suggested}
                          </td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
                </div>
              )}
            </ModalBody>
            <ModalFooter>
              <Button
                variant="bordered"
                onPress={handleExport}
                isDisabled={rows.length === 0}
              >
                {t("reorder.exportCsv")}
              </Button>
              <Button
                className="bg-brand text-white"
                onPress={handlePrint}
                isDisabled={rows.length === 0}
              >
                {t("reorder.print")}
              </Button>
            </ModalFooter>
          </>
        )}
      </ModalContent>
    </Modal>
  );
}
