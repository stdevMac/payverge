/** @jest-environment jsdom */
import React from "react";
import {
  act,
  fireEvent,
  render,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import AdminTranslationsPage from "./page";
import {
  getMissingTranslations,
  updateMissingTranslationStatus,
  type MissingTranslationRow,
} from "@/api/adminMissingTranslations";

jest.mock("@/api/adminMissingTranslations", () => ({
  getMissingTranslations: jest.fn(),
  updateMissingTranslationStatus: jest.fn(),
}));

jest.mock("@/components/admin/primitives", () => ({
  AdminPageFrame: ({
    children,
    description,
    actions,
  }: {
    children: React.ReactNode;
    description: string;
    actions?: React.ReactNode;
  }) => (
    <main>
      <p>{description}</p>
      <div>{actions}</div>
      {children}
    </main>
  ),
}));

jest.mock("@nextui-org/react", () => ({
  Button: ({
    children,
    onPress,
    isDisabled,
    isLoading,
    disabled,
    "aria-label": ariaLabel,
  }: {
    children: React.ReactNode;
    onPress?: () => void;
    isDisabled?: boolean;
    isLoading?: boolean;
    disabled?: boolean;
    "aria-label"?: string;
  }) => (
    <button
      type="button"
      disabled={Boolean(isDisabled || disabled)}
      aria-busy={isLoading || undefined}
      aria-label={ariaLabel}
      onClick={onPress}
    >
      {children}
    </button>
  ),
  Card: ({ children }: { children: React.ReactNode }) => (
    <section>{children}</section>
  ),
  CardBody: ({ children }: { children: React.ReactNode }) => (
    <div>{children}</div>
  ),
  CardHeader: ({ children }: { children: React.ReactNode }) => (
    <header>{children}</header>
  ),
  Chip: ({ children }: { children: React.ReactNode }) => (
    <span>{children}</span>
  ),
  Select: ({
    children,
    label,
    onChange,
    selectedKeys,
  }: {
    children: React.ReactNode;
    label: string;
    onChange?: (event: React.ChangeEvent<HTMLSelectElement>) => void;
    selectedKeys?: string[];
  }) => (
    <label>
      {label}
      <select
        aria-label={label}
        value={selectedKeys?.[0] ?? "all"}
        onChange={onChange}
      >
        {children}
      </select>
    </label>
  ),
  SelectItem: ({
    children,
    value,
  }: {
    children: React.ReactNode;
    value?: string;
  }) => <option value={value}>{children}</option>,
  Spinner: () => <div role="status">Loading</div>,
}));

const mockedGetMissingTranslations =
  getMissingTranslations as jest.MockedFunction<typeof getMissingTranslations>;
const mockedUpdateMissingTranslationStatus =
  updateMissingTranslationStatus as jest.MockedFunction<
    typeof updateMissingTranslationStatus
  >;

function report(
  overrides: Partial<MissingTranslationRow> = {},
): MissingTranslationRow {
  return {
    id: 1,
    locale: "en",
    key_path: "businessDashboard.dashboard.liveBills.loading",
    page: "/business/demo/dashboard",
    fallback_used: "leaf",
    hit_count: 7,
    first_seen_at: "2026-07-01T00:00:00Z",
    last_seen_at: new Date(Date.now() - 5 * 60 * 1000).toISOString(),
    status: "open",
    status_updated_at: null,
    ...overrides,
  };
}

function response(rows: MissingTranslationRow[], total = rows.length) {
  return { rows, total, limit: 100, offset: 0 };
}

function actionName(
  action: "Resolve" | "Ignore" | "Reopen",
  row: MissingTranslationRow,
) {
  return `${action} ${row.key_path} for ${row.locale} on ${row.page || "unknown page"} (${row.fallback_used} fallback, report ${row.id})`;
}

function deferred<T>() {
  let resolve!: (value: T) => void;
  let reject!: (reason?: unknown) => void;
  const promise = new Promise<T>((resolvePromise, rejectPromise) => {
    resolve = resolvePromise;
    reject = rejectPromise;
  });
  return { promise, resolve, reject };
}

describe("AdminTranslationsPage", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    mockedGetMissingTranslations.mockReset();
    mockedUpdateMissingTranslationStatus.mockReset();
    mockedUpdateMissingTranslationStatus.mockResolvedValue();
  });

  it("shows hit counts, supports es-AR filtering, and paginates missing reports", async () => {
    mockedGetMissingTranslations
      .mockResolvedValueOnce({
        rows: [
          {
            ...report(),
          },
        ],
        total: 125,
        limit: 100,
        offset: 0,
      })
      .mockResolvedValueOnce({
        rows: [
          {
            ...report({
              id: 2,
              locale: "es-AR",
              key_path: "businessDashboard.dateRange.startDate",
              hit_count: 3,
            }),
          },
        ],
        total: 125,
        limit: 100,
        offset: 100,
      });

    render(<AdminTranslationsPage />);

    expect(
      await screen.findByText("businessDashboard.dashboard.liveBills.loading"),
    ).toBeInTheDocument();
    expect(screen.getByText("7 hits")).toBeInTheDocument();
    expect(screen.getByText(/125 reports/i)).toBeInTheDocument();

    const localeFilter = screen.getByLabelText("Locale filter");
    expect(
      within(localeFilter).getByRole("option", { name: "es-AR" }),
    ).toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: /next/i }));

    await waitFor(() =>
      expect(mockedGetMissingTranslations).toHaveBeenLastCalledWith({
        limit: 100,
        offset: 100,
        locale: undefined,
        status: "open",
      }),
    );
    expect(
      await screen.findByText("businessDashboard.dateRange.startDate"),
    ).toBeInTheDocument();
    expect(screen.getByText("Page 2 of 2")).toBeInTheDocument();
  });

  it("defaults to Open reports and renders relative plus absolute last-seen time", async () => {
    const lastSeen = new Date(Date.now() - 5 * 60 * 1000).toISOString();
    mockedGetMissingTranslations.mockResolvedValue(
      response([report({ last_seen_at: lastSeen })]),
    );

    render(<AdminTranslationsPage />);

    await waitFor(() =>
      expect(mockedGetMissingTranslations).toHaveBeenCalledWith({
        limit: 100,
        offset: 0,
        locale: undefined,
        status: "open",
      }),
    );
    expect(screen.getByLabelText("Status filter")).toHaveValue("open");

    const timestamp = await screen.findByText(/Last seen 5 minutes ago/i);
    expect(timestamp.tagName).toBe("TIME");
    expect(timestamp).toHaveAttribute("dateTime", lastSeen);
    expect(timestamp).toHaveTextContent(
      new Intl.DateTimeFormat("en", {
        dateStyle: "medium",
        timeStyle: "short",
      }).format(new Date(lastSeen)),
    );
  });

  it.each([
    ["Resolve", "resolved"],
    ["Ignore", "ignored"],
  ] as const)(
    "%s updates the report and refreshes the current Open queue",
    async (action, status) => {
      const row = report();
      mockedGetMissingTranslations
        .mockResolvedValueOnce(response([row]))
        .mockResolvedValueOnce(response([]));

      render(<AdminTranslationsPage />);

      fireEvent.click(
        await screen.findByRole("button", {
          name: actionName(action, row),
        }),
      );

      await waitFor(() =>
        expect(mockedUpdateMissingTranslationStatus).toHaveBeenCalledWith(
          1,
          status,
        ),
      );
      await waitFor(() =>
        expect(mockedGetMissingTranslations).toHaveBeenCalledTimes(2),
      );
      expect(mockedGetMissingTranslations).toHaveBeenLastCalledWith({
        limit: 100,
        offset: 0,
        locale: undefined,
        status: "open",
      });
    },
  );

  it.each(["resolved", "ignored"] as const)(
    "%s reports expose a Reopen action",
    async (status) => {
      const row = report({
        status,
        status_updated_at: "2026-07-19T12:00:00Z",
      });
      mockedGetMissingTranslations
        .mockResolvedValueOnce(response([]))
        .mockResolvedValueOnce(response([row]));

      render(<AdminTranslationsPage />);

      fireEvent.change(screen.getByLabelText("Status filter"), {
        target: { value: status },
      });

      expect(
        await screen.findByRole("button", {
          name: actionName("Reopen", row),
        }),
      ).toBeInTheDocument();
      expect(mockedGetMissingTranslations).toHaveBeenLastCalledWith({
        limit: 100,
        offset: 0,
        locale: undefined,
        status,
      });
    },
  );

  it("disables only the row being updated", async () => {
    let finishUpdate: (() => void) | undefined;
    mockedUpdateMissingTranslationStatus.mockImplementation(
      () =>
        new Promise<void>((resolve) => {
          finishUpdate = resolve;
        }),
    );
    const firstReport = report();
    const secondReport = report({
      id: 2,
      key_path: "businessDashboard.dateRange.startDate",
    });
    mockedGetMissingTranslations.mockResolvedValue(
      response([firstReport, secondReport]),
    );

    render(<AdminTranslationsPage />);

    const firstRow = await screen.findByRole("article", {
      name: "Missing translation businessDashboard.dashboard.liveBills.loading",
    });
    const secondRow = screen.getByRole("article", {
      name: "Missing translation businessDashboard.dateRange.startDate",
    });
    fireEvent.click(
      within(firstRow).getByRole("button", {
        name: actionName("Ignore", firstReport),
      }),
    );

    await waitFor(() =>
      expect(
        within(firstRow).getByRole("button", {
          name: actionName("Ignore", firstReport),
        }),
      ).toBeDisabled(),
    );
    expect(
      within(firstRow).getByRole("button", {
        name: actionName("Ignore", firstReport),
      }),
    ).toHaveAttribute("aria-busy", "true");
    expect(
      within(secondRow).getByRole("button", {
        name: actionName("Ignore", secondReport),
      }),
    ).toBeEnabled();

    await act(async () => {
      finishUpdate?.();
    });
    await waitFor(() =>
      expect(
        within(firstRow).getByRole("button", {
          name: actionName("Ignore", firstReport),
        }),
      ).toBeEnabled(),
    );
  });

  it("shows a row-level mutation error and restores its actions", async () => {
    const rowReport = report();
    mockedGetMissingTranslations.mockResolvedValue(response([rowReport]));
    mockedUpdateMissingTranslationStatus.mockRejectedValue(
      new Error("network"),
    );

    render(<AdminTranslationsPage />);

    const row = await screen.findByRole("article", {
      name: "Missing translation businessDashboard.dashboard.liveBills.loading",
    });
    fireEvent.click(
      within(row).getByRole("button", {
        name: actionName("Ignore", rowReport),
      }),
    );

    expect(await within(row).findByRole("alert")).toHaveTextContent(
      "Failed to update report status. Try again.",
    );
    expect(
      within(row).getByRole("button", {
        name: actionName("Ignore", rowReport),
      }),
    ).toBeEnabled();
    expect(mockedGetMissingTranslations).toHaveBeenCalledTimes(1);
  });

  it("gives duplicate keys unique lifecycle names with locale and source context", async () => {
    const englishReport = report();
    const spanishReport = report({
      id: 2,
      locale: "es-AR",
      page: "/business/otra/dashboard",
      fallback_used: "namespace",
    });
    mockedGetMissingTranslations.mockResolvedValue(
      response([englishReport, spanishReport]),
    );

    render(<AdminTranslationsPage />);

    expect(
      await screen.findByRole("button", {
        name: actionName("Resolve", englishReport),
      }),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("button", {
        name: actionName("Resolve", spanishReport),
      }),
    ).toBeInTheDocument();
  });

  it("refreshes a completed mutation with the filters active at completion time", async () => {
    let finishUpdate: (() => void) | undefined;
    const openReport = report();
    const ignoredReport = report({
      id: 2,
      key_path: "businessDashboard.ignored.example",
      status: "ignored",
      status_updated_at: "2026-07-19T12:00:00Z",
    });
    mockedUpdateMissingTranslationStatus.mockImplementation(
      () =>
        new Promise<void>((resolve) => {
          finishUpdate = resolve;
        }),
    );
    mockedGetMissingTranslations.mockImplementation(async (params) =>
      params?.status === "ignored"
        ? response([ignoredReport])
        : response([openReport]),
    );

    render(<AdminTranslationsPage />);

    fireEvent.click(
      await screen.findByRole("button", {
        name: actionName("Resolve", openReport),
      }),
    );
    await waitFor(() =>
      expect(mockedUpdateMissingTranslationStatus).toHaveBeenCalledWith(
        openReport.id,
        "resolved",
      ),
    );

    fireEvent.change(screen.getByLabelText("Status filter"), {
      target: { value: "ignored" },
    });
    expect(
      await screen.findByText("businessDashboard.ignored.example"),
    ).toBeInTheDocument();

    await act(async () => {
      finishUpdate?.();
    });

    await waitFor(() =>
      expect(mockedGetMissingTranslations).toHaveBeenCalledTimes(3),
    );
    expect(mockedGetMissingTranslations).toHaveBeenLastCalledWith({
      limit: 100,
      offset: 0,
      locale: undefined,
      status: "ignored",
    });
    expect(
      screen.getByText("businessDashboard.ignored.example"),
    ).toBeInTheDocument();
    expect(
      screen.queryByText("businessDashboard.dashboard.liveBills.loading"),
    ).not.toBeInTheDocument();
  });

  it("ignores a late response from a filter that is no longer active", async () => {
    const resolvedRequest = deferred<ReturnType<typeof response>>();
    const ignoredRequest = deferred<ReturnType<typeof response>>();
    const resolvedReport = report({
      id: 2,
      key_path: "businessDashboard.resolved.example",
      status: "resolved",
    });
    const ignoredReport = report({
      id: 3,
      key_path: "businessDashboard.ignored.example",
      status: "ignored",
    });
    mockedGetMissingTranslations.mockImplementation((params) => {
      if (params?.status === "resolved") return resolvedRequest.promise;
      if (params?.status === "ignored") return ignoredRequest.promise;
      return Promise.resolve(response([report()]));
    });

    render(<AdminTranslationsPage />);
    await screen.findByText("businessDashboard.dashboard.liveBills.loading");

    fireEvent.change(screen.getByLabelText("Status filter"), {
      target: { value: "resolved" },
    });
    await waitFor(() =>
      expect(mockedGetMissingTranslations).toHaveBeenLastCalledWith(
        expect.objectContaining({ status: "resolved" }),
      ),
    );
    fireEvent.change(screen.getByLabelText("Status filter"), {
      target: { value: "ignored" },
    });
    await waitFor(() =>
      expect(mockedGetMissingTranslations).toHaveBeenLastCalledWith(
        expect.objectContaining({ status: "ignored" }),
      ),
    );

    await act(async () => {
      ignoredRequest.resolve(response([ignoredReport]));
    });
    expect(
      await screen.findByText("businessDashboard.ignored.example"),
    ).toBeInTheDocument();

    await act(async () => {
      resolvedRequest.resolve(response([resolvedReport]));
    });
    expect(
      screen.getByText("businessDashboard.ignored.example"),
    ).toBeInTheDocument();
    expect(
      screen.queryByText("businessDashboard.resolved.example"),
    ).not.toBeInTheDocument();
  });

  it("keeps the current request loading when an older request rejects", async () => {
    const resolvedRequest = deferred<ReturnType<typeof response>>();
    const ignoredRequest = deferred<ReturnType<typeof response>>();
    const ignoredReport = report({
      id: 3,
      key_path: "businessDashboard.ignored.example",
      status: "ignored",
    });
    mockedGetMissingTranslations.mockImplementation((params) => {
      if (params?.status === "resolved") return resolvedRequest.promise;
      if (params?.status === "ignored") return ignoredRequest.promise;
      return Promise.resolve(response([report()]));
    });

    render(<AdminTranslationsPage />);
    await screen.findByText("businessDashboard.dashboard.liveBills.loading");

    fireEvent.change(screen.getByLabelText("Status filter"), {
      target: { value: "resolved" },
    });
    await waitFor(() =>
      expect(mockedGetMissingTranslations).toHaveBeenLastCalledWith(
        expect.objectContaining({ status: "resolved" }),
      ),
    );
    fireEvent.change(screen.getByLabelText("Status filter"), {
      target: { value: "ignored" },
    });
    await waitFor(() =>
      expect(mockedGetMissingTranslations).toHaveBeenLastCalledWith(
        expect.objectContaining({ status: "ignored" }),
      ),
    );

    await act(async () => {
      resolvedRequest.reject(new Error("stale failure"));
    });
    expect(screen.getByText("Loading")).toBeInTheDocument();
    expect(
      screen.queryByText("Failed to load missing translation reports."),
    ).not.toBeInTheDocument();

    await act(async () => {
      ignoredRequest.resolve(response([ignoredReport]));
    });
    expect(
      await screen.findByText("businessDashboard.ignored.example"),
    ).toBeInTheDocument();
  });

  it("clamps and reloads when a mutation empties the last page", async () => {
    const firstPageReport = report({
      key_path: "businessDashboard.firstPage.example",
    });
    const lastPageReport = report({
      id: 101,
      key_path: "businessDashboard.lastPage.example",
    });
    mockedGetMissingTranslations
      .mockResolvedValueOnce(response([firstPageReport], 101))
      .mockResolvedValueOnce({
        ...response([lastPageReport], 101),
        offset: 100,
      })
      .mockResolvedValueOnce({ ...response([], 100), offset: 100 })
      .mockResolvedValueOnce(response([firstPageReport], 100));

    render(<AdminTranslationsPage />);
    await screen.findByText("businessDashboard.firstPage.example");

    fireEvent.click(screen.getByRole("button", { name: "Next" }));
    await screen.findByText("businessDashboard.lastPage.example");

    fireEvent.click(
      screen.getByRole("button", {
        name: actionName("Resolve", lastPageReport),
      }),
    );

    await waitFor(() =>
      expect(mockedGetMissingTranslations).toHaveBeenCalledTimes(4),
    );
    expect(mockedGetMissingTranslations).toHaveBeenLastCalledWith({
      limit: 100,
      offset: 0,
      locale: undefined,
      status: "open",
    });
    expect(
      await screen.findByText("businessDashboard.firstPage.example"),
    ).toBeInTheDocument();
    expect(screen.getByText("Page 1 of 1")).toBeInTheDocument();
    expect(mockedGetMissingTranslations).toHaveBeenCalledTimes(4);
  });

  it("alerts when an open English report is older than 48 hours", async () => {
    mockedGetMissingTranslations.mockResolvedValue(
      response([
        report({
          id: 99,
          locale: "en",
          first_seen_at: new Date(Date.now() - 49 * 60 * 60 * 1000).toISOString(),
          status: "open",
        }),
        report({
          id: 100,
          locale: "es",
          first_seen_at: new Date(Date.now() - 72 * 60 * 60 * 1000).toISOString(),
          status: "open",
          key_path: "other.key",
        }),
      ]),
    );

    render(<AdminTranslationsPage />);

    expect(
      await screen.findByRole("alert"),
    ).toHaveTextContent(/English missing-translation report has been open for more than 48 hours/i);
    expect(screen.getByText(/open >48h/i)).toBeInTheDocument();
    // Non-en stale open rows do not raise the default-locale alert.
    expect(screen.getByRole("alert")).not.toHaveTextContent("other.key");
  });

});
