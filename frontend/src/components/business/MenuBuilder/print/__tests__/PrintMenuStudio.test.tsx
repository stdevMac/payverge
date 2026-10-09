/** @jest-environment jsdom */
import React from "react";
import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { Business, MenuCategory } from "@/api/business";
import type { Dollars } from "@/types/money";
import type { ImageMeasurementsResult } from "../useImageMeasurements";

class ResizeObserverStub {
  observe() {}
  unobserve() {}
  disconnect() {}
}

(
  globalThis as unknown as { ResizeObserver: typeof ResizeObserverStub }
).ResizeObserver = ResizeObserverStub;

const completeMeasurements = {
  status: "complete" as const,
  measurements: {
    "harvest-bowl.jpg": {
      status: "ready" as const,
      width: 1800,
      height: 1400,
    },
  },
};
const mockUseImageMeasurements = jest.fn<
  ImageMeasurementsResult,
  [readonly string[]]
>(() => completeMeasurements);
jest.mock("../useImageMeasurements", () => ({
  useImageMeasurements: (urls: readonly string[]) =>
    mockUseImageMeasurements(urls),
}));

const mockPrint = jest.fn().mockResolvedValue({ status: "complete" });
const mockCancelPrint = jest.fn();
jest.mock("../useMenuPrintWindow", () => ({
  useMenuPrintWindow: () => ({ print: mockPrint, cancel: mockCancelPrint }),
}));

const mockGetMenu = jest.fn();
jest.mock("@/api/business", () => {
  const actual = jest.requireActual("@/api/business");
  return {
    ...actual,
    getMenu: (...args: unknown[]) => mockGetMenu(...args),
  };
});

const mockQrToDataUrl = jest
  .fn()
  .mockResolvedValue(
    "data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJ",
  );
jest.mock("qrcode", () => ({
  __esModule: true,
  default: { toDataURL: (...args: unknown[]) => mockQrToDataUrl(...args) },
}));

import { PrintMenuStudio } from "../PrintMenuStudio";

const tString = (key: string) => key;

function makeBusiness(overrides: Partial<Business> = {}): Business {
  return {
    id: 1,
    owner_address: "0xabc",
    name: "Cafe <Test>",
    logo: "",
    address: {
      street: "1 Main",
      city: "Austin",
      state: "TX",
      postal_code: "78701",
      country: "US",
    },
    settlement_address: "",
    tipping_address: "",
    tax_rate: 0,
    service_fee_rate: 0,
    tax_inclusive: false,
    service_inclusive: false,
    is_active: true,
    custom_url: "cafe-test",
    business_type: "restaurant",
    design_settings: {
      primary_color: "#1a6b6a",
      secondary_color: "#f4f2ed",
    } as Business["design_settings"],
    created_at: "",
    updated_at: "",
    ...overrides,
  };
}

function makeItem(
  name: string,
  price: number,
  extra: Partial<MenuCategory["items"][number]> = {},
): MenuCategory["items"][number] {
  return {
    id: name.toLowerCase().replace(/\s+/g, "-"),
    name,
    description: extra.description ?? `${name} description`,
    price: price as Dollars,
    currency: "USD",
    is_available: true,
    ...extra,
  };
}

function fixtureCategories(itemCount = 3): MenuCategory[] {
  const items = Array.from({ length: itemCount }, (_, index) =>
    makeItem(index === 0 ? "Harvest Bowl" : `Dish ${index + 1}`, 12 + index, {
      description: index === 0 ? "Short" : `Description for dish ${index + 1}`,
      image: index === 0 ? "harvest-bowl.jpg" : undefined,
    }),
  );
  return [
    {
      id: "cat-mains",
      name: "Mains",
      description: "Main courses",
      items,
      sort_order: 0,
    },
  ];
}

const onClose = jest.fn();
const baseProps = {
  isOpen: true,
  onClose,
  business: makeBusiness(),
  categories: fixtureCategories(),
  defaultCurrency: "USD",
  languages: [{ language_code: "en", is_default: true } as never],
  supportedLanguages: [
    { code: "en", name: "English", native_name: "English" } as never,
  ],
  currentViewLanguage: "en",
  defaultLanguage: "en",
  businessId: 1,
  tString,
};

type User = ReturnType<typeof userEvent.setup>;

/** The whole visible flow: choose a look, then personalize and print. */
async function personalize(user: User) {
  await user.click(screen.getByRole("button", { name: "print.next" }));
  expect(
    screen.getByRole("heading", { name: "print.stage.personalize" }),
  ).toBeInTheDocument();
}

async function chooseLook(user: User, familyKey: string) {
  if (!screen.queryByRole("radio", { name: familyKey })) {
    await user.click(
      screen.getByRole("button", { name: "print.look.showAll" }),
    );
  }
  await user.click(screen.getByRole("radio", { name: familyKey }));
}

async function openFineTune(user: User): Promise<HTMLElement> {
  await user.click(screen.getByRole("button", { name: "print.fineTune.open" }));
  return screen.getByTestId("print-fine-tune-panel");
}

async function closeFineTune(user: User) {
  await user.click(screen.getByRole("button", { name: "print.fineTune.done" }));
  await waitFor(() =>
    expect(
      screen.queryByTestId("print-fine-tune-panel"),
    ).not.toBeInTheDocument(),
  );
}

function previewDocument(): string | null {
  return screen.getByTitle("print.previewPage 1").getAttribute("srcdoc");
}

beforeEach(() => {
  jest.clearAllMocks();
  mockUseImageMeasurements.mockReturnValue(completeMeasurements);
  mockGetMenu.mockResolvedValue({ categories: [] });
  mockPrint.mockResolvedValue({ status: "complete" });
  mockQrToDataUrl.mockResolvedValue(
    "data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJ",
  );
});

describe("PrintMenuStudio", () => {
  it("opens on restaurant-specific looks and keeps the choice through the two moments", async () => {
    const user = userEvent.setup();
    render(<PrintMenuStudio {...baseProps} />);

    expect(
      await screen.findByRole("heading", { name: "print.stage.look" }),
    ).toBeInTheDocument();
    expect(screen.queryByText("print.templatesLabel")).not.toBeInTheDocument();
    expect(screen.getByText("1 / 2")).toBeInTheDocument();

    await chooseLook(user, "print.family.gallery");
    await personalize(user);

    expect(screen.getByTestId("print-current-look")).toHaveTextContent(
      "print.family.gallery",
    );
    expect(
      screen.getAllByRole("button", { name: /print.pageThumbnail/ }).length,
    ).toBeGreaterThan(0);

    await user.click(screen.getByRole("button", { name: "print.back" }));
    expect(
      screen.getByRole("radio", { name: "print.family.gallery" }),
    ).toBeChecked();
  });

  it("lands a chosen look's own recommended format instead of silently keeping the prior one", async () => {
    // Regression coverage: the default recommendation is atelier/single-sheet.
    // Gallery ALSO supports single-sheet, so before the fix, selecting
    // gallery from the look picker would silently keep "single-sheet"
    // (the reducer's family-switch fallback only overwrites the format when
    // the new family doesn't support the current one) instead of landing on
    // gallery's own recommended "two-page-spread" format.
    const user = userEvent.setup();
    render(<PrintMenuStudio {...baseProps} />);

    await chooseLook(user, "print.family.gallery");
    await personalize(user);

    const currentLookCard = screen.getByTestId("print-current-look")
      .parentElement as HTMLElement;
    expect(currentLookCard).toHaveTextContent("print.format.twoPageSpread");
    expect(previewDocument()).toContain("format-two-page-spread");
    expect(previewDocument()).not.toContain("format-single-sheet");

    const panel = await openFineTune(user);
    expect(
      within(panel).getByRole("button", {
        name: "print.format.twoPageSpread",
      }),
    ).toHaveAttribute("aria-pressed", "true");
    expect(
      within(panel).getByRole("button", { name: "print.format.singleSheet" }),
    ).toHaveAttribute("aria-pressed", "false");
  });

  it("still allows a manual format override in fine tune after a look selection", async () => {
    // The fix must not make the format sticky to the recommendation — an
    // operator can still override it manually afterward.
    const user = userEvent.setup();
    render(<PrintMenuStudio {...baseProps} />);

    await chooseLook(user, "print.family.gallery");
    await personalize(user);

    const panel = await openFineTune(user);
    await user.click(
      within(panel).getByRole("button", { name: "print.format.singleSheet" }),
    );
    await closeFineTune(user);

    const currentLookCard = screen.getByTestId("print-current-look")
      .parentElement as HTMLElement;
    expect(currentLookCard).toHaveTextContent("print.format.singleSheet");
    expect(previewDocument()).toContain("format-single-sheet");
  });

  it("surfaces a QR toggle in the always-visible side rail, off by default", async () => {
    const user = userEvent.setup();
    render(<PrintMenuStudio {...baseProps} />);

    await personalize(user);

    const sideRail = screen.getByTestId("print-side-rail");
    const qrToggle = within(sideRail).getByTestId("print-side-rail-qr");
    const checkbox = within(qrToggle).getByRole("checkbox", {
      name: "print.qr.include",
    });
    expect(checkbox).not.toBeChecked();
    expect(checkbox).toBeEnabled();
    expect(previewDocument()).not.toContain("iVBORw0KGgoAAAANSUhEUgAAAAEAAAAB");

    await user.click(checkbox);
    expect(checkbox).toBeChecked();
    await waitFor(() => {
      expect(previewDocument()).toContain(
        "iVBORw0KGgoAAAANSUhEUgAAAAEAAAAB",
      );
    });

    // The fine-tune drawer's own QR checkbox reads the same `includeQr`
    // state, so it must reflect the side rail's toggle.
    const panel = await openFineTune(user);
    expect(
      within(panel).getByRole("checkbox", { name: "print.qr.include" }),
    ).toBeChecked();

    await user.click(
      within(panel).getByRole("checkbox", { name: "print.qr.include" }),
    );
    await closeFineTune(user);

    expect(
      within(screen.getByTestId("print-side-rail-qr")).getByRole("checkbox", {
        name: "print.qr.include",
      }),
    ).not.toBeChecked();
  });

  it("shows an actionable link instead of the QR toggle when the business has no custom URL", async () => {
    const user = userEvent.setup();
    render(
      <PrintMenuStudio {...baseProps} business={makeBusiness({ custom_url: "" })} />,
    );

    await personalize(user);

    const sideRail = screen.getByTestId("print-side-rail");
    const qrToggle = within(sideRail).getByTestId("print-side-rail-qr");
    expect(
      within(qrToggle).getByRole("checkbox", { name: "print.qr.include" }),
    ).toBeDisabled();

    const manageLink = within(sideRail).getByRole("link", {
      name: "print.qr.manageCta",
    });
    expect(manageLink).toHaveAttribute(
      "href",
      "/business/1/dashboard?tab=business-page",
    );
  });

  it("gives the live preview the stage and keeps every advanced control behind fine tune", async () => {
    const user = userEvent.setup();
    render(<PrintMenuStudio {...baseProps} />);

    expect(screen.queryByTestId("print-side-rail")).not.toBeInTheDocument();
    await personalize(user);

    expect(screen.getByTestId("print-live-preview")).toBeInTheDocument();
    expect(screen.getByTestId("print-side-rail")).toBeInTheDocument();
    expect(
      screen.queryByRole("button", { name: "print.paper.a4" }),
    ).not.toBeInTheDocument();
    expect(
      screen.queryByRole("button", { name: "print.typography.refined" }),
    ).not.toBeInTheDocument();
    expect(
      screen.queryByRole("checkbox", { name: "Harvest Bowl" }),
    ).not.toBeInTheDocument();

    const panel = await openFineTune(user);
    expect(
      within(panel).getByRole("button", { name: "print.paper.a4" }),
    ).toBeInTheDocument();
    expect(
      within(panel).getByRole("button", { name: "print.typography.refined" }),
    ).toBeInTheDocument();
    expect(
      within(panel).getByRole("checkbox", { name: "Harvest Bowl" }),
    ).toBeInTheDocument();
  });

  it("treats fine tune as a dialog with trapped, restorable focus", async () => {
    const user = userEvent.setup();
    render(<PrintMenuStudio {...baseProps} />);
    await personalize(user);

    const trigger = screen.getByRole("button", { name: "print.fineTune.open" });
    await user.click(trigger);

    const panel = screen.getByRole("dialog", { name: "print.fineTune.title" });
    expect(panel).toHaveAttribute("aria-modal", "true");
    await waitFor(() =>
      expect(
        within(panel).getAllByRole("button", {
          name: "print.fineTune.close",
        })[0],
      ).toHaveFocus(),
    );

    await user.keyboard("{Escape}");
    await waitFor(() =>
      expect(
        screen.queryByTestId("print-fine-tune-panel"),
      ).not.toBeInTheDocument(),
    );
    expect(trigger).toHaveFocus();
  });

  it("renders localized page thumbnails and changes the selected planned page", async () => {
    const user = userEvent.setup();
    render(
      <PrintMenuStudio {...baseProps} categories={fixtureCategories(24)} />,
    );
    await personalize(user);
    const panel = await openFineTune(user);
    await user.click(
      within(panel).getByRole("button", { name: "print.format.twoPageSpread" }),
    );
    await closeFineTune(user);

    const thumbnails = screen.getAllByRole("button", {
      name: /print.pageThumbnail/,
    });
    expect(thumbnails.length).toBeGreaterThan(1);
    await user.click(thumbnails[1]);
    expect(thumbnails[1]).toHaveAttribute("aria-pressed", "true");
    expect(screen.getByTitle("print.previewPage 2")).toBeInTheDocument();
  });

  it("navigates an actionable image note and focuses the affected item inside fine tune", async () => {
    const user = userEvent.setup();
    mockUseImageMeasurements.mockReturnValue({
      status: "complete",
      measurements: {
        "harvest-bowl.jpg": { status: "ready", width: 240, height: 180 },
      },
    });
    render(<PrintMenuStudio {...baseProps} />);
    await personalize(user);

    await user.click(
      await screen.findByRole("button", {
        name: "print.diagnostics.imageLowResolution",
      }),
    );

    const panel = await screen.findByTestId("print-fine-tune-panel");
    await waitFor(() =>
      expect(within(panel).getByText("Harvest Bowl")).toHaveFocus(),
    );

    // The heading only carries its focus-enabling tabindex while focused;
    // leaving it behind would let tagged real controls fall out of the
    // panel's tab order for good.
    const heading = within(panel).getByText("Harvest Bowl");
    expect(heading).toHaveAttribute("tabindex", "-1");
    await user.tab();
    expect(heading).not.toHaveAttribute("tabindex");
  });

  it("focuses a real control from a setting note without dropping it from the tab order", async () => {
    const user = userEvent.setup();
    // A name this long overflows the masthead on every format, so the
    // deterministic `identity:overflow` setting diagnostic always fires.
    const business = makeBusiness({
      name: "The Extraordinarily Long Test Restaurant Name Nobody Could Print "
        .repeat(4)
        .trim(),
    });
    render(<PrintMenuStudio {...baseProps} business={business} />);
    await personalize(user);

    await user.click(
      await screen.findByRole("button", {
        name: "print.diagnostics.chooseLargerFormat",
      }),
    );

    const panel = await screen.findByTestId("print-fine-tune-panel");
    await waitFor(() => {
      const active = document.activeElement as HTMLElement;
      expect(panel.contains(active)).toBe(true);
      expect(active).not.toHaveAttribute("tabindex");
    });
  });

  it("keeps stage content first in a responsive shell", async () => {
    render(<PrintMenuStudio {...baseProps} />);
    const shell = await screen.findByTestId("print-workflow-shell");
    expect(shell).toHaveClass("grid-cols-1");
    expect(shell.className).toContain("md:grid-cols-");
    expect(shell.firstElementChild).toHaveAttribute(
      "data-print-workflow-stage",
      "look",
    );
  });

  it("returns the workflow panel to its top when moving between stages", async () => {
    const user = userEvent.setup();
    render(<PrintMenuStudio {...baseProps} />);
    const main = await screen.findByTestId("print-stage-scroll-region");
    main.scrollTop = 420;

    await user.click(screen.getByRole("button", { name: "print.next" }));

    expect(main.scrollTop).toBe(0);
  });

  it("passes the exact rendered document from preview to the dedicated print window", async () => {
    const user = userEvent.setup();
    render(<PrintMenuStudio {...baseProps} />);
    await personalize(user);

    const renderedDocument = previewDocument();
    expect(renderedDocument?.toLowerCase()).toContain("<!doctype html>");
    expect(renderedDocument).toContain("Cafe &lt;Test&gt;");

    await user.click(screen.getByRole("button", { name: "print.print" }));
    await waitFor(() =>
      expect(mockPrint).toHaveBeenCalledWith(renderedDocument),
    );
    expect(
      document.querySelector('iframe[style*="-9999"]'),
    ).not.toBeInTheDocument();
    expect(
      document.querySelector("iframe[data-print-parent-frame]"),
    ).not.toBeInTheDocument();
  });

  it("carries fine-tuned typography and contact choices into the printed document", async () => {
    const user = userEvent.setup();
    render(<PrintMenuStudio {...baseProps} />);
    await personalize(user);
    const panel = await openFineTune(user);

    await user.click(
      within(panel).getByRole("button", { name: "print.typography.refined" }),
    );
    await user.click(
      within(panel).getByRole("button", { name: "print.contact.none" }),
    );
    await closeFineTune(user);

    const renderedDocument = previewDocument();
    expect(renderedDocument).toContain("typography-refined");
    expect(renderedDocument).toContain("contact-none");
  });

  it("places the photos the operator keeps and drops the ones it removes", async () => {
    const user = userEvent.setup();
    render(<PrintMenuStudio {...baseProps} />);
    await personalize(user);
    expect(previewDocument()).toContain("harvest-bowl.jpg");

    const panel = await openFineTune(user);
    const photo = within(panel).getByRole("checkbox", { name: "Harvest Bowl" });
    expect(photo).toBeChecked();
    await user.click(photo);
    await closeFineTune(user);

    expect(previewDocument()).not.toContain("harvest-bowl.jpg");
  });

  it("supports cancel and repeat printing without closing or discarding the document", async () => {
    const user = userEvent.setup();
    mockPrint
      .mockResolvedValueOnce({ status: "complete" })
      .mockResolvedValueOnce({ status: "complete" });
    render(<PrintMenuStudio {...baseProps} />);
    await personalize(user);
    const printButton = screen.getByRole("button", { name: "print.print" });

    await user.click(printButton);
    await waitFor(() => expect(printButton).toBeEnabled());
    await user.click(printButton);

    await waitFor(() => expect(mockPrint).toHaveBeenCalledTimes(2));
    expect(mockPrint.mock.calls[1][0]).toEqual(mockPrint.mock.calls[0][0]);
    expect(onClose).not.toHaveBeenCalled();
    await user.click(screen.getByRole("button", { name: "print.back" }));
    expect(
      screen.getByRole("heading", { name: "print.stage.look" }),
    ).toBeInTheDocument();
  });

  it.each(["blocked", "timeout", "error", "superseded"] as const)(
    "shows inline %s print feedback and leaves the modal interactive",
    async (status) => {
      const user = userEvent.setup();
      mockPrint.mockResolvedValueOnce({ status });
      render(<PrintMenuStudio {...baseProps} />);
      await personalize(user);

      await user.click(screen.getByRole("button", { name: "print.print" }));

      expect(await screen.findByRole("status")).toHaveTextContent(
        `print.printStatus.${status}`,
      );
      expect(screen.getByRole("button", { name: "print.print" })).toBeEnabled();
      await user.click(screen.getByRole("button", { name: "print.back" }));
      expect(
        screen.getByRole("heading", { name: "print.stage.look" }),
      ).toBeInTheDocument();
      expect(onClose).not.toHaveBeenCalled();
    },
  );

  it("gates printing while image measurements are loading", async () => {
    const user = userEvent.setup();
    mockUseImageMeasurements.mockReturnValue({
      status: "loading",
      measurements: { "harvest-bowl.jpg": { status: "loading" } },
    });
    render(<PrintMenuStudio {...baseProps} />);
    await personalize(user);

    expect(screen.getByRole("button", { name: "print.print" })).toBeDisabled();
    expect(screen.getByRole("status")).toHaveTextContent(
      "print.images.measuring",
    );
  });

  it("gates printing behind a plain-language blocker when the menu has no items", async () => {
    const user = userEvent.setup();
    render(<PrintMenuStudio {...baseProps} categories={[]} />);
    await personalize(user);

    const notes = screen.getByRole("complementary", {
      name: "print.notes.label",
    });
    expect(
      within(notes).getByText("print.diagnostics.blockers"),
    ).toBeInTheDocument();
    expect(within(notes).getByText("print.emptyMenu")).toBeInTheDocument();
    expect(
      screen.queryByText("print.readiness.blocked"),
    ).not.toBeInTheDocument();
    expect(screen.getByRole("button", { name: "print.print" })).toBeDisabled();
  });

  it("summarizes the chosen look and print guidance beside the preview", async () => {
    const user = userEvent.setup();
    render(
      <PrintMenuStudio {...baseProps} categories={fixtureCategories(24)} />,
    );
    await personalize(user);

    const rail = screen.getByTestId("print-side-rail");
    expect(
      within(rail).getByText("print.personalize.currentLook"),
    ).toBeInTheDocument();
    expect(within(rail).getByTestId("print-current-look")).toHaveTextContent(
      /^print\.family\./,
    );
    expect(within(rail).getByText(/print\.format\./)).toBeInTheDocument();
    expect(within(rail).getByText("print.guidance.review")).toBeInTheDocument();
    expect(
      within(rail).getByText("print.guidance.savePdf"),
    ).toBeInTheDocument();
    expect(within(rail).queryByText(/dpi/i)).not.toBeInTheDocument();
  });

  it("retains the last valid menu when a translated menu fails and retries in place", async () => {
    const user = userEvent.setup();
    mockGetMenu
      .mockRejectedValueOnce(new Error("translation unavailable"))
      .mockResolvedValueOnce({
        categories: [
          {
            ...fixtureCategories(1)[0],
            name: "Principales",
            items: [makeItem("Ensalada", 14)],
          },
        ],
      });
    render(
      <PrintMenuStudio
        {...baseProps}
        languages={[
          { language_code: "en", is_default: true } as never,
          { language_code: "es", is_default: false } as never,
        ]}
        supportedLanguages={[
          { code: "en", name: "English", native_name: "English" } as never,
          { code: "es", name: "Spanish", native_name: "Español" } as never,
        ]}
      />,
    );
    await personalize(user);
    expect(previewDocument()).toContain("Harvest Bowl");

    const panel = await openFineTune(user);
    await user.click(within(panel).getByRole("button", { name: "Español" }));

    expect(await screen.findByRole("alert")).toHaveTextContent(
      "print.language.error",
    );
    expect(screen.getByRole("button", { name: "print.print" })).toBeDisabled();
    expect(previewDocument()).toContain("Harvest Bowl");
    await user.click(
      screen.getByRole("button", { name: "print.language.retry" }),
    );

    await waitFor(() => expect(previewDocument()).toContain("Ensalada"));
    expect(mockGetMenu).toHaveBeenCalledTimes(2);
  });

  it("stops requesting image measurements as soon as the studio closes", async () => {
    const { rerender } = render(<PrintMenuStudio {...baseProps} />);

    expect(mockUseImageMeasurements).toHaveBeenLastCalledWith([
      "harvest-bowl.jpg",
    ]);
    rerender(<PrintMenuStudio {...baseProps} isOpen={false} />);

    await waitFor(() =>
      expect(mockUseImageMeasurements).toHaveBeenLastCalledWith([]),
    );
  });

  it("fetches another menu language without letting stale work replace the active menu", async () => {
    const user = userEvent.setup();
    let resolveSpanish: ((value: unknown) => void) | undefined;
    mockGetMenu.mockImplementation(
      () =>
        new Promise((resolve) => {
          resolveSpanish = resolve;
        }),
    );
    render(
      <PrintMenuStudio
        {...baseProps}
        languages={[
          { language_code: "en", is_default: true } as never,
          { language_code: "es", is_default: false } as never,
        ]}
        supportedLanguages={[
          { code: "en", name: "English", native_name: "English" } as never,
          { code: "es", name: "Spanish", native_name: "Español" } as never,
        ]}
      />,
    );
    await personalize(user);
    const panel = await openFineTune(user);
    await user.click(within(panel).getByRole("button", { name: "Español" }));
    expect(mockGetMenu).toHaveBeenCalledWith(1, "es");
    await user.click(within(panel).getByRole("button", { name: "English" }));
    resolveSpanish?.({ categories: fixtureCategories(1) });

    await waitFor(() => {
      expect(
        within(screen.getByTestId("print-fine-tune-panel")).getByRole(
          "button",
          {
            name: "English",
          },
        ),
      ).toHaveAttribute("aria-pressed", "true");
    });
  });

  it("prunes default photos when their category is removed without crashing", async () => {
    const user = userEvent.setup();
    const categories: MenuCategory[] = [
      {
        id: "cat-starters",
        name: "Starters",
        description: "To begin",
        sort_order: 0,
        items: [makeItem("Crispy Squash", 11, { image: "crispy-squash.jpg" })],
      },
      {
        id: "cat-mains",
        name: "Mains",
        description: "Main courses",
        sort_order: 1,
        items: [makeItem("Roast Trout", 24, { image: "roast-trout.jpg" })],
      },
    ];
    render(<PrintMenuStudio {...baseProps} categories={categories} />);

    await personalize(user);
    const panel = await openFineTune(user);
    await user.click(within(panel).getByRole("checkbox", { name: "Starters" }));

    expect(within(panel).queryByText("Crispy Squash")).not.toBeInTheDocument();
    expect(within(panel).getByText("Roast Trout")).toBeInTheDocument();
  });

  it("re-seeds photo defaults after a language menu finishes loading", async () => {
    const user = userEvent.setup();
    mockUseImageMeasurements.mockReturnValue({
      status: "complete",
      measurements: {
        "trucha-asada.jpg": {
          status: "ready",
          width: 1800,
          height: 1400,
        },
      },
    });
    mockGetMenu.mockResolvedValue({
      categories: [
        {
          id: "cat-mains",
          name: "Principales",
          description: "Platos principales",
          sort_order: 0,
          items: [makeItem("Trucha Asada", 24, { image: "trucha-asada.jpg" })],
        },
      ],
    });
    render(
      <PrintMenuStudio
        {...baseProps}
        languages={[
          { language_code: "en", is_default: true } as never,
          { language_code: "es", is_default: false } as never,
        ]}
        supportedLanguages={[
          { code: "en", name: "English", native_name: "English" } as never,
          { code: "es", name: "Spanish", native_name: "Español" } as never,
        ]}
      />,
    );

    await personalize(user);
    const panel = await openFineTune(user);
    await user.click(within(panel).getByRole("button", { name: "Español" }));
    await waitFor(() => expect(previewDocument()).toContain("Trucha Asada"));

    expect(
      within(panel).getByRole("checkbox", { name: "Trucha Asada" }),
    ).toBeChecked();
  });
});
