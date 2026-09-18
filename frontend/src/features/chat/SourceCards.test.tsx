import { render, screen, fireEvent } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { SourceCards } from "./SourceCards";
import type { Source } from "@/lib/types";

const sources: Source[] = [
  { documentId: "d1", title: "notes.md", heading: "Install", page: 2, snippet: "Paris is the capital of France.", score: 0.87 },
  { documentId: "d1", title: "notes.md", heading: "", page: 0, snippet: "Second fact.", score: 0.42 },
];

describe("SourceCards", () => {
  it("renders collapsed with a count and expands on click", () => {
    render(<SourceCards sources={sources} />);
    const toggle = screen.getByRole("button", { name: /sources \(2\)/i });
    expect(screen.queryByText(/paris is the capital/i)).toBeNull();
    fireEvent.click(toggle);
    expect(screen.getByText(/paris is the capital/i)).toBeTruthy();
    expect(screen.getByText(/second fact/i)).toBeTruthy();
  });

  it("renders nothing without sources", () => {
    const { container } = render(<SourceCards sources={[]} />);
    expect(container).toBeEmptyDOMElement();
  });

  it("shows heading and page when present", () => {
    render(<SourceCards sources={sources} />);
    fireEvent.click(screen.getByRole("button", { name: /sources \(2\)/i }));
    expect(screen.getAllByText("notes.md")).toHaveLength(2);
    expect(screen.getByText(/Install/)).toBeTruthy();
    expect(screen.getByText(/p\.2/)).toBeTruthy();
  });

  it("hides heading and page markers when absent", () => {
    render(
      <SourceCards
        sources={[{ documentId: "d", title: "plain.md", heading: "", page: 0, snippet: "Just text.", score: 0.1 }]}
      />,
    );
    fireEvent.click(screen.getByRole("button", { name: /sources \(1\)/i }));
    expect(screen.getByText("plain.md")).toBeTruthy();
    expect(screen.queryByText(/§/)).toBeNull();
    expect(screen.queryByText(/p\./)).toBeNull();
  });

  it("expands and flashes the jumped-to source", () => {
    render(<SourceCards sources={sources} jump={{ n: 2, seq: 1 }} />);
    // Collapsed by default, but a jump opens the list and highlights card 2.
    expect(screen.getByText(/second fact/i)).toBeInTheDocument();
    expect(screen.getByText(/second fact/i).closest("li")).toHaveClass("ring-primary");
  });

  it("ignores out-of-range jumps", () => {
    render(<SourceCards sources={sources} jump={{ n: 9, seq: 1 }} />);
    expect(screen.queryByText(/paris is the capital/i)).toBeNull();
  });
});
