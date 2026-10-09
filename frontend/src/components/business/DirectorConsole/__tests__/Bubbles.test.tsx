/** @jest-environment jsdom */
import React from "react";
import { render, screen, fireEvent } from "@testing-library/react";
import UserBubble from "../UserBubble";
import AssistantMessage from "../AssistantMessage";
import type { DirectorThreadMessage } from "@/api/directorConsole";

jest.mock("@/i18n/SimpleTranslationProvider", () => ({
  useSimpleLocale: () => ({ locale: "en", setLocale: jest.fn() }),
  getTranslation: (key: string) => key,
}));

describe("UserBubble", () => {
  it("renders right-aligned with brand-tinted background", () => {
    const { container } = render(<UserBubble content="hello" onCopy={jest.fn()} onEdit={jest.fn()} />);
    const wrap = container.querySelector("[data-testid='dc-user-bubble']");
    expect(wrap?.className).toMatch(/justify-end/);
    expect(wrap?.querySelector("div")?.className).toMatch(/bg-brand\/10/);
  });

  it("calls onEdit when Edit clicked", () => {
    const onEdit = jest.fn();
    render(<UserBubble content="hello" onCopy={jest.fn()} onEdit={onEdit} />);
    fireEvent.click(screen.getByRole("button", { name: /edit/i }));
    expect(onEdit).toHaveBeenCalledWith("hello");
  });
});

const baseMessage: DirectorThreadMessage = {
  id: 1,
  thread_id: 1,
  business_id: 1,
  role: "assistant",
  locale: "en",
  content: "summary",
  structured_response: {
    summary: "Tuesday was slow",
    diagnosis: "Lower foot traffic and weather",
    evidence: ["Traffic was 30% below median", "Rain all afternoon"],
    actions: [
      { title: "Promote Tuesday combo", description: "Discount lunch", deep_link: "/business/1/dashboard?tab=menu", priority: "high" },
    ],
    expected_impact: "Recover ~10% of lost Tuesday revenue",
    follow_ups: ["Schedule it now?"],
  },
  created_at: new Date().toISOString(),
};

describe("AssistantMessage", () => {
  it("shows the spoken-line summary and reveals 'See the details' on the first reply", () => {
    render(
      <AssistantMessage
        message={baseMessage}
        isFirstAssistant={true}
        onActionClick={jest.fn()}
        onFeedback={jest.fn()}
        onFollowUpClick={jest.fn()}
        onCopy={jest.fn()}
        onRegenerate={jest.fn()}
      />,
    );
    expect(screen.getByText("Tuesday was slow")).toBeVisible();
    // Diagnosis now lives inside the single "See the details" disclosure,
    // which is opened by default only on the first assistant reply.
    const diagnosis = screen.getByText("Lower foot traffic and weather");
    expect(diagnosis.closest("details")?.open).toBe(true);
  });

  it("encodes action priority as a Pre-Shift-style tone circle (not a clinical dot)", () => {
    const { container } = render(
      <AssistantMessage
        message={baseMessage}
        isFirstAssistant={true}
        onActionClick={jest.fn()}
        onFeedback={jest.fn()}
        onFollowUpClick={jest.fn()}
        onCopy={jest.fn()}
        onRegenerate={jest.fn()}
      />,
    );
    const tone = container.querySelector("[data-testid='dc-action-priority']");
    // high priority → warm rose tone fill, not the old solid bg-rose-500 dot.
    expect(tone?.className).toMatch(/bg-rose-50/);
  });

  it("surfaces actions directly always, and opens 'See the details' on the first reply only", () => {
    const { rerender, container } = render(
      <AssistantMessage
        message={baseMessage}
        isFirstAssistant={true}
        onActionClick={jest.fn()}
        onFeedback={jest.fn()}
        onFollowUpClick={jest.fn()}
        onCopy={jest.fn()}
        onRegenerate={jest.fn()}
      />,
    );

    // Actions are surfaced directly as cards — not gated behind an accordion.
    expect(screen.getByText("Promote Tuesday combo")).toBeVisible();
    // The lone disclosure is the "See the details" section.
    expect(container.querySelector("details")?.open).toBe(true);

    rerender(
      <AssistantMessage
        message={baseMessage}
        isFirstAssistant={false}
        onActionClick={jest.fn()}
        onFeedback={jest.fn()}
        onFollowUpClick={jest.fn()}
        onCopy={jest.fn()}
        onRegenerate={jest.fn()}
      />,
    );

    // Actions stay visible regardless; only the disclosure collapses.
    expect(screen.getByText("Promote Tuesday combo")).toBeVisible();
    expect(container.querySelector("details")?.open).toBe(false);
  });
});
