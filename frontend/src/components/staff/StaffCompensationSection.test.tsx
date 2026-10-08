/** @jest-environment jsdom */
import React from "react";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import StaffCompensationSection, {
  type StaffCompensationSectionProps,
} from "./StaffCompensationSection";
import { staffCompensationApi } from "@/api/staffCompensation";

jest.mock("@/api/staffCompensation", () => ({
  staffCompensationApi: {
    getCompensation: jest.fn(),
    updateCompensation: jest.fn(),
  },
}));
jest.mock("react-hot-toast", () => ({
  toast: { success: jest.fn(), error: jest.fn() },
}));

const labels: StaffCompensationSectionProps["labels"] = {
  title: "Compensation",
  subtitle: "Owner-only wage settings",
  employmentType: "Employment type",
  employmentTypes: { unset: "Unset", hourly: "Hourly", salaried: "Salaried" },
  hourlyRate: "Hourly rate",
  annualSalary: "Annual salary",
  save: "Save",
  saved: "Saved",
  saveError: "Could not save",
};

describe("StaffCompensationSection", () => {
  beforeEach(() => {
    (staffCompensationApi.getCompensation as jest.Mock).mockReset();
    (staffCompensationApi.updateCompensation as jest.Mock).mockReset();
  });

  it("loads existing compensation on mount", async () => {
    (staffCompensationApi.getCompensation as jest.Mock).mockResolvedValue({
      employment_type: "hourly",
      hourly_rate: 18.5,
      annual_salary: 0,
    });
    render(
      <StaffCompensationSection businessId="7" staffId={9} labels={labels} />,
    );
    await waitFor(() =>
      expect(staffCompensationApi.getCompensation).toHaveBeenCalledWith("7", "9"),
    );
    expect(screen.getByText("Compensation")).toBeInTheDocument();
  });

  it("saves the entered compensation via updateCompensation", async () => {
    (staffCompensationApi.getCompensation as jest.Mock).mockResolvedValue({
      employment_type: "",
      hourly_rate: 0,
      annual_salary: 0,
    });
    (staffCompensationApi.updateCompensation as jest.Mock).mockResolvedValue({
      employment_type: "hourly",
      hourly_rate: 20,
      annual_salary: 0,
    });
    render(
      <StaffCompensationSection businessId="7" staffId={9} labels={labels} />,
    );
    await waitFor(() =>
      expect(staffCompensationApi.getCompensation).toHaveBeenCalled(),
    );
    const hourly = screen.getByLabelText("Hourly rate");
    fireEvent.change(hourly, { target: { value: "20" } });
    fireEvent.click(screen.getByRole("button", { name: "Save" }));
    await waitFor(() =>
      expect(staffCompensationApi.updateCompensation).toHaveBeenCalledWith(
        "7",
        "9",
        expect.objectContaining({ hourly_rate: 20 }),
      ),
    );
  });
});
