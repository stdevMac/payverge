"use client";

import React, {
  useState,
  useEffect,
  useRef,
  useCallback,
  useMemo,
  useLayoutEffect,
} from "react";
import {
  Button,
  Modal,
  ModalContent,
  ModalBody,
} from "@nextui-org/react";
import { toast } from "react-hot-toast";
import { X, Sparkles, Bot, RotateCcw, ShoppingCart } from "lucide-react";
import { motion, AnimatePresence, useReducedMotion } from "framer-motion";
import { ImageCarousel } from "./ImageCarousel";
import { classifyChatError } from "@/api/aiWaiter";
import { getStarterQuestions } from "./aiWaiterCopy";
import { AI_WAITER_PANEL_MODAL } from "./aiWaiterPanel";
import { resolveCartAction, resolveLegacyCartAction } from "./aiWaiterActions";
import {
  dinerEntityMenuItem,
  forDinerTranscript,
  type DinerOrderabilityMap,
} from "./aiWaiterDinerTranscript";
import { isMenuItemOrderable } from "./menuItemAvailability";
import { formatGuestCurrency } from "@/utils/guestCurrencyFormatter";
import { useAiWaiterSession } from "./useAiWaiterSession";
import { useAiWaiterTransport, type AiWaiterSendOutcome } from "./useAiWaiterTransport";
import {
  adaptLegacyResponse,
  type AssistantEntity,
  type AssistantResponse,
} from "@/types/assistant";
import enGuest from "@/i18n/guest-messages/en.json";
import ProactiveNudge from "@/components/chat/ProactiveNudge";
import {
  AssistantMessage,
  type AssistantEntityInteraction,
  type AssistantMessageLabels,
} from "@/components/assistant/AssistantMessage";
import { AssistantViewport } from "@/components/assistant/AssistantViewport";
import { AssistantComposer } from "@/components/assistant/AssistantComposer";
import {
  assistantEventProperties,
  type AssistantAnalyticsContext,
  trackAssistantEvent,
} from "@/components/assistant/assistantAnalytics";

const appendAssistantCopy = (base: string, extra: string): string => {
  const left = base.trim();
  const right = extra.trim();
  if (!left) return right;
  if (!right) return left;
  return `${left}\n\n${right}`;
};

// Local greetings removed in favor of backend-driven persistent greetings

export type AiWaiterCartActionOutcome = "applied" | "deferred" | "rejected";

interface AiWaiterProps {
    businessId: number;
    businessName: string;
    aiName?: string;
    menuData: any[];
  bundles?: Array<{
    id?: number;
    name: string;
    price: number;
    currency?: string;
    is_active?: boolean;
  }>;
    language?: string;
    onAddToCart: (
        itemName: string,
        price: number,
        quantity: number,
        notes?: string,
        metadata?: {
            itemType?: "menu_item" | "bundle";
            menuItemId?: string;
            bundleId?: number;
        },
  ) => AiWaiterCartActionOutcome | void;
    hasActiveBill?: boolean;
    billItems?: Array<{ name: string; price: number; quantity: number }>;
    mode?: "ordering" | "concierge"; // Default to "ordering"
    tableCode?: string;
    isOrderingEnabled?: boolean;
    /**
     * Live `item_orderability` projection. Needed so closed-hours items
     * (`business_closed`) still paint dish cards — the catalog row is often
     * projected `is_available: false` even though the diner can browse.
     */
    itemOrderability?: DinerOrderabilityMap;
    currency?: string;
    /**
     * True when the guest cart pill is currently on-screen. The cart pill lives
     * in the same bottom band as this floating button; when it is present we
     * lift the button above it so the two controls cannot collide in the
     * bottom-right thumb zone on notched phones (L7).
     */
    cartVisible?: boolean;
    /**
     * `ai_waiter_mode` from the guest business projection. "basic" means no
     * model is configured and the helper answers with set replies built from
     * the menu snapshot, so the UI must not label it as AI.
     */
    waiterMode?: "llm" | "basic" | string;
}

export const AiWaiter: React.FC<AiWaiterProps> = ({
    businessId,
    businessName,
    aiName = "Sage",
    menuData,
    bundles = [],
    language = "en",
    onAddToCart,
    hasActiveBill = false,
    billItems = [],
    mode = "ordering",
    tableCode = "",
    isOrderingEnabled = true,
    itemOrderability = {},
    currency = "USD",
    cartVisible = false,
    waiterMode,
}) => {
    const prefersReducedMotion = useReducedMotion();
    const [isOpen, setIsOpen] = useState(false);
    const assistantAnalytics = useMemo<AssistantAnalyticsContext>(
      () => ({ surface: "waiter", locale: language }),
      [language],
    );
    const openRef = useRef(false);
    const closeAssistant = useCallback(() => {
      if (!openRef.current) return;
      openRef.current = false;
      setIsOpen(false);
      trackAssistantEvent(
        "assistant_closed",
        assistantEventProperties(assistantAnalytics),
      );
    }, [assistantAnalytics]);

    // PG-15.3: close panel when hash-tab navigation changes (business page
    // mounts AiWaiter outside the tab switch, so it would otherwise persist).
    // Also listen to popstate for back/forward; force close so NextUI cannot
    // leave a hit-testable exit frame over the storefront.
    useEffect(() => {
        window.addEventListener("hashchange", closeAssistant);
        window.addEventListener("popstate", closeAssistant);
        return () => {
            window.removeEventListener("hashchange", closeAssistant);
            window.removeEventListener("popstate", closeAssistant);
        };
    }, [closeAssistant]);
    const [nudgeVisible, setNudgeVisible] = useState(false);
    /** Once the guest has opened the chat, never nudge them this page-load. */
    const openedEverRef = useRef(false);
    const [inputValue, setInputValue] = useState("");
    const [isRecreatingSession, setIsRecreatingSession] = useState(false);
    const [isMobile, setIsMobile] = useState(false);

  const dispatchCartAction = useCallback(
    (
      itemName: string,
      price: number,
      quantity: number,
      notes: string,
      metadata: {
        itemType: "menu_item" | "bundle";
        menuItemId?: string;
        bundleId?: number;
      },
    ): { outcome: AiWaiterCartActionOutcome; surfacedByHost: boolean } => {
      try {
        const outcome = onAddToCart(itemName, price, quantity, notes, metadata);
        if (outcome === "applied" || outcome === "deferred") {
          return { outcome, surfacedByHost: false };
        }
        // An explicit "rejected" means the host cart handler refused and
        // already surfaced its own reason-specific feedback (the menu page
        // toasts before returning false). A void/unknown return or a throw
        // surfaced nothing — the caller still owes the guest feedback.
        return { outcome: "rejected", surfacedByHost: outcome === "rejected" };
      } catch {
        return { outcome: "rejected", surfacedByHost: false };
      }
    },
    [onAddToCart],
  );

    const requestInFlightRef = useRef(false);
    const recoveringSessionRef = useRef(false);
    const componentMountedRef = useRef(false);

    useLayoutEffect(() => {
      componentMountedRef.current = true;
      return () => {
        componentMountedRef.current = false;
      };
    }, []);

    useEffect(() => {
        openRef.current = isOpen;
        if (isOpen) openedEverRef.current = true;
    }, [isOpen]);

    // Quiet discovery nudge: many guests never notice the AI waiter. Tease it
    // once per tab session per business, well after page load, and auto-hide so
    // it never lingers over the cramped mobile bottom band. Opening the chat or
    // dismissing it wins permanently for the session. Hide while the cart pill
    // is up so the coachmark cannot cover Add / Place Order (#84).
    useEffect(() => {
        if (isOpen || cartVisible) {
            setNudgeVisible(false);
            return;
        }
        if (openedEverRef.current) return;
        try {
      if (sessionStorage.getItem(`pv_ai_waiter_nudge_seen_${businessId}`))
        return;
        } catch {
            return;
        }
        let hideId: number | undefined;
        const showId = window.setTimeout(() => {
            if (openRef.current || openedEverRef.current) return;
            setNudgeVisible(true);
            try {
                sessionStorage.setItem(`pv_ai_waiter_nudge_seen_${businessId}`, "1");
            } catch {
                // ignore
            }
            hideId = window.setTimeout(() => setNudgeVisible(false), 12000);
        }, 8000);
        return () => {
            window.clearTimeout(showId);
            if (hideId) window.clearTimeout(hideId);
        };
    }, [isOpen, businessId, cartVisible]);

    const { sessionToken, ensureSession, recreateSession } = useAiWaiterSession({
        businessId,
        tableCode,
        mode,
        language,
    });

    const [activeBundle, setActiveBundle] = useState<Record<string, any>>({});
    useEffect(() => {
        let cancelled = false;
        // Guard against an empty language: a blank interpolation resolves to
        // `./.json` and throws. Default to the English bundle.
        import(`@/i18n/guest-messages/${language || "en"}.json`)
      .then((m) => {
        if (!cancelled) setActiveBundle(m.default);
      })
      .catch(() => {
        if (!cancelled) setActiveBundle({});
      });
    return () => {
      cancelled = true;
    };
    }, [language]);

    const [starterQuestions, setStarterQuestions] = useState<string[]>([]);
    useEffect(() => {
        let cancelled = false;
    getStarterQuestions(language).then((qs) => {
      if (!cancelled) setStarterQuestions(qs);
    });
    return () => {
      cancelled = true;
    };
    }, [language]);

  const tUi = useCallback(
    (key: string, vars?: Record<string, string | number>): string => {
        const fromActive = activeBundle?.aiWaiter?.[key];
        const fromEn = (enGuest as any)?.aiWaiter?.[key];
      const tpl: string =
        typeof fromActive === "string"
          ? fromActive
          : typeof fromEn === "string"
            ? fromEn
            : key;
        if (!vars) return tpl;
        return Object.entries(vars).reduce(
            (acc, [k, v]) => acc.replace(new RegExp(`\\{${k}\\}`, "g"), String(v)),
            tpl,
        );
    },
    [activeBundle],
  );

  // Identity copy: in basic mode (no model) the helper is not an AI, so the
  // header, disclosure and accessible names use the "Scripted" variants.
  const scripted = waiterMode === "basic";
  const tId = useCallback(
    (key: string, vars?: Record<string, string | number>): string =>
      tUi(scripted ? `${key}Scripted` : key, vars),
    [scripted, tUi],
  );

  const billContext = useMemo(
    () =>
      billItems
        .map(
          (item) =>
            `- ${item.quantity}x ${item.name} (${currency} ${item.price})`,
        )
        .join("\n"),
    [billItems, currency],
  );

  const projectAssistantResponse = useCallback(
    (
      response: AssistantResponse,
      context: Parameters<
        NonNullable<
          Parameters<typeof useAiWaiterTransport>[0]["projectAssistantResponse"]
        >
      >[1],
    ): AssistantResponse => {
      let content = response.answer.content;

      const applyResolvedCart = (
        resolved:
          | ReturnType<typeof resolveCartAction>
          | ReturnType<typeof resolveLegacyCartAction>,
      ) => {
        if (mode === "concierge") {
          content = appendAssistantCopy(content, tUi("conciergeOrderingOnly"));
          return;
        }
        if (!isOrderingEnabled) {
          content = appendAssistantCopy(content, tUi("orderingUnavailable"));
          return;
        }
        if (!resolved.ok) {
          content = appendAssistantCopy(content, tUi("itemUnclear"));
          return;
        }

        const request = resolved.request;
        const { outcome } = dispatchCartAction(
          request.itemName,
          request.price,
          request.quantity,
          request.notes ?? "",
          request.itemType === "bundle"
            ? { itemType: "bundle", bundleId: request.bundleId }
            : { itemType: "menu_item", menuItemId: request.menuItemId },
        );
        if (outcome === "applied") {
          trackAssistantEvent(
            "assistant_action_completed",
            assistantEventProperties(
              {
                ...assistantAnalytics,
                contract_version: context.source === "v2" ? "v2" : "v1",
              },
              { action_kind: "add_cart_item" },
            ),
          );
          toast.success(
            tUi("addedToast", {
              qty: request.quantity,
              name: request.itemName,
            }),
            {
              icon: (
                <ShoppingCart
                  className="w-4 h-4 text-brand"
                  aria-hidden="true"
                />
              ),
              className:
                "rounded-xl bg-warm-50 text-ink-900 border border-warm-200",
            },
          );
          content = appendAssistantCopy(
            content,
            tUi("addedChat", {
              qty: request.quantity,
              name: request.itemName,
            }),
          );
          return;
        }
        if (outcome === "rejected") {
          trackAssistantEvent(
            "assistant_action_failed",
            assistantEventProperties(
              {
                ...assistantAnalytics,
                contract_version: context.source === "v2" ? "v2" : "v1",
              },
              { action_kind: "add_cart_item" },
            ),
          );
        }
        content = appendAssistantCopy(
          content,
          tUi(outcome === "deferred" ? "continueOnMenu" : "errUnderstand"),
        );
      };

      if (context.source === "v2") {
        if (response.actions.some((action) => action.type === "add_cart_item")) {
          applyResolvedCart(
            resolveCartAction(
              response,
              menuData,
              bundles as unknown as Parameters<typeof resolveCartAction>[2],
            ),
          );
        }
      } else {
        const legacyCalls = context.legacyParts.flatMap((part) => {
          const rawCall =
            part.function_call ?? part.functionCall ?? part.FunctionCall;
          if (
            typeof rawCall !== "object" ||
            rawCall === null ||
            Array.isArray(rawCall)
          ) {
            return [];
          }
          const call = rawCall as Record<string, unknown>;
          if (
            call.name !== "add_to_cart" ||
            typeof call.args !== "object" ||
            call.args === null ||
            Array.isArray(call.args)
          ) {
            return [];
          }
          return [
            {
              args: call.args as Parameters<typeof resolveLegacyCartAction>[0],
            },
          ];
        });
        if (legacyCalls.length === 1) {
          applyResolvedCart(
            resolveLegacyCartAction(
              legacyCalls[0].args,
              menuData,
              bundles as unknown as Parameters<
                typeof resolveLegacyCartAction
              >[2],
            ),
          );
        } else if (legacyCalls.length > 1) {
          content = appendAssistantCopy(content, tUi("itemUnclear"));
        }
      }

      if (!content.trim()) {
        content = tUi("emptyResponseRetry");
      }
      return {
        ...response,
        answer: { ...response.answer, content: content.trim() },
      };
    },
    [
      bundles,
      assistantAnalytics,
      dispatchCartAction,
      isOrderingEnabled,
      menuData,
      mode,
      tUi,
    ],
  );

  const handleHistoryContractMismatch = useCallback(() => {
    trackAssistantEvent(
      "assistant_history_contract_mismatch",
      assistantEventProperties({
        ...assistantAnalytics,
        contract_version: "v2",
      }),
    );
  }, [assistantAnalytics]);

  const handleRenderFallback = useCallback(() => {
    trackAssistantEvent(
      "assistant_render_fallback",
      assistantEventProperties({
        ...assistantAnalytics,
        contract_version: "v2",
      }),
    );
  }, [assistantAnalytics]);

  const {
    messages,
    resumedFromStorage,
    isLoading: transportIsLoading,
    isPolling,
    lastFailedInput,
    sendMessage,
    retryFailedMessage,
    appendAssistantText,
    resetConversation,
  } = useAiWaiterTransport({
    businessId,
    tableCode,
    mode,
    language,
    isOpen,
    sessionToken,
    ensureSession,
    billContext,
    projectAssistantResponse,
    onHistoryContractMismatch: handleHistoryContractMismatch,
    onRenderFallback: handleRenderFallback,
  });
  const isLoading = transportIsLoading || isRecreatingSession;

  // A refresh replays the transcript from this browser, but the bearer token
  // was memory-only so the server minted a NEW conversation, and the model is
  // fed only that conversation's rows. Say so, once, right under the replayed
  // turns — the diner is otherwise reading a thread Sage has no memory of
  // (#724). The notice is transient, so it is not stored and cannot stack.
  const resumedNoticeShownRef = useRef(false);
  useEffect(() => {
    if (!resumedFromStorage) {
      resumedNoticeShownRef.current = false;
      return;
    }
    if (!isOpen || resumedNoticeShownRef.current) return;
    resumedNoticeShownRef.current = true;
    appendAssistantText(tUi("resumedTranscriptNotice", { name: aiName }));
  }, [aiName, appendAssistantText, isOpen, resumedFromStorage, tUi]);

  const handleTransportOutcome = useCallback(
    async (outcome: AiWaiterSendOutcome) => {
      if (outcome.type === "human_ack") {
        toast(outcome.text || tUi("humanAssisting"), {
          icon: "🧑‍🍳",
          id: "ai-waiter-human-ack",
          className:
            "rounded-xl bg-warm-50 text-ink-900 border border-warm-200",
        });
        return;
      }
      if (outcome.type !== "error") return;
      if (outcome.error === "session_unknown") {
        if (recoveringSessionRef.current) return;
        recoveringSessionRef.current = true;
        resetConversation();
        setIsRecreatingSession(true);
        if (componentMountedRef.current) {
          appendAssistantText(tUi("newConversationNotice"));
        }
        try {
          await recreateSession();
        } catch (error) {
          if (componentMountedRef.current) {
            appendAssistantText(
              tUi(
                classifyChatError(error) === "rate_limited"
                  ? "rateLimited"
                  : "errConnect",
              ),
              "degraded",
            );
          }
        } finally {
          recoveringSessionRef.current = false;
          if (componentMountedRef.current) setIsRecreatingSession(false);
        }
        return;
      }
      const key =
        outcome.error === "rate_limited"
          ? "rateLimited"
          : outcome.error === "too_long"
            ? "tooLong"
            : outcome.error === "over_budget"
              ? "overBudget"
              : "errConnect";
      appendAssistantText(tUi(key), "degraded");
    },
    [appendAssistantText, recreateSession, resetConversation, tUi],
  );

    const assistantLabels = useMemo<AssistantMessageLabels>(
      () => ({
        usedSources: (count) =>
          tUi(count === 1 ? "sourceUsed" : "sourcesUsed", { count }),
        sourcesRegion: (count) =>
          tUi(count === 1 ? "sourceDetail" : "sourceDetails", { count }),
        sourceOrigin: () => tUi("sourceVerified"),
        externalSource: (hostname) => tUi("externalSource", { hostname }),
        actions: tUi("actions"),
        steps: tUi("steps"),
        entities: tUi("relatedItems"),
        entityAvailability: (availability) => {
          const state =
            availability === "available"
              ? tUi("availabilityAvailable")
              : availability === "unavailable"
                ? tUi("availabilityUnavailable")
                : tUi("availabilityUnknown");
          return tUi("availability", { availability: state });
        },
        followUps: tUi("followUps"),
        notices: tUi("notices"),
        noticeKind: () => tUi("notice"),
        status: (status) =>
          tUi(
            status === "needs_clarification"
              ? "statusNeedsClarification"
              : status === "blocked"
                ? "statusBlocked"
                : status === "degraded"
                  ? "statusDegraded"
                  : "statusComplete",
          ),
        workflowProgress: ({ current, total }) =>
          tUi("workflowProgress", { current, total }),
        disabledActionReason: () => tUi("actionUnavailable"),
        renderError: tUi("renderError"),
      }),
      [tUi],
    );

    // Every assistant turn is reshaped for the diner, and its dish photos are
    // resolved here rather than only for the newest turn, so scrolling back
    // still shows the pictures instead of bare names.
    const { renderedMessages, mediaByEntity } = useMemo(() => {
      const mediaByEntity = new Map<AssistantEntity, string[]>();
      const renderedMessages = messages.map((message, index) => {
        let response: AssistantResponse | null = null;
        if (message.role === "assistant") {
          const diner = forDinerTranscript(
            message.response ??
              adaptLegacyResponse(
                `waiter-display-${message.id ?? message.createdAt ?? index}`,
                {
                  answer: message.content,
                  steps: [],
                  actions: [],
                  follow_ups: [],
                },
              ),
            menuData,
            itemOrderability,
          );
          response = diner.response;
          for (const [entity, images] of diner.media) {
            mediaByEntity.set(entity, images);
          }
        }
        return {
          message,
          response,
          stableID:
            response?.response_id ??
            message.clientNonce ??
            `${message.role}-${message.id ?? message.createdAt ?? index}`,
        };
      });
      return { renderedMessages, mediaByEntity };
    }, [itemOrderability, menuData, messages]);
    const restoredResponseIDsForOpenRef = useRef(new Set<string>());
    const openAssistant = useCallback(() => {
      if (openRef.current) return;
      restoredResponseIDsForOpenRef.current = new Set(
        messages.flatMap((message) =>
          message.response ? [message.response.response_id] : [],
        ),
      );
      openRef.current = true;
      setIsOpen(true);
      trackAssistantEvent(
        "assistant_opened",
        assistantEventProperties(assistantAnalytics),
      );
    }, [assistantAnalytics, messages]);

    let latestUserIndex = -1;
    for (let index = messages.length - 1; index >= 0; index -= 1) {
      if (messages[index].role === "user") {
        latestUserIndex = index;
        break;
      }
    }
    let activeAssistantIndex = -1;
    for (
      let index = Math.max(0, latestUserIndex + 1);
      index < renderedMessages.length;
      index += 1
    ) {
      if (renderedMessages[index].response) activeAssistantIndex = index;
    }
    if (latestUserIndex < 0) {
      for (let index = renderedMessages.length - 1; index >= 0; index -= 1) {
        if (renderedMessages[index].response) {
          activeAssistantIndex = index;
          break;
        }
      }
    }

    const renderEntityMedia = useCallback((entity: AssistantEntity) => {
      const images = mediaByEntity.get(entity) ?? [];
      return images.length > 0 ? (
        <ImageCarousel
          className="mb-3"
          images={images}
          itemName={entity.display_name}
          size="lg"
        />
      ) : null;
    }, [mediaByEntity]);

    // #791: a suggested dish is a decision the guest is about to make, so its
    // card carries the menu price and — while ordering is live — a tap target
    // that drops one into the cart. Prices come off the live catalog row,
    // already whole-dollar floats (money wire contract): format, never divide.
    // Browsable-but-paused dishes (closed hours, kitchen off, 86 via the live
    // map) keep the price and lose the tap.
    const entityInteraction = useCallback(
      (entity: AssistantEntity): AssistantEntityInteraction | null => {
        const match = dinerEntityMenuItem(entity, menuData);
        if (!match) return null;
        const { item, rawID } = match;
        const price = typeof item.price === "number" ? item.price : null;
        const interaction: AssistantEntityInteraction = {};
        if (price !== null) {
          interaction.priceLabel = formatGuestCurrency(
            price,
            currency,
            language,
          );
        }
        const itemName =
          typeof item.name === "string" ? item.name : entity.display_name;
        const canOrder =
          price !== null &&
          mode === "ordering" &&
          isOrderingEnabled &&
          itemOrderability[rawID]?.orderable !== false &&
          isMenuItemOrderable(item);
        if (canOrder) {
          interaction.selectLabel = tUi("addItemAria", { name: itemName });
          interaction.onSelect = () => {
            const { outcome, surfacedByHost } = dispatchCartAction(
              itemName,
              price,
              1,
              "",
              { itemType: "menu_item", menuItemId: rawID },
            );
            if (outcome === "applied") {
              trackAssistantEvent(
                "assistant_action_completed",
                assistantEventProperties(assistantAnalytics, {
                  action_kind: "add_cart_item",
                }),
              );
              toast.success(tUi("addedToast", { qty: 1, name: itemName }), {
                icon: (
                  <ShoppingCart
                    className="w-4 h-4 text-brand"
                    aria-hidden="true"
                  />
                ),
                className:
                  "rounded-xl bg-warm-50 text-ink-900 border border-warm-200",
              });
            } else if (outcome === "deferred") {
              toast(tUi("continueOnMenu"));
            } else if (!surfacedByHost) {
              // Only toast when the host cart handler surfaced nothing —
              // otherwise its reason-specific toast already told the guest
              // why, and a generic overlay would contradict it.
              // Chat copy carries a leading paragraph break — trim for toast.
              toast.error(tUi("itemUnavailable", { name: itemName }).trim());
            }
          };
        }
        return interaction.priceLabel != null || interaction.onSelect
          ? interaction
          : null;
      },
      [
        assistantAnalytics,
        currency,
        dispatchCartAction,
        isOrderingEnabled,
        itemOrderability,
        language,
        menuData,
        mode,
        tUi,
      ],
    );

    const turnBaseID =
      latestUserIndex >= 0
        ? `waiter-turn-${
            messages[latestUserIndex].clientNonce ??
            messages[latestUserIndex].id ??
            latestUserIndex
          }`
        : activeAssistantIndex >= 0
          ? `waiter-turn-${renderedMessages[activeAssistantIndex].stableID}`
          : null;
    const activeResponseID =
      activeAssistantIndex >= 0
        ? renderedMessages[activeAssistantIndex].response?.response_id
        : null;
    const latestTurnID =
      turnBaseID && activeResponseID
        ? `${turnBaseID}:${activeResponseID}`
        : turnBaseID;
    const [responseStartNode, setResponseStartNode] =
      useState<HTMLElement | null>(null);
    const setLatestResponseStart = useCallback((node: HTMLElement | null) => {
      setResponseStartNode((current) => (current === node ? current : node));
    }, []);
    useEffect(() => {
      if (activeAssistantIndex < 0) setResponseStartNode(null);
    }, [activeAssistantIndex]);
    const contentChange = useMemo(
      () =>
        latestTurnID
          ? {
              turnId: latestTurnID,
              responseStart:
                activeAssistantIndex >= 0 ? responseStartNode : null,
              responseComplete: !isLoading && activeAssistantIndex >= 0,
            }
          : undefined,
      [activeAssistantIndex, isLoading, latestTurnID, responseStartNode],
    );
    const activeAssistantMessage =
      activeAssistantIndex >= 0
        ? renderedMessages[activeAssistantIndex].message
        : null;
    const latestUserMessage =
      latestUserIndex >= 0 ? messages[latestUserIndex] : null;
    const contentChangeOrigin: "live" | "restored" =
      activeAssistantMessage?.response &&
      restoredResponseIDsForOpenRef.current.has(
        activeAssistantMessage.response.response_id,
      )
        ? "restored"
        : activeAssistantMessage?.delivery === "history"
          ? "restored"
          : latestUserMessage?.delivery === "optimistic" ||
              activeAssistantMessage?.delivery === "direct" ||
              activeAssistantMessage?.delivery === "poll" ||
              activeAssistantMessage?.delivery === "stream" ||
              activeAssistantMessage?.delivery === "client"
            ? "live"
            : "restored";

    // Sample a real menu item so the input hint reflects this restaurant's menu
    // instead of a hardcoded off-menu dish. Falls back to a neutral prompt when
    // the menu isn't loaded/empty. (audit D6)
    const sampleExampleDish = useMemo(() => {
        if (!Array.isArray(menuData)) return "";
        for (const cat of menuData) {
            const items = cat?.items;
            if (Array.isArray(items)) {
                for (const it of items) {
                    if (it?.name && typeof it.name === "string") return it.name;
                }
            }
        }
        return "";
    }, [menuData]);

    const handleResetChat = useCallback(() => {
      if (requestInFlightRef.current || recoveringSessionRef.current) return;
      recoveringSessionRef.current = true;
      void (async () => {
        resetConversation();
        setIsRecreatingSession(true);
        if (componentMountedRef.current) {
          appendAssistantText(tUi("newConversationNotice"));
        }
        try {
          await recreateSession();
        } catch (error) {
          if (componentMountedRef.current) {
            appendAssistantText(
              tUi(
                classifyChatError(error) === "rate_limited"
                  ? "rateLimited"
                  : "errConnect",
              ),
              "degraded",
            );
          }
        } finally {
          recoveringSessionRef.current = false;
          if (componentMountedRef.current) setIsRecreatingSession(false);
        }
      })();
    }, [appendAssistantText, recreateSession, resetConversation, tUi]);
    // Detect mobile device
    useEffect(() => {
        const checkMobile = () => {
      const isMobileDevice =
        /Android|webOS|iPhone|iPad|iPod|BlackBerry|IEMobile|Opera Mini/i.test(
          navigator.userAgent,
        );
            setIsMobile(isMobileDevice || window.innerWidth < 768);
        };
        checkMobile();
    window.addEventListener("resize", checkMobile);
    return () => window.removeEventListener("resize", checkMobile);
    }, []);

    // We removed the local initialize effect because the backend now handles 
    // persistent greetings via /messages if the session is new.

  const handleSendMessage = useCallback(
    async (content?: string): Promise<boolean> => {
      const textToSend = content ?? inputValue;
      if (
        !textToSend.trim() ||
        isLoading ||
        requestInFlightRef.current ||
        recoveringSessionRef.current
      ) {
        return false;
      }
      trackAssistantEvent(
        "assistant_message_sent",
        assistantEventProperties(assistantAnalytics, {
          message_size_bucket: Array.from(textToSend.trim()).length,
        }),
      );
      requestInFlightRef.current = true;
      try {
        const outcome = await sendMessage(textToSend);
        await handleTransportOutcome(outcome);
        return outcome.type === "assistant" || outcome.type === "human_ack";
      } finally {
        requestInFlightRef.current = false;
      }
    }, [assistantAnalytics, handleTransportOutcome, inputValue, isLoading, sendMessage],
  );

  const handleRetryMessage = useCallback(async () => {
    if (requestInFlightRef.current || recoveringSessionRef.current) return;
    trackAssistantEvent(
      "assistant_retry_requested",
      assistantEventProperties(assistantAnalytics),
    );
    requestInFlightRef.current = true;
    try {
      await handleTransportOutcome(await retryFailedMessage());
    } finally {
      requestInFlightRef.current = false;
    }
  }, [assistantAnalytics, handleTransportOutcome, retryFailedMessage]);

    return (
        <>
            {/* Floating Button */}
            {/*
             * Dock-aware bottom: PersistentGuestNav publishes --guest-nav-height.
             * Safe-area keeps the FAB clear of the home indicator; cartVisible
             * lifts an extra band so the 56px button clears the cart pill row.
             * Avoid magic 96px/148px that ignore dock height changes.
             */}
            <div
                className={`fixed end-6 z-40 ${
                    cartVisible
                        ? "bottom-[calc(var(--guest-nav-height,5.5rem)+var(--cookie-banner-height,0px)+env(safe-area-inset-bottom)+5.5rem)]"
                        : "bottom-[calc(var(--guest-nav-height,5.5rem)+var(--cookie-banner-height,0px)+env(safe-area-inset-bottom)+0.75rem)]"
                }`}
            >
                <AnimatePresence>
                    {nudgeVisible && !isOpen && !cartVisible ? (
                        <ProactiveNudge
                            message={tUi("nudgeMessage", { name: aiName })}
                            dismissLabel={tUi("nudgeDismiss")}
                            onEngage={() => {
                                setNudgeVisible(false);
                                openAssistant();
                            }}
                            onDismiss={() => setNudgeVisible(false)}
                            className="absolute bottom-full end-0 mb-3"
                        />
                    ) : null}
                </AnimatePresence>
                <motion.div
          animate={
            prefersReducedMotion
              ? undefined
              : {
                        scale: [1, 1.05, 1],
                }
          }
          transition={
            prefersReducedMotion
              ? undefined
              : {
                        duration: 3,
                        repeat: Infinity,
                        ease: "easeInOut",
                }
          }
                >
                    <Button
                        isIconOnly
                        radius="full"
                        size="lg"
                        aria-label={tId("openAssistant")}
                        className="bg-brand text-white shadow-2xl relative overflow-hidden group min-w-[56px] h-[56px]"
                        onPress={openAssistant}
                    >
                        <Bot className="w-6 h-6 z-10" />
                    </Button>
                </motion.div>
            </div>

            {/* Chat Interface — mount only while open (PG-15.3): a controlled
                isOpen=false exit can leave the portaled dialog hit-testable. */}
            {isOpen ? (
            <Modal
                isOpen
                onClose={closeAssistant}
                // The header is a custom <div> (not <ModalHeader>), so NextUI
                // won't auto-wire aria-labelledby — name the dialog explicitly to
                // match its visible title (WCAG 4.1.2).
                aria-label={aiName}
                {...AI_WAITER_PANEL_MODAL}
          motionProps={
            prefersReducedMotion
              ? {
                  variants: {
                    enter: { opacity: 1, transition: { duration: 0 } },
                    exit: { opacity: 0, transition: { duration: 0 } },
                  },
                }
              : undefined
          }
            >
                <ModalContent className="flex h-full min-h-0 flex-col">
                    {/* Premium Header — deep-teal ink-900 surface (D-3 re-skin) */}
                    <div className="p-4 sm:p-6 pb-3 sm:pb-4 flex items-center justify-between bg-ink-900 text-white rounded-none sm:rounded-ss-[1.5rem]">
                        <div className="flex items-center gap-3 sm:gap-4">
                            <div className="relative">
                                <div className="p-2 sm:p-2.5 bg-brand rounded-xl sm:rounded-2xl shadow-lg shadow-brand/20">
                                    <Sparkles className="w-4 h-4 sm:w-5 sm:h-5 text-white" />
                                </div>
                  <div
                    className={`absolute -bottom-1 -end-1 w-3 h-3 sm:w-3.5 sm:h-3.5 border-2 border-ink-900 rounded-full shadow-sm transition-colors ${isPolling ? "bg-brand motion-safe:animate-pulse" : "bg-emerald-500"}`}
                  />
                            </div>
                            <div>
                  <h3 className="text-base sm:text-lg font-semibold tracking-tight">
                    {aiName}
                  </h3>
                                <div className="flex items-center gap-1 sm:gap-1.5">
                    <div
                      className={`w-1.5 h-1.5 rounded-full ${isPolling ? "bg-brand motion-safe:animate-pulse" : "bg-emerald-500"}`}
                    />
                                    {/* subtitle is light-on-ink-900: text-ink-300 satisfies AA (≥4.5:1) */}
                                    <p className="text-[10px] sm:text-[11px] text-ink-300 font-medium uppercase tracking-wider">
                                        {tId("identity")}
                                    </p>
                                </div>
                            </div>
                        </div>
                        <div className="flex items-center gap-1 sm:gap-2">
                            <Button
                                isIconOnly
                                variant="light"
                                className="text-white hover:bg-white/10 rounded-xl sm:rounded-2xl h-8 w-8 sm:h-10 sm:w-10"
                                onPress={handleResetChat}
                                isDisabled={isLoading}
                                aria-label={tUi("resetChat")}
                                title={tUi("resetChat")}
                            >
                                <RotateCcw className="w-4 h-4 sm:w-5 sm:h-5 opacity-70" />
                            </Button>
                            <Button
                                isIconOnly
                                variant="light"
                                aria-label={tId("closeAssistant")}
                                className="text-white hover:bg-white/10 rounded-xl sm:rounded-2xl h-8 w-8 sm:h-10 sm:w-10"
                                onPress={closeAssistant}
                            >
                                <X className="w-4 h-4 sm:w-5 sm:h-5" />
                            </Button>
                        </div>
                    </div>

            <div
              className="px-4 sm:px-6 py-2 bg-warm-50 border-b border-warm-200/70 text-[10px] sm:text-[11px] leading-snug text-ink-500"
              role="note"
            >
                        {tId("disclosure")}
                    </div>

                    {/* Chat Content */}
                    <ModalBody className="min-h-0 flex-1 p-0">
                      <AssistantViewport
                        className="h-full min-h-0"
                        conversationLabel={tId("messagesRegionLabel")}
                        jumpToLatestLabel={tUi("jumpToLatest")}
                        completionAnnouncement={tUi("responseReady")}
                        contentChange={contentChange}
                        contentChangeOrigin={contentChangeOrigin}
                        analytics={assistantAnalytics}
                      >
                        <div className="space-y-4 p-4 sm:space-y-6 sm:p-6">
                          {/* #791: an empty thread must still read as a chat.
                              Greet the guest with an assistant bubble so the
                              panel is a conversation, not a settings rail with
                              chips floating in a void. */}
                          {renderedMessages.length === 0 && !isLoading ? (
                            <div className="flex justify-start">
                              <div className="max-w-[90%] rounded-2xl rounded-ss-sm border border-warm-200/70 bg-white px-4 py-3 text-[13px] leading-relaxed text-ink-800 shadow-sm sm:max-w-[85%] sm:rounded-3xl sm:px-5 sm:py-3.5 sm:text-[14px]">
                                {tUi("nudgeMessage", { name: aiName })}
                              </div>
                            </div>
                          ) : null}
                          {renderedMessages.map(
                            ({ message, response, stableID }, index) => (
                              <motion.div
                                key={stableID}
                                ref={
                                  response && index === activeAssistantIndex
                                    ? setLatestResponseStart
                                    : undefined
                                }
                                data-assistant-response-start={
                                  response && index === activeAssistantIndex
                                    ? "true"
                                    : undefined
                                }
                                initial={
                                  prefersReducedMotion
                                    ? undefined
                                    : { opacity: 0, y: 10 }
                                }
                                animate={
                                  prefersReducedMotion
                                    ? undefined
                                    : { opacity: 1, y: 0 }
                                }
                                className={`flex ${
                                  message.role === "user"
                                    ? "justify-end"
                                    : "justify-start"
                                }`}
                              >
                                {response ? (
                                  <div className="max-w-[90%] rounded-2xl rounded-ss-sm border border-warm-200/70 bg-white px-4 py-3 text-[13px] leading-relaxed text-ink-800 shadow-sm sm:max-w-[85%] sm:rounded-3xl sm:px-5 sm:py-3.5 sm:text-[14px]">
                                    <AssistantMessage
                                      response={response}
                                      labels={assistantLabels}
                                      renderEntityMedia={renderEntityMedia}
                                      entityInteraction={entityInteraction}
                                      compactEntities
                                      hideProtocolChrome
                                      onFollowUp={(followUp) =>
                                        void handleSendMessage(followUp.prompt)
                                      }
                                      analytics={{
                                        ...assistantAnalytics,
                                        contract_version:
                                          message.contractVersion,
                                      }}
                                    />
                                  </div>
                                ) : (
                                  <div className="max-w-[90%] whitespace-pre-wrap rounded-2xl rounded-se-sm bg-brand px-4 py-3 text-[13px] leading-relaxed text-white shadow-sm sm:max-w-[85%] sm:rounded-3xl sm:px-5 sm:py-3.5 sm:text-[14px]">
                                    {message.content}
                                  </div>
                                )}
                              </motion.div>
                            ),
                          )}

                          {!isLoading &&
                          !messages.some((message) => message.role === "user") ? (
                            <motion.div
                              initial={{ opacity: 0 }}
                              animate={{ opacity: 1 }}
                              className="flex flex-wrap gap-2 pt-2"
                            >
                              {starterQuestions.map((question) => (
                                <Button
                                  key={question}
                                  size="sm"
                                  variant="flat"
                                  radius="full"
                                  className="h-8 border border-warm-200 bg-white text-[11px] text-ink-600 hover:bg-warm-50"
                                  onPress={() =>
                                    void handleSendMessage(question)
                                  }
                                >
                                  {question}
                                </Button>
                              ))}
                            </motion.div>
                          ) : null}

                          {isLoading ? (
                            <div className="flex justify-start">
                              <div className="flex items-center gap-1 rounded-2xl rounded-ss-sm border border-warm-200/70 bg-white px-4 py-3 sm:rounded-3xl sm:px-5 sm:py-4">
                                <span className="sr-only">
                                  {tUi("typingIndicator")}
                                </span>
                                <div
                                  className="h-1.5 w-1.5 animate-bounce rounded-full bg-ink-400 [animation-delay:-0.3s]"
                                  aria-hidden="true"
                                />
                                <div
                                  className="h-1.5 w-1.5 animate-bounce rounded-full bg-ink-400 [animation-delay:-0.15s]"
                                  aria-hidden="true"
                                />
                                <div
                                  className="h-1.5 w-1.5 animate-bounce rounded-full bg-ink-400"
                                  aria-hidden="true"
                                />
                              </div>
                            </div>
                          ) : null}

                          {lastFailedInput && !isLoading ? (
                            <div className="flex justify-start">
                              <Button
                                size="sm"
                                variant="flat"
                                radius="full"
                                className="h-8 border border-warm-200 bg-white text-[11px] text-ink-600"
                                onPress={() => void handleRetryMessage()}
                              >
                                <RotateCcw className="me-1 h-3 w-3" />
                                {tUi("retrySend")}
                              </Button>
                            </div>
                          ) : null}
                        </div>
                      </AssistantViewport>
                    </ModalBody>

                    {/* Input Area */}
                    <div className="bg-white">
                      <AssistantComposer
                        focusOnMount
                        labels={{
                          textarea: tId("inputLabel"),
                          send: tUi("sendMessage"),
                          busy: tUi("sendingMessage"),
                          characterCount: (current, maximum) =>
                            tUi("charCount", {
                              count: current,
                              max: maximum,
                            }),
                        }}
                        maxLength={500}
                        value={inputValue}
                        onValueChange={setInputValue}
                        onSend={handleSendMessage}
                        busy={isLoading}
                        mobile={isMobile}
                        placeholder={
                          sampleExampleDish
                            ? tUi("placeholderWithDish", {
                                dish: sampleExampleDish,
                              })
                            : tUi("placeholder")
                        }
                      />
                        <p className="px-4 pb-3 text-center text-[10px] text-ink-500 sm:px-6 sm:pb-4">
                            {/* Concierge mode has no cart (the backend prompt takes no
                                orders here), so promising "ask me to add to your cart"
                                is dishonest — point guests at menu Q&A instead.
                                Same honesty when ordering is off (closed hours /
                                kitchen toggle): cartPrompt would lie. Basic
                                (no-model) mode never emits add_to_cart tool
                                calls either, so it gets the same copy. */}
                            {mode === "concierge" || !isOrderingEnabled || scripted
                                ? tUi("placeholder")
                                : tUi("cartPrompt")}
                        </p>
                    </div>
                </ModalContent>
            </Modal>
            ) : null}
        </>
    );
};
