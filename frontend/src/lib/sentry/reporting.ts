import * as Sentry from "@sentry/nextjs";
import {
  getFrontendSentryRuntimeConfig,
  sanitizeObject,
} from "@/lib/sentry/config";

export type AuthSource = "staff" | "user" | "web3" | "customer";

export interface PayvergeSentryIdentity {
  authSource: AuthSource;
  userId?: number | string;
  staffId?: number | string;
  customerId?: number | string;
  businessId?: number | string;
  walletAddress?: string;
}

export interface CaptureClientErrorInput {
  error: Error | string;
  component: string;
  functionName: string;
  additionalInfo?: Record<string, unknown>;
  requestId?: string;
}

const PRIMARY_AUTH_SOURCES: AuthSource[] = ["staff", "user", "web3"];
const identityBySource: Partial<Record<AuthSource, PayvergeSentryIdentity>> = {};
let appliedIdentity: PayvergeSentryIdentity | null = null;

function isEnabled(): boolean {
  return getFrontendSentryRuntimeConfig().enabled;
}

export function toError(error: Error | string): Error {
  return error instanceof Error ? error : new Error(error);
}

export function captureClientError({
  error,
  component,
  functionName,
  additionalInfo,
  requestId,
}: CaptureClientErrorInput): void {
  if (!isEnabled()) {
    return;
  }

  Sentry.withScope((scope) => {
    scope.setTag("component", component);
    scope.setTag("function", functionName);

    if (requestId) {
      scope.setTag("request_id", requestId);
    }

    if (additionalInfo) {
      scope.setContext("payverge", sanitizeObject(additionalInfo));
    }

    Sentry.captureException(toError(error));
  });
}

export function setSentryIdentity(identity: PayvergeSentryIdentity): void {
  if (!isEnabled()) {
    return;
  }

  if (isGenericCustomerDowngrade(identity)) {
    return;
  }

  if (identity.authSource !== "customer") {
    clearStoredPrimaryIdentities();
  }

  identityBySource[identity.authSource] = identity;
  applyHighestPriorityIdentity();
}

export function clearSentryIdentity(authSource?: AuthSource): void {
  if (!isEnabled()) {
    return;
  }

  if (authSource && !identityBySource[authSource]) {
    return;
  }

  if (authSource) {
    delete identityBySource[authSource];
    applyHighestPriorityIdentity();
    return;
  }

  clearStoredIdentities();
  clearAppliedIdentity();
}

function applyHighestPriorityIdentity(): void {
  const nextIdentity = getHighestPriorityIdentity();

  if (!nextIdentity) {
    clearAppliedIdentity();
    return;
  }

  if (identityKeysMatch(appliedIdentity, nextIdentity)) {
    return;
  }

  appliedIdentity = nextIdentity;
  Sentry.setUser({ id: identityId(nextIdentity) });
  Sentry.setTag("auth_source", nextIdentity.authSource);

  if (nextIdentity.businessId !== undefined) {
    Sentry.setTag("business_id", String(nextIdentity.businessId));
  } else {
    Sentry.setTag("business_id", undefined);
  }

  Sentry.setContext("payverge_identity", buildIdentityContext(nextIdentity));
}

function clearAppliedIdentity(): void {
  appliedIdentity = null;
  Sentry.setUser(null);
  Sentry.setTag("auth_source", undefined);
  Sentry.setTag("business_id", undefined);
  Sentry.setContext("payverge_identity", null);
}

function isGenericCustomerDowngrade(identity: PayvergeSentryIdentity): boolean {
  const storedCustomerIdentity = identityBySource.customer;

  return (
    identity.authSource === "customer" &&
    identity.customerId === undefined &&
    storedCustomerIdentity?.customerId !== undefined
  );
}

function clearStoredPrimaryIdentities(): void {
  for (const source of PRIMARY_AUTH_SOURCES) {
    delete identityBySource[source];
  }
}

function clearStoredIdentities(): void {
  for (const source of [...PRIMARY_AUTH_SOURCES, "customer"] as const) {
    delete identityBySource[source];
  }
}

function getHighestPriorityIdentity(): PayvergeSentryIdentity | null {
  for (const source of PRIMARY_AUTH_SOURCES) {
    if (identityBySource[source]) {
      return identityBySource[source] ?? null;
    }
  }

  return identityBySource.customer ?? null;
}

function identityKeysMatch(
  left: PayvergeSentryIdentity | null,
  right: PayvergeSentryIdentity,
): boolean {
  return left
    ? identityId(left) === identityId(right) &&
        JSON.stringify(buildIdentityContext(left)) ===
          JSON.stringify(buildIdentityContext(right))
    : false;
}

function identityId(identity: PayvergeSentryIdentity): string {
  if (identity.staffId !== undefined) {
    return `staff:${identity.staffId}`;
  }

  if (identity.customerId !== undefined) {
    return `customer:${identity.customerId}`;
  }

  if (identity.userId !== undefined) {
    return `user:${identity.userId}`;
  }

  if (identity.walletAddress) {
    return `web3:${fingerprintString(identity.walletAddress.toLowerCase())}`;
  }

  return identity.authSource;
}

function fingerprintString(value: string): string {
  let firstHash = 0xdeadbeef ^ value.length;
  let secondHash = 0x41c6ce57 ^ value.length;

  for (let index = 0; index < value.length; index += 1) {
    const charCode = value.charCodeAt(index);
    firstHash = Math.imul(firstHash ^ charCode, 2654435761);
    secondHash = Math.imul(secondHash ^ charCode, 1597334677);
  }

  firstHash =
    Math.imul(firstHash ^ (firstHash >>> 16), 2246822507) ^
    Math.imul(secondHash ^ (secondHash >>> 13), 3266489909);
  secondHash =
    Math.imul(secondHash ^ (secondHash >>> 16), 2246822507) ^
    Math.imul(firstHash ^ (firstHash >>> 13), 3266489909);

  return (4294967296 * (2097151 & secondHash) + (firstHash >>> 0)).toString(36);
}

function buildIdentityContext(
  identity: PayvergeSentryIdentity,
): Record<string, number | string> {
  const context: Record<string, number | string> = {
    auth_source: identity.authSource,
  };

  if (identity.userId !== undefined) {
    context.user_id = identity.userId;
  }

  if (identity.staffId !== undefined) {
    context.staff_id = identity.staffId;
  }

  if (identity.customerId !== undefined) {
    context.customer_id = identity.customerId;
  }

  if (identity.businessId !== undefined) {
    context.business_id = identity.businessId;
  }

  return context;
}
