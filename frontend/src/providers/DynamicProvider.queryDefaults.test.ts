/** @jest-environment node */
// Import from the standalone factory file to avoid pulling in wagmi (ESM-only)
// transitively through DynamicProvider.tsx in the node test environment.
import { createAppQueryClient } from "../lib/createAppQueryClient";

describe("app-wide QueryClient defaults (P-2)", () => {
  it("ships staleTime, refetchOnWindowFocus, and retry defaults", () => {
    const client = createAppQueryClient();
    const queries = client.getDefaultOptions().queries;
    expect(queries?.staleTime).toBe(30_000);
    expect(queries?.refetchOnWindowFocus).toBe(false);
    expect(queries?.retry).toBe(1);
  });
});
