"use client";

import { useCallback, useEffect, useState } from "react";
import { Card, CardBody, CardHeader, Chip, Button, Spinner } from "@nextui-org/react";
import { RefreshCw, Server, Activity } from "lucide-react";
import {
  getAdminSystemHealth,
  type AdminSystemHealth,
} from "@/api/adminSystem";
import Link from "next/link";
import { useAdminPolling } from "@/components/admin/primitives";

function statusColor(
  status: string,
): "success" | "warning" | "danger" | "default" {
  switch (status) {
    case "healthy":
    case "running":
      return "success";
    case "degraded":
    case "disabled":
      return "warning";
    case "unhealthy":
    case "stopped":
      return "danger";
    default:
      return "default";
  }
}

export function SystemHealthPanel() {
  const [health, setHealth] = useState<AdminSystemHealth | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  const load = useCallback(async (silent = false) => {
    if (!silent) {
      setLoading(true);
    }
    setError(null);
    try {
      const data = await getAdminSystemHealth();
      setHealth(data);
    } catch {
      if (!silent) {
        setError("Failed to load system health");
      }
    } finally {
      if (!silent) {
        setLoading(false);
      }
    }
  }, []);

  useEffect(() => {
    void load(false);
  }, [load]);

  useAdminPolling(() => load(true), 60_000, Boolean(health));

  if (loading && !health) {
    return (
      <Card className="border border-default-200 shadow-sm">
        <CardBody className="flex justify-center py-10">
          <Spinner />
        </CardBody>
      </Card>
    );
  }

  if (error || !health) {
    return (
      <Card className="border border-danger/30 shadow-sm">
        <CardBody className="flex items-center justify-between gap-4">
          <p className="text-danger text-sm">{error}</p>
          <Button size="sm" variant="flat" onPress={() => load(false)}>
            Retry
          </Button>
        </CardBody>
      </Card>
    );
  }

  return (
    <Card className="border border-default-200 shadow-sm">
      <CardHeader className="flex items-center justify-between px-5 pt-5 pb-2">
        <div className="flex items-center gap-2">
          <Server size={18} className="text-brand" />
          <p className="text-base font-semibold">System Health</p>
          <Chip size="sm" color={statusColor(health.status)} variant="flat">
            {health.status}
          </Chip>
        </div>
        <div className="flex items-center gap-3">
          <span className="text-xs text-default-400 hidden sm:inline">
            Uptime {health.uptime}
          </span>
          <Button
            isIconOnly
            size="sm"
            variant="light"
            aria-label="Refresh system health"
            onPress={() => load(false)}
            isLoading={loading}
          >
            <RefreshCw size={16} />
          </Button>
        </div>
      </CardHeader>
      <CardBody className="px-5 pb-5 pt-0 space-y-4">
        <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-3">
          {health.checks.map((check) => (
            <div
              key={check.service}
              className="rounded-lg border border-default-200 p-3"
            >
              <div className="flex items-center justify-between gap-2">
                <p className="text-sm font-medium capitalize">{check.service}</p>
                <Chip size="sm" color={statusColor(check.status)} variant="flat">
                  {check.status}
                </Chip>
              </div>
              {check.latency ? (
                <p className="text-xs text-default-400 mt-1">{check.latency}</p>
              ) : null}
              {check.message ? (
                <p className="text-xs text-default-500 mt-1">{check.message}</p>
              ) : null}
            </div>
          ))}
        </div>

        <div>
          <p className="text-xs font-semibold uppercase tracking-wider text-default-400 mb-2">
            Queues
          </p>
          <div className="flex flex-wrap gap-2">
            {health.queues.map((q) => (
              <Chip
                key={q.name}
                size="sm"
                variant="flat"
                color={q.count > 0 ? "warning" : "default"}
              >
                {q.label || q.name}: {q.count}
              </Chip>
            ))}
          </div>
        </div>

        <div>
          <p className="text-xs font-semibold uppercase tracking-wider text-default-400 mb-2">
            Background workers
          </p>
          <div className="grid grid-cols-1 sm:grid-cols-2 gap-2">
            {health.workers.map((worker) => (
              <div
                key={worker.name}
                className="flex items-center justify-between rounded-lg bg-default-50 px-3 py-2"
              >
                <div className="min-w-0">
                  <p className="text-sm font-medium truncate">{worker.name}</p>
                  {worker.detail ? (
                    <p className="text-xs text-default-400 truncate">
                      {worker.detail}
                    </p>
                  ) : null}
                </div>
                <Chip size="sm" color={statusColor(worker.status)} variant="flat">
                  {worker.status}
                </Chip>
              </div>
            ))}
          </div>
        </div>

        {(health.queues.find((q) => q.name === "errors_1h")?.count ?? 0) > 0 && (
          <Link
            href="/admin/errors"
            className="inline-flex items-center gap-1 text-xs text-brand hover:underline"
          >
            <Activity size={14} />
            View recent errors
          </Link>
        )}
      </CardBody>
    </Card>
  );
}
