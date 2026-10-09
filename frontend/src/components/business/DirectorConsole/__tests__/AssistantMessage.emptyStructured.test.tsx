/** @jest-environment jsdom */
/**
 * Live-review regression: seeded/legacy assistant messages carry a
 * structured_response whose fields are ALL empty ({"summary":"",...}).
 * The structured path won over `content` and painted a blank transcript —
 * the demo's "Demo Director Briefing" thread rendered as an empty pane
 * with just the teal border bar. An empty structured payload must fall
 * back to rendering the plain message content.
 */
import React from "react";
import { render, screen } from "@testing-library/react";

import AssistantMessage from "../AssistantMessage";
import type { DirectorThreadMessage } from "@/api/directorConsole";

jest.mock("@/i18n/SimpleTranslationProvider", () => ({
  useSimpleLocale: () => ({ locale: "en" }),
  getTranslation: (key: string) => key,
}));

const noop = () => {};

const baseMessage = {
  id: 68,
  thread_id: 21,
  business_id: 39,
  role: "assistant",
  locale: "en",
  content: "Dinner revenue is tracking above the 30-day median.",
  model_name: "demo-seed",
  latency_ms: 120,
  created_at: "2026-07-02T23:01:38Z",
} as unknown as DirectorThreadMessage;

const renderMessage = (structured: unknown) =>
  render(
    <AssistantMessage
      message={{ ...baseMessage, structured_response: structured } as DirectorThreadMessage}
      isFirstAssistant
      onActionClick={noop}
      onFeedback={noop}
      onFollowUpClick={noop}
      onCopy={noop}
      onRegenerate={noop}
    />,
  );

describe("AssistantMessage with empty structured_response", () => {
  it("falls back to plain content when every structured field is empty", () => {
    renderMessage({
      summary: "",
      diagnosis: "",
      evidence: null,
      actions: null,
      expected_impact: "",
      follow_ups: null,
    });
    // #241: the demo seed's qualitative revenue line has no figures — honesty
    // gate replaces it rather than painting an unauditable claim.
    expect(screen.getByTestId("dc-revenue-not-enough-data")).toBeInTheDocument();
    expect(
      screen.queryByText(/Dinner revenue is tracking above the 30-day median/i),
    ).not.toBeInTheDocument();
  });

  it("still renders grounded non-revenue fallback content", () => {
    render(
      <AssistantMessage
        message={
          {
            ...baseMessage,
            content: "Inventory for Premium Beef is at zero.",
            structured_response: {
              summary: "",
              diagnosis: "",
              evidence: null,
              actions: null,
              expected_impact: "",
              follow_ups: null,
            },
          } as unknown as DirectorThreadMessage
        }
        isFirstAssistant
        onActionClick={noop}
        onFeedback={noop}
        onFollowUpClick={noop}
        onCopy={noop}
        onRegenerate={noop}
      />,
    );
    expect(
      screen.getByText(/Inventory for Premium Beef is at zero/i),
    ).toBeInTheDocument();
  });

  it("still prefers the structured summary when it has content", () => {
    renderMessage({
      summary: "Cocktail attach is up.",
      diagnosis: "",
      evidence: null,
      actions: null,
      expected_impact: "",
      follow_ups: null,
    });
    expect(screen.getByText(/Cocktail attach is up/i)).toBeInTheDocument();
    expect(
      screen.queryByText(/Dinner revenue is tracking/i),
    ).not.toBeInTheDocument();
  });
});
