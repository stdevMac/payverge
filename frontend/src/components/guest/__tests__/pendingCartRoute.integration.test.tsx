/** @jest-environment jsdom */

import React, { StrictMode, useCallback, useState } from "react";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";

import { stagePendingCartNavigation } from "@/components/guest/GuestTableView";
import {
  consumePendingCartIntent,
  getOrCreatePendingCartSessionBinding,
} from "@/components/guest/pendingCartIntent";
import {
  canApplyPendingCartDelta,
  GuestMenuTableScope,
  menuRouteScopeKey,
  PendingCartAppliedNotice,
  PendingCartRouteConsumer,
  resolvePendingCartTarget,
  undoPendingCartDelta,
} from "@/app/t/[tableCode]/menu/_pendingCartRoute";

const SESSION_ID = "11111111-1111-4111-8111-111111111111";
const NONCE = "22222222-2222-4222-8222-222222222222";
const BOWL_ID = "33333333-3333-4333-8333-333333333333";

describe("table landing to localized menu pending-cart route", () => {
  beforeEach(() => sessionStorage.clear());

  it("uses a different canonical remount key when the route changes tables", () => {
    expect(menuRouteScopeKey(" t1 ")).toBe("T1");
    expect(menuRouteScopeKey("t2")).toBe("T2");
    expect(menuRouteScopeKey("t1")).not.toBe(menuRouteScopeKey("t2"));
  });

  it("remounts the real menu table scope so cart/catalog readiness cannot cross tables", () => {
    const mounts: string[] = [];
    function ScopedState({ table }: { table: string }) {
      const [cartCount, setCartCount] = useState(0);
      React.useEffect(() => {
        mounts.push(table);
      }, [table]);
      return (
        <button onClick={() => setCartCount((value) => value + 1)}>
          {table}:{cartCount}
        </button>
      );
    }
    const view = render(
      <GuestMenuTableScope tableCode="T1">
        <ScopedState table="T1" />
      </GuestMenuTableScope>,
    );
    fireEvent.click(screen.getByRole("button", { name: "T1:0" }));
    expect(screen.getByRole("button", { name: "T1:1" })).toBeInTheDocument();
    view.rerender(
      <GuestMenuTableScope tableCode="T2">
        <ScopedState table="T2" />
      </GuestMenuTableScope>,
    );
    expect(screen.getByRole("button", { name: "T2:0" })).toBeInTheDocument();
    expect(mounts).toEqual(["T1", "T2"]);
  });

  it("preserves the stable Bowl ID, applies once, and undoes the exact delta", () => {
    const navigate = jest.fn();
    const ids = [SESSION_ID, NONCE];
    const outcome = stagePendingCartNavigation(
      {
        tableCode: "mesa-7",
        quantity: 1,
        notes: "sin cebolla",
        metadata: { itemType: "menu_item", menuItemId: BOWL_ID },
        navigate,
      },
      {
        now: 1_000,
        storage: sessionStorage,
        randomUUID: () => ids.shift() ?? "",
      },
    );

    expect(outcome).toBe("deferred");
    expect(navigate).toHaveBeenCalledWith("/t/MESA-7/menu");
    expect(navigate.mock.calls[0][0]).not.toContain(BOWL_ID);

    const binding = getOrCreatePendingCartSessionBinding("MESA-7", {
      storage: sessionStorage,
    });
    expect(binding).toEqual({ ok: true, sessionId: SESSION_ID });
    if (!binding.ok) throw new Error("binding should exist");

    const consumed = consumePendingCartIntent("mesa-7", binding.sessionId, {
      now: 1_001,
      storage: sessionStorage,
    });
    expect(consumed.ok).toBe(true);
    if (!consumed.ok) throw new Error("intent should be consumed");

    const target = resolvePendingCartTarget(
      consumed.intent,
      [
        {
          id: "cat-1",
          name: "Platos",
          items: [
            {
              id: BOWL_ID,
              name: "Bowl de cosecha",
              price: 18,
              is_available: true,
            },
            {
              id: "44444444-4444-4444-8444-444444444444",
              name: "Bowl",
              price: 15,
              is_available: true,
            },
          ],
        },
      ] as never,
      [],
      { [BOWL_ID]: { orderable: true } } as never,
    );
    expect(target).toEqual({
      ok: true,
      target: {
        itemType: "menu_item",
        menuItemId: BOWL_ID,
        itemName: "Bowl de cosecha",
        price: 18,
      },
    });
    if (!target.ok) throw new Error("target should resolve");
    if (target.target.itemType !== "menu_item") {
      throw new Error("menu item target expected");
    }

    const applied = [
      {
        itemType: "menu_item" as const,
        menuItemId: target.target.menuItemId,
        name: target.target.itemName,
        price: target.target.price,
        quantity: consumed.intent.quantity,
        specialRequests: consumed.intent.notes,
      },
    ];
    expect(
      undoPendingCartDelta(applied, target.target, consumed.intent),
    ).toEqual([]);

    expect(
      consumePendingCartIntent("mesa-7", binding.sessionId, {
        now: 1_002,
        storage: sessionStorage,
      }),
    ).toEqual({ ok: false, error: "not_found" });
  });

  it.each([
    [
      "duplicate IDs",
      [
        {
          id: "cat-a",
          name: "A",
          items: [{ id: BOWL_ID, name: "Uno", price: 1, is_available: true }],
        },
        {
          id: "cat-b",
          name: "B",
          items: [{ id: BOWL_ID, name: "Dos", price: 2, is_available: true }],
        },
      ],
      {},
    ],
    [
      "unavailable item",
      [
        {
          id: "cat-a",
          name: "A",
          items: [{ id: BOWL_ID, name: "Uno", price: 1, is_available: false }],
        },
      ],
      {},
    ],
    [
      "not orderable item",
      [
        {
          id: "cat-a",
          name: "A",
          items: [{ id: BOWL_ID, name: "Uno", price: 1, is_available: true }],
        },
      ],
      { [BOWL_ID]: { orderable: false } },
    ],
  ])(
    "rejects %s without substituting a same-name item",
    (_case, categories, orderability) => {
      expect(
        resolvePendingCartTarget(
          {
            version: 1,
            tableCode: "MESA-7",
            sessionId: SESSION_ID,
            nonce: NONCE,
            expiresAt: 100_000,
            menuItemId: BOWL_ID,
            quantity: 1,
          },
          categories as never,
          [],
          orderability as never,
        ),
      ).toEqual({ ok: false, error: "item_unavailable" });
    },
  );

  it("waits for readiness, commits once in StrictMode, then shows localized success and exact undo", async () => {
    const ids = [SESSION_ID, NONCE];
    expect(
      stagePendingCartNavigation(
        {
          tableCode: "mesa-7",
          quantity: 1,
          notes: "sin cebolla",
          metadata: { itemType: "menu_item", menuItemId: BOWL_ID },
          navigate: jest.fn(),
        },
        {
          now: 2_000,
          storage: sessionStorage,
          randomUUID: () => ids.shift() ?? "",
        },
      ),
    ).toBe("deferred");

    const addSpy = jest.fn();
    const rejectSpy = jest.fn();
    const categories = [
      {
        id: "cat-1",
        name: "Platos",
        items: [
          {
            id: BOWL_ID,
            name: "Bowl de cosecha",
            price: 18,
            is_available: true,
          },
        ],
      },
    ] as never;

    function Harness({ ready }: { ready: boolean }) {
      const [cart, setCart] = useState<any[]>([]);
      const [undo, setUndo] = useState<null | (() => void)>(null);
      const add = useCallback((target: any, intent: any) => {
        addSpy(target, intent);
        setCart((current) => [
          ...current,
          {
            itemType: target.itemType,
            menuItemId: target.menuItemId,
            name: target.itemName,
            price: target.price,
            quantity: intent.quantity,
            specialRequests: intent.notes,
          },
        ]);
        return true;
      }, []);
      const undoCart = useCallback((target: any, intent: any) => {
        setCart((current) => undoPendingCartDelta(current, target, intent));
      }, []);
      const applied = useCallback(
        (_target: any, _intent: any, undoAction: () => void) => {
          setUndo(() => undoAction);
        },
        [],
      );
      return (
        <>
          <PendingCartRouteConsumer
            tableCode="MESA-7"
            ready={ready}
            categories={categories}
            bundles={[]}
            orderability={{} as never}
            cart={cart}
            addToCart={add}
            undo={undoCart}
            onApplied={applied}
            onRejected={rejectSpy}
            storage={sessionStorage}
            now={2_001}
          />
          <output data-testid="cart-count">
            {cart.reduce((sum, item) => sum + item.quantity, 0)}
          </output>
          {undo ? (
            <div>
              <span>¡Añadido 1x Bowl de cosecha al carrito!</span>
              <button onClick={undo}>Deshacer</button>
            </div>
          ) : null}
        </>
      );
    }

    const view = render(
      <StrictMode>
        <Harness ready={false} />
      </StrictMode>,
    );
    expect(addSpy).not.toHaveBeenCalled();
    view.rerender(
      <StrictMode>
        <Harness ready />
      </StrictMode>,
    );
    await waitFor(() =>
      expect(screen.getByTestId("cart-count")).toHaveTextContent("1"),
    );
    expect(addSpy).toHaveBeenCalledTimes(1);
    expect(
      screen.getByText("¡Añadido 1x Bowl de cosecha al carrito!"),
    ).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Deshacer" }));
    expect(screen.getByTestId("cart-count")).toHaveTextContent("0");
    expect(rejectSpy).not.toHaveBeenCalled();

    view.unmount();
    render(
      <StrictMode>
        <Harness ready />
      </StrictMode>,
    );
    await Promise.resolve();
    expect(addSpy).toHaveBeenCalledTimes(1);
    expect(rejectSpy).not.toHaveBeenCalled();
  });

  it("rejects an aggregate quantity above the cart cap before mutation", () => {
    const intent = {
      version: 1 as const,
      tableCode: "MESA-7",
      sessionId: SESSION_ID,
      nonce: NONCE,
      expiresAt: 100_000,
      menuItemId: BOWL_ID,
      quantity: 10,
    };
    const target = {
      itemType: "menu_item" as const,
      menuItemId: BOWL_ID,
      itemName: "Bowl",
      price: 18,
    };
    expect(
      canApplyPendingCartDelta(
        [
          {
            itemType: "menu_item",
            menuItemId: BOWL_ID,
            name: "Bowl",
            price: 18,
            quantity: 45,
          },
        ],
        target,
        intent,
        50,
      ),
    ).toBe(false);
    expect(
      undoPendingCartDelta(
        [
          {
            itemType: "menu_item",
            menuItemId: BOWL_ID,
            name: "Nombre anterior",
            price: 17,
            quantity: 2,
          },
          {
            itemType: "menu_item",
            menuItemId: BOWL_ID,
            name: "Bowl",
            price: 17,
            quantity: 3,
          },
          {
            itemType: "menu_item",
            menuItemId: BOWL_ID,
            name: "Bowl",
            price: 18,
            quantity: 10,
          },
        ],
        target,
        intent,
      ),
    ).toEqual([
      {
        itemType: "menu_item",
        menuItemId: BOWL_ID,
        name: "Nombre anterior",
        price: 17,
        quantity: 2,
      },
      {
        itemType: "menu_item",
        menuItemId: BOWL_ID,
        name: "Bowl",
        price: 17,
        quantity: 3,
      },
    ]);
  });

  it("cleans the staged intent if navigation throws and rejects malformed entropy", () => {
    const ids = [SESSION_ID, NONCE];
    expect(
      stagePendingCartNavigation(
        {
          tableCode: " mesa-7 ",
          quantity: 1,
          metadata: { itemType: "menu_item", menuItemId: BOWL_ID },
          navigate: () => {
            throw new Error("navigation failed");
          },
        },
        {
          now: 3_000,
          storage: sessionStorage,
          randomUUID: () => ids.shift() ?? "",
        },
      ),
    ).toBe("rejected");
    expect(
      sessionStorage.getItem("payverge_pending_cart_v1:MESA-7"),
    ).toBeNull();

    sessionStorage.clear();
    const badIds = [SESSION_ID, "not-a-uuid"];
    expect(
      stagePendingCartNavigation(
        {
          tableCode: "mesa-7",
          quantity: 1,
          metadata: { itemType: "menu_item", menuItemId: BOWL_ID },
          navigate: jest.fn(),
        },
        {
          now: 3_000,
          storage: sessionStorage,
          randomUUID: () => badIds.shift() ?? "",
        },
      ),
    ).toBe("rejected");
    expect(
      stagePendingCartNavigation(
        {
          tableCode: "mesa-7",
          quantity: 1,
          metadata: { itemType: "menu_item", menuItemId: BOWL_ID },
          navigate: jest.fn(),
        },
        {
          now: 3_000,
          storage: sessionStorage,
          randomUUID: () => "ABCDEFAB-CDEF-4ABC-8DEF-ABCDEFABCDEF",
        },
      ),
    ).toBe("rejected");
    expect(
      stagePendingCartNavigation(
        {
          tableCode: "mesa-7",
          quantity: 1,
          metadata: { itemType: "menu_item", menuItemId: BOWL_ID },
          navigate: jest.fn(),
        },
        {
          now: 3_000,
          storage: sessionStorage,
          randomUUID: () => {
            throw new Error("entropy");
          },
        },
      ),
    ).toBe("rejected");
  });

  it("renders the production localized success notice and invokes undo", () => {
    const undo = jest.fn();
    render(
      <PendingCartAppliedNotice
        message="¡Añadido 1x Bowl de cosecha al carrito!"
        undoLabel="Deshacer"
        onUndo={undo}
      />,
    );
    expect(
      screen.getByText("¡Añadido 1x Bowl de cosecha al carrito!"),
    ).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Deshacer" }));
    expect(undo).toHaveBeenCalledTimes(1);
  });

  it("resolves only one active bundle whose live child items remain orderable", () => {
    const childId = "88888888-8888-4888-8888-888888888888";
    const intent = {
      version: 1 as const,
      tableCode: "MESA-7",
      sessionId: SESSION_ID,
      nonce: NONCE,
      expiresAt: 100_000,
      bundleId: 42,
      quantity: 1,
    };
    const categories = [
      {
        id: "cat",
        name: "Platos",
        items: [{ id: childId, name: "Bowl", price: 18, is_available: true }],
      },
    ] as never;
    const bundle = {
      id: 42,
      name: "Combo Bowl",
      description: "",
      price: 25,
      is_active: true,
      items: [{ menu_item_id: childId, quantity: 1 }],
    };
    expect(
      resolvePendingCartTarget(intent, categories, [bundle] as never, {}),
    ).toEqual({
      ok: true,
      target: {
        itemType: "bundle",
        bundleId: 42,
        itemName: "Combo Bowl",
        price: 25,
      },
    });
    expect(
      resolvePendingCartTarget(
        intent,
        categories,
        [
          {
            ...bundle,
            items: [
              { menu_item_id: childId, quantity: 1 },
              { menu_item_id: childId, quantity: 2 },
            ],
          },
        ] as never,
        {},
      ),
    ).toEqual({
      ok: true,
      target: {
        itemType: "bundle",
        bundleId: 42,
        itemName: "Combo Bowl",
        price: 25,
      },
    });

    const invalidBundles = [
      { ...bundle, is_active: false },
      { ...bundle, items: [] },
      { ...bundle, items: [null] },
      { ...bundle, items: [{ menu_item_id: "missing", quantity: 1 }] },
      { ...bundle, items: [{ menu_item_id: childId, quantity: 0 }] },
    ];
    for (const invalid of invalidBundles) {
      expect(
        resolvePendingCartTarget(intent, categories, [invalid] as never, {}),
      ).toEqual({ ok: false, error: "item_unavailable" });
    }
    expect(
      resolvePendingCartTarget(
        intent,
        categories,
        [bundle, bundle] as never,
        {},
      ),
    ).toEqual({ ok: false, error: "item_unavailable" });
    expect(
      resolvePendingCartTarget(
        intent,
        [
          {
            id: "cat",
            name: "Platos",
            items: [
              { id: childId, name: "Bowl", price: 18, is_available: false },
            ],
          },
        ] as never,
        [bundle] as never,
        {},
      ),
    ).toEqual({ ok: false, error: "item_unavailable" });
    expect(
      resolvePendingCartTarget(
        intent,
        [
          {
            id: "cat",
            name: "Platos",
            items: [
              { id: childId, name: "Bowl 1", price: 18, is_available: true },
              { id: childId, name: "Bowl 2", price: 19, is_available: true },
            ],
          },
        ] as never,
        [bundle] as never,
        {},
      ),
    ).toEqual({ ok: false, error: "item_unavailable" });
  });

  it("catches a throwing cart mutator and reports rejection once across rerenders", async () => {
    const ids = [SESSION_ID, NONCE];
    stagePendingCartNavigation(
      {
        tableCode: "MESA-7",
        quantity: 1,
        metadata: { itemType: "menu_item", menuItemId: BOWL_ID },
        navigate: jest.fn(),
      },
      {
        now: 4_000,
        storage: sessionStorage,
        randomUUID: () => ids.shift() ?? "",
      },
    );
    const reject = jest.fn();
    const props = {
      tableCode: "MESA-7",
      ready: true,
      categories: [
        {
          id: "cat",
          name: "Platos",
          items: [{ id: BOWL_ID, name: "Bowl", price: 18, is_available: true }],
        },
      ] as never,
      bundles: [] as never,
      orderability: {} as never,
      cart: [],
      addToCart: () => {
        throw new Error("mutation failed");
      },
      undo: jest.fn(),
      onApplied: jest.fn(),
      onRejected: reject,
      storage: sessionStorage,
      now: 4_001,
    };
    const view = render(<PendingCartRouteConsumer {...props} />);
    await waitFor(() => expect(reject).toHaveBeenCalledWith("mutation"));
    expect(reject).toHaveBeenCalledTimes(1);
    view.rerender(
      <PendingCartRouteConsumer
        {...props}
        categories={[...props.categories] as never}
      />,
    );
    expect(reject).toHaveBeenCalledTimes(1);
  });

  it("never applies a new-table intent against the prior table catalog or before readiness", async () => {
    const newItemId = "99999999-9999-4999-8999-999999999999";
    const ids = [SESSION_ID, NONCE];
    stagePendingCartNavigation(
      {
        tableCode: "T2",
        quantity: 1,
        metadata: { itemType: "menu_item", menuItemId: newItemId },
        navigate: jest.fn(),
      },
      {
        now: 5_000,
        storage: sessionStorage,
        randomUUID: () => ids.shift() ?? "",
      },
    );
    const add = jest.fn().mockReturnValue(true);
    const common = {
      bundles: [] as never,
      orderability: {} as never,
      cart: [],
      addToCart: add,
      undo: jest.fn(),
      onApplied: jest.fn(),
      onRejected: jest.fn(),
      storage: sessionStorage,
      now: 5_001,
    };
    const oldCatalog = [
      {
        id: "old",
        name: "Old",
        items: [
          { id: newItemId, name: "Old Bowl", price: 1, is_available: true },
        ],
      },
    ] as never;
    const newCatalog = [
      {
        id: "new",
        name: "Nuevo",
        items: [
          { id: newItemId, name: "Bowl nuevo", price: 2, is_available: true },
        ],
      },
    ] as never;
    const view = render(
      <PendingCartRouteConsumer
        {...common}
        tableCode="T1"
        ready
        categories={oldCatalog}
      />,
    );
    expect(add).not.toHaveBeenCalled();
    view.rerender(
      <PendingCartRouteConsumer
        {...common}
        tableCode="T2"
        ready={false}
        categories={oldCatalog}
      />,
    );
    expect(add).not.toHaveBeenCalled();
    view.rerender(
      <PendingCartRouteConsumer
        {...common}
        tableCode="T2"
        ready
        categories={newCatalog}
      />,
    );
    await waitFor(() => expect(add).toHaveBeenCalledTimes(1));
    expect(add.mock.calls[0][0].itemName).toBe("Bowl nuevo");
  });
});
