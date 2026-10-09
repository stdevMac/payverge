import * as Sentry from "@sentry/nextjs";

import { onPublicEnvReady } from "./src/config/publicConfig";
import {
  hasAnalyticsConsent,
  onConsentChange,
} from "./src/lib/analytics/consentGate";
import {
  getFrontendSentryRuntimeConfig,
  scrubSentryEvent,
  scrubSentryLog,
  scrubSentryReplayRecordingEvent,
  scrubSentryTransaction,
} from "./src/lib/sentry/config";

// Next evaluates this module from its async main chunk, which can run before
// the parser reaches the root layout's window.__PAYVERGE_ENV__ script. The
// DSN and sample rates are runtime config, so init waits for that script.
onPublicEnvReady(initSentry);

function buildReplayIntegration() {
  return Sentry.replayIntegration({
    maskAllText: true,
    maskAllInputs: true,
    blockAllMedia: true,
    mask: [".sentry-mask", "[data-sentry-mask]", "[data-payverge-sensitive]"],
    block: [".sentry-block", "[data-sentry-block]", "[data-payverge-block]"],
    ignore: [
      ".sentry-ignore",
      "[data-sentry-ignore]",
      "[data-payverge-ignore]",
    ],
    beforeAddRecordingEvent: scrubSentryReplayRecordingEvent,
  });
}

// Replay records the DOM, so it is not part of Sentry.init. It is added only
// after analytics consent, stopped when consent is withdrawn, and never built
// when both sample rates are 0 (operators opt in via env).
function installConsentGatedReplay(sessionSampleRate: number): void {
  const replay = buildReplayIntegration();
  let installed = false;
  let running = false;

  const syncReplay = () => {
    if (hasAnalyticsConsent()) {
      if (running) return;
      if (!installed) {
        // addIntegration runs Replay's own sampling on setup.
        Sentry.addIntegration(replay);
        installed = true;
      } else if (sessionSampleRate > 0) {
        // addIntegration is a no-op for an installed integration, so a
        // re-grant after a withdrawal restarts the stopped recorder.
        replay.start();
      } else {
        replay.startBuffering();
      }
      running = true;
      return;
    }
    if (running) {
      running = false;
      try {
        void replay.stop().catch(() => {});
      } catch {
        // Not recording: nothing to stop.
      }
    }
  };

  syncReplay();
  onConsentChange(syncReplay);
}

function initSentry() {
  const sentryConfig = getFrontendSentryRuntimeConfig();
  if (!sentryConfig.enabled) {
    return;
  }
  Sentry.init({
    dsn: sentryConfig.dsn,
    environment: sentryConfig.environment,
    release: sentryConfig.release,
    sendDefaultPii: false,
    tracesSampleRate: sentryConfig.tracesSampleRate,
    replaysSessionSampleRate: sentryConfig.replaysSessionSampleRate,
    replaysOnErrorSampleRate: sentryConfig.replaysOnErrorSampleRate,
    beforeSend: scrubSentryEvent,
    beforeSendTransaction: scrubSentryTransaction,
    enableLogs: true,
    beforeSendLog: scrubSentryLog,
  });

  if (
    sentryConfig.replaysSessionSampleRate === 0 &&
    sentryConfig.replaysOnErrorSampleRate === 0
  ) {
    return;
  }

  installConsentGatedReplay(sentryConfig.replaysSessionSampleRate);
}

export const onRouterTransitionStart = Sentry.captureRouterTransitionStart;
