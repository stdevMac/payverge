"use client";

import React, { Component, type ErrorInfo, type ReactNode, useId } from "react";
import type {
  AssistantAction,
  AssistantEntity,
  AssistantFollowUp,
  AssistantNotice,
  AssistantResponse,
} from "@/types/assistant";
import { AssistantActions } from "./AssistantActions";
import { AssistantRichText } from "./AssistantRichText";
import {
  AssistantSources,
  type AssistantSourcesLabels,
} from "./AssistantSources";
import {
  assistantEventProperties,
  type AssistantAnalyticsContext,
  trackAssistantEvent,
} from "./assistantAnalytics";

export interface AssistantMessageLabels extends AssistantSourcesLabels {
  actions: string;
  steps: string;
  entities: string;
  entityAvailability: (availability: string) => string;
  followUps: string;
  notices: string;
  noticeKind: (kind: string) => string;
  status: (status: AssistantResponse["status"]) => string;
  workflowProgress: (progress: {
    id: string;
    current: number;
    total: number;
  }) => string;
  disabledActionReason: (reason: string | null) => string;
  renderError: string;
}

/**
 * How one suggested entity behaves as a card (#791): an optional price line
 * and an optional tap target. `onSelect` turns the name/price row into a
 * button (labelled by `selectLabel` for assistive tech); without it the row
 * stays static — e.g. a browsable dish while the venue is closed.
 */
export interface AssistantEntityInteraction {
  priceLabel?: ReactNode;
  onSelect?: () => void;
  selectLabel?: string;
}

export interface AssistantMessageProps {
  response: AssistantResponse;
  labels: AssistantMessageLabels;
  onAction?: (
    action: AssistantAction,
  ) => "completed" | "failed" | void | Promise<"completed" | "failed" | void>;
  onFollowUp?: (followUp: AssistantFollowUp) => void;
  renderEntityMedia?: (entity: AssistantEntity) => ReactNode;
  /**
   * Per-entity price + tap behavior (#791). Optional and off by default so
   * operator surfaces keep plain entity rows.
   */
  entityInteraction?: (
    entity: AssistantEntity,
  ) => AssistantEntityInteraction | null;
  /**
   * Lay entities out as a compact 2-up grid (chat-card style) instead of the
   * full-width stack. Off by default for operator surfaces.
   */
  compactEntities?: boolean;
  /**
   * Hide the protocol chrome an operator finds useful but a diner reads as a
   * debug dump: the `Complete` / workflow status chips and the per-entity
   * `Availability: Available` row. Off by default so operator surfaces keep
   * the full envelope.
   */
  hideProtocolChrome?: boolean;
  className?: string;
  analytics?: AssistantAnalyticsContext;
}

interface MessageBoundaryProps {
  fallback: string;
  children: ReactNode;
  analytics?: AssistantAnalyticsContext;
}

interface MessageBoundaryState {
  failed: boolean;
}

class MessageBoundary extends Component<
  MessageBoundaryProps,
  MessageBoundaryState
> {
  state: MessageBoundaryState = { failed: false };

  static getDerivedStateFromError(): MessageBoundaryState {
    return { failed: true };
  }

  componentDidCatch(_error: Error, _info: ErrorInfo): void {
    if (!this.props.analytics) return;
    trackAssistantEvent(
      "assistant_render_fallback",
      assistantEventProperties(this.props.analytics),
    );
  }

  render() {
    if (this.state.failed) {
      return (
        <div
          role="alert"
          className="rounded-xl border border-rose-200 bg-rose-50 p-3 text-body-sm text-rose-800"
        >
          {this.props.fallback}
        </div>
      );
    }
    return this.props.children;
  }
}

function Steps({ steps, label }: { steps: readonly string[]; label: string }) {
  const labelID = useId();
  if (steps.length === 0) return null;
  return (
    <section aria-labelledby={labelID}>
      <h4 id={labelID} className="text-label font-semibold text-ink-700">
        {label}
      </h4>
      <ol className="mt-2 list-decimal space-y-1 ps-6 text-body-md text-ink-800">
        {steps.map((step, index) => (
          <li key={`${index}-${step}`} className="ps-1">
            {step}
          </li>
        ))}
      </ol>
    </section>
  );
}

function Entities({
  entities,
  labels,
  renderEntityMedia,
  entityInteraction,
  compactEntities,
  hideProtocolChrome,
}: {
  entities: readonly AssistantEntity[];
  labels: AssistantMessageLabels;
  renderEntityMedia?: (entity: AssistantEntity) => ReactNode;
  entityInteraction?: (
    entity: AssistantEntity,
  ) => AssistantEntityInteraction | null;
  compactEntities?: boolean;
  hideProtocolChrome?: boolean;
}) {
  const labelID = useId();
  if (entities.length === 0) return null;
  return (
    <section aria-labelledby={labelID}>
      <h4 id={labelID} className="text-label font-semibold text-ink-700">
        {labels.entities}
      </h4>
      <ul
        data-entity-layout={compactEntities ? "grid" : undefined}
        className={
          compactEntities
            ? "mt-2 grid grid-cols-2 gap-2"
            : "mt-2 grid gap-2"
        }
      >
        {entities.map((entity) => {
          const interaction = entityInteraction?.(entity) ?? null;
          // Two-column cards are too narrow to put the price beside the name:
          // the name wrapped a few letters per line. Stack them there instead.
          const identityRow = compactEntities
            ? "flex-col items-start gap-0.5"
            : "items-baseline justify-between gap-2";
          const identity = (
            <>
              <span className="min-w-0 break-words font-semibold text-ink-900">
                {entity.display_name}
              </span>
              {interaction?.priceLabel != null ? (
                <span className="shrink-0 text-body-sm font-medium tabular-nums text-ink-700">
                  {interaction.priceLabel}
                </span>
              ) : null}
            </>
          );
          return (
            <li
              key={`${entity.type}-${entity.id}`}
              className={
                compactEntities
                  ? "min-w-0 rounded-xl border border-warm-200 bg-warm-50 p-2"
                  : "rounded-xl border border-warm-200 bg-warm-50 p-3"
              }
            >
              {renderEntityMedia?.(entity)}
              {interaction?.onSelect ? (
                // Media stays outside the button: carousels carry their own
                // controls and nested interactives break assistive tech.
                <button
                  type="button"
                  onClick={interaction.onSelect}
                  aria-label={interaction.selectLabel ?? entity.display_name}
                  className={`mt-1 flex w-full ${identityRow} rounded-lg text-start transition hover:text-brand focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand`}
                >
                  {identity}
                </button>
              ) : interaction ? (
                <p className={`mt-1 flex ${identityRow}`}>
                  {identity}
                </p>
              ) : (
                // No interaction opt-in (operator transcript / plain path):
                // keep the pre-791 markup pixel-identical.
                <p className="font-semibold text-ink-900">
                  {entity.display_name}
                </p>
              )}
              {hideProtocolChrome ? null : (
                <p className="text-label text-ink-600">
                  {labels.entityAvailability(entity.availability)}
                </p>
              )}
            </li>
          );
        })}
      </ul>
    </section>
  );
}

function Notices({
  notices,
  labels,
}: {
  notices: readonly AssistantNotice[];
  labels: AssistantMessageLabels;
}) {
  const labelID = useId();
  if (notices.length === 0) return null;
  return (
    <section aria-labelledby={labelID}>
      <h4 id={labelID} className="text-label font-semibold text-ink-700">
        {labels.notices}
      </h4>
      <ul className="mt-2 space-y-2">
        {notices.map((notice) => (
          <li
            key={notice.id}
            className="rounded-xl border border-amber-200 bg-amber-50 p-3 text-body-sm text-ink-800"
          >
            <p className="font-semibold">{labels.noticeKind(notice.kind)}</p>
            <p>{notice.message}</p>
          </li>
        ))}
      </ul>
    </section>
  );
}

function claimReferenced<T extends { id: string }>(
  records: readonly T[],
  referencedIDs: readonly string[],
  claimed: Set<string>,
): T[] {
  const requested = new Set(referencedIDs);
  return records.filter((record) => {
    if (!requested.has(record.id) || claimed.has(record.id)) return false;
    claimed.add(record.id);
    return true;
  });
}

function AssistantMessageContent({
  response,
  labels,
  onAction,
  onFollowUp,
  renderEntityMedia,
  entityInteraction,
  compactEntities,
  hideProtocolChrome,
  className,
  analytics,
}: AssistantMessageProps) {
  const followUpsID = useId();
  const claimedActions = new Set<string>();
  const claimedSources = new Set<string>();
  const claimedEntities = new Set<string>();

  const sectionRecords = response.sections.map((section) => ({
    section,
    actions: claimReferenced(
      response.actions,
      section.action_ids,
      claimedActions,
    ),
    sources: claimReferenced(
      response.sources,
      section.source_ids,
      claimedSources,
    ),
    entities: claimReferenced(
      response.entities,
      section.entity_ids,
      claimedEntities,
    ),
  }));
  const remainingActions = response.actions.filter(
    ({ id }) => !claimedActions.has(id),
  );
  const remainingSources = response.sources.filter(
    ({ id }) => !claimedSources.has(id),
  );
  const remainingEntities = response.entities.filter(
    ({ id }) => !claimedEntities.has(id),
  );

  return (
    <article className={["space-y-4", className].filter(Boolean).join(" ")}>
      <AssistantRichText
        content={response.answer.content}
        format={response.answer.format}
        analytics={analytics}
      />

      {sectionRecords.map(({ section, actions, sources, entities }) => (
        <section
          key={section.id}
          className="space-y-3 rounded-2xl border border-warm-200 bg-warm-50/60 p-4"
        >
          <h3 className="text-heading-sm font-semibold text-ink-950">
            {section.title}
          </h3>
          <AssistantRichText
            content={section.answer}
            format="markdown"
            analytics={analytics}
          />
          <Steps steps={section.steps} label={labels.steps} />
          <AssistantActions
            actions={actions}
            labels={labels}
            onAction={onAction}
            analytics={analytics}
          />
          <AssistantSources
            sources={sources}
            labels={labels}
            analytics={analytics}
          />
          <Entities
            entities={entities}
            labels={labels}
            renderEntityMedia={renderEntityMedia}
            entityInteraction={entityInteraction}
            compactEntities={compactEntities}
            hideProtocolChrome={hideProtocolChrome}
          />
        </section>
      ))}

      <Steps steps={response.steps} label={labels.steps} />
      <AssistantActions
        actions={remainingActions}
        labels={labels}
        onAction={onAction}
        analytics={analytics}
      />
      <AssistantSources
        sources={remainingSources}
        labels={labels}
        analytics={analytics}
      />
      <Entities
        entities={remainingEntities}
        labels={labels}
        renderEntityMedia={renderEntityMedia}
        entityInteraction={entityInteraction}
        compactEntities={compactEntities}
        hideProtocolChrome={hideProtocolChrome}
      />
      <Notices notices={response.notices} labels={labels} />

      {response.follow_ups.length > 0 ? (
        <section aria-labelledby={followUpsID}>
          <h4
            id={followUpsID}
            className="text-label font-semibold text-ink-700"
          >
            {labels.followUps}
          </h4>
          <div className="mt-2 flex flex-wrap gap-2">
            {response.follow_ups.map((followUp) => (
              <button
                key={followUp.id}
                type="button"
                onClick={() => onFollowUp?.(followUp)}
                className="max-w-full whitespace-normal break-words rounded-full border border-brand/30 bg-brand/5 px-3 py-1.5 text-left text-body-sm font-medium text-brand hover:bg-brand/10 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand"
              >
                {followUp.label}
              </button>
            ))}
          </div>
        </section>
      ) : null}

      {hideProtocolChrome ? null : (
        <div className="flex flex-wrap items-center gap-2 text-label text-ink-600">
          <span className="rounded-full bg-warm-100 px-2 py-1">
            {labels.status(response.status)}
          </span>
          {response.workflow ? (
            <span className="rounded-full bg-brand/10 px-2 py-1 text-brand">
              {labels.workflowProgress({
                id: response.workflow.id,
                current: response.workflow.step_index + 1,
                total: response.workflow.step_total,
              })}
            </span>
          ) : null}
        </div>
      )}
    </article>
  );
}

export function AssistantMessage(props: AssistantMessageProps) {
  return (
    <MessageBoundary
      key={props.response.response_id}
      fallback={props.labels.renderError}
      analytics={props.analytics}
    >
      <AssistantMessageContent {...props} />
    </MessageBoundary>
  );
}
