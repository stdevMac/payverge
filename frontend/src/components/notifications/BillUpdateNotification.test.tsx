/** @jest-environment jsdom */
import { render } from "@testing-library/react";
import BillUpdateNotification from "./BillUpdateNotification";

jest.mock("@/i18n/GuestTranslationProvider", () => ({
  useGuestTranslation: () => ({
    t: (key: string) => key,
  }),
}));

jest.mock("@nextui-org/react", () => ({
  Card: ({ children, className }: any) => (
    <div className={className}>{children}</div>
  ),
  CardBody: ({ children, className }: any) => (
    <div className={className}>{children}</div>
  ),
  Button: ({ children }: any) => <button type="button">{children}</button>,
}));

const baseUpdate = {
  billNumber: "B-42",
  timestamp: new Date().toISOString(),
};

describe("BillUpdateNotification static color classes", () => {
  it("emits complete static literal classes (no interpolated bg-${scheme}) for bill_updated", () => {
    const { container } = render(
      <BillUpdateNotification
        update={{ ...baseUpdate, type: "bill_updated" }}
        onDismiss={jest.fn()}
        autoHide={false}
      />,
    );
    const html = container.innerHTML;
    // The previously-purged title color must now be a literal.
    expect(html).toContain("text-default-800");
    expect(html).toContain("bg-default-50");
    expect(html).toContain("border-default-200");
    // No leftover unresolved template syntax.
    expect(html).not.toContain("${");
  });

  it("uses the primary scheme literals for item_added", () => {
    const { container } = render(
      <BillUpdateNotification
        update={{ ...baseUpdate, type: "item_added", itemName: "Latte" }}
        onDismiss={jest.fn()}
        autoHide={false}
      />,
    );
    const html = container.innerHTML;
    expect(html).toContain("text-primary-800");
    expect(html).toContain("bg-primary-50");
  });

  it("uses the warning scheme literals for item_removed", () => {
    const { container } = render(
      <BillUpdateNotification
        update={{ ...baseUpdate, type: "item_removed", itemName: "Latte" }}
        onDismiss={jest.fn()}
        autoHide={false}
      />,
    );
    const html = container.innerHTML;
    expect(html).toContain("text-warning-800");
    expect(html).toContain("bg-warning-50");
  });
});
