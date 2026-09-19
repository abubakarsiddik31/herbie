import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import { http, HttpResponse } from "msw";
import { setupServer } from "msw/node";
import type { ReactNode } from "react";
import { afterAll, afterEach, beforeAll, describe, expect, it, vi } from "vitest";
import { LinkAppDialog } from "./LinkAppDialog";

let createdServerPayload: unknown = null;

const server = setupServer(
  http.get("*/api/tool-oauth/providers", () =>
    HttpResponse.json({
      providers: [
        {
          id: "github",
          name: "GitHub",
          configured: false,
          connected: false,
        },
        {
          id: "slack",
          name: "Slack",
          configured: true,
          connected: false,
        },
        {
          id: "google_calendar",
          name: "Google Calendar",
          configured: true,
          connected: true,
          connectedVia: "oauth",
        },
      ],
    }),
  ),
  http.get("*/api/mcp/servers", () =>
    HttpResponse.json({
      servers: [],
    }),
  ),
  http.post("*/api/mcp/servers/test", () =>
    HttpResponse.json({
      ok: true,
      count: 3,
      tools: [
        { name: "list_issues", description: "List GitHub issues", inputSchema: {} },
        { name: "create_issue", description: "Create GitHub issue", inputSchema: {} },
        { name: "list_pull_requests", description: "List PRs", inputSchema: {} },
      ],
    }),
  ),
  http.post("*/api/mcp/servers", async ({ request }) => {
    createdServerPayload = await request.json();
    return HttpResponse.json({
      server: {
        id: "mcp-gh-1",
        userId: "user-1",
        name: "github_mcp_server",
        url: "http://localhost:8000/mcp",
        appId: "github",
        enabled: true,
        createdAt: "2026-09-19T00:00:00Z",
        updatedAt: "2026-09-19T00:00:00Z",
      },
    });
  }),
);

beforeAll(() => server.listen({ onUnhandledRequest: "error" }));
afterEach(() => {
  server.resetHandlers();
  createdServerPayload = null;
});
afterAll(() => server.close());

function wrapper(client: QueryClient) {
  return function Wrapper({ children }: { children: ReactNode }) {
    return <QueryClientProvider client={client}>{children}</QueryClientProvider>;
  };
}

describe("LinkAppDialog", () => {
  it("renders with GitHub preselected and displays MCP preset form", async () => {
    const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    render(<LinkAppDialog open={true} onOpenChange={vi.fn()} initialAppId="github" />, {
      wrapper: wrapper(client),
    });

    expect(await screen.findByText("Link GitHub App")).toBeInTheDocument();
    expect(screen.getByText("Link via MCP Server (Recommended)")).toBeInTheDocument();
    expect(screen.getByDisplayValue("GitHub MCP Server")).toBeInTheDocument();
    expect(screen.getByDisplayValue("http://localhost:8000/mcp")).toBeInTheDocument();
  });

  it("tests MCP server connection and saves app-linked MCP server", async () => {
    const onOpenChange = vi.fn();
    const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    render(<LinkAppDialog open={true} onOpenChange={onOpenChange} initialAppId="github" />, {
      wrapper: wrapper(client),
    });

    // Test connection
    const testBtn = screen.getByText("Test Connection");
    fireEvent.click(testBtn);

    await waitFor(() => {
      expect(screen.getByText("Endpoint Active — Discovered 3 Tool(s)")).toBeInTheDocument();
      expect(screen.getByText("github_list_issues")).toBeInTheDocument();
    });

    // Save and link
    const saveBtn = screen.getByText("Link GitHub via MCP");
    fireEvent.click(saveBtn);

    await waitFor(() => {
      expect(createdServerPayload).toMatchObject({
        name: "github_mcp_server",
        url: "http://localhost:8000/mcp",
        appId: "github",
      });
      expect(onOpenChange).toHaveBeenCalledWith(false);
    });
  });

  it("switches to Slack preset when clicking Slack pill", async () => {
    const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    render(<LinkAppDialog open={true} onOpenChange={vi.fn()} initialAppId="github" />, {
      wrapper: wrapper(client),
    });

    const slackPill = screen.getByRole("button", { name: "Slack" });
    fireEvent.click(slackPill);

    expect(await screen.findByText("Link Slack App")).toBeInTheDocument();
    expect(screen.getByDisplayValue("Slack MCP Server")).toBeInTheDocument();
    expect(screen.getByDisplayValue("http://localhost:8001/mcp")).toBeInTheDocument();
  });
});
