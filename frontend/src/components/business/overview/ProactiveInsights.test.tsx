/** @jest-environment jsdom */
import React from "react";
import { render, screen, fireEvent } from "@testing-library/react";
import ProactiveInsights from "./ProactiveInsights";

// next/link renders as a plain <a> in jsdom without the Next.js runtime
jest.mock("next/link", () => {
  const MockLink = ({
    href,
    children,
    className,
  }: {
    href: string;
    children: React.ReactNode;
    className?: string;
  }) => (
    <a href={href} className={className}>
      {children}
    </a>
  );
  MockLink.displayName = "MockLink";
  return MockLink;
});

const tStub = (key: string, params?: Record<string, string | number>) => {
  if (params && Object.keys(params).length > 0) {
    return `${key}:${JSON.stringify(params)}`;
  }
  return key;
};

describe("ProactiveInsights", () => {
  it("formats stale-bill age from numeric minutes through the active locale", () => {
    const esT = (key: string, params?: Record<string, string | number>) => {
      const copy: Record<string, string> = {
        "proactive.types.stale_open_bills.title_other":
          "{count} cuentas abiertas desde hace más de {duration}",
        "proactive.duration.days_other": "{count} días",
      };
      let value = copy[key] ?? key;
      for (const [name, replacement] of Object.entries(params ?? {})) {
        value = value.replace(`{${name}}`, String(replacement));
      }
      return value;
    };

    render(
      <ProactiveInsights
        insights={[
          {
            id: "stale",
            type: "stale_open_bills",
            params: {
              count: 2,
              oldest_minutes: 26 * 24 * 60,
              duration: "26 days",
            },
            cta: { tab: "bills" },
          },
        ]}
        t={esT}
        onOpenConsole={jest.fn()}
      />,
    );

    expect(
      screen.getByRole("heading", {
        name: "2 cuentas abiertas desde hace más de 26 días",
      }),
    ).toBeInTheDocument();
    expect(screen.queryByText(/26 days/)).not.toBeInTheDocument();
  });

  it("formats the 40-day stale-bills title from minutes, ignoring English duration prose", () => {
    const esT = (key: string, params?: Record<string, string | number>) => {
      const copy: Record<string, string> = {
        "proactive.types.stale_open_bills.title_other":
          "{count} cuentas abiertas desde hace más de {duration} — fijate si los clientes se fueron.",
        "proactive.duration.days_other": "{count} días",
      };
      let value = copy[key] ?? key;
      for (const [name, replacement] of Object.entries(params ?? {})) {
        value = value.replace(`{${name}}`, String(replacement));
      }
      return value;
    };

    render(
      <ProactiveInsights
        insights={[
          {
            id: "stale-40",
            type: "stale_open_bills",
            params: {
              count: 2,
              oldest_minutes: 40 * 24 * 60,
              duration: "40 days",
            },
            cta: { tab: "bills" },
          },
        ]}
        t={esT}
        onOpenConsole={jest.fn()}
      />,
    );

    expect(
      screen.getByRole("heading", {
        name: "2 cuentas abiertas desde hace más de 40 días — fijate si los clientes se fueron.",
      }),
    ).toBeInTheDocument();
    expect(screen.queryByText(/40 days/)).not.toBeInTheDocument();
  });

  it("renders loading skeleton", () => {
    const { container } = render(
      <ProactiveInsights
        insights={[]}
        loading
        t={tStub}
        onOpenConsole={jest.fn()}
      />,
    );
    expect(container.querySelector(".animate-pulse")).toBeInTheDocument();
  });

  it("renders empty state when no insights", () => {
    render(
      <ProactiveInsights insights={[]} t={tStub} onOpenConsole={jest.fn()} />,
    );
    expect(screen.getByText("proactive.empty.copy")).toBeInTheDocument();
    expect(screen.getByText("proactive.empty.label")).toBeInTheDocument();
  });

  it("renders insights list with title and summary", () => {
    render(
      <ProactiveInsights
        insights={[
          {
            id: "i1",
            type: "inventory_out_of_stock",
            params: { count: 3, item_names: ["A", "B"] },
            cta: { tab: "inventory" },
          },
        ]}
        t={tStub}
        onOpenConsole={jest.fn()}
      />,
    );
    expect(
      screen.getByText(
        'proactive.types.inventory_out_of_stock.title_other:{"count":3}',
      ),
    ).toBeInTheDocument();
  });

  it("calls onOpenConsole when Open console button clicked", () => {
    const onOpen = jest.fn();
    render(
      <ProactiveInsights
        insights={[
          {
            id: "i1",
            type: "inventory_out_of_stock",
            params: { count: 3, item_names: ["A"] },
            cta: { tab: "inventory" },
          },
        ]}
        t={tStub}
        onOpenConsole={onOpen}
      />,
    );
    fireEvent.click(screen.getByText("proactive.openConsole"));
    expect(onOpen).toHaveBeenCalled();
  });

  it("limits to 3 insights and shows the first three", () => {
    const insights = [1, 2, 3, 4, 5, 6].map((n) => ({
      id: `i${n}`,
      type: "inventory_out_of_stock",
      params: { count: n, item_names: [`item-${n}`] },
      cta: { tab: "inventory" },
    }));
    render(
      <ProactiveInsights
        insights={insights}
        t={tStub}
        onOpenConsole={jest.fn()}
      />,
    );

    // Titles are rendered inside <h4> elements — match the exact title strings
    // count=1 uses title_one; count>1 uses title_other
    const title = (n: number) => {
      const subKey = n === 1 ? "title_one" : "title_other";
      return `proactive.types.inventory_out_of_stock.${subKey}:${JSON.stringify({ count: n })}`;
    };

    // First three items must appear as headings
    expect(screen.getByRole("heading", { name: title(1) })).toBeInTheDocument();
    expect(screen.getByRole("heading", { name: title(2) })).toBeInTheDocument();
    expect(screen.getByRole("heading", { name: title(3) })).toBeInTheDocument();
    // Items 4–6 must NOT render
    expect(
      screen.queryByRole("heading", { name: title(4) }),
    ).not.toBeInTheDocument();
    expect(
      screen.queryByRole("heading", { name: title(5) }),
    ).not.toBeInTheDocument();
    expect(
      screen.queryByRole("heading", { name: title(6) }),
    ).not.toBeInTheDocument();
  });

  it("renders CTA link for each insight", () => {
    render(
      <ProactiveInsights
        insights={[
          {
            id: "i1",
            type: "inventory_low_stock",
            params: { count: 2, threshold_minutes: 30 },
            cta: { tab: "inventory" },
          },
        ]}
        t={tStub}
        onOpenConsole={jest.fn()}
      />,
    );
    const link = screen.getByRole("link");
    expect(link).toHaveAttribute("href", "?tab=inventory");
    expect(link).toHaveTextContent("proactive.ctas.inventory");
  });

  it("drops insights with unknown types and warns", () => {
    const warnSpy = jest.spyOn(console, "warn").mockImplementation(() => {});

    render(
      <ProactiveInsights
        insights={[
          {
            id: "i1",
            type: "high_table_wait",
            params: { count: 2, threshold_minutes: 30 },
            cta: { tab: "tables" },
          },
        ]}
        t={tStub}
        onOpenConsole={jest.fn()}
      />,
    );
    // Unknown type is filtered; no item links are rendered
    expect(screen.queryByRole("link")).toBeNull();
    // The container still shows (raw insights.length > 0), not the empty-state copy
    expect(screen.queryByText("proactive.empty.copy")).toBeNull();
    expect(warnSpy).toHaveBeenCalledWith(
      "[ProactiveInsights] All insights have unknown types; none rendered.",
      ["high_table_wait"],
    );

    warnSpy.mockRestore();
  });
});
