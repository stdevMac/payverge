"use client";

import { useCallback, useEffect, useMemo, useState } from "react";
import Link from "next/link";
import {
  Button,
  Card,
  CardBody,
  CardHeader,
  Chip,
  Divider,
  Spinner,
  type ChipProps,
} from "@nextui-org/react";
import {
  Activity,
  CheckCircle2,
  ExternalLink,
  KeyRound,
  RefreshCw,
  RotateCcw,
  ShieldCheck,
  Sparkles,
  TriangleAlert,
  Users,
} from "lucide-react";
import {
  appendAdminDemoDay,
  ensureAdminDemo,
  getAdminDemo,
  resetAdminDemo,
  verifyAdminDemo,
  type AdminDemoAccessIdentity,
  type AdminDemoBusiness,
  type AdminDemoCoverageCheck,
  type AdminDemoSummary,
} from "@/api/adminDemo";

function statusColor(status?: string): ChipProps["color"] {
  switch (status?.toLowerCase()) {
    case "ready":
    case "passed":
    case "succeeded":
    case "fresh":
      return "success";
    case "failed":
    case "stale":
      return "danger";
    case "creating":
    case "resetting":
    case "running":
      return "warning";
    default:
      return "default";
  }
}

function formatDate(value?: string | null): string {
  if (!value) return "Not generated";
  return new Intl.DateTimeFormat("en-US", {
    month: "short",
    day: "numeric",
    year: "numeric",
  }).format(new Date(value));
}

function labelForCoverage(key: string): string {
  return key.replaceAll("_", " ");
}

function DemoBusinessCard({ business }: { business: AdminDemoBusiness }) {
  const publicSlug = business.custom_url || business.business_id;

  return (
    <Card className="border border-default-200 shadow-sm">
      <CardBody className="gap-4 p-5">
        <div className="flex items-start justify-between gap-3">
          <div className="min-w-0">
            <p className="truncate text-base font-semibold">{business.name}</p>
            <p className="mt-1 text-sm text-default-500">
              {business.business_id || `business-${business.id}`}
            </p>
          </div>
        </div>
        <div className="flex flex-wrap gap-2">
          <Button
            as={Link}
            href={`/business/${business.id}/dashboard`}
            size="sm"
            color="primary"
            startContent={<ExternalLink size={16} />}
          >
            Dashboard
          </Button>
          {publicSlug && (
            <Button
              as={Link}
              href={`/b/${publicSlug}`}
              size="sm"
              variant="flat"
              startContent={<ExternalLink size={16} />}
            >
              Public Page
            </Button>
          )}
        </div>
      </CardBody>
    </Card>
  );
}

function CoverageRow({ check }: { check: AdminDemoCoverageCheck }) {
  const passed = check.status === "passed";
  return (
    <div className="flex items-center justify-between gap-3 rounded-lg border border-default-200 px-3 py-2">
      <div className="flex min-w-0 items-center gap-2">
        {passed ? (
          <CheckCircle2 size={16} className="shrink-0 text-success" />
        ) : (
          <TriangleAlert size={16} className="shrink-0 text-danger" />
        )}
        <div className="min-w-0">
          <p className="truncate text-sm font-medium">
            {labelForCoverage(check.key)}
          </p>
          {check.message && (
            <p className="truncate text-xs text-danger">{check.message}</p>
          )}
        </div>
      </div>
      <Chip size="sm" variant="flat" color={statusColor(check.status)}>
        {check.count}
      </Chip>
    </div>
  );
}

function AccessCard({ access }: { access: AdminDemoAccessIdentity[] }) {
  const grouped = access.reduce<Record<string, AdminDemoAccessIdentity[]>>(
    (acc, identity) => {
      const key = `${identity.business_id}:${identity.business_name}`;
      acc[key] = acc[key] || [];
      acc[key].push(identity);
      return acc;
    },
    {},
  );

  return (
    <Card className="border border-default-200 shadow-sm">
      <CardHeader className="flex items-center gap-2 px-5 pt-5 pb-2">
        <Users size={18} className="text-brand" />
        <p className="text-base font-semibold">Demo Staff Access</p>
      </CardHeader>
      <CardBody className="gap-4 p-5 pt-2">
        {Object.entries(grouped).map(([key, identities], index) => (
          <div key={key} className="space-y-2">
            {index > 0 && <Divider />}
            <p className="text-sm font-medium">{identities[0].business_name}</p>
            <div className="grid gap-2 sm:grid-cols-2">
              {identities.map((identity) => (
                <div
                  key={`${identity.business_id}-${identity.email}`}
                  className="rounded-lg border border-default-200 px-3 py-2"
                >
                  <div className="flex items-center justify-between gap-2">
                    <p className="truncate text-sm font-medium">
                      {identity.name}
                    </p>
                    <Chip size="sm" variant="flat">
                      {identity.role}
                    </Chip>
                  </div>
                  <p className="mt-1 truncate text-xs text-default-500">
                    {identity.email}
                  </p>
                  {identity.pin_hint && (
                    <p className="mt-1 flex items-center gap-1 text-xs text-default-500">
                      <KeyRound size={12} />
                      Kiosk PIN {identity.pin_hint}
                    </p>
                  )}
                </div>
              ))}
            </div>
          </div>
        ))}
        {access.length === 0 && (
          <p className="text-sm text-default-500">No demo staff generated yet.</p>
        )}
      </CardBody>
    </Card>
  );
}

export default function AdminDemoPage() {
  const [summary, setSummary] = useState<AdminDemoSummary | null>(null);
  const [loading, setLoading] = useState(true);
  const [action, setAction] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);

  const load = useCallback(async () => {
    try {
      setError(null);
      const data = await getAdminDemo();
      setSummary(data);
    } catch (err) {
      console.error("Failed to load admin demo:", err);
      setError("Failed to load demo state");
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    void load();
  }, [load]);

  const runSummaryAction = useCallback(
    async (name: string, fn: () => Promise<AdminDemoSummary>) => {
      try {
        setAction(name);
        setError(null);
        const data = await fn();
        setSummary(data);
      } catch (err) {
        console.error(`Admin demo ${name} failed:`, err);
        setError(`Failed to ${name.toLowerCase()} demo`);
      } finally {
        setAction(null);
      }
    },
    [],
  );

  const runVerify = useCallback(async () => {
    try {
      setAction("Verify");
      setError(null);
      const verification = await verifyAdminDemo();
      setSummary((current) =>
        current ? { ...current, verification } : current,
      );
    } catch (err) {
      console.error("Admin demo verify failed:", err);
      setError("Failed to verify demo");
    } finally {
      setAction(null);
    }
  }, []);

  const sortedCoverage = useMemo(
    () =>
      [...(summary?.verification.coverage || [])].sort((a, b) =>
        a.key.localeCompare(b.key),
      ),
    [summary?.verification.coverage],
  );

  if (loading) {
    return (
      <div className="flex min-h-[420px] items-center justify-center p-8">
        <Spinner size="lg" aria-label="Loading demo" />
      </div>
    );
  }

  return (
    <div className="flex flex-col gap-6 p-4 sm:p-6 md:p-8">
      <div className="flex flex-col gap-4 lg:flex-row lg:items-end lg:justify-between">
        <div>
          <h1 className="font-serif text-3xl font-bold text-foreground">
            Demo Center
          </h1>
          <p className="mt-1 text-default-500">
            Isolated restaurant demos for the current admin account
          </p>
        </div>
        <div className="flex flex-wrap gap-2">
          <Button
            variant="flat"
            startContent={<RefreshCw size={16} />}
            isLoading={action === "Ensure"}
            onPress={() => void runSummaryAction("Ensure", ensureAdminDemo)}
          >
            Ensure
          </Button>
          <Button
            variant="flat"
            startContent={<Activity size={16} />}
            isLoading={action === "Append Day"}
            onPress={() =>
              void runSummaryAction("Append Day", appendAdminDemoDay)
            }
          >
            Append Day
          </Button>
          <Button
            variant="flat"
            startContent={<ShieldCheck size={16} />}
            isLoading={action === "Verify"}
            onPress={() => void runVerify()}
          >
            Verify
          </Button>
          <Button
            color="danger"
            variant="flat"
            startContent={<RotateCcw size={16} />}
            isLoading={action === "Reset"}
            onPress={() => void runSummaryAction("Reset", resetAdminDemo)}
          >
            Reset
          </Button>
        </div>
      </div>

      {error && (
        <div className="rounded-lg border border-danger/30 bg-danger/5 px-4 py-3 text-sm text-danger">
          {error}
        </div>
      )}

      <div className="grid grid-cols-1 gap-4 md:grid-cols-2 xl:grid-cols-4">
        <Card className="border border-default-200 shadow-sm">
          <CardBody className="gap-2 p-5">
            <p className="text-sm text-default-500">Instance</p>
            <div className="flex items-center gap-2">
              <Chip
                size="sm"
                variant="flat"
                color={statusColor(summary?.instance?.status)}
              >
                {summary?.instance?.status || "missing"}
              </Chip>
              <span className="text-sm text-default-500">
                seed {summary?.instance?.seed_version || "unknown"}
              </span>
            </div>
          </CardBody>
        </Card>
        <Card className="border border-default-200 shadow-sm">
          <CardBody className="gap-2 p-5">
            <p className="text-sm text-default-500">Generated Window</p>
            <p className="text-lg font-semibold">
              {formatDate(summary?.instance?.baseline_start_date)} -{" "}
              {formatDate(summary?.instance?.last_simulated_business_date)}
            </p>
          </CardBody>
        </Card>
        <Card className="border border-default-200 shadow-sm">
          <CardBody className="gap-2 p-5">
            <p className="text-sm text-default-500">Append heartbeat</p>
            <div className="flex flex-wrap items-center gap-2">
              <Chip
                size="sm"
                variant="flat"
                color={statusColor(summary?.heartbeat?.status)}
              >
                {summary?.heartbeat?.status || "unknown"}
              </Chip>
              <span className="text-sm text-default-500">
                last {formatDate(summary?.heartbeat?.last_append_at)}
              </span>
            </div>
            {summary?.heartbeat?.status === "stale" && (
              <p className="text-xs text-danger">
                Hourly append is behind — demo data may be frozen.
              </p>
            )}
          </CardBody>
        </Card>
        <Card className="border border-default-200 shadow-sm">
          <CardBody className="gap-2 p-5">
            <p className="text-sm text-default-500">Verification</p>
            <div className="flex items-center gap-2">
              <Chip
                size="sm"
                variant="flat"
                color={statusColor(summary?.verification.status)}
              >
                {summary?.verification.status || "unknown"}
              </Chip>
              <span className="text-sm text-default-500">
                {summary?.verification.errors.length || 0} errors
              </span>
            </div>
          </CardBody>
        </Card>
      </div>

      <div className="grid grid-cols-1 gap-4 lg:grid-cols-2">
        {summary?.businesses.map((business) => (
          <DemoBusinessCard key={business.id} business={business} />
        ))}
      </div>

      <AccessCard access={summary?.access || []} />

      <div className="grid grid-cols-1 gap-4 xl:grid-cols-[minmax(0,1.3fr)_minmax(360px,0.7fr)]">
        <Card className="border border-default-200 shadow-sm">
          <CardHeader className="flex items-center justify-between px-5 pt-5 pb-2">
            <div className="flex items-center gap-2">
              <Sparkles size={18} className="text-brand" />
              <p className="text-base font-semibold">Coverage</p>
            </div>
            <Chip
              size="sm"
              variant="flat"
              color={statusColor(summary?.verification.status)}
            >
              {
                sortedCoverage.filter((check) => check.status === "passed")
                  .length
              }
              /{sortedCoverage.length}
            </Chip>
          </CardHeader>
          <CardBody className="grid grid-cols-1 gap-2 p-5 pt-2 sm:grid-cols-2">
            {sortedCoverage.map((check) => (
              <CoverageRow key={check.key} check={check} />
            ))}
          </CardBody>
        </Card>

        <Card className="border border-default-200 shadow-sm">
          <CardHeader className="px-5 pt-5 pb-2">
            <p className="text-base font-semibold">Recent Runs</p>
          </CardHeader>
          <CardBody className="gap-3 p-5 pt-2">
            {(summary?.runs || []).map((run) => (
              <div
                key={run.id}
                className="flex items-center justify-between gap-3 rounded-lg border border-default-200 px-3 py-2"
              >
                <div className="min-w-0">
                  <p className="truncate text-sm font-medium">{run.run_type}</p>
                  <p className="text-xs text-default-500">
                    {formatDate(run.started_at)}
                  </p>
                </div>
                <Chip size="sm" variant="flat" color={statusColor(run.status)}>
                  {run.status}
                </Chip>
              </div>
            ))}
            {summary?.verification.errors.map((message) => (
              <div
                key={message}
                className="rounded-lg border border-danger/30 bg-danger/5 px-3 py-2 text-sm text-danger"
              >
                {message}
              </div>
            ))}
          </CardBody>
        </Card>
      </div>
    </div>
  );
}
