/** @jest-environment jsdom */
import React from "react";
import { render, screen, waitFor, fireEvent } from "@testing-library/react";
import {
  AccountEmailPreferences,
  type AccountEmailPreferencesCopy,
} from "./AccountEmailPreferences";
import {
  getEmailNotificationPreferences,
  updateEmailNotificationPreferences,
} from "@/api/notificationPreferences";

jest.mock("@/api/notificationPreferences", () => ({
  getEmailNotificationPreferences: jest.fn(),
  updateEmailNotificationPreferences: jest.fn(),
}));

const mockShowSuccess = jest.fn();
const mockShowError = jest.fn();
jest.mock("@/contexts/ToastContext", () => ({
  useToast: () => ({ showSuccess: mockShowSuccess, showError: mockShowError }),
}));

const mockGet = getEmailNotificationPreferences as jest.Mock;
const mockUpdate = updateEmailNotificationPreferences as jest.Mock;

const copy: AccountEmailPreferencesCopy = {
  sectionTitle: "Email preferences",
  sectionDescription: "Choose which emails we send you.",
  loadingLabel: "Loading…",
  errorMessage: "Couldn't load your preferences.",
  savedToast: "Saved",
  saveErrorToast: "Save failed",
  items: {
    transactional: { label: "Account & transactional", description: "Essential." },
    reports: { label: "Reports", description: "Summaries." },
  },
};

const serverPrefs = {
  email_enabled: true,
  transactional_enabled: true,
  reports_enabled: false,
  news_enabled: false,
  updates_enabled: true,
  security_enabled: true,
  statistics_enabled: false,
};

describe("AccountEmailPreferences", () => {
  beforeEach(() => jest.clearAllMocks());

  it("loads and renders the two toggles reflecting server state", async () => {
    mockGet.mockResolvedValue({ ...serverPrefs });
    render(<AccountEmailPreferences copy={copy} />);

    expect(await screen.findByText("Email preferences")).toBeInTheDocument();
    const reports = screen.getByRole("switch", { name: "Reports" });
    const transactional = screen.getByRole("switch", {
      name: "Account & transactional",
    });
    expect(reports).not.toBeChecked();
    expect(transactional).toBeChecked();
  });

  it("persists the full preference object (untouched flags preserved) on toggle", async () => {
    mockGet.mockResolvedValue({ ...serverPrefs });
    mockUpdate.mockResolvedValue({ ...serverPrefs, reports_enabled: true });
    render(<AccountEmailPreferences copy={copy} />);

    const reports = await screen.findByRole("switch", { name: "Reports" });
    fireEvent.click(reports);

    await waitFor(() =>
      expect(mockUpdate).toHaveBeenCalledWith({
        ...serverPrefs,
        reports_enabled: true,
      }),
    );
    await waitFor(() => expect(mockShowSuccess).toHaveBeenCalled());
  });

  it("reverts and warns when the save fails", async () => {
    mockGet.mockResolvedValue({ ...serverPrefs });
    mockUpdate.mockRejectedValue(new Error("boom"));
    render(<AccountEmailPreferences copy={copy} />);

    const reports = await screen.findByRole("switch", { name: "Reports" });
    fireEvent.click(reports);
    expect(reports).toBeChecked(); // optimistic

    await waitFor(() => expect(mockShowError).toHaveBeenCalled());
    await waitFor(() => expect(reports).not.toBeChecked()); // reverted
  });

  it("does not offer news, updates or security email toggles", async () => {
    mockGet.mockResolvedValue({ ...serverPrefs });
    mockUpdate.mockResolvedValue({ ...serverPrefs, transactional_enabled: false });
    render(<AccountEmailPreferences copy={copy} />);

    const transactional = await screen.findByRole("switch", {
      name: "Account & transactional",
    });
    expect(screen.queryByRole("switch", { name: "Product news" })).toBeNull();
    expect(screen.queryByRole("switch", { name: "Product updates" })).toBeNull();
    expect(screen.queryByRole("switch", { name: "Security alerts" })).toBeNull();

    fireEvent.click(transactional);
    await waitFor(() => expect(mockUpdate).toHaveBeenCalled());
    const payload = mockUpdate.mock.calls[0][0] as typeof serverPrefs;
    expect(payload.news_enabled).toBe(serverPrefs.news_enabled);
    expect(payload.updates_enabled).toBe(serverPrefs.updates_enabled);
    expect(payload.security_enabled).toBe(serverPrefs.security_enabled);
  });

  it("shows an error state when preferences fail to load", async () => {
    mockGet.mockRejectedValue(new Error("nope"));
    render(<AccountEmailPreferences copy={copy} />);

    expect(
      await screen.findByText("Couldn't load your preferences."),
    ).toBeInTheDocument();
  });
});
