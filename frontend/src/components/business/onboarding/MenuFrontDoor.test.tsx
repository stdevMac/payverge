/** @jest-environment jsdom */
import React from "react";
import { render, screen, fireEvent } from "@testing-library/react";
import MenuFrontDoor from "./MenuFrontDoor";

const copy: Record<string, string> = {
  "ai.emptyState.pdf.title": "Scan your menu",
  "ai.emptyState.pdf.description":
    "Photograph or upload your current menu — AI rebuilds it in minutes.",
  "ai.emptyState.wizard.title": "Describe it to AI",
  "ai.emptyState.wizard.description":
    "Tell us about your restaurant and AI will build your menu.",
  "ai.emptyState.manual.title": "Build by hand",
  "ai.emptyState.manual.description": "Add categories and items yourself.",
  "ai.emptyState.badges.ai": "AI-assisted",
  "ai.emptyState.badges.manual": "Manual",
};

const tString = (key: string) => copy[key] ?? key;

describe("MenuFrontDoor", () => {
  it("renders the three tiles in scan → describe → manual order", () => {
    render(
      <MenuFrontDoor tString={tString} onAddCategoryOpen={jest.fn()} />,
    );
    const headings = screen.getAllByRole("heading", { level: 4 }).map((h) => h.textContent);
    expect(headings).toEqual(["Scan your menu", "Describe it to AI", "Build by hand"]);
  });

  it("labels the AI tiles as AI-assisted and the manual tile as manual", () => {
    render(
      <MenuFrontDoor tString={tString} onAddCategoryOpen={jest.fn()} />,
    );
    expect(screen.getAllByText("AI-assisted")).toHaveLength(2);
    expect(screen.getByText("Manual")).toBeInTheDocument();
  });

  it("opens the AI feature directly from an AI tile", () => {
    const onOpenAIFeature = jest.fn();
    render(
      <MenuFrontDoor
        tString={tString}
        onAddCategoryOpen={jest.fn()}
        onOpenAIFeature={onOpenAIFeature}
      />,
    );
    fireEvent.click(screen.getByText("Scan your menu"));
    expect(onOpenAIFeature).toHaveBeenCalledWith("pdf");
  });

  it("calls onAddCategoryOpen for the manual tile", () => {
    const onAddCategoryOpen = jest.fn();
    render(
      <MenuFrontDoor
        tString={tString}
        onAddCategoryOpen={onAddCategoryOpen}
      />,
    );
    fireEvent.click(screen.getByText("Build by hand"));
    expect(onAddCategoryOpen).toHaveBeenCalled();
  });
});
