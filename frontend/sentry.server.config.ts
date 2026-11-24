import * as Sentry from "@sentry/nextjs";

import {
  getFrontendSentryRuntimeConfig,
  scrubSentryEvent,
  scrubSentryLog,
  scrubSentryTransaction,
  SENTRY_DATA_COLLECTION,
} from "./src/lib/sentry/config";

const sentryConfig = getFrontendSentryRuntimeConfig();

if (sentryConfig.enabled) {
  Sentry.init({
    dsn: sentryConfig.dsn,
    environment: sentryConfig.environment,
    release: sentryConfig.release,
    dataCollection: SENTRY_DATA_COLLECTION,
    tracesSampleRate: sentryConfig.tracesSampleRate,
    beforeSend: scrubSentryEvent,
    beforeSendTransaction: scrubSentryTransaction,
    beforeSendLog: scrubSentryLog,
  });
}
