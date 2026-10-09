/** @jest-environment jsdom */
/**
 * Related to issue 832: when the hub stats fetch failed, the page recorded the
 * error into a state nothing rendered — rows showed "—" and the combined view
 * said "No data yet", indistinguishable from a genuinely quiet day. A failed
 * load must announce itself and offer a retry.
 */
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import React from "react";
import fs from "fs";
import path from "path";
import { BusinessOverviewList } from "./BusinessOverviewList";
import type { Business } from "@/api/business";

jest.mock("./BusinessOverviewPanel", () => ({
  __esModule: true,
  BusinessOverviewPanel: () => <div data-testid="business-detail-panel" />,
  default: () => <div data-testid="business-detail-panel" />,
}));
jest.mock("./CombinedOverviewPanel", () => ({
  __esModule: true,
  CombinedOverviewPanel: () => <div data-testid="combined-overview-panel" />,
  default: () => <div data-testid="combined-overview-panel" />,
}));

const t = (key: string) => key;

const wrapper = ({ children }: { children: React.ReactNode }) => {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return <QueryClientProvider client={client}>{children}</QueryClientProvider>;
};

function biz(id: number, name: string): Business {
  return {
    id,
    name,
    logo: "",
    is_active: true,
    default_currency: "USD",
    owner_address: "0x0",
    address: { street: "", city: "", state: "", postal_code: "", country: "" },
    settlement_address: "",
    tipping_address: "",
    tax_rate: 0,
    service_fee_rate: 0,
    tax_inclusive: false,
    service_inclusive: false,
  } as Business;
}

describe("BusinessOverviewList stats-failure honesty (issue 832)", () => {
  it("announces a failed stats load with a retry instead of posing as an empty day", async () => {
    const onRetryStats = jest.fn();
    render(
      <BusinessOverviewList
        businesses={[biz(1, "Payverge Core Demo Kitchen")]}
        stats={{}}
        statsLoading={false}
        statsError="dashboard.stats.loadError"
        onRetryStats={onRetryStats}
        onManage={jest.fn()}
        navigatingBusinessId={null}
        t={t}
      />,
      { wrapper },
    );
    const alert = screen.getByRole("alert");
    expect(alert).toHaveTextContent("dashboard.stats.loadError");
    await userEvent.click(
      screen.getByRole("button", { name: "stats.retry" }),
    );
    expect(onRetryStats).toHaveBeenCalledTimes(1);
  });

  it("shows no failure banner when stats simply have not failed", () => {
    render(
      <BusinessOverviewList
        businesses={[biz(1, "Payverge Core Demo Kitchen")]}
        stats={{}}
        statsLoading
        statsError={null}
        onManage={jest.fn()}
        navigatingBusinessId={null}
        t={t}
      />,
      { wrapper },
    );
    expect(screen.queryByRole("alert")).not.toBeInTheDocument();
  });

  it("dashboard page binds statsError and wires it into the list", () => {
    // Source-level guard: the page used to keep `setStatsError` but drop the
    // value binding, so the error state was write-only. It must be read and
    // passed to BusinessOverviewList together with a retry handler.
    const pageSrc = fs.readFileSync(
      path.join(__dirname, "../../app/(shop)/dashboard/page.tsx"),
      "utf8",
    );
    expect(pageSrc).toMatch(/const \[statsError, setStatsError\]/);
    expect(pageSrc).toMatch(/statsError=\{statsError\}/);
    expect(pageSrc).toMatch(/onRetryStats=\{/);
  });
});
