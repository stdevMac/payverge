import en from "@/i18n/messages/en/aiWaiterDashboard.json";
import es from "@/i18n/messages/es/aiWaiterDashboard.json";
import {
  liveChatsHeaderValue,
  metricsHaveConversationData,
  planAiPriorityChange,
  shouldShowIdleServiceCopy,
} from "../aiWaiterHonesty";

describe("metricsHaveConversationData", () => {
  it("is false for zero, NaN, and negative counts", () => {
    expect(metricsHaveConversationData(0)).toBe(false);
    expect(metricsHaveConversationData(Number.NaN)).toBe(false);
    expect(metricsHaveConversationData(-1)).toBe(false);
  });

  it("is true once at least one conversation exists", () => {
    expect(metricsHaveConversationData(1)).toBe(true);
    expect(metricsHaveConversationData(12)).toBe(true);
  });
});

describe("shouldShowIdleServiceCopy", () => {
  const readyEmpty = {
    aiEnabled: true,
    insightsReady: true,
    insightsError: false,
    totalConversations: 0,
  };

  it("tells the operator nothing is happening when service is on and metrics are 0", () => {
    expect(shouldShowIdleServiceCopy(readyEmpty)).toBe(true);
  });

  it("does not claim idle before insights land or after an insights error", () => {
    expect(
      shouldShowIdleServiceCopy({ ...readyEmpty, insightsReady: false }),
    ).toBe(false);
    expect(
      shouldShowIdleServiceCopy({ ...readyEmpty, insightsError: true }),
    ).toBe(false);
  });

  it("hides idle copy when service is off or conversations exist", () => {
    expect(shouldShowIdleServiceCopy({ ...readyEmpty, aiEnabled: false })).toBe(
      false,
    );
    expect(
      shouldShowIdleServiceCopy({ ...readyEmpty, totalConversations: 3 }),
    ).toBe(false);
  });
});

describe("liveChatsHeaderValue", () => {
  it("omits zero and unknown so the header never shows a proud 0", () => {
    expect(liveChatsHeaderValue(null)).toBeNull();
    expect(liveChatsHeaderValue(undefined)).toBeNull();
    expect(liveChatsHeaderValue(0)).toBeNull();
    expect(liveChatsHeaderValue(Number.NaN)).toBeNull();
  });

  it("returns a positive live count", () => {
    expect(liveChatsHeaderValue(2)).toBe(2);
  });
});

describe("planAiPriorityChange", () => {
  it("requires confirm when switching into upselling", () => {
    expect(planAiPriorityChange("balanced", "upselling")).toBe(
      "confirm-upselling",
    );
    expect(planAiPriorityChange("service", "upselling")).toBe(
      "confirm-upselling",
    );
  });

  it("applies balanced/service immediately and ignores no-ops", () => {
    expect(planAiPriorityChange("upselling", "balanced")).toBe("apply");
    expect(planAiPriorityChange("balanced", "service")).toBe("apply");
    expect(planAiPriorityChange("balanced", "balanced")).toBe("ignore");
    expect(planAiPriorityChange("balanced", "")).toBe("ignore");
    expect(planAiPriorityChange("upselling", "upselling")).toBe("ignore");
  });
});

describe("operator copy does not loosen allergen / 86 guards", () => {
  it("keeps idle and upselling copy explicit in en and es", () => {
    expect(en.shell.serviceOnIdle).toMatch(/no conversations yet/i);
    expect(en.shell.serviceOnIdleDetail).toMatch(/enough data/i);
    expect(en.settings.allergenGuardrail).toMatch(/allergen/i);
    expect(en.settings.upsellingConfirmDescription).toMatch(/86/);
    expect(en.settings.upsellingConfirmDescription).toMatch(/allergen/i);
    expect(es.shell.serviceOnIdle).toMatch(/conversaciones/i);
    expect(es.settings.upsellingConfirmDescription).toMatch(/86/);
    expect(es.settings.upsellingConfirmDescription).toMatch(/alérgenos/i);
  });
});
