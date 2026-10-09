"use client";
import React, {
  useEffect,
  useState,
  useCallback,
  useMemo,
  useRef,
} from "react";
import {
  Card,
  CardBody,
  CardHeader,
  Input,
  Table,
  TableHeader,
  TableColumn,
  TableBody,
  TableRow,
  TableCell,
  Chip,
  Pagination,
  Spinner,
} from "@nextui-org/react";
import { errorLogsAPI, ErrorLog } from "@/api/errorLogs";
import { Search, TriangleAlert } from "lucide-react";

const PAGE_SIZE = 20;

function formatTimestamp(dateStr: string | null | undefined): string {
  if (!dateStr) return "N/A";
  return new Date(dateStr).toLocaleString("en-US", {
    year: "numeric",
    month: "short",
    day: "numeric",
    hour: "2-digit",
    minute: "2-digit",
    second: "2-digit",
  });
}

function truncateMessage(msg: string, maxLen = 80): string {
  if (msg.length <= maxLen) return msg;
  return msg.slice(0, maxLen) + "...";
}

// ─── Expanded Detail ────────────────────────────────────────────────────────
function ErrorDetail({ log }: { log: ErrorLog }) {
  return (
    <Card shadow="sm" className="mt-4">
      <CardHeader className="pb-2">
        <div className="flex items-center gap-2">
          <TriangleAlert size={16} className="text-danger" />
          <h3 className="text-lg font-semibold">Error Detail</h3>
        </div>
      </CardHeader>
      <CardBody className="gap-4 text-sm">
        <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
          <DetailRow label="ID" value={String(log.id)} />
          <DetailRow label="Timestamp" value={formatTimestamp(log.timestamp || log.created_at)} />
          <DetailRow label="Source" value={log.source} />
          <DetailRow label="Component" value={log.component} />
          <DetailRow label="Function" value={log.function} />
          {log.request_id && <DetailRow label="Request ID" value={log.request_id} />}
        </div>

        <div>
          <p className="text-default-500 text-xs uppercase tracking-wider mb-1">Message</p>
          <p className="text-sm font-medium text-foreground break-words">{log.message}</p>
        </div>

        {log.error && log.error !== log.message && (
          <div>
            <p className="text-default-500 text-xs uppercase tracking-wider mb-1">Error</p>
            <p className="text-sm text-danger break-words">{log.error}</p>
          </div>
        )}

        {log.stack && (
          <div>
            <p className="text-default-500 text-xs uppercase tracking-wider mb-1">Stack Trace</p>
            <pre className="text-xs bg-default-50 p-4 rounded-lg overflow-auto max-h-64 text-default-600 font-mono whitespace-pre-wrap break-words">
              {log.stack}
            </pre>
          </div>
        )}
      </CardBody>
    </Card>
  );
}

function DetailRow({ label, value }: { label: string; value: string }) {
  return (
    <div className="flex flex-col gap-0.5">
      <span className="text-default-500 text-xs uppercase tracking-wider">{label}</span>
      <span className="font-medium text-sm break-all">{value}</span>
    </div>
  );
}

// ─── Main Page ──────────────────────────────────────────────────────────────
export default function ErrorLogsPage() {
  const [errors, setErrors] = useState<ErrorLog[]>([]);
  const [total, setTotal] = useState(0);
  const [page, setPage] = useState(1);
  const [loading, setLoading] = useState(true);
  const [sourceFilter, setSourceFilter] = useState("");
  const [componentFilter, setComponentFilter] = useState("");
  const [selectedErrorId, setSelectedErrorId] = useState<number | null>(null);
  const detailRef = useRef<HTMLDivElement | null>(null);

  const totalPages = useMemo(() => Math.max(1, Math.ceil(total / PAGE_SIZE)), [total]);

  const fetchErrors = useCallback(async () => {
    setLoading(true);
    try {
      const offset = (page - 1) * PAGE_SIZE;
      const data = await errorLogsAPI.getErrors({
        offset,
        limit: PAGE_SIZE,
        source: sourceFilter || undefined,
        component: componentFilter || undefined,
      });
      setErrors(data.errors || []);
      setTotal(data.total || 0);
    } catch {
      setErrors([]);
      setTotal(0);
    } finally {
      setLoading(false);
    }
  }, [page, sourceFilter, componentFilter]);

  useEffect(() => {
    fetchErrors();
  }, [fetchErrors]);

  useEffect(() => {
    setPage(1);
  }, [sourceFilter, componentFilter]);

  const selectedError = useMemo(
    () => errors.find((e) => e.id === selectedErrorId) || null,
    [errors, selectedErrorId]
  );

  // Scroll the detail panel into view when a row is selected — the panel
  // renders below the whole table and is otherwise easy to miss.
  useEffect(() => {
    if (selectedError) {
      detailRef.current?.scrollIntoView({
        behavior: "smooth",
        block: "nearest",
      });
    }
  }, [selectedError]);

  // Changing pages keeps the selected id from a row that no longer exists on
  // the new page, which silently empties the detail. Clear it on page change.
  const handlePageChange = (nextPage: number) => {
    setSelectedErrorId(null);
    setPage(nextPage);
  };

  const handleRowClick = (errorId: number) => {
    setSelectedErrorId(selectedErrorId === errorId ? null : errorId);
  };

  return (
    <div className="p-6 max-w-7xl mx-auto space-y-6">
      {/* Header */}
      <div>
        <h1 className="text-2xl font-bold text-foreground">Error Logs</h1>
        <p className="text-sm text-default-400 mt-1">
          Monitor and investigate application errors
        </p>
      </div>

      {/* Filters */}
      <Card shadow="sm">
        <CardBody>
          <div className="flex flex-col sm:flex-row gap-4">
            <Input
              className="flex-1"
              placeholder="Filter by source..."
              startContent={<Search size={16} className="text-default-300" />}
              value={sourceFilter}
              onValueChange={setSourceFilter}
              size="sm"
              isClearable
              onClear={() => setSourceFilter("")}
            />
            <Input
              className="flex-1"
              placeholder="Filter by component..."
              startContent={<Search size={16} className="text-default-300" />}
              value={componentFilter}
              onValueChange={setComponentFilter}
              size="sm"
              isClearable
              onClear={() => setComponentFilter("")}
            />
          </div>
        </CardBody>
      </Card>

      {/* Table */}
      <Card shadow="sm">
        <CardBody className="p-0">
          <Table
            aria-label="Error logs table"
            removeWrapper
            selectionMode="single"
            selectedKeys={selectedErrorId ? new Set([String(selectedErrorId)]) : new Set()}
            onSelectionChange={(keys) => {
              if (keys === "all") return;
              const selected = Array.from(keys)[0];
              if (selected) handleRowClick(Number(selected));
              else setSelectedErrorId(null);
            }}
            classNames={{
              th: "bg-default-50 text-default-600 text-xs uppercase tracking-wider",
              td: "py-3",
              tr: "cursor-pointer hover:bg-default-50 transition-colors",
            }}
            bottomContent={
              totalPages > 1 ? (
                <div className="flex justify-center py-4">
                  <Pagination
                    total={totalPages}
                    page={page}
                    onChange={handlePageChange}
                    showControls
                    classNames={{
                      cursor: "bg-brand text-white",
                    }}
                  />
                </div>
              ) : null
            }
          >
            <TableHeader>
              <TableColumn>Timestamp</TableColumn>
              <TableColumn>Message</TableColumn>
              <TableColumn>Source</TableColumn>
              <TableColumn>Component</TableColumn>
              <TableColumn>Function</TableColumn>
            </TableHeader>
            <TableBody
              isLoading={loading}
              loadingContent={<Spinner color="primary" />}
              emptyContent="No error logs found."
              items={errors}
            >
              {(log) => (
                <TableRow key={String(log.id)}>
                  <TableCell>
                    <span className="text-sm text-default-400 whitespace-nowrap">
                      {formatTimestamp(log.timestamp || log.created_at)}
                    </span>
                  </TableCell>
                  <TableCell>
                    <span className="text-sm font-medium">
                      {truncateMessage(log.message)}
                    </span>
                  </TableCell>
                  <TableCell>
                    <Chip size="sm" variant="flat" color="default">
                      {log.source}
                    </Chip>
                  </TableCell>
                  <TableCell>
                    <Chip size="sm" variant="flat" color="primary">
                      {log.component}
                    </Chip>
                  </TableCell>
                  <TableCell>
                    <span className="text-sm text-default-500 font-mono text-xs">
                      {log.function}
                    </span>
                  </TableCell>
                </TableRow>
              )}
            </TableBody>
          </Table>
        </CardBody>
      </Card>

      {/* Detail Panel */}
      {selectedError && (
        <div ref={detailRef}>
          <ErrorDetail log={selectedError} />
        </div>
      )}
    </div>
  );
}
