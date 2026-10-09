import { axiosInstance } from "@/api/tools/instance";

// Staff chat + announcements (Slice 7). NO money anywhere on these types — team
// messaging and broadcasts only. Mirrors the backend models in
// backend/internal/database/chat_service.go + chat_announcement_service.go
// (envelope { success, data }). Channel/message reads additionally pass a per-row
// CanReadChannel gate server-side (a non-member gets 403); the realtime nudges
// (chat.message / chat.announcement) carry ids ONLY — clients refetch through
// these authorized read paths, never trusting an event body (DM privacy).

type ChatChannelType = "direct" | "group" | "role" | "announcement";

export interface ChatChannel {
  id: number;
  business_id: number;
  type: ChatChannelType;
  name: string;
  ref_key: string; // "role:server" / "dept:BOH" / "dm:lo:hi"; '' for manual groups
  is_archived: boolean;
  created_by_staff_id: number | null;
  created_at: string;
  updated_at: string;
}

export interface ChatMessage {
  id: number;
  business_id: number;
  channel_id: number;
  sender_staff_id: number;
  sender_name: string;
  content: string;
  parent_id: number | null; // reserved for future threading (v1 flat)
  attachment_url: string;
  created_at: string; // RFC3339
  updated_at: string;
}

export interface ChatRead {
  channel_id: number;
  staff_id: number;
  business_id: number;
  last_read_message_id: number;
  last_read_at: string;
}

export interface Announcement {
  id: number;
  business_id: number;
  author_staff_id: number;
  title: string;
  content: string;
  require_ack: boolean;
  audience_filter: string; // "all" | "role:<role>" | "dept:<dept>"
  created_at: string;
  updated_at: string;
}

export interface AnnouncementAck {
  announcement_id: number;
  staff_id: number;
  business_id: number;
  acknowledged_at: string;
}

/** One entry in the unacked roster ("waiting on …"). */
interface AckRosterMember {
  staff_id: number;
  name: string;
}

/** The "X of Y confirmed" aggregate + the unacked roster (announce-holders only). */
export interface AckStatus {
  announcement_id: number;
  acked: number;
  total_eligible: number;
  unacked: AckRosterMember[];
}

/**
 * The compact per-announcement ack aggregate the feed renders inline: acked/total
 * plus the first few acker names. The full unacked roster stays behind
 * announcementAcks (loaded on expand). Fetched in ONE batch call for the visible
 * require_ack announcements, replacing the per-announcement AckRoster poll.
 */
export interface AckSummary {
  announcement_id: number;
  acked: number;
  total_eligible: number;
  first_acker_names: string[];
}

/** A DM resolve: the channel, plus the posted message when content was sent. */
export interface DMResult {
  channel: ChatChannel;
  message?: ChatMessage;
}

export interface MessagePageParams {
  /** Oldest id already held → fetch the page strictly older than it (keyset). */
  cursor?: number;
  /** Server clamps to 50. */
  limit?: number;
}

export interface AnnouncementInput {
  title: string;
  content: string;
  require_ack: boolean;
  /** "all" | "role:<role>" | "dept:<dept>"; empty → all (server default). */
  audience_filter: string;
}

/**
 * The channels list with its unread-badge source. `unread` is keyed by channel id
 * (JSON string key) → count of messages the caller hasn't read and didn't send;
 * channels with zero unread are ABSENT (a missing key means 0). This is the only
 * server-provided unread signal — the realtime `chat.message` nudge is content-free.
 */
/**
 * Per-channel vitality strip: the newest live message as a bounded snippet.
 * Keyed like `unread` (JSON string channel id); channels with no live messages
 * are ABSENT. Assembled server-side in one grouped query (no N+1).
 */
interface ChatChannelPreview {
  snippet: string;
  sender_name: string;
  created_at: string;
}

export interface ChannelList {
  channels: ChatChannel[];
  unread: Record<string, number>;
  previews: Record<string, ChatChannelPreview>;
}

/**
 * The announcements feed plus the caller's per-announcement ack state. `acked[id]`
 * is true only for announcements this caller has confirmed; a missing id means
 * not-yet-acked. Lets the feed hide the Acknowledge button across reloads.
 */
export interface AnnouncementFeed {
  announcements: Announcement[];
  acked: Record<string, boolean>;
}

const base = (businessId: string) => "/inside/businesses/" + businessId;

export const chatApi = {
  // The caller's channels: their role channel + one per department they staff +
  // their manual DMs/groups (never another member's DMs), plus the unread map.
  listChannels: async (businessId: string): Promise<ChannelList> => {
    const res = await axiosInstance.get(base(businessId) + "/chat/channels");
    return {
      channels: res.data.data,
      unread: res.data.unread ?? {},
      previews: res.data.previews ?? {},
    };
  },

  // Keyset page of a channel's live messages (newest first, max 50). A non-member
  // receives 403 (CanReadChannel).
  listMessages: async (
    businessId: string,
    channelId: number,
    params?: MessagePageParams,
  ): Promise<ChatMessage[]> => {
    const res = await axiosInstance.get(
      base(businessId) + "/chat/channels/" + channelId + "/messages",
      { params },
    );
    return res.data.data;
  },

  // Post a message to a channel (chat:send + CanReadChannel). 201 with the message.
  postMessage: async (
    businessId: string,
    channelId: number,
    content: string,
  ): Promise<ChatMessage> => {
    const res = await axiosInstance.post(
      base(businessId) + "/chat/channels/" + channelId + "/messages",
      { content },
    );
    return res.data.data;
  },

  // Advance the caller's read marker. An empty/0 id means "mark everything read"
  // (the server resolves the channel's newest live message).
  markRead: async (
    businessId: string,
    channelId: number,
    lastReadMessageId?: number,
  ): Promise<ChatRead> => {
    const res = await axiosInstance.post(
      base(businessId) + "/chat/channels/" + channelId + "/read",
      { last_read_message_id: lastReadMessageId ?? 0 },
    );
    return res.data.data;
  },

  // Resolve (or create) the DM channel with `staffId`. With content → posts and
  // returns { channel, message } (201); without content → ensure-and-return
  // { channel } (200).
  dm: async (
    businessId: string,
    staffId: number,
    content?: string,
  ): Promise<DMResult> => {
    const res = await axiosInstance.post(
      base(businessId) + "/chat/dm/" + staffId,
      content ? { content } : {},
    );
    return res.data.data;
  },

  // Soft-delete a message (chat:moderate — manager/owner). 404 if no live match.
  deleteMessage: async (businessId: string, messageId: number): Promise<void> => {
    await axiosInstance.delete(base(businessId) + "/chat/messages/" + messageId);
  },

  // ---- Announcements ----

  // Broadcast a notice (chat:announce — manager/owner). 201 with the announcement.
  createAnnouncement: async (
    businessId: string,
    input: AnnouncementInput,
  ): Promise<Announcement> => {
    const res = await axiosInstance.post(base(businessId) + "/announcements", input);
    return res.data.data;
  },

  // The audience-filtered feed for the caller (newest first), plus the caller's
  // per-announcement ack state so the feed can hide the Acknowledge button on
  // notices already confirmed (survives reload, unlike client-only tracking).
  listAnnouncements: async (businessId: string): Promise<AnnouncementFeed> => {
    const res = await axiosInstance.get(base(businessId) + "/announcements");
    return { announcements: res.data.data, acked: res.data.acked ?? {} };
  },

  // Record the caller's acknowledgment (idempotent). Owners (no staff row) → 403.
  ackAnnouncement: async (
    businessId: string,
    announcementId: number,
  ): Promise<AnnouncementAck> => {
    const res = await axiosInstance.post(
      base(businessId) + "/announcements/" + announcementId + "/ack",
    );
    return res.data.data;
  },

  // The "X of Y confirmed" roster (chat:announce — manager/owner).
  announcementAcks: async (
    businessId: string,
    announcementId: number,
  ): Promise<AckStatus> => {
    const res = await axiosInstance.get(
      base(businessId) + "/announcements/" + announcementId + "/acks",
    );
    return res.data.data;
  },

  // Batch ack summaries for a bounded id list (chat:announce). Returns a map keyed
  // by announcement id → {acked,total_eligible,first_acker_names}. ONE call per
  // feed refresh for the VISIBLE require_ack announcements — the replacement for
  // the per-announcement AckRoster 30s poll. An empty id list is a no-op ({}).
  announcementAckSummaries: async (
    businessId: string,
    announcementIds: number[],
  ): Promise<Record<string, AckSummary>> => {
    if (announcementIds.length === 0) return {};
    const res = await axiosInstance.get(
      base(businessId) + "/announcements/ack-summaries",
      { params: { ids: announcementIds.join(",") } },
    );
    return res.data.data ?? {};
  },

  // Edit an announcement in place (chat:announce). Preserves the id + existing
  // acks (unlike a delete+recreate). 200 with the updated announcement.
  updateAnnouncement: async (
    businessId: string,
    announcementId: number,
    input: AnnouncementInput,
  ): Promise<Announcement> => {
    const res = await axiosInstance.put(
      base(businessId) + "/announcements/" + announcementId,
      input,
    );
    return res.data.data;
  },

  // Hard-delete an announcement (chat:announce). Removes it + its acks (stops ack
  // tracking) and records an RBAC audit entry server-side.
  deleteAnnouncement: async (
    businessId: string,
    announcementId: number,
  ): Promise<void> => {
    await axiosInstance.delete(base(businessId) + "/announcements/" + announcementId);
  },
};
