import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";
import { PayloadViewer } from "./PayloadViewer";

describe("PayloadViewer", () => {
  it("renders rich text markdown for analysis fields and toggles to JSON view", async () => {
    const user = userEvent.setup();
    const data = {
      analysis: "### Executive Brief\n\n- Key trend 1\n- Key trend 2",
      model: "gemini-2.5-flash",
      topic: "Artificial Intelligence",
    };

    render(<PayloadViewer title="Node Output Payload" data={data} defaultMode="rich" />);

    // Check header
    expect(screen.getByText("Node Output Payload")).toBeInTheDocument();

    // Check rendered markdown
    expect(await screen.findByRole("heading", { level: 3, name: "Executive Brief" })).toBeInTheDocument();
    expect(screen.getByText("Key trend 1")).toBeInTheDocument();
    expect(screen.getByText("gemini-2.5-flash")).toBeInTheDocument();

    // Switch to raw JSON mode
    const jsonBtn = screen.getByRole("button", { name: /JSON/i });
    await user.click(jsonBtn);

    // Verify raw JSON is displayed
    expect(screen.getByText(/"analysis":/i)).toBeInTheDocument();
    expect(screen.getByText(/"model": "gemini-2.5-flash"/i)).toBeInTheDocument();
  });

  it("renders raw JSON directly when no text fields exist", () => {
    const data = { count: 42, active: true };
    render(<PayloadViewer title="Metrics" data={data} />);

    expect(screen.getByText("Metrics")).toBeInTheDocument();
    expect(screen.getByText(/"count": 42/i)).toBeInTheDocument();
  });
});
