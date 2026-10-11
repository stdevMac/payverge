/** @jest-environment jsdom */

import React from "react";
import { fireEvent, render, screen } from "@testing-library/react";
import { AssistantViewport } from "../AssistantViewport";

describe("AssistantViewport", () => {
  it.each([undefined, "", "   "])(
    "omits the completion status when the announcement is %p",
    (completionAnnouncement) => {
      render(
        <AssistantViewport
          conversationLabel="Conversation"
          jumpToLatestLabel="Jump to latest"
          completionAnnouncement={completionAnnouncement}
        >
          <article>Welcome</article>
        </AssistantViewport>,
      );

      expect(screen.getByRole("log")).toHaveAttribute("aria-busy", "false");
      expect(screen.queryByRole("status")).not.toBeInTheDocument();
    },
  );

  it("exposes log semantics without announcing restored completed history", () => {
    const onContentChange = jest.fn();
    const restoredChange = {
      turnId: "restored-turn",
      responseStart: document.createElement("article"),
      responseComplete: true,
    };
    const controller = {
      setScroller: jest.fn(),
      onContentChange,
      jumpToLatest: jest.fn(),
      showJumpToLatest: false,
    };

    const { rerender } = render(
      <AssistantViewport
        conversationLabel="Conversation"
        jumpToLatestLabel="Nuevas respuestas"
        completionAnnouncement="Respuesta lista"
        contentChange={restoredChange}
        controller={controller}
      >
        <article>Answer</article>
      </AssistantViewport>,
    );

    const log = screen.getByRole("log", { name: "Conversation" });
    expect(log).toHaveAttribute("aria-live", "polite");
    expect(log).toHaveAttribute("aria-relevant", "additions text");
    expect(log).toHaveAttribute("aria-busy", "false");
    expect(log).toHaveTextContent("Answer");

    const status = screen.getByRole("status");
    expect(status).not.toBe(log);
    expect(status).toBeEmptyDOMElement();
    expect(onContentChange).not.toHaveBeenCalled();
    expect(controller.jumpToLatest).not.toHaveBeenCalled();

    rerender(
      <AssistantViewport
        conversationLabel="Conversation"
        jumpToLatestLabel="Nuevas respuestas"
        completionAnnouncement="Respuesta lista"
        contentChange={{ ...restoredChange }}
        controller={controller}
      >
        <article>Answer</article>
      </AssistantViewport>,
    );
    expect(onContentChange).not.toHaveBeenCalled();

    const nextChange = {
      turnId: "live-turn",
      responseStart: document.createElement("article"),
      responseComplete: false,
    };
    rerender(
      <AssistantViewport
        conversationLabel="Conversation"
        jumpToLatestLabel="Nuevas respuestas"
        completionAnnouncement="Respuesta lista"
        contentChange={nextChange}
        controller={controller}
      >
        <article>Answer</article>
      </AssistantViewport>,
    );

    expect(onContentChange).toHaveBeenCalledWith(nextChange);
  });

  it("renders the injected localized jump label and delegates the click", () => {
    const jumpToLatest = jest.fn();

    render(
      <AssistantViewport
        conversationLabel="Conversación"
        jumpToLatestLabel="Ir a lo último"
        completionAnnouncement=""
        controller={{
          setScroller: jest.fn(),
          onContentChange: jest.fn(),
          jumpToLatest,
          showJumpToLatest: true,
        }}
      >
        <article>Respuesta</article>
      </AssistantViewport>,
    );

    fireEvent.click(screen.getByRole("button", { name: "Ir a lo último" }));
    expect(jumpToLatest).toHaveBeenCalledTimes(1);
  });

  it("does not render a jump control when the reader is following", () => {
    render(
      <AssistantViewport
        conversationLabel="Conversation"
        jumpToLatestLabel="Jump to latest"
        controller={{
          setScroller: jest.fn(),
          onContentChange: jest.fn(),
          jumpToLatest: jest.fn(),
          showJumpToLatest: false,
        }}
      >
        <article>Answer</article>
      </AssistantViewport>,
    );

    expect(screen.queryByRole("button")).not.toBeInTheDocument();
  });

  it("gates streaming announcements and forwards later content changes", () => {
    const onContentChange = jest.fn();
    const responseStart = document.createElement("article");
    const streamingChange = {
      turnId: "turn-8",
      responseStart,
      responseComplete: false,
    };
    const controller = {
      setScroller: jest.fn(),
      onContentChange,
      jumpToLatest: jest.fn(),
      showJumpToLatest: false,
    };

    const { rerender } = render(
      <AssistantViewport
        conversationLabel="Conversation"
        jumpToLatestLabel="Jump to latest"
        completionAnnouncement="Answer ready"
        contentChange={streamingChange}
        controller={controller}
      >
        <article>Answer</article>
      </AssistantViewport>,
    );

    expect(onContentChange).toHaveBeenCalledWith(streamingChange);
    expect(screen.getByRole("log")).toHaveAttribute("aria-busy", "true");
    expect(screen.getByRole("status")).toBeEmptyDOMElement();

    const completedChange = {
      ...streamingChange,
      responseComplete: true,
    };
    rerender(
      <AssistantViewport
        conversationLabel="Conversation"
        jumpToLatestLabel="Jump to latest"
        completionAnnouncement="Answer ready"
        contentChange={completedChange}
        controller={controller}
      >
        <article>Answer</article>
      </AssistantViewport>,
    );

    expect(onContentChange).toHaveBeenLastCalledWith(completedChange);
    expect(screen.getByRole("log")).toHaveAttribute("aria-busy", "false");
    const status = screen.getByRole("status");
    expect(status).toHaveAttribute("aria-live", "polite");
    expect(status).toHaveAttribute("aria-atomic", "true");
    expect(status).toHaveTextContent("Answer ready");
  });

  it("bottoms the first late restored hydration once without announcing it", () => {
    const onContentChange = jest.fn();
    const controller = {
      setScroller: jest.fn(),
      onContentChange,
      jumpToLatest: jest.fn(),
      showJumpToLatest: false,
    };
    const { rerender } = render(
      <AssistantViewport
        conversationLabel="Conversation"
        jumpToLatestLabel="Jump to latest"
        completionAnnouncement="Answer ready"
        controller={controller}
      >
        <article>Answer</article>
      </AssistantViewport>,
    );
    const restoredChange = {
      turnId: "late-restored-turn",
      responseStart: document.createElement("article"),
      responseComplete: true,
    };

    rerender(
      <AssistantViewport
        conversationLabel="Conversation"
        jumpToLatestLabel="Jump to latest"
        completionAnnouncement="Answer ready"
        contentChange={restoredChange}
        contentChangeOrigin="restored"
        controller={controller}
      >
        <article>Restored answer</article>
      </AssistantViewport>,
    );

    expect(onContentChange).not.toHaveBeenCalled();
    expect(controller.jumpToLatest).toHaveBeenCalledTimes(1);
    expect(screen.getByRole("status")).toBeEmptyDOMElement();

    rerender(
      <AssistantViewport
        conversationLabel="Conversation"
        jumpToLatestLabel="Jump to latest"
        completionAnnouncement="Answer ready"
        contentChange={{ ...restoredChange, turnId: "later-restored-turn" }}
        contentChangeOrigin="restored"
        controller={controller}
      >
        <article>Later restored answer</article>
      </AssistantViewport>,
    );

    expect(onContentChange).not.toHaveBeenCalled();
    expect(controller.jumpToLatest).toHaveBeenCalledTimes(1);
    expect(screen.getByRole("status")).toBeEmptyDOMElement();

    rerender(
      <AssistantViewport
        conversationLabel="Conversation"
        jumpToLatestLabel="Jump to latest"
        completionAnnouncement="Answer ready"
        contentChange={{ ...restoredChange, turnId: "live-turn" }}
        contentChangeOrigin="live"
        controller={controller}
      >
        <article>Live answer</article>
      </AssistantViewport>,
    );

    expect(onContentChange).toHaveBeenCalledWith(
      expect.objectContaining({ turnId: "live-turn" }),
    );
    expect(screen.getByRole("status")).toHaveTextContent("Answer ready");
  });

  it("re-arms late restored hydration after the conversation is cleared", () => {
    const controller = {
      setScroller: jest.fn(),
      onContentChange: jest.fn(),
      jumpToLatest: jest.fn(),
      showJumpToLatest: false,
    };
    const restoredChange = {
      turnId: "first-scope",
      responseStart: document.createElement("article"),
      responseComplete: true,
    };
    const { rerender } = render(
      <AssistantViewport
        conversationLabel="Conversation"
        jumpToLatestLabel="Jump to latest"
        completionAnnouncement="Answer ready"
        contentChange={restoredChange}
        contentChangeOrigin="restored"
        controller={controller}
      >
        <article>First scope</article>
      </AssistantViewport>,
    );

    expect(controller.jumpToLatest).not.toHaveBeenCalled();
    rerender(
      <AssistantViewport
        conversationLabel="Conversation"
        jumpToLatestLabel="Jump to latest"
        completionAnnouncement="Answer ready"
        controller={controller}
      >
        <article>Empty next scope</article>
      </AssistantViewport>,
    );
    rerender(
      <AssistantViewport
        conversationLabel="Conversation"
        jumpToLatestLabel="Jump to latest"
        completionAnnouncement="Answer ready"
        contentChange={{ ...restoredChange, turnId: "second-scope" }}
        contentChangeOrigin="restored"
        controller={controller}
      >
        <article>Second scope</article>
      </AssistantViewport>,
    );

    expect(controller.jumpToLatest).toHaveBeenCalledTimes(1);
    expect(controller.onContentChange).not.toHaveBeenCalled();
    expect(screen.getByRole("status")).toBeEmptyDOMElement();
  });
});
