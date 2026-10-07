import React, {
  useState,
  useEffect,
  useCallback,
  useMemo,
  useRef,
} from "react";
import toast from "react-hot-toast";
import {
  Input,
  Textarea,
  Button,
  Select,
  SelectItem,
  Chip,
  Spinner,
  Table,
  TableHeader,
  TableColumn,
  TableBody,
  TableRow,
  TableCell,
  Pagination,
  useDisclosure,
  Modal,
  ModalContent,
  ModalHeader,
  ModalBody,
  ModalFooter,
  Switch,
} from "@nextui-org/react";
import { EmptyState } from "@/components/ui/EmptyState";
import {
  Bot,
  Save,
  Play,
  Pause,
  Send,
  MessageSquare,
  BarChart3,
  Settings,
  AlertCircle,
  RefreshCw,
  ExternalLink,
  Sparkles,
} from "lucide-react";
import { useRouter } from "next/navigation";
import { usePolling } from "@/hooks/usePolling";
import { Business, getMenu, MenuCategory, MenuItem } from "@/api/business";
import { axiosInstance } from "@/api/tools/instance";
import {
  getTranslation,
  useSimpleLocale,
} from "@/i18n/SimpleTranslationProvider";
import type { Locale } from "@/i18n/config";
import {
  saveSettingsErrorMessage,
  saveSettingsSuccessMessage,
} from "./saveSettingsFeedback";
import {
  computeAllowedImageHosts,
  isAllowedImageUrl,
} from "@/components/guest/aiImageOrigins";
import { TrendChart } from "@/components/dashboard/charts/TrendChart";
import { HourOfDayChart } from "@/components/dashboard/charts/HourOfDayChart";
import { useBusinessAccess } from "@/hooks/useBusinessAccess";
import { getBusinessPageEditorPath } from "@/utils/businessUrl";
import {
  DATE_TIME_SHORT,
  formatBusinessDateTime,
} from "@/utils/businessTime";
import { useAuth } from "@/providers/HybridAuthProvider";
import { getAiWaiterCapabilities } from "@/utils/staffAuth";
import { useStaffPermissionsContext } from "@/contexts/StaffPermissionsContext";
import DashboardLockedTabView from "../DashboardLockedTabView";
import AiWaiterToggle from "./AiWaiterToggle";
import AiWaiterTestChat from "./AiWaiterTestChat";
import { resolveAiWaiterSandboxLocale } from "./resolveAiWaiterSandboxLocale";
import WhatsAppChannelStatus from "./WhatsAppChannelStatus";
import { AI_NAME_MAX_LEN, isAiNameTooLong } from "./aiNameValidation";
import {
  liveChatsHeaderValue,
  metricsHaveConversationData,
  planAiPriorityChange,
  shouldShowIdleServiceCopy,
} from "./aiWaiterHonesty";
import ConfirmationModal from "../modals/ConfirmationModal";
import { Metric as HonestMetric } from "@/components/ui/Metric";
import DashboardTabShell from "../shared/DashboardTabShell";
import DashboardTabLoadingSkeleton from "../shared/DashboardTabLoadingSkeleton";
import { DashboardTabTransition, PremiumPanel } from "../premium";
import { tableClassNames } from "../shared/tableStyles";
import { btnGhostIcon, btnPrimaryNextUI } from "@/components/ui/buttonStyles";

const aiWaiterTableClassNames = {
  ...tableClassNames,
  // Horizontal scroll + end padding keep ACTION fully visible (#177).
  base: `${tableClassNames.base} pr-16 sm:pr-20`,
  table: `${tableClassNames.table} min-w-[52rem]`,
} as const;

export function renderChatContent(
  text: string,
  allowedHosts: Set<string>,
  stripImageMarkdown: boolean = false,
): React.ReactNode {
  const MD_IMG = /!\[([^\]]*)\]\(([^)]+)\)/g;
  if (!MD_IMG.test(text)) return text;
  const parts: React.ReactNode[] = [];
  let last = 0;
  MD_IMG.lastIndex = 0;
  let m: RegExpExecArray | null;
  while ((m = MD_IMG.exec(text)) !== null) {
    if (m.index > last) parts.push(text.slice(last, m.index));
    if (isAllowedImageUrl(m[2], allowedHosts)) {
      parts.push(
        // Dynamic AI markdown URLs with allowlisted hosts — next/image needs
        // static remotePatterns; bare img is intentional here.
        // eslint-disable-next-line @next/next/no-img-element
        <img
          key={m.index}
          src={m[2]}
          alt={m[1]}
          className="rounded-lg max-w-full mt-2 mb-1"
        />,
      );
    } else if (!stripImageMarkdown) {
      parts.push(m[0]);
    }
    // When stripImageMarkdown is set (operator transcript), drop disallowed
    // image markdown entirely instead of leaking raw ![alt](url) syntax. (audit E5)
    last = m.index + m[0].length;
  }
  if (last < text.length) parts.push(text.slice(last));
  return <>{parts}</>;
}

interface AiWaiterDashboardProps {
  business: Business;
  onUpdateBusiness: (updatedBusiness: Business) => void;
  /** Wave 4: jump to Tables focused on a conversation's table_code. */
  onNavigateToTab?: (tab: string) => void;
}

interface Conversation {
  id: number;
  session_id: string;
  table_code: string;
  language: string;
  status: string;
  created_at: string;
  updated_at: string;
  message_count: number;
  is_paused: boolean;
  cart_items_added: number;
  claimed_by_staff_id?: number | null;
  claimed_by_name?: string;
  claimed_by_role?: string;
  claimed_at?: string | null;
}

interface Message {
  id: number;
  role: string;
  content: string;
  created_at: string;
  author_name?: string;
  author_role?: string;
}

// The backend rejects pause/claim/reply on a closed conversation with
// 409 conversation_closed (closed in another tab between load and click).
// Callers surface transcript.conversationClosed instead of a misleading
// conflict/failure message. (L4-7)
const isConversationClosedError = (err: unknown): boolean => {
  const ax = err as {
    response?: { status?: number; data?: { code?: string } };
  };
  return (
    ax?.response?.status === 409 &&
    ax.response?.data?.code === "conversation_closed"
  );
};


/** One-decimal avg messages/session for Monitor + Insights (L4-11). */
export function formatAvgMessages(
  totalMessages: number,
  totalConversations: number,
): string {
  if (totalConversations <= 0) return "0";
  return (totalMessages / totalConversations).toFixed(1);
}

export { metricsHaveConversationData } from "./aiWaiterHonesty";

export default function AiWaiterDashboard({
  business,
  onUpdateBusiness,
  onNavigateToTab,
}: AiWaiterDashboardProps) {
  const [aiName, setAiName] = useState(business.ai_settings?.ai_name || "Sage");
  const [aiPriority, setAiPriority] = useState(
    business.ai_settings?.ai_priority || "balanced",
  );
  const [specialInstructions, setSpecialInstructions] = useState(
    business.ai_settings?.special_instructions || "",
  );
  const [isSaving, setIsSaving] = useState(false);
  // Config-first: land on Overview & Settings so operators see kill switch +
  // guardrails before an empty Live Monitor (#177 / #173).
  const [activeTab, setActiveTab] = useState("overview");
  const router = useRouter();
  const {
    hasAccess,
    aiConfigured,
    loading: accessLoading,
  } = useBusinessAccess(business.id);
  const aiRequestsEnabled =
    !accessLoading && hasAccess && aiConfigured;

  // AI Waiter enabled state
  const [aiEnabled, setAiEnabled] = useState(
    business.ai_settings?.ai_enabled !== false,
  );
  const [businessPageAiEnabled, setBusinessPageAiEnabled] = useState(
    business.ai_settings?.business_page_ai_enabled || false,
  );

  // Up to two real, available menu-item names used to make the "Simulated
  // Intro" upsell preview reflect THIS business's menu instead of hardcoded
  // placeholder dishes. Empty until the menu loads (or if the menu is empty),
  // in which case the preview falls back to generic, non-fictional copy.
  const [sampleMenuItems, setSampleMenuItems] = useState<string[]>([]);

  const { locale } = useSimpleLocale();
  const t = useCallback(
    (key: string, params?: Record<string, string | number>) =>
      getTranslation(`aiWaiterDashboard.${key}`, locale, params) as string,
    [locale],
  );

  const allowedImageHosts = useMemo(() => computeAllowedImageHosts([]), []);
  // Operator transcripts strip image markdown rather than render it: the guest
  // already saw the AI's image recommendations live, and the dashboard has no
  // menu context to build a real allowlist, so raw ![alt](url) would otherwise
  // leak into the transcript. (audit E5)
  const stripImageMarkdownInTranscript = true;

  // Monitoring State
  const [conversations, setConversations] = useState<Conversation[]>([]);
  const [isLoadingConversations, setIsLoadingConversations] = useState(false);
  const [page, setPage] = useState(1);
  const [totalPages, setTotalPages] = useState(1);
  const [listTotalCount, setListTotalCount] = useState(0);
  // Server-truth active-conversation count (rides the list response) for the
  // "Live chats" stat — no longer the visible page length. (fix 11)
  const [activeCount, setActiveCount] = useState<number | null>(null);
  // Owner/manager status filter ("" = all). Front-line staff are forced to
  // "active" below so their paginator isn't padded with closed rows. (fix 5)
  const [statusFilter, setStatusFilter] = useState<"" | "active" | "closed">(
    "",
  );

  // Fetch error state for the on-mount trio (shows inline retry banner)
  const [fetchConversationsError, setFetchConversationsError] = useState(false);
  const [fetchInsightsError, setFetchInsightsError] = useState(false);
  const [insightsReady, setInsightsReady] = useState(false);

  // Insights State
  const [insights, setInsights] = useState<{
    total_conversations: number;
    total_messages: number;
    upsell_success_rate: number;
    conversation_trends_7d: { date: string; value: number }[];
    busiest_hours: { hour: number; value: number }[];
  }>({
    total_conversations: 0,
    total_messages: 0,
    upsell_success_rate: 0,
    conversation_trends_7d: [],
    busiest_hours: [],
  });

  const [pausingIds, setPausingIds] = useState<Set<number>>(new Set());
  const [optimisticPaused, setOptimisticPaused] = useState<
    Record<number, boolean>
  >({});

  // Transcript State
  const [selectedConversation, setSelectedConversation] =
    useState<Conversation | null>(null);
  const [transcript, setTranscript] = useState<Message[]>([]);
  const [isLoadingTranscript, setIsLoadingTranscript] = useState(false);
  const [transcriptError, setTranscriptError] = useState(false);
  const { isOpen, onOpen, onClose } = useDisclosure();
  // Focus restoration + explicit Escape-to-close for the transcript modal
  // (audit C3). NextUI provides these by default; we wire them explicitly to
  // match the ManagerPinModal pattern and guarantee WCAG keyboard dismissal
  // and focus return to the triggering "View chat" button on close.
  const transcriptTriggerRef = useRef<HTMLElement | null>(null);
  const handleModalClose = useCallback(() => {
    onClose();
    transcriptTriggerRef.current?.focus?.();
  }, [onClose]);
  useEffect(() => {
    if (!isOpen) return;
    const onKeyDown = (e: KeyboardEvent) => {
      if (e.key === "Escape") {
        e.preventDefault();
        handleModalClose();
      }
    };
    window.addEventListener("keydown", onKeyDown);
    return () => window.removeEventListener("keydown", onKeyDown);
  }, [isOpen, handleModalClose]);

  // Live transcript refresh while the modal is open. Without this the operator
  // never saw a guest's new reply until they sent a message or reopened the
  // modal — all while the claim TTL ticked. Poll INCREMENTALLY: send `since`
  // = the newest message's created_at so the server returns only new rows,
  // which we append/dedupe instead of re-downloading the full 200-message
  // payload every 4s. (fix 6)
  const selectedConvId = selectedConversation?.id ?? null;
  // Ref-backed latest-message cursor so the poll reads the current value
  // without re-subscribing the interval on every transcript change.
  const transcriptCursorRef = useRef<string | null>(null);
  useEffect(() => {
    transcriptCursorRef.current =
      transcript.length > 0
        ? transcript[transcript.length - 1].created_at
        : null;
  }, [transcript]);
  const pollTranscript = useCallback(
    async (signal?: AbortSignal) => {
      if (!aiRequestsEnabled || !isOpen || selectedConvId == null) return;
      try {
        const since = transcriptCursorRef.current;
        const url = since
          ? `/inside/businesses/${business.id}/ai/conversations/${selectedConvId}/messages?since=${encodeURIComponent(since)}`
          : `/inside/businesses/${business.id}/ai/conversations/${selectedConvId}/messages`;
        const response = await axiosInstance.get(url, signal ? { signal } : undefined);
        if (signal?.aborted) return;
        const incoming = (response.data ?? []) as Message[];
        if (!since) {
          setTranscript(incoming);
        } else if (incoming.length > 0) {
          setTranscript((prev) => {
            const seen = new Set(prev.map((m) => m.id));
            const fresh = incoming.filter((m) => !seen.has(m.id));
            return fresh.length > 0 ? [...prev, ...fresh] : prev;
          });
        }
      } catch (error) {
        if (
          (error instanceof DOMException && error.name === "AbortError") ||
          (error instanceof Error && error.name === "AbortError")
        ) {
          return;
        }
        console.warn("AI transcript poll failed", error);
      }
    },
    [aiRequestsEnabled, isOpen, selectedConvId, business.id],
  );
  usePolling({
    callback: pollTranscript,
    interval: 4000,
    enabled: Boolean(aiRequestsEnabled && isOpen && selectedConvId != null),
    immediate: false,
    pauseWhenHidden: true,
  });

  // Permission-scoped capabilities. Owners (no staff role) may configure the AI;
  // staff use effective permissions from context.
  const { staffData } = useAuth();
  const { permissions } = useStaffPermissionsContext();
  const isOwner = !staffData;
  const aiCaps = getAiWaiterCapabilities(permissions, isOwner);
  const myStaffId = staffData?.id;

  // 1-second tick driving the claim-expiry countdown chip while a takeover is
  // held. Only runs while the modal is open and this staffer holds the claim,
  // so it never spins idly. (R3-AI-5)
  const [nowTick, setNowTick] = useState(() => Date.now());
  const claimHeldByMe =
    selectedConversation?.claimed_by_staff_id != null &&
    selectedConversation?.claimed_by_staff_id === myStaffId &&
    !!selectedConversation?.claimed_at;
  useEffect(() => {
    if (!isOpen || !claimHeldByMe) return;
    setNowTick(Date.now());
    const interval = setInterval(() => setNowTick(Date.now()), 1000);
    return () => clearInterval(interval);
  }, [isOpen, claimHeldByMe, selectedConversation?.claimed_at]);

  // Milliseconds until the 5-minute idle claim TTL lapses (null when not held).
  const CLAIM_TTL_MS = 5 * 60 * 1000;
  const claimRemainingMs =
    claimHeldByMe && selectedConversation?.claimed_at
      ? CLAIM_TTL_MS -
        (nowTick - new Date(selectedConversation.claimed_at).getTime())
      : null;
  const formatClaimRemaining = (ms: number): string => {
    const total = Math.max(0, Math.floor(ms / 1000));
    const m = Math.floor(total / 60);
    const s = total % 60;
    return `${m}:${String(s).padStart(2, "0")}`;
  };

  const [staffReply, setStaffReply] = useState("");
  const [isSendingReply, setIsSendingReply] = useState(false);
  const [isClosingConversation, setIsClosingConversation] = useState(false);
  const [showBusinessPageRedirectConfirm, setShowBusinessPageRedirectConfirm] =
    useState(false);
  // Confirm before ending a live guest chat (destructive, no reopen). (fix 10)
  const [showCloseConfirm, setShowCloseConfirm] = useState(false);
  const [stealTarget, setStealTarget] = useState<{
    conv: Conversation;
    name: string;
  } | null>(null);
  const [showUpsellConfirm, setShowUpsellConfirm] = useState(false);

  // Keep the active sub-tab valid: front-line staff never land on a hidden tab.
  useEffect(() => {
    if (activeTab === "overview" && !aiCaps.canViewConfig)
      setActiveTab("monitor");
    if (activeTab === "insights" && !aiCaps.canViewInsights)
      setActiveTab("monitor");
  }, [activeTab, aiCaps.canViewConfig, aiCaps.canViewInsights]);

  // Front-line staff (server/host, canClose=false) only handle live chats.
  // The server now filters their page to status=active (fix 5), so this is a
  // belt-and-suspenders client guard — NOT the primary mechanism — that also
  // keeps a stale in-memory page from briefly showing a closed row. It no
  // longer distorts the paginator because the server page is already active-only.
  const visibleConversations = aiCaps.canClose
    ? conversations
    : conversations.filter((cv) => cv.status === "active");

  // Replying requires holding a fresh claim — including managers/owners
  // (decision #4: no silent bypass; steal via Claim with confirm first).
  const selectedClaimedByMe =
    (selectedConversation?.claimed_by_staff_id === myStaffId &&
      selectedConversation?.claimed_by_staff_id != null) ||
    (myStaffId == null &&
      selectedConversation?.claimed_by_staff_id == null &&
      selectedConversation?.claimed_by_role === "owner" &&
      !!selectedConversation?.claimed_at);
  const canReplyNow = selectedClaimedByMe;

  // AI Toggle status change handler
  const handleAiStatusChange = (enabled: boolean) => {
    // Restriction: AI Waiter for Business Page requires Business Page to be enabled
    if (enabled && !business.business_page_enabled) {
      setShowBusinessPageRedirectConfirm(true);
      return;
    }

    setAiEnabled(enabled);
    // Refresh business data
    refreshBusiness();
  };

  // Use refreshBusiness from onUpdateBusiness
  const refreshBusiness = () => {
    // Trigger a refresh by calling onUpdateBusiness with current data
    // The parent will re-fetch from the API
    onUpdateBusiness(business);
  };

  // Front-line staff (no canClose) only ever see live chats, so they must ask
  // the server for status=active — otherwise a server page of ALL statuses gets
  // client-filtered to active and the paginator shows "500 pages, all empty".
  // Owners/managers use the chosen filter (default all). (fix 5)
  const effectiveStatus: "" | "active" | "closed" = aiCaps.canClose
    ? statusFilter
    : "active";

  // Monotonic request id so a slow page/filter response can't overwrite a newer
  // one (request-id guard).
  const conversationsReqId = useRef(0);

  // Fetch conversations
  const fetchConversations = useCallback(
    async (pageNum: number) => {
      const reqId = ++conversationsReqId.current;
      setIsLoadingConversations(true);
      try {
        const params = new URLSearchParams({ page: String(pageNum) });
        if (effectiveStatus) params.set("status", effectiveStatus);
        const response = await axiosInstance.get(
          `/inside/businesses/${business.id}/ai/conversations?${params.toString()}`,
        );
        if (reqId !== conversationsReqId.current) return; // superseded
        const rows = response.data?.conversations;
        setConversations(Array.isArray(rows) ? rows : []);
        setTotalPages(response.data?.total_pages ?? 1);
        if (typeof response.data?.total_count === "number") {
          setListTotalCount(response.data.total_count);
        } else {
          setListTotalCount(Array.isArray(rows) ? rows.length : 0);
        }
        // active_count is a server-truth COUNT independent of the page/filter.
        if (typeof response.data.active_count === "number") {
          setActiveCount(response.data.active_count);
        }
        // If the result set shrank below the requested page, clamp back —
        // the paginator hides at totalPages <= 1, which would otherwise
        // strand the user on an empty out-of-range page.
        if (pageNum > response.data.total_pages) {
          setPage(Math.max(1, response.data.total_pages));
        }
        setFetchConversationsError(false);
      } catch (error) {
        if (reqId !== conversationsReqId.current) return;
        console.error("Failed to fetch conversations:", error);
        setFetchConversationsError(true);
      } finally {
        if (reqId === conversationsReqId.current) {
          setIsLoadingConversations(false);
        }
      }
    },
    [business.id, effectiveStatus],
  );

  // Fetch insights
  const fetchInsights = useCallback(async () => {
    try {
      const response = await axiosInstance.get(
        `/inside/businesses/${business.id}/ai/insights`,
      );
      // Spread defaults so a partial/empty payload can't crash the
      // StatsSummary tiles (e.g. `.upsell_success_rate.toFixed(1)`).
      setInsights({
        total_conversations: 0,
        total_messages: 0,
        upsell_success_rate: 0,
        conversation_trends_7d: [],
        busiest_hours: [],
        ...(response.data ?? {}),
      });
      setFetchInsightsError(false);
    } catch (error) {
      console.error("Failed to fetch insights:", error);
      setFetchInsightsError(true);
    } finally {
      setInsightsReady(true);
    }
  }, [business.id]);

  // Conversations refetch on page/filter change (cheap, paginated).
  useEffect(() => {
    if (!aiRequestsEnabled || activeTab !== "monitor") return;
    fetchConversations(page).catch((err) =>
      console.error("fetchConversations failed:", err),
    );
  }, [aiRequestsEnabled, activeTab, page, fetchConversations]);

  // Insights once per dashboard mount so Overview can tell "service on, no
  // chats yet" from a live service. Do not refetch on pagination (fix 4).
  useEffect(() => {
    if (!aiRequestsEnabled) return;
    setInsightsReady(false);
    fetchInsights().catch((err) =>
      console.error("fetchInsights failed:", err),
    );
  }, [aiRequestsEnabled, fetchInsights]);

  // Monitor liveness: the takeover surface had no SSE and no polling, so a
  // teammate's claim or a brand-new conversation stayed invisible while the
  // 5-minute claim clock ran. There is no conversation event on the shared SSE
  // stream, so poll the current page + counts every 15s, paused while the tab
  // is backgrounded. (fix 7)
  useEffect(() => {
    // L4-6: history remains visible while service is paused — keep polling.
    if (!aiRequestsEnabled || activeTab !== "monitor") return;
    const tick = () => {
      if (typeof document !== "undefined" && document.hidden) return;
      fetchConversations(page).catch((err) =>
        console.warn("monitor liveness poll failed", err),
      );
    };
    const interval = setInterval(tick, 15000);
    // Catch up immediately when the tab regains focus.
    const onVisible = () => {
      if (typeof document !== "undefined" && !document.hidden) tick();
    };
    document.addEventListener("visibilitychange", onVisible);
    return () => {
      clearInterval(interval);
      document.removeEventListener("visibilitychange", onVisible);
    };
  }, [aiRequestsEnabled, activeTab, page, fetchConversations]);

  // Load a couple of real, available menu items so the "Simulated Intro"
  // upsell preview cites this business's actual dishes instead of hardcoded
  // placeholders. Config-only (overview tab, owners) and best-effort: a
  // failure/empty menu simply leaves the preview on its generic fallback line.
  useEffect(() => {
    if (!aiRequestsEnabled || activeTab !== "overview" || !aiCaps.canViewConfig)
      return;
    if (sampleMenuItems.length > 0) return;
    let cancelled = false;
    (async () => {
      try {
        const menu = await getMenu(business.id);
        const rawCategories =
          typeof menu.categories === "string"
            ? (JSON.parse(menu.categories || "[]") as MenuCategory[])
            : menu.categories;
        const categories = Array.isArray(rawCategories) ? rawCategories : [];
        const names = categories
          .flatMap((cat) => (Array.isArray(cat.items) ? cat.items : []))
          .filter((item: MenuItem) => item?.is_available && item?.name?.trim())
          .map((item: MenuItem) => item.name.trim())
          .slice(0, 2);
        if (!cancelled) setSampleMenuItems(names);
      } catch (error) {
        console.error("Failed to load menu for AI intro preview:", error);
      }
    })();
    return () => {
      cancelled = true;
    };
  }, [
    aiRequestsEnabled,
    activeTab,
    aiCaps.canViewConfig,
    business.id,
    sampleMenuItems.length,
  ]);

  // Upsell line of the simulated intro. Prefer real menu items; when the menu
  // hasn't loaded or is empty, use generic copy that doesn't invent dishes.
  const upsellIntroLine = useMemo(() => {
    if (sampleMenuItems.length >= 2) {
      return t("settings.introTemplates.upsellingItems", {
        first: sampleMenuItems[0],
        second: sampleMenuItems[1],
      });
    }
    if (sampleMenuItems.length === 1) {
      return t("settings.introTemplates.upsellingItem", {
        first: sampleMenuItems[0],
      });
    }
    return t("settings.introTemplates.upsellingGeneric");
  }, [sampleMenuItems, t]);

  // Claim/release a conversation (front-line takeover). Defined here, before
  // the tier early-returns, so the hook order stays stable across renders.
  const handleClaim = useCallback(
    async (conv: Conversation, opts?: { steal?: boolean }) => {
      try {
        const res = await axiosInstance.post(
          `/inside/businesses/${business.id}/ai/conversations/${conv.id}/claim`,
          opts?.steal ? { steal: true } : {},
        );
        // The claim response returns the new holder (claimed_by_staff_id/name/role);
        // the server also set is_paused:true and claimed_at:now. Merge that into
        // the OPEN transcript modal so canReplyNow flips immediately — without this
        // selectedConversation stayed stale and "Pick up" looped forever. (R3-AI-2)
        const data = (res?.data ?? {}) as Partial<Conversation>;
        setSelectedConversation((prev) =>
          prev && prev.id === conv.id
            ? {
                ...prev,
                is_paused: true,
                claimed_by_staff_id:
                  data.claimed_by_staff_id ??
                  myStaffId ??
                  prev.claimed_by_staff_id,
                claimed_by_name: data.claimed_by_name ?? prev.claimed_by_name,
                claimed_by_role: data.claimed_by_role ?? prev.claimed_by_role,
                claimed_at: data.claimed_at ?? new Date().toISOString(),
              }
            : prev,
        );
        await fetchConversations(page);
      } catch (err: unknown) {
        const ax = err as {
          response?: {
            status?: number;
            data?: { claimed_by_name?: string; code?: string };
          };
        };
        if (isConversationClosedError(err)) {
          toast.error(t("transcript.conversationClosed"));
          setSelectedConversation((prev) =>
            prev && prev.id === conv.id ? { ...prev, status: "closed" } : prev,
          );
          await fetchConversations(page);
        } else if (ax?.response?.status === 409) {
          const who =
            ax.response.data?.claimed_by_name || t("monitor.someoneElse");
          const code = ax.response.data?.code;
          // Explicit steal: managers/owners confirm take-over instead of silent bypass.
          if (
            code === "claim_steal_required" &&
            aiCaps.canForceRelease &&
            !opts?.steal
          ) {
            setStealTarget({ conv, name: who });
            return;
          }
          if (opts?.steal) {
            toast.error(t("monitor.claimConflict", { name: who }));
          } else {
            toast.error(t("monitor.claimConflict", { name: who }));
          }
          await fetchConversations(page);
        } else {
          toast.error(t("monitor.claimFailed"));
        }
      }
    },
    [business.id, page, fetchConversations, myStaffId, t, aiCaps.canForceRelease],
  );

  const handleRelease = useCallback(
    async (conv: Conversation) => {
      try {
        await axiosInstance.post(
          `/inside/businesses/${business.id}/ai/conversations/${conv.id}/release`,
        );
        await fetchConversations(page);
      } catch {
        toast.error(t("monitor.releaseFailed"));
      }
    },
    [business.id, page, fetchConversations, t],
  );

  // Inline fetch-error banner with retry button (shown above a section on on-mount fetch failures).
  const FetchErrorBanner = ({
    messageKey,
    onRetry,
    testId,
  }: {
    messageKey: string;
    onRetry: () => void;
    testId?: string;
  }) => (
    <div
      data-testid={testId}
      role="alert"
      className="flex items-center justify-between gap-2 p-3 mb-3 bg-red-50 text-red-700 text-xs rounded-lg border border-red-100"
    >
      <div className="flex items-center gap-2">
        <AlertCircle size={14} />
        <span>{t(messageKey)}</span>
      </div>
      <Button
        size="sm"
        variant="flat"
        color="danger"
        startContent={<RefreshCw className="w-3 h-3" />}
        onPress={onRetry}
      >
        {t("errors.retry")}
      </Button>
    </div>
  );

  // Stats Summary — consistent empty treatment across all three KPI tiles (#195).
  // No conversations yet → all three "No data yet". Once chats exist, show
  // numbers (including a real 0.0% upsell rate) so the row never mixes formats.
  const hasConversationData = metricsHaveConversationData(
    insights.total_conversations,
  );
  const showIdleCopy = shouldShowIdleServiceCopy({
    aiEnabled,
    insightsReady,
    insightsError: fetchInsightsError,
    totalConversations: insights.total_conversations,
  });
  const liveChats = liveChatsHeaderValue(activeCount);

  const handlePriorityChange = (next: string) => {
    const plan = planAiPriorityChange(aiPriority, next);
    if (plan === "confirm-upselling") {
      setShowUpsellConfirm(true);
      return;
    }
    if (plan === "apply") {
      setAiPriority(next);
    }
  };
  const StatsSummary = () => (
    <div className="grid grid-cols-1 md:grid-cols-3 gap-3 mb-6">
      <PremiumPanel className="flex flex-col px-5 py-6" withTexture>
        {hasConversationData ? (
          <HonestMetric
            label={t("stats.totalConversations")}
            state="ok"
            value={insights.total_conversations}
            format="count"
            size="lg"
            data-testid="metric-total-conversations"
          />
        ) : (
          <HonestMetric
            label={t("stats.totalConversations")}
            state="empty"
            value={0}
            reason={t("stats.noData")}
            format="count"
            size="lg"
            data-testid="metric-total-conversations"
          />
        )}
        <p className="text-body-sm text-ink-500 mt-1">
          {t("stats.totalConversationsHint")}
        </p>
      </PremiumPanel>
      <PremiumPanel className="flex flex-col px-5 py-6" withTexture>
        {hasConversationData ? (
          <HonestMetric
            label={t("stats.upsellSuccessRate")}
            state="ok"
            value={
              Number.isFinite(insights.upsell_success_rate)
                ? insights.upsell_success_rate
                : 0
            }
            format={(n) => `${n.toFixed(1)}%`}
            size="lg"
            data-testid="metric-upsell-success-rate"
          />
        ) : (
          <HonestMetric
            label={t("stats.upsellSuccessRate")}
            state="empty"
            value={0}
            reason={t("stats.noData")}
            format={(n) => `${n.toFixed(1)}%`}
            size="lg"
            data-testid="metric-upsell-success-rate"
          />
        )}
        {hasConversationData ? (
          <p className="text-body-sm text-ink-500 mt-1">
            {t("stats.upsellSuccessRateHint")}
          </p>
        ) : null}
      </PremiumPanel>
      <PremiumPanel className="flex flex-col px-5 py-6" withTexture>
        {hasConversationData ? (
          <HonestMetric
            label={t("stats.avgMessages")}
            state="ok"
            value={Number(
              formatAvgMessages(
                insights.total_messages,
                insights.total_conversations,
              ),
            )}
            format={(n) => n.toFixed(1)}
            size="lg"
            data-testid="metric-avg-messages"
          />
        ) : (
          <HonestMetric
            label={t("stats.avgMessages")}
            state="empty"
            value={0}
            reason={t("stats.noData")}
            format={(n) => n.toFixed(1)}
            size="lg"
            data-testid="metric-avg-messages"
          />
        )}
        <p className="text-body-sm text-ink-500 mt-1">
          {t("stats.avgMessagesHint")}
        </p>
      </PremiumPanel>
    </div>
  );

  const handleSaveSettings = async () => {
    setIsSaving(true);
    try {
      const response = await axiosInstance.put(
        `/inside/businesses/${business.id}`,
        {
          ai_name: aiName,
          ai_priority: aiPriority,
          ai_special_instructions: specialInstructions,
          business_page_ai_enabled: businessPageAiEnabled,
        },
      );
      onUpdateBusiness(response.data);
      // L4-4: success feedback was missing entirely on "Guardar Configuración".
      toast.success(saveSettingsSuccessMessage(t("errors.saveSuccess")));
    } catch (error) {
      console.error("Failed to update settings:", error);
      toast.error(
        saveSettingsErrorMessage(
          error,
          locale as Locale,
          t("errors.saveFailed"),
        ),
      );
    } finally {
      setIsSaving(false);
    }
  };

  const handleViewTranscript = async (conv: Conversation) => {
    // Remember the trigger so focus returns to it when the modal closes (C3).
    transcriptTriggerRef.current =
      (document.activeElement as HTMLElement) || null;
    setSelectedConversation(conv);
    setStaffReply("");
    onOpen();
    setTranscriptError(false);
    setIsLoadingTranscript(true);
    try {
      const response = await axiosInstance.get(
        `/inside/businesses/${business.id}/ai/conversations/${conv.id}/messages`,
      );
      setTranscript(response.data);
    } catch (error) {
      console.error("Failed to fetch transcript:", error);
      setTranscript([]);
      setTranscriptError(true);
    } finally {
      setIsLoadingTranscript(false);
    }
  };

  const handlePauseAi = async () => {
    if (!selectedConversation) return;
    const newPausedState = !selectedConversation.is_paused;
    try {
      await axiosInstance.post(
        `/inside/businesses/${business.id}/ai/conversations/${selectedConversation.id}/pause`,
        {
          is_paused: newPausedState,
        },
      );
      // Update local state
      setSelectedConversation({
        ...selectedConversation,
        is_paused: newPausedState,
      });
      // Refresh conversation list to show badge
      await fetchConversations(page);
    } catch (error) {
      console.error("Failed to toggle pause:", error);
      if (isConversationClosedError(error)) {
        toast.error(t("transcript.conversationClosed"));
        setSelectedConversation((prev) =>
          prev ? { ...prev, status: "closed", is_paused: false } : prev,
        );
        await fetchConversations(page);
      } else {
        toast.error(t("errors.pauseFailed"));
      }
    }
  };

  const handleInlinePause = async (conv: Conversation) => {
    if (pausingIds.has(conv.id)) return; // already in-flight

    const newPausedState = !conv.is_paused;
    setOptimisticPaused((prev) => ({ ...prev, [conv.id]: newPausedState }));
    setPausingIds((prev) => {
      const next = new Set(prev);
      next.add(conv.id);
      return next;
    });

    try {
      await axiosInstance.post(
        `/inside/businesses/${business.id}/ai/conversations/${conv.id}/pause`,
        { is_paused: newPausedState },
      );
      await fetchConversations(page);
      setOptimisticPaused((prev) => {
        const next = { ...prev };
        delete next[conv.id];
        return next;
      });
    } catch (error) {
      console.error("Inline pause failed:", error);
      if (isConversationClosedError(error)) {
        toast.error(t("transcript.conversationClosed"));
        await fetchConversations(page);
      } else {
        toast.error(t("monitor.pauseFailed"));
      }
      setOptimisticPaused((prev) => {
        const next = { ...prev };
        delete next[conv.id];
        return next;
      });
    } finally {
      setPausingIds((prev) => {
        const next = new Set(prev);
        next.delete(conv.id);
        return next;
      });
    }
  };

  const handleSendReply = async () => {
    if (!selectedConversation || !staffReply.trim()) return;
    setIsSendingReply(true);
    try {
      await axiosInstance.post(
        `/inside/businesses/${business.id}/ai/conversations/${selectedConversation.id}/reply`,
        {
          content: staffReply,
        },
      );
      // Refresh transcript
      const response = await axiosInstance.get(
        `/inside/businesses/${business.id}/ai/conversations/${selectedConversation.id}/messages`,
      );
      setTranscript(response.data);
      setStaffReply("");
      // Refresh conversation list to show latest message in table
      await fetchConversations(page);
    } catch (error) {
      console.error("Failed to send reply:", error);
      const ax = error as { response?: { status?: number } };
      if (isConversationClosedError(error)) {
        toast.error(t("transcript.conversationClosed"));
        setSelectedConversation((prev) =>
          prev && prev.id === selectedConversation.id
            ? {
                ...prev,
                status: "closed",
                is_paused: false,
                claimed_by_staff_id: null,
                claimed_at: null,
              }
            : prev,
        );
        await fetchConversations(page);
      } else if (ax?.response?.status === 409) {
        // The claim was lost (idle-expired / reassigned) — the reply box
        // stays open but replying is blocked until re-claimed. Reflect the
        // lost claim in local state so the footer swaps back to a "Pick up
        // again" affordance, and tell the operator what happened. (R3-AI-5)
        toast.error(t("transcript.claimExpired"));
        setSelectedConversation((prev) =>
          prev && prev.id === selectedConversation.id
            ? { ...prev, claimed_by_staff_id: null, claimed_at: null }
            : prev,
        );
        await fetchConversations(page);
      } else {
        toast.error(t("errors.sendReplyFailed"));
      }
    } finally {
      setIsSendingReply(false);
    }
  };

  const handleCloseConversation = async () => {
    if (!selectedConversation) return;
    setIsClosingConversation(true);
    try {
      await axiosInstance.post(
        `/inside/businesses/${business.id}/ai/conversations/${selectedConversation.id}/close`,
      );
      // Update local state and close modal
      setSelectedConversation(null);
      onClose();
      // Refresh conversation list
      await fetchConversations(page);
    } catch (error) {
      console.error("Failed to close conversation:", error);
      toast.error(t("errors.closeFailed"));
    } finally {
      setIsClosingConversation(false);
    }
  };

  // Sub-view strip lives in the shell (the one tab language). Built from caps
  // so front-line staff never see config/insights, mirroring the content gates.
  const tabItems = [
    ...(aiCaps.canViewConfig
      ? [{ key: "overview", label: t("tabs.overview"), icon: Settings }]
      : []),
    { key: "monitor", label: t("tabs.monitor"), icon: MessageSquare },
    ...(aiCaps.canViewInsights
      ? [{ key: "insights", label: t("tabs.insights"), icon: BarChart3 }]
      : []),
  ];

  return (
    <DashboardTabShell
      loading={
        accessLoading ? (
          <DashboardTabLoadingSkeleton labelKey="loadingTab" withPageChrome={false} />
        ) : null
      }
      locked={
        !accessLoading && !hasAccess ? (
          <DashboardLockedTabView
            title={t("title")}
            subtitle={t("subtitle")}
            businessId={business.id}
          />
        ) : null
      }
      header={{
        title: t("title"),
        subtitle: !aiEnabled
          ? t("subtitle")
          : showIdleCopy
            ? t("shell.serviceOnIdleDetail")
            : t("shell.serviceOnDetail"),
        status: {
          label: !aiEnabled
            ? t("shell.serviceOff")
            : showIdleCopy
              ? t("shell.serviceOnIdle")
              : t("shell.serviceOn"),
          tone: !aiEnabled ? "neutral" : showIdleCopy ? "attention" : "positive",
        },
        stats:
          aiEnabled && liveChats != null
            ? [
                {
                  label: t("shell.liveChats"),
                  value: liveChats,
                },
              ]
            : [],
      }}
      tabs={{
        items: tabItems,
        activeKey: activeTab,
        onChange: setActiveTab,
        ariaLabel: t("tabsAria") || "AI Dashboard tabs",
      }}
    >
      {/* AI Waiter Activation Card (only shows when disabled) */}
      <AiWaiterToggle
        businessId={business.id}
        isLocked={!hasAccess}
        initialEnabled={aiEnabled}
        onStatusChange={handleAiStatusChange}
        variant="card"
      />

      {/* Workspace stays visible when service is paused so history is not orphaned (L4-6).
          Guest runtime + write controls still gate on aiEnabled. */}
      <DashboardTabTransition tabKey={activeTab}>
          {activeTab === "overview" && aiCaps.canViewConfig && (
            <div className="space-y-4">
              {showIdleCopy ? (
                <div
                  className="flex items-start gap-2 rounded-xl border border-amber-200 bg-amber-50 p-3 text-sm text-amber-800"
                  role="status"
                  data-testid="ai-idle-banner"
                >
                  <Bot size={16} aria-hidden="true" className="mt-0.5 shrink-0" />
                  <span>{t("shell.idleBanner")}</span>
                </div>
              ) : null}
              <WhatsAppChannelStatus businessId={business.id} locale={locale} />
              <PremiumPanel className="p-4 sm:p-6" withTexture={false}>
                <div className="flex items-center justify-between gap-4">
                  <div className="flex flex-col">
                    <p className="text-md font-bold">
                      {t("settings.personalityTitle")}
                    </p>
                    <p className="text-small text-default-500">
                      {t("settings.personalitySubtitle")}
                    </p>
                  </div>
                  <AiWaiterToggle
                    businessId={business.id}
                    isLocked={!hasAccess}
                    initialEnabled={aiEnabled}
                    onStatusChange={handleAiStatusChange}
                    variant="button"
                  />
                </div>
                <div className="my-4 h-px bg-warm-200/70" />
                <div className="flex flex-col gap-6 max-w-2xl">
                  <div className="space-y-4">
                    <Input
                      label={t("settings.aiName")}
                      placeholder={t("settings.aiNamePlaceholder")}
                      value={aiName}
                      onValueChange={setAiName}
                      description={t("settings.aiNameDescription")}
                      isDisabled={!aiEnabled}
                      maxLength={AI_NAME_MAX_LEN}
                      isInvalid={isAiNameTooLong(aiName)}
                      errorMessage={
                        isAiNameTooLong(aiName)
                          ? t("settings.aiNameTooLong")
                          : undefined
                      }
                    />

                    <Select
                      label={t("settings.priorityMode")}
                      selectedKeys={[aiPriority]}
                      onChange={(e) => handlePriorityChange(e.target.value)}
                      description={t("settings.priorityDescription")}
                      isDisabled={!aiEnabled}
                      data-testid="ai-priority-select"
                    >
                      <SelectItem
                        key="balanced"
                        value="balanced"
                        description={t("settings.priorities.balancedDesc")}
                      >
                        {t("settings.priorities.balanced")}
                      </SelectItem>
                      <SelectItem
                        key="service"
                        value="service"
                        description={t("settings.priorities.serviceDesc")}
                      >
                        {t("settings.priorities.service")}
                      </SelectItem>
                      <SelectItem
                        key="upselling"
                        value="upselling"
                        description={t("settings.priorities.upsellingDesc")}
                      >
                        {t("settings.priorities.upselling")}
                      </SelectItem>
                    </Select>
                    {aiPriority === "upselling" ? (
                      <p
                        className="-mt-2 text-[12px] leading-snug text-amber-700"
                        role="note"
                        data-testid="ai-upselling-guardrail"
                      >
                        {t("settings.upsellingGuardrail")}
                      </p>
                    ) : null}

                    <Textarea
                      label={t("settings.specialInstructions")}
                      placeholder={t("settings.specialInstructionsPlaceholder")}
                      value={specialInstructions}
                      onValueChange={setSpecialInstructions}
                      description={t("settings.specialInstructionsDescription")}
                      minRows={3}
                      isDisabled={!aiEnabled}
                    />
                    <p
                      className="-mt-2 text-[12px] leading-snug text-amber-700"
                      role="note"
                    >
                      {t("settings.specialInstructionsWarning")}
                    </p>
                    <p
                      className="-mt-1 text-[12px] leading-snug text-amber-700"
                      role="note"
                      data-testid="ai-allergen-guardrail"
                    >
                      {t("settings.allergenGuardrail")}
                    </p>

                    <div className="p-4 bg-warm-50 rounded-lg border border-warm-100">
                      <h4 className="text-sm font-semibold mb-2">
                        {t("settings.simulatedIntro")}
                      </h4>
                      <p className="text-sm text-ink-500 italic">
                        &quot;
                        {t("settings.introTemplates.default", {
                          name: aiName || "...",
                          business: business.name,
                        })}
                        {aiPriority === "upselling" && " " + upsellIntroLine}
                        {aiPriority === "balanced" &&
                          " " + t("settings.introTemplates.balanced")}
                        {aiPriority === "service" &&
                          " " + t("settings.introTemplates.service")}
                        &quot;
                      </p>
                    </div>

                    <AiWaiterTestChat
                      businessId={business.id}
                      aiEnabled={aiEnabled}
                      language={resolveAiWaiterSandboxLocale(
                        locale,
                        business.default_language,
                      )}
                      t={t}
                    />
                  </div>

                  <Button
                    radius="full"
                    className={`mt-4 ${btnPrimaryNextUI}`}
                    startContent={<Save className="w-4 h-4" />}
                    isLoading={isSaving}
                    isDisabled={!aiEnabled}
                    onPress={handleSaveSettings}
                  >
                    {t("settings.saveButton")}
                  </Button>
                </div>
              </PremiumPanel>

              <PremiumPanel className="p-4 sm:p-6" withTexture={false}>
                <div className="flex gap-3 items-center">
                  <Bot className="w-5 h-5 text-primary" />
                  <div className="flex flex-col">
                    <p className="text-md font-bold">{t("visibility.title")}</p>
                    <p className="text-small text-default-500">
                      {t("visibility.subtitle")}
                    </p>
                  </div>
                </div>
                <div className="my-4 h-px bg-warm-200/70" />
                <div className="flex flex-col gap-4">
                  <div
                    className={`flex items-center justify-between p-4 bg-warm-50 rounded-xl border border-warm-100 transition-opacity ${!aiEnabled ? "opacity-50" : ""}`}
                  >
                    <div className="flex items-center gap-3">
                      <div
                        className={`p-2 rounded-lg ${businessPageAiEnabled ? "bg-brand/10 text-brand" : "bg-warm-100 text-ink-400"}`}
                      >
                        <ExternalLink size={20} />
                      </div>
                      <div>
                        <p className="font-medium">
                          {t("visibility.publicBusinessPage")}
                        </p>
                        <p className="text-xs text-ink-500">
                          {t("visibility.publicBusinessPageDesc")}
                        </p>
                      </div>
                    </div>
                    <Switch
                      isSelected={businessPageAiEnabled}
                      onValueChange={setBusinessPageAiEnabled}
                      isDisabled={!aiEnabled}
                      color="primary"
                    />
                  </div>

                  {!business.business_page_enabled && (
                    <div className="flex items-center gap-2 p-3 bg-amber-50 text-amber-700 text-xs rounded-lg border border-amber-100">
                      <AlertCircle size={14} />
                      <span>{t("visibility.businessPageDisabled")}</span>
                    </div>
                  )}

                  {!aiEnabled && (
                    <div className="flex items-center gap-2 p-3 bg-brand/5 text-brand text-xs rounded-lg border border-brand/20">
                      <Bot size={14} />
                      <span>{t("visibility.enableAIWaiterFirst")}</span>
                    </div>
                  )}
                </div>
              </PremiumPanel>
            </div>
          )}

          {activeTab === "monitor" && (
            <div>
              <StatsSummary />

              {!aiEnabled && (
                <div
                  className="mb-4 flex items-center gap-2 rounded-xl border border-amber-200 bg-amber-50 p-3 text-sm text-amber-800"
                  role="status"
                  data-testid="ai-service-paused-banner"
                >
                  <Bot size={16} aria-hidden="true" />
                  <span>{t("monitor.servicePausedBanner")}</span>
                </div>
              )}

              {fetchConversationsError && (
                <FetchErrorBanner
                  testId="fetch-error-conversations"
                  messageKey="errors.fetchConversationsFailed"
                  onRetry={() => {
                    fetchConversations(page).catch(() => undefined);
                  }}
                />
              )}

              <PremiumPanel className="p-4 sm:p-6" withTexture={false}>
                <div className="flex justify-between items-center gap-3">
                  <h3 className="text-lg font-semibold">
                    {t("monitor.recentConversations")}
                  </h3>
                  <div className="flex items-center gap-2">
                    {/* Status filter (owners/managers only — front-line staff
                        are already pinned to active server-side). (fix 5) */}
                    {aiCaps.canClose ? (
                      <Select
                        aria-label={t("monitor.statusFilter.label")}
                        size="sm"
                        className="w-36"
                        selectedKeys={[statusFilter || "all"]}
                        onSelectionChange={(keys) => {
                          const key = Array.from(keys)[0] as string;
                          setStatusFilter(
                            key === "all"
                              ? ""
                              : (key as "active" | "closed"),
                          );
                          setPage(1);
                        }}
                      >
                        <SelectItem key="all" role="option">
                          {t("monitor.statusFilter.all")}
                        </SelectItem>
                        <SelectItem key="active" role="option">
                          {t("monitor.statusFilter.active")}
                        </SelectItem>
                        <SelectItem key="closed" role="option">
                          {t("monitor.statusFilter.closed")}
                        </SelectItem>
                      </Select>
                    ) : null}
                    <button
                      type="button"
                      className={btnGhostIcon}
                      onClick={() => fetchConversations(page)}
                      aria-label={t("monitor.refresh")}
                      title={t("monitor.refresh")}
                    >
                      <RefreshCw className="h-4 w-4" aria-hidden="true" />
                    </button>
                  </div>
                </div>
                <div className="mt-4" data-testid="ai-conversations-table">
                  <Table
                    aria-label={
                      t("monitor.table.tableAria") || "Conversations table"
                    }
                    removeWrapper
                    classNames={aiWaiterTableClassNames}
                  >
                    <TableHeader>
                      <TableColumn>{t("monitor.table.session")}</TableColumn>
                      <TableColumn>{t("monitor.table.table")}</TableColumn>
                      <TableColumn>{t("monitor.table.language")}</TableColumn>
                      <TableColumn>{t("monitor.table.started")}</TableColumn>
                      <TableColumn>{t("monitor.table.lastActivity")}</TableColumn>
                      <TableColumn>{t("monitor.table.status")}</TableColumn>
                      <TableColumn
                        className="min-w-[11rem] text-right"
                        data-testid="ai-monitor-actions-col"
                      >
                        {t("monitor.table.action")}
                      </TableColumn>
                    </TableHeader>
                    <TableBody
                      items={visibleConversations}
                      emptyContent={
                        isLoadingConversations ? (
                          t("monitor.loading")
                        ) : fetchConversationsError ||
                          listTotalCount > 0 ||
                          (insights.total_conversations ?? 0) > 0 ? null : (
                          <EmptyState
                            panel
                            compact
                            icon={MessageSquare}
                            title={t("monitor.emptyTitle")}
                            subtitle={t("monitor.emptyBody")}
                          />
                        )
                      }
                    >
                      {(item) => {
                        const isPaused =
                          optimisticPaused[item.id] ?? item.is_paused;
                        return (
                          <TableRow key={item.id}>
                            <TableCell className="font-mono text-xs text-ink-500">
                              <span
                                title={item.session_id}
                                className="cursor-help underline decoration-dotted decoration-ink-300 underline-offset-2"
                              >
                                {item.session_id.substring(0, 8)}…
                              </span>
                            </TableCell>
                            <TableCell>
                              {item.table_code && onNavigateToTab ? (
                                <button
                                  type="button"
                                  onClick={() =>
                                    onNavigateToTab(
                                      `tables?tableSearch=${encodeURIComponent(item.table_code)}`,
                                    )
                                  }
                                  className="rounded text-brand underline decoration-brand/40 underline-offset-2 hover:text-brand-dark focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand"
                                >
                                  {item.table_code}
                                </button>
                              ) : (
                                item.table_code || t("monitor.unknownTable")
                              )}
                            </TableCell>
                            <TableCell>
                              <Chip size="sm" variant="flat">
                                {item.language.toUpperCase()}
                              </Chip>
                            </TableCell>
                            <TableCell className="text-sm">
                              {formatBusinessDateTime(
                                item.created_at,
                                locale,
                                business.timezone ?? null,
                                DATE_TIME_SHORT,
                              )}
                            </TableCell>
                            <TableCell className="text-sm">
                              {formatBusinessDateTime(
                                item.updated_at,
                                locale,
                                business.timezone ?? null,
                                DATE_TIME_SHORT,
                              )}
                            </TableCell>
                            <TableCell>
                              <Chip
                                color={
                                  item.status === "active"
                                    ? "success"
                                    : "default"
                                }
                                size="sm"
                                variant="dot"
                              >
                                {/* Map backend status → locale-aware label. `fallback`
                                                                    catches anything new the backend ships that we
                                                                    haven't translated yet, so the chip still reads
                                                                    "OPEN/ABIERTA" instead of leaking a raw code. */}
                                {(() => {
                                  const key = `monitor.status.${item.status}`;
                                  const translated = t(key);
                                  return translated && translated !== key
                                    ? translated
                                    : t("monitor.status.fallback");
                                })()}
                              </Chip>
                              {isPaused && (
                                <Chip
                                  size="sm"
                                  color="warning"
                                  variant="flat"
                                  className="ml-2"
                                >
                                  {t("monitor.badges.paused")}
                                </Chip>
                              )}
                              {item.cart_items_added > 0 && (
                                <Chip
                                  size="sm"
                                  variant="flat"
                                  startContent={
                                    <Sparkles
                                      size={12}
                                      className="text-brand"
                                    />
                                  }
                                  className="ml-2 bg-brand/10 text-brand-dark font-semibold"
                                >
                                  {t("monitor.badges.upsell")}
                                </Chip>
                              )}
                            </TableCell>
                            <TableCell className="min-w-[11rem]">
                              <div className="flex flex-wrap items-center justify-end gap-1 whitespace-nowrap">
                                <Button
                                  size="sm"
                                  radius="full"
                                  variant="light"
                                  onPress={() => handleViewTranscript(item)}
                                >
                                  {t("monitor.viewChat")}
                                </Button>
                                {(() => {
                                  // Closed chats are view-only — never offer
                                  // Pick up / hand back on a finished guest session.
                                  if (item.status === "closed") {
                                    return null;
                                  }
                                  const claimedByOther =
                                    item.claimed_by_staff_id != null &&
                                    item.claimed_by_staff_id !== myStaffId;
                                  const claimedByMe =
                                    item.claimed_by_staff_id != null &&
                                    item.claimed_by_staff_id === myStaffId;
                                  if (
                                    claimedByOther &&
                                    !aiCaps.canForceRelease
                                  ) {
                                    return (
                                      <Chip
                                        size="sm"
                                        variant="flat"
                                        color="warning"
                                      >
                                        {t("monitor.handledBy", {
                                          name: item.claimed_by_name || "",
                                        })}
                                      </Chip>
                                    );
                                  }
                                  if (claimedByMe) {
                                    return (
                                      <Button
                                        size="sm"
                                        radius="full"
                                        variant="flat"
                                        onPress={() => handleRelease(item)}
                                      >
                                        {t("monitor.handBack")}
                                      </Button>
                                    );
                                  }
                                  return (
                                    <Button
                                      size="sm"
                                      radius="full"
                                      className={btnPrimaryNextUI}
                                      onPress={() => handleClaim(item)}
                                    >
                                      {t("monitor.pickUp")}
                                    </Button>
                                  );
                                })()}
                                {item.status !== "closed" && (
                                  <Button
                                    isIconOnly
                                    size="sm"
                                    radius="full"
                                    variant="light"
                                    isLoading={pausingIds.has(item.id)}
                                    isDisabled={pausingIds.has(item.id)}
                                    onPress={(e) => {
                                      (
                                        e as unknown as {
                                          stopPropagation?: () => void;
                                        }
                                      )?.stopPropagation?.();
                                      handleInlinePause(item);
                                    }}
                                    aria-label={t(
                                      isPaused
                                        ? "monitor.resumeInline"
                                        : "monitor.pauseInline",
                                    )}
                                    className={
                                      isPaused
                                        ? "text-amber-600 hover:text-amber-700"
                                        : "text-ink-500 hover:text-brand"
                                    }
                                  >
                                    {isPaused ? (
                                      <Play className="w-4 h-4" />
                                    ) : (
                                      <Pause className="w-4 h-4" />
                                    )}
                                  </Button>
                                )}
                              </div>
                            </TableCell>
                          </TableRow>
                        );
                      }}
                    </TableBody>
                  </Table>
                  {totalPages > 1 && !fetchConversationsError && (
                    <div className="flex justify-center mt-4">
                      <Pagination
                        total={totalPages}
                        page={page}
                        onChange={(p) => setPage(p)}
                      />
                    </div>
                  )}
                </div>
              </PremiumPanel>
            </div>
          )}

          {activeTab === "insights" && aiCaps.canViewInsights && (
            <div>
              {fetchInsightsError && (
                <div className="mt-4">
                  <FetchErrorBanner
                    testId="fetch-error-insights"
                    messageKey="errors.fetchInsightsFailed"
                    onRetry={() => {
                      fetchInsights().catch(() => undefined);
                    }}
                  />
                </div>
              )}
              <div className="grid grid-cols-1 md:grid-cols-3 gap-3 mt-4">
                <PremiumPanel className="flex flex-col px-5 py-6" withTexture>
                  {hasConversationData ? (
                    <HonestMetric
                      label={t("stats.totalConversations")}
                      state="ok"
                      value={insights.total_conversations}
                      format="count"
                      size="lg"
                      data-testid="metric-total-conversations-insights"
                    />
                  ) : (
                    <HonestMetric
                      label={t("stats.totalConversations")}
                      state="empty"
                      value={0}
                      reason={t("stats.noData")}
                      format="count"
                      size="lg"
                      data-testid="metric-total-conversations-insights"
                    />
                  )}
                </PremiumPanel>
                <PremiumPanel className="flex flex-col px-5 py-6" withTexture>
                  {hasConversationData ? (
                    <HonestMetric
                      label={t("stats.upsellSuccessRate")}
                      state="ok"
                      value={
                        Number.isFinite(insights.upsell_success_rate)
                          ? insights.upsell_success_rate
                          : 0
                      }
                      format={(n) => `${n.toFixed(1)}%`}
                      size="lg"
                      data-testid="metric-upsell-success-rate-insights"
                    />
                  ) : (
                    <HonestMetric
                      label={t("stats.upsellSuccessRate")}
                      state="empty"
                      value={0}
                      reason={t("stats.noData")}
                      format={(n) => `${n.toFixed(1)}%`}
                      size="lg"
                      data-testid="metric-upsell-success-rate-insights"
                    />
                  )}
                </PremiumPanel>
                <PremiumPanel className="flex flex-col px-5 py-6" withTexture>
                  {hasConversationData ? (
                    <HonestMetric
                      label={t("stats.avgMessages")}
                      state="ok"
                      value={Number(
                        formatAvgMessages(
                          insights.total_messages,
                          insights.total_conversations,
                        ),
                      )}
                      format={(n) => n.toFixed(1)}
                      size="lg"
                      data-testid="metric-avg-messages-insights"
                    />
                  ) : (
                    <HonestMetric
                      label={t("stats.avgMessages")}
                      state="empty"
                      value={0}
                      reason={t("stats.noData")}
                      format={(n) => n.toFixed(1)}
                      size="lg"
                      data-testid="metric-avg-messages-insights"
                    />
                  )}
                </PremiumPanel>
              </div>
              <div className="grid grid-cols-1 lg:grid-cols-2 gap-4 mt-4">
                <PremiumPanel className="p-4" withTexture={false}>
                  <p className="text-sm font-semibold text-ink-700 mb-3">
                    {t("stats.conversationsTrend7d")}
                  </p>
                  <TrendChart
                    points={insights.conversation_trends_7d}
                    formatValue={(n) => String(n)}
                    height={200}
                    ariaLabel={t("stats.conversationsTrend7d")}
                    emptyLabel={t("stats.noData")}
                    locale={locale}
                  />
                </PremiumPanel>
                <PremiumPanel className="p-4" withTexture={false}>
                  <p className="text-sm font-semibold text-ink-700 mb-3">
                    {t("stats.busiestHours")}
                  </p>
                  <HourOfDayChart
                    hours={insights.busiest_hours}
                    formatValue={(n) => String(n)}
                    height={200}
                    ariaLabel={t("stats.busiestHours")}
                    emptyLabel={t("stats.noData")}
                  />
                </PremiumPanel>
              </div>
            </div>
          )}
        </DashboardTabTransition>

      {/* Transcript Modal */}
      <Modal
        isOpen={isOpen}
        onClose={handleModalClose}
        size="2xl"
        scrollBehavior="inside"
        isDismissable
      >
        <ModalContent>
          {(onClose) => (
            <>
              <ModalHeader className="flex flex-col gap-1">
                {t("transcript.title")}
                <span className="text-xs font-normal text-ink-500">
                  {t("transcript.session", {
                    id: selectedConversation?.session_id ?? "",
                  })}
                </span>
              </ModalHeader>
              <ModalBody>
                {isLoadingTranscript ? (
                  <div className="flex justify-center p-10">
                    <Spinner />
                  </div>
                ) : transcriptError ? (
                  <div className="flex flex-col items-center gap-3 p-10 text-center">
                    <p className="text-sm text-ink-600">
                      {t("transcript.loadFailed")}
                    </p>
                    <Button
                      size="sm"
                      variant="flat"
                      onPress={() =>
                        selectedConversation &&
                        handleViewTranscript(selectedConversation)
                      }
                    >
                      {t("transcript.retry")}
                    </Button>
                  </div>
                ) : (
                  <div className="space-y-4 p-2">
                    {transcript.map((msg) => (
                      <div
                        key={msg.id}
                        className={`flex ${msg.role === "user" ? "justify-end" : "justify-start"}`}
                      >
                        <div
                          className={`max-w-[80%] p-3 rounded-lg text-sm ${
                            msg.role === "user"
                              ? "bg-brand/10 text-brand rounded-tr-none"
                              : "bg-warm-100 text-ink-800 rounded-tl-none"
                          }`}
                        >
                          <p className="font-semibold text-xs mb-1 opacity-50 uppercase">
                            {msg.role === "user"
                              ? t("transcript.roles.guest")
                              : msg.role === "system"
                                ? t("transcript.roles.system")
                                : aiName}
                            {msg.author_name ? (
                              <span className="ml-1 normal-case text-ink-500">
                                — {msg.author_name}
                                {msg.author_role ? ` · ${msg.author_role}` : ""}
                              </span>
                            ) : null}
                          </p>
                          {renderChatContent(
                            msg.content,
                            allowedImageHosts,
                            stripImageMarkdownInTranscript,
                          )}
                        </div>
                      </div>
                    ))}
                  </div>
                )}
              </ModalBody>
              <ModalFooter className="flex flex-col gap-4">
                {selectedConversation?.status === "closed" ? (
                  <p
                    className="text-sm text-ink-500 w-full"
                    data-testid="transcript-closed-notice"
                  >
                    {t("transcript.conversationClosed")}
                  </p>
                ) : canReplyNow ? (
                  <div className="flex flex-col gap-2 w-full">
                    {claimRemainingMs != null && (
                      <div className="flex justify-end">
                        <Chip
                          size="sm"
                          variant="flat"
                          color={
                            claimRemainingMs <= 60_000 ? "warning" : "default"
                          }
                        >
                          {claimRemainingMs <= 0
                            ? t("transcript.claimExpiredChip")
                            : t("transcript.claimExpiresIn", {
                                time: formatClaimRemaining(claimRemainingMs),
                              })}
                        </Chip>
                      </div>
                    )}
                    <div className="flex gap-2 w-full">
                      <Input
                        placeholder={t("transcript.staffReplyPlaceholder")}
                        value={staffReply}
                        onValueChange={setStaffReply}
                        onKeyDown={(e) => {
                          // Enter-to-send during a live takeover, mirroring the
                          // AIWizard chat. The icon button stays as a secondary
                          // affordance; handleSendReply guards empty/in-flight.
                          if (
                            e.key === "Enter" &&
                            !e.shiftKey &&
                            !isSendingReply
                          ) {
                            e.preventDefault();
                            handleSendReply();
                          }
                        }}
                        size="sm"
                      />
                      <Button
                        size="sm"
                        radius="full"
                        isIconOnly
                        aria-label={t("transcript.sendReply")}
                        className={btnPrimaryNextUI}
                        isLoading={isSendingReply}
                        onPress={handleSendReply}
                      >
                        <Send size={16} />
                      </Button>
                    </div>
                  </div>
                ) : (
                  // Not currently replyable — offer a claim affordance whether the
                  // chat is paused (held by someone else / idle) OR still live on the
                  // AI. Picking up pauses the AI and hands the chat to this staffer.
                  // Closed conversations never reach here (gated above).
                  // (R3-AI-2)
                  <div className="flex items-center justify-between gap-2 w-full">
                    <p className="text-sm text-ink-500">
                      {selectedConversation?.is_paused
                        ? t("transcript.pickUpToReply")
                        : t("transcript.pickUpToReplyIdle")}
                    </p>
                    {selectedConversation ? (
                      <Button
                        size="sm"
                        radius="full"
                        className={btnPrimaryNextUI}
                        onPress={() => handleClaim(selectedConversation)}
                      >
                        {t("monitor.pickUp")}
                      </Button>
                    ) : null}
                  </div>
                )}
                <div className="flex w-full justify-between items-center">
                  {/* Closed is terminal — offering close/pause again would
                      re-arm a finished guest session (L4-7). */}
                  {aiCaps.canClose &&
                  selectedConversation?.status !== "closed" ? (
                    <Button
                      color="danger"
                      variant="light"
                      onPress={() => setShowCloseConfirm(true)}
                      isLoading={isClosingConversation}
                      size="sm"
                    >
                      {t("transcript.closeConversation")}
                    </Button>
                  ) : (
                    <span />
                  )}
                  <div className="flex gap-2">
                    {selectedConversation?.status !== "closed" && (
                      <Button
                        color={
                          selectedConversation?.is_paused
                            ? "success"
                            : "warning"
                        }
                        variant="flat"
                        size="sm"
                        startContent={
                          selectedConversation?.is_paused ? (
                            <Play size={16} />
                          ) : (
                            <Pause size={16} />
                          )
                        }
                        onPress={handlePauseAi}
                      >
                        {selectedConversation?.is_paused
                          ? t("transcript.resumeAi")
                          : t("transcript.pauseAi")}
                      </Button>
                    )}
                    <Button
                      color="primary"
                      variant="light"
                      size="sm"
                      radius="full"
                      onPress={handleModalClose}
                    >
                      {t("transcript.close")}
                    </Button>
                  </div>
                </div>
              </ModalFooter>
            </>
          )}
        </ModalContent>
      </Modal>

      <ConfirmationModal
        isOpen={showBusinessPageRedirectConfirm}
        onOpenChange={() => setShowBusinessPageRedirectConfirm(false)}
        title={t("businessPageRedirect.title")}
        description={t("businessPageRedirect.description")}
        cancelLabel={t("businessPageRedirect.cancel")}
        confirmLabel={t("businessPageRedirect.confirm")}
        onConfirm={() => {
          // Slug-first (getBusinessUrl prefers business_id) so the numeric
          // id never lands in the URL bar. Targets the real BusinessPageEditor
          // mount (?tab=business-page); the old /design route 404'd.
          router.push(getBusinessPageEditorPath(business));
        }}
      />

      <ConfirmationModal
        isOpen={showCloseConfirm}
        onOpenChange={() => setShowCloseConfirm(false)}
        title={t("transcript.closeConfirmTitle")}
        description={t("transcript.closeConfirmDescription")}
        cancelLabel={t("transcript.closeConfirmCancel")}
        confirmLabel={t("transcript.closeConfirmConfirm")}
        isDanger
        onConfirm={handleCloseConversation}
      />

      <ConfirmationModal
        isOpen={stealTarget !== null}
        onOpenChange={() => setStealTarget(null)}
        title={t("monitor.stealConfirm")}
        description={t("monitor.stealRequired", {
          name: stealTarget?.name ?? "",
        })}
        confirmLabel={t("monitor.stealConfirm")}
        cancelLabel={t("monitor.stealCancel")}
        isDanger
        onConfirm={async () => {
          if (!stealTarget) return;
          const target = stealTarget;
          setStealTarget(null);
          await handleClaim(target.conv, { steal: true });
        }}
      />

      <ConfirmationModal
        isOpen={showUpsellConfirm}
        onOpenChange={() => setShowUpsellConfirm(false)}
        title={t("settings.upsellingConfirmTitle")}
        description={t("settings.upsellingConfirmDescription")}
        cancelLabel={t("settings.upsellingConfirmCancel")}
        confirmLabel={t("settings.upsellingConfirmConfirm")}
        onConfirm={() => {
          setAiPriority("upselling");
        }}
      />
    </DashboardTabShell>
  );
}
