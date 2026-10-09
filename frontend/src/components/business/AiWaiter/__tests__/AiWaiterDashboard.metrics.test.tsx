/** @jest-environment jsdom */
/**
 * Regression coverage for B4: the AI Waiter dashboard StatsSummary on the
 * "Monitor" tab used to render zeros because fetchInsights only ran when the
 * "Insights" tab was selected. Recent Conversations would display 9+ active
 * sessions while the metric cards at the top of the same tab claimed
 * "Total Conversations: 0".
 *
 * L4-11: Monitor used Math.round while Insights used toFixed(1), so the same
 * ratio could show as "3" vs "2.6". Both surfaces must share formatAvgMessages.
 */

import React from "react";
import { act, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";

jest.mock("react-hot-toast", () => {
    const errorFn = jest.fn();
    const successFn = jest.fn();
    const toastFn = Object.assign(jest.fn(), {
        error: errorFn,
        success: successFn,
    });
    return {
        __esModule: true,
        default: toastFn,
        toast: toastFn,
    };
});

jest.mock("@/i18n/SimpleTranslationProvider", () => ({
    useSimpleLocale: () => ({ locale: "en", setLocale: jest.fn() }),
    getTranslation: (key: string) => key,
}));

jest.mock("@/api/tools/instance", () => ({
    axiosInstance: {
        get: jest.fn(),
        post: jest.fn(),
        put: jest.fn(),
    },
}));

jest.mock("@/hooks/useBusinessAccess", () => ({
    useBusinessAccess: jest.fn(),
}));

jest.mock("../AiWaiterToggle", () => ({
    __esModule: true,
    default: () => null,
}));
jest.mock("../../modals/ConfirmationModal", () => ({
    __esModule: true,
    default: () => null,
}));
jest.mock("../../DashboardLockedTabView", () => ({
    __esModule: true,
    default: () => null,
}));
jest.mock("../../overview/Metric", () => ({
    __esModule: true,
    default: ({ label, value }: { label: string; value: string }) => (
        <div data-testid={`metric-${label}`}>
            <span>{label}</span>
            <span data-testid={`metric-value-${label}`}>{value}</span>
        </div>
    ),
}));

// HybridAuthProvider transitively imports wagmi (ESM); stub useAuth to an
// owner principal (staffData: null) so the dashboard renders the full view.
jest.mock("@/providers/HybridAuthProvider", () => ({
    useAuth: () => ({ staffData: null }),
}));

import AiWaiterDashboard, {
    formatAvgMessages,
} from "../AiWaiterDashboard";
import { axiosInstance } from "@/api/tools/instance";
import { useBusinessAccess } from "@/hooks/useBusinessAccess";

const mockedAxios = axiosInstance as jest.Mocked<typeof axiosInstance>;

const makeBusiness = () =>
    ({
        id: 42,
        name: "Test Bistro",
        owner_address: "0x0",
        logo: "",
        address: {},
        settlement_address: "",
        tipping_address: "",
        tax_rate: 0,
        service_fee_rate: 0,
        tax_inclusive: false,
        service_inclusive: false,
        is_active: true,
        business_page_enabled: true,
        ai_settings: {
            ai_enabled: true,
            ai_name: "Sage",
            ai_priority: "balanced",
            special_instructions: "",
            business_page_ai_enabled: false,
        },
    }) as any;

const mockTierActive = () => {
    (useBusinessAccess as jest.Mock).mockReturnValue({
        access: null,
        loading: false,
        error: null,
        hasAccess: true,
        isSuspended: false,
        lockState: "active",
        aiConfigured: true,
        refetch: jest.fn(),
    });
};

describe("formatAvgMessages (L4-11)", () => {
    it("formats 13/5 as one-decimal '2.6' (not Math.round → 3)", () => {
        expect(formatAvgMessages(13, 5)).toBe("2.6");
    });

    it("returns '0' when there are no conversations", () => {
        expect(formatAvgMessages(10, 0)).toBe("0");
    });
});

describe("AiWaiterDashboard insights metrics on Monitor tab (B4 + L4-11)", () => {
    beforeEach(() => {
        jest.clearAllMocks();
        mockTierActive();
    });

    it("fetches insights on mount so StatsSummary reflects live data, not zeros", async () => {
        // Default Monitor tab is selected. Recent Conversations renders 5 active
        // sessions with 13 messages total — ratio 2.6 must match Insights.
        mockedAxios.get.mockImplementation((url: string) => {
            if (url.includes("/ai/insights")) {
                return Promise.resolve({
                    data: {
                        total_conversations: 5,
                        total_messages: 13,
                        upsell_success_rate: 33.3,
                    },
                });
            }
            if (url.includes("/ai/conversations?")) {
                return Promise.resolve({
                    data: {
                        conversations: Array.from({ length: 5 }, (_, i) => ({
                            id: i + 1,
                            session_id: `sess-${i}`,
                            table_code: "T1",
                            language: "en",
                            status: "active",
                            created_at: "2026-05-12T12:00:00Z",
                            updated_at: "2026-05-12T12:05:00Z",
                            message_count: 2,
                            is_paused: false,
                            cart_items_added: 0,
                        })),
                        total_pages: 1,
                    },
                });
            }
            if (url.includes("/whatsapp/status")) {
                return Promise.resolve({ data: { status: "disconnected", enabled: false, built: false } });
            }
            return Promise.resolve({ data: {} });
        });

        await act(async () => {
            render(
                <AiWaiterDashboard
                    business={makeBusiness()}
                    onUpdateBusiness={jest.fn()}
                />,
            );
        });

        // Overview is the default landing tab; insights load when Monitor opens
        // so StatsSummary never paints stale zeros.
        const { goToMonitorTab } = await import("./_goToMonitorTab");
        await goToMonitorTab();

        await waitFor(() => {
            const insightsCalls = mockedAxios.get.mock.calls.filter(([url]) =>
                String(url).includes("/ai/insights"),
            );
            expect(insightsCalls.length).toBeGreaterThanOrEqual(1);
        });

        // HonestMetric tiles share consistent empty/ok treatment across the row.
        await waitFor(() => {
            const total = screen.getByTestId("metric-total-conversations");
            expect(total.getAttribute("data-state")).toBe("ok");
            expect(total.textContent).toMatch(/5/);
        });

        const upsellCell = screen.getByTestId("metric-upsell-success-rate");
        expect(upsellCell.textContent).toMatch(/33\.3%/);
        expect(upsellCell.getAttribute("data-state")).toBe("ok");

        const monitorAvg = screen.getByTestId("metric-avg-messages");
        // L4-11: toFixed(1) → "2.6", not Math.round → "3"
        expect(monitorAvg.textContent).toMatch(/2\.6/);

        // Switch to Insights (Estadísticas) and assert the same formatting.
        await userEvent.click(
            screen.getByRole("tab", { name: "aiWaiterDashboard.tabs.insights" }),
        );

        await waitFor(() => {
            const insightsAvg = screen.getByTestId("metric-avg-messages-insights");
            expect(insightsAvg.textContent).toMatch(/2\.6/);
        });
    });
});
