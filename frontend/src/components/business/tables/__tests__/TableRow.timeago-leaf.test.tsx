/** @jest-environment jsdom */
/**
 * #618 — if timeAgo.* is missing, sentenceCaseLeaf is "Hours ago", not a
 * seated-ago time. The column must use the numeric fallback instead.
 */
import { render, screen } from "@testing-library/react";

jest.mock("@/i18n/SimpleTranslationProvider", () => {
  const actual = jest.requireActual("@/i18n/SimpleTranslationProvider");
  const { sentenceCaseLeaf } = jest.requireActual("@/i18n/sentenceCaseLeaf");
  return {
    ...actual,
    getTranslation: (
      key: string,
      locale?: string,
      params?: Record<string, string | number>,
    ) => {
      if (String(key).includes("timeAgo")) {
        return sentenceCaseLeaf(String(key));
      }
      return actual.getTranslation(key, locale, params);
    },
  };
});

jest.mock("next/link", () => ({
  __esModule: true,
  default: ({
    href,
    children,
    ...rest
  }: {
    href: string;
    children: React.ReactNode;
    [key: string]: unknown;
  }) => (
    <a href={href} {...rest}>
      {children}
    </a>
  ),
}));

import { sentenceCaseLeaf } from "@/i18n/sentenceCaseLeaf";
import TableRow from "../TableRow";

it("uses a numeric seated-ago fallback when timeAgo keys are leaves", () => {
  render(
    <table>
      <tbody>
        <TableRow
          table={{
            id: 9,
            name: "Table 9",
            table_code: "AI-T09",
            is_active: true,
            status: "occupied",
            capacity: 4,
            active_bill: {
              id: 9,
              total: 408.55,
              physical_item_quantity: 0,
              created_at: new Date(Date.now() - 130 * 60 * 1000).toISOString(),
            },
            server_name: null,
            next_reservation: null,
            last_seen: null,
          }}
          onSelect={jest.fn()}
          currency="USD"
        />
      </tbody>
    </table>,
  );
  const seated = screen.getByTestId("table-seated-age");
  expect(seated.textContent).toMatch(/^2h$/);
  expect(seated.textContent).not.toBe(sentenceCaseLeaf("timeAgo.hoursAgo"));
});
