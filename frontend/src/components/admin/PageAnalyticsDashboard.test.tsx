/** @jest-environment jsdom */
import React from "react";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import { PageAnalyticsDashboard } from "./PageAnalyticsDashboard";

const mockSummary = jest.fn();
const mockSessions = jest.fn();

jest.mock("@/api/pageAnalytics", () => ({
  pageAnalyticsAPI: {
    getAnalyticsSummary: (...a: unknown[]) => mockSummary(...a),
    getRecentSessions: (...a: unknown[]) => mockSessions(...a),
  },
}));

jest.mock("@/api/adminMissingTranslations", () => ({
  getMissingTranslations: jest.fn().mockResolvedValue({ rows: [], total: 0 }),
}));

beforeEach(() => {
  jest.clearAllMocks();
});

describe("PageAnalyticsDashboard — error state", () => {
  it("renders the sanitized friendly message, not the raw error string", async () => {
    mockSummary.mockRejectedValue(new Error("Request failed with status code 500"));
    mockSessions.mockResolvedValue([]);

    render(<PageAnalyticsDashboard />);

    await waitFor(() => {
      expect(screen.getByText(/Error loading analytics/i)).toBeInTheDocument();
    });
    // The raw axios-style string must not leak.
    expect(
      screen.queryByText(/Request failed with status code 500/i),
    ).not.toBeInTheDocument();
    // A friendly sanitized fallback is shown instead.
    expect(
      screen.getByText(/unexpected error occurred|went wrong/i),
    ).toBeInTheDocument();
  });

  it("retry button is a real NextUI button (not a raw button with dead hover)", async () => {
    mockSummary.mockRejectedValue(new Error("boom"));
    mockSessions.mockResolvedValue([]);

    render(<PageAnalyticsDashboard />);

    const retry = await screen.findByRole("button", { name: /Retry/i });
    // Dead-hover regression guard: the resting bg class must not equal the
    // hover class.
    expect(retry.className).not.toMatch(/hover:bg-brand(?![\w-])/);

    // Clicking retry re-fetches.
    mockSummary.mockResolvedValue({
      total_page_views: 0,
      total_sessions: 0,
      total_interactions: 0,
      total_conversions: 0,
      average_session_time: 0,
      bounce_rate: 0,
      conversion_rate: 0,
      top_pages: [],
      top_interactions: [],
      device_breakdown: {},
      country_breakdown: {},
      conversion_funnel: [],
    });
    fireEvent.click(retry);
    await waitFor(() => expect(mockSummary).toHaveBeenCalledTimes(2));
  });
});

describe("PageAnalyticsDashboard — conversion funnel", () => {
  it("does not render numeric dropoff when the previous step is zero", async () => {
    mockSummary.mockResolvedValue({
      total_page_views: 80,
      total_sessions: 80,
      total_interactions: 0,
      total_conversions: 3,
      average_session_time: 0,
      bounce_rate: 0,
      conversion_rate: 0,
      top_pages: [],
      top_interactions: [],
      device_breakdown: {},
      country_breakdown: {},
      conversion_funnel: [
        { step: "Operator Sign-in", sessions: 0, dropoff_rate: null },
        { step: "Venue Registration", sessions: 0, dropoff_rate: null },
        { step: "Venue Dashboard", sessions: 3, dropoff_rate: null },
      ],
    });
    mockSessions.mockResolvedValue([]);

    render(<PageAnalyticsDashboard />);

    await waitFor(() => {
      expect(screen.getByText("Venue Dashboard")).toBeInTheDocument();
    });
    expect(screen.queryByText(/-0\.0% dropoff/i)).not.toBeInTheDocument();
    expect(screen.getAllByText(/not applicable/i).length).toBeGreaterThan(0);
  });
});

describe("PageAnalyticsDashboard — selected range (#452)", () => {
  it("exposes aria-pressed on the active date range", async () => {
    mockSummary.mockResolvedValue({
      total_page_views: 1,
      total_sessions: 1,
      total_interactions: 0,
      total_conversions: 0,
      average_session_time: 0,
      bounce_rate: 0,
      conversion_rate: 0,
      top_pages: [],
      top_interactions: [],
      device_breakdown: {},
      country_breakdown: {},
      conversion_funnel: [],
    });
    mockSessions.mockResolvedValue([]);

    render(<PageAnalyticsDashboard />);
    const seven = await screen.findByRole("button", { name: /Last 7 Days/i });
    const thirty = screen.getByRole("button", { name: /Last 30 Days/i });
    expect(thirty).toHaveAttribute("aria-pressed", "true");
    expect(seven).toHaveAttribute("aria-pressed", "false");
    fireEvent.click(seven);
    expect(seven).toHaveAttribute("aria-pressed", "true");
    expect(thirty).toHaveAttribute("aria-pressed", "false");
  });
});
