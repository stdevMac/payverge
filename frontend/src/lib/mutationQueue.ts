export interface QueuedMutation {
  id: string;
  method: string;
  url: string;
  body: unknown;
  timestamp: number;
  retries: number;
  principalId?: string;
}

const MAX_QUEUE_SIZE = 50;
const MAX_RETRIES = 3;
const MAX_AGE_MS = 24 * 60 * 60 * 1000; // 24 hours

let _activeUserId = "";
export function setActiveUserId(id: string) { _activeUserId = id; }
export function getActiveUserId() { return _activeUserId; }

function getKey(userId: string) {
  return `payverge:mutation-queue:${userId}`;
}

export function getQueue(userId: string): QueuedMutation[] {
  if (typeof window === "undefined") return [];
  try {
    const raw = localStorage.getItem(getKey(userId));
    if (!raw) return [];
    const items: QueuedMutation[] = JSON.parse(raw);
    const now = Date.now();
    return items.filter((item) => now - item.timestamp < MAX_AGE_MS);
  } catch {
    return [];
  }
}

function saveQueue(userId: string, queue: QueuedMutation[]): boolean {
  try {
    localStorage.setItem(getKey(userId), JSON.stringify(queue));
    return true;
  } catch {
    return false;
  }
}

export function enqueue(
  userId: string,
  mutation: Omit<QueuedMutation, "id" | "timestamp" | "retries">,
): QueuedMutation | null {
  if (!userId) return null;
  const queue = getQueue(userId);
  const entry: QueuedMutation = {
    ...mutation,
    id: crypto.randomUUID(),
    timestamp: Date.now(),
    retries: 0,
    principalId: userId,
  };
  queue.push(entry);
  while (queue.length > MAX_QUEUE_SIZE) {
    queue.shift();
  }
  if (!saveQueue(userId, queue)) return null;
  return entry;
}

function removeMutation(userId: string, mutationId: string) {
  const queue = getQueue(userId).filter((m) => m.id !== mutationId);
  saveQueue(userId, queue);
}

function incrementRetry(userId: string, mutationId: string): boolean {
  const queue = getQueue(userId);
  const item = queue.find((m) => m.id === mutationId);
  if (!item) return false;
  item.retries += 1;
  if (item.retries > MAX_RETRIES) {
    saveQueue(userId, queue.filter((m) => m.id !== mutationId));
    return false;
  }
  saveQueue(userId, queue);
  return true;
}

export function clearAllMutationQueues() {
  if (typeof window === "undefined") return;
  const keys: string[] = [];
  for (let i = 0; i < localStorage.length; i += 1) {
    const key = localStorage.key(i);
    if (key?.startsWith("payverge:mutation-queue:")) {
      keys.push(key);
    }
  }
  for (const key of keys) {
    localStorage.removeItem(key);
  }
}

export function principalQueueId(
  oauthUserId?: number | null,
  staffId?: number | string | null,
  walletAddress?: string | null,
): string {
  if (oauthUserId) return `user:${oauthUserId}`;
  if (staffId) return `staff:${staffId}`;
  if (walletAddress) return `wallet:${walletAddress.toLowerCase()}`;
  return "";
}

export async function drainQueue(
  userId: string,
  executor: (mutation: QueuedMutation) => Promise<boolean>,
): Promise<{ succeeded: number; failed: number }> {
  if (!userId) return { succeeded: 0, failed: 0 };
  const queue = getQueue(userId).filter(
    (mutation) => !mutation.principalId || mutation.principalId === userId,
  );
  const rejected = getQueue(userId).filter(
    (mutation) => mutation.principalId && mutation.principalId !== userId,
  );
  for (const mutation of rejected) {
    removeMutation(userId, mutation.id);
  }
  let succeeded = 0;
  let failed = 0;

  for (const mutation of queue) {
    try {
      const ok = await executor(mutation);
      if (ok) {
        removeMutation(userId, mutation.id);
        succeeded++;
      } else {
        const canRetry = incrementRetry(userId, mutation.id);
        if (!canRetry) failed++;
      }
    } catch {
      const canRetry = incrementRetry(userId, mutation.id);
      if (!canRetry) failed++;
    }
  }

  return { succeeded, failed };
}
