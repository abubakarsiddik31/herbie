import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { ThinkingTrace } from "./ThinkingTrace";

describe("ThinkingTrace", () => {
  it("renders humanized steps and friendly summary instead of raw agent thinking", () => {
    const rawRows = [
      "search_documents…",
      "model call · 1500 in / 45 out",
      "earlier history summarized",
    ];

    render(<ThinkingTrace rows={rawRows} />);

    // Header says "Research & Steps" rather than "thinking"
    expect(screen.getByText("Research & Steps")).toBeInTheDocument();
    expect(screen.queryByText(/^thinking/i)).not.toBeInTheDocument();

    // Humanized steps replace raw developer tokens and internal names
    expect(screen.getByText("Searched referenced workspace documents")).toBeInTheDocument();
    expect(screen.getByText("Composed and structured findings")).toBeInTheDocument();
    expect(screen.getByText("Synthesized previous conversation context")).toBeInTheDocument();

    // Raw developer debug string must NOT be displayed
    expect(screen.queryByText(/1500 in \/ 45 out/)).not.toBeInTheDocument();
  });

  it("returns null when rows is empty", () => {
    const { container } = render(<ThinkingTrace rows={[]} />);
    expect(container.firstChild).toBeNull();
  });
});
