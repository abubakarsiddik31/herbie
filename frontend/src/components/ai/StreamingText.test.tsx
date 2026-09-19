import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { StreamingText } from "./StreamingText";
import type { Source } from "@/lib/types";

function sources(count: number): Source[] {
  return Array.from({ length: count }, (_, i) => ({
    documentId: `doc-${i + 1}`,
    title: `Report ${i + 1}.pdf`,
    heading: "Overview",
    page: i + 2,
    snippet: "chunk text",
    score: 0.9,
  }));
}

describe("StreamingText citations", () => {
  it("links a bracket citation to its source number", async () => {
    const onCite = vi.fn();
    const user = userEvent.setup();
    render(<StreamingText content="Herons nest here [2]." citations={{ sources: sources(3), onCite }} />);
    const link = screen.getByRole("button", { name: "Source 2: Report 2.pdf, p.3" });
    expect(link).toHaveTextContent("2");
    await user.click(link);
    expect(onCite).toHaveBeenCalledWith(2);
  });

  it("links each number in a grouped citation", () => {
    const onCite = vi.fn();
    render(<StreamingText content="Both agree [1, 3]." citations={{ sources: sources(3), onCite }} />);
    expect(screen.getByRole("button", { name: /Source 1:/ })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /Source 3:/ })).toBeInTheDocument();
  });

  it("names the source file and page on the chip", () => {
    const onCite = vi.fn();
    render(<StreamingText content="Claim [1]." citations={{ sources: sources(1), onCite }} />);
    expect(screen.getByRole("button", { name: "Source 1: Report 1.pdf, p.2" })).toBeInTheDocument();
  });

  it("leaves out-of-range citations as plain text", () => {
    const onCite = vi.fn();
    const { container } = render(
      <StreamingText content="Mystery claim [9]." citations={{ sources: sources(2), onCite }} />,
    );
    expect(screen.queryByRole("button")).not.toBeInTheDocument();
    expect(container).toHaveTextContent("Mystery claim [9].");
  });

  it("does not link citations inside code spans", () => {
    const onCite = vi.fn();
    render(<StreamingText content="Run `[1]` to test." citations={{ sources: sources(3), onCite }} />);
    expect(screen.queryByRole("button")).not.toBeInTheDocument();
  });

  it("renders plain text without the citations prop", () => {
    const { container } = render(<StreamingText content="Plain [1] text." />);
    expect(screen.queryByRole("button")).not.toBeInTheDocument();
    expect(container).toHaveTextContent("Plain [1] text.");
  });

  it("leaves normal markdown links alone", () => {
    const onCite = vi.fn();
    render(
      <StreamingText content="See [docs](https://example.com) [1]." citations={{ sources: sources(2), onCite }} />,
    );
    expect(screen.getByRole("link", { name: "docs" })).toHaveAttribute("href", "https://example.com");
    expect(screen.getByRole("button", { name: /Source 1:/ })).toBeInTheDocument();
  });

  it("renders inline and block math with KaTeX", () => {
    const { container } = render(
      <StreamingText content={"To calculate compound interest on **$10,000**:\n\n$$A = P \\left(1 + \\frac{r}{n}\\right)^{nt}$$\n\nWhere:\n- $P$ = Principal amount ($10,000)\n- $r$ = Annual interest rate (0.07)"} />
    );
    expect(container.querySelector(".katex")).toBeInTheDocument();
  });
});
