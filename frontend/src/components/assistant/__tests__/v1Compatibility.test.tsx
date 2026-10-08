/** @jest-environment jsdom */

import React from "react";
import { render, renderHook, screen, waitFor } from "@testing-library/react";
import { axiosInstance } from "@/api";
import type { ChatMessage } from "@/components/chat/types";
import { parseOpsThreadMessages } from "@/api/opsAssistant";
import { AssistantMessage } from "@/components/assistant/AssistantMessage";
import type { AssistantResponse } from "@/types/assistant";
import { useAiWaiterTransport } from "@/components/guest/useAiWaiterTransport";
import { useSSEMessages } from "@/hooks/useSSEMessages";

jest.mock("@/api", () => ({
  axiosInstance: { get: jest.fn(), post: jest.fn() },
}));
jest.mock("@/hooks/useSSEMessages", () => ({ useSSEMessages: jest.fn() }));

const mockedAxios = axiosInstance as jest.Mocked<typeof axiosInstance>;
const mockedSSE = useSSEMessages as jest.MockedFunction<typeof useSSEMessages>;

const labels = {
  usedSources: (count: number) => `Sources ${count}`,
  sourcesRegion: (count: number) => `Sources ${count}`,
  sourceOrigin: (origin: string) => origin,
  externalSource: (hostname: string) => hostname,
  actions: "Actions",
  steps: "Steps",
  entities: "Entities",
  entityAvailability: (availability: string) => availability,
  followUps: "Follow ups",
  notices: "Notices",
  noticeKind: (kind: string) => kind,
  status: (status: AssistantResponse["status"]) => status,
  workflowProgress: ({ current, total }: { current: number; total: number }) =>
    `${current}/${total}`,
  disabledActionReason: (reason: string | null) => reason ?? "Unavailable",
  renderError: "Unable to render",
};

function renderStoredAssistant(message: ChatMessage) {
  if (!message.response) throw new Error("stored assistant response missing");
  return render(
    <AssistantMessage response={message.response} labels={labels} />,
  );
}

describe("V1 retirement compatibility", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    mockedAxios.get.mockResolvedValue({ data: [] });
    mockedSSE.mockImplementation(() => undefined);
  });

  it("renders a stored Ops V1-only response through the real Ops history adapter", () => {
    const result = parseOpsThreadMessages({
      messages: [
        {
          id: 22,
          role: "assistant",
          content: "Stored Ops fallback",
          structured_response: {
            answer: "Stored Ops V1 answer",
            steps: ["Open Menu Builder"],
            actions: [],
            follow_ups: [],
          },
        },
      ],
    });
    const stored = result.messages[0];

    expect(stored).toMatchObject({
      id: 22,
      contract_version: "v1",
      response_v2: { response_id: "ops-history-22" },
    });
    renderStoredAssistant({
      role: "assistant",
      content: stored.content,
      response: stored.response_v2,
      contractVersion: stored.contract_version,
    });
    expect(screen.getByText("Stored Ops V1 answer")).toBeVisible();
    expect(screen.getByText("Open Menu Builder")).toBeVisible();
  });

  it("renders a stored Waiter V1-only response through the real transport history adapter", async () => {
    mockedAxios.get.mockResolvedValueOnce({
      data: [
        {
          id: 33,
          role: "assistant",
          content: "Stored Waiter V1 answer",
          created_at: 33,
        },
      ],
    });

    const { result } = renderHook(() =>
      useAiWaiterTransport({
        businessId: 7,
        tableCode: "T1",
        mode: "ordering",
        language: "en",
        isOpen: true,
        sessionToken: "waiter-session-token",
        ensureSession: jest.fn().mockResolvedValue("waiter-session-token"),
        billContext: "",
      }),
    );

    await waitFor(() => expect(result.current.messages).toHaveLength(1));
    const stored = result.current.messages[0];
    expect(stored).toMatchObject({
      id: 33,
      contractVersion: "v1",
      response: { response_id: "waiter-message-33" },
    });
    renderStoredAssistant({
      role: "assistant",
      content: stored.content,
      response: stored.response,
      contractVersion: stored.contractVersion,
    });
    expect(screen.getByText("Stored Waiter V1 answer")).toBeVisible();
  });
});
