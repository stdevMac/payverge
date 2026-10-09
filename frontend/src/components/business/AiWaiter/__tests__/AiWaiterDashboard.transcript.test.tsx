/** @jest-environment jsdom */
/**
 * L8 transcript-modal behaviors for AiWaiterDashboard:
 *  - F1: message bubbles render a translated role label ("Guest") / the AI's
 *        configured name instead of the raw backend enum ("USER"/"ASSISTANT").
 *  - F2: the staff-reply input sends on Enter (keyboard submit) during a live
 *        conversation takeover.
 */

import React from "react";
import { act, fireEvent, render, screen, waitFor } from "@testing-library/react";

jest.mock("react-hot-toast", () => {
    const errorFn = jest.fn();
    const successFn = jest.fn();
    const toastFn = Object.assign(jest.fn(), { error: errorFn, success: successFn });
    return { __esModule: true, default: toastFn, toast: toastFn };
});

// Identity-ish translation: return the key so we can assert exact keys, EXCEPT
// the role label key which we resolve to a human string to prove humanization.
jest.mock("@/i18n/SimpleTranslationProvider", () => ({
    useSimpleLocale: () => ({ locale: "en", setLocale: jest.fn() }),
    getTranslation: (key: string) => {
        if (key === "aiWaiterDashboard.transcript.roles.guest") return "Guest";
        return key;
    },
}));

jest.mock("@/api/tools/instance", () => ({
    axiosInstance: { get: jest.fn(), post: jest.fn(), put: jest.fn() },
}));

jest.mock("@/hooks/useBusinessAccess", () => ({ useBusinessAccess: jest.fn() }));

jest.mock("../AiWaiterToggle", () => ({ __esModule: true, default: () => null }));
jest.mock("../../modals/ConfirmationModal", () => ({ __esModule: true, default: () => null }));
jest.mock("../../DashboardLockedTabView", () => ({ __esModule: true, default: () => null }));
jest.mock("../../overview/Metric", () => ({
    __esModule: true,
    default: ({ label, value }: { label: string; value: string }) => (
        <div>
            <span>{label}</span>
            <span>{value}</span>
        </div>
    ),
}));

// HybridAuthProvider transitively imports wagmi (ESM); stub useAuth to an
// owner principal (staffData: null) so the dashboard renders the full view.
jest.mock("@/providers/HybridAuthProvider", () => ({
    useAuth: () => ({ staffData: null }),
}));

import AiWaiterDashboard from "../AiWaiterDashboard";
import { axiosInstance } from "@/api/tools/instance";
import { useBusinessAccess } from "@/hooks/useBusinessAccess";

const mockedAxios = axiosInstance as jest.Mocked<typeof axiosInstance>;

const makeBusiness = () =>
    ({
        id: 42,
        name: "Test Bistro",
        ai_settings: {
            ai_enabled: true,
            ai_name: "Sage",
            ai_priority: "balanced",
            special_instructions: "",
            business_page_ai_enabled: false,
        },
    }) as any;

const SAMPLE_CONV = {
    id: 7,
    session_id: "abcdef0123456789",
    table_code: "T1",
    language: "en",
    status: "active",
    created_at: "2026-04-15T12:00:00Z",
    updated_at: "2026-04-15T12:05:00Z",
    message_count: 3,
    is_paused: true,
    cart_items_added: 0,
    // Replying requires holding the claim, owners included (audit decision #4 —
    // the old owner/manager bypass is gone). `useAuth` is mocked to the owner
    // principal (staffData: null), so the owner-held claim shape is
    // staff-id-less. The unclaimed case has its own test below.
    claimed_by_staff_id: null,
    claimed_by_name: "Owner",
    claimed_by_role: "owner",
    claimed_at: "2026-04-15T12:05:00Z",
};

const TRANSCRIPT = [
    { id: 1, role: "user", content: "Do you have vegan options?", created_at: "2026-04-15T12:00:00Z" },
    { id: 2, role: "assistant", content: "Yes! We have several.", created_at: "2026-04-15T12:01:00Z" },
];

const mockTierActive = () => {
    (useBusinessAccess as jest.Mock).mockReturnValue({
        loading: false,
        hasAccess: true,
        isSuspended: false,
        aiConfigured: true,
        refetch: jest.fn(),
    });
};

const mockGets = (conv: Record<string, unknown> = SAMPLE_CONV) => {
    mockedAxios.get.mockImplementation((url: string) => {
        if (url.includes("/ai/conversations/") && url.includes("/messages")) {
            return Promise.resolve({ data: TRANSCRIPT });
        }
        if (url.includes("/ai/conversations?")) {
            return Promise.resolve({ data: { conversations: [conv], total_pages: 1 } });
        }
        if (url.includes("/whatsapp/status")) {
            return Promise.resolve({ data: { status: "disconnected", enabled: false, built: false } });
        }
        if (url.includes("/ai/insights")) {
            return Promise.resolve({
                data: { total_conversations: 0, total_messages: 0, upsell_success_rate: 0 },
            });
        }
        return Promise.resolve({ data: {} });
    });
};

describe("AiWaiterDashboard transcript modal (L8)", () => {
    beforeEach(() => {
        jest.clearAllMocks();
        jest.spyOn(console, "error").mockImplementation(() => {});
        mockTierActive();
        mockGets();
    });

    afterEach(() => {
        (console.error as jest.Mock).mockRestore?.();
    });

    async function openTranscript() {
        render(<AiWaiterDashboard business={makeBusiness()} onUpdateBusiness={jest.fn()} />);
        const { goToMonitorTab } = await import("./_goToMonitorTab");
        await goToMonitorTab();
        const viewChat = await screen.findByRole("button", { name: /monitor\.viewChat/i });
        await act(async () => {
            fireEvent.click(viewChat);
        });
    }

    it("F1: renders 'Guest' and the AI name as role labels, not the raw enum", async () => {
        await openTranscript();

        // The user bubble shows the translated "Guest" label; the assistant
        // bubble shows the configured AI name "Sage". The raw enum strings must
        // not appear as bubble labels.
        expect(await screen.findByText("Guest")).toBeInTheDocument();
        // "Sage" appears in the header too; ensure at least one is the bubble label.
        const sageLabels = screen.getAllByText("Sage");
        expect(sageLabels.length).toBeGreaterThan(0);
        // The raw role enums must NOT be rendered anywhere.
        expect(screen.queryByText("user")).not.toBeInTheDocument();
        expect(screen.queryByText("assistant")).not.toBeInTheDocument();
    });

    it("R17: renders the conversation start time in the business timezone, not the device/UTC clock", async () => {
        // SAMPLE_CONV.created_at is 2026-04-15T12:00:00Z. In Buenos Aires
        // (UTC-3) the wall-clock is 9:00 AM; the raw UTC/device instant is
        // 12:00 PM. With business.timezone set, the Monitor table must show the
        // business-time hour and never leak the UTC hour.
        render(
            <AiWaiterDashboard
                business={{ ...makeBusiness(), timezone: "America/Argentina/Buenos_Aires" }}
                onUpdateBusiness={jest.fn()}
            />,
        );
        const { goToMonitorTab } = await import("./_goToMonitorTab");
        await goToMonitorTab();
        // Wait for the conversation row to hydrate (the session cell id prefix).
        await screen.findByText(/abcdef01/);
        const hasSubstring = (needle: string) => (content: string) =>
            content.includes(needle);
        // Business-time hour present…
        expect(screen.getByText(hasSubstring("9:00"))).toBeInTheDocument();
        // …and the UTC/device hour absent.
        expect(screen.queryByText(hasSubstring("12:00"))).not.toBeInTheDocument();
    });

    it("F2: pressing Enter in the staff-reply input sends the reply", async () => {
        await openTranscript();

        const replyInput = await screen.findByPlaceholderText(
            /transcript\.staffReplyPlaceholder/i,
        );
        await act(async () => {
            fireEvent.change(replyInput, { target: { value: "On our way!" } });
        });

        mockedAxios.post.mockResolvedValueOnce({ data: {} });
        await act(async () => {
            fireEvent.keyDown(replyInput, { key: "Enter" });
        });

        await waitFor(() => {
            // The reply POST fired (handleSendReply) with the staff content.
            const replyCall = mockedAxios.post.mock.calls.find(
                ([, body]) =>
                    body && typeof body === "object" && (body as any).content === "On our way!",
            );
            expect(replyCall).toBeTruthy();
        });
    });

    it("hides the reply composer until the conversation is claimed (decision #4)", async () => {
        // Same owner principal as above, but nobody holds the claim. Before
        // decision #4 an owner could type straight into a live guest chat while
        // another actor was mid-handover; the composer must now stay closed
        // until this actor takes the claim, so the backend's 409 is never the
        // first thing the operator learns about it.
        mockGets({
            ...SAMPLE_CONV,
            claimed_by_staff_id: null,
            claimed_by_name: "",
            claimed_by_role: "",
            claimed_at: null,
        });
        await openTranscript();

        // The transcript itself is still readable — only the composer is gated.
        expect(await screen.findByText("Guest")).toBeInTheDocument();
        expect(
            screen.queryByPlaceholderText(/transcript\.staffReplyPlaceholder/i),
        ).not.toBeInTheDocument();
    });
});
