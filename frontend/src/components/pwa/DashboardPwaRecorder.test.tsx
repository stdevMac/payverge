/** @jest-environment jsdom */

import React from "react";
import { render } from "@testing-library/react";
import { readFileSync } from "fs";
import { join } from "path";
import DashboardPwaRecorder from "./DashboardPwaRecorder";

let mockRecordDashboardVisit = jest.fn();
let mockPathname = "/business/casa/dashboard";

jest.mock("@/providers/PwaInstallProvider", () => ({
  usePwaInstall: () => ({ recordDashboardVisit: mockRecordDashboardVisit }),
}));

jest.mock("next/navigation", () => ({
  usePathname: () => mockPathname,
}));

describe("DashboardPwaRecorder", () => {
  beforeEach(() => {
    mockPathname = "/business/casa/dashboard";
    mockRecordDashboardVisit = jest.fn();
  });

  it("records only after the principal and business are resolved", () => {
    const view = render(<DashboardPwaRecorder ready={false} />);
    expect(mockRecordDashboardVisit).not.toHaveBeenCalled();

    view.rerender(<DashboardPwaRecorder ready />);
    expect(mockRecordDashboardVisit).toHaveBeenCalledTimes(1);
    expect(mockRecordDashboardVisit).toHaveBeenCalledWith(
      "/business/casa/dashboard",
    );
  });

  it("records a stable ready path once across rerenders and StrictMode effects", () => {
    const view = render(
      <React.StrictMode>
        <DashboardPwaRecorder ready />
      </React.StrictMode>,
    );

    view.rerender(
      <React.StrictMode>
        <DashboardPwaRecorder ready />
      </React.StrictMode>,
    );
    expect(mockRecordDashboardVisit).toHaveBeenCalledTimes(1);
  });

  it("records the same path again when the authenticated identity changes", () => {
    const view = render(<DashboardPwaRecorder ready />);
    expect(mockRecordDashboardVisit).toHaveBeenCalledTimes(1);

    const nextIdentityRecorder = jest.fn();
    mockRecordDashboardVisit = nextIdentityRecorder;
    view.rerender(<DashboardPwaRecorder ready />);

    expect(nextIdentityRecorder).toHaveBeenCalledTimes(1);
    expect(nextIdentityRecorder).toHaveBeenLastCalledWith(
      "/business/casa/dashboard",
    );
  });

  it("records staff home but ignores login, invitations, and malformed paths", () => {
    const view = render(<DashboardPwaRecorder ready />);
    expect(mockRecordDashboardVisit).toHaveBeenLastCalledWith(
      "/business/casa/dashboard",
    );

    mockPathname = "/staff/home";
    view.rerender(<DashboardPwaRecorder ready />);
    expect(mockRecordDashboardVisit).toHaveBeenLastCalledWith("/staff/home");

    for (const pathname of [
      "/staff/login",
      "/staff/accept-invitation",
      "/business/casa/dashboard/extra",
      "/dashboard",
    ]) {
      mockPathname = pathname;
      view.rerender(<DashboardPwaRecorder ready />);
    }
    expect(mockRecordDashboardVisit).toHaveBeenCalledTimes(2);
  });

  it("is mounted behind resolved owner dashboard state", () => {
    const source = readFileSync(
      join(
        process.cwd(),
        "src/app/(shop)/business/[businessId]/dashboard/page.tsx",
      ),
      "utf8",
    );

    expect(source).toContain("<DashboardPwaRecorder");
    expect(source).toContain(
      "authResolved && finalBusiness && !finalLoading && !error",
    );
  });
});
