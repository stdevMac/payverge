"use client";

import React, { useId, useRef } from "react";
import type { AssistantAction } from "@/types/assistant";
import {
  assistantEventProperties,
  type AssistantAnalyticsContext,
  trackAssistantEvent,
} from "./assistantAnalytics";

interface AssistantActionsLabels {
  actions: string;
  disabledActionReason: (reason: string | null) => string;
}

export interface AssistantActionsProps {
  actions: readonly AssistantAction[];
  labels: AssistantActionsLabels;
  onAction?: (
    action: AssistantAction,
  ) => "completed" | "failed" | void | Promise<"completed" | "failed" | void>;
  analytics?: AssistantAnalyticsContext;
}

export function AssistantActions({
  actions,
  labels,
  onAction,
  analytics,
}: AssistantActionsProps) {
  const groupID = useId();
  const pendingActionsRef = useRef(new Set<string>());

  if (actions.length === 0) return null;

  return (
    <section aria-labelledby={groupID} className="space-y-2">
      <h4 id={groupID} className="text-label font-semibold text-ink-700">
        {labels.actions}
      </h4>
      <div className="flex flex-wrap gap-2">
        {actions.map((action, index) => {
          const disabled = action.state !== "ready";
          const reasonID = `${groupID}-reason-${index}`;
          return (
            <div key={action.id} className="max-w-full">
              <button
                type="button"
                disabled={disabled}
                aria-describedby={disabled ? reasonID : undefined}
                onClick={() => {
                  if (
                    disabled ||
                    !onAction ||
                    pendingActionsRef.current.has(action.id)
                  ) {
                    return;
                  }
                  pendingActionsRef.current.add(action.id);
                  if (analytics) {
                    trackAssistantEvent(
                      "assistant_action_clicked",
                      assistantEventProperties(analytics, {
                        action_kind: action.type,
                      }),
                    );
                  }
                  let actionResult:
                    | "completed"
                    | "failed"
                    | void
                    | Promise<"completed" | "failed" | void>;
                  try {
                    actionResult = onAction(action);
                  } catch {
                    if (analytics) {
                      trackAssistantEvent(
                        "assistant_action_failed",
                        assistantEventProperties(analytics, {
                          action_kind: action.type,
                        }),
                      );
                    }
                    pendingActionsRef.current.delete(action.id);
                    return;
                  }
                  void Promise.resolve(actionResult)
                    .then((outcome) => {
                      if (!analytics || !outcome) return;
                      trackAssistantEvent(
                        outcome === "completed"
                          ? "assistant_action_completed"
                          : "assistant_action_failed",
                        assistantEventProperties(analytics, {
                          action_kind: action.type,
                        }),
                      );
                    })
                    .catch(() => {
                      if (!analytics) return;
                      trackAssistantEvent(
                        "assistant_action_failed",
                        assistantEventProperties(analytics, {
                          action_kind: action.type,
                        }),
                      );
                    })
                    .finally(() => {
                      pendingActionsRef.current.delete(action.id);
                    });
                }}
                className="max-w-full rounded-xl bg-brand px-4 py-2 text-body-sm font-semibold text-white hover:bg-brand-700 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand focus-visible:ring-offset-2 disabled:cursor-not-allowed disabled:bg-warm-200 disabled:text-ink-500"
              >
                {action.label}
              </button>
              {disabled ? (
                <p
                  id={reasonID}
                  className="mt-1 max-w-xs text-label text-ink-600"
                >
                  {labels.disabledActionReason(action.disabled_reason)}
                </p>
              ) : null}
            </div>
          );
        })}
      </div>
    </section>
  );
}
