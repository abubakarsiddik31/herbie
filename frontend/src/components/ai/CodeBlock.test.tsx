import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { CodeBlock } from "./CodeBlock";

describe("CodeBlock", () => {
  it("renders non-previewable code blocks with language badge and copy button", async () => {
    const user = userEvent.setup();
    const writeSpy = vi.spyOn(navigator.clipboard, "writeText");
    render(
      <CodeBlock>
        <code className="language-python">{"def hello():\n    return 'world'"}</code>
      </CodeBlock>,
    );

    expect(screen.getByText("python")).toBeInTheDocument();
    expect(screen.queryByLabelText("View preview")).not.toBeInTheDocument();
    expect(screen.getByText(/def hello/)).toBeInTheDocument();

    const copyBtn = screen.getByRole("button", { name: "Copy code" });
    await user.click(copyBtn);
    expect(writeSpy).toHaveBeenCalledWith("def hello():\n    return 'world'");
    expect(screen.getByText("Copied")).toBeInTheDocument();
  });

  it("renders previewable svg block with preview and code tabs", async () => {
    const user = userEvent.setup();
    const svgCode = '<svg width="100" height="100"><circle cx="50" cy="50" r="40" fill="red" /></svg>';

    render(
      <CodeBlock>
        <code className="language-svg">{svgCode}</code>
      </CodeBlock>,
    );

    expect(screen.getByText("svg")).toBeInTheDocument();
    expect(screen.getByText("artifact")).toBeInTheDocument();

    // Default tab is preview
    expect(screen.getByTestId("svg-preview")).toBeInTheDocument();

    // Switch to Code tab
    const codeTab = screen.getByRole("button", { name: "View code" });
    await user.click(codeTab);
    expect(screen.getByText(svgCode)).toBeInTheDocument();

    // Switch back to Preview tab
    const previewTab = screen.getByRole("button", { name: "View preview" });
    await user.click(previewTab);
    expect(screen.getByTestId("svg-preview")).toBeInTheDocument();
  });

  it("renders previewable html block in a sandboxed iframe", async () => {
    const user = userEvent.setup();
    const htmlCode = "<h1>Interactive Mockup</h1><button>Click</button>";

    render(
      <CodeBlock>
        <code className="language-html">{htmlCode}</code>
      </CodeBlock>,
    );

    expect(screen.getByText("html")).toBeInTheDocument();
    const iframe = screen.getByTitle("HTML Preview") as HTMLIFrameElement;
    expect(iframe).toBeInTheDocument();
    expect(iframe.getAttribute("sandbox")).toBe("allow-scripts");
    expect(iframe.getAttribute("srcdoc")).toContain("<h1>Interactive Mockup</h1>");

    // Can reload the preview
    const reloadBtn = screen.getByLabelText("Reload preview");
    await user.click(reloadBtn);
    expect(screen.getByTitle("HTML Preview")).toBeInTheDocument();
  });

  it("renders mermaid diagram with loading state or rendered svg", async () => {
    render(
      <CodeBlock>
        <code className="language-mermaid">{"graph TD;\n  A-->B;"}</code>
      </CodeBlock>,
    );

    expect(screen.getByText("mermaid")).toBeInTheDocument();
    expect(screen.getByText("artifact")).toBeInTheDocument();
    // Loading indicator appears initially or SVG renders
    await waitFor(() => {
      const loading = screen.queryByTestId("mermaid-loading");
      const svg = screen.queryByTestId("mermaid-svg");
      const error = screen.queryByTestId("mermaid-error");
      expect(loading !== null || svg !== null || error !== null).toBe(true);
    });
  });

  it("opens fullscreen expanded dialog for artifacts", async () => {
    const user = userEvent.setup();
    render(
      <CodeBlock>
        <code className="language-html">{"<div>Expanded content</div>"}</code>
      </CodeBlock>,
    );

    const expandBtn = screen.getByLabelText("Expand view");
    await user.click(expandBtn);

    // Dialog content opens
    expect(screen.getByText("artifact preview")).toBeInTheDocument();
  });
});
