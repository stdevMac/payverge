/** @jest-environment jsdom */
import React from "react";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import ManageCategoriesPanel from "./ManageCategoriesPanel";
import { accountingApi } from "@/api/accounting";

jest.mock("@/api/accounting", () => ({
  accountingApi: {
    listAccountingCategories: jest.fn(),
    createAccountingCategory: jest.fn(),
    updateAccountingCategory: jest.fn(),
  },
}));

const mockList = accountingApi.listAccountingCategories as jest.Mock;
const mockUpdate = accountingApi.updateAccountingCategory as jest.Mock;

const t = (key: string) => key;

describe("ManageCategoriesPanel (L6-14)", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    mockList.mockResolvedValue({
      defaults: [],
      custom: [
        {
          id: 3,
          key: "wine_club",
          label: "Wine club",
          entry_type: "income",
          source: "custom",
          active: true,
        },
      ],
    });
    mockUpdate.mockResolvedValue({});
  });

  it("archives a custom category via PATCH active:false (no DELETE)", async () => {
    render(<ManageCategoriesPanel businessId="42" t={t} />);
    fireEvent.click(screen.getByText("categories.manage"));
    expect(await screen.findByText("Wine club")).toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: "categories.archive" }));
    await waitFor(() =>
      expect(mockUpdate).toHaveBeenCalledWith("42", 3, { active: false }),
    );
    expect(mockUpdate.mock.calls[0][0]).toBe("42");
  });
});
