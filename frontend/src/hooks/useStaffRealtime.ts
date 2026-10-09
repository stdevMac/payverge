import { useCallback, useEffect, useRef } from "react";
import { useSSEEvents, type SSEEvent, type UseSSEEventsResult } from "./useSSEEvents";

// Staff-shell realtime hook. Subscribes to the SHARED business SSE stream
// (reusing the single EventSource from useSSEEvents) and fans named events out
// to typed callbacks. Introduced in Slice 2 for scheduling events; later slices
// (availability/time-off, coverage/swaps, chat) extend the handler set and the
// switch below — keep it ONE hook on the staff shell, one case per event.

interface SchedulePublishedPayload {
  schedule_id?: number;
  week_start?: string;
}
interface ShiftAssignedPayload {
  shift_id?: number;
  staff_id?: number;
  starts_at?: string;
}
interface ShiftUpdatedPayload {
  shift_id?: number;
  starts_at?: string;
  staff_id?: number;
}
interface TimeOffRequestedPayload {
  request_id?: number;
  staff_id?: number;
  starts_at?: string;
  ends_at?: string;
}
interface TimeOffDecidedPayload {
  request_id?: number;
  staff_id?: number;
  status?: string;
}
// Slice 5 — coverage. Open-shift claims + swap/give-up requests flowing through
// the approval state machine. Loosely typed (handlers usually just invalidate).
interface OpenShiftClaimedPayload {
  claim_id?: number;
  shift_id?: number;
  staff_id?: number;
}
interface OpenShiftDecidedPayload {
  claim_id?: number;
  shift_id?: number;
  status?: string;
}
interface SwapRequestedPayload {
  swap_id?: number;
  shift_id?: number;
  staff_id?: number;
}
interface SwapAcceptedPayload {
  swap_id?: number;
  shift_id?: number;
  staff_id?: number;
}
interface SwapDecidedPayload {
  swap_id?: number;
  shift_id?: number;
  status?: string;
}
// Slice 7 — chat & announcements. Content-free realtime nudges (ids only — the
// message text / DM body is intentionally absent for privacy); consumers refetch
// the authorized read path on receipt. See backend chat.go + sse_permissions.go.
interface ChatMessagePayload {
  channel_id?: number;
  message_id?: number;
}
interface ChatAnnouncementPayload {
  announcement_id?: number;
}
// Slice 4 — time clock. A clock-out or a manager correction (approve/reject/edit/
// manual-entry) changes the manager review queue. Ids + status only; the durable
// rows stay behind the timeclock:manage-gated timesheet reads. See backend
// timeclock.go + sse_permissions.go (timeclock:manage).
interface TimeclockEntryPayload {
  entry_id?: number;
  status?: string;
}

interface StaffRealtimeHandlers {
  onSchedulePublished?: (data: SchedulePublishedPayload) => void;
  onShiftAssigned?: (data: ShiftAssignedPayload) => void;
  onShiftUpdated?: (data: ShiftUpdatedPayload) => void;
  // Slice 3 — availability & time-off. A manager files/decides; the requester's
  // own list (and the operator approvals queue) live-refresh off these.
  onTimeOffRequested?: (data: TimeOffRequestedPayload) => void;
  onTimeOffDecided?: (data: TimeOffDecidedPayload) => void;
  // Slice 5 — coverage (open shifts & swaps). Drive CoverageBoard + the operator
  // ApprovalsPanel coverage rows; consumers typically invalidate the coverage
  // open/mine query keys on any of these.
  onOpenShiftClaimed?: (data: OpenShiftClaimedPayload) => void;
  onOpenShiftDecided?: (data: OpenShiftDecidedPayload) => void;
  onSwapRequested?: (data: SwapRequestedPayload) => void;
  onSwapAccepted?: (data: SwapAcceptedPayload) => void;
  onSwapDecided?: (data: SwapDecidedPayload) => void;
  // Slice 7 — chat & announcements. chat.message → the channel list/thread
  // refetches (and the unread badge bumps); chat.announcement → the announcements
  // feed refetches. Both are ids-only (DM privacy); see backend sse_permissions.go
  // (chat:read — universal across staff roles).
  onChatMessage?: (data: ChatMessagePayload) => void;
  onChatAnnouncement?: (data: ChatAnnouncementPayload) => void;
  // chat.announcement_ack (chat:announce holders only) → an open "X of Y
  // confirmed" roster refetches its acks live instead of via manual refresh.
  onChatAnnouncementAck?: (data: ChatAnnouncementPayload) => void;
  // Slice 4 — time clock. timeclock.entry (timeclock:manage holders only) → the
  // manager timesheet review queue refetches live when a clock-out or correction
  // lands, matching the time-off half of the inbox's freshness.
  onTimeclockEntry?: (data: TimeclockEntryPayload) => void;
  /**
   * Fired when the shared SSE connection recovers AFTER a drop (never on the
   * initial connect). Events emitted during the gap were not delivered, so
   * consumers must refetch/invalidate every query their other handlers feed —
   * otherwise schedules, coverage boards, and chat lists stay stale forever.
   */
  onReconnect?: () => void;
}

export interface UseStaffRealtimeOptions extends StaffRealtimeHandlers {
  businessId: number;
  /** Defaults to true; the hook is also a no-op when businessId is not positive. */
  enabled?: boolean;
}

/**
 * Subscribes the staff shell to scheduling realtime. Returns the underlying
 * SSE connection status so callers can surface a degraded/blocked hint.
 */
export function useStaffRealtime({
  businessId,
  enabled = true,
  ...handlers
}: UseStaffRealtimeOptions): UseSSEEventsResult {
  // Hold the latest handlers in a ref so onEvent stays referentially stable
  // (the shared connection re-subscribes only on businessId/enabled changes).
  const handlersRef = useRef<StaffRealtimeHandlers>(handlers);
  useEffect(() => {
    handlersRef.current = handlers;
  });

  const onEvent = useCallback((event: SSEEvent) => {
    const h = handlersRef.current;
    switch (event.type) {
      case "schedule.published":
        h.onSchedulePublished?.(event.data as SchedulePublishedPayload);
        break;
      case "shift.assigned":
        h.onShiftAssigned?.(event.data as ShiftAssignedPayload);
        break;
      case "shift.updated":
        h.onShiftUpdated?.(event.data as ShiftUpdatedPayload);
        break;
      case "timeoff.requested":
        h.onTimeOffRequested?.(event.data as TimeOffRequestedPayload);
        break;
      case "timeoff.decided":
        h.onTimeOffDecided?.(event.data as TimeOffDecidedPayload);
        break;
      case "openshift.claimed":
        h.onOpenShiftClaimed?.(event.data as OpenShiftClaimedPayload);
        break;
      case "openshift.decided":
        h.onOpenShiftDecided?.(event.data as OpenShiftDecidedPayload);
        break;
      case "shift.swap.requested":
        h.onSwapRequested?.(event.data as SwapRequestedPayload);
        break;
      case "shift.swap.accepted":
        h.onSwapAccepted?.(event.data as SwapAcceptedPayload);
        break;
      case "shift.swap.decided":
        h.onSwapDecided?.(event.data as SwapDecidedPayload);
        break;
      case "chat.message":
        h.onChatMessage?.(event.data as ChatMessagePayload);
        break;
      case "chat.announcement":
        h.onChatAnnouncement?.(event.data as ChatAnnouncementPayload);
        break;
      case "chat.announcement_ack":
        h.onChatAnnouncementAck?.(event.data as ChatAnnouncementPayload);
        break;
      case "timeclock.entry":
        h.onTimeclockEntry?.(event.data as TimeclockEntryPayload);
        break;
    }
  }, []);

  // Referentially stable (reads through handlersRef) so the shared connection
  // subscription isn't churned when a consumer re-renders with new closures.
  const onReconnect = useCallback(() => {
    handlersRef.current.onReconnect?.();
  }, []);

  return useSSEEvents({
    businessId,
    enabled: enabled && businessId > 0,
    onEvent,
    onReconnect,
  });
}
