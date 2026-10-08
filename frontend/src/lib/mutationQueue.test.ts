/** @jest-environment jsdom */

import {
  clearAllMutationQueues,
  drainQueue,
  enqueue,
  getQueue,
  principalQueueId,
} from "./mutationQueue";

describe("mutationQueue principal isolation", () => {
  beforeEach(() => {
    localStorage.clear();
  });

  it("keys the queue by principal, not by a shared business id", () => {
    expect(principalQueueId(11, 74)).toBe("user:11");
    expect(principalQueueId(undefined, 74)).toBe("staff:74");
    expect(principalQueueId(undefined, undefined, "0xAbC")).toBe("wallet:0xabc");
    expect(principalQueueId()).toBe("");
  });

  it("does not drain operator A's body after operator B mounts the same key", async () => {
    const businessKey = "75";
    enqueue("user:A", { method: "POST", url: "/bills/1/close", body: { from: "A" } });
    localStorage.setItem(
      "payverge:mutation-queue:75",
      JSON.stringify(getQueue("user:A")),
    );

    const sent: unknown[] = [];
    const result = await drainQueue(businessKey, async (mutation) => {
      sent.push(mutation.body);
      return true;
    });

    expect(result.succeeded).toBe(0);
    expect(sent).toEqual([]);
    expect(getQueue(businessKey)).toHaveLength(0);
  });

  it("clears every durable queue on logout", () => {
    enqueue("user:A", { method: "POST", url: "/x", body: { from: "A" } });
    enqueue("user:B", { method: "POST", url: "/y", body: { from: "B" } });
    clearAllMutationQueues();
    expect(getQueue("user:A")).toHaveLength(0);
    expect(getQueue("user:B")).toHaveLength(0);
  });
});
