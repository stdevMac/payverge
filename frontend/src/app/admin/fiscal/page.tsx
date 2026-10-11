"use client";

import { useCallback, useEffect, useMemo, useState } from "react";
import {
  Card,
  CardBody,
  CardHeader,
  Button,
  Spinner,
  Chip,
  Pagination,
  Tabs,
  Tab,
} from "@nextui-org/react";
import { RefreshCw, FileText, RotateCcw } from "lucide-react";
import {
  getAdminFiscalSummary,
  getAdminFiscalJobs,
  getAdminFiscalReceipts,
  requeueFiscalJob,
  isFiscalJobRequeueable,
  type AdminFiscalSummary,
  type FiscalJobRow,
  type FiscalReceiptRow,
} from "@/api/adminFiscal";
import { AdminPageFrame, formatAdminDate } from "@/components/admin/primitives";
import { formatAdminCurrency } from "@/utils/adminCurrency";
import Link from "next/link";
import { apiErrorDetail } from "@/utils/apiError";

const PAGE_SIZE = 20;

function centsToMoney(cents: number, currency = "USD") {
  return formatAdminCurrency(cents / 100, { currency });
}

export default function AdminFiscalPage() {
  const [summary, setSummary] = useState<AdminFiscalSummary | null>(null);
  const [jobs, setJobs] = useState<FiscalJobRow[]>([]);
  const [receipts, setReceipts] = useState<FiscalReceiptRow[]>([]);
  const [jobTotal, setJobTotal] = useState(0);
  const [receiptTotal, setReceiptTotal] = useState(0);
  const [jobPage, setJobPage] = useState(1);
  const [receiptPage, setReceiptPage] = useState(1);
  const [tab, setTab] = useState("jobs");
  const [loading, setLoading] = useState(true);
  const [requeueing, setRequeueing] = useState<number | null>(null);
  const [requeueError, setRequeueError] = useState<string | null>(null);

  const jobPages = useMemo(() => Math.max(1, Math.ceil(jobTotal / PAGE_SIZE)), [jobTotal]);
  const receiptPages = useMemo(() => Math.max(1, Math.ceil(receiptTotal / PAGE_SIZE)), [receiptTotal]);

  const load = useCallback(async () => {
    setLoading(true);
    try {
      const [sum, jobData, receiptData] = await Promise.all([
        getAdminFiscalSummary(),
        getAdminFiscalJobs({ page: jobPage, limit: PAGE_SIZE }),
        getAdminFiscalReceipts({
          page: receiptPage,
          limit: PAGE_SIZE,
          status: "failed_retryable",
        }),
      ]);
      setSummary(sum);
      setJobs(jobData.jobs);
      setJobTotal(jobData.total);
      setReceipts(receiptData.receipts);
      setReceiptTotal(receiptData.total);
    } finally {
      setLoading(false);
    }
  }, [jobPage, receiptPage]);

  useEffect(() => {
    load();
  }, [load]);

  const handleRequeue = async (id: number, status: string) => {
    setRequeueing(id);
    setRequeueError(null);
    try {
      await requeueFiscalJob(id, status === "failed_permanent");
      await load();
    } catch (err: unknown) {
      setRequeueError(apiErrorDetail(err) || "Failed to requeue fiscal job");
    } finally {
      setRequeueing(null);
    }
  };

  return (
    <AdminPageFrame
      description="Platform-wide fiscal compliance — AFIP jobs, receipts, and failures"
      actions={
        <Button variant="bordered" onPress={load} isLoading={loading} startContent={<RefreshCw size={16} />}>
          Refresh
        </Button>
      }
    >
      {requeueError && (
        <Card className="border border-danger/30 bg-danger-50/30">
          <CardBody>
            <p className="text-sm text-danger">{requeueError}</p>
          </CardBody>
        </Card>
      )}

      {summary && (
        <div className="grid grid-cols-2 lg:grid-cols-4 gap-3">
          <Card><CardBody><p className="text-xs text-default-400">Businesses w/ fiscal</p><p className="text-2xl font-semibold">{summary.businesses_with_fiscal}</p></CardBody></Card>
          <Card><CardBody><p className="text-xs text-default-400">Due jobs</p><p className="text-2xl font-semibold">{summary.due_jobs}</p></CardBody></Card>
          <Card><CardBody><p className="text-xs text-default-400">Failed (retryable)</p><p className="text-2xl font-semibold text-warning">{summary.failed_retryable_jobs}</p></CardBody></Card>
          <Card><CardBody><p className="text-xs text-default-400">Failed (permanent)</p><p className="text-2xl font-semibold text-danger">{summary.failed_permanent_jobs}</p></CardBody></Card>
        </div>
      )}

      <Card>
        <CardHeader className="flex items-center gap-2">
          <FileText size={18} className="text-brand" />
          <p className="font-semibold">Fiscal queue</p>
        </CardHeader>
        <CardBody>
          <Tabs selectedKey={tab} onSelectionChange={(k) => setTab(String(k))}>
            <Tab key="jobs" title="Jobs" />
            <Tab key="receipts" title="Failed receipts" />
          </Tabs>

          {loading ? (
            <div className="flex justify-center py-12"><Spinner /></div>
          ) : tab === "jobs" ? (
            <div className="mt-4 overflow-x-auto">
              <table className="w-full text-sm">
                <thead>
                  <tr className="border-b text-left text-default-500">
                    <th className="py-2 pr-3">ID</th>
                    <th className="py-2 pr-3">Business</th>
                    <th className="py-2 pr-3">Bill</th>
                    <th className="py-2 pr-3">Action</th>
                    <th className="py-2 pr-3">Status</th>
                    <th className="py-2 pr-3">Attempts</th>
                    <th className="py-2 pr-3">Error</th>
                    <th className="py-2 pr-3">Actions</th>
                  </tr>
                </thead>
                <tbody>
                  {jobs.map((job) => (
                    <tr key={job.id} className="border-b border-default-100 align-top">
                      <td className="py-2 pr-3">#{job.id}</td>
                      <td
                        className="py-2 pr-3"
                        data-testid={`fiscal-job-business-${job.id}`}
                      >
                        <Link
                          href={`/admin/businesses`}
                          className="text-brand hover:underline"
                        >
                          {job.business_name?.trim()
                            ? job.business_name
                            : `Business #${job.business_id}`}
                        </Link>
                      </td>
                      <td className="py-2 pr-3">#{job.bill_id}</td>
                      <td className="py-2 pr-3">{job.action}</td>
                      <td className="py-2 pr-3"><Chip size="sm" variant="flat">{job.status}</Chip></td>
                      <td className="py-2 pr-3">{job.attempts}/{job.max_attempts}</td>
                      <td className="py-2 pr-3 max-w-[16rem] text-xs text-default-500">{job.last_error_message || "—"}</td>
                      <td className="py-2 pr-3">
                        {isFiscalJobRequeueable(job.status) && (
                          <Button
                            size="sm"
                            variant="flat"
                            isDisabled={false}
                            isLoading={requeueing === job.id}
                            startContent={<RotateCcw size={14} />}
                            onPress={() => handleRequeue(job.id, job.status)}
                            data-testid={`fiscal-job-requeue-${job.id}`}
                          >
                            Requeue
                          </Button>
                        )}
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
              {jobPages > 1 && (
                <div className="flex justify-center pt-4">
                  <Pagination total={jobPages} page={jobPage} onChange={setJobPage} showControls classNames={{ cursor: "bg-brand text-white" }} />
                </div>
              )}
            </div>
          ) : (
            <div className="mt-4 overflow-x-auto">
              <table className="w-full text-sm">
                <thead>
                  <tr className="border-b text-left text-default-500">
                    <th className="py-2 pr-3">ID</th>
                    <th className="py-2 pr-3">Business</th>
                    <th className="py-2 pr-3">Type</th>
                    <th className="py-2 pr-3">Amount</th>
                    <th className="py-2 pr-3">Status</th>
                    <th className="py-2 pr-3">Updated</th>
                    <th className="py-2 pr-3">Error</th>
                  </tr>
                </thead>
                <tbody>
                  {receipts.map((r) => (
                    <tr key={r.id} className="border-b border-default-100 align-top">
                      <td className="py-2 pr-3">#{r.id}</td>
                      <td className="py-2 pr-3">
                        {r.business_name?.trim()
                          ? r.business_name
                          : `Business #${r.business_id}`}
                      </td>
                      <td className="py-2 pr-3">{r.receipt_type} ({r.provider})</td>
                      <td className="py-2 pr-3">{centsToMoney(r.total_amount_cents, r.currency)}</td>
                      <td className="py-2 pr-3"><Chip size="sm" color="danger" variant="flat">{r.status}</Chip></td>
                      <td className="py-2 pr-3 text-default-500">{formatAdminDate(r.updated_at, true)}</td>
                      <td className="py-2 pr-3 max-w-[16rem] text-xs">{r.error_message || "—"}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
              {receiptPages > 1 && (
                <div className="flex justify-center pt-4">
                  <Pagination total={receiptPages} page={receiptPage} onChange={setReceiptPage} showControls classNames={{ cursor: "bg-brand text-white" }} />
                </div>
              )}
            </div>
          )}
        </CardBody>
      </Card>
    </AdminPageFrame>
  );
}
