/** @jest-environment jsdom */

import React from "react";
import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import {
  parseAssistantResponse,
  type AssistantResponse,
} from "@/types/assistant";
import {
  AssistantMessage,
  type AssistantMessageLabels,
} from "../AssistantMessage";

const labels: AssistantMessageLabels = {
  usedSources: (count) => `Used ${count} sources`,
  sourcesRegion: (count) => `${count} source details`,
  sourceOrigin: (origin) => `Origin: ${origin}`,
  externalSource: (hostname) => `External source: ${hostname}`,
  actions: "Actions",
  steps: "Steps",
  entities: "Related items",
  entityAvailability: (availability) => `Availability: ${availability}`,
  followUps: "Follow-ups",
  notices: "Notices",
  noticeKind: (kind) => `Notice: ${kind}`,
  status: (status) => `Status: ${status}`,
  workflowProgress: ({ id, current, total }) =>
    `Workflow ${id}: step ${current} of ${total}`,
  disabledActionReason: (reason) =>
    reason ? `Unavailable: ${reason}` : "Unavailable",
  renderError: "This answer could not be displayed.",
};

function response(
  overrides: Partial<AssistantResponse> = {},
): AssistantResponse {
  return parseAssistantResponse({
    version: 2,
    response_id: "response-1",
    answer: { format: "markdown", content: "A **useful** answer." },
    sections: [],
    steps: [],
    actions: [],
    sources: [],
    entities: [],
    follow_ups: [],
    workflow: null,
    notices: [],
    status: "complete",
    ...overrides,
  });
}

const readyAction = {
  id: "action-menu",
  type: "navigate" as const,
  label: "Open Menu",
  target: {
    kind: "dashboard_area" as const,
    id: "menu",
    href: "/business/7/dashboard?tab=menu",
  },
  state: "ready",
  confirmation: "none",
  disabled_reason: null,
  expires_at: null,
};

const internalSource = {
  id: "source-dashboard",
  type: "dashboard_guide" as const,
  title: "Menu guide",
  href: "/business/7/dashboard?tab=menu",
  origin: "dashboard_guide",
  retrieved_at: "2026-08-07T12:00:00Z",
};

describe("AssistantMessage", () => {
  it("injects dedicated media beside the exact structured entity", () => {
    const entity = {
      id: "42",
      type: "menu_item" as const,
      display_name: "Harvest Bowl",
      availability: "available",
      source_id: internalSource.id,
    };
    const renderEntityMedia = jest.fn((record) => (
      <div data-testid={`media-${record.type}-${record.id}`}>Plate image</div>
    ));

    render(
      <AssistantMessage
        response={response({ entities: [entity], sources: [internalSource] })}
        labels={labels}
        renderEntityMedia={renderEntityMedia}
      />,
    );

    expect(renderEntityMedia).toHaveBeenCalledTimes(1);
    expect(renderEntityMedia).toHaveBeenCalledWith(entity);
    const card = screen.getByText("Harvest Bowl").closest("li");
    expect(card).not.toBeNull();
    expect(
      within(card as HTMLElement).getByTestId("media-menu_item-42"),
    ).toHaveTextContent("Plate image");
  });

  it("keeps the original non-interactive entity markup when no interaction props are passed", () => {
    // Issue 791 follow-up (GM1): the operator transcript / plain path must be
    // pixel-identical to the pre-791 markup — a bare <p> name row with the
    // original classes, and no data-entity-layout attribute anywhere.
    const entity = {
      id: "42",
      type: "menu_item" as const,
      display_name: "Harvest Bowl",
      availability: "available",
      source_id: internalSource.id,
    };

    const { container } = render(
      <AssistantMessage
        response={response({ entities: [entity], sources: [internalSource] })}
        labels={labels}
      />,
    );

    const name = screen.getByText("Harvest Bowl");
    expect(name.tagName).toBe("P");
    expect(name.className).toBe("font-semibold text-ink-900");
    expect(container.querySelector("[data-entity-layout]")).toBeNull();
  });

  it("stacks the price under the name in the two-column card grid", () => {
    // A long dish name beside its price in a half-width card wrapped a few
    // letters per line ("Gril / led / pro / vol / eta"). Compact cards stack
    // them; the full-width list keeps name and price on one row.
    const entity = {
      id: "42",
      type: "menu_item" as const,
      display_name: "Grilled provoleta cheese",
      availability: "available",
      source_id: internalSource.id,
    };
    const interaction = () => ({ priceLabel: "ARS 9,800.00", onSelect: jest.fn() });

    const { rerender } = render(
      <AssistantMessage
        response={response({ entities: [entity], sources: [internalSource] })}
        labels={labels}
        entityInteraction={interaction}
        compactEntities
      />,
    );
    const compactRow = screen.getByRole("button", { name: "Grilled provoleta cheese" });
    expect(compactRow).toHaveClass("flex-col");
    expect(compactRow).not.toHaveClass("justify-between");

    rerender(
      <AssistantMessage
        response={response({ entities: [entity], sources: [internalSource] })}
        labels={labels}
        entityInteraction={interaction}
      />,
    );
    const listRow = screen.getByRole("button", { name: "Grilled provoleta cheese" });
    expect(listRow).toHaveClass("justify-between");
    expect(listRow).not.toHaveClass("flex-col");
  });

  it("renders rich answer content and localized status", () => {
    render(<AssistantMessage response={response()} labels={labels} />);

    expect(
      screen.getByText("useful", { selector: "strong" }),
    ).toBeInTheDocument();
    expect(screen.getByText("Status: complete")).toBeInTheDocument();
  });

  it("expands a labelled source region with mouse and keyboard controls", async () => {
    const user = userEvent.setup();
    render(
      <AssistantMessage
        response={response({ sources: [internalSource] })}
        labels={labels}
      />,
    );

    const disclosure = screen.getByRole("button", { name: "Used 1 sources" });
    expect(disclosure).toHaveAttribute("aria-expanded", "false");
    expect(
      screen.queryByRole("region", { name: "1 source details" }),
    ).not.toBeInTheDocument();

    disclosure.focus();
    await user.keyboard("{Enter}");
    expect(disclosure).toHaveAttribute("aria-expanded", "true");
    expect(
      screen.getByRole("region", { name: "1 source details" }),
    ).toBeInTheDocument();

    await user.click(disclosure);
    expect(
      screen.queryByRole("region", { name: "1 source details" }),
    ).not.toBeInTheDocument();
  });

  it("keeps sources without href readable and non-clickable", async () => {
    const user = userEvent.setup();
    render(
      <AssistantMessage
        response={response({
          sources: [
            {
              ...internalSource,
              id: "source-menu",
              title: "Harvest Bowl",
              href: null,
            },
          ],
        })}
        labels={labels}
      />,
    );

    await user.click(screen.getByRole("button", { name: "Used 1 sources" }));
    expect(screen.getByText("Harvest Bowl")).toBeInTheDocument();
    expect(
      screen.queryByRole("link", { name: /Harvest Bowl/ }),
    ).not.toBeInTheDocument();
  });

  it("uses trusted application routes for internal sources and protects external sources", async () => {
    const user = userEvent.setup();
    render(
      <AssistantMessage
        response={response({
          sources: [
            internalSource,
            {
              ...internalSource,
              id: "source-docs",
              title: "Partner docs",
              type: "knowledge_entry",
              href: "https://docs.example.com/guide",
              origin: "knowledge_base",
            },
          ],
        })}
        labels={labels}
      />,
    );

    await user.click(screen.getByRole("button", { name: "Used 2 sources" }));
    expect(screen.getByRole("link", { name: "Menu guide" })).toHaveAttribute(
      "href",
      "/business/7/dashboard?tab=menu",
    );
    const external = screen.getByRole("link", { name: /Partner docs/ });
    expect(external).toHaveAttribute("href", "https://docs.example.com/guide");
    expect(external).toHaveAttribute("target", "_blank");
    expect(external).toHaveAttribute("rel", "noopener noreferrer");
    expect(screen.getByText("External source: docs.example.com")).toBeVisible();
    expect(external).toHaveTextContent("↗");
    expect(external.lastElementChild).toHaveClass("ms-1");
  });

  it("links registered public and narrowly registered application source routes", async () => {
    const user = userEvent.setup();
    render(
      <AssistantMessage
        response={response({
          sources: [
            {
              ...internalSource,
              id: "source-register",
              title: "Register",
              href: "/business/register",
            },
            {
              ...internalSource,
              id: "source-es-register",
              title: "Registro",
              href: "/es-ar/business/register",
            },
            {
              ...internalSource,
              id: "source-dashboard-root",
              title: "Dashboard",
              href: "/dashboard?from=assistant#overview",
            },
            internalSource,
          ],
        })}
        labels={labels}
      />,
    );

    await user.click(screen.getByRole("button", { name: "Used 4 sources" }));
    expect(screen.getByRole("link", { name: "Register" })).toHaveAttribute(
      "href",
      "/business/register",
    );
    expect(screen.getByRole("link", { name: "Registro" })).toHaveAttribute(
      "href",
      "/es-ar/business/register",
    );
    expect(screen.getByRole("link", { name: "Dashboard" })).toHaveAttribute(
      "href",
      "/dashboard?from=assistant#overview",
    );
    expect(screen.getByRole("link", { name: "Menu guide" })).toHaveAttribute(
      "href",
      "/business/7/dashboard?tab=menu",
    );
  });

  it.each([
    "/api/health",
    "/admin",
    "/auth/login",
    "/account",
    "/staff/home",
    "/business/7/not-real",
    "/business/7/dashboard%3Ffake",
    "/business%2F7%2Fdashboard",
    "/business%2f7%2fdashboard",
    "/business/7%2Fdashboard",
    "/blog/categories/menu%2Fpricing",
  ])(
    "renders unsupported internal source destination %s inert",
    async (href) => {
      const user = userEvent.setup();
      const unsupported = response({ sources: [internalSource] });
      unsupported.sources[0] = {
        ...unsupported.sources[0],
        title: `Unsupported ${href}`,
        href,
      };
      render(<AssistantMessage response={unsupported} labels={labels} />);

      await user.click(screen.getByRole("button", { name: "Used 1 sources" }));
      expect(screen.getByText(`Unsupported ${href}`)).toBeInTheDocument();
      expect(
        screen.queryByRole("link", { name: `Unsupported ${href}` }),
      ).not.toBeInTheDocument();
    },
  );

  it("defensively refuses an unsafe source href without hiding its title", async () => {
    const user = userEvent.setup();
    const unsafe = response({ sources: [internalSource] });
    unsafe.sources[0].href = "javascript:alert(1)";
    render(<AssistantMessage response={unsafe} labels={labels} />);

    await user.click(screen.getByRole("button", { name: "Used 1 sources" }));
    expect(screen.getByText("Menu guide")).toBeInTheDocument();
    expect(
      screen.queryByRole("link", { name: /Menu guide/ }),
    ).not.toBeInTheDocument();
  });

  it("renders section-scoped records once in deterministic response order", async () => {
    const user = userEvent.setup();
    const actionTwo = {
      ...readyAction,
      id: "action-orders",
      label: "Open Orders",
      target: {
        ...readyAction.target,
        id: "orders",
        href: "/business/7/dashboard?tab=orders",
      },
    };
    const sourceTwo = {
      ...internalSource,
      id: "source-orders",
      title: "Orders guide",
    };
    const entityOne = {
      id: "42",
      type: "menu_item" as const,
      display_name: "Harvest Bowl",
      availability: "available",
      source_id: internalSource.id,
    };
    const entityTwo = {
      ...entityOne,
      id: "43",
      display_name: "Soup",
      source_id: sourceTwo.id,
    };
    render(
      <AssistantMessage
        response={response({
          sections: [
            {
              id: "section-one",
              title: "First area",
              answer: "First details",
              steps: ["First step"],
              action_ids: [actionTwo.id, readyAction.id],
              source_ids: [sourceTwo.id, internalSource.id],
              entity_ids: [entityTwo.id, entityOne.id],
            },
            {
              id: "section-two",
              title: "Second area",
              answer: "Second details",
              steps: [],
              action_ids: [readyAction.id],
              source_ids: [internalSource.id],
              entity_ids: [entityOne.id],
            },
          ],
          actions: [readyAction, actionTwo],
          sources: [internalSource, sourceTwo],
          entities: [entityOne, entityTwo],
        })}
        labels={labels}
      />,
    );

    expect(screen.getAllByRole("button", { name: "Open Menu" })).toHaveLength(
      1,
    );
    expect(screen.getAllByRole("button", { name: "Open Orders" })).toHaveLength(
      1,
    );
    expect(screen.getAllByText("Harvest Bowl")).toHaveLength(1);
    expect(screen.getAllByText("Soup")).toHaveLength(1);
    const menuButton = screen.getByRole("button", { name: "Open Menu" });
    const ordersButton = screen.getByRole("button", { name: "Open Orders" });
    expect(
      menuButton.compareDocumentPosition(ordersButton) &
        Node.DOCUMENT_POSITION_FOLLOWING,
    ).toBeTruthy();

    await user.click(screen.getByRole("button", { name: "Used 2 sources" }));
    const sourceRegion = screen.getByRole("region", {
      name: "2 source details",
    });
    expect(within(sourceRegion).getAllByText("Menu guide")).toHaveLength(1);
    expect(within(sourceRegion).getAllByText("Orders guide")).toHaveLength(1);
  });

  it("renders only unreferenced top-level actions, sources, and entities after sections", async () => {
    const user = userEvent.setup();
    const remainingAction = {
      ...readyAction,
      id: "action-pricing",
      label: "Open Pricing",
      target: { ...readyAction.target, id: "pricing", href: "/business/register" },
    };
    const remainingSource = {
      ...internalSource,
      id: "source-pricing",
      title: "Pricing registry",
      href: "/business/register",
      type: "pricing_registry" as const,
    };
    const sectionEntity = {
      id: "42",
      type: "menu_item" as const,
      display_name: "Harvest Bowl",
      availability: "available",
      source_id: internalSource.id,
    };
    const remainingEntity = {
      ...sectionEntity,
      id: "43",
      display_name: "Soup",
      source_id: remainingSource.id,
    };
    render(
      <AssistantMessage
        response={response({
          sections: [
            {
              id: "section-menu",
              title: "Menu",
              answer: "Menu details",
              steps: [],
              action_ids: [readyAction.id],
              source_ids: [internalSource.id],
              entity_ids: [sectionEntity.id],
            },
          ],
          steps: ["Top-level step"],
          actions: [readyAction, remainingAction],
          sources: [internalSource, remainingSource],
          entities: [sectionEntity, remainingEntity],
        })}
        labels={labels}
      />,
    );

    expect(screen.getByText("Top-level step")).toBeInTheDocument();
    expect(screen.getAllByRole("button", { name: "Open Menu" })).toHaveLength(
      1,
    );
    expect(
      screen.getAllByRole("button", { name: "Open Pricing" }),
    ).toHaveLength(1);
    expect(screen.getAllByText("Harvest Bowl")).toHaveLength(1);
    expect(screen.getAllByText("Soup")).toHaveLength(1);
    expect(
      screen.getAllByRole("button", { name: "Used 1 sources" }),
    ).toHaveLength(2);
    await user.click(
      screen.getAllByRole("button", { name: "Used 1 sources" })[1],
    );
    expect(
      screen.getByRole("link", { name: "Pricing registry" }),
    ).toHaveAttribute("href", "/business/register");
  });

  it("disables non-ready actions, localizes their reason, and dispatches only the exact ready action", async () => {
    const user = userEvent.setup();
    const onAction = jest.fn();
    const disabledAction = {
      ...readyAction,
      id: "action-disabled",
      label: "Unavailable action",
      state: "blocked",
      disabled_reason: "Requires manager access",
    };
    const validated = response({ actions: [readyAction, disabledAction] });
    render(
      <AssistantMessage
        response={validated}
        labels={labels}
        onAction={onAction}
      />,
    );

    const disabled = screen.getByRole("button", { name: "Unavailable action" });
    expect(disabled).toBeDisabled();
    expect(
      screen.getByText("Unavailable: Requires manager access"),
    ).toBeVisible();
    await user.click(disabled);
    await user.click(screen.getByRole("button", { name: "Open Menu" }));
    expect(onAction).toHaveBeenCalledTimes(1);
    expect(onAction).toHaveBeenCalledWith(readyAction);
    expect(onAction.mock.calls[0][0]).toBe(validated.actions[0]);
  });

  it("never navigates when an action is clicked", async () => {
    const user = userEvent.setup();
    const onAction = jest.fn();
    const before = window.location.href;
    render(
      <AssistantMessage
        response={response({ actions: [readyAction] })}
        labels={labels}
        onAction={onAction}
      />,
    );

    await user.click(screen.getByRole("button", { name: "Open Menu" }));
    expect(window.location.href).toBe(before);
    expect(onAction).toHaveBeenCalledWith(readyAction);
  });

  it("passes a required-confirmation action intact to its parent without navigating", async () => {
    const user = userEvent.setup();
    const onAction = jest.fn();
    const validated = response({
      actions: [{ ...readyAction, confirmation: "required" }],
    });
    const before = window.location.href;
    render(
      <AssistantMessage
        response={validated}
        labels={labels}
        onAction={onAction}
      />,
    );

    await user.click(screen.getByRole("button", { name: "Open Menu" }));
    expect(window.location.href).toBe(before);
    expect(onAction).toHaveBeenCalledTimes(1);
    expect(onAction.mock.calls[0][0]).toBe(validated.actions[0]);
    expect(onAction.mock.calls[0][0].confirmation).toBe("required");
  });

  it("renders localized follow-ups and passes the complete object including its prompt", async () => {
    const user = userEvent.setup();
    const onFollowUp = jest.fn();
    const followUp = {
      id: "hide-item",
      label: "How do I hide it?",
      prompt: "How do I hide an unavailable menu item?",
    };
    render(
      <AssistantMessage
        response={response({ follow_ups: [followUp] })}
        labels={labels}
        onFollowUp={onFollowUp}
      />,
    );

    await user.click(screen.getByRole("button", { name: followUp.label }));
    expect(onFollowUp).toHaveBeenCalledWith(followUp);
    expect(onFollowUp.mock.calls[0][0].prompt).toBe(followUp.prompt);
  });

  it("renders localized notices and workflow progress", () => {
    render(
      <AssistantMessage
        response={response({
          notices: [
            {
              id: "notice-1",
              kind: "warning",
              message: "Stock may be delayed.",
            },
          ],
          workflow: { id: "menu-flow", step_index: 1, step_total: 3 },
          status: "degraded",
        })}
        labels={labels}
      />,
    );

    expect(screen.getByText("Notice: warning")).toBeInTheDocument();
    expect(screen.getByText("Stock may be delayed.")).toBeInTheDocument();
    expect(
      screen.getByText("Workflow menu-flow: step 2 of 3"),
    ).toBeInTheDocument();
    expect(screen.getByText("Status: degraded")).toBeInTheDocument();
  });

  it("contains renderer failures at the message boundary", () => {
    const broken = response();
    Object.defineProperty(broken, "answer", {
      get() {
        throw new Error("renderer exploded");
      },
    });
    const consoleError = jest
      .spyOn(console, "error")
      .mockImplementation(() => undefined);

    render(<AssistantMessage response={broken} labels={labels} />);

    expect(screen.getByRole("alert")).toHaveTextContent(labels.renderError);
    consoleError.mockRestore();
  });

  it("keeps the same broken response contained and recovers for a new response identity", () => {
    const broken = response();
    Object.defineProperty(broken, "answer", {
      get() {
        throw new Error("renderer exploded");
      },
    });
    const consoleError = jest
      .spyOn(console, "error")
      .mockImplementation(() => undefined);
    const { rerender } = render(
      <AssistantMessage response={broken} labels={labels} />,
    );

    expect(screen.getByRole("alert")).toHaveTextContent(labels.renderError);
    rerender(<AssistantMessage response={broken} labels={labels} />);
    expect(screen.getByRole("alert")).toHaveTextContent(labels.renderError);

    const recovered = response({
      response_id: "response-2",
      answer: { format: "plain_text", content: "Recovered answer" },
    });
    rerender(<AssistantMessage response={recovered} labels={labels} />);
    expect(screen.queryByRole("alert")).not.toBeInTheDocument();
    expect(screen.getByText("Recovered answer")).toBeInTheDocument();
    consoleError.mockRestore();
  });
});
