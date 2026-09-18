import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import { ShortcutsDialog } from "./ShortcutsDialog";

afterEach(cleanup);

describe("ShortcutsDialog", () => {
  it("renders keyboard shortcuts list when open", () => {
    render(<ShortcutsDialog open={true} onOpenChange={() => {}} />);
    expect(screen.getByText("Keyboard shortcuts")).toBeInTheDocument();
    expect(screen.getByText("Open command palette")).toBeInTheDocument();
    expect(screen.getByText("Send message")).toBeInTheDocument();
    expect(screen.getByText("Regenerate last response")).toBeInTheDocument();
  });

  it("renders nothing when closed", () => {
    render(<ShortcutsDialog open={false} onOpenChange={() => {}} />);
    expect(screen.queryByText("Keyboard shortcuts")).not.toBeInTheDocument();
  });
});
