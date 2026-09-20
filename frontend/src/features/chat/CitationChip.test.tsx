import { describe, it, expect, vi } from "vitest";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { CitationChip } from "./CitationChip";

describe("CitationChip", () => {
  it("renders chip and opens hover preview for document source", async () => {
    const user = userEvent.setup();
    const onCite = vi.fn();
    const source = {
      documentId: "doc-1",
      title: "financial-report.pdf",
      heading: "Q3 Results",
      page: 12,
      snippet: "Revenue increased by 15% year-over-year.",
      score: 0.95,
    };

    render(<CitationChip n={1} source={source} onCite={onCite} />);

    const chip = screen.getByRole("button", { name: "Source 1: financial-report.pdf, p.12" });
    expect(chip).toHaveTextContent("1");

    await user.hover(chip);

    expect(await screen.findByText("Document")).toBeInTheDocument();
    expect(screen.getByText("financial-report.pdf")).toBeInTheDocument();
    expect(screen.getByText("Revenue increased by 15% year-over-year.")).toBeInTheDocument();
    expect(screen.getByText(/Page 12/)).toBeInTheDocument();
    expect(screen.getByText(/§ Q3 Results/)).toBeInTheDocument();

    await user.click(chip);
    expect(onCite).toHaveBeenCalledWith(1);
  });

  it("renders chip and opens hover preview for web source with link", async () => {
    const user = userEvent.setup();
    const openSpy = vi.spyOn(window, "open").mockImplementation(() => null);
    const source = {
      documentId: "https://blogs.microsoft.com/copilot-studio",
      title: "Microsoft Copilot Studio Deep Agentic Upgrades",
      heading: "https://blogs.microsoft.com/copilot-studio",
      page: 0,
      snippet: "Enterprises can build, orchestrate, and deploy autonomous agents.",
      score: 1.0,
      url: "https://blogs.microsoft.com/copilot-studio",
    };

    render(<CitationChip n={2} source={source} />);

    const chip = screen.getByRole("button", { name: "Source 2: Microsoft Copilot Studio Deep Agentic Upgrades" });
    expect(chip).toHaveTextContent("2");

    await user.hover(chip);

    expect(await screen.findByText("Web Search")).toBeInTheDocument();
    expect(screen.getByText("Microsoft Copilot Studio Deep Agentic Upgrades")).toBeInTheDocument();
    expect(screen.getByText("blogs.microsoft.com")).toBeInTheDocument();
    expect(screen.getByText("Enterprises can build, orchestrate, and deploy autonomous agents.")).toBeInTheDocument();
    expect(screen.getByRole("link", { name: /Visit website/i })).toHaveAttribute(
      "href",
      "https://blogs.microsoft.com/copilot-studio"
    );

    await user.click(chip);
    expect(openSpy).toHaveBeenCalledWith("https://blogs.microsoft.com/copilot-studio", "_blank", "noopener,noreferrer");
    openSpy.mockRestore();
  });

  it("renders chip without source and shows fallback hover information", async () => {
    const user = userEvent.setup();
    render(<CitationChip n={20} />);

    const chip = screen.getByRole("button", { name: "Source 20" });
    expect(chip).toHaveTextContent("20");

    await user.hover(chip);

    expect(await screen.findByText("Web Search")).toBeInTheDocument();
    expect(screen.getByText("Source 20")).toBeInTheDocument();
  });
});
