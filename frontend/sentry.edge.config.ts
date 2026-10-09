import * as Sentry from "@sentry/nextjs";

import {
  getFrontendSentryRuntimeConfig,
  scrubSentryEvent,
  scrubSentryLog,
  scrubSentryTransaction,
} from "./src/lib/sentry/config";

const sentryConfig = getFrontendSentryRuntimeConfig();

if (sentryConfig.enabled) {
  Sentry.init({
    dsn: sentryConfig.dsn,
    environment: sentryConfig.environment,
    release: sentryConfig.release,
    sendDefaultPii: false,
    tracesSampleRate: sentryConfig.tracesSampleRate,
    beforeSend: scrubSentryEvent,
    beforeSendTransaction: scrubSentryTransaction,
    enableLogs: true,
    beforeSendLog: scrubSentryLog,
  });
}
