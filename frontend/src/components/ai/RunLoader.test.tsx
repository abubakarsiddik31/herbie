import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { RunLoader } from "./RunLoader";

describe("RunLoader", () => {
  it("renders friendly dynamic verbiage instead of static thinking", () => {
    render(<RunLoader />);

    // Initial verbiage is friendly and engaging
    expect(screen.getByText("Analyzing your request...")).toBeInTheDocument();
  });

  it("contextually adapts verbiage when tool activity is in the trace", () => {
    const { rerender } = render(<RunLoader trace={["search_documents…"]} />);
    expect(screen.getByText("Reading through referenced documents...")).toBeInTheDocument();

    rerender(<RunLoader trace={["web_fetch…"]} />);
    expect(screen.getByText("Retrieving relevant information from the web...")).toBeInTheDocument();

    rerender(<RunLoader trace={["calendar…"]} />);
    expect(screen.getByText("Checking calendar schedule...")).toBeInTheDocument();

    rerender(<RunLoader trace={["github…"]} />);
    expect(screen.getByText("Inspecting GitHub repository...")).toBeInTheDocument();
  });
});
