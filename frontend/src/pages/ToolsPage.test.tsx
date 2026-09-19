import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { http, HttpResponse } from "msw";
import { setupServer } from "msw/node";
import { MemoryRouter } from "react-router";
import { afterAll, afterEach, beforeAll, describe, expect, it } from "vitest";
import { SidebarProvider } from "@/components/layout/SidebarContext";
import { ToolsPage } from "./ToolsPage";
import type { UserTool } from "@/lib/types";

let mockTools: UserTool[] = [];

const server = setupServer(
  http.get("*/api/tools", () => HttpResponse.json(mockTools)),
  http.get("*/api/mcp/servers", () => HttpResponse.json({ servers: [] })),
  http.post("*/api/mcp/catalog/:appId/link", () => HttpResponse.json({ ok: true })),
  http.post("*/api/mcp/catalog/:appId/unlink", () => HttpResponse.json({ ok: true })),
  http.post("*/api/tools", async ({ request }) => {
    const body = (await request.json()) as Partial<UserTool>;
    const created: UserTool = {
      id: `tool-${mockTools.length + 1}`,
      name: body.name || "custom_tool",
      description: body.description || "",
      method: body.method || "GET",
      urlTemplate: body.urlTemplate || "https://api.example.com",
      params: body.params || [],
      bodyTemplate: body.bodyTemplate || "",
      headers: (body.headers as Record<string, string>) || {},
      requireApproval: body.requireApproval ?? false,
      enabled: body.enabled ?? true,
      createdAt: new Date().toISOString(),
      updatedAt: new Date().toISOString(),
    };
    mockTools.push(created);
    return HttpResponse.json(created, { status: 201 });
  }),
  http.patch("*/api/tools/:id", async ({ params, request }) => {
    const body = (await request.json()) as Partial<UserTool>;
    const target = mockTools.find((t) => t.id === params.id);
    if (target) {
      Object.assign(target, body);
      return HttpResponse.json(target);
    }
    return new HttpResponse(null, { status: 404 });
  }),
  http.delete("*/api/tools/:id", ({ params }) => {
    mockTools = mockTools.filter((t) => t.id !== params.id);
    return new HttpResponse(null, { status: 204 });
  }),
);

beforeAll(() => server.listen({ onUnhandledRequest: "error" }));
afterEach(() => {
  cleanup();
  server.resetHandlers();
  mockTools = [];
});
afterAll(() => server.close());

function renderToolsPage() {
  const queryClient = new QueryClient({
    defaultOptions: {
      queries: { retry: false },
    },
  });

  return render(
    <QueryClientProvider client={queryClient}>
      <MemoryRouter>
        <SidebarProvider>
          <ToolsPage />
        </SidebarProvider>
      </MemoryRouter>
    </QueryClientProvider>,
  );
}

describe("ToolsPage UI and UX", () => {
  it("renders empty state with quick starter templates when user has 0 tools", async () => {
    renderToolsPage();

    await waitFor(() => {
      expect(screen.getByText("Equip Herbie with Live Web APIs")).toBeInTheDocument();
    });

    expect(screen.getByText("Quick Install Starters (Zero Configuration)")).toBeInTheDocument();
    expect(screen.getByText("Weather report")).toBeInTheDocument();
    expect(screen.getByText("GitHub repo details")).toBeInTheDocument();
    expect(screen.getByText("Webpage reader")).toBeInTheDocument();
  });

  it("renders user tools and filters them via search bar", async () => {
    mockTools = [
      {
        id: "tool-1",
        name: "get_crypto_price",
        description: "Fetch live cryptocurrency rates and volume",
        method: "GET",
        urlTemplate: "https://api.coingecko.com/api/v3/simple/price",
        params: [{ name: "ids", in: "query", type: "string", required: true, description: "Coin IDs" }],
        bodyTemplate: "",
        headers: {},
        requireApproval: false,
        enabled: true,
        createdAt: "2026-01-01",
        updatedAt: "2026-01-01",
      },
      {
        id: "tool-2",
        name: "send_slack_alert",
        description: "Post a message to team slack channel",
        method: "POST",
        urlTemplate: "https://slack.example.com/api/chat.postMessage",
        params: [],
        bodyTemplate: "{}",
        headers: {},
        requireApproval: true,
        enabled: true,
        createdAt: "2026-01-01",
        updatedAt: "2026-01-01",
      },
    ];

    const user = userEvent.setup();
    renderToolsPage();

    await waitFor(() => {
      expect(screen.getByText("get_crypto_price")).toBeInTheDocument();
      expect(screen.getByText("send_slack_alert")).toBeInTheDocument();
    });

    // Test Search Input
    const searchInput = screen.getByPlaceholderText(/search tools by name/i);
    await user.type(searchInput, "crypto");

    expect(screen.getByText("get_crypto_price")).toBeInTheDocument();
    expect(screen.queryByText("send_slack_alert")).not.toBeInTheDocument();

    // Clear search
    await user.clear(searchInput);
    expect(screen.getByText("send_slack_alert")).toBeInTheDocument();
  });

  it("switches to Template Directory and filters by category and search", async () => {
    const user = userEvent.setup();
    renderToolsPage();

    await waitFor(() => {
      expect(screen.getByRole("button", { name: /template directory/i })).toBeInTheDocument();
    });

    // Switch view to Template Directory
    await user.click(screen.getByRole("button", { name: /template directory/i }));

    // Verify templates are rendered
    expect(screen.getByText("Hacker News search")).toBeInTheDocument();
    expect(screen.getByText("Wikipedia summary")).toBeInTheDocument();

    // Filter by Finance category
    const financeCategory = screen.getByRole("button", { name: /finance/i });
    await user.click(financeCategory);

    expect(screen.getByText("Exchange rates")).toBeInTheDocument();
    expect(screen.getByText("Crypto prices")).toBeInTheDocument();
    expect(screen.queryByText("Wikipedia summary")).not.toBeInTheDocument();

    // Search in templates
    const templateSearch = screen.getByPlaceholderText(/search templates/i);
    await user.type(templateSearch, "exchange");

    expect(screen.getByText("Exchange rates")).toBeInTheDocument();
    expect(screen.queryByText("Crypto prices")).not.toBeInTheDocument();
  });

  it("opens tool inspector details dialog when clicking inspect", async () => {
    mockTools = [
      {
        id: "tool-1",
        name: "test_inspect_tool",
        description: "Tool for testing inspector",
        method: "GET",
        urlTemplate: "https://api.example.com/test",
        params: [{ name: "query", in: "query", type: "string", required: true, description: "Search query" }],
        bodyTemplate: "",
        headers: { "X-Test": "••••" },
        requireApproval: true,
        enabled: true,
        createdAt: "2026-01-01",
        updatedAt: "2026-01-01",
      },
    ];

    const user = userEvent.setup();
    renderToolsPage();

    await waitFor(() => {
      expect(screen.getByText("test_inspect_tool")).toBeInTheDocument();
    });

    const inspectBtn = screen.getByRole("button", { name: /inspect test_inspect_tool/i });
    await user.click(inspectBtn);

    // Inspector modal should appear with tabs
    expect(screen.getByRole("dialog")).toBeInTheDocument();
    expect(screen.getByText("Overview & Parameters")).toBeInTheDocument();
    expect(screen.getByText("Agent Schema")).toBeInTheDocument();
    expect(screen.getByText("cURL Example")).toBeInTheDocument();
  });

  it("displays Code Sandbox tool and allows searching for sandbox", async () => {
    const user = userEvent.setup();
    renderToolsPage();

    // In My Tools, Code Sandbox is listed
    await waitFor(() => {
      expect(screen.getAllByText("Code Sandbox")[0]).toBeInTheDocument();
    });

    // In Template Directory, Code Sandbox is listed and searchable
    await user.click(screen.getByRole("button", { name: /template directory/i }));
    const templateSearch = screen.getByPlaceholderText(/search templates/i);
    await user.type(templateSearch, "sandbox");

    expect(screen.getAllByText("Code Sandbox")[0]).toBeInTheDocument();
  });
});
