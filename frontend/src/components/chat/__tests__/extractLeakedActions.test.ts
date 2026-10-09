import { extractLeakedActions } from "../extractLeakedActions";

describe("extractLeakedActions", () => {
  it("extracts the exact observed key-value run and normalizes primary -> navigate", () => {
    const { content, actions } = extractLeakedActions(
      "You can update your menu here.\nhref: /business/2/dashboard?tab=menu kind: primary disabled: false disabled_reason: null",
    );
    expect(actions).toHaveLength(1);
    expect(actions[0].href).toBe("/business/2/dashboard?tab=menu");
    expect(actions[0].kind).toBe("navigate");
    expect(actions[0].disabled).toBe(false);
    expect(actions[0].label).not.toBe("");
    expect(content).toBe("You can update your menu here.");
  });

  it("extracts an embedded JSON action object and preserves surrounding prose", () => {
    const { content, actions } = extractLeakedActions(
      'Open the menu builder: {"label":"Edit menu","href":"/business/3/dashboard?tab=menu","kind":"navigate","disabled":false,"disabled_reason":null} and you are set.',
    );
    expect(actions).toHaveLength(1);
    expect(actions[0].label).toBe("Edit menu");
    expect(actions[0].href).toBe("/business/3/dashboard?tab=menu");
    expect(content).not.toContain("{");
    expect(content).toContain("Open the menu builder");
    expect(content).toContain("you are set");
  });

  it("extracts multiple leaked actions", () => {
    const { actions } = extractLeakedActions(
      "Menu: href: /business/4/dashboard?tab=menu kind: navigate\nBills: href: /business/4/dashboard?tab=bills kind: navigate",
    );
    expect(actions).toHaveLength(2);
    expect(actions[0].href).toBe("/business/4/dashboard?tab=menu");
    expect(actions[1].href).toBe("/business/4/dashboard?tab=bills");
  });

  it("maps unknown kind: internal -> navigate, external URL -> external", () => {
    const internal = extractLeakedActions(
      "Go: href: /business/5/dashboard?tab=menu kind: primary",
    );
    expect(internal.actions[0].kind).toBe("navigate");
    const external = extractLeakedActions("Docs: href: https://payverge.io/docs kind: primary");
    expect(external.actions[0].kind).toBe("external");
  });

  it("drops empty-href actions", () => {
    const { content, actions } = extractLeakedActions(
      'Here: {"label":"Nowhere","href":"","kind":"navigate"}',
    );
    expect(actions).toHaveLength(0);
    expect(content).not.toContain("{");
  });

  it("passes clean content through unchanged with no actions", () => {
    const input = "Bills live under the Serve tab. Ask me anything else.";
    const { content, actions } = extractLeakedActions(input);
    expect(actions).toHaveLength(0);
    expect(content).toBe(input);
  });

  it.each([
    ["javascript", "Docs: href: javascript:alert(document.cookie) kind: primary"],
    ["data", "Docs: href: data:text/html,x kind: primary"],
    ["protocol-relative", "Docs: href: //evil.com/x kind: primary"],
  ])("rejects unsafe %s scheme and preserves prose", (_name, input) => {
    const { content, actions } = extractLeakedActions(input);
    expect(actions).toHaveLength(0);
    expect(content).toBe(input);
  });

  it("passes prose that merely mentions href: through unchanged", () => {
    const input = "To add a link, type the href: field name in config.";
    const { content, actions } = extractLeakedActions(input);
    expect(actions).toHaveLength(0);
    expect(content).toBe(input);
  });

  it("captures a multi-word unquoted disabled_reason to end of line", () => {
    const { content, actions } = extractLeakedActions(
      "Menu is limited. href: /business/2/dashboard?tab=menu kind: navigate disabled: true disabled_reason: Owner only. Contact your manager.",
    );
    expect(actions).toHaveLength(1);
    expect(actions[0].disabled_reason).toBe("Owner only. Contact your manager.");
    expect(content).not.toContain("only. Contact");
  });

  it("trims trailing sentence punctuation from a captured href", () => {
    const { actions } = extractLeakedActions("Go here: href: /business/2/dashboard?tab=menu.");
    expect(actions).toHaveLength(1);
    expect(actions[0].href).toBe("/business/2/dashboard?tab=menu");
  });
});
