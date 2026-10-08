import fixture from "@/constants/__fixtures__/role-default-permissions.json";
import {
  getReservationCapabilities,
  getKitchenCapabilities,
  getBillPaymentCapabilities,
  getAllowedStaffTabs,
  getAiWaiterCapabilities,
} from "@/utils/staffAuth";

describe("getReservationCapabilities", () => {
  it("grants full access from manager default permissions", () => {
    expect(getReservationCapabilities(fixture.manager)).toEqual({
      canRead: true,
      canCreate: true,
      canEdit: true,
      canDelete: true,
      canManageSettings: true,
    });
  });

  it("grants host edit/delete but not settings", () => {
    expect(getReservationCapabilities(fixture.host)).toEqual({
      canRead: true,
      canCreate: true,
      canEdit: true,
      canDelete: true,
      canManageSettings: false,
    });
  });

  it("grants server only read + create", () => {
    expect(getReservationCapabilities(fixture.server)).toEqual({
      canRead: true,
      canCreate: true,
      canEdit: false,
      canDelete: false,
      canManageSettings: false,
    });
  });

  it("denies kitchen default permissions", () => {
    expect(getReservationCapabilities(fixture.kitchen)).toEqual({
      canRead: false,
      canCreate: false,
      canEdit: false,
      canDelete: false,
      canManageSettings: false,
    });
  });

  it("denies null/undefined/empty permissions", () => {
    const expected = {
      canRead: false,
      canCreate: false,
      canEdit: false,
      canDelete: false,
      canManageSettings: false,
    };
    expect(getReservationCapabilities(null)).toEqual(expected);
    expect(getReservationCapabilities(undefined)).toEqual(expected);
    expect(getReservationCapabilities([])).toEqual(expected);
  });
});

describe("getKitchenCapabilities", () => {
  it("lets manager activate and update kitchen orders", () => {
    expect(getKitchenCapabilities(fixture.manager)).toEqual({
      canActivate: true,
      canUpdateOrderStatus: true,
    });
  });

  it("lets server and kitchen update order status but not activate", () => {
    expect(getKitchenCapabilities(fixture.server)).toEqual({
      canActivate: false,
      canUpdateOrderStatus: true,
    });
    expect(getKitchenCapabilities(fixture.kitchen)).toEqual({
      canActivate: false,
      canUpdateOrderStatus: true,
    });
  });

  it("keeps host read-only in Kitchen", () => {
    expect(getKitchenCapabilities(fixture.host)).toEqual({
      canActivate: false,
      canUpdateOrderStatus: false,
    });
  });

  it("denies null/undefined/empty permissions", () => {
    const expected = {
      canActivate: false,
      canUpdateOrderStatus: false,
    };
    expect(getKitchenCapabilities(null)).toEqual(expected);
    expect(getKitchenCapabilities(undefined)).toEqual(expected);
    expect(getKitchenCapabilities([])).toEqual(expected);
  });
});

describe("getBillPaymentCapabilities", () => {
  it("grants record + view to owner, manager, and server", () => {
    const full = { canViewPayments: true, canRecordPayment: true };
    expect(getBillPaymentCapabilities(fixture.manager, false)).toEqual(full);
    expect(getBillPaymentCapabilities(fixture.server, false)).toEqual(full);
    expect(getBillPaymentCapabilities(null, true)).toEqual(full);
    expect(getBillPaymentCapabilities(undefined, true)).toEqual(full);
    expect(getBillPaymentCapabilities([], true)).toEqual(full);
  });

  it("grants view-only to host and kitchen", () => {
    const readOnly = { canViewPayments: true, canRecordPayment: false };
    expect(getBillPaymentCapabilities(fixture.host, false)).toEqual(readOnly);
    expect(getBillPaymentCapabilities(fixture.kitchen, false)).toEqual(readOnly);
  });

  it("denies empty permissions for non-owners", () => {
    expect(getBillPaymentCapabilities([], false)).toEqual({
      canViewPayments: false,
      canRecordPayment: false,
    });
    expect(getBillPaymentCapabilities(null, false)).toEqual({
      canViewPayments: false,
      canRecordPayment: false,
    });
  });
});

describe("getAllowedStaffTabs feature-off rules", () => {
  it("filters kitchen for server when kitchenOrdersEnabled is false", () => {
    const tabs = getAllowedStaffTabs("server", {
      permissions: fixture.server,
      kitchenOrdersEnabled: false,
    });
    expect(tabs).not.toContain("kitchen");
    expect(tabs).not.toContain("bills");
  });

  it("filters kitchen for host when kitchenOrdersEnabled is false", () => {
    const tabs = getAllowedStaffTabs("host", {
      permissions: fixture.host,
      kitchenOrdersEnabled: false,
    });
    expect(tabs).not.toContain("kitchen");
    expect(tabs).not.toContain("bills");
  });

  it("retains kitchen for kitchen role when kitchenOrdersEnabled is false", () => {
    const tabs = getAllowedStaffTabs("kitchen", {
      permissions: fixture.kitchen,
      kitchenOrdersEnabled: false,
    });
    expect(tabs).toContain("kitchen");
  });

  it("does not filter kitchen for manager when kitchenOrdersEnabled is false", () => {
    const tabs = getAllowedStaffTabs("manager", {
      permissions: fixture.manager,
      kitchenOrdersEnabled: false,
    });
    expect(tabs).toContain("kitchen");
    expect(tabs).toContain("bills");
  });

  it("includes reservations for server", () => {
    expect(
      getAllowedStaffTabs("server", { permissions: fixture.server }),
    ).toContain("reservations");
  });

  it("still includes reservations for host", () => {
    expect(
      getAllowedStaffTabs("host", { permissions: fixture.host }),
    ).toContain("reservations");
  });

  it("does not give kitchen role the reservations tab", () => {
    expect(
      getAllowedStaffTabs("kitchen", { permissions: fixture.kitchen }),
    ).not.toContain("reservations");
  });

  it("fails closed to overview when permissions are omitted", () => {
    expect(getAllowedStaffTabs("manager")).toEqual(["overview"]);
    expect(getAllowedStaffTabs("server")).toEqual(["overview"]);
  });
});

describe("getAllowedStaffTabs — ai-waiter", () => {
  it("grants ai-waiter to manager, server, and host", () => {
    expect(
      getAllowedStaffTabs("manager", { permissions: fixture.manager }),
    ).toContain("ai-waiter");
    expect(
      getAllowedStaffTabs("server", { permissions: fixture.server }),
    ).toContain("ai-waiter");
    expect(
      getAllowedStaffTabs("host", { permissions: fixture.host }),
    ).toContain("ai-waiter");
  });
  it("does not grant ai-waiter to kitchen", () => {
    expect(
      getAllowedStaffTabs("kitchen", { permissions: fixture.kitchen }),
    ).not.toContain("ai-waiter");
  });
  it("keeps ai-waiter for server even when kitchen/orders are off", () => {
    expect(
      getAllowedStaffTabs("server", {
        permissions: fixture.server,
        kitchenOrdersEnabled: false,
      }),
    ).toContain("ai-waiter");
  });
});

describe("getAllowedStaffTabs — cash-register", () => {
  it("grants cash-register to manager and server only", () => {
    expect(
      getAllowedStaffTabs("manager", { permissions: fixture.manager }),
    ).toContain("cash-register");
    expect(
      getAllowedStaffTabs("server", { permissions: fixture.server }),
    ).toContain("cash-register");
    expect(
      getAllowedStaffTabs("host", { permissions: fixture.host }),
    ).not.toContain("cash-register");
    expect(
      getAllowedStaffTabs("kitchen", { permissions: fixture.kitchen }),
    ).not.toContain("cash-register");
  });
});

describe("getAiWaiterCapabilities", () => {
  it("owner gets everything including config", () => {
    expect(getAiWaiterCapabilities(null, true)).toEqual({
      canViewConfig: true,
      canViewConversations: true,
      canViewInsights: true,
      canReply: true,
      canClose: true,
      canForceRelease: true,
    });
  });
  it("manager gets conversations + insights but not config", () => {
    expect(getAiWaiterCapabilities(fixture.manager, false)).toEqual({
      canViewConfig: false,
      canViewConversations: true,
      canViewInsights: true,
      canReply: true,
      canClose: true,
      canForceRelease: true,
    });
  });
  it("server/host get conversations + reply only", () => {
    const expected = {
      canViewConfig: false,
      canViewConversations: true,
      canViewInsights: false,
      canReply: true,
      canClose: false,
      canForceRelease: false,
    };
    expect(getAiWaiterCapabilities(fixture.server, false)).toEqual(expected);
    expect(getAiWaiterCapabilities(fixture.host, false)).toEqual(expected);
  });
  it("kitchen gets nothing", () => {
    expect(getAiWaiterCapabilities(fixture.kitchen, false)).toEqual({
      canViewConfig: false,
      canViewConversations: false,
      canViewInsights: false,
      canReply: false,
      canClose: false,
      canForceRelease: false,
    });
  });
});
