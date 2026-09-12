import { render, screen, fireEvent } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { SourceCards } from "./SourceCards";
import type { Source } from "@/lib/types";

const sources: Source[] = [
  { documentId: "d1", title: "notes.md", snippet: "Paris is the capital of France.", score: 0.87 },
  { documentId: "d1", title: "notes.md", snippet: "Second fact.", score: 0.42 },
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
});
