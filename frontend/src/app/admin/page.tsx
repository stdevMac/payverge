"use client";
import {
  getAdminStats,
  AdminStats,
  AdminAction,
  ErrorLogSummary,
} from "@/api/admin";
import { Card, CardBody, CardHeader, Spinner, Chip } from "@nextui-org/react";
import { useCallback, useEffect, useState } from "react";
import Link from "next/link";
import {
  Users,
  Building2,
  DollarSign,
  FlaskConical,
  TriangleAlert,
  Receipt,
  TrendingUp,
  Activity,
  type LucideIcon,
} from "lucide-react";
import { AdminLineChart } from "@/components/dashboard/charts/AdminLineChart";
import { AdminBarChart } from "@/components/dashboard/charts/AdminBarChart";
import { Metric } from "@/components/ui/Metric";
import { formatAdminActionType } from "@/utils/adminActionLabel";
import { formatAdminCurrency } from "@/utils/adminCurrency";
import { SystemHealthPanel } from "@/components/admin/SystemHealthPanel";
import { useAdminPolling } from "@/components/admin/primitives";

/* eslint-disable no-restricted-syntax -- #hex chart colors passed to chart.js dataset props; not Tailwind classes */
const CHART_COLORS = {
  teal: "#1a6b6a", // brand token shared by growth charts
};
/* eslint-enable no-restricted-syntax */

function formatNumber(value: number) {
  return new Intl.NumberFormat("en-US").format(value);
}

function formatTimestamp(ts: string) {
  const date = new Date(ts);
  return date.toLocaleDateString("en-US", {
    month: "short",
    day: "numeric",
    hour: "2-digit",
    minute: "2-digit",
  });
}

function StatCard({
  label,
  value,
  format = (n) => String(n),
  sublabel,
  icon: Icon,
  href,
  alert,
  exact,
}: {
  label: string;
  value: number;
  format?: (n: number) => string;
  sublabel?: string;
  icon: LucideIcon;
  href?: string;
  alert?: boolean;
  /** Full-precision value shown on hover when the tile value is abbreviated. */
  exact?: string;
}) {
  // Metric derives tone from value: zero is neutral (never success-green).
  // Alert cards keep the danger shell; positive alert counts use rose text.
  const valueClass = alert
    ? "[&_[data-tone]]:!text-rose-700"
    : "";
  const labelClass =
    "[&_p:first-child]:text-sm [&_p:first-child]:font-normal [&_p:first-child]:normal-case [&_p:first-child]:tracking-normal [&_p:first-child]:text-default-500";

  const body = (
    <Card
      className={`border shadow-sm transition-shadow hover:shadow-md ${
        alert ? "border-danger/30 bg-danger/5" : "border-default-200"
      }`}
    >
      <CardBody className="flex flex-row items-center gap-4 p-5">
        <div
          className={`flex h-12 w-12 shrink-0 items-center justify-center rounded-xl ${
            alert ? "bg-danger/10" : "bg-brand/10"
          }`}
        >
          <Icon size={20} className={alert ? "text-danger" : "text-brand"} />
        </div>
        <div className="min-w-0" title={exact}>
          <Metric
            label={label}
            value={value}
            state="ok"
            format={format}
            size="lg"
            className={`${labelClass} ${valueClass}`.trim()}
          />
          {sublabel ? (
            <p className="text-xs text-default-400">{sublabel}</p>
          ) : null}
        </div>
      </CardBody>
    </Card>
  );

  if (href) {
    return (
      <Link href={href} className="block">
        {body}
      </Link>
    );
  }
  return body;
}

const AdminDashboard = () => {
  const [stats, setStats] = useState<AdminStats | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  const fetchData = useCallback(async (silent = false) => {
    try {
      if (!silent) {
        setLoading(true);
      }
      setError(null);
      const adminStats = await getAdminStats();
      setStats(adminStats);
    } catch (err) {
      console.error("Error fetching admin stats:", err);
      if (!silent) {
        setError("Failed to load admin statistics");
      }
    } finally {
      if (!silent) {
        setLoading(false);
      }
    }
  }, []);

  useEffect(() => {
    void fetchData(false);
  }, [fetchData]);

  useAdminPolling(() => fetchData(true), 60_000, Boolean(stats));

  if (loading) {
    return (
      <div className="flex items-center justify-center min-h-[400px]">
        <div className="text-center">
          <Spinner size="lg" />
          <p className="mt-4 text-default-500">Loading dashboard...</p>
        </div>
      </div>
    );
  }

  if (error || !stats) {
    return (
      <div className="flex items-center justify-center min-h-[400px]">
        <p className="text-danger">{error || "Failed to load data"}</p>
      </div>
    );
  }

  const roleEntries = Object.entries(stats.users_by_role || {}).sort(
    ([, a], [, b]) => b - a,
  );
  const billStatusEntries = Object.entries(stats.bills_by_status || {}).sort(
    ([, a], [, b]) => b - a,
  );

  // Volume sums are not converted between currencies. Format them in the one
  // currency every venue uses; with several, show plain numbers and say so.
  const volumeCurrencies = stats.payment_volume_currencies ?? [];
  const volumeCurrency =
    stats.payment_volume_currency ||
    (volumeCurrencies.length === 0 ? "USD" : "");
  // Lifetime totals use compact notation from a million up so the tile value
  // never wraps inside the digits; the card's title keeps the exact figure.
  const formatVolume = (n: number, fractionDigits?: number, compact = false) =>
    volumeCurrency
      ? formatAdminCurrency(n, { fractionDigits, currency: volumeCurrency, compact })
      : new Intl.NumberFormat("en-US", {
          minimumFractionDigits: compact && Math.abs(n) >= 1_000_000 ? 0 : (fractionDigits ?? 2),
          maximumFractionDigits: compact && Math.abs(n) >= 1_000_000 ? 1 : (fractionDigits ?? 2),
          notation: compact && Math.abs(n) >= 1_000_000 ? "compact" : "standard",
        }).format(n);
  const volumeNote = volumeCurrency
    ? ""
    : ` · mixed currencies (${volumeCurrencies.join(", ")}), not converted`;

  return (
    <div className="flex flex-col gap-6">
      {/* Platform KPIs — Metric owns zero tone (never success-green on $0). */}
      <div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
        <StatCard
          label="Businesses"
          value={stats.total_businesses}
          format={formatNumber}
          sublabel={`${stats.active_businesses} active / ${stats.inactive_businesses} inactive · real venues · excludes demo`}
          icon={Building2}
          href="/admin/businesses"
        />
        <StatCard
          label="Users"
          value={stats.total_users}
          format={formatNumber}
          icon={Users}
          href="/admin/users"
        />
      </div>

      {/* Platform volume */}
      <div className="grid grid-cols-1 sm:grid-cols-2 xl:grid-cols-4 gap-4">
        <StatCard
          label="Payment Volume"
          value={stats.total_payment_volume}
          format={(n) => formatVolume(n, 0, true)}
          exact={formatVolume(stats.total_payment_volume, 0)}
          sublabel={`Lifetime · includes demo · recognized guest payments${volumeNote}`}
          icon={TrendingUp}
        />
        <StatCard
          label="Processed volume (all businesses)"
          value={stats.gross_merchandise_volume}
          format={(n) => formatVolume(n, 0, true)}
          exact={formatVolume(stats.gross_merchandise_volume, 0)}
          sublabel={`Lifetime · includes demo · restaurant takings processed — not Payverge revenue${volumeNote}`}
          icon={DollarSign}
        />
        <StatCard
          label="Avg Transaction"
          value={stats.average_transaction_size}
          format={(n) => formatVolume(n, undefined, true)}
          exact={formatVolume(stats.average_transaction_size)}
          sublabel={`Lifetime · includes demo${volumeNote}`}
          icon={Activity}
        />
        <StatCard
          label="Bills"
          value={stats.total_bills ?? 0}
          format={formatNumber}
          sublabel={`Lifetime · includes demo · ${formatNumber(stats.recognized_bills ?? 0)} recognized`}
          icon={Receipt}
        />
      </div>

      {/* Alerts */}
      <div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
        <StatCard
          label="Failed Webhooks"
          value={stats.failed_webhooks_count}
          format={formatNumber}
          sublabel={
            stats.failed_webhooks_count === 0
              ? "All webhooks healthy"
              : "Needs attention"
          }
          icon={TriangleAlert}
          alert={stats.failed_webhooks_count > 0}
        />
      </div>

      <SystemHealthPanel />

      {/* Growth charts — empty/all-zero series render a real empty state
          (no fabricated $1 axis). */}
      <div className="grid grid-cols-1 lg:grid-cols-2 gap-4">
        <Card className="border border-default-200 shadow-sm">
          <CardHeader className="flex-col items-start gap-1 px-5 pt-5 pb-0">
            <p className="text-base font-semibold">Payment Volume</p>
            <p className="text-xs font-normal text-default-400">{`Lifetime · includes demo${volumeNote}`}</p>
          </CardHeader>
          <CardBody className="h-[260px] w-full px-2 pb-4">
            <AdminLineChart
              points={stats.payment_volume_growth}
              ariaLabel="Payment Volume"
              formatValue={(v: number) => formatVolume(v, 0)}
              emptyLabel="No payment volume yet"
            />
          </CardBody>
        </Card>

        <Card className="border border-default-200 shadow-sm">
          <CardHeader className="flex-col items-start gap-1 px-5 pt-5 pb-0">
            <p className="text-base font-semibold">Business Growth</p>
            <p className="text-xs font-normal text-default-400">Real venues · excludes demo</p>
          </CardHeader>
          <CardBody className="h-[260px] w-full px-2 pb-4">
            <AdminBarChart
              points={stats.business_growth}
              ariaLabel="Business Growth"
              barColor={CHART_COLORS.teal}
              integerTicks
              emptyLabel="No business growth data"
            />
          </CardBody>
        </Card>
      </div>

      <div className="grid grid-cols-1 lg:grid-cols-2 gap-4">
        <Card className="border border-default-200 shadow-sm">
          <CardHeader className="px-5 pt-5 pb-0">
            <p className="text-base font-semibold">User Growth</p>
          </CardHeader>
          <CardBody className="h-[260px] w-full px-2 pb-4">
            <AdminBarChart
              points={stats.user_growth}
              ariaLabel="User Growth"
              barColor={CHART_COLORS.teal}
              integerTicks
              emptyLabel="No user growth data"
            />
          </CardBody>
        </Card>

        <Card className="border border-default-200 shadow-sm">
          <CardHeader className="flex-col items-start gap-1 px-5 pt-5 pb-0">
            <p className="text-base font-semibold">Bill Volume</p>
            <p className="text-xs font-normal text-default-400">Lifetime · includes demo</p>
          </CardHeader>
          <CardBody className="h-[260px] w-full px-2 pb-4">
            <AdminBarChart
              points={stats.bill_growth ?? []}
              ariaLabel="Bill Volume"
              barColor={CHART_COLORS.teal}
              integerTicks
              emptyLabel="No bill volume data"
            />
          </CardBody>
        </Card>
      </div>

      {/* Breakdowns */}
      <div className="grid grid-cols-1 lg:grid-cols-2 gap-4">
        <Card className="border border-default-200 shadow-sm">
          <CardHeader className="px-5 pt-5 pb-2">
            <p className="text-base font-semibold">Users by Role</p>
          </CardHeader>
          <CardBody className="px-5 pb-5 pt-0">
            {roleEntries.length > 0 ? (
              <div className="flex flex-wrap gap-2">
                {roleEntries.map(([role, count]) => (
                  <Chip key={role} size="sm" variant="flat">
                    {role}: {formatNumber(count)}
                  </Chip>
                ))}
              </div>
            ) : (
              <p className="text-sm text-default-400">No role data</p>
            )}
          </CardBody>
        </Card>

        <Card className="border border-default-200 shadow-sm">
          <CardHeader className="px-5 pt-5 pb-2">
            <p className="text-base font-semibold">Bills by Status</p>
          </CardHeader>
          <CardBody className="px-5 pb-5 pt-0">
            {billStatusEntries.length > 0 ? (
              <div className="flex flex-wrap gap-2">
                {billStatusEntries.map(([status, count]) => (
                  <Chip key={status} size="sm" variant="flat" color="default">
                    {status}: {formatNumber(count)}
                  </Chip>
                ))}
              </div>
            ) : (
              <p className="text-sm text-default-400">No bill data</p>
            )}
          </CardBody>
        </Card>
      </div>

      {/* Operational */}
      <div className="grid grid-cols-1 lg:grid-cols-3 gap-4">
        <Card className="border border-default-200 shadow-sm">
          <CardHeader className="px-5 pt-5 pb-2">
            <p className="text-base font-semibold">Recent Admin Actions</p>
          </CardHeader>
          <CardBody className="px-5 pb-5 pt-0">
            {stats.recent_admin_actions?.length > 0 ? (
              <ul className="space-y-3">
                {stats.recent_admin_actions
                  .slice(0, 5)
                  .map((action: AdminAction) => (
                    <li
                      key={action.id}
                      className="flex items-center justify-between text-sm"
                    >
                      <div className="flex items-center gap-2 min-w-0">
                        <Chip size="sm" variant="flat" className="shrink-0">
                          {formatAdminActionType(action.action_type)}
                        </Chip>
                        <span className="text-default-400 truncate">
                          User #{action.target_user_id}
                        </span>
                      </div>
                      <span className="text-xs text-default-400 shrink-0 ml-2">
                        {formatTimestamp(action.created_at)}
                      </span>
                    </li>
                  ))}
              </ul>
            ) : (
              <p className="text-sm text-default-400">No recent actions</p>
            )}
          </CardBody>
        </Card>

        <Card className="border border-default-200 shadow-sm">
          <CardHeader className="flex items-center justify-between px-5 pt-5 pb-2">
            <p className="text-base font-semibold">Recent Errors</p>
            <Link
              href="/admin/errors"
              className="text-xs text-brand hover:underline"
            >
              View all
            </Link>
          </CardHeader>
          <CardBody className="px-5 pb-5 pt-0">
            {stats.recent_errors?.length > 0 ? (
              <ul className="space-y-3">
                {stats.recent_errors.slice(0, 5).map((err: ErrorLogSummary) => (
                  <li key={err.id} className="text-sm">
                    <div className="flex items-center justify-between">
                      <span className="font-medium text-danger truncate max-w-[60%]">
                        {err.message}
                      </span>
                      <span className="text-xs text-default-400 shrink-0 ml-2">
                        {formatTimestamp(err.created_at)}
                      </span>
                    </div>
                    <span className="text-xs text-default-400">
                      {err.source} / {err.component}
                    </span>
                  </li>
                ))}
              </ul>
            ) : (
              <p className="text-sm text-default-400">No recent errors</p>
            )}
          </CardBody>
        </Card>

        <Card className="border border-default-200 shadow-sm">
          <CardHeader className="flex items-center justify-between px-5 pt-5 pb-2">
            <div className="flex items-center gap-2">
              <FlaskConical size={18} className="text-brand" />
              <p className="text-base font-semibold">Demo Center</p>
            </div>
            <Link
              href="/admin/demo"
              className="text-xs text-brand hover:underline"
            >
              Open
            </Link>
          </CardHeader>
          <CardBody className="px-5 pb-5 pt-0">
            <p className="text-sm text-default-500">
              Isolated admin demos with reset, daily append, and coverage
              checks.
            </p>
          </CardBody>
        </Card>
      </div>
    </div>
  );
};

export default AdminDashboard;
