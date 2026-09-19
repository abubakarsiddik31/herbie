import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { http, HttpResponse } from "msw";
import { setupServer } from "msw/node";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { MemoryRouter } from "react-router";
import { afterAll, afterEach, beforeAll, describe, expect, it } from "vitest";
import { AppSidebar } from "./AppSidebar";
import { SidebarProvider } from "./SidebarContext";

const mockConversations = [
  {
    id: "conv-1",
    title: "--- File: 2026-q3-forecast.pdf ---\n```pdf\n[binary content]\n```\n\nPlease analyze the attached file(s) above.",
    createdAt: new Date().toISOString(),
    updatedAt: new Date().toISOString(),
    model: "gpt-4o",
    temperature: 0.7,
    systemPrompt: "",
    ragEnabled: false,
  },
  {
    id: "conv-2",
    title: "--- File: audit.docx ---\n```docx\n...\n```\n\nFind compliance violations",
    createdAt: new Date().toISOString(),
    updatedAt: new Date().toISOString(),
    model: "gpt-4o",
    temperature: 0.7,
    systemPrompt: "",
    ragEnabled: false,
  },
  {
    id: "conv-3",
    title: "Regular discussion without files",
    createdAt: new Date().toISOString(),
    updatedAt: new Date().toISOString(),
    model: "gpt-4o",
    temperature: 0.7,
    systemPrompt: "",
    ragEnabled: false,
  },
];

const server = setupServer(
  http.get("*/api/conversations", () => HttpResponse.json(mockConversations)),
  http.get("*/api/tools", () => HttpResponse.json([])),
  http.get("*/api/projects", () => HttpResponse.json({ projects: [] })),
  http.get("*/api/workflows", () => HttpResponse.json({ workflows: [] })),
  http.get("*/api/tool-oauth/providers", () => HttpResponse.json([])),
  http.get("*/api/mcp/servers", () => HttpResponse.json({ servers: [] })),
);

beforeAll(() => server.listen());
afterEach(() => {
  cleanup();
  server.resetHandlers();
});
afterAll(() => server.close());

function renderSidebar() {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return render(
    <QueryClientProvider client={queryClient}>
      <MemoryRouter initialEntries={["/chat/conv-1"]}>
        <SidebarProvider>
          <AppSidebar />
        </SidebarProvider>
      </MemoryRouter>
    </QueryClientProvider>
  );
}

describe("AppSidebar", () => {
  it("cleans file conversation titles and does not show raw file wrappers", async () => {
    renderSidebar();

    // The first conv should be displayed cleanly as "2026-q3-forecast.pdf", NOT "--- File: ..."
    expect(await screen.findByText("2026-q3-forecast.pdf")).toBeInTheDocument();
    expect(screen.queryByText(/--- File:/i)).not.toBeInTheDocument();

    // The second conv had a custom prompt "Find compliance violations", so that prompt should be the title
    expect(screen.getByText("Find compliance violations")).toBeInTheDocument();

    // Regular conversation
    expect(screen.getByText("Regular discussion without files")).toBeInTheDocument();
  });

  it("renders options dropdown menu and opens rename dialog with clean title", async () => {
    const user = userEvent.setup();
    renderSidebar();

    await screen.findByText("2026-q3-forecast.pdf");

    // Options button for active conversation conv-1
    const optionsBtn = screen.getByRole("button", { name: "Options for 2026-q3-forecast.pdf" });
    expect(optionsBtn).toBeInTheDocument();
    await user.click(optionsBtn);

    // Dropdown items should be visible
    const renameItem = await screen.findByRole("menuitem", { name: "Rename 2026-q3-forecast.pdf" });
    expect(screen.getByRole("menuitem", { name: "Share 2026-q3-forecast.pdf" })).toBeInTheDocument();
    expect(screen.getByRole("menuitem", { name: "Delete 2026-q3-forecast.pdf" })).toBeInTheDocument();

    // Click rename
    await user.click(renameItem);

    // Rename dialog should appear with clean title in the input
    expect(await screen.findByRole("heading", { name: "Rename conversation" })).toBeInTheDocument();
    const titleInput = screen.getByLabelText("Title");
    expect(titleInput).toHaveValue("2026-q3-forecast.pdf");
  });

  it("opens delete confirmation dialog from options menu", async () => {
    const user = userEvent.setup();
    renderSidebar();

    await screen.findByText("2026-q3-forecast.pdf");

    const optionsBtn = screen.getByRole("button", { name: "Options for 2026-q3-forecast.pdf" });
    await user.click(optionsBtn);

    const deleteItem = await screen.findByRole("menuitem", { name: "Delete 2026-q3-forecast.pdf" });
    await user.click(deleteItem);

    expect(await screen.findByRole("heading", { name: "Delete conversation?" })).toBeInTheDocument();
    expect(screen.getByText(/and all of its messages will be permanently removed/)).toBeInTheDocument();
  });

  it("renders Apps & MCP navigation item with neutral icon and aligned count badge", async () => {
    const user = userEvent.setup();
    renderSidebar();

    // Verify Apps & MCP button exists
    const appsBtn = await screen.findByRole("button", { name: /Apps & MCP/i });
    expect(appsBtn).toBeInTheDocument();

    // Verify the icon inside does not have purple styling
    const plugIcon = appsBtn.querySelector("svg");
    expect(plugIcon).toBeInTheDocument();
    expect(plugIcon?.className).not.toContain("text-purple-500");

    // Default count is 1 for built-in web search
    expect(screen.getByText("Apps & MCP")).toBeInTheDocument();
    expect(appsBtn.textContent).toContain("1");

    // Apps & MCP is expanded by default -> Web Search is visible
    expect(await screen.findByText("Web Search")).toBeInTheDocument();
    expect(screen.getByText("Add & Manage Apps")).toBeInTheDocument();

    // Clicking appsBtn collapses it
    await user.click(appsBtn);
    expect(screen.queryByText("Web Search")).not.toBeInTheDocument();

    // Clicking again expands it
    await user.click(appsBtn);
    expect(await screen.findByText("Web Search")).toBeInTheDocument();
  });
});
