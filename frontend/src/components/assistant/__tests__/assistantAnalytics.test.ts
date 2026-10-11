/** @jest-environment jsdom */

import React from "react";
import { readFileSync } from "node:fs";
import { join } from "node:path";
import {
  act,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import { trackEvent } from "@/utils/analytics";
import { parseAssistantResponse } from "@/types/assistant";
import ChatShell from "@/components/chat/ChatShell";
import {
  ASSISTANT_EVENT_NAMES,
  type AssistantAnalyticsContext,
  assistantEventProperties,
  trackAssistantEvent,
} from "../assistantAnalytics";
import { AssistantActions } from "../AssistantActions";
import { AssistantMessage } from "../AssistantMessage";
import { AssistantRichText } from "../AssistantRichText";
import { AssistantSources } from "../AssistantSources";
import { AssistantViewport } from "../AssistantViewport";

jest.mock("@/utils/analytics", () => ({
  trackEvent: jest.fn(),
}));

const mockedTrackEvent = jest.mocked(trackEvent);
const analytics: AssistantAnalyticsContext = {
  surface: "ops",
  contract_version: "v2",
  locale: "es-AR",
};

const action = {
  id: "action-menu",
  type: "navigate" as const,
  label: "Open Menu",
  target: {
    kind: "dashboard_area" as const,
    id: "menu",
    href: "/business/7/dashboard?tab=menu",
  },
  state: "ready",
  confirmation: "none",
  disabled_reason: null,
  expires_at: null,
};

const source = {
  id: "source-menu",
  type: "dashboard_guide" as const,
  title: "Menu guide",
  href: "/business/7/dashboard?tab=menu",
  origin: "free form title that must not be emitted",
  retrieved_at: "2026-08-07T12:00:00Z",
};

const messageLabels = {
  usedSources: (count: number) => `Used ${count} sources`,
  sourcesRegion: (count: number) => `${count} source details`,
  sourceOrigin: (origin: string) => `Origin: ${origin}`,
  externalSource: (hostname: string) => `External: ${hostname}`,
  actions: "Actions",
  steps: "Steps",
  entities: "Entities",
  entityAvailability: (availability: string) => availability,
  followUps: "Follow ups",
  notices: "Notices",
  noticeKind: (kind: string) => kind,
  status: (status: string) => status,
  workflowProgress: () => "Progress",
  disabledActionReason: () => "Unavailable",
  renderError: "Could not render",
};

describe("assistantAnalytics", () => {
  beforeEach(() => {
    mockedTrackEvent.mockClear();
  });

  it("exposes only the approved assistant event names", () => {
    expect(ASSISTANT_EVENT_NAMES).toEqual([
      "assistant_opened",
      "assistant_closed",
      "assistant_message_sent",
      "assistant_retry_requested",
      "assistant_render_fallback",
      "assistant_link_clicked",
      "assistant_source_clicked",
      "assistant_action_clicked",
      "assistant_action_completed",
      "assistant_action_failed",
      "assistant_jump_control_shown",
      "assistant_jump_control_clicked",
      "assistant_user_scrolled_away",
      "assistant_feedback_submitted",
      "assistant_history_contract_mismatch",
    ]);

    for (const eventName of ASSISTANT_EVENT_NAMES) {
      trackAssistantEvent(eventName, { surface: "waiter" });
    }

    expect(mockedTrackEvent.mock.calls.map(([name]) => name)).toEqual(
      ASSISTANT_EVENT_NAMES,
    );
  });

  it("drops unknown event names at runtime", () => {
    trackAssistantEvent(
      "assistant_prompt_captured" as (typeof ASSISTANT_EVENT_NAMES)[number],
      { surface: "waiter" },
    );

    expect(mockedTrackEvent).not.toHaveBeenCalled();
  });

  it("passes only normalized bounded properties to the consent-aware ingest", () => {
    trackAssistantEvent("assistant_action_completed", {
      surface: "waiter",
      contract_version: "v2",
      locale: "es_ar",
      action_kind: "add_cart_item",
      source_kind: "menu_item",
      destination_kind: "internal",
      outcome: "completed",
      message_size_bucket: 80,
      response_size_bucket: 1_001,
    });

    expect(mockedTrackEvent).toHaveBeenCalledWith(
      "assistant_action_completed",
      {
        surface: "waiter",
        contract_version: "v2",
        locale: "es-AR",
        action_kind: "add_cart_item",
        source_kind: "menu_item",
        destination_kind: "internal",
        outcome: "completed",
        message_size_bucket: "0-80",
        response_size_bucket: "1001+",
      },
    );
  });

  it.each([
    [0, "0-80"],
    [80, "0-80"],
    [81, "81-300"],
    [300, "81-300"],
    [301, "301-1000"],
    [1_000, "301-1000"],
    [1_001, "1001+"],
  ])("buckets a size of %i as %s", (size, bucket) => {
    trackAssistantEvent("assistant_message_sent", {
      surface: "ops",
      message_size_bucket: size,
    });

    expect(mockedTrackEvent).toHaveBeenCalledWith("assistant_message_sent", {
      surface: "ops",
      message_size_bucket: bucket,
    });
  });

  it("cannot forward content, identifiers, URLs, labels, or raw errors", () => {
    trackAssistantEvent("assistant_action_failed", {
      surface: "ops",
      contract_version: "v2",
      locale: "en",
      action_kind: "Open Margherita for table 42",
      source_kind: "https://private.example/orders/42",
      destination_kind: "/business/private/dashboard",
      outcome: "Card declined for alice@example.com",
      message_size_bucket: "Please add my usual order",
      response_size_bucket: -12,
      prompt: "Please add my usual order",
      response: "I added Margherita",
      label: "Open Margherita",
      exact_url: "https://private.example/orders/42",
      item_name: "Margherita",
      session_id: "session-secret",
      thread_id: "thread-secret",
      error: "Card declined for alice@example.com",
      arbitrary: "not allowed",
    } as never);

    expect(mockedTrackEvent).toHaveBeenCalledWith("assistant_action_failed", {
      surface: "ops",
      contract_version: "v2",
      locale: "en",
    });
    const serialized = JSON.stringify(mockedTrackEvent.mock.calls);
    expect(serialized).not.toMatch(
      /usual order|Margherita|private\.example|alice@example\.com|session-secret|thread-secret|arbitrary/,
    );
  });

  it("drops invalid enum values and non-finite or negative sizes", () => {
    trackAssistantEvent("assistant_render_fallback", {
      surface: "finance",
      contract_version: "v3",
      locale: "unknown-locale",
      action_kind: "delete_everything",
      source_kind: "database_row",
      destination_kind: "javascript:alert(1)",
      outcome: "stack trace",
      message_size_bucket: Number.POSITIVE_INFINITY,
      response_size_bucket: -1,
    } as never);

    expect(mockedTrackEvent).toHaveBeenCalledWith(
      "assistant_render_fallback",
      {},
    );
  });

  it("does not let event-specific properties override trusted context", () => {
    expect(
      assistantEventProperties(
        { surface: "waiter", contract_version: "v2", locale: "es-AR" },
        {
          surface: "ops",
          contract_version: "v1",
          locale: "en",
          action_kind: "navigate",
        } as never,
      ),
    ).toEqual({
      surface: "waiter",
      contract_version: "v2",
      locale: "es-AR",
      action_kind: "navigate",
    });
  });

  it("cannot let an analytics ingest failure escape into product behavior", () => {
    mockedTrackEvent.mockImplementationOnce(() => {
      throw new Error("storage unavailable");
    });

    expect(() =>
      trackAssistantEvent("assistant_action_clicked", {
        surface: "ops",
        action_kind: "navigate",
      }),
    ).not.toThrow();
  });

  it("does not bypass the adapter with raw route or tab properties in owner shells", () => {
    for (const relativePath of [
      "src/components/opsAssistant/OpsAssistantWidget.tsx",
      "src/components/guest/AiWaiter.tsx",
    ]) {
      const sourceCode = readFileSync(
        join(process.cwd(), relativePath),
        "utf8",
      );
      expect(sourceCode).not.toMatch(
        /trackEvent\([\s\S]{0,160}(?:page_path\s*:|tab\s*:)/,
      );
      expect(sourceCode).not.toContain(
        "onVote: (vote) => void onFeedback(vote)",
      );
    }
  });

  it("does not claim feedback submission when the owning callback rejects", async () => {
    render(
      React.createElement(ChatShell, {
        title: "Assistant",
        messages: [{ role: "assistant", content: "Answer" }],
        input: "",
        onInputChange: jest.fn(),
        onSend: jest.fn(),
        onClose: jest.fn(),
        placeholder: "Ask",
        sendLabel: "Send",
        loadingLabel: "Loading",
        dialogLabel: "Assistant",
        closeLabel: "Close",
        feedback: {
          onVote: () => Promise.reject(new Error("private feedback error")),
          upLabel: "Useful",
          downLabel: "Not useful",
        },
        portal: false,
        assistantAnalytics: analytics,
      }),
    );

    fireEvent.click(screen.getByRole("button", { name: "Useful" }));
    await act(async () => {
      await Promise.resolve();
    });

    expect(mockedTrackEvent).not.toHaveBeenCalledWith(
      "assistant_feedback_submitted",
      expect.anything(),
    );
  });

  it("contains a synchronous feedback callback failure", async () => {
    render(
      React.createElement(ChatShell, {
        title: "Assistant",
        messages: [{ role: "assistant", content: "Answer" }],
        input: "",
        onInputChange: jest.fn(),
        onSend: jest.fn(),
        onClose: jest.fn(),
        placeholder: "Ask",
        sendLabel: "Send",
        loadingLabel: "Loading",
        dialogLabel: "Assistant",
        closeLabel: "Close",
        feedback: {
          onVote: () => {
            throw new Error("private synchronous failure");
          },
          upLabel: "Useful",
          downLabel: "Not useful",
        },
        portal: false,
        assistantAnalytics: analytics,
      }),
    );

    expect(() =>
      fireEvent.click(screen.getByRole("button", { name: "Useful" })),
    ).not.toThrow();
    await act(async () => {
      await Promise.resolve();
    });
    expect(mockedTrackEvent).not.toHaveBeenCalledWith(
      "assistant_feedback_submitted",
      expect.anything(),
    );
  });

  it("tracks link and source clicks with classifications but no destinations", () => {
    render(
      React.createElement(
        React.Fragment,
        null,
        React.createElement(AssistantRichText, {
          content:
            "[Register](/business/register) [Docs](https://docs.example.com/private?q=1)",
          analytics,
        }),
        React.createElement(AssistantSources, {
          sources: [source],
          labels: messageLabels,
          analytics,
        }),
      ),
    );

    const pricingLink = screen.getByRole("link", { name: "Register" });
    const docsLink = screen.getByRole("link", { name: /Docs/ });
    pricingLink.addEventListener("click", (event) => event.preventDefault());
    docsLink.addEventListener("click", (event) => event.preventDefault());
    fireEvent.click(pricingLink);
    fireEvent.click(docsLink);
    fireEvent.click(screen.getByRole("button", { name: "Used 1 sources" }));
    const sourceLink = screen.getByRole("link", { name: "Menu guide" });
    sourceLink.addEventListener("click", (event) => event.preventDefault());
    fireEvent.click(sourceLink);

    expect(mockedTrackEvent).toHaveBeenCalledWith("assistant_link_clicked", {
      ...analytics,
      destination_kind: "internal",
    });
    expect(mockedTrackEvent).toHaveBeenCalledWith("assistant_link_clicked", {
      ...analytics,
      destination_kind: "external",
    });
    expect(mockedTrackEvent).toHaveBeenCalledWith("assistant_source_clicked", {
      ...analytics,
      source_kind: "dashboard_guide",
    });
    expect(JSON.stringify(mockedTrackEvent.mock.calls)).not.toMatch(
      /docs\.example\.com|free form title|dashboard\?tab=menu/,
    );
  });

  it("tracks a ready action once and waits for the owning callback outcome", async () => {
    let finish: ((outcome: "completed") => void) | undefined;
    const onAction = jest.fn(
      () =>
        new Promise<"completed">((resolve) => {
          finish = resolve;
        }),
    );

    render(
      React.createElement(AssistantActions, {
        actions: [action],
        labels: messageLabels,
        onAction,
        analytics,
      }),
    );

    const button = screen.getByRole("button", { name: "Open Menu" });
    fireEvent.click(button);
    fireEvent.click(button);

    expect(onAction).toHaveBeenCalledTimes(1);
    expect(mockedTrackEvent).toHaveBeenCalledWith("assistant_action_clicked", {
      ...analytics,
      action_kind: "navigate",
    });
    expect(mockedTrackEvent).not.toHaveBeenCalledWith(
      "assistant_action_completed",
      expect.anything(),
    );

    finish?.("completed");
    await waitFor(() =>
      expect(mockedTrackEvent).toHaveBeenCalledWith(
        "assistant_action_completed",
        { ...analytics, action_kind: "navigate" },
      ),
    );
  });

  it("tracks callback rejection as an action failure without raw errors", async () => {
    render(
      React.createElement(AssistantActions, {
        actions: [action],
        labels: messageLabels,
        onAction: () => Promise.reject(new Error("private order 42 failed")),
        analytics,
      }),
    );

    fireEvent.click(screen.getByRole("button", { name: "Open Menu" }));

    await waitFor(() =>
      expect(mockedTrackEvent).toHaveBeenCalledWith("assistant_action_failed", {
        ...analytics,
        action_kind: "navigate",
      }),
    );
    expect(JSON.stringify(mockedTrackEvent.mock.calls)).not.toContain(
      "private order 42 failed",
    );
  });

  it("tracks jump UI only on a scroll-away transition and its user click", () => {
    const controller = {
      setScroller: jest.fn(),
      onContentChange: jest.fn(),
      jumpToLatest: jest.fn(),
      showJumpToLatest: false,
    };
    const { rerender } = render(
      React.createElement(
        AssistantViewport,
        {
          conversationLabel: "Conversation",
          jumpToLatestLabel: "Latest",
          controller,
          analytics,
        } as unknown as React.ComponentProps<typeof AssistantViewport>,
        React.createElement("p", null, "Answer"),
      ),
    );

    expect(mockedTrackEvent).not.toHaveBeenCalled();
    const scrolledController = {
      ...controller,
      showJumpToLatest: true,
      userScrolledAwayTransition: 1,
    };
    rerender(
      React.createElement(
        AssistantViewport,
        {
          conversationLabel: "Conversation",
          jumpToLatestLabel: "Latest",
          controller: scrolledController,
          analytics,
        } as unknown as React.ComponentProps<typeof AssistantViewport>,
        React.createElement("p", null, "Answer"),
      ),
    );

    expect(mockedTrackEvent).toHaveBeenCalledWith(
      "assistant_user_scrolled_away",
      analytics,
    );
    expect(mockedTrackEvent).toHaveBeenCalledWith(
      "assistant_jump_control_shown",
      analytics,
    );
    fireEvent.click(screen.getByRole("button", { name: "Latest" }));
    expect(mockedTrackEvent).toHaveBeenCalledWith(
      "assistant_jump_control_clicked",
      analytics,
    );
    expect(scrolledController.jumpToLatest).toHaveBeenCalledTimes(1);
  });

  it("tracks a contained render failure once without error content", () => {
    const broken = parseAssistantResponse({
      version: 2,
      response_id: "response-1",
      answer: { format: "plain_text", content: "Safe" },
      sections: [],
      steps: [],
      actions: [],
      sources: [],
      entities: [],
      follow_ups: [],
      workflow: null,
      notices: [],
      status: "complete",
    });
    Object.defineProperty(broken, "answer", {
      get() {
        throw new Error("secret response body");
      },
    });
    const consoleError = jest
      .spyOn(console, "error")
      .mockImplementation(() => undefined);

    render(
      React.createElement(AssistantMessage, {
        response: broken,
        labels: messageLabels,
        analytics,
      }),
    );

    expect(screen.getByRole("alert")).toHaveTextContent("Could not render");
    expect(mockedTrackEvent).toHaveBeenCalledWith(
      "assistant_render_fallback",
      analytics,
    );
    expect(JSON.stringify(mockedTrackEvent.mock.calls)).not.toContain(
      "secret response body",
    );
    consoleError.mockRestore();
  });

  it("tracks shell send, retry, and feedback user outcomes without text", async () => {
    const onSend = jest.fn();
    const onRetry = jest.fn();
    const onVote = jest.fn(() => true);
    render(
      React.createElement(ChatShell, {
        title: "Assistant",
        messages: [{ role: "assistant", content: "Private response" }],
        input: "private prompt",
        onInputChange: jest.fn(),
        onSend,
        onClose: jest.fn(),
        placeholder: "Ask",
        sendLabel: "Send",
        loadingLabel: "Loading",
        dialogLabel: "Assistant",
        closeLabel: "Close",
        errorMessage: "Try again",
        onRetry,
        retryLabel: "Retry",
        feedback: { onVote, upLabel: "Useful", downLabel: "Not useful" },
        portal: false,
        assistantAnalytics: analytics,
      }),
    );

    await act(async () => {
      fireEvent.click(screen.getByRole("button", { name: "Send" }));
    });
    fireEvent.click(screen.getByRole("button", { name: "Retry" }));
    fireEvent.click(screen.getByRole("button", { name: "Useful" }));
    fireEvent.click(screen.getByRole("button", { name: "Useful" }));

    await waitFor(() => expect(onVote).toHaveBeenCalledTimes(1));

    expect(onSend).toHaveBeenCalledTimes(1);
    expect(onRetry).toHaveBeenCalledTimes(1);
    expect(onVote).toHaveBeenCalledWith("up");
    expect(mockedTrackEvent).toHaveBeenCalledWith("assistant_message_sent", {
      ...analytics,
      message_size_bucket: "0-80",
    });
    expect(mockedTrackEvent).toHaveBeenCalledWith(
      "assistant_retry_requested",
      analytics,
    );
    await waitFor(() =>
      expect(mockedTrackEvent).toHaveBeenCalledWith(
        "assistant_feedback_submitted",
        { ...analytics, outcome: "positive" },
      ),
    );
    expect(
      mockedTrackEvent.mock.calls.filter(
        ([name]) => name === "assistant_feedback_submitted",
      ),
    ).toHaveLength(1);
    expect(JSON.stringify(mockedTrackEvent.mock.calls)).not.toMatch(
      /private prompt|Private response|Try again/,
    );
  });
});
