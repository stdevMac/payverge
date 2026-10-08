/**
 * @jest-environment jsdom
 */
import {
  EDITOR_CANVAS_COLUMN_STYLE,
  EDITOR_CANVAS_MIN_HEIGHT_PX,
  EDITOR_PROPERTIES_COLUMN_STYLE,
  EDITOR_WORKSPACE_EDIT_STYLE,
  propertiesCoversCanvas,
} from "../editorLayout";

function el(style: Record<string, string>, children: HTMLElement[] = []): HTMLElement {
  const node = document.createElement("div");
  Object.assign(node.style, style);
  for (const child of children) node.appendChild(child);
  return node;
}

describe("propertiesCoversCanvas", () => {
  it("is false for a row-flex sibling layout with a growing canvas", () => {
    const canvas = el({
      flexGrow: "1",
      minHeight: `${EDITOR_CANVAS_MIN_HEIGHT_PX}px`,
      position: "relative",
    });
    const properties = el({
      flexGrow: "0",
      position: "relative",
    });
    const workspace = el(
      { display: "flex", flexDirection: "row" },
      [canvas, properties],
    );
    expect(propertiesCoversCanvas(workspace, canvas, properties)).toBe(false);
  });

  it("is true when Properties is nested inside the canvas column", () => {
    const properties = el({ position: "relative", flexGrow: "0" });
    const canvas = el(
      { flexGrow: "1", position: "relative" },
      [properties],
    );
    const workspace = el(
      { display: "flex", flexDirection: "row" },
      [canvas],
    );
    expect(propertiesCoversCanvas(workspace, canvas, properties)).toBe(true);
  });

  it("is true when Properties is absolutely positioned over the map", () => {
    const canvas = el({ flexGrow: "1", position: "relative" });
    const properties = el({ flexGrow: "0", position: "absolute" });
    const workspace = el(
      { display: "flex", flexDirection: "row" },
      [canvas, properties],
    );
    expect(propertiesCoversCanvas(workspace, canvas, properties)).toBe(true);
  });

  it("is true when the workspace stacks (Properties sits on the canvas)", () => {
    const canvas = el({ flexGrow: "1", position: "relative" });
    const properties = el({ flexGrow: "0", position: "relative" });
    const workspace = el(
      { display: "flex", flexDirection: "column" },
      [canvas, properties],
    );
    expect(propertiesCoversCanvas(workspace, canvas, properties)).toBe(true);
  });

  it("exported column styles keep canvas as the growing hero", () => {
    expect(EDITOR_WORKSPACE_EDIT_STYLE.display).toBe("flex");
    expect(EDITOR_WORKSPACE_EDIT_STYLE.flexDirection).toBe("row");
    expect(EDITOR_CANVAS_COLUMN_STYLE.flexGrow).toBe(1);
    expect(EDITOR_CANVAS_COLUMN_STYLE.minHeight).toBe(EDITOR_CANVAS_MIN_HEIGHT_PX);
    expect(EDITOR_PROPERTIES_COLUMN_STYLE.flexGrow).toBe(0);
    expect(EDITOR_PROPERTIES_COLUMN_STYLE.position).toBe("relative");
  });
});
