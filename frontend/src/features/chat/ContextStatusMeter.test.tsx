import { describe, it, expect } from "vitest";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { ContextStatusMeter } from "./ContextStatusMeter";

describe("ContextStatusMeter", () => {
  it("renders formatted token usage and threshold", () => {
    render(<ContextStatusMeter estimatedTokens={1200} thresholdTokens={40000} />);
    expect(screen.getByText("1.2k / 40k")).toBeInTheDocument();
  });

  it("indicates when history is compacted", () => {
    render(<ContextStatusMeter estimatedTokens={15000} thresholdTokens={40000} compacted={true} />);
    expect(screen.getByText("compacted")).toBeInTheDocument();
  });

  it("shows the exact token breakdown", async () => {
    const user = userEvent.setup();
    render(
      <ContextStatusMeter
        estimatedTokens={2400}
        thresholdTokens={40000}
        exact={true}
        breakdown={{
          systemTokens: 800,
          memoryTokens: 100,
          instructionsTokens: 50,
          historyTokens: 1500,
          promptTokens: 60,
          attachmentTokens: 0,
          totalTokens: 2400,
          exact: true,
          model: "gemini-3.5-flash",
        }}
      />,
    );
    await user.click(screen.getByRole("button", { name: /Context status:/i }));
    expect(await screen.findByText("Token breakdown")).toBeInTheDocument();
    expect(screen.getByText("exact")).toBeInTheDocument();
    expect(screen.getByText("System (incl. memory)")).toBeInTheDocument();
    expect(screen.getByText("1,500")).toBeInTheDocument();
  });

  it("opens tooltip with details on click", async () => {
    const user = userEvent.setup();
    render(
      <ContextStatusMeter estimatedTokens={28000} thresholdTokens={40000} keepRecent={10} />
    );

    const trigger = screen.getByRole("button", { name: /Context status:/i });
    await user.click(trigger);

    expect(await screen.findByText("Context & Compaction")).toBeInTheDocument();
    expect(screen.getByText(/Auto-compacts when conversation exceeds/i)).toBeInTheDocument();
    expect(screen.getByText("40,000")).toBeInTheDocument();
    expect(screen.getByText("10")).toBeInTheDocument();
  });
});
