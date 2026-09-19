import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { http, HttpResponse } from "msw";
import { setupServer } from "msw/node";
import { afterAll, afterEach, beforeAll, describe, expect, it, vi } from "vitest";
import { ParsedFileViewerDialog } from "./ParsedFileViewerDialog";

const server = setupServer(
  http.get("*/api/documents/:id/content", ({ params }) => {
    if (params.id === "doc-123") {
      return HttpResponse.json({
        id: "doc-123",
        filename: "project-specs.md",
        mime: "text/markdown",
        sizeBytes: 1024,
        status: "ready",
        chunkCount: 3,
        createdAt: "2026-09-19T10:00:00Z",
        text: "# Project Architecture\n\nThis is the parsed architecture document.",
      });
    }
    return HttpResponse.json({ error: { code: "not_found", message: "document not found" } }, { status: 404 });
  }),
);

beforeAll(() => server.listen({ onUnhandledRequest: "error" }));
afterEach(() => server.resetHandlers());
afterAll(() => server.close());

function renderWithClient(ui: React.ReactElement) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(<QueryClientProvider client={client}>{ui}</QueryClientProvider>);
}

describe("ParsedFileViewerDialog", () => {
  it("renders in-memory content directly without network fetch (inline chat)", async () => {
    renderWithClient(
      <ParsedFileViewerDialog
        open={true}
        onOpenChange={vi.fn()}
        title="inline-code.py"
        content="def hello():\n    print('Hello World')\n"
      />
    );

    expect(screen.getByText("inline-code.py")).toBeInTheDocument();
    expect(screen.getByText(/Hello World/)).toBeInTheDocument();
    expect(screen.getByText(/lines/)).toBeInTheDocument();
  });

  it("fetches document content by documentId (project file)", async () => {
    renderWithClient(
      <ParsedFileViewerDialog
        open={true}
        onOpenChange={vi.fn()}
        title="project-specs.md"
        documentId="doc-123"
      />
    );

    await waitFor(() => {
      expect(screen.getByText("Project Architecture")).toBeInTheDocument();
    });
    expect(screen.getByText(/This is the parsed architecture document/)).toBeInTheDocument();
  });

  it("toggles between formatted and raw view mode", async () => {
    renderWithClient(
      <ParsedFileViewerDialog
        open={true}
        onOpenChange={vi.fn()}
        title="doc.md"
        content={"# Header 1\n\nParagraph 1"}
      />
    );

    // In formatted mode, it renders a heading
    expect(screen.getByRole("heading", { name: "Header 1" })).toBeInTheDocument();

    const rawBtn = screen.getByTestId("raw-mode-btn");
    fireEvent.click(rawBtn);

    // Line 1 should be visible with markdown hash in raw line table
    await waitFor(() => {
      expect(screen.getByText("# Header 1")).toBeInTheDocument();
    });
  });
});
