"use client";

import React, { useCallback, useEffect, useRef, useState } from "react";
import { Button, Input, Spinner } from "@nextui-org/react";
import { MessageSquare, Send } from "lucide-react";
import {
  createAiWaiterTestSession,
  parseAiWaiterChatResponse,
  sendAiWaiterTestChat,
} from "@/api/aiWaiter";
import { btnPrimaryNextUI, btnSecondaryNextUI } from "@/components/ui/buttonStyles";

type ChatTurn = { role: "user" | "assistant"; content: string };

interface Props {
  businessId: number;
  aiEnabled: boolean;
  language: string;
  t: (key: string, params?: Record<string, string | number>) => string;
}

/**
 * Operator sandbox: probes allergen / availability answers against the live
 * waiter path via authenticated /ai/test-chat endpoints. Never calls guest
 * session APIs, never sets pv_ai_waiter_session, and never appears in Live
 * Monitor or guest quota.
 */
export default function AiWaiterTestChat({
  businessId,
  aiEnabled,
  language,
  t,
}: Props) {
  const [turns, setTurns] = useState<ChatTurn[]>([]);
  const [draft, setDraft] = useState("");
  const sessionTokenRef = useRef<string | null>(null);
  const inputRef = useRef<HTMLInputElement>(null);
  const draftRef = useRef(draft);
  draftRef.current = draft;
  const pendingSendRef = useRef("");
  const sendingRef = useRef(false);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [composerKey, setComposerKey] = useState(0);

  const composerInput = () => {
    const host =
      (inputRef.current as unknown as HTMLElement | null) ??
      (typeof document === "undefined"
        ? null
        : document.querySelector<HTMLElement>(
            '[data-testid="ai-waiter-test-chat-input"]',
          ));
    const native =
      host instanceof HTMLInputElement
        ? host
        : host?.querySelector?.("input");
    return ((native && "value" in native ? native.value : "") || draftRef.current).trim();
  };

  const captureComposer = () => {
    pendingSendRef.current = composerInput();
  };

  useEffect(() => {
    sessionTokenRef.current = null;
    setTurns([]);
  }, [businessId, language]);

  const ensureSession = useCallback(async (): Promise<string> => {
    if (sessionTokenRef.current) return sessionTokenRef.current;
    const session = await createAiWaiterTestSession(businessId, {
      language: language || "en",
    });
    sessionTokenRef.current = session.session_token;
    const greeting = session.greeting?.trim() ?? "";
    if (greeting) {
      setTurns((prev) => {
        if (prev.some((turn) => turn.role === "assistant" && turn.content === greeting)) {
          return prev;
        }
        return [{ role: "assistant", content: greeting }, ...prev];
      });
    }
    return session.session_token;
  }, [businessId, language]);

  const send = useCallback(
    async (text: string) => {
      const trimmed = text.trim();
      if (!trimmed || sendingRef.current || !aiEnabled) return;
      sendingRef.current = true;
      setBusy(true);
      setError(null);
      const nextTurns: ChatTurn[] = [
        ...turns,
        { role: "user", content: trimmed },
      ];
      setTurns(nextTurns);
      setDraft("");
      setComposerKey((key) => key + 1);
      pendingSendRef.current = "";
      try {
        const token = await ensureSession();
        const history = nextTurns.map((turn) => ({
          role: turn.role === "assistant" ? "assistant" : "user",
          content: turn.content,
        }));
        const raw = await sendAiWaiterTestChat(businessId, {
          history,
          language: language || "en",
          session_token: token,
        });
        const parsed = parseAiWaiterChatResponse(raw);
        const answer = parsed.response_v2.answer.content.trim();
        setTurns((prev) => [
          ...prev,
          {
            role: "assistant",
            content: answer || t("settings.testChat.emptyReply"),
          },
        ]);
      } catch {
        setError(t("settings.testChat.error"));
        setTurns((prev) => prev.slice(0, -1));
        setDraft(trimmed);
        pendingSendRef.current = trimmed;
        setComposerKey((key) => key + 1);
      } finally {
        sendingRef.current = false;
        setBusy(false);
      }
    },
    [aiEnabled, businessId, ensureSession, language, t, turns],
  );

  const probes = [
    {
      key: "allergen",
      label: t("settings.testChat.probeAllergen"),
      text: t("settings.testChat.probeAllergenText"),
    },
    {
      key: "recommend",
      label: t("settings.testChat.probeRecommend"),
      text: t("settings.testChat.probeRecommendText"),
    },
  ];

  return (
    <div
      className="rounded-lg border border-warm-100 bg-warm-50/60 p-4"
      data-testid="ai-waiter-test-chat"
    >
      <div className="mb-2 flex items-start gap-2">
        <MessageSquare className="mt-0.5 h-4 w-4 shrink-0 text-brand" />
        <div>
          <h4 className="text-sm font-semibold text-ink-900">
            {t("settings.testChat.title")}
          </h4>
          <p className="text-xs text-ink-500">
            {t("settings.testChat.subtitle")}
          </p>
        </div>
      </div>

      <div className="mb-3 flex flex-wrap gap-2">
        {probes.map((probe) => (
          <Button
            key={probe.key}
            size="sm"
            radius="full"
            className={btnSecondaryNextUI}
            isDisabled={!aiEnabled || busy}
            onPress={() => send(probe.text)}
          >
            {probe.label}
          </Button>
        ))}
      </div>

      <div
        className="mb-3 max-h-56 space-y-2 overflow-y-auto rounded-md border border-warm-100 bg-white p-3"
        aria-live="polite"
      >
        {turns.length === 0 ? (
          <p className="text-sm text-ink-400">{t("settings.testChat.empty")}</p>
        ) : (
          turns.map((turn, index) => (
            <div
              key={`${turn.role}-${index}`}
              className={
                turn.role === "user"
                  ? "text-sm text-ink-800"
                  : "text-sm text-ink-600"
              }
            >
              <span className="font-semibold text-ink-500">
                {turn.role === "user"
                  ? t("settings.testChat.you")
                  : t("settings.testChat.assistant")}
                :{" "}
              </span>
              {turn.content}
            </div>
          ))
        )}
        {busy ? (
          <div className="flex items-center gap-2 text-xs text-ink-400">
            <Spinner size="sm" />
            {t("settings.testChat.thinking")}
          </div>
        ) : null}
      </div>

      {error ? (
        <p className="mb-2 text-xs text-rose-700" role="alert">
          {error}
        </p>
      ) : null}

      <form
        className="flex gap-2"
        onPointerDownCapture={captureComposer}
        onMouseDownCapture={captureComposer}
        onSubmit={(event) => {
          event.preventDefault();
          captureComposer();
          void send(pendingSendRef.current || composerInput());
        }}
      >
        <Input
          key={`${businessId}-${language}-${composerKey}`}
          ref={inputRef}
          aria-label={t("settings.testChat.inputLabel")}
          placeholder={t("settings.testChat.placeholder")}
          defaultValue={draft}
          onValueChange={setDraft}
          onChange={(event) => setDraft(event.target.value)}
          isDisabled={!aiEnabled || busy}
          data-testid="ai-waiter-test-chat-input"
          onKeyDown={(event) => {
            if (event.key === "Enter" && !event.shiftKey) {
              event.preventDefault();
              captureComposer();
              void send(pendingSendRef.current || composerInput());
            }
          }}
        />
        <Button
          type="submit"
          isIconOnly
          radius="full"
          className={btnPrimaryNextUI}
          isDisabled={!aiEnabled || busy}
          onPress={() => {
            captureComposer();
            void send(pendingSendRef.current || composerInput());
          }}
          aria-label={t("settings.testChat.send")}
        >
          <Send className="h-4 w-4" />
        </Button>
      </form>
    </div>
  );
}
