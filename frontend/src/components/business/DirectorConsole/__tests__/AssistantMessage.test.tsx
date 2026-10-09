/** @jest-environment jsdom */
/**
 * Phase 3 — de-clinicalized chat answer.
 *
 * AssistantMessage now reads like a GM talking to the operator, not a lab
 * report: a spoken-line summary (clean sans, never serif `font-title`), the
 * actions surfaced directly as Pre-Shift-style cards, and a single quiet
 * "See the details" disclosure replacing the old DIAGNOSIS / EVIDENCE /
 * ACTION PLAN / EXPECTED IMPACT accordions.
 *
 * The translation provider is mocked to echo its key (with params appended)
 * so assertions key off the stable `directorConsole.*` paths.
 */
import React from "react";
import { render, screen, fireEvent } from "@testing-library/react";

import AssistantMessage from "../AssistantMessage";
import type {
  DirectorAction,
  DirectorThreadMessage,
} from "@/api/directorConsole";

jest.mock("@/i18n/SimpleTranslationProvider", () => ({
  useSimpleLocale: () => ({ locale: "en", setLocale: jest.fn() }),
  getTranslation: (key: string, _locale: unknown, params?: Record<string, unknown>) =>
    params ? `${key} ${JSON.stringify(params)}` : key,
}));

const highAction: DirectorAction = {
  title: "Run a lunch promo Tuesday",
  description: "Push a 2-for-1 to win back the slow midday.",
  deep_link: "/business/marketing",
  priority: "high",
};

const lowAction: DirectorAction = {
  title: "Notify regulars",
  description: "Send the loyalty list a heads-up.",
  deep_link: "/business/loyalty",
  priority: "low",
};

function makeMessage(overrides: Partial<DirectorThreadMessage> = {}): DirectorThreadMessage {
  return {
    id: 42,
    thread_id: 1,
    business_id: 1,
    role: "assistant",
    locale: "en",
    content: "fallback markdown",
    created_at: "2026-06-30T00:00:00Z",
    structured_response: {
      summary: "Tuesday lunch slumped 12% — let's win it back.",
      diagnosis: "A slow midday dragged the day down.",
      evidence: ["Lunch covers down 12%", "Dinner held steady"],
      actions: [highAction, lowAction],
      expected_impact: "Roughly AED 800 of recovery next week.",
      follow_ups: ["What drove the slump?"],
    },
    ...overrides,
  };
}

function renderMessage(
  overrides: Partial<DirectorThreadMessage> = {},
  props: Partial<React.ComponentProps<typeof AssistantMessage>> = {},
) {
  const onActionClick = jest.fn();
  const utils = render(
    <AssistantMessage
      message={makeMessage(overrides)}
      isFirstAssistant={false}
      onActionClick={onActionClick}
      onFeedback={jest.fn()}
      onFollowUpClick={jest.fn()}
      onCopy={jest.fn()}
      onRegenerate={jest.fn()}
      {...props}
    />,
  );
  return { onActionClick, ...utils };
}

describe("AssistantMessage — conversational chat answer", () => {
  it("renders the action cards directly (no accordion) by default", () => {
    renderMessage();
    expect(screen.getByText("Run a lunch promo Tuesday")).toBeInTheDocument();
    expect(screen.getByText("Notify regulars")).toBeInTheDocument();
  });

  it("calls onActionClick with the clicked action", () => {
    const { onActionClick } = renderMessage();
    fireEvent.click(
      screen.getByRole("button", { name: /Run a lunch promo Tuesday/ }),
    );
    expect(onActionClick).toHaveBeenCalledTimes(1);
    expect(onActionClick).toHaveBeenCalledWith(highAction);
  });

  it("gives each action card its own accessible name (open + title)", () => {
    renderMessage();
    // Each card's aria-label combines the open affordance with its title so
    // screen-reader users can tell the buttons apart.
    expect(
      screen.getByRole("button", {
        name: /directorConsole\.actions\.open.*Run a lunch promo Tuesday/,
      }),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("button", {
        name: /directorConsole\.actions\.open.*Notify regulars/,
      }),
    ).toBeInTheDocument();
  });

  it("encodes priority on the dc-action-priority tone circle", () => {
    renderMessage();
    const tones = screen.getAllByTestId("dc-action-priority");
    expect(tones).toHaveLength(2);
    // high → rose tone, low → brand tone
    expect(tones[0].className).toContain("bg-rose-50");
    expect(tones[0].getAttribute("title")).toBe("high");
    expect(tones[1].className).toContain("bg-brand/10");
  });

  it("renders the medium (amber) tone and falls back to ink for an unknown priority", () => {
    const mediumAction: DirectorAction = { ...highAction, priority: "medium" };
    // An unknown priority (and, by extension, a missing one) has no tone mapping
    // and must land on the neutral ink fallback badge — never an uncolored gap.
    const unknownAction: DirectorAction = {
      ...lowAction,
      priority: "unspecified" as unknown as DirectorAction["priority"],
    };
    renderMessage({
      structured_response: {
        summary: "Two actions, two tones.",
        diagnosis: "",
        evidence: [],
        actions: [mediumAction, unknownAction],
        expected_impact: "",
        follow_ups: [],
      },
    });
    const tones = screen.getAllByTestId("dc-action-priority");
    expect(tones[0].className).toContain("bg-amber-50");
    expect(tones[1].className).toContain("bg-warm-100");
  });

  it("collapses diagnosis/evidence/impact into one quiet 'See the details' disclosure with warm sub-labels", () => {
    renderMessage();
    // Single sentence-case disclosure, not uppercase lab accordions.
    expect(screen.getByText("directorConsole.chat.details")).toBeInTheDocument();
    // Warm sentence-case sub-labels inside.
    expect(screen.getByText("directorConsole.sections.context")).toBeInTheDocument();
    expect(screen.getByText("directorConsole.sections.signals")).toBeInTheDocument();
    expect(screen.getByText("directorConsole.sections.outlook")).toBeInTheDocument();
    // The old clinical accordion labels are gone.
    expect(screen.queryByText("directorConsole.sections.diagnosis")).not.toBeInTheDocument();
    expect(screen.queryByText("directorConsole.sections.evidence")).not.toBeInTheDocument();
    expect(screen.queryByText("directorConsole.sections.actionPlan")).not.toBeInTheDocument();
    expect(screen.queryByText("directorConsole.sections.expectedImpact")).not.toBeInTheDocument();
  });

  it("renders the summary as a spoken line, not a serif font-title headline", () => {
    renderMessage();
    const summary = screen.getByText("Tuesday lunch slumped 12% — let's win it back.");
    expect(summary.className).not.toContain("font-title");
    expect(summary.className).not.toContain("heading-md");
    expect(summary.className).toContain("text-body-lg");
  });

  it("keeps the non-structured branch (raw content via DirectorMessage)", () => {
    renderMessage({ structured_response: undefined, content: "Plain answer text." });
    expect(screen.getByText(/Plain answer text\./)).toBeInTheDocument();
  });

  it("replaces ungrounded revenue advice with not-enough-data and an analytics link (#241)", () => {
    const { onActionClick } = renderMessage({
      structured_response: {
        summary: "Dinner revenue is tracking above the 30-day median.",
        diagnosis: "Cocktail attach is up and labor is on target.",
        evidence: [],
        actions: [],
        expected_impact: "Featuring cocktails before 8pm should extend the trend.",
        follow_ups: [],
      },
    });
    expect(screen.getByTestId("dc-revenue-not-enough-data")).toHaveTextContent(
      "directorConsole.honesty.notEnoughData",
    );
    expect(
      screen.queryByText(/Dinner revenue is tracking/i),
    ).not.toBeInTheDocument();
    fireEvent.click(screen.getByTestId("dc-revenue-open-analytics"));
    expect(onActionClick).toHaveBeenCalledWith(
      expect.objectContaining({
        deep_link: "/business/1/dashboard?tab=analytics",
      }),
    );
  });
});

describe("L4-15 — regenerate is offered only on the trailing assistant turn", () => {
  const regenerateLabel = "directorConsole.actions.regenerate";

  it("hides the regenerate affordance on a non-trailing structured message", () => {
    renderMessage({}, { isTrailing: false });
    expect(screen.queryByLabelText(regenerateLabel)).not.toBeInTheDocument();
    // The rest of the action row survives.
    expect(screen.getByLabelText("directorConsole.actions.copy")).toBeInTheDocument();
  });

  it("hides the regenerate affordance on a non-trailing plain-content message", () => {
    renderMessage(
      { structured_response: undefined, content: "Plain answer text." },
      { isTrailing: false },
    );
    expect(screen.queryByLabelText(regenerateLabel)).not.toBeInTheDocument();
  });

  it("defaults to hidden when isTrailing is not provided (safe default)", () => {
    renderMessage();
    expect(screen.queryByLabelText(regenerateLabel)).not.toBeInTheDocument();
  });

  it("shows and wires regenerate on the trailing assistant message", () => {
    const onRegenerate = jest.fn();
    renderMessage({}, { isTrailing: true, onRegenerate });
    fireEvent.click(screen.getByLabelText(regenerateLabel));
    expect(onRegenerate).toHaveBeenCalledTimes(1);
    expect(onRegenerate).toHaveBeenCalledWith(42);
  });
});
