"use client";
import React, { useState, useCallback, useEffect } from "react";
import {
  Card,
  CardBody,
  CardHeader,
  Divider,
  Spinner,
  Chip,
  Button,
} from "@nextui-org/react";
import {
  pageAnalyticsAPI,
  AnalyticsSummary,
  SessionSummary,
} from "@/api/pageAnalytics";
import {
  getMissingTranslations,
  type MissingTranslationRow,
} from "@/api/adminMissingTranslations";
import { sanitizeError } from "@/utils/errorMessages";
import { localDateKey } from "@/lib/localDate";
import Link from "next/link";

export const PageAnalyticsDashboard: React.FC = () => {
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [summary, setSummary] = useState<AnalyticsSummary | null>(null);
  const [sessions, setSessions] = useState<SessionSummary[]>([]);
  const [missingTranslations, setMissingTranslations] = useState<
    MissingTranslationRow[]
  >([]);
  const [missingTranslationsTotal, setMissingTranslationsTotal] = useState(0);
  const [dateRange, setDateRange] = useState<"7d" | "30d" | "90d">("30d");

  const loadAnalytics = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      const endDate = new Date();
      const startDate = new Date();

      switch (dateRange) {
        case "7d":
          startDate.setDate(endDate.getDate() - 7);
          break;
        case "30d":
          startDate.setDate(endDate.getDate() - 30);
          break;
        case "90d":
          startDate.setDate(endDate.getDate() - 90);
          break;
      }

      const [summaryData, sessionsData, missingData] = await Promise.all([
        pageAnalyticsAPI.getAnalyticsSummary(
          localDateKey(startDate),
          localDateKey(endDate),
        ),
        pageAnalyticsAPI.getRecentSessions(50),
        getMissingTranslations({ limit: 25 }),
      ]);

      setSummary(summaryData);
      setSessions(sessionsData);
      setMissingTranslations(missingData.rows);
      setMissingTranslationsTotal(missingData.total);
    } catch (error) {
      console.error("[PageAnalytics] Failed to load analytics:", error);
      // Show the sanitized, user-friendly message — not the raw axios/network
      // string (e.g. "Request failed with status code 500").
      const { message, status } = sanitizeError(error);
      console.error("[PageAnalytics] Error details:", { message, status });
      setError(message);
    } finally {
      setLoading(false);
    }
  }, [dateRange]);

  useEffect(() => {
    loadAnalytics();
  }, [loadAnalytics]);

  const formatDuration = (seconds: number): string => {
    const minutes = Math.floor(seconds / 60);
    const secs = seconds % 60;
    return `${minutes}m ${secs}s`;
  };

  const formatPercentage = (value: number): string => {
    return `${value.toFixed(1)}%`;
  };

  if (loading) {
    return (
      <div className="flex justify-center items-center h-64">
        <Spinner size="lg" />
      </div>
    );
  }

  if (error) {
    return (
      <div className="text-center p-8">
        <p className="text-danger font-semibold">Error loading analytics</p>
        <p className="text-default-500 mt-2">{error}</p>
        <Button
          onPress={loadAnalytics}
          color="primary"
          className="mt-4 bg-brand hover:bg-brand-dark"
        >
          Retry
        </Button>
      </div>
    );
  }

  if (!summary) {
    return (
      <div className="text-center p-8">
        <p className="text-default-500">No analytics data available</p>
      </div>
    );
  }

  return (
    <div className="space-y-6">
      {/* Date Range Selector */}
      <div
        role="group"
        aria-label="Date range"
        className="flex gap-2"
      >
        <button
          type="button"
          onClick={() => setDateRange("7d")}
          aria-pressed={dateRange === "7d"}
          className={`px-4 py-2 rounded-lg ${
            dateRange === "7d" ? "bg-brand text-white" : "bg-default-100"
          }`}
        >
          Last 7 Days
        </button>
        <button
          type="button"
          onClick={() => setDateRange("30d")}
          aria-pressed={dateRange === "30d"}
          className={`px-4 py-2 rounded-lg ${
            dateRange === "30d" ? "bg-brand text-white" : "bg-default-100"
          }`}
        >
          Last 30 Days
        </button>
        <button
          type="button"
          onClick={() => setDateRange("90d")}
          aria-pressed={dateRange === "90d"}
          className={`px-4 py-2 rounded-lg ${
            dateRange === "90d" ? "bg-brand text-white" : "bg-default-100"
          }`}
        >
          Last 90 Days
        </button>
      </div>

      {/* Overview Cards */}
      <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-4 gap-4">
        <Card>
          <CardBody>
            <p className="text-sm text-default-500">Total Page Views</p>
            <p className="text-3xl font-bold">
              {summary.total_page_views.toLocaleString()}
            </p>
          </CardBody>
        </Card>
        <Card>
          <CardBody>
            <p className="text-sm text-default-500">Total Sessions</p>
            <p className="text-3xl font-bold">
              {summary.total_sessions.toLocaleString()}
            </p>
          </CardBody>
        </Card>
        <Card>
          <CardBody>
            <p className="text-sm text-default-500">Total Interactions</p>
            <p className="text-3xl font-bold">
              {summary.total_interactions.toLocaleString()}
            </p>
          </CardBody>
        </Card>
        <Card>
          <CardBody>
            <p className="text-sm text-default-500">Conversions</p>
            <p className="text-3xl font-bold">
              {summary.total_conversions.toLocaleString()}
            </p>
          </CardBody>
        </Card>
      </div>

      {/* Key Metrics */}
      <div className="grid grid-cols-1 md:grid-cols-3 gap-4">
        <Card>
          <CardBody>
            <p className="text-sm text-default-500">Avg Session Time</p>
            <p className="text-2xl font-bold">
              {formatDuration(Math.round(summary.average_session_time))}
            </p>
          </CardBody>
        </Card>
        <Card>
          <CardBody>
            <p className="text-sm text-default-500">Bounce Rate</p>
            <p className="text-2xl font-bold">
              {formatPercentage(summary.bounce_rate)}
            </p>
          </CardBody>
        </Card>
        <Card>
          <CardBody>
            <p className="text-sm text-default-500">Conversion Rate</p>
            <p className="text-2xl font-bold text-success-600">
              {formatPercentage(summary.conversion_rate)}
            </p>
          </CardBody>
        </Card>
      </div>

      {/* Top Pages */}
      <Card>
        <CardHeader>
          <h3 className="text-lg font-semibold">Top Pages</h3>
        </CardHeader>
        <Divider />
        <CardBody>
          <div className="space-y-2">
            {summary.top_pages && summary.top_pages.length > 0 ? (
              summary.top_pages.map((page, index) => (
                <div
                  key={index}
                  className="flex justify-between items-center p-2 hover:bg-default-100 rounded"
                >
                  <div className="flex-1">
                    <p className="font-medium">{page.page}</p>
                    <p className="text-sm text-default-500">
                      {page.unique_visitors} unique visitors · Avg{" "}
                      {formatDuration(Math.round(page.average_duration))}
                    </p>
                  </div>
                  <Chip color="primary" variant="flat">
                    {page.views} views
                  </Chip>
                </div>
              ))
            ) : (
              <p className="text-default-500 text-center py-4">
                No page data available
              </p>
            )}
          </div>
        </CardBody>
      </Card>

      {/* Top Interactions */}
      <Card>
        <CardHeader>
          <h3 className="text-lg font-semibold">Top Interactions</h3>
        </CardHeader>
        <Divider />
        <CardBody>
          <div className="space-y-2">
            {summary.top_interactions && summary.top_interactions.length > 0 ? (
              summary.top_interactions
                .slice(0, 10)
                .map((interaction, index) => (
                  <div
                    key={index}
                    className="flex justify-between items-center p-2 hover:bg-default-100 rounded"
                  >
                    <div className="flex-1">
                      <p className="font-medium">{interaction.event_label}</p>
                      <p className="text-sm text-default-500">
                        {interaction.event_type} · {interaction.event_category}
                      </p>
                    </div>
                    <div className="text-right">
                      <p className="font-semibold">{interaction.count}</p>
                      <p className="text-xs text-default-500">
                        {interaction.unique_sessions} sessions
                      </p>
                    </div>
                  </div>
                ))
            ) : (
              <p className="text-default-500 text-center py-4">
                No interaction data available
              </p>
            )}
          </div>
        </CardBody>
      </Card>

      {/* Device Breakdown */}
      <Card>
        <CardHeader>
          <h3 className="text-lg font-semibold">Device Breakdown</h3>
        </CardHeader>
        <Divider />
        <CardBody>
          <div className="grid grid-cols-2 md:grid-cols-3 gap-4">
            {summary.device_breakdown &&
            Object.keys(summary.device_breakdown).length > 0 ? (
              Object.entries(summary.device_breakdown).map(
                ([device, count]) => (
                  <div
                    key={device}
                    className="text-center p-4 bg-default-100 rounded-lg"
                  >
                    <p className="text-2xl font-bold">{count}</p>
                    <p className="text-sm text-default-500 capitalize">
                      {device}
                    </p>
                  </div>
                ),
              )
            ) : (
              <p className="text-default-500 text-center py-4">
                No device data available
              </p>
            )}
          </div>
        </CardBody>
      </Card>

      {/* Country Breakdown */}
      <Card>
        <CardHeader>
          <h3 className="text-lg font-semibold">Top Countries</h3>
        </CardHeader>
        <Divider />
        <CardBody>
          <div className="space-y-2">
            {summary.country_breakdown &&
            Object.keys(summary.country_breakdown).length > 0 ? (
              Object.entries(summary.country_breakdown)
                .sort(([, a], [, b]) => b - a)
                .map(([country, count]) => (
                  <div
                    key={country}
                    className="flex justify-between items-center p-2 hover:bg-default-100 rounded"
                  >
                    <p className="font-medium">{country || "Unknown"}</p>
                    <Chip color="default" variant="flat">
                      {count} sessions
                    </Chip>
                  </div>
                ))
            ) : (
              <p className="text-default-500 text-center py-4">
                No country data available
              </p>
            )}
          </div>
        </CardBody>
      </Card>

      {/* Conversion Funnel */}
      {summary.conversion_funnel && summary.conversion_funnel.length > 0 && (
        <Card>
          <CardHeader>
            <h3 className="text-lg font-semibold">Conversion Funnel</h3>
          </CardHeader>
          <Divider />
          <CardBody>
            <div className="space-y-4">
              {summary.conversion_funnel.map((step, index) => (
                <div key={index}>
                  <div className="flex justify-between items-center mb-2">
                    <p className="font-medium">{step.step}</p>
                    <div className="text-right">
                      <p className="font-semibold">
                        {step.sessions.toLocaleString()}
                      </p>
                      {index > 0 && step.dropoff_rate != null ? (
                        <p className="text-sm text-danger-500">
                          -{formatPercentage(step.dropoff_rate)} dropoff
                        </p>
                      ) : index > 0 ? (
                        <p className="text-sm text-ink-500">Not applicable</p>
                      ) : null}
                    </div>
                  </div>
                  <div className="w-full bg-default-200 rounded-full h-2">
                    <div
                      className="bg-brand h-2 rounded-full"
                      style={{
                        width: `${
                          summary.conversion_funnel[0].sessions > 0
                            ? (step.sessions /
                                summary.conversion_funnel[0].sessions) *
                              100
                            : 0
                        }%`,
                      }}
                    />
                  </div>
                </div>
              ))}
            </div>
          </CardBody>
        </Card>
      )}

      {/* Recent Sessions */}
      <Card>
        <CardHeader>
          <h3 className="text-lg font-semibold">Recent Sessions</h3>
        </CardHeader>
        <Divider />
        <CardBody>
          <div className="space-y-2">
            {sessions && sessions.length > 0 ? (
              sessions.slice(0, 20).map((session) => (
                <div
                  key={session.session_id}
                  className="p-3 border rounded-lg hover:bg-default-100"
                >
                  <div className="flex justify-between items-start mb-2">
                    <div className="flex-1">
                      <p className="text-xs text-default-500 font-mono">
                        {session.session_id.substring(0, 8)}...
                      </p>
                      <p className="text-sm">
                        {session.total_page_views} pages ·{" "}
                        {session.total_interactions} interactions
                      </p>
                    </div>
                    <div className="text-right">
                      <p className="text-sm">
                        {formatDuration(session.total_duration)}
                      </p>
                      <p className="text-xs text-default-500">
                        {session.device_type}
                      </p>
                    </div>
                  </div>
                  {session.converted && (
                    <Chip color="success" size="sm" variant="flat">
                      Converted: {session.conversion_type}
                    </Chip>
                  )}
                </div>
              ))
            ) : (
              <p className="text-default-500 text-center py-4">
                No session data available
              </p>
            )}
          </div>
        </CardBody>
      </Card>

      {/* Missing i18n keys — fed by guest/operator fallback reporter */}
      <Card>
        <CardHeader className="flex flex-col sm:flex-row sm:items-center sm:justify-between gap-2">
          <h3 className="text-lg font-semibold">
            Missing Translations
            {missingTranslationsTotal > 0 ? (
              <span className="ml-2 text-sm font-normal text-default-500">
                ({missingTranslationsTotal} reports)
              </span>
            ) : null}
          </h3>
          <Button
            as={Link}
            href="/admin/translations"
            size="sm"
            variant="flat"
            className="text-brand"
          >
            View all
          </Button>
        </CardHeader>
        <Divider />
        <CardBody>
          {missingTranslations.length > 0 ? (
            <div className="space-y-2">
              {missingTranslations.map((row) => (
                <div
                  key={row.id}
                  className="flex flex-col sm:flex-row sm:items-center sm:justify-between gap-1 p-2 hover:bg-default-100 rounded"
                >
                  <div className="min-w-0">
                    <p className="font-mono text-sm truncate">{row.key_path}</p>
                    <p className="text-xs text-default-500 truncate">
                      {row.locale} · {row.fallback_used}
                      {row.page ? ` · ${row.page}` : ""}
                    </p>
                  </div>
                  <div className="flex items-center gap-2 shrink-0">
                    <Chip size="sm" variant="flat">
                      {row.hit_count} hits
                    </Chip>
                  </div>
                </div>
              ))}
            </div>
          ) : (
            <p className="text-default-500 text-center py-4">
              No missing translation keys reported
            </p>
          )}
        </CardBody>
      </Card>
    </div>
  );
};
