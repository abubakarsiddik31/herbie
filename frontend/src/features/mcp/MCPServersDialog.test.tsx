import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import { http, HttpResponse } from "msw";
import { setupServer } from "msw/node";
import type { ReactNode } from "react";
import { afterAll, afterEach, beforeAll, describe, expect, it, vi } from "vitest";
import { MCPServersDialog } from "./MCPServersDialog";

const server = setupServer(
  http.get("*/api/mcp/servers", () =>
    HttpResponse.json({
      servers: [
        {
          id: "mcp-1",
          userId: "user-1",
          name: "sqlite_mcp",
          url: "http://localhost:3000/mcp",
          transport: "http",
          enabled: true,
          createdAt: "2026-09-19T00:00:00Z",
          updatedAt: "2026-09-19T00:00:00Z",
        },
      ],
    }),
  ),
  http.post("*/api/mcp/servers/test", () =>
    HttpResponse.json({
      ok: true,
      count: 2,
      tools: [
        { name: "read_query", description: "Read DB", inputSchema: {} },
        { name: "write_query", description: "Write DB", inputSchema: {} },
      ],
    }),
  ),
);

beforeAll(() => server.listen({ onUnhandledRequest: "error" }));
afterEach(() => server.resetHandlers());
afterAll(() => server.close());

function wrapper(client: QueryClient) {
  return function Wrapper({ children }: { children: ReactNode }) {
    return <QueryClientProvider client={client}>{children}</QueryClientProvider>;
  };
}

describe("MCPServersDialog", () => {
  it("renders registered MCP server and test endpoint details", async () => {
    const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    render(<MCPServersDialog open={true} onOpenChange={vi.fn()} />, {
      wrapper: wrapper(client),
    });

    expect(screen.getByText("Model Context Protocol (MCP) Servers")).toBeInTheDocument();
    expect(await screen.findByText("sqlite_mcp")).toBeInTheDocument();
    expect(screen.getByText("http://localhost:3000/mcp")).toBeInTheDocument();
    expect(screen.getByText("POST /api/mcp")).toBeInTheDocument();
  });

  it("opens add form and tests connection displaying discovered tools", async () => {
    const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    render(<MCPServersDialog open={true} onOpenChange={vi.fn()} />, {
      wrapper: wrapper(client),
    });

    const addBtn = screen.getByText("Add MCP Server");
    fireEvent.click(addBtn);

    expect(screen.getByLabelText("Server Name")).toBeInTheDocument();
    expect(screen.getByLabelText("Endpoint URL")).toBeInTheDocument();

    const urlInput = screen.getByLabelText("Endpoint URL");
    fireEvent.change(urlInput, { target: { value: "http://localhost:4000/mcp" } });

    const testBtn = screen.getByText("Test Connection");
    fireEvent.click(testBtn);

    await waitFor(() => {
      expect(screen.getByText("Discovered 2 tool(s):")).toBeInTheDocument();
      expect(screen.getByText("read_query")).toBeInTheDocument();
      expect(screen.getByText("write_query")).toBeInTheDocument();
    });
  });
});
