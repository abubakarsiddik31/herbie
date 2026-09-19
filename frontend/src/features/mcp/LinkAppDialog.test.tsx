import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import { http, HttpResponse } from "msw";
import { setupServer } from "msw/node";
import type { ReactNode } from "react";
import { afterAll, afterEach, beforeAll, describe, expect, it, vi } from "vitest";
import { LinkAppDialog } from "./LinkAppDialog";

let linkCalledAppId: string | null = null;
let unlinkCalledAppId: string | null = null;

const server = setupServer(
  http.get("*/api/mcp/catalog", () =>
    HttpResponse.json({
      catalog: [
        {
          id: "github",
          name: "GitHub",
          mention: "github",
          category: "Developer",
          description: "Inspect repositories, search code, list pull requests, and manage issues.",
          icon: "github",
          requiresAuth: false,
          authType: "token",
          authPrompt: "Optional Personal Access Token",
          connected: false,
          tools: [{ name: "github_search_repos", description: "Search repos", inputSchema: {} }],
        },
        {
          id: "slack",
          name: "Slack",
          mention: "slack",
          category: "Communication",
          description: "Send notifications and announcements to Slack.",
          icon: "slack",
          requiresAuth: false,
          connected: false,
          tools: [{ name: "slack_post_message", description: "Post message", inputSchema: {} }],
        },
        {
          id: "google_calendar",
          name: "Google Calendar",
          mention: "calendar",
          category: "Productivity",
          description: "View schedule, check agenda, and manage events.",
          icon: "calendar",
          requiresAuth: false,
          connected: true,
          tools: [{ name: "google_calendar", description: "Manage events", inputSchema: {} }],
        },
      ],
    }),
  ),
  http.get("*/api/tool-oauth/providers", () =>
    HttpResponse.json({
      providers: [],
    }),
  ),
  http.post("*/api/mcp/catalog/:appId/link", ({ params }) => {
    linkCalledAppId = params.appId as string;
    return HttpResponse.json({ ok: true });
  }),
  http.post("*/api/mcp/catalog/:appId/unlink", ({ params }) => {
    unlinkCalledAppId = params.appId as string;
    return HttpResponse.json({ ok: true });
  }),
);

beforeAll(() => server.listen({ onUnhandledRequest: "error" }));
afterEach(() => {
  server.resetHandlers();
  linkCalledAppId = null;
  unlinkCalledAppId = null;
});
afterAll(() => server.close());

function wrapper(client: QueryClient) {
  return function Wrapper({ children }: { children: ReactNode }) {
    return <QueryClientProvider client={client}>{children}</QueryClientProvider>;
  };
}

describe("LinkAppDialog", () => {
  it("renders curated MCP catalog with 1-click controls", async () => {
    const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    render(<LinkAppDialog open={true} onOpenChange={vi.fn()} />, {
      wrapper: wrapper(client),
    });

    expect(await screen.findByText("Apps & MCP Integrations")).toBeInTheDocument();
    expect(await screen.findByText("GitHub")).toBeInTheDocument();
    expect(screen.getByText("Slack")).toBeInTheDocument();
    expect(screen.getByText("Google Calendar")).toBeInTheDocument();
    expect(screen.getByText("Connected")).toBeInTheDocument();
  });

  it("triggers 1-click link on GitHub", async () => {
    const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    render(<LinkAppDialog open={true} onOpenChange={vi.fn()} />, {
      wrapper: wrapper(client),
    });

    const linkButtons = await screen.findAllByText("1-Click Link");
    fireEvent.click(linkButtons[0]);

    await waitFor(() => {
      expect(linkCalledAppId).toBe("github");
    });
  });

  it("triggers 1-click unlink on connected app", async () => {
    const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    render(<LinkAppDialog open={true} onOpenChange={vi.fn()} />, {
      wrapper: wrapper(client),
    });

    const unlinkBtn = await screen.findByRole("button", { name: /unlink/i });
    fireEvent.click(unlinkBtn);

    await waitFor(() => {
      expect(unlinkCalledAppId).toBe("google_calendar");
    });
  });

  it("filters apps by category", async () => {
    const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    render(<LinkAppDialog open={true} onOpenChange={vi.fn()} />, {
      wrapper: wrapper(client),
    });

    expect(await screen.findByText("GitHub")).toBeInTheDocument();

    const commFilter = screen.getByRole("button", { name: "Communication" });
    fireEvent.click(commFilter);

    expect(screen.getByText("Slack")).toBeInTheDocument();
    expect(screen.queryByText("GitHub")).not.toBeInTheDocument();
  });
});
