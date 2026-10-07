/** @jest-environment jsdom */
/**
 * #388 — Edit Item must dismiss from Photos & tags (Close / Cancel / Escape).
 *
 * Production: after switching Basics → Photos & tags, every dismiss path
 * leaves the dialog open. Control: Close on Basics still works.
 *
 * Mirrors MenuBuilder: NextUI useDisclosure().onOpenChange is a TOGGLE that
 * ignores the boolean Modal passes. The harness uses that same toggle so a
 * double-close signal (or a swallowed close) is observable as isOpen staying
 * true.
 */
import fs from "fs";
import path from "path";
import React, { useState } from "react";
import { fireEvent, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";

jest.mock("next/image", () => ({
  __esModule: true,
  default: (props: { alt?: string }) => (
    // eslint-disable-next-line @next/next/no-img-element
    <img alt={props.alt || ""} />
  ),
}));
jest.mock("@/api/uploads", () => ({
  uploadFile: jest.fn(),
}));
jest.mock("../../../../api/currency", () => ({
  formatCurrency: (n: number) => `$${(n || 0).toFixed(2)}`,
}));

import type { MenuItemOption } from "@/api/business";
import EditItemModal from "../EditItemModal";

const tString = (k: string) => k;

function modalProps(
  isOpen: boolean,
  onOpenChange: () => void,
  extras: Partial<React.ComponentProps<typeof EditItemModal>> = {},
) {
  return {
    isOpen,
    onOpenChange,
    selectedCategoryIndex: 0,
    menu: [{ id: "c1", name: "Mains", description: "", items: [] }],
    itemName: "Steak Plate",
    setItemName: jest.fn(),
    itemDescription: "Grilled",
    setItemDescription: jest.fn(),
    itemPrice: "12.00",
    setItemPrice: jest.fn(),
    itemCogs: "3.50",
    setItemCogs: jest.fn(),
    defaultCurrency: "USD",
    itemImages: ["https://cdn.example.com/steak.jpg"] as string[],
    setItemImages: jest.fn(),
    itemAvailable: true,
    setItemAvailable: jest.fn(),
    itemSortOrder: 0,
    setItemSortOrder: jest.fn(),
    businessId: 1,
    itemOptions: [] as MenuItemOption[],
    newOptionName: "",
    setNewOptionName: jest.fn(),
    newOptionPrice: "",
    setNewOptionPrice: jest.fn(),
    onAddOption: jest.fn(),
    onRemoveOption: jest.fn(),
    itemAllergens: [] as string[],
    newAllergen: "",
    setNewAllergen: jest.fn(),
    onAddAllergen: jest.fn(),
    onRemoveAllergen: jest.fn(),
    itemDietaryTags: [] as string[],
    newDietaryTag: "",
    setNewDietaryTag: jest.fn(),
    onAddDietaryTag: jest.fn(),
    onRemoveDietaryTag: jest.fn(),
    onUpdateItem: jest.fn(),
    onResetForm: jest.fn(),
    tString,
    onGeneratePhoto: async () => null,
    onGenerateBreakdown: async () => null,
    onEnhancePhoto: async () => null,
    ...extras,
  };
}

/** Same contract as NextUI useDisclosure().onOpenChange — toggle, ignore args. */
function Harness({
  extras = {},
}: {
  extras?: Partial<React.ComponentProps<typeof EditItemModal>>;
}) {
  const [isOpen, setIsOpen] = useState(true);
  const onOpenChange = () => setIsOpen((open) => !open);
  return (
    <>
      <span data-testid="edit-item-open-state">{isOpen ? "open" : "closed"}</span>
      <EditItemModal {...modalProps(isOpen, onOpenChange, extras)} />
    </>
  );
}

function openState() {
  return screen.getByTestId("edit-item-open-state").textContent;
}

function dialog() {
  return screen.getByRole("dialog");
}

function closeButton() {
  return screen.getByRole("button", { name: /^Close$/i });
}

function cancelButton() {
  return screen.getByText("buttons.cancel").closest("button") as HTMLButtonElement;
}

async function openDetails(user: ReturnType<typeof userEvent.setup>) {
  await user.click(screen.getByTestId("edit-item-section-details"));
  expect(screen.getByTestId("edit-item-details-panel")).toBeInTheDocument();
}

describe("EditItemModal dismiss (#388)", () => {
  it("disables NextUI Modal/Tabs animation so a tab switch cannot trap dismiss", () => {
    // NextUI 2.6.11: Tabs layoutId cursor + Modal AnimatePresence leave a
    // blocking overlay after the section changes (heroui-inc/heroui#4561).
    // jsdom cannot see that overlay, so pin the workaround in source.
    const src = fs.readFileSync(
      path.join(__dirname, "../EditItemModal.tsx"),
      "utf8",
    );
    expect(src).toMatch(/<Modal[\s\S]*?disableAnimation/);
    expect(src).toMatch(/<Tabs[\s\S]*?disableAnimation/);
  });

  it("closes from Basics via the Close control (positive control)", async () => {
    const user = userEvent.setup();
    render(<Harness />);
    expect(openState()).toBe("open");
    expect(screen.getByTestId("edit-item-basics-panel")).toBeInTheDocument();

    await user.click(closeButton());

    expect(openState()).toBe("closed");
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
  });

  it("closes from Photos & tags via the Close control", async () => {
    const user = userEvent.setup();
    render(<Harness />);
    await openDetails(user);

    await user.click(closeButton());

    expect(openState()).toBe("closed");
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
  });

  it("closes from Photos & tags via Cancel and resets the form", async () => {
    const user = userEvent.setup();
    const onResetForm = jest.fn();
    render(<Harness extras={{ onResetForm }} />);
    await openDetails(user);

    await user.click(cancelButton());

    expect(onResetForm).toHaveBeenCalledTimes(1);
    expect(openState()).toBe("closed");
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
  });

  it("closes from Photos & tags via Escape", async () => {
    const user = userEvent.setup();
    render(<Harness />);
    await openDetails(user);

    fireEvent.keyDown(dialog(), { key: "Escape", code: "Escape" });

    expect(openState()).toBe("closed");
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
  });

  it("reopens on Basics after a Photos & tags dismiss", async () => {
    const user = userEvent.setup();
    function ReopenHarness() {
      const [isOpen, setIsOpen] = useState(true);
      const onOpenChange = () => setIsOpen((open) => !open);
      return (
        <>
          <button type="button" onClick={() => setIsOpen(true)}>
            reopen-edit
          </button>
          <span data-testid="edit-item-open-state">
            {isOpen ? "open" : "closed"}
          </span>
          <EditItemModal {...modalProps(isOpen, onOpenChange)} />
        </>
      );
    }
    render(<ReopenHarness />);
    await openDetails(user);
    await user.click(cancelButton());
    expect(openState()).toBe("closed");

    await user.click(screen.getByText("reopen-edit"));

    expect(openState()).toBe("open");
    expect(screen.getByTestId("edit-item-basics-panel")).toBeInTheDocument();
    expect(screen.queryByTestId("edit-item-details-panel")).not.toBeInTheDocument();
  });
});
