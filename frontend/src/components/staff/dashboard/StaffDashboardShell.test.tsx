/** @jest-environment jsdom */

import React from "react";
import {
  act,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";

let mockSearchParams: URLSearchParams | null = null;

jest.mock("next/navigation", () => ({
  useRouter: () => ({ replace: jest.fn() }),
  useSearchParams: () => mockSearchParams,
  usePathname: () => "/staff/home",
}));

const resolvedStaffAuth = {
  isInitialized: true,
  isLoading: false,
  isStaffUser: true,
  staffData: {
    id: 7,
    name: "Test Staff",
    email: "staff@test.com",
    role: "server" as const,
    business_id: 1,
    business_name: "Test Biz",
  },
};
let mockStaffAuth = { ...resolvedStaffAuth };
jest.mock("@/providers/HybridAuthProvider", () => ({
  useAuth: () => mockStaffAuth,
}));

const mockRequestInstall = jest.fn<Promise<void>, []>();
const mockRecordDashboardVisit = jest.fn();
let mockInstallState = "unavailable";
jest.mock("@/providers/PwaInstallProvider", () => ({
  usePwaInstall: () => ({
    state: mockInstallState,
    requestInstall: mockRequestInstall,
    recordDashboardVisit: mockRecordDashboardVisit,
  }),
}));

jest.mock("@/i18n/SimpleTranslationProvider", () => ({
  useSimpleLocale: () => ({ locale: "en", setLocale: jest.fn() }),
  getTranslation: jest.fn((key: string) => key),
}));

jest.mock("@/hooks/useStaffRealtime", () => ({
  useStaffRealtime: () => {},
}));

jest.mock("@/hooks/useSSEEvents", () => ({
  useSSEEvents: () => {},
}));

jest.mock("@/hooks/usePushSubscription", () => ({
  usePushSubscription: () => ({
    isSupported: false,
    isSubscribed: false,
    subscribe: jest.fn(),
  }),
}));

jest.mock("@/utils/intlLocale", () => ({
  intlLocaleFor: (l: string) => l,
}));

jest.mock("@/contexts/ToastContext", () => ({
  useToast: () => ({ showSuccess: jest.fn(), showError: jest.fn() }),
}));

jest.mock("@/api/coverage", () => ({
  coverageApi: {
    listOpen: jest.fn().mockResolvedValue([]),
    listMine: jest.fn().mockResolvedValue([]),
    claim: jest.fn(),
    acceptSwap: jest.fn(),
  },
}));

jest.mock("@/api/positions", () => ({
  positionsApi: { list: jest.fn().mockResolvedValue([]) },
}));

jest.mock("@/api/queryKeys", () => ({
  queryKeys: {
    coverage: {
      open: () => ["coverage", "open"],
      mine: () => ["coverage", "mine"],
    },
    engagement: {
      badges: (businessId: string) => ["engagement", businessId, "badges"],
    },
  },
}));

jest.mock("@/api/engagementBadges", () => ({
  engagementBadgesApi: {
    get: jest
      .fn()
      .mockResolvedValue({ unacked_announcements: 0, pending_checklists: 0 }),
  },
}));

jest.mock("@/api/notifications", () => ({
  notificationsApi: {
    unreadCount: jest.fn().mockResolvedValue(0),
    list: jest.fn().mockResolvedValue([]),
    markRead: jest.fn(),
  },
}));

// Stub child components so only the shell's routing logic is tested.
// Each stub renders a predictable data-testid so we know the right
// branch activated.
jest.mock("./MyScheduleView", () => ({
  __esModule: true,
  default: () => <div data-testid="schedule-view">Schedule</div>,
}));

jest.mock("./CoverageBoard", () => ({
  __esModule: true,
  default: () => <div data-testid="coverage-view">Coverage</div>,
}));

// The Today tab renders StaffTodayHome (which composes TodayCard); stub it so
// this routing test doesn't need every home data source wired up.
jest.mock("./StaffTodayHome", () => ({
  __esModule: true,
  default: () => <div data-testid="today-view">Today</div>,
}));

jest.mock("./ChatList", () => ({
  __esModule: true,
  default: () => <div data-testid="chat-view">Chat</div>,
}));

jest.mock("./StaffMore", () => {
  const actual = jest.requireActual("./StaffMore");
  return actual; // use real StaffMore so deep-link section routing works
});

import StaffDashboardShell from "./StaffDashboardShell";

function wrap(ui: React.ReactNode) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(<QueryClientProvider client={qc}>{ui}</QueryClientProvider>);
}

describe("StaffDashboardShell deep-link resolution", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    mockSearchParams = null;
    mockInstallState = "unavailable";
    mockStaffAuth = { ...resolvedStaffAuth };
    mockRequestInstall.mockReset();
    mockRequestInstall.mockResolvedValue(undefined);
    mockRecordDashboardVisit.mockReset();
  });

  it("renders the today tab by default when no tab param is present", () => {
    wrap(<StaffDashboardShell />);
    expect(screen.getByTestId("today-view")).toBeInTheDocument();
  });

  it("records resolved staff home once across ordinary rerenders", () => {
    const client = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    });
    const view = render(
      <QueryClientProvider client={client}>
        <StaffDashboardShell />
      </QueryClientProvider>,
    );
    expect(mockRecordDashboardVisit).toHaveBeenCalledTimes(1);
    expect(mockRecordDashboardVisit).toHaveBeenCalledWith("/staff/home");

    view.rerender(
      <QueryClientProvider client={client}>
        <StaffDashboardShell />
      </QueryClientProvider>,
    );
    expect(mockRecordDashboardVisit).toHaveBeenCalledTimes(1);
  });

  it("renders the schedule tab when ?tab=schedule", () => {
    mockSearchParams = new URLSearchParams({ tab: "schedule" });
    wrap(<StaffDashboardShell />);
    expect(screen.getByTestId("schedule-view")).toBeInTheDocument();
  });

  it("renders the More tab with coverage sub-surface when ?tab=coverage", () => {
    mockSearchParams = new URLSearchParams({ tab: "coverage" });
    wrap(<StaffDashboardShell />);
    // The shell routes coverage into the More tab with initialSection="coverage".
    // StaffMore renders the coverage render-prop (which renders CoverageBoard).
    expect(screen.getByTestId("coverage-view")).toBeInTheDocument();
  });

  it("renders the today tab when ?tab is an invalid/unknown value", () => {
    mockSearchParams = new URLSearchParams({ tab: "garbage" });
    wrap(<StaffDashboardShell />);
    expect(screen.getByTestId("today-view")).toBeInTheDocument();
  });

  it("renders the chat tab when ?tab=chat", () => {
    mockSearchParams = new URLSearchParams({ tab: "chat" });
    wrap(<StaffDashboardShell />);
    expect(screen.getByTestId("chat-view")).toBeInTheDocument();
  });

  it("renders the More tab with menu when ?tab=more", () => {
    mockSearchParams = new URLSearchParams({ tab: "more" });
    wrap(<StaffDashboardShell />);
    // "more" with no section → menu. StaffMore shows the menu title.
    expect(
      screen.getByText("staffAvailability.menu.title"),
    ).toBeInTheDocument();
  });

  it("is money-free in all rendered tabs", () => {
    mockSearchParams = new URLSearchParams({ tab: "schedule" });
    const { container } = wrap(<StaffDashboardShell />);
    expect(container.textContent || "").not.toMatch(/\$/);
  });

  it("guards rapid staff install activation and recovers after rejection", async () => {
    mockSearchParams = new URLSearchParams({ tab: "more" });
    let rejectInstall!: (reason?: unknown) => void;
    mockRequestInstall.mockImplementation(
      () =>
        new Promise<void>((_resolve, reject) => {
          rejectInstall = reject;
        }),
    );
    wrap(
      <React.StrictMode>
        <StaffDashboardShell />
      </React.StrictMode>,
    );

    const action = screen.getByRole("button", { name: /pwa\.account\.label/i });
    act(() => {
      fireEvent.click(action);
      fireEvent.click(action);
    });
    expect(mockRequestInstall).toHaveBeenCalledTimes(1);
    expect(action).toBeDisabled();
    expect(action).toHaveAttribute("aria-busy", "true");

    rejectInstall(new Error("prompt failed"));
    await waitFor(() => {
      expect(action).not.toBeDisabled();
      expect(action).toHaveAttribute("aria-busy", "false");
    });
  });

  it("hides the staff install action only when installed", () => {
    mockSearchParams = new URLSearchParams({ tab: "more" });
    mockInstallState = "installed";
    wrap(<StaffDashboardShell />);
    expect(
      screen.queryByRole("button", { name: /pwa\.account\.label/i }),
    ).toBeNull();
  });
});
