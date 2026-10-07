/** @jest-environment jsdom */
import React from "react";
import { render, screen, fireEvent } from "@testing-library/react";
import { CustomerNoteSection } from "../CustomerNoteSection";

const t = (k: string) => k;

describe("CustomerNoteSection", () => {
  it("renders a textarea with the current value", () => {
    render(
      <CustomerNoteSection
        settings={{ delivery_instructions: "Leave at door" }}
        onChange={jest.fn()}
        tString={t}
      />,
    );
    const textarea = screen.getByRole("textbox");
    expect(textarea).toBeInTheDocument();
    expect((textarea as HTMLTextAreaElement).value).toBe("Leave at door");
  });

  it("renders with empty instructions", () => {
    render(
      <CustomerNoteSection
        settings={{ delivery_instructions: "" }}
        onChange={jest.fn()}
        tString={t}
      />,
    );
    expect(screen.getByRole("textbox")).toBeInTheDocument();
  });

  it("fires onChange when textarea value changes", () => {
    const onChange = jest.fn();
    render(
      <CustomerNoteSection
        settings={{ delivery_instructions: "" }}
        onChange={onChange}
        tString={t}
      />,
    );
    fireEvent.change(screen.getByRole("textbox"), {
      target: { value: "Ring the bell" },
    });
    expect(onChange).toHaveBeenCalledWith("Ring the bell");
  });

  it("renders card title from tString", () => {
    render(
      <CustomerNoteSection
        settings={{ delivery_instructions: "" }}
        onChange={jest.fn()}
        tString={t}
      />,
    );
    expect(screen.getByText("note.cardTitle")).toBeInTheDocument();
  });
});
