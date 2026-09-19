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
