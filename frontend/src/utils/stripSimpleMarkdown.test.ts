import { stripSimpleMarkdown } from "./stripSimpleMarkdown";

describe("stripSimpleMarkdown PG-15.1", () => {
  it("removes bold asterisks while keeping the words", () => {
    expect(stripSimpleMarkdown("Try the **pad thai** tonight")).toBe(
      "Try the pad thai tonight",
    );
  });

  it("removes single-asterisk emphasis", () => {
    expect(stripSimpleMarkdown("Hello *world*")).toBe("Hello world");
  });

  it("does not invent HTML", () => {
    expect(stripSimpleMarkdown("**x**")).not.toMatch(/</);
  });

  it("keeps plain text unchanged", () => {
    expect(stripSimpleMarkdown("No markup here")).toBe("No markup here");
  });

  it("collapses image tokens to their alt text by default", () => {
    expect(
      stripSimpleMarkdown("Here ![Burger](https://cdn.example/b.jpg)"),
    ).toBe("Here Burger");
  });

  // Merge regression: AiWaiter renders allowlisted menu images itself, so the
  // shared stripper must be able to leave `![alt](url)` intact for that caller
  // while still flattening every other marker.
  it("keeps image tokens intact when keepImages is set", () => {
    expect(
      stripSimpleMarkdown("**Here** ![Burger](https://cdn.example/b.jpg)", {
        keepImages: true,
      }),
    ).toBe("Here ![Burger](https://cdn.example/b.jpg)");
  });

  it("still flattens plain links when keepImages is set", () => {
    expect(
      stripSimpleMarkdown("See [the menu](https://example.com/menu)", {
        keepImages: true,
      }),
    ).toBe("See the menu");
  });
});
