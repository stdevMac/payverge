import { axiosInstance } from '@/api/index';
import { hasAnalyticsConsent, onConsentChange } from '@/lib/analytics/consentGate';
import { randomUUID } from '@/lib/randomUUID';

// Session management
const SESSION_KEY = 'analytics_session_id';
const SESSION_DURATION = 30 * 60 * 1000; // 30 minutes

interface PageViewData {
  session_id: string;
  page: string;
  referrer: string;
  user_agent: string;
  device_type: string;
  browser: string;
  os: string;
  screen_width: number;
  screen_height: number;
  locale: string;
  timestamp?: string;
}

interface InteractionData {
  session_id: string;
  page: string;
  event_type: string;
  event_category: string;
  event_label: string;
  event_value?: string;
  x_position?: number;
  y_position?: number;
  timestamp?: string;
}

interface ConversionData {
  session_id: string;
  conversion_type: string;
  value?: number;
  metadata?: string;
  timestamp?: string;
}

interface InteractionOptions {
  pageOverride?: string;
}

export const PWA_ANALYTICS_EVENT_NAMES = [
  'pwa_service_worker_registration_error',
  'pwa_appinstalled_observed',
  'pwa_install_card_dismissed',
  'pwa_install_action_clicked',
  'pwa_ios_instructions_viewed',
  'pwa_native_prompt_outcome',
  'pwa_install_card_viewed',
  'pwa_standalone_launched',
  'pwa_launch_resolution_error',
] as const;

export type PwaAnalyticsEventName =
  (typeof PWA_ANALYTICS_EVENT_NAMES)[number];

export interface PwaAnalyticsProps {
  platform_path?:
    | 'installed'
    | 'native-installable'
    | 'manual-install'
    | 'unavailable';
  device_category?: 'mobile' | 'tablet' | 'desktop';
  role_type?: 'owner' | 'staff' | 'unknown';
  outcome?: 'accepted' | 'dismissed';
  destination?: 'chooser' | 'business';
}

const PWA_ANALYTICS_PAGE = '/app';

function normalizePwaProps(
  props?: PwaAnalyticsProps,
): Record<string, string> | undefined {
  if (!props) return undefined;

  const normalized: Record<string, string> = {};
  if (
    props.platform_path === 'installed' ||
    props.platform_path === 'native-installable' ||
    props.platform_path === 'manual-install' ||
    props.platform_path === 'unavailable'
  ) {
    normalized.platform_path = props.platform_path;
  }
  if (
    props.device_category === 'mobile' ||
    props.device_category === 'tablet' ||
    props.device_category === 'desktop'
  ) {
    normalized.device_category = props.device_category;
  }
  if (
    props.role_type === 'owner' ||
    props.role_type === 'staff' ||
    props.role_type === 'unknown'
  ) {
    normalized.role_type = props.role_type;
  }
  if (props.outcome === 'accepted' || props.outcome === 'dismissed') {
    normalized.outcome = props.outcome;
  }
  if (props.destination === 'chooser' || props.destination === 'business') {
    normalized.destination = props.destination;
  }

  return Object.keys(normalized).length > 0 ? normalized : undefined;
}

class AnalyticsTracker {
  private sessionId: string | null = null;
  private pageStartTime: number;
  private currentPage: string;

  constructor() {
    this.pageStartTime = Date.now();
    this.currentPage = '';
    // Clear the session id as soon as analytics consent is declined or
    // withdrawn, not only on the next tracking attempt.
    onConsentChange(() => {
      if (!hasAnalyticsConsent()) this.forgetSession();
    });
  }

  private forgetSession(): void {
    this.sessionId = null;
    if (typeof window === 'undefined') return;
    try {
      sessionStorage.removeItem(SESSION_KEY);
    } catch {
      // Storage can throw when it is disabled; the cached id is already cleared.
    }
  }

  /** Creates or slides the stored session. Called only after consent passes. */
  private getSessionId(): string {
    if (typeof window === 'undefined') {
      if (!this.sessionId) this.sessionId = randomUUID();
      return this.sessionId;
    }

    const now = Date.now();
    let stored: string | null = null;
    try {
      stored = sessionStorage.getItem(SESSION_KEY);
    } catch {
      stored = null;
    }

    if (stored) {
      try {
        const parsed = JSON.parse(stored) as { id?: unknown; timestamp?: unknown };
        if (
          typeof parsed.id === 'string' &&
          parsed.id !== '' &&
          typeof parsed.timestamp === 'number' &&
          now - parsed.timestamp < SESSION_DURATION
        ) {
          try {
            sessionStorage.setItem(
              SESSION_KEY,
              JSON.stringify({ id: parsed.id, timestamp: now }),
            );
          } catch {
            // Keep the id in memory when the refresh write is blocked.
          }
          this.sessionId = parsed.id;
          return parsed.id;
        }
      } catch {
        // Invalid stored data, create a new session below.
      }
    }

    const newId = randomUUID();
    try {
      sessionStorage.setItem(
        SESSION_KEY,
        JSON.stringify({ id: newId, timestamp: now }),
      );
    } catch {
      // sessionStorage unavailable; the in-memory id still scopes this page.
    }
    this.sessionId = newId;
    return newId;
  }

  private getDeviceType(): string {
    if (typeof window === 'undefined') return 'unknown';
    const width = window.innerWidth;
    if (width < 768) return 'mobile';
    if (width < 1024) return 'tablet';
    return 'desktop';
  }

  private getBrowser(): string {
    if (typeof navigator === 'undefined') return 'unknown';
    const ua = navigator.userAgent;
    if (ua.includes('Chrome')) return 'Chrome';
    if (ua.includes('Firefox')) return 'Firefox';
    if (ua.includes('Safari')) return 'Safari';
    if (ua.includes('Edge')) return 'Edge';
    return 'Other';
  }

  private getOS(): string {
    if (typeof navigator === 'undefined') return 'unknown';
    const ua = navigator.userAgent;
    if (ua.includes('Windows')) return 'Windows';
    if (ua.includes('Mac')) return 'macOS';
    if (ua.includes('Linux')) return 'Linux';
    if (ua.includes('Android')) return 'Android';
    if (ua.includes('iOS')) return 'iOS';
    return 'Other';
  }

  // Network emission is gated on cookie consent. Decline-by-default: with no
  // stored choice, readConsent() returns null => no events are sent. The
  // session id is created lazily inside the tracking methods, only after this
  // gate passes. A decline (consent-change event) or a tracking call without
  // consent removes any stored analytics_session_id. No buffering — events before consent are dropped.
  private canTrack(): boolean {
    const allowed = typeof window !== 'undefined' && hasAnalyticsConsent();
    if (!allowed) {
      this.forgetSession();
      return false;
    }
    return true;
  }

  async trackPageView(page: string): Promise<void> {
    if (!this.canTrack()) return;

    // Calculate duration on previous page
    if (this.currentPage) {
      const duration = Math.floor((Date.now() - this.pageStartTime) / 1000);
      await this.updatePageDuration(this.currentPage, duration);
    }

    this.currentPage = page;
    this.pageStartTime = Date.now();

    const data: PageViewData = {
      session_id: this.getSessionId(),
      page,
      referrer: document.referrer || '',
      user_agent: navigator.userAgent,
      device_type: this.getDeviceType(),
      browser: this.getBrowser(),
      os: this.getOS(),
      screen_width: window.innerWidth,
      screen_height: window.innerHeight,
      locale: navigator.language,
    };

    try {
      await axiosInstance.post('/analytics/page-view', data, {
        _skipErrorToast: true,
      } as never);
    } catch (error) {
      console.error('[Analytics] Failed to track page view:', error);
    }
  }

  private async updatePageDuration(page: string, duration: number): Promise<void> {
    // Dwell time is emitted as a page_duration interaction so the backend can
    // roll it into session_summaries.total_duration (AverageSessionTime). The
    // payload shape {duration: seconds} is the contract — do not rename the key.
    if (!Number.isFinite(duration) || duration <= 0) return;
    await this.trackInteraction({
      event_type: 'page_duration',
      event_category: 'engagement',
      event_label: page,
      event_value: JSON.stringify({ duration: Math.floor(duration) }),
    });
  }

  async trackInteraction(
    data: Omit<InteractionData, 'session_id' | 'page'>,
    options: InteractionOptions = {},
  ): Promise<void> {
    if (!this.canTrack()) return;

    const fullData: InteractionData = {
      session_id: this.getSessionId(),
      ...data,
      page:
        options.pageOverride ?? (this.currentPage || window.location.pathname),
    };

    try {
      await axiosInstance.post('/analytics/interaction', fullData, {
        _skipErrorToast: true,
      } as never);
    } catch (error) {
      console.error('Failed to track interaction:', error);
    }
  }

  async trackConversion(data: Omit<ConversionData, 'session_id'>): Promise<void> {
    if (!this.canTrack()) return;

    const fullData: ConversionData = {
      session_id: this.getSessionId(),
      ...data,
    };

    try {
      await axiosInstance.post('/analytics/conversion', fullData, {
        _skipErrorToast: true,
      } as never);
    } catch (error) {
      console.error('Failed to track conversion:', error);
    }
  }

  // Convenience methods for common interactions
  trackClick(element: string, category: string = 'button', value?: string): void {
    void this.trackInteraction({
      event_type: 'click',
      event_category: category,
      event_label: element,
      event_value: value,
    });
  }

  trackScroll(section: string, percentage: number): void {
    void this.trackInteraction({
      event_type: 'scroll',
      event_category: 'engagement',
      event_label: section,
      event_value: JSON.stringify({ percentage }),
    });
  }

  trackFormSubmit(formName: string, success: boolean): void {
    void this.trackInteraction({
      event_type: 'form_submit',
      event_category: 'form',
      event_label: formName,
      event_value: JSON.stringify({ success }),
    });
  }

  trackCarouselInteraction(carouselName: string, action: string, slide: number): void {
    void this.trackInteraction({
      event_type: 'carousel',
      event_category: 'interaction',
      event_label: carouselName,
      event_value: JSON.stringify({ action, slide }),
    });
  }

  trackVideoInteraction(videoName: string, action: string, timestamp?: number): void {
    void this.trackInteraction({
      event_type: 'video',
      event_category: 'media',
      event_label: videoName,
      event_value: JSON.stringify({ action, timestamp }),
    });
  }
}

// Singleton instance
let tracker: AnalyticsTracker | null = null;

export const getAnalyticsTracker = (): AnalyticsTracker => {
  if (!tracker) {
    tracker = new AnalyticsTracker();
  }
  return tracker;
};

// Export convenience functions
export const trackPageView = (page: string) => getAnalyticsTracker().trackPageView(page);

/** PostHog-style product events routed through page analytics interaction ingest. */
export const trackEvent = (
  eventName: string,
  props?: Record<string, string | number | boolean>,
): void => {
  void getAnalyticsTracker().trackInteraction({
    event_type: eventName,
    event_category: 'product',
    event_label: eventName,
    event_value: props ? JSON.stringify(props) : undefined,
  });
};

/** PWA funnel events never inherit an authenticated business pathname. */
export const trackPwaEvent = (
  eventName: PwaAnalyticsEventName,
  props?: PwaAnalyticsProps,
): void => {
  const normalizedProps = normalizePwaProps(props);
  void getAnalyticsTracker().trackInteraction(
    {
      event_type: eventName,
      event_category: 'product',
      event_label: eventName,
      event_value: normalizedProps ? JSON.stringify(normalizedProps) : undefined,
    },
    { pageOverride: PWA_ANALYTICS_PAGE },
  );
};
