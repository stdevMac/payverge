/** @jest-environment jsdom */
const mockFetchDemoTables = jest.fn();
jest.mock("@/api/demo", () => ({
  fetchDemoTables: (...args: unknown[]) => mockFetchDemoTables(...args),
}));
jest.mock("qrcode", () => ({
  __esModule: true,
  default: { toDataURL: jest.fn().mockResolvedValue("data:image/png;base64,QR") },
}));
jest.mock("@/api/tools/instance", () => ({ axiosInstance: { get: jest.fn(() => new Promise(() => {})) } }));

import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import DemoBanner, { DEMO_ENTER_HREF, DEMO_INSTALL_URL } from "../DemoBanner";
import { resetInstanceCacheForTests, setInstanceForTests } from "@/hooks/useInstance";
import { parseInstanceInfo } from "@/lib/instance/instanceInfo";
import { PROJECT_REPO_URL } from "@/config/brand";

function instance(demo: Record<string, unknown>) {
  return parseInstanceInfo({ registration_mode: "closed", features: {}, demo });
}

afterEach(() => {
  resetInstanceCacheForTests();
  mockFetchDemoTables.mockReset();
});

describe("DemoBanner", () => {
  it("renders nothing on a normal install", () => {
    setInstanceForTests(instance({ enabled: true, mode: false }));
    const { container } = render(<DemoBanner />);
    expect(container).toBeEmptyDOMElement();
  });

  it("renders nothing while the instance is unknown", () => {
    setInstanceForTests(null);
    const { container } = render(<DemoBanner />);
    expect(container).toBeEmptyDOMElement();
  });

  it("states the public-demo terms with GitHub and install links", () => {
    setInstanceForTests(instance({ enabled: true, mode: true, reset_utc: "03:00" }));
    render(<DemoBanner />);
    const region = screen.getByRole("region", { name: "Public demo notice" });
    expect(region).toHaveTextContent(
      "Public demo — anyone can see changes, everything resets nightly at 03:00 UTC",
    );
    expect(screen.getByRole("link", { name: "Enter as owner or staff" })).toHaveAttribute(
      "href",
      DEMO_ENTER_HREF,
    );
    expect(DEMO_ENTER_HREF).toBe("/dashboard?auth=signin");
    expect(screen.getByRole("link", { name: "GitHub" })).toHaveAttribute("href", PROJECT_REPO_URL);
    expect(screen.getByRole("link", { name: "Install your own" })).toHaveAttribute("href", DEMO_INSTALL_URL);
    expect(mockFetchDemoTables).not.toHaveBeenCalled();
  });

  it("moves the fixed site header below the banner, and only in demo mode", () => {
    setInstanceForTests(instance({ enabled: true, mode: true, reset_utc: "03:00" }));
    const { unmount } = render(<DemoBanner />);
    const root = document.documentElement;
    expect(root.hasAttribute("data-public-demo")).toBe(true);
    expect(root.style.getPropertyValue("--demo-banner-offset")).toMatch(/^\d+px$/);
    unmount();
    expect(root.hasAttribute("data-public-demo")).toBe(false);
    expect(root.style.getPropertyValue("--demo-banner-offset")).toBe("");

    setInstanceForTests(instance({ enabled: true, mode: false }));
    render(<DemoBanner />);
    expect(root.hasAttribute("data-public-demo")).toBe(false);
  });

  it("shows the configured reset time", () => {
    setInstanceForTests(instance({ enabled: true, mode: true, reset_utc: "04:15" }));
    render(<DemoBanner />);
    expect(screen.getByTestId("demo-banner")).toHaveTextContent("resets nightly at 04:15 UTC");
  });

  it("lists guest table links and QR codes on demand", async () => {
    setInstanceForTests(instance({ enabled: true, mode: true, reset_utc: "03:00" }));
    mockFetchDemoTables.mockResolvedValue([
      { name: "Bodegón Doña Rosa", custom_url: "bodegon", tables: [{ name: "Mesa 1", code: "demo-t1" }] },
    ]);
    render(<DemoBanner />);
    const toggle = screen.getByRole("button", { name: "Try as a guest" });
    expect(toggle).toHaveAttribute("aria-expanded", "false");
    fireEvent.click(toggle);
    expect(toggle).toHaveAttribute("aria-expanded", "true");

    const tableLink = await screen.findByRole("link", { name: "Mesa 1" });
    expect(tableLink).toHaveAttribute("href", "/t/demo-t1");
    expect(screen.getByRole("link", { name: "Storefront" })).toHaveAttribute("href", "/b/bodegon");
    // A fixed label, never the (visitor-editable) venue name.
    expect(screen.getByRole("region", { name: "Demo venue 1" })).toBeInTheDocument();
    expect(screen.queryByText(/Bodegón Doña Rosa/)).not.toBeInTheDocument();
    await waitFor(() =>
      expect(screen.getByAltText("QR code for Mesa 1")).toHaveAttribute("src", "data:image/png;base64,QR"),
    );
  });

  it("says so when the tables cannot load", async () => {
    setInstanceForTests(instance({ enabled: true, mode: true }));
    mockFetchDemoTables.mockRejectedValue(new Error("boom"));
    render(<DemoBanner />);
    fireEvent.click(screen.getByRole("button", { name: "Try as a guest" }));
    expect(await screen.findByText("Could not load the demo tables.")).toBeInTheDocument();
  });
});
